package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/barzoj/yaralpho/internal/config"
	"github.com/barzoj/yaralpho/internal/copilot"
	"github.com/barzoj/yaralpho/internal/notify"
	"github.com/barzoj/yaralpho/internal/scheduler"
	"github.com/barzoj/yaralpho/internal/storage"
	"github.com/barzoj/yaralpho/internal/tracker"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
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

func (f *fakeStorage) CreateRepository(ctx context.Context, repo *storage.Repository) error {
	return nil
}
func (f *fakeStorage) UpdateRepository(ctx context.Context, repo *storage.Repository) error {
	return nil
}
func (f *fakeStorage) GetRepository(ctx context.Context, id string) (*storage.Repository, error) {
	return nil, mongo.ErrNoDocuments
}
func (f *fakeStorage) ListRepositories(ctx context.Context) ([]storage.Repository, error) {
	return nil, nil
}
func (f *fakeStorage) DeleteRepository(ctx context.Context, id string) error { return nil }
func (f *fakeStorage) RepositoryHasActiveBatches(ctx context.Context, id string) (bool, error) {
	return false, nil
}

func (f *fakeStorage) CreateAgent(ctx context.Context, agent *storage.Agent) error { return nil }
func (f *fakeStorage) UpdateAgent(ctx context.Context, agent *storage.Agent) error { return nil }
func (f *fakeStorage) GetAgent(ctx context.Context, id string) (*storage.Agent, error) {
	return nil, mongo.ErrNoDocuments
}
func (f *fakeStorage) ListAgents(ctx context.Context) ([]storage.Agent, error) { return nil, nil }
func (f *fakeStorage) DeleteAgent(ctx context.Context, id string) error        { return nil }

func (f *fakeStorage) CreateBatch(ctx context.Context, batch *storage.Batch) error { return nil }
func (f *fakeStorage) UpdateBatch(ctx context.Context, batch *storage.Batch) error { return nil }
func (f *fakeStorage) GetBatch(ctx context.Context, batchID string) (*storage.Batch, error) {
	return nil, nil
}
func (f *fakeStorage) ListBatches(ctx context.Context, limit int64) ([]storage.Batch, error) {
	return nil, nil
}
func (f *fakeStorage) ListBatchesByRepository(ctx context.Context, repositoryID string, status storage.BatchStatus, limit int64) ([]storage.Batch, error) {
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
func (f *fakeStorage) ListTaskRunsByRepository(ctx context.Context, repositoryID string) ([]storage.TaskRunSummary, error) {
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

func (fakeTracker) AddComment(ctx context.Context, repoPath, ref string, text string) error {
	return nil
}
func (fakeTracker) FetchComments(ctx context.Context, repoPath, ref string) ([]tracker.Comment, error) {
	return nil, nil
}

func (fakeTracker) GetTitle(ctx context.Context, repoPath, ref string) (string, error) {
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

func TestNewValidatesDependencies(t *testing.T) {
	_, err := New(nil, nil, nil, nil, nil, nil)
	require.Error(t, err)
}

func TestHealthRoute(t *testing.T) {
	cfg := fakeConfig{
		config.PortKey:     "0",
		config.MongoURIKey: "mongodb://example",
		config.MongoDBKey:  "db",
		config.RepoPathKey: "/repo",
	}

	st := &fakeStorage{}
	tr := &fakeTracker{}
	nt := fakeNotifier{}
	cp := fakeCopilot{}

	a, err := New(zap.NewNop(), cfg, st, tr, nt, cp)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	a.Router().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"ok"`)
}

func TestVersionRoute(t *testing.T) {
	originalVersion := Version
	Version = "abc123"
	t.Cleanup(func() { Version = originalVersion })

	cfg := fakeConfig{
		config.PortKey:     "0",
		config.MongoURIKey: "mongodb://example",
		config.MongoDBKey:  "db",
		config.RepoPathKey: "/repo",
	}

	st := &fakeStorage{}
	tr := &fakeTracker{}
	nt := fakeNotifier{}
	cp := fakeCopilot{}

	a, err := New(zap.NewNop(), cfg, st, tr, nt, cp)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	rec := httptest.NewRecorder()
	a.Router().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"version":"abc123"`)
}

func TestVersionRouteDefaultsToDev(t *testing.T) {
	originalVersion := Version
	Version = "dev"
	t.Cleanup(func() { Version = originalVersion })

	cfg := fakeConfig{
		config.PortKey:     "0",
		config.MongoURIKey: "mongodb://example",
		config.MongoDBKey:  "db",
		config.RepoPathKey: "/repo",
	}

	st := &fakeStorage{}
	tr := &fakeTracker{}
	nt := fakeNotifier{}
	cp := fakeCopilot{}

	a, err := New(zap.NewNop(), cfg, st, tr, nt, cp)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	rec := httptest.NewRecorder()
	a.Router().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"version":"dev"`)
}

// BuildWithOptions currently mirrors Build; retained for compatibility only.
func TestBuildWithOptions_SameAsBuild(t *testing.T) {
	cfg := fakeConfig{
		config.PortKey:     "0",
		config.MongoURIKey: "mongodb://example",
		config.MongoDBKey:  "db",
		config.RepoPathKey: "/repo",
	}

	origNewStorage := newStorage
	origNewTracker := newTracker
	origNewNotifier := newNotifier
	t.Cleanup(func() {
		newStorage = origNewStorage
		newTracker = origNewTracker
		newNotifier = origNewNotifier
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

	a, err := BuildWithOptions(context.Background(), zap.NewNop(), cfg, BuildOptions{})
	require.NoError(t, err)
	require.NotNil(t, a)
	_, ok := a.copilot.(copilot.Client)
	require.True(t, ok)
}

func TestBuildWithOptions_WiresSchedulerOptionsFromConfig(t *testing.T) {
	cfg := fakeConfig{
		config.PortKey:              "0",
		config.MongoURIKey:          "mongodb://example",
		config.MongoDBKey:           "db",
		config.RepoPathKey:          "/repo",
		config.SchedulerIntervalKey: "250ms",
		config.MaxRetriesKey:        "9",
	}

	origNewStorage := newStorage
	origNewTracker := newTracker
	origNewNotifier := newNotifier
	origNewScheduler := newScheduler
	t.Cleanup(func() {
		newStorage = origNewStorage
		newTracker = origNewTracker
		newNotifier = origNewNotifier
		newScheduler = origNewScheduler
	})

	newStorage = func(ctx context.Context, uri, dbName string, logger *zap.Logger) (storage.Storage, error) {
		return &fakeStorage{}, nil
	}
	newTracker = func(cfg config.Config, logger *zap.Logger) (tracker.Tracker, error) {
		return fakeTracker{}, nil
	}
	newNotifier = func(cfg config.Config, logger *zap.Logger) (notify.Notifier, error) {
		return fakeNotifier{}, nil
	}

	var captured scheduler.Options
	newScheduler = func(st scheduler.Storage, worker scheduler.Worker, logger *zap.Logger, opts scheduler.Options) schedulerController {
		captured = opts
		return fakeSchedulerController{}
	}

	a, err := BuildWithOptions(context.Background(), zap.NewNop(), cfg, BuildOptions{})
	require.NoError(t, err)
	require.NotNil(t, a)

	require.Equal(t, 250*time.Millisecond, captured.Interval)
	require.Equal(t, 9, captured.MaxRetries)
}

type fakeSchedulerController struct{}

func (fakeSchedulerController) SetDraining(bool)                      {}
func (fakeSchedulerController) Draining() bool                        { return false }
func (fakeSchedulerController) ActiveCount() int                      { return 0 }
func (fakeSchedulerController) WaitForIdle(ctx context.Context) error { return nil }
func (fakeSchedulerController) Tick(ctx context.Context) error        { return nil }
func (fakeSchedulerController) Stop(ctx context.Context) error        { return nil }

type tickingScheduler struct {
	tickCount int
}

func (t *tickingScheduler) SetDraining(bool)                      {}
func (t *tickingScheduler) Draining() bool                        { return false }
func (t *tickingScheduler) ActiveCount() int                      { return 0 }
func (t *tickingScheduler) WaitForIdle(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }
func (t *tickingScheduler) Tick(ctx context.Context) error {
	t.tickCount++
	return ctx.Err()
}
func (t *tickingScheduler) Stop(ctx context.Context) error { _ = ctx; return nil }

type drainingScheduler struct {
	active     atomic.Int64
	draining   atomic.Bool
	waitCalled atomic.Bool
	waitCh     chan struct{}
	waitErr    error
	stopCount  atomic.Int64
}

func (d *drainingScheduler) SetDraining(draining bool) { d.draining.Store(draining) }
func (d *drainingScheduler) Draining() bool            { return d.draining.Load() }
func (d *drainingScheduler) ActiveCount() int          { return int(d.active.Load()) }
func (d *drainingScheduler) WaitForIdle(ctx context.Context) error {
	d.waitCalled.Store(true)
	if d.waitCh == nil {
		return d.waitErr
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-d.waitCh:
		return d.waitErr
	}
}
func (d *drainingScheduler) Tick(ctx context.Context) error { return ctx.Err() }
func (d *drainingScheduler) Stop(ctx context.Context) error {
	_ = ctx
	d.draining.Store(true)
	d.stopCount.Add(1)
	return nil
}

func TestRestartWaitTriggersShutdown(t *testing.T) {
	st := newHandlerTestStorage()
	app := newTestApp(t, st)
	if cfg, ok := app.cfg.(fakeConfig); ok {
		cfg[config.RestartWaitTimeoutKey] = "200ms"
	}

	sched := &drainingScheduler{waitCh: make(chan struct{})}
	sched.active.Store(1)
	app.SetScheduler(sched)

	runCtx, runCancel := context.WithCancel(context.Background())
	defer runCancel()

	runDone := make(chan error, 1)
	go func() {
		runDone <- app.Run(runCtx)
	}()

	// Allow server goroutine to start.
	time.Sleep(10 * time.Millisecond)

	req := httptest.NewRequest(http.MethodPost, "/restart?wait=true", nil)
	rec := httptest.NewRecorder()
	handlerDone := make(chan struct{})
	go func() {
		app.Router().ServeHTTP(rec, req)
		close(handlerDone)
	}()

	select {
	case <-handlerDone:
		t.Fatalf("handler returned before draining completed")
	case <-time.After(30 * time.Millisecond):
	}

	sched.active.Store(0)
	close(sched.waitCh)

	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatalf("handler did not return after scheduler drained")
	}

	require.Equal(t, http.StatusOK, rec.Code)
	require.True(t, sched.waitCalled.Load())
	require.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	require.Contains(t, rec.Body.String(), `"status":"drained"`)
	require.GreaterOrEqual(t, sched.stopCount.Load(), int64(1))

	select {
	case err := <-runDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatalf("app did not shut down after restart wait")
	}
}

func TestRestartNoWaitKeepsRunning(t *testing.T) {
	st := newHandlerTestStorage()
	app := newTestApp(t, st)

	sched := &drainingScheduler{}
	sched.active.Store(2)
	app.SetScheduler(sched)

	runCtx, runCancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() {
		runDone <- app.Run(runCtx)
	}()

	// Allow server goroutine to start.
	time.Sleep(10 * time.Millisecond)

	req := httptest.NewRequest(http.MethodPost, "/restart", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)

	require.Equal(t, http.StatusAccepted, rec.Code)
	require.True(t, sched.draining.Load())
	require.False(t, sched.waitCalled.Load())
	require.Equal(t, int64(0), sched.stopCount.Load())

	select {
	case err := <-runDone:
		t.Fatalf("app exited unexpectedly: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	runCancel()
	select {
	case err := <-runDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatalf("app did not exit after cancellation")
	}
}

func TestRunScheduler_TicksUntilContextCancel(t *testing.T) {
	cfg := fakeConfig{
		config.PortKey:              "0",
		config.MongoURIKey:          "mongodb://example",
		config.MongoDBKey:           "db",
		config.RepoPathKey:          "/repo",
		config.SchedulerIntervalKey: "5ms",
	}

	st := &fakeStorage{}
	app, err := New(zap.NewNop(), cfg, st, noopTracker{}, noopNotifier{}, noopCopilot{})
	require.NoError(t, err)

	sched := &tickingScheduler{}
	app.SetScheduler(sched)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		app.runScheduler(ctx)
		close(done)
	}()

	time.Sleep(12 * time.Millisecond)
	cancel()
	<-done

	require.GreaterOrEqual(t, sched.tickCount, 1)
}
