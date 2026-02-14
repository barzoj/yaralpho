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
	"github.com/barzoj/yaralpho/internal/queue"
	"github.com/barzoj/yaralpho/internal/storage"
	"github.com/barzoj/yaralpho/internal/tracker"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
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
	s.repos[repo.ID] = *repo
	return nil
}
func (s *handlerTestStorage) UpdateRepository(ctx context.Context, repo *storage.Repository) error {
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
	s.agents[agent.ID] = *agent
	return nil
}
func (s *handlerTestStorage) UpdateAgent(ctx context.Context, agent *storage.Agent) error {
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

type handlerTestQueue struct {
	items  []string
	closed bool
}

func (q *handlerTestQueue) Enqueue(item string) error {
	if q.closed {
		return queue.ErrClosed
	}
	q.items = append(q.items, item)
	return nil
}
func (q *handlerTestQueue) Dequeue(ctx context.Context) (string, error) { return "", queue.ErrClosed }
func (q *handlerTestQueue) Close()                                      { q.closed = true }

type noopTracker struct{}

func (noopTracker) IsEpic(ctx context.Context, ref string) (bool, error) { return false, nil }
func (noopTracker) ListChildren(ctx context.Context, ref string) ([]string, error) {
	return nil, nil
}
func (noopTracker) AddComment(ctx context.Context, ref string, text string) error { return nil }
func (noopTracker) FetchComments(ctx context.Context, ref string) ([]tracker.Comment, error) {
	return nil, nil
}

func (noopTracker) GetTitle(ctx context.Context, ref string) (string, error) {
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

type noopConsumer struct{}

func (noopConsumer) Run(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }

// --- helpers -----------------------------------------------------------------

func newTestApp(t *testing.T, st *handlerTestStorage, q *handlerTestQueue) *App {
	t.Helper()
	cfg := fakeConfig{
		config.PortKey:     "0",
		config.MongoURIKey: "mongodb://example",
		config.MongoDBKey:  "db",
		config.RepoPathKey: "/repo",
		config.BdRepoKey:   "/bd",
	}

	app, err := New(zap.NewNop(), cfg, st, q, noopTracker{}, noopNotifier{}, noopCopilot{}, noopConsumer{})
	require.NoError(t, err)
	return app
}

// --- tests -------------------------------------------------------------------

func TestHandlers_AddAndList(t *testing.T) {
	st := newHandlerTestStorage()
	q := &handlerTestQueue{}
	app := newTestApp(t, st, q)

	req := httptest.NewRequest(http.MethodPost, "/add?items=T1,T2&session_name=test", bytes.NewBuffer(nil))
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)

	require.Equal(t, http.StatusAccepted, rec.Code)

	var resp map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	batchID := resp["batch_id"]
	require.NotEmpty(t, batchID)
	require.Len(t, q.items, 2)

	// list batches
	req = httptest.NewRequest(http.MethodGet, "/batches", nil)
	rec = httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var listResp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listResp))
	require.Equal(t, float64(1), listResp["count"])
}

func TestHandlers_RunDetailEventCap(t *testing.T) {
	st := newHandlerTestStorage()
	q := &handlerTestQueue{}
	app := newTestApp(t, st, q)

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
	q := &handlerTestQueue{}
	app := newTestApp(t, st, q)

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
	q := &handlerTestQueue{}
	app := newTestApp(t, st, q)

	req := httptest.NewRequest(http.MethodGet, "/runs/does-not-exist/events", nil)
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHandlers_ListRunsIncludesTaskRefAndEventCounts(t *testing.T) {
	st := newHandlerTestStorage()
	q := &handlerTestQueue{}
	app := newTestApp(t, st, q)

	now := time.Now().UTC()
	run1 := storage.TaskRun{
		ID:        "run-newer",
		BatchID:   "batch-1",
		TaskRef:   "task-1",
		Status:    storage.TaskRunStatusSucceeded,
		SessionID: "s1",
		StartedAt: now,
	}
	run2 := storage.TaskRun{
		ID:        "run-older",
		BatchID:   "batch-1",
		TaskRef:   "task-2",
		Status:    storage.TaskRunStatusFailed,
		SessionID: "s2",
		StartedAt: now.Add(-time.Minute),
	}
	st.runs[run1.ID] = run1
	st.runs[run2.ID] = run2
	st.events["s1"] = []storage.SessionEvent{
		{RunID: "run-newer", SessionID: "s1", Event: map[string]any{"n": 1}},
		{RunID: "run-newer", SessionID: "s1", Event: map[string]any{"n": 2}},
	}
	st.events["s2"] = []storage.SessionEvent{
		{RunID: "run-older", SessionID: "s2", Event: map[string]any{"n": 1}},
	}

	req := httptest.NewRequest(http.MethodGet, "/runs?batch_id=batch-1&limit=2", nil)
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
	q := &handlerTestQueue{}
	app := newTestApp(t, st, q)

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
	q := &handlerTestQueue{}
	app := newTestApp(t, st, q)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("X-Request-ID", "abc123")
	rec := httptest.NewRecorder()
	app.Router().ServeHTTP(rec, req)

	require.Equal(t, "abc123", rec.Header().Get("X-Request-ID"))
}
