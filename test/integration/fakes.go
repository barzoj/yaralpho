package integration

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/barzoj/yaralpho/internal/consumer"
	"github.com/barzoj/yaralpho/internal/copilot"
	"github.com/barzoj/yaralpho/internal/notify"
	"github.com/barzoj/yaralpho/internal/storage"
	"github.com/barzoj/yaralpho/internal/tracker"
	"go.mongodb.org/mongo-driver/mongo"
)

// fakeStorage is an in-memory implementation of storage.Storage for fast, deterministic tests.
// It uses a mutex for coarse safety; not optimized for production use.
type fakeStorage struct {
	mu            sync.Mutex
	repositories  map[string]storage.Repository
	agents        map[string]storage.Agent
	batches       map[string]storage.Batch
	runs          map[string]storage.TaskRun
	sessionEvents []storage.SessionEvent
}

func newFakeStorage() *fakeStorage {
	return &fakeStorage{
		repositories: make(map[string]storage.Repository),
		agents:       make(map[string]storage.Agent),
		batches:      make(map[string]storage.Batch),
		runs:         make(map[string]storage.TaskRun),
	}
}

func (f *fakeStorage) CreateRepository(ctx context.Context, repo *storage.Repository) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.repositories[repo.ID]; ok {
		return storage.ErrConflict
	}
	f.repositories[repo.ID] = *repo
	return nil
}

func (f *fakeStorage) UpdateRepository(ctx context.Context, repo *storage.Repository) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.repositories[repo.ID]; !ok {
		return mongo.ErrNoDocuments
	}
	f.repositories[repo.ID] = *repo
	return nil
}

func (f *fakeStorage) GetRepository(ctx context.Context, id string) (*storage.Repository, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	repo, ok := f.repositories[id]
	if !ok {
		return nil, mongo.ErrNoDocuments
	}
	cp := repo
	return &cp, nil
}

func (f *fakeStorage) ListRepositories(ctx context.Context) ([]storage.Repository, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]storage.Repository, 0, len(f.repositories))
	for _, r := range f.repositories {
		out = append(out, r)
	}
	return out, nil
}

func (f *fakeStorage) DeleteRepository(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.repositories, id)
	return nil
}

func (f *fakeStorage) RepositoryHasActiveBatches(ctx context.Context, id string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, b := range f.batches {
		if b.RepositoryID == id && b.Status != storage.BatchStatusDone && b.Status != storage.BatchStatusFailed {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeStorage) CreateAgent(ctx context.Context, agent *storage.Agent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.agents[agent.ID]; ok {
		return storage.ErrConflict
	}
	f.agents[agent.ID] = *agent
	return nil
}

func (f *fakeStorage) UpdateAgent(ctx context.Context, agent *storage.Agent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.agents[agent.ID]; !ok {
		return mongo.ErrNoDocuments
	}
	f.agents[agent.ID] = *agent
	return nil
}

func (f *fakeStorage) GetAgent(ctx context.Context, id string) (*storage.Agent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	agent, ok := f.agents[id]
	if !ok {
		return nil, mongo.ErrNoDocuments
	}
	cp := agent
	return &cp, nil
}

func (f *fakeStorage) ListAgents(ctx context.Context) ([]storage.Agent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]storage.Agent, 0, len(f.agents))
	for _, a := range f.agents {
		out = append(out, a)
	}
	return out, nil
}

func (f *fakeStorage) DeleteAgent(ctx context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.agents, id)
	return nil
}

func (f *fakeStorage) CreateBatch(ctx context.Context, batch *storage.Batch) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.batches[batch.ID]; ok {
		return storage.ErrConflict
	}
	f.batches[batch.ID] = *batch
	return nil
}

func (f *fakeStorage) UpdateBatch(ctx context.Context, batch *storage.Batch) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.batches[batch.ID]; !ok {
		return mongo.ErrNoDocuments
	}
	f.batches[batch.ID] = *batch
	return nil
}

func (f *fakeStorage) GetBatch(ctx context.Context, batchID string) (*storage.Batch, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.batches[batchID]
	if !ok {
		return nil, mongo.ErrNoDocuments
	}
	cp := b
	return &cp, nil
}

func (f *fakeStorage) ListBatches(ctx context.Context, limit int64) ([]storage.Batch, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]storage.Batch, 0, len(f.batches))
	for _, b := range f.batches {
		out = append(out, b)
	}
	return out, nil
}

func (f *fakeStorage) ListBatchesByRepository(ctx context.Context, repositoryID string, status storage.BatchStatus, limit int64) ([]storage.Batch, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]storage.Batch, 0)
	for _, b := range f.batches {
		if b.RepositoryID != repositoryID {
			continue
		}
		if status != "" && b.Status != status {
			continue
		}
		out = append(out, b)
	}
	return out, nil
}

func (f *fakeStorage) CreateTaskRun(ctx context.Context, run *storage.TaskRun) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runs[run.ID] = *run
	return nil
}

func (f *fakeStorage) UpdateTaskRun(ctx context.Context, run *storage.TaskRun) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.runs[run.ID]; !ok {
		return mongo.ErrNoDocuments
	}
	f.runs[run.ID] = *run
	return nil
}

func (f *fakeStorage) GetTaskRun(ctx context.Context, runID string) (*storage.TaskRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.runs[runID]
	if !ok {
		return nil, mongo.ErrNoDocuments
	}
	cp := r
	return &cp, nil
}

func (f *fakeStorage) ListTaskRunsByRepository(ctx context.Context, repositoryID string) ([]storage.TaskRunSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []storage.TaskRunSummary
	for _, r := range f.runs {
		if r.RepositoryID == repositoryID {
			out = append(out, storage.TaskRunSummary{TaskRun: r, TotalEvents: 0})
		}
	}
	return out, nil
}

func (f *fakeStorage) ListTaskRuns(ctx context.Context, batchID string) ([]storage.TaskRunSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []storage.TaskRunSummary
	for _, r := range f.runs {
		if batchID == "" || r.BatchID == batchID {
			out = append(out, storage.TaskRunSummary{TaskRun: r, TotalEvents: 0})
		}
	}
	return out, nil
}

func (f *fakeStorage) InsertSessionEvent(ctx context.Context, event *storage.SessionEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessionEvents = append(f.sessionEvents, *event)
	return nil
}

func (f *fakeStorage) ListSessionEvents(ctx context.Context, sessionID string) ([]storage.SessionEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []storage.SessionEvent
	for _, e := range f.sessionEvents {
		if e.SessionID == sessionID {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeStorage) GetBatchProgress(ctx context.Context, batchID string) (storage.BatchProgress, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.batches[batchID]
	if !ok {
		return storage.BatchProgress{}, mongo.ErrNoDocuments
	}
	var done, failed, pending int
	for _, it := range b.Items {
		switch it.Status {
		case storage.ItemStatusDone:
			done++
		case storage.ItemStatusFailed:
			failed++
		default:
			pending++
		}
	}
	return storage.BatchProgress{
		Total:     len(b.Items),
		Pending:   pending,
		Succeeded: done,
		Failed:    failed,
	}, nil
}

// fakeTracker records comments only.
type fakeTracker struct {
	mu       sync.Mutex
	comments map[string][]tracker.Comment
}

func newFakeTracker() *fakeTracker {
	return &fakeTracker{comments: make(map[string][]tracker.Comment)}
}

func (f *fakeTracker) AddComment(ctx context.Context, ref string, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.comments[ref] = append(f.comments[ref], tracker.Comment{
		ID:        fmt.Sprintf("c-%d", len(f.comments[ref])+1),
		Text:      text,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	})
	return nil
}

func (f *fakeTracker) FetchComments(ctx context.Context, ref string) ([]tracker.Comment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]tracker.Comment(nil), f.comments[ref]...), nil
}

func (f *fakeTracker) GetTitle(ctx context.Context, ref string) (string, error) {
	return "", nil
}

// fakeNotifier records events for assertions.
type fakeNotifier struct {
	mu     sync.Mutex
	events []notify.Event
}

func (f *fakeNotifier) NotifyEvent(ctx context.Context, event notify.Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, event)
	return nil
}

func (f *fakeNotifier) NotifyTaskFinished(ctx context.Context, batchID, runID, taskRef, taskName, status, commitHash string) error {
	return nil
}

func (f *fakeNotifier) NotifyBatchIdle(ctx context.Context, batchID string) error { return nil }
func (f *fakeNotifier) NotifyError(ctx context.Context, batchID, runID, taskRef string, err error) error {
	return nil
}

// fakeCopilot returns canned session IDs and closes immediately.
type fakeCopilot struct{}

func (fakeCopilot) StartSession(ctx context.Context, prompt, repoPath string) (string, <-chan copilot.RawEvent, func(), error) {
	ch := make(chan copilot.RawEvent)
	close(ch)
	return "session-1", ch, func() {}, nil
}

// fakeWorker implements scheduler.Worker with controllable outcomes.
type fakeWorker struct {
	mu        sync.Mutex
	fail      bool
	callCount int
	store     *fakeStorage
}

func newFakeWorker(store *fakeStorage, fail bool) *fakeWorker {
	return &fakeWorker{store: store, fail: fail}
}

func (f *fakeWorker) Process(ctx context.Context, item consumer.WorkItem) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callCount++
	run := storage.TaskRun{
		ID:           fmt.Sprintf("run-%d", time.Now().UTC().UnixNano()),
		BatchID:      item.BatchID,
		RepositoryID: "",
		TaskRef:      item.TaskRef,
		StartedAt:    time.Now().UTC(),
	}
	status := storage.TaskRunStatusSucceeded
	if f.fail {
		status = storage.TaskRunStatusFailed
	}
	run.Status = status
	now := time.Now().UTC()
	run.FinishedAt = &now
	if f.store != nil {
		if b, err := f.store.GetBatch(ctx, item.BatchID); err == nil {
			run.RepositoryID = b.RepositoryID
		}
		_ = f.store.CreateTaskRun(ctx, &run)
	}
	if f.fail {
		return errors.New("boom")
	}
	return nil
}

func (f *fakeWorker) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.callCount
}
