package consumer

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/barzoj/yaralpho/internal/config"
	"github.com/barzoj/yaralpho/internal/copilot"
	"github.com/barzoj/yaralpho/internal/storage"
	"github.com/barzoj/yaralpho/internal/tracker"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestWorker_TaskPromptAndEvents(t *testing.T) {
	ctx := context.Background()
	batch := storage.Batch{ID: "b1", Status: storage.BatchStatusCreated}

	st := newFakeStorage()
	st.batches["b1"] = batch

	cp := &fakeCopilot{
		events:     make(chan copilot.RawEvent, 2),
		sessionIDs: []string{"s123", "s123-verify"},
	}
	cp.events <- copilot.RawEvent{"type": "event1"}
	cp.events <- copilot.RawEvent{"type": "event2"}
	close(cp.events)

	tr := &fakeTracker{}

	nt := &fakeNotifier{}
	cfg := stubConfig{
		config.ExecutionTaskPromptKey:    "exec base",
		config.VerificationTaskPromptKey: "verify base",
	}

	w := NewWorker(nil, tr, cp, st, nt, cfg, "/repo", zap.NewNop())
	nextRun := 0
	w.newRunID = func() string {
		nextRun++
		return fmt.Sprintf("run-%d", nextRun)
	}
	now := time.Date(2026, 2, 8, 12, 0, 0, 0, time.UTC)
	w.now = func() time.Time { return now }

	payload, _ := EncodeQueueItem(QueueItem{BatchID: "b1", TaskRef: "task-1"})
	require.NoError(t, w.handleItem(ctx, payload))

	require.Equal(t, []string{
		"exec base\n\nWork on task task-1",
		"verify base\n\nVerify task task-1",
	}, cp.prompts)
	require.Equal(t, []string{"/repo", "/repo"}, cp.repoPaths)

	run, ok := st.runs["run-1"]
	require.True(t, ok)
	require.Equal(t, storage.TaskRunStatusSucceeded, run.Status)
	require.Equal(t, "task-1", run.TaskRef)
	require.Equal(t, "s123", run.SessionID)

	verifyRun := st.runs["run-2"]
	require.Equal(t, storage.TaskRunStatusSucceeded, verifyRun.Status)
	require.Equal(t, "task-1", verifyRun.TaskRef)
	require.Equal(t, "s123-verify", verifyRun.SessionID)

	require.Len(t, st.sessionEvents, 2)
	require.Equal(t, storage.BatchStatusIdle, st.batches["b1"].Status)

	require.Len(t, nt.finished, 2)
	require.Equal(t, notifyFinished{batchID: "b1", runID: "run-1", taskRef: "task-1", status: "succeeded"}, nt.finished[0])
	require.Equal(t, notifyFinished{batchID: "b1", runID: "run-2", taskRef: "task-1", status: "succeeded"}, nt.finished[1])
}

func TestWorker_StopsOnSessionIdleEvent(t *testing.T) {
	ctx := context.Background()
	st := newFakeStorage()
	st.batches["b1"] = storage.Batch{ID: "b1", Status: storage.BatchStatusCreated}

	firstRunEvents := make(chan copilot.RawEvent, 1)
	firstRunEvents <- copilot.RawEvent{"type": "session.idle", "id": "ev-1"}
	close(firstRunEvents)

	cp := &fakeCopilot{
		eventQueue: []chan copilot.RawEvent{firstRunEvents, closedChan()},
		sessionIDs: []string{"s-idle", "s-verify"},
	}

	tr := &fakeTracker{}
	nt := &fakeNotifier{}

	cfg := stubConfig{
		config.ExecutionTaskPromptKey:    "exec",
		config.VerificationTaskPromptKey: "verify",
	}

	w := NewWorker(nil, tr, cp, st, nt, cfg, "/repo", zap.NewNop())
	nextRun := 0
	w.newRunID = func() string {
		nextRun++
		return fmt.Sprintf("run-idle-%d", nextRun)
	}
	now := time.Date(2026, 2, 8, 15, 0, 0, 0, time.UTC)
	w.now = func() time.Time { return now }

	payload, _ := EncodeQueueItem(QueueItem{BatchID: "b1", TaskRef: "task-idle"})
	require.NoError(t, w.handleItem(ctx, payload))

	run := st.runs["run-idle-1"]
	require.Equal(t, storage.TaskRunStatusSucceeded, run.Status)
	require.Equal(t, "s-idle", run.SessionID)

	verifyRun := st.runs["run-idle-2"]
	require.Equal(t, storage.TaskRunStatusSucceeded, verifyRun.Status)
	require.Equal(t, "s-verify", verifyRun.SessionID)

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
	cfg := stubConfig{
		config.ExecutionTaskPromptKey:    "exec base",
		config.VerificationTaskPromptKey: "verify base",
	}

	w := NewWorker(nil, tr, cp, st, nt, cfg, "/repo", zap.NewNop())
	nextID := 2
	w.newRunID = func() string {
		id := fmt.Sprintf("run-%d", nextID)
		nextID++
		return id
	}
	w.now = func() time.Time { return time.Date(2026, 2, 8, 13, 0, 0, 0, time.UTC) }

	payload, _ := EncodeQueueItem(QueueItem{BatchID: "b1", TaskRef: "epic-1"})
	require.NoError(t, w.handleItem(ctx, payload))

	require.Equal(t, []string{
		"exec base\n\nPick first ready task from epic epic-1 and execute",
		"verify base\n\nVerify task child-2",
		"exec base\n\nPick first ready task from epic epic-1 and execute",
		"verify base\n\nVerify task child-3",
	}, cp.prompts)

	run1 := st.runs["run-2"]
	require.Equal(t, "child-2", run1.TaskRef)
	require.Equal(t, "epic-1", run1.EpicRef)
	require.Equal(t, storage.TaskRunStatusSucceeded, run1.Status)

	verifyRun1 := st.runs["run-3"]
	require.Equal(t, "child-2", verifyRun1.TaskRef)
	require.Equal(t, "epic-1", verifyRun1.EpicRef)
	require.Equal(t, storage.TaskRunStatusSucceeded, verifyRun1.Status)

	run2 := st.runs["run-4"]
	require.Equal(t, "child-3", run2.TaskRef)
	require.Equal(t, "epic-1", run2.EpicRef)
	require.Equal(t, storage.TaskRunStatusSucceeded, run2.Status)

	verifyRun2 := st.runs["run-5"]
	require.Equal(t, "child-3", verifyRun2.TaskRef)
	require.Equal(t, "epic-1", verifyRun2.EpicRef)
	require.Equal(t, storage.TaskRunStatusSucceeded, verifyRun2.Status)
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

	cfg := stubConfig{
		config.ExecutionTaskPromptKey:    "exec",
		config.VerificationTaskPromptKey: "verify",
	}

	w := NewWorker(nil, tr, cp, st, nt, cfg, "/repo", zap.NewNop())
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
	cfg := stubConfig{
		config.ExecutionTaskPromptKey:    "exec",
		config.VerificationTaskPromptKey: "verify",
	}

	w := NewWorker(nil, tr, cp, st, nt, cfg, "/repo", zap.NewNop())
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

func TestExecuteTask_Success(t *testing.T) {
	ctx := context.Background()
	st := newFakeStorage()
	batch := storage.Batch{ID: "b1", Status: storage.BatchStatusCreated}
	st.batches["b1"] = batch

	cp := &fakeCopilot{events: closedChan(), sessionID: "s-success"}
	nt := &fakeNotifier{}

	now := time.Date(2026, 2, 8, 16, 0, 0, 0, time.UTC)
	status, err := executeTask(
		ctx,
		cp,
		st,
		nt,
		zap.NewNop(),
		"/repo",
		func() string { return "run-success" },
		func() time.Time { return now },
		&batch,
		"task-1",
		"epic-1",
		"prompt",
	)
	require.NoError(t, err)
	require.Equal(t, storage.TaskRunStatusSucceeded, status)

	run := st.runs["run-success"]
	require.Equal(t, "task-1", run.TaskRef)
	require.Equal(t, "epic-1", run.EpicRef)
	require.Equal(t, "s-success", run.SessionID)
	require.Equal(t, storage.TaskRunStatusSucceeded, run.Status)
	require.Equal(t, &now, run.FinishedAt)

	require.Len(t, nt.finished, 1)
	require.Equal(t, storage.BatchStatusCreated, st.batches["b1"].Status)
}

func TestExecuteTask_StartSessionErrorSetsFailed(t *testing.T) {
	ctx := context.Background()
	st := newFakeStorage()
	batch := storage.Batch{ID: "b1", Status: storage.BatchStatusCreated}
	st.batches["b1"] = batch

	cp := &fakeCopilot{startErr: errors.New("boom")}
	nt := &fakeNotifier{}

	now := time.Date(2026, 2, 8, 17, 0, 0, 0, time.UTC)
	status, err := executeTask(
		ctx,
		cp,
		st,
		nt,
		zap.NewNop(),
		"/repo",
		func() string { return "run-fail" },
		func() time.Time { return now },
		&batch,
		"task-err",
		"",
		"prompt",
	)

	require.Error(t, err)
	require.Equal(t, storage.TaskRunStatusFailed, status)

	run := st.runs["run-fail"]
	require.Equal(t, storage.TaskRunStatusFailed, run.Status)
	require.Equal(t, storage.BatchStatusFailed, st.batches["b1"].Status)
	require.Len(t, nt.errors, 1)
}

func TestExecuteTaskWithStructuredOutput_Success(t *testing.T) {
	ctx := context.Background()
	st := newFakeStorage()
	batch := storage.Batch{ID: "b1", Status: storage.BatchStatusCreated}
	st.batches["b1"] = batch

	cp := &fakeCopilot{events: closedChan(), sessionID: "s-structured"}
	nt := &fakeNotifier{}

	now := time.Date(2026, 2, 8, 18, 0, 0, 0, time.UTC)
	status, resp, err := executeTaskWithStructuredOutput(
		ctx,
		cp,
		st,
		nt,
		zap.NewNop(),
		"/repo",
		func() string { return "run-structured" },
		func() time.Time { return now },
		&batch,
		"task-structured",
		"epic-structured",
		"prompt",
	)
	require.NoError(t, err)
	require.Equal(t, storage.TaskRunStatusSucceeded, status)
	require.Empty(t, resp)

	run := st.runs["run-structured"]
	require.Equal(t, "task-structured", run.TaskRef)
	require.Equal(t, "epic-structured", run.EpicRef)
	require.Equal(t, &now, run.FinishedAt)
}

func TestExecuteTaskWithStructuredOutput_ReturnsLatestAssistantMessage(t *testing.T) {
	ctx := context.Background()
	st := newFakeStorage()
	batch := storage.Batch{ID: "b1", Status: storage.BatchStatusCreated}
	st.batches["b1"] = batch

	events := make(chan copilot.RawEvent, 3)
	events <- copilot.RawEvent{"type": "assistant.message", "data": map[string]any{"content": "old"}}
	events <- copilot.RawEvent{"type": "user.message", "data": map[string]any{"content": "user"}}
	events <- copilot.RawEvent{"type": "assistant.message", "data": map[string]any{"content": `{"ok":true}`}}
	close(events)

	cp := &fakeCopilot{events: events, sessionID: "s-structured-content"}
	nt := &fakeNotifier{}

	now := time.Date(2026, 2, 8, 18, 30, 0, 0, time.UTC)
	status, resp, err := executeTaskWithStructuredOutput(
		ctx,
		cp,
		st,
		nt,
		zap.NewNop(),
		"/repo",
		func() string { return "run-structured-output" },
		func() time.Time { return now },
		&batch,
		"task-structured-output",
		"epic-structured-output",
		"prompt",
	)
	require.NoError(t, err)
	require.Equal(t, storage.TaskRunStatusSucceeded, status)
	require.Equal(t, `{"ok":true}`, resp)

	run := st.runs["run-structured-output"]
	require.Equal(t, "s-structured-content", run.SessionID)
	require.Len(t, st.sessionEvents, 3)
}

func TestExecuteTaskWithStructuredOutput_StartSessionErrorSetsFailed(t *testing.T) {
	ctx := context.Background()
	st := newFakeStorage()
	batch := storage.Batch{ID: "b1", Status: storage.BatchStatusCreated}
	st.batches["b1"] = batch

	cp := &fakeCopilot{startErr: errors.New("structured boom")}
	nt := &fakeNotifier{}

	now := time.Date(2026, 2, 8, 19, 0, 0, 0, time.UTC)
	status, resp, err := executeTaskWithStructuredOutput(
		ctx,
		cp,
		st,
		nt,
		zap.NewNop(),
		"/repo",
		func() string { return "run-structured-fail" },
		func() time.Time { return now },
		&batch,
		"task-structured-fail",
		"",
		"prompt",
	)

	require.Error(t, err)
	require.Equal(t, storage.TaskRunStatusFailed, status)
	require.Empty(t, resp)

	run := st.runs["run-structured-fail"]
	require.Equal(t, storage.TaskRunStatusFailed, run.Status)
	require.Equal(t, storage.BatchStatusFailed, st.batches["b1"].Status)
	require.Len(t, nt.errors, 1)
}

func TestSetBatchStatus(t *testing.T) {
	ctx := context.Background()
	st := newFakeStorage()
	batch := storage.Batch{ID: "b1", Status: storage.BatchStatusCreated}
	st.batches["b1"] = batch

	setBatchStatus(ctx, st, zap.NewNop(), nil, storage.BatchStatusRunning)
	require.Equal(t, 0, st.updateBatchCalls)

	setBatchStatus(ctx, st, zap.NewNop(), &batch, storage.BatchStatusCreated)
	require.Equal(t, 0, st.updateBatchCalls)

	setBatchStatus(ctx, st, zap.NewNop(), &batch, storage.BatchStatusRunning)
	require.Equal(t, 1, st.updateBatchCalls)
	require.Equal(t, storage.BatchStatusRunning, st.batches["b1"].Status)
}

func closedChan() chan copilot.RawEvent {
	ch := make(chan copilot.RawEvent)
	close(ch)
	return ch
}

// fakes below ---------------------------------------------------------------

type fakeCopilot struct {
	prompts    []string
	repoPaths  []string
	events     chan copilot.RawEvent
	eventQueue []chan copilot.RawEvent
	sessionID  string
	sessionIDs []string
	startErr   error
	stopped    bool
	stopCalls  int
}

func (f *fakeCopilot) StartSession(ctx context.Context, prompt, repoPath string) (string, <-chan copilot.RawEvent, func(), error) {
	if f.startErr != nil {
		return "", nil, nil, f.startErr
	}

	f.prompts = append(f.prompts, prompt)
	f.repoPaths = append(f.repoPaths, repoPath)

	sessionID := f.sessionID
	if len(f.sessionIDs) > 0 {
		sessionID = f.sessionIDs[0]
		f.sessionIDs = f.sessionIDs[1:]
	}

	events := f.events
	if len(f.eventQueue) > 0 {
		events = f.eventQueue[0]
		f.eventQueue = f.eventQueue[1:]
	}
	if events == nil {
		events = closedChan()
	}

	stop := func() {
		f.stopped = true
		f.stopCalls++
	}
	return sessionID, events, stop, nil
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

func (f *fakeTracker) AddComment(ctx context.Context, ref string, text string) error {
	return nil
}

func (f *fakeTracker) FetchComments(ctx context.Context, ref string) ([]tracker.Comment, error) {
	return []tracker.Comment{}, nil
}

type fakeStorage struct {
	batches          map[string]storage.Batch
	runs             map[string]storage.TaskRun
	sessionEvents    []storage.SessionEvent
	updateBatchCalls int
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
	f.updateBatchCalls++
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
