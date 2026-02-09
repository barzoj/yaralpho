package consumer

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/barzoj/yaralpho/internal/copilot"
	"github.com/barzoj/yaralpho/internal/storage"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestWorker_TaskPromptAndEvents(t *testing.T) {
	ctx := context.Background()
	batch := storage.Batch{ID: "b1", Status: storage.BatchStatusCreated}

	st := newFakeStorage()
	st.batches["b1"] = batch

	cp := &fakeCopilot{
		events:    make(chan copilot.RawEvent, 2),
		sessionID: "s123",
	}
	cp.events <- copilot.RawEvent{"type": "event1"}
	cp.events <- copilot.RawEvent{"type": "event2"}
	close(cp.events)

	tr := &fakeTracker{}

	nt := &fakeNotifier{}

	w := NewWorker(nil, tr, cp, st, nt, "/repo", zap.NewNop())
	w.newRunID = func() string { return "run-1" }
	now := time.Date(2026, 2, 8, 12, 0, 0, 0, time.UTC)
	w.now = func() time.Time { return now }

	payload, _ := EncodeQueueItem(QueueItem{BatchID: "b1", TaskRef: "task-1"})
	require.NoError(t, w.handleItem(ctx, payload))

	require.Equal(t, "Work on task task-1", cp.prompt)
	require.Equal(t, "/repo", cp.repoPath)

	run, ok := st.runs["run-1"]
	require.True(t, ok)
	require.Equal(t, storage.TaskRunStatusSucceeded, run.Status)
	require.Equal(t, "task-1", run.TaskRef)
	require.Equal(t, "s123", run.SessionID)

	require.Len(t, st.sessionEvents, 2)
	require.Equal(t, storage.BatchStatusIdle, st.batches["b1"].Status)

	require.Len(t, nt.finished, 1)
	require.Equal(t, notifyFinished{batchID: "b1", runID: "run-1", taskRef: "task-1", status: "succeeded"}, nt.finished[0])
}

func TestWorker_StopsOnSessionIdleEvent(t *testing.T) {
	ctx := context.Background()
	st := newFakeStorage()
	st.batches["b1"] = storage.Batch{ID: "b1", Status: storage.BatchStatusCreated}

	events := make(chan copilot.RawEvent, 1)
	events <- copilot.RawEvent{"type": "session.idle", "id": "ev-1"}

	cp := &fakeCopilot{
		events:    events,
		sessionID: "s-idle",
	}

	tr := &fakeTracker{}
	nt := &fakeNotifier{}

	w := NewWorker(nil, tr, cp, st, nt, "/repo", zap.NewNop())
	w.newRunID = func() string { return "run-idle" }
	now := time.Date(2026, 2, 8, 15, 0, 0, 0, time.UTC)
	w.now = func() time.Time { return now }

	payload, _ := EncodeQueueItem(QueueItem{BatchID: "b1", TaskRef: "task-idle"})
	require.NoError(t, w.handleItem(ctx, payload))

	run := st.runs["run-idle"]
	require.Equal(t, storage.TaskRunStatusSucceeded, run.Status)
	require.Equal(t, "s-idle", run.SessionID)
	require.Len(t, st.sessionEvents, 1)
	require.Equal(t, storage.BatchStatusIdle, st.batches["b1"].Status)
	require.True(t, cp.stopped)
}

func TestWorker_EpicChoosesFirstAvailableChild(t *testing.T) {
	ctx := context.Background()
	batch := storage.Batch{ID: "b1", Status: storage.BatchStatusCreated}

	st := newFakeStorage()
	st.batches["b1"] = batch
	st.runs["old"] = storage.TaskRun{ID: "old", BatchID: "b1", TaskRef: "child-1"}

	cp := &fakeCopilot{events: closedChan(), sessionID: "s999"}

	tr := &fakeTracker{
		epics: map[string]bool{"epic-1": true},
		children: map[string][]string{
			"epic-1": {"child-1", "child-2", "child-3"},
		},
	}

	nt := &fakeNotifier{}

	w := NewWorker(nil, tr, cp, st, nt, "/repo", zap.NewNop())
	nextID := 2
	w.newRunID = func() string {
		id := fmt.Sprintf("run-%d", nextID)
		nextID++
		return id
	}
	w.now = func() time.Time { return time.Date(2026, 2, 8, 13, 0, 0, 0, time.UTC) }

	payload, _ := EncodeQueueItem(QueueItem{BatchID: "b1", TaskRef: "epic-1"})
	require.NoError(t, w.handleItem(ctx, payload))

	require.Equal(t, "Pick first ready task from epic epic-1 and execute", cp.prompt)

	run1 := st.runs["run-2"]
	require.Equal(t, "child-2", run1.TaskRef)
	require.Equal(t, "epic-1", run1.EpicRef)
	require.Equal(t, storage.TaskRunStatusSucceeded, run1.Status)

	run2 := st.runs["run-3"]
	require.Equal(t, "child-3", run2.TaskRef)
	require.Equal(t, "epic-1", run2.EpicRef)
	require.Equal(t, storage.TaskRunStatusSucceeded, run2.Status)
	require.Equal(t, storage.BatchStatusIdle, st.batches["b1"].Status)
}

func TestWorker_NoRemainingChildrenMarksIdle(t *testing.T) {
	ctx := context.Background()
	st := newFakeStorage()
	st.batches["b1"] = storage.Batch{ID: "b1", Status: storage.BatchStatusCreated}
	st.runs["r1"] = storage.TaskRun{ID: "r1", BatchID: "b1", TaskRef: "child-1"}

	tr := &fakeTracker{
		epics:    map[string]bool{"epic-1": true},
		children: map[string][]string{"epic-1": {"child-1"}},
	}
	nt := &fakeNotifier{}
	cp := &fakeCopilot{events: closedChan(), sessionID: "s1"}

	w := NewWorker(nil, tr, cp, st, nt, "/repo", zap.NewNop())
	payload, _ := EncodeQueueItem(QueueItem{BatchID: "b1", TaskRef: "epic-1"})
	err := w.handleItem(ctx, payload)
	require.NoError(t, err)

	require.Equal(t, storage.BatchStatusIdle, st.batches["b1"].Status)
	require.Len(t, nt.batchIdle, 1)
	require.Len(t, st.runs, 1) // no new run created
}

func TestWorker_StartSessionErrorMarksFailed(t *testing.T) {
	ctx := context.Background()
	st := newFakeStorage()
	st.batches["b1"] = storage.Batch{ID: "b1", Status: storage.BatchStatusCreated}

	cp := &fakeCopilot{startErr: errors.New("no token")}
	tr := &fakeTracker{}
	nt := &fakeNotifier{}

	w := NewWorker(nil, tr, cp, st, nt, "/repo", zap.NewNop())
	w.newRunID = func() string { return "run-fail" }
	w.now = func() time.Time { return time.Date(2026, 2, 8, 14, 0, 0, 0, time.UTC) }

	payload, _ := EncodeQueueItem(QueueItem{BatchID: "b1", TaskRef: "task-err"})
	err := w.handleItem(ctx, payload)
	require.Error(t, err)

	run := st.runs["run-fail"]
	require.Equal(t, storage.TaskRunStatusFailed, run.Status)
	require.Equal(t, storage.BatchStatusFailed, st.batches["b1"].Status)
	require.Len(t, nt.errors, 1)
}

func closedChan() chan copilot.RawEvent {
	ch := make(chan copilot.RawEvent)
	close(ch)
	return ch
}

// fakes below ---------------------------------------------------------------

type fakeCopilot struct {
	prompt    string
	repoPath  string
	events    chan copilot.RawEvent
	sessionID string
	startErr  error
	stopped   bool
}

func (f *fakeCopilot) StartSession(ctx context.Context, prompt, repoPath string) (string, <-chan copilot.RawEvent, func(), error) {
	if f.startErr != nil {
		return "", nil, nil, f.startErr
	}
	f.prompt = prompt
	f.repoPath = repoPath
	stop := func() { f.stopped = true }
	return f.sessionID, f.events, stop, nil
}

type fakeTracker struct {
	epics     map[string]bool
	children  map[string][]string
	isEpicErr error
	listErr   error
}

func (f *fakeTracker) IsEpic(ctx context.Context, ref string) (bool, error) {
	if f.isEpicErr != nil {
		return false, f.isEpicErr
	}
	if f.epics == nil {
		return false, nil
	}
	return f.epics[ref], nil
}

func (f *fakeTracker) ListChildren(ctx context.Context, ref string) ([]string, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	if f.children == nil {
		return []string{}, nil
	}
	return f.children[ref], nil
}

type fakeStorage struct {
	batches       map[string]storage.Batch
	runs          map[string]storage.TaskRun
	sessionEvents []storage.SessionEvent
}

func newFakeStorage() *fakeStorage {
	return &fakeStorage{
		batches: make(map[string]storage.Batch),
		runs:    make(map[string]storage.TaskRun),
	}
}

func (f *fakeStorage) CreateBatch(ctx context.Context, batch *storage.Batch) error {
	f.batches[batch.ID] = *batch
	return nil
}

func (f *fakeStorage) UpdateBatch(ctx context.Context, batch *storage.Batch) error {
	f.batches[batch.ID] = *batch
	return nil
}

func (f *fakeStorage) GetBatch(ctx context.Context, batchID string) (*storage.Batch, error) {
	b, ok := f.batches[batchID]
	if !ok {
		return nil, errors.New("not found")
	}
	return &b, nil
}

func (f *fakeStorage) ListBatches(ctx context.Context, limit int64) ([]storage.Batch, error) {
	out := make([]storage.Batch, 0, len(f.batches))
	for _, b := range f.batches {
		out = append(out, b)
	}
	return out, nil
}

func (f *fakeStorage) CreateTaskRun(ctx context.Context, run *storage.TaskRun) error {
	f.runs[run.ID] = *run
	return nil
}

func (f *fakeStorage) UpdateTaskRun(ctx context.Context, run *storage.TaskRun) error {
	f.runs[run.ID] = *run
	return nil
}

func (f *fakeStorage) GetTaskRun(ctx context.Context, runID string) (*storage.TaskRun, error) {
	r, ok := f.runs[runID]
	if !ok {
		return nil, errors.New("not found")
	}
	return &r, nil
}

func (f *fakeStorage) ListTaskRuns(ctx context.Context, batchID string) ([]storage.TaskRun, error) {
	out := []storage.TaskRun{}
	for _, r := range f.runs {
		if r.BatchID == batchID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeStorage) InsertSessionEvent(ctx context.Context, event *storage.SessionEvent) error {
	f.sessionEvents = append(f.sessionEvents, *event)
	return nil
}

func (f *fakeStorage) ListSessionEvents(ctx context.Context, sessionID string) ([]storage.SessionEvent, error) {
	out := []storage.SessionEvent{}
	for _, e := range f.sessionEvents {
		if e.SessionID == sessionID {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeStorage) GetBatchProgress(ctx context.Context, batchID string) (storage.BatchProgress, error) {
	return storage.BatchProgress{}, nil
}

type fakeNotifier struct {
	finished  []notifyFinished
	batchIdle []string
	errors    []notifyError
}

type notifyFinished struct {
	batchID    string
	runID      string
	taskRef    string
	status     string
	commitHash string
}

type notifyError struct {
	batchID string
	runID   string
	taskRef string
	err     error
}

func (f *fakeNotifier) NotifyTaskFinished(ctx context.Context, batchID, runID, taskRef, status, commitHash string) error {
	f.finished = append(f.finished, notifyFinished{batchID, runID, taskRef, status, commitHash})
	return nil
}

func (f *fakeNotifier) NotifyBatchIdle(ctx context.Context, batchID string) error {
	f.batchIdle = append(f.batchIdle, batchID)
	return nil
}

func (f *fakeNotifier) NotifyError(ctx context.Context, batchID, runID, taskRef string, err error) error {
	f.errors = append(f.errors, notifyError{batchID, runID, taskRef, err})
	return nil
}
