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
	"github.com/barzoj/yaralpho/internal/notify"
	"github.com/barzoj/yaralpho/internal/queue"
	"github.com/barzoj/yaralpho/internal/storage"
	"github.com/barzoj/yaralpho/internal/tracker"
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
func (f *fakeStorage) ListTaskRuns(ctx context.Context, batchID string) ([]storage.TaskRunSummary, error) {
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
func (fakeTracker) AddComment(ctx context.Context, ref string, text string) error { return nil }
func (fakeTracker) FetchComments(ctx context.Context, ref string) ([]tracker.Comment, error) {
	return nil, nil
}

func (fakeTracker) GetTitle(ctx context.Context, ref string) (string, error) {
	return "", nil
}

type fakeNotifier struct{}

func (fakeNotifier) NotifyEvent(ctx context.Context, event notify.Event) error { return nil }

func (fakeNotifier) NotifyTaskFinished(ctx context.Context, batchID, runID, taskRef, taskName, status, commitHash string) error {
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

type taggedCopilot struct {
	tag string
}

func (t taggedCopilot) StartSession(ctx context.Context, prompt, repoPath string) (string, <-chan copilot.RawEvent, func(), error) {
	ch := make(chan copilot.RawEvent)
	close(ch)
	return t.tag, ch, func() {}, nil
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

func TestBuildWithOptionsSelectsAgent(t *testing.T) {
	cfg := fakeConfig{
		config.PortKey:     "0",
		config.MongoURIKey: "mongodb://example",
		config.MongoDBKey:  "db",
		config.RepoPathKey: "/repo",
		config.BdRepoKey:   "/repo",
	}

	origNewStorage := newStorage
	origNewTracker := newTracker
	origNewNotifier := newNotifier
	origNewGitHubClient := newGitHubClient
	origNewCodexClient := newCodexClient
	origNewWorker := newWorker
	t.Cleanup(func() {
		newStorage = origNewStorage
		newTracker = origNewTracker
		newNotifier = origNewNotifier
		newGitHubClient = origNewGitHubClient
		newCodexClient = origNewCodexClient
		newWorker = origNewWorker
	})

	newStorage = func(ctx context.Context, uri, db string, logger *zap.Logger) (storage.Storage, error) {
		return &fakeStorage{}, nil
	}
	newTracker = func(cfg config.Config, logger *zap.Logger) (tracker.Tracker, error) {
		return fakeTracker{}, nil
	}
	newNotifier = func(cfg config.Config, logger *zap.Logger) (notify.Notifier, error) {
		return fakeNotifier{}, nil
	}
	newGitHubClient = func(logger *zap.Logger) copilot.Client {
		return taggedCopilot{tag: "github"}
	}
	newCodexClient = func(logger *zap.Logger) copilot.Client {
		return taggedCopilot{tag: "codex"}
	}

	tests := []struct {
		name      string
		agent     string
		wantTag   string
		wantError string
	}{
		{name: "default agent uses codex", agent: "", wantTag: "codex"},
		{name: "explicit codex", agent: "codex", wantTag: "codex"},
		{name: "explicit github", agent: "github", wantTag: "github"},
		{name: "invalid agent errors", agent: "invalid", wantError: "unknown agent"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var selected copilot.Client
			newWorker = func(q queue.Queue, tr tracker.Tracker, cp copilot.Client, st storage.Storage, nt notify.Notifier, cfg config.Config, repoPath string, logger *zap.Logger) Consumer {
				selected = cp
				return &fakeConsumer{}
			}

			a, err := BuildWithOptions(context.Background(), zap.NewNop(), cfg, BuildOptions{Agent: tc.agent})
			if tc.wantError != "" {
				require.Error(t, err)
				require.ErrorContains(t, err, tc.wantError)
				require.Nil(t, a)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, a)
			cp, ok := selected.(taggedCopilot)
			require.True(t, ok)
			require.Equal(t, tc.wantTag, cp.tag)
		})
	}
}
