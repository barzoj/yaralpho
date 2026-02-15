package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/barzoj/yaralpho/internal/config"
	"github.com/barzoj/yaralpho/internal/copilot"
	"github.com/barzoj/yaralpho/internal/notify"
	"github.com/barzoj/yaralpho/internal/storage"
	"github.com/barzoj/yaralpho/internal/tracker"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// --- fakes -------------------------------------------------------------------

type handlerTestStorage struct {
	batches  map[string]storage.Batch
	runs     map[string]storage.TaskRun
	events   map[string][]storage.SessionEvent
	progress map[string]storage.BatchProgress
	repos    map[string]storage.Repository
	agents   map[string]storage.Agent
}

func newHandlerTestStorage() *handlerTestStorage {
	return &handlerTestStorage{
		batches:  make(map[string]storage.Batch),
		runs:     make(map[string]storage.TaskRun),
		events:   make(map[string][]storage.SessionEvent),
		progress: make(map[string]storage.BatchProgress),
		repos:    make(map[string]storage.Repository),
		agents:   make(map[string]storage.Agent),
	}
}

// repository methods ---------------------------------------------------------

func (s *handlerTestStorage) CreateRepository(ctx context.Context, repo *storage.Repository) error {
	for _, existing := range s.repos {
		if existing.Name == repo.Name || existing.Path == repo.Path {
			return storage.ErrConflict
		}
	}
	s.repos[repo.ID] = *repo
	return nil
}
func (s *handlerTestStorage) UpdateRepository(ctx context.Context, repo *storage.Repository) error {
	for id, existing := range s.repos {
		if id == repo.ID {
			continue
		}
		if existing.Name == repo.Name || existing.Path == repo.Path {
			return storage.ErrConflict
		}
	}
	s.repos[repo.ID] = *repo
	return nil
}
func (s *handlerTestStorage) GetRepository(ctx context.Context, id string) (*storage.Repository, error) {
	r, ok := s.repos[id]
	if !ok {
		return nil, mongo.ErrNoDocuments
	}
	return &r, nil
}
func (s *handlerTestStorage) ListRepositories(ctx context.Context) ([]storage.Repository, error) {
	out := make([]storage.Repository, 0, len(s.repos))
	for _, r := range s.repos {
		out = append(out, r)
	}
	return out, nil
}
func (s *handlerTestStorage) DeleteRepository(ctx context.Context, id string) error {
	delete(s.repos, id)
	return nil
}
func (s *handlerTestStorage) RepositoryHasActiveBatches(ctx context.Context, id string) (bool, error) {
	for _, b := range s.batches {
		if b.RepositoryID == id && b.Status != storage.BatchStatusDone && b.Status != storage.BatchStatusFailed {
			return true, nil
		}
	}
	return false, nil
}

// agent methods -------------------------------------------------------------

func (s *handlerTestStorage) CreateAgent(ctx context.Context, agent *storage.Agent) error {
	for _, existing := range s.agents {
		if existing.Name == agent.Name {
			return storage.ErrConflict
		}
	}
	s.agents[agent.ID] = *agent
	return nil
}
func (s *handlerTestStorage) UpdateAgent(ctx context.Context, agent *storage.Agent) error {
	for id, existing := range s.agents {
		if id == agent.ID {
			continue
		}
		if existing.Name == agent.Name {
			return storage.ErrConflict
		}
	}
	s.agents[agent.ID] = *agent
	return nil
}
func (s *handlerTestStorage) GetAgent(ctx context.Context, id string) (*storage.Agent, error) {
	a, ok := s.agents[id]
	if !ok {
		return nil, mongo.ErrNoDocuments
	}
	return &a, nil
}
func (s *handlerTestStorage) ListAgents(ctx context.Context) ([]storage.Agent, error) {
	out := make([]storage.Agent, 0, len(s.agents))
	for _, a := range s.agents {
		out = append(out, a)
	}
	return out, nil
}
func (s *handlerTestStorage) DeleteAgent(ctx context.Context, id string) error {
	delete(s.agents, id)
	return nil
}

func (s *handlerTestStorage) CreateBatch(ctx context.Context, batch *storage.Batch) error {
	s.batches[batch.ID] = *batch
	return nil
}
func (s *handlerTestStorage) UpdateBatch(ctx context.Context, batch *storage.Batch) error {
	s.batches[batch.ID] = *batch
	return nil
}
func (s *handlerTestStorage) GetBatch(ctx context.Context, batchID string) (*storage.Batch, error) {
	b, ok := s.batches[batchID]
	if !ok {
		return nil, mongo.ErrNoDocuments
	}
	return &b, nil
}
func (s *handlerTestStorage) ListBatches(ctx context.Context, limit int64) ([]storage.Batch, error) {
	out := make([]storage.Batch, 0, len(s.batches))
	for _, b := range s.batches {
		out = append(out, b)
	}
	if limit > 0 && int64(len(out)) > limit {
		out = out[:limit]
	}
	return out, nil
}
func (s *handlerTestStorage) ListBatchesByRepository(ctx context.Context, repositoryID string, status storage.BatchStatus, limit int64) ([]storage.Batch, error) {
	out := make([]storage.Batch, 0)
	for _, b := range s.batches {
		if b.RepositoryID != repositoryID {
			continue
		}
		if status != "" && b.Status != status {
			continue
		}
		out = append(out, b)
	}
	if limit > 0 && int64(len(out)) > limit {
		out = out[:limit]
	}
	return out, nil
}
func (s *handlerTestStorage) CreateTaskRun(ctx context.Context, run *storage.TaskRun) error {
	s.runs[run.ID] = *run
	return nil
}
func (s *handlerTestStorage) UpdateTaskRun(ctx context.Context, run *storage.TaskRun) error {
	s.runs[run.ID] = *run
	return nil
}
func (s *handlerTestStorage) GetTaskRun(ctx context.Context, runID string) (*storage.TaskRun, error) {
	r, ok := s.runs[runID]
	if !ok {
		return nil, mongo.ErrNoDocuments
	}
	return &r, nil
}
func (s *handlerTestStorage) ListTaskRuns(ctx context.Context, batchID string) ([]storage.TaskRunSummary, error) {
	out := []storage.TaskRunSummary{}
	for _, r := range s.runs {
		if batchID == "" || r.BatchID == batchID {
			var total int64
			for _, events := range s.events {
				for _, evt := range events {
					if evt.RunID == r.ID {
						total++
					}
				}
			}
			out = append(out, storage.TaskRunSummary{TaskRun: r, TotalEvents: total})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	return out, nil
}
func (s *handlerTestStorage) ListTaskRunsByRepository(ctx context.Context, repositoryID string) ([]storage.TaskRunSummary, error) {
	out := []storage.TaskRunSummary{}
	for _, r := range s.runs {
		if r.RepositoryID == repositoryID {
			out = append(out, storage.TaskRunSummary{TaskRun: r})
		}
	}
	return out, nil
}
func (s *handlerTestStorage) InsertSessionEvent(ctx context.Context, event *storage.SessionEvent) error {
	s.events[event.SessionID] = append(s.events[event.SessionID], *event)
	return nil
}
func (s *handlerTestStorage) ListSessionEvents(ctx context.Context, sessionID string) ([]storage.SessionEvent, error) {
	return s.events[sessionID], nil
}
func (s *handlerTestStorage) GetBatchProgress(ctx context.Context, batchID string) (storage.BatchProgress, error) {
	if p, ok := s.progress[batchID]; ok {
		return p, nil
	}
	return storage.BatchProgress{}, mongo.ErrNoDocuments
}

type noopTracker struct{}

func (noopTracker) AddComment(ctx context.Context, repoPath, ref string, text string) error { return nil }
func (noopTracker) FetchComments(ctx context.Context, repoPath, ref string) ([]tracker.Comment, error) {
	return nil, nil
}

func (noopTracker) GetTitle(ctx context.Context, repoPath, ref string) (string, error) {
	return "", nil
}

type noopNotifier struct{}

func (noopNotifier) NotifyEvent(ctx context.Context, event notify.Event) error { return nil }

func (noopNotifier) NotifyTaskFinished(ctx context.Context, batchID, runID, taskRef, taskName, status, commitHash string) error {
	return nil
}
func (noopNotifier) NotifyBatchIdle(ctx context.Context, batchID string) error { return nil }
func (noopNotifier) NotifyError(ctx context.Context, batchID, runID, taskRef string, err error) error {
	return nil
}

type noopCopilot struct{}

func (noopCopilot) StartSession(ctx context.Context, prompt, repoPath string) (string, <-chan copilot.RawEvent, func(), error) {
	ch := make(chan copilot.RawEvent)
	close(ch)
	return "s", ch, func() {}, nil
}

// --- helpers -----------------------------------------------------------------

func newTestApp(t *testing.T, st *handlerTestStorage) *App {
	t.Helper()
	return newTestAppWithLogger(t, st, zap.NewNop())
}

func newTestAppWithLogger(t *testing.T, st *handlerTestStorage, logger *zap.Logger) *App {
	t.Helper()
	cfg := fakeConfig{
		config.PortKey:     "0",
		config.MongoURIKey: "mongodb://example",
		config.MongoDBKey:  "db",
		config.RepoPathKey: "/repo",
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	app, err := New(logger, cfg, st, noopTracker{}, noopNotifier{}, noopCopilot{})
	require.NoError(t, err)
	return app
}

func assertLogField(t *testing.T, entry observer.LoggedEntry, key string, expected any) {
	t.Helper()
	for _, f := range entry.Context {
		if f.Key == key {
			switch f.Type {
			case zapcore.StringType:
				require.Equal(t, expected, f.String)
			case zapcore.Int64Type, zapcore.Int32Type, zapcore.Int16Type, zapcore.Int8Type:
				require.EqualValues(t, expected, f.Integer)
			default:
				require.EqualValues(t, expected, f.Interface)
			}
			return
		}
	}
	t.Fatalf("field %s not found", key)
}

// --- tests -------------------------------------------------------------------

func TestBatchCreateUnderRepository(t *testing.T) {
	st := newHandlerTestStorage()
	repoID := "repo-1"
	st.repos[repoID] = storage.Repository{ID: repoID, Name: "Test Repo", Path: "/tmp/repo"}
	app := newTestApp(t, st)

	body := bytes.NewBufferString(`{"items":["T1","T2"],"session_name":"test"}`)
	req := httptest.NewRequest(http.MethodPost, "/repository/repo-1/add", body)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	batchID, _ := resp["batch_id"].(string)
	require.NotEmpty(t, batchID)
	require.Equal(t, "pending", resp["status"])
	require.Equal(t, repoID, resp["repository_id"])
	batch, ok := st.batches[batchID]
	require.True(t, ok)
	require.Equal(t, repoID, batch.RepositoryID)
	require.Equal(t, storage.BatchStatusPending, batch.Status)
	require.Equal(t, "test", batch.SessionName)
	require.Len(t, batch.Items, 2)
	for _, it := range batch.Items {
		require.Equal(t, storage.ItemStatusPending, it.Status)
		require.Equal(t, 0, it.Attempts)
	}

	// list batches
	req = httptest.NewRequest(http.MethodGet, "/batches", nil)
	rec = httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var listResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	require.Equal(t, float64(1), listResp["count"])
}

func TestBatchCreateMissingRepository(t *testing.T) {
	st := newHandlerTestStorage()
	app := newTestApp(t, st)

	req := httptest.NewRequest(http.MethodPost, "/repository/missing/add", bytes.NewBufferString(`{"items":["T1"]}`))
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Empty(t, st.batches)
}

func TestBatchCreateNoItems(t *testing.T) {
	st := newHandlerTestStorage()
	repoID := "repo-1"
	st.repos[repoID] = storage.Repository{ID: repoID, Name: "Test Repo", Path: "/tmp/repo"}
	app := newTestApp(t, st)

	req := httptest.NewRequest(http.MethodPost, "/repository/repo-1/add", bytes.NewBufferString(`{"items":[]}`))
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Empty(t, st.batches)
}

func TestPauseBatch(t *testing.T) {
	st := newHandlerTestStorage()
	repoID := "repo-1"
	batchID := "batch-1"
	now := time.Now().Add(-time.Minute).UTC()
	st.repos[repoID] = storage.Repository{ID: repoID, Name: "Repo", Path: "/tmp/repo"}
	st.batches[batchID] = storage.Batch{
		ID:           batchID,
		RepositoryID: repoID,
		Status:       storage.BatchStatusInProgress,
		CreatedAt:    now,
		UpdatedAt:    now,
		Items: []storage.BatchItem{
			{Input: "task1", Status: storage.ItemStatusInProgress, Attempts: 1},
		},
	}
	app := newTestApp(t, st)

	req := httptest.NewRequest(http.MethodPut, "/repository/repo-1/batch/batch-1/pause", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, batchID, resp["batch_id"])
	require.Equal(t, string(storage.BatchStatusPaused), resp["status"])
	require.Equal(t, repoID, resp["repository_id"])

	updated := st.batches[batchID]
	require.Equal(t, storage.BatchStatusPaused, updated.Status)
	require.Len(t, updated.Items, 1)
	require.Equal(t, storage.ItemStatusInProgress, updated.Items[0].Status)
	require.Equal(t, 1, updated.Items[0].Attempts)
	require.True(t, updated.UpdatedAt.After(now))
}

func TestPauseBatch_InvalidState(t *testing.T) {
	st := newHandlerTestStorage()
	repoID := "repo-1"
	batchID := "batch-1"
	st.repos[repoID] = storage.Repository{ID: repoID, Name: "Repo", Path: "/tmp/repo"}
	st.batches[batchID] = storage.Batch{
		ID:           batchID,
		RepositoryID: repoID,
		Status:       storage.BatchStatusDone,
		Items:        []storage.BatchItem{{Input: "task", Status: storage.ItemStatusDone}},
	}
	app := newTestApp(t, st)

	req := httptest.NewRequest(http.MethodPut, "/repository/repo-1/batch/batch-1/pause", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	require.Equal(t, storage.BatchStatusDone, st.batches[batchID].Status)
}

func TestPauseBatch_Logs(t *testing.T) {
	st := newHandlerTestStorage()
	repoID := "repo-1"
	batchID := "batch-1"
	st.repos[repoID] = storage.Repository{ID: repoID, Name: "Repo", Path: "/tmp/repo"}
	st.batches[batchID] = storage.Batch{
		ID:           batchID,
		RepositoryID: repoID,
		Status:       storage.BatchStatusPending,
		Items:        []storage.BatchItem{{Input: "task", Status: storage.ItemStatusPending}},
	}
	core, obs := observer.New(zap.InfoLevel)
	app := newTestAppWithLogger(t, st, zap.New(core))

	req := httptest.NewRequest(http.MethodPut, "/repository/repo-1/batch/batch-1/pause", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	entries := obs.FilterMessage("batch paused").All()
	require.Len(t, entries, 1)
	assertLogField(t, entries[0], "batch_id", batchID)
	assertLogField(t, entries[0], "repository_id", repoID)
}

func TestResumeBatch(t *testing.T) {
	st := newHandlerTestStorage()
	repoID := "repo-1"
	batchID := "batch-1"
	now := time.Now().Add(-time.Minute).UTC()
	st.repos[repoID] = storage.Repository{ID: repoID, Name: "Repo", Path: "/tmp/repo"}
	st.batches[batchID] = storage.Batch{
		ID:           batchID,
		RepositoryID: repoID,
		Status:       storage.BatchStatusPaused,
		CreatedAt:    now,
		UpdatedAt:    now,
		Items:        []storage.BatchItem{{Input: "task", Status: storage.ItemStatusPending}},
	}
	app := newTestApp(t, st)

	req := httptest.NewRequest(http.MethodPut, "/repository/repo-1/batch/batch-1/resume", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, batchID, resp["batch_id"])
	require.Equal(t, string(storage.BatchStatusPending), resp["status"])
	require.Equal(t, repoID, resp["repository_id"])

	updated := st.batches[batchID]
	require.Equal(t, storage.BatchStatusPending, updated.Status)
	require.Equal(t, storage.ItemStatusPending, updated.Items[0].Status)
	require.True(t, updated.UpdatedAt.After(now))
}

func TestResumeBatch_InvalidState(t *testing.T) {
	st := newHandlerTestStorage()
	repoID := "repo-1"
	batchID := "batch-1"
	st.repos[repoID] = storage.Repository{ID: repoID, Name: "Repo", Path: "/tmp/repo"}
	st.batches[batchID] = storage.Batch{
		ID:           batchID,
		RepositoryID: repoID,
		Status:       storage.BatchStatusPending,
		Items:        []storage.BatchItem{{Input: "task", Status: storage.ItemStatusPending}},
	}
	app := newTestApp(t, st)

	req := httptest.NewRequest(http.MethodPut, "/repository/repo-1/batch/batch-1/resume", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
	require.Equal(t, storage.BatchStatusPending, st.batches[batchID].Status)
}

func TestResumeBatch_Logs(t *testing.T) {
	st := newHandlerTestStorage()
	repoID := "repo-1"
	batchID := "batch-1"
	st.repos[repoID] = storage.Repository{ID: repoID, Name: "Repo", Path: "/tmp/repo"}
	st.batches[batchID] = storage.Batch{
		ID:           batchID,
		RepositoryID: repoID,
		Status:       storage.BatchStatusPaused,
		Items:        []storage.BatchItem{{Input: "task", Status: storage.ItemStatusPending}},
	}
	core, obs := observer.New(zap.InfoLevel)
	app := newTestAppWithLogger(t, st, zap.New(core))

	req := httptest.NewRequest(http.MethodPut, "/repository/repo-1/batch/batch-1/resume", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	entries := obs.FilterMessage("batch resumed").All()
	require.Len(t, entries, 1)
	assertLogField(t, entries[0], "batch_id", batchID)
	assertLogField(t, entries[0], "repository_id", repoID)
}

func TestRestartFailedBatch(t *testing.T) {
	st := newHandlerTestStorage()
	repoID := "repo-1"
	batchID := "batch-1"
	now := time.Now().Add(-time.Minute).UTC()
	st.repos[repoID] = storage.Repository{ID: repoID, Name: "Repo", Path: "/tmp/repo"}
	st.batches[batchID] = storage.Batch{
		ID:           batchID,
		RepositoryID: repoID,
		Status:       storage.BatchStatusFailed,
		CreatedAt:    now,
		UpdatedAt:    now,
		Items: []storage.BatchItem{
			{Input: "first", Status: storage.ItemStatusDone, Attempts: 1},
			{Input: "second", Status: storage.ItemStatusFailed, Attempts: 2},
		},
	}
	app := newTestApp(t, st)

	req := httptest.NewRequest(http.MethodPut, "/repository/repo-1/batch/batch-1/restart", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, batchID, resp["batch_id"])
	require.Equal(t, repoID, resp["repository_id"])
	require.Equal(t, string(storage.BatchStatusPending), resp["status"])

	updated := st.batches[batchID]
	require.Equal(t, storage.BatchStatusPending, updated.Status)
	require.Equal(t, storage.ItemStatusPending, updated.Items[1].Status)
	require.Equal(t, 0, updated.Items[1].Attempts)
	require.True(t, updated.UpdatedAt.After(now))
}

func TestRestartFailedBatch_Logs(t *testing.T) {
	st := newHandlerTestStorage()
	repoID := "repo-1"
	batchID := "batch-1"
	st.repos[repoID] = storage.Repository{ID: repoID, Name: "Repo", Path: "/tmp/repo"}
	st.batches[batchID] = storage.Batch{
		ID:           batchID,
		RepositoryID: repoID,
		Status:       storage.BatchStatusFailed,
		Items:        []storage.BatchItem{{Input: "task", Status: storage.ItemStatusFailed}},
	}
	core, obs := observer.New(zap.InfoLevel)
	app := newTestAppWithLogger(t, st, zap.New(core))

	req := httptest.NewRequest(http.MethodPut, "/repository/repo-1/batch/batch-1/restart", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	entries := obs.FilterMessage("batch restart scheduled").All()
	require.Len(t, entries, 1)
	assertLogField(t, entries[0], "batch_id", batchID)
	assertLogField(t, entries[0], "repository_id", repoID)
	assertLogField(t, entries[0], "failed_index", 0)
}

func TestRestartFailedBatch_InvalidState(t *testing.T) {
	st := newHandlerTestStorage()
	repoID := "repo-1"
	batchID := "batch-1"
	st.repos[repoID] = storage.Repository{ID: repoID, Name: "Repo", Path: "/tmp/repo"}
	st.batches[batchID] = storage.Batch{
		ID:           batchID,
		RepositoryID: repoID,
		Status:       storage.BatchStatusPending,
		Items:        []storage.BatchItem{{Input: "task", Status: storage.ItemStatusPending}},
	}
	app := newTestApp(t, st)

	req := httptest.NewRequest(http.MethodPut, "/repository/repo-1/batch/batch-1/restart", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)

	require.Equal(t, http.StatusConflict, rec.Code)
}

func TestRestartFailedBatch_NoFailedItem(t *testing.T) {
	st := newHandlerTestStorage()
	repoID := "repo-1"
	batchID := "batch-1"
	st.repos[repoID] = storage.Repository{ID: repoID, Name: "Repo", Path: "/tmp/repo"}
	st.batches[batchID] = storage.Batch{
		ID:           batchID,
		RepositoryID: repoID,
		Status:       storage.BatchStatusFailed,
		Items:        []storage.BatchItem{{Input: "task", Status: storage.ItemStatusDone}},
	}
	app := newTestApp(t, st)

	req := httptest.NewRequest(http.MethodPut, "/repository/repo-1/batch/batch-1/restart", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRestartFailedBatch_RepositoryMismatch(t *testing.T) {
	st := newHandlerTestStorage()
	st.repos["repo-1"] = storage.Repository{ID: "repo-1", Name: "Repo", Path: "/tmp/repo"}
	st.batches["batch-1"] = storage.Batch{
		ID:           "batch-1",
		RepositoryID: "repo-2",
		Status:       storage.BatchStatusFailed,
		Items:        []storage.BatchItem{{Input: "task", Status: storage.ItemStatusFailed}},
	}
	app := newTestApp(t, st)

	req := httptest.NewRequest(http.MethodPut, "/repository/repo-1/batch/batch-1/restart", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestListBatchesByRepository(t *testing.T) {
	st := newHandlerTestStorage()
	repoID := "repo-1"
	st.repos[repoID] = storage.Repository{ID: repoID, Name: "Repo", Path: "/tmp/repo"}
	now := time.Now().UTC()
	st.batches["b1"] = storage.Batch{
		ID:           "b1",
		RepositoryID: repoID,
		Status:       storage.BatchStatusPending,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	st.batches["b2"] = storage.Batch{
		ID:           "b2",
		RepositoryID: repoID,
		Status:       storage.BatchStatusDone,
		CreatedAt:    now.Add(-time.Minute),
		UpdatedAt:    now.Add(-time.Minute),
	}
	st.batches["other"] = storage.Batch{
		ID:           "other",
		RepositoryID: "repo-2",
		Status:       storage.BatchStatusFailed,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	app := newTestApp(t, st)

	req := httptest.NewRequest(http.MethodGet, "/repository/repo-1/batches", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Batches []map[string]any `json:"batches"`
		Count   float64          `json:"count"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, float64(2), resp.Count)
	require.Len(t, resp.Batches, 2)

	ids := []string{resp.Batches[0]["batch_id"].(string), resp.Batches[1]["batch_id"].(string)}
	require.Subset(t, ids, []string{"b1", "b2"})
}

func TestListBatchesByRepository_WithStatusFilter(t *testing.T) {
	st := newHandlerTestStorage()
	repoID := "repo-1"
	st.repos[repoID] = storage.Repository{ID: repoID, Name: "Repo", Path: "/tmp/repo"}
	now := time.Now().UTC()
	st.batches["b1"] = storage.Batch{
		ID:           "b1",
		RepositoryID: repoID,
		Status:       storage.BatchStatusPending,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	st.batches["b2"] = storage.Batch{
		ID:           "b2",
		RepositoryID: repoID,
		Status:       storage.BatchStatusDone,
		CreatedAt:    now.Add(-time.Minute),
		UpdatedAt:    now.Add(-time.Minute),
	}
	app := newTestApp(t, st)

	req := httptest.NewRequest(http.MethodGet, "/repository/repo-1/batches?status=done", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Batches []map[string]any `json:"batches"`
		Count   float64          `json:"count"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, float64(1), resp.Count)
	require.Len(t, resp.Batches, 1)
	require.Equal(t, "b2", resp.Batches[0]["batch_id"])
	require.Equal(t, string(storage.BatchStatusDone), resp.Batches[0]["status"])
}

func TestListBatchesByRepository_InvalidStatus(t *testing.T) {
	st := newHandlerTestStorage()
	repoID := "repo-1"
	st.repos[repoID] = storage.Repository{ID: repoID, Name: "Repo", Path: "/tmp/repo"}
	app := newTestApp(t, st)

	req := httptest.NewRequest(http.MethodGet, "/repository/repo-1/batches?status=unknown", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestListBatchesByRepository_RepositoryNotFound(t *testing.T) {
	st := newHandlerTestStorage()
	app := newTestApp(t, st)

	req := httptest.NewRequest(http.MethodGet, "/repository/missing/batches", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandlers_RunDetailEventCap(t *testing.T) {
	st := newHandlerTestStorage()
	app := newTestApp(t, st)

	run := storage.TaskRun{
		ID:        "run-1",
		BatchID:   "b1",
		SessionID: "s1",
		Status:    storage.TaskRunStatusRunning,
		StartedAt: time.Now().UTC(),
	}
	st.runs[run.ID] = run
	st.events["s1"] = []storage.SessionEvent{
		{SessionID: "s1", Event: map[string]any{"n": 1}},
		{SessionID: "s1", Event: map[string]any{"n": 2}},
		{SessionID: "s1", Event: map[string]any{"n": 3}},
	}

	req := httptest.NewRequest(http.MethodGet, "/runs/run-1?event_limit=2", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Events          []any `json:"events"`
		EventsTruncated bool  `json:"events_truncated"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Events, 2)
	require.True(t, resp.EventsTruncated)
}

func TestHandlers_RunEvents(t *testing.T) {
	st := newHandlerTestStorage()
	app := newTestApp(t, st)

	run := storage.TaskRun{
		ID:        "run-1",
		BatchID:   "b1",
		SessionID: "s1",
		Status:    storage.TaskRunStatusRunning,
		StartedAt: time.Now().UTC(),
	}
	st.runs[run.ID] = run
	st.events["s1"] = []storage.SessionEvent{
		{SessionID: "s1", Event: map[string]any{"n": 1}},
		{SessionID: "s1", Event: map[string]any{"n": 2}},
		{SessionID: "s1", Event: map[string]any{"n": 3}},
	}

	req := httptest.NewRequest(http.MethodGet, "/runs/run-1/events?limit=2", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Events          []any `json:"events"`
		Count           int   `json:"count"`
		EventsTruncated bool  `json:"events_truncated"`
		EventLimitUsed  int   `json:"event_limit_used"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Len(t, resp.Events, 2)
	require.Equal(t, 2, resp.Count)
	require.True(t, resp.EventsTruncated)
	require.Equal(t, 2, resp.EventLimitUsed)
}

func TestHandlers_RunEvents_NotFound(t *testing.T) {
	st := newHandlerTestStorage()
	app := newTestApp(t, st)

	req := httptest.NewRequest(http.MethodGet, "/runs/does-not-exist/events", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandlers_ListRunsIncludesTaskRefAndEventCounts(t *testing.T) {
	st := newHandlerTestStorage()
	app := newTestApp(t, st)
	st.repos["repo-1"] = storage.Repository{ID: "repo-1", Name: "Repo"}

	now := time.Now().UTC()
	run1 := storage.TaskRun{
		ID:           "run-newer",
		BatchID:      "batch-1",
		RepositoryID: "repo-1",
		TaskRef:      "task-1",
		Status:       storage.TaskRunStatusSucceeded,
		SessionID:    "s1",
		StartedAt:    now,
	}
	run2 := storage.TaskRun{
		ID:           "run-older",
		BatchID:      "batch-1",
		RepositoryID: "repo-1",
		TaskRef:      "task-2",
		Status:       storage.TaskRunStatusFailed,
		SessionID:    "s2",
		StartedAt:    now.Add(-time.Minute),
	}
	st.batches["batch-1"] = storage.Batch{ID: "batch-1", RepositoryID: "repo-1", CreatedAt: now, UpdatedAt: now}
	st.runs[run1.ID] = run1
	st.runs[run2.ID] = run2
	st.events["s1"] = []storage.SessionEvent{
		{RunID: "run-newer", SessionID: "s1", Event: map[string]any{"n": 1}},
		{RunID: "run-newer", SessionID: "s1", Event: map[string]any{"n": 2}},
	}
	st.events["s2"] = []storage.SessionEvent{
		{RunID: "run-older", SessionID: "s2", Event: map[string]any{"n": 1}},
	}

	req := httptest.NewRequest(http.MethodGet, "/repository/repo-1/batch/batch-1/runs?limit=2", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp struct {
		Runs  []map[string]any `json:"runs"`
		Count float64          `json:"count"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, float64(2), resp.Count)
	require.Len(t, resp.Runs, 2)

	first := resp.Runs[0]
	second := resp.Runs[1]

	require.Equal(t, "run-newer", first["run_id"])
	require.Equal(t, "task-1", first["task_ref"])
	require.Equal(t, float64(2), first["total_events"])

	require.Equal(t, "run-older", second["run_id"])
	require.Equal(t, "task-2", second["task_ref"])
	require.Equal(t, float64(1), second["total_events"])
}

func TestHandlers_BatchProgress(t *testing.T) {
	st := newHandlerTestStorage()
	app := newTestApp(t, st)

	st.progress["b99"] = storage.BatchProgress{Total: 3, Succeeded: 2, Failed: 1}

	req := httptest.NewRequest(http.MethodGet, "/batches/b99/progress", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp storage.BatchProgress
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 3, resp.Total)
	require.Equal(t, 2, resp.Succeeded)
}

// requestID header is set even when provided from client.
func TestHandlers_RequestIDMiddleware(t *testing.T) {
	st := newHandlerTestStorage()
	app := newTestApp(t, st)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Request-ID", "abc123")
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)

	require.Equal(t, "abc123", rec.Header().Get("X-Request-ID"))
}
