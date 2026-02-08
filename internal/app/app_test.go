package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/barzoj/yaralpho/internal/config"
	"github.com/barzoj/yaralpho/internal/copilot"
	"github.com/barzoj/yaralpho/internal/queue"
	"github.com/barzoj/yaralpho/internal/storage"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeConfig map[string]string

func (f fakeConfig) Get(key string) (string, error) {
	if val, ok := f[key]; ok {
		return val, nil
	}
	return "", errors.New("not found")
}

type fakeStorage struct {
	closeCalled bool
}

func (f *fakeStorage) CreateBatch(ctx context.Context, batch *storage.Batch) error { return nil }
func (f *fakeStorage) UpdateBatch(ctx context.Context, batch *storage.Batch) error { return nil }
func (f *fakeStorage) GetBatch(ctx context.Context, batchID string) (*storage.Batch, error) {
	return nil, nil
}
func (f *fakeStorage) ListBatches(ctx context.Context, limit int64) ([]storage.Batch, error) {
	return nil, nil
}
func (f *fakeStorage) CreateTaskRun(ctx context.Context, run *storage.TaskRun) error { return nil }
func (f *fakeStorage) UpdateTaskRun(ctx context.Context, run *storage.TaskRun) error { return nil }
func (f *fakeStorage) GetTaskRun(ctx context.Context, runID string) (*storage.TaskRun, error) {
	return nil, nil
}
func (f *fakeStorage) ListTaskRuns(ctx context.Context, batchID string) ([]storage.TaskRun, error) {
	return nil, nil
}
func (f *fakeStorage) InsertSessionEvent(ctx context.Context, event *storage.SessionEvent) error {
	return nil
}
func (f *fakeStorage) ListSessionEvents(ctx context.Context, sessionID string) ([]storage.SessionEvent, error) {
	return nil, nil
}
func (f *fakeStorage) GetBatchProgress(ctx context.Context, batchID string) (storage.BatchProgress, error) {
	return storage.BatchProgress{}, nil
}
func (f *fakeStorage) Close(ctx context.Context) error {
	f.closeCalled = true
	return nil
}

type fakeTracker struct{}

func (fakeTracker) IsEpic(ctx context.Context, ref string) (bool, error) { return false, nil }
func (fakeTracker) ListChildren(ctx context.Context, ref string) ([]string, error) {
	return nil, nil
}

type fakeNotifier struct{}

func (fakeNotifier) NotifyTaskFinished(ctx context.Context, batchID, runID, taskRef, status, commitHash string) error {
	return nil
}
func (fakeNotifier) NotifyBatchIdle(ctx context.Context, batchID string) error { return nil }
func (fakeNotifier) NotifyError(ctx context.Context, batchID, runID, taskRef string, err error) error {
	return nil
}

type fakeCopilot struct{}

func (fakeCopilot) StartSession(ctx context.Context, prompt, repoPath string) (string, <-chan copilot.RawEvent, func(), error) {
	ch := make(chan copilot.RawEvent)
	close(ch)
	return "s1", ch, func() {}, nil
}

type fakeConsumer struct {
	mu    sync.Mutex
	calls int
}

func (f *fakeConsumer) Run(ctx context.Context) error {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	<-ctx.Done()
	return ctx.Err()
}

func TestNewValidatesDependencies(t *testing.T) {
	_, err := New(nil, nil, nil, nil, nil, nil, nil, nil)
	require.Error(t, err)
}

func TestHealthRoute(t *testing.T) {
	cfg := fakeConfig{
		config.PortKey:     "0",
		config.MongoURIKey: "mongodb://example",
		config.MongoDBKey:  "db",
		config.RepoPathKey: "/repo",
		config.BdRepoKey:   "/repo",
	}

	st := &fakeStorage{}
	q := queue.NewMemoryQueue(zap.NewNop())
	tr := &fakeTracker{}
	nt := fakeNotifier{}
	cp := fakeCopilot{}
	cons := &fakeConsumer{}

	a, err := New(zap.NewNop(), cfg, st, q, tr, nt, cp, cons)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	a.Router().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"ok"`)
}

func TestRunStartsConsumerOnce(t *testing.T) {
	cfg := fakeConfig{
		config.PortKey:     "0",
		config.MongoURIKey: "mongodb://example",
		config.MongoDBKey:  "db",
		config.RepoPathKey: "/repo",
		config.BdRepoKey:   "/repo",
	}

	st := &fakeStorage{}
	q := queue.NewMemoryQueue(zap.NewNop())
	tr := &fakeTracker{}
	nt := fakeNotifier{}
	cp := fakeCopilot{}
	cons := &fakeConsumer{}

	a, err := New(zap.NewNop(), cfg, st, q, tr, nt, cp, cons)
	require.NoError(t, err)

	a.server.Addr = "127.0.0.1:0"

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	require.NoError(t, a.Run(ctx))

	cons.mu.Lock()
	calls := cons.calls
	cons.mu.Unlock()
	require.Equal(t, 1, calls)
	require.True(t, st.closeCalled)

	err = a.Run(context.Background())
	require.Error(t, err)
}
