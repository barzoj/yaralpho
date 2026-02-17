package consumer

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/barzoj/yaralpho/internal/bus"
	"github.com/barzoj/yaralpho/internal/config"
	"github.com/barzoj/yaralpho/internal/copilot"
	"github.com/barzoj/yaralpho/internal/notify"
	"github.com/barzoj/yaralpho/internal/storage"
	"github.com/barzoj/yaralpho/internal/tracker"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestWorker_TaskPromptAndEvents(t *testing.T) {
	ctx := context.Background()
	batch := storage.Batch{ID: "b1", Status: storage.BatchStatusPending}

	st := newFakeStorage()
	st.batches["b1"] = batch

	cp := &fakeCopilot{
		events:     make(chan copilot.RawEvent, 2),
		sessionIDs: []string{"s123", "s123-verify"},
	}
	cp.events <- copilot.RawEvent{"type": "event1"}
	cp.events <- copilot.RawEvent{"type": "event2"}
	close(cp.events)

	tr := &fakeTracker{titles: map[string]string{"task-1": "Task One"}}

	nt := &fakeNotifier{}
	cfg := stubConfig{
		config.ExecutionTaskPromptKey:    "exec base",
		config.VerificationTaskPromptKey: "verify base",
	}

	w := NewWorker(tr, cp, st, nt, cfg, zap.NewNop())
	nextRun := 0
	w.newRunID = func() string {
		nextRun++
		return fmt.Sprintf("run-%d", nextRun)
	}
	now := time.Date(2026, 2, 8, 12, 0, 0, 0, time.UTC)
	w.now = func() time.Time { return now }

	err := w.Process(ctx, WorkItem{BatchID: "b1", TaskRef: "task-1", Runtime: "codex"})
	require.NoError(t, err)

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
	require.Equal(t, storage.BatchStatusPending, st.batches["b1"].Status)

	require.Len(t, nt.events, 4)
	require.Equal(t, notify.Event{Type: "task_received", BatchID: "b1", TaskRef: "task-1", TaskName: "Task One", Status: "pending"}, nt.events[0])
	require.Equal(t, notify.Event{Type: "task_started", BatchID: "b1", TaskRef: "task-1", TaskName: "Task One", Status: "running"}, nt.events[1])
	require.Equal(t, notify.Event{Type: "attempt_started", BatchID: "b1", TaskRef: "task-1", TaskName: "Task One", Status: "in_progress", Attempt: 1}, nt.events[2])
	require.Equal(t, notify.Event{Type: "verification_succeeded", BatchID: "b1", TaskRef: "task-1", TaskName: "Task One", Status: "succeeded", Attempt: 1, Details: "Agent response unavailable."}, nt.events[3])

	require.Len(t, nt.finished, 2)
	require.Equal(t, notifyFinished{batchID: "b1", runID: "run-1", taskRef: "task-1", status: "succeeded"}, nt.finished[0])
	require.Equal(t, notifyFinished{batchID: "b1", runID: "run-2", taskRef: "task-1", status: "succeeded"}, nt.finished[1])
}

func TestWorker_StopsOnSessionIdleEvent(t *testing.T) {
	ctx := context.Background()
	st := newFakeStorage()
	st.batches["b1"] = storage.Batch{ID: "b1", Status: storage.BatchStatusPending}

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

	w := NewWorker(tr, cp, st, nt, cfg, zap.NewNop())
	nextRun := 0
	w.newRunID = func() string {
		nextRun++
		return fmt.Sprintf("run-idle-%d", nextRun)
	}
	now := time.Date(2026, 2, 8, 15, 0, 0, 0, time.UTC)
	w.now = func() time.Time { return now }

	err := w.Process(ctx, WorkItem{BatchID: "b1", TaskRef: "task-idle", Runtime: "codex"})
	require.NoError(t, err)

	run := st.runs["run-idle-1"]
	require.Equal(t, storage.TaskRunStatusSucceeded, run.Status)
	require.Equal(t, "s-idle", run.SessionID)

	verifyRun := st.runs["run-idle-2"]
	require.Equal(t, storage.TaskRunStatusSucceeded, verifyRun.Status)
	require.Equal(t, "s-verify", verifyRun.SessionID)

	require.Len(t, st.sessionEvents, 1)
	require.Equal(t, storage.BatchStatusPending, st.batches["b1"].Status)
	require.True(t, cp.stopped)
}

func TestWorker_ExecutionListHappyPath(t *testing.T) {
	ctx := context.Background()
	st := newFakeStorage()
	st.batches["b1"] = storage.Batch{ID: "b1", Status: storage.BatchStatusPending}

	eventQueue := make([]chan copilot.RawEvent, 0, 6)
	for i := 0; i < 6; i++ {
		eventQueue = append(eventQueue, closedChan())
	}

	cp := &fakeCopilot{
		eventQueue: eventQueue,
		sessionIDs: []string{"s1", "s1-verify", "s2", "s2-verify", "s3", "s3-verify"},
	}

	tr := &fakeTracker{
		titles: map[string]string{
			"task-1": "Task One",
			"task-2": "Task Two",
			"task-3": "Task Three",
		},
	}

	nt := &fakeNotifier{}
	cfg := stubConfig{
		config.ExecutionTaskPromptKey:    "exec",
		config.VerificationTaskPromptKey: "verify",
	}

	w := NewWorker(tr, cp, st, nt, cfg, zap.NewNop())
	startRun := 1
	w.newRunID = func() string {
		id := fmt.Sprintf("run-%d", startRun)
		startRun++
		return id
	}
	w.now = func() time.Time { return time.Date(2026, 2, 8, 22, 0, 0, 0, time.UTC) }

	require.NoError(t, w.Process(ctx, WorkItem{BatchID: "b1", TaskRef: "task-1", Runtime: "codex"}))
	require.NoError(t, w.Process(ctx, WorkItem{BatchID: "b1", TaskRef: "task-2", Runtime: "codex"}))
	require.NoError(t, w.Process(ctx, WorkItem{BatchID: "b1", TaskRef: "task-3", Runtime: "codex"}))

	require.Equal(t, []string{
		"exec\n\nWork on task task-1",
		"verify\n\nVerify task task-1",
		"exec\n\nWork on task task-2",
		"verify\n\nVerify task task-2",
		"exec\n\nWork on task task-3",
		"verify\n\nVerify task task-3",
	}, cp.prompts)

	require.Equal(t, storage.BatchStatusPending, st.batches["b1"].Status)
	require.Empty(t, nt.batchIdle)

	require.Len(t, nt.events, 12)
	require.Equal(t, notify.Event{Type: "task_received", BatchID: "b1", TaskRef: "task-1", TaskName: "Task One", Status: "pending"}, nt.events[0])
	require.Equal(t, notify.Event{Type: "task_started", BatchID: "b1", TaskRef: "task-1", TaskName: "Task One", Status: "running"}, nt.events[1])
	require.Equal(t, notify.Event{Type: "attempt_started", BatchID: "b1", TaskRef: "task-1", TaskName: "Task One", Status: "in_progress", Attempt: 1}, nt.events[2])
	require.Equal(t, notify.Event{Type: "verification_succeeded", BatchID: "b1", TaskRef: "task-1", TaskName: "Task One", Status: "succeeded", Attempt: 1, Details: "Agent response unavailable."}, nt.events[3])
	require.Equal(t, notify.Event{Type: "task_received", BatchID: "b1", TaskRef: "task-2", TaskName: "Task Two", Status: "pending"}, nt.events[4])
	require.Equal(t, notify.Event{Type: "task_started", BatchID: "b1", TaskRef: "task-2", TaskName: "Task Two", Status: "running"}, nt.events[5])
	require.Equal(t, notify.Event{Type: "attempt_started", BatchID: "b1", TaskRef: "task-2", TaskName: "Task Two", Status: "in_progress", Attempt: 1}, nt.events[6])
	require.Equal(t, notify.Event{Type: "verification_succeeded", BatchID: "b1", TaskRef: "task-2", TaskName: "Task Two", Status: "succeeded", Attempt: 1, Details: "Agent response unavailable."}, nt.events[7])
	require.Equal(t, notify.Event{Type: "task_received", BatchID: "b1", TaskRef: "task-3", TaskName: "Task Three", Status: "pending"}, nt.events[8])
	require.Equal(t, notify.Event{Type: "task_started", BatchID: "b1", TaskRef: "task-3", TaskName: "Task Three", Status: "running"}, nt.events[9])
	require.Equal(t, notify.Event{Type: "attempt_started", BatchID: "b1", TaskRef: "task-3", TaskName: "Task Three", Status: "in_progress", Attempt: 1}, nt.events[10])
	require.Equal(t, notify.Event{Type: "verification_succeeded", BatchID: "b1", TaskRef: "task-3", TaskName: "Task Three", Status: "succeeded", Attempt: 1, Details: "Agent response unavailable."}, nt.events[11])

	require.Len(t, nt.finished, 6)
	require.Equal(t, notifyFinished{batchID: "b1", runID: "run-1", taskRef: "task-1", status: "succeeded"}, nt.finished[0])
	require.Equal(t, notifyFinished{batchID: "b1", runID: "run-2", taskRef: "task-1", status: "succeeded"}, nt.finished[1])
	require.Equal(t, notifyFinished{batchID: "b1", runID: "run-3", taskRef: "task-2", status: "succeeded"}, nt.finished[2])
	require.Equal(t, notifyFinished{batchID: "b1", runID: "run-4", taskRef: "task-2", status: "succeeded"}, nt.finished[3])
	require.Equal(t, notifyFinished{batchID: "b1", runID: "run-5", taskRef: "task-3", status: "succeeded"}, nt.finished[4])
	require.Equal(t, notifyFinished{batchID: "b1", runID: "run-6", taskRef: "task-3", status: "succeeded"}, nt.finished[5])
}

func TestWorker_StartSessionErrorMarksFailed(t *testing.T) {
	ctx := context.Background()
	st := newFakeStorage()
	st.batches["b1"] = storage.Batch{ID: "b1", Status: storage.BatchStatusPending}

	cp := &fakeCopilot{startErr: errors.New("no token")}
	tr := &fakeTracker{}
	nt := &fakeNotifier{}
	cfg := stubConfig{
		config.ExecutionTaskPromptKey:    "exec",
		config.VerificationTaskPromptKey: "verify",
	}

	w := NewWorker(tr, cp, st, nt, cfg, zap.NewNop())
	w.newRunID = func() string { return "run-fail" }
	w.now = func() time.Time { return time.Date(2026, 2, 8, 14, 0, 0, 0, time.UTC) }

	err := w.Process(ctx, WorkItem{BatchID: "b1", TaskRef: "task-err", Runtime: "codex"})
	require.Error(t, err)

	run := st.runs["run-fail"]
	require.Equal(t, storage.TaskRunStatusFailed, run.Status)
	require.Equal(t, storage.BatchStatusFailed, st.batches["b1"].Status)
	require.Len(t, nt.errors, 1)
}

func TestWorker_RunStartsSecondAfterFirstCompletes(t *testing.T) {
	ctx := context.Background()
	st := newFakeStorage()
	st.batches["b1"] = storage.Batch{ID: "b1", Status: storage.BatchStatusPending}
	st.batches["b2"] = storage.Batch{ID: "b2", Status: storage.BatchStatusPending}

	firstExec := make(chan copilot.RawEvent)
	cp := &fakeCopilot{
		eventQueue: []chan copilot.RawEvent{firstExec, closedChan(), closedChan(), closedChan()},
		sessionIDs: []string{"s1", "s1-verify", "s2", "s2-verify"},
	}

	tr := &fakeTracker{}
	nt := &fakeNotifier{}
	cfg := stubConfig{
		config.ExecutionTaskPromptKey:    "exec",
		config.VerificationTaskPromptKey: "verify",
	}

	w := NewWorker(tr, cp, st, nt, cfg, zap.NewNop())

	errCh := make(chan error, 1)
	go func() {
		defer close(errCh)
		errCh <- w.Process(ctx, WorkItem{BatchID: "b1", TaskRef: "task-1"})
	}()

	require.Eventually(t, func() bool {
		return len(cp.prompts) == 1
	}, time.Second, 10*time.Millisecond)
	require.Equal(t, 1, len(cp.prompts))

	close(firstExec)

	require.NoError(t, <-errCh)

	require.NoError(t, w.Process(ctx, WorkItem{BatchID: "b2", TaskRef: "task-2"}))

	require.Equal(t, []string{
		"exec\n\nWork on task task-1",
		"verify\n\nVerify task task-1",
		"exec\n\nWork on task task-2",
		"verify\n\nVerify task task-2",
	}, cp.prompts)

	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.Fail(t, "worker did not stop")
	}
}

func TestWorker_FailedVerificationStopsAfterMaxRetries(t *testing.T) {
	ctx := context.Background()

	st := newFakeStorage()
	st.batches["b1"] = storage.Batch{
		ID:           "b1",
		RepositoryID: "repo-1",
		Status:       storage.BatchStatusPending,
		Items:        []storage.BatchItem{{Input: "task-1", Status: storage.ItemStatusPending}},
	}

	failureEvent := copilot.RawEvent{
		"type": "assistant.message",
		"data": map[string]any{"content": `{"status":"failure","reason":"task_not_closed","details":"status in_progress"}`},
	}

	firstVerify := make(chan copilot.RawEvent, 1)
	firstVerify <- failureEvent
	close(firstVerify)

	secondVerify := make(chan copilot.RawEvent, 1)
	secondVerify <- failureEvent
	close(secondVerify)

	cp := &fakeCopilot{
		eventQueue: []chan copilot.RawEvent{
			closedChan(), // exec attempt 1
			firstVerify,  // verify attempt 1 (fails)
			closedChan(), // exec attempt 2
			secondVerify, // verify attempt 2 (fails, hits max)
		},
		sessionIDs: []string{"exec-1", "verify-1", "exec-2", "verify-2"},
	}

	tr := &fakeTracker{titles: map[string]string{"task-1": "Task One"}}
	nt := &fakeNotifier{}
	cfg := stubConfig{
		config.ExecutionTaskPromptKey:    "exec",
		config.VerificationTaskPromptKey: "verify",
		config.MaxRetriesKey:             "2",
	}

	w := NewWorker(tr, cp, st, nt, cfg, zap.NewNop())
	w.now = func() time.Time { return time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC) }

	err := w.Process(ctx, WorkItem{BatchID: "b1", TaskRef: "task-1", Runtime: "codex"})
	require.Error(t, err, "expected worker to fail after max retries")

	batch := st.batches["b1"]
	require.Equal(t, storage.BatchStatusFailed, batch.Status, "batch should be failed after max retries")

	require.NotEmpty(t, nt.events, "expected notification events recorded")
	lastEvent := nt.events[len(nt.events)-1]
	require.Equal(t, "verification_failed_max_retries", lastEvent.Type, "should emit max retry failure")

	for _, ev := range nt.events {
		require.NotEqual(t, "verification_succeeded", ev.Type, "must not succeed after retry exhaustion")
	}
}

func TestExecuteTask_Success(t *testing.T) {
	ctx := context.Background()
	st := newFakeStorage()
	batch := storage.Batch{ID: "b1", Status: storage.BatchStatusPending}
	st.batches["b1"] = batch

	cp := &fakeCopilot{events: closedChan(), sessionID: "s-success"}
	nt := &fakeNotifier{}

	now := time.Date(2026, 2, 8, 16, 0, 0, 0, time.UTC)
	status, err := executeTask(
		ctx,
		cp,
		st,
		nil,
		nt,
		zap.NewNop(),
		"/repo",
		func() string { return "run-success" },
		func() time.Time { return now },
		&batch,
		"task-1",
		"prompt",
	)
	require.NoError(t, err)
	require.Equal(t, storage.TaskRunStatusSucceeded, status)

	run := st.runs["run-success"]
	require.Equal(t, "task-1", run.TaskRef)
	require.Equal(t, "s-success", run.SessionID)
	require.Equal(t, storage.TaskRunStatusSucceeded, run.Status)
	require.Equal(t, &now, run.FinishedAt)

	require.Len(t, nt.finished, 1)
	require.Equal(t, storage.BatchStatusPending, st.batches["b1"].Status)
}

func TestExecuteTask_StartSessionErrorSetsFailed(t *testing.T) {
	ctx := context.Background()
	st := newFakeStorage()
	batch := storage.Batch{ID: "b1", Status: storage.BatchStatusPending}
	st.batches["b1"] = batch

	cp := &fakeCopilot{startErr: errors.New("boom")}
	nt := &fakeNotifier{}

	now := time.Date(2026, 2, 8, 17, 0, 0, 0, time.UTC)
	status, err := executeTask(
		ctx,
		cp,
		st,
		nil,
		nt,
		zap.NewNop(),
		"/repo",
		func() string { return "run-fail" },
		func() time.Time { return now },
		&batch,
		"task-err",
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
	batch := storage.Batch{ID: "b1", Status: storage.BatchStatusPending}
	st.batches["b1"] = batch

	cp := &fakeCopilot{events: closedChan(), sessionID: "s-structured"}
	nt := &fakeNotifier{}

	now := time.Date(2026, 2, 8, 18, 0, 0, 0, time.UTC)
	status, resp, err := executeTaskWithStructuredOutput(
		ctx,
		cp,
		st,
		nil,
		nt,
		zap.NewNop(),
		"/repo",
		func() string { return "run-structured" },
		func() time.Time { return now },
		&batch,
		"task-structured",
		"prompt",
	)
	require.NoError(t, err)
	require.Equal(t, storage.TaskRunStatusSucceeded, status)
	require.Empty(t, resp)

	run := st.runs["run-structured"]
	require.Equal(t, "task-structured", run.TaskRef)
	require.Equal(t, &now, run.FinishedAt)
}

func TestExecuteTaskWithStructuredOutput_ReturnsLatestAssistantMessage(t *testing.T) {
	ctx := context.Background()
	st := newFakeStorage()
	batch := storage.Batch{ID: "b1", Status: storage.BatchStatusPending}
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
		nil,
		nt,
		zap.NewNop(),
		"/repo",
		func() string { return "run-structured-output" },
		func() time.Time { return now },
		&batch,
		"task-structured-output",
		"prompt",
	)
	require.NoError(t, err)
	require.Equal(t, storage.TaskRunStatusSucceeded, status)
	require.Equal(t, `{"ok":true}`, resp)

	run := st.runs["run-structured-output"]
	require.Equal(t, "s-structured-content", run.SessionID)
	require.Len(t, st.sessionEvents, 3)
}

func TestExecuteTask_PublishesSessionEventsToBus(t *testing.T) {
	ctx := context.Background()
	setSessionEventBus(nil)
	defer setSessionEventBus(nil)

	st := newFakeStorage()
	batch := storage.Batch{ID: "b1", Status: storage.BatchStatusPending}
	st.batches["b1"] = batch

	events := make(chan copilot.RawEvent, 1)
	events <- copilot.RawEvent{"type": "assistant.message", "data": map[string]any{"content": "hi"}}
	close(events)

	cp := &fakeCopilot{events: events, sessionID: "s-bus"}
	nt := &fakeNotifier{}
	fb := &fakeBus{}
	setSessionEventBus(fb)

	now := time.Date(2026, 2, 8, 20, 0, 0, 0, time.UTC)
	status, err := executeTask(
		ctx,
		cp,
		st,
		nil,
		nt,
		zap.NewNop(),
		"/repo",
		func() string { return "run-bus" },
		func() time.Time { return now },
		&batch,
		"task-bus",
		"prompt",
	)
	require.NoError(t, err)
	require.Equal(t, storage.TaskRunStatusSucceeded, status)

	require.Len(t, fb.published, 1)
	require.Equal(t, []string{"s-bus"}, fb.sessionIDs)
	require.Equal(t, "b1", fb.published[0].BatchID)
	require.Equal(t, "run-bus", fb.published[0].RunID)
	require.Equal(t, "s-bus", fb.published[0].SessionID)
	require.Empty(t, nt.errors)
}

func TestExecuteTask_PublishErrorNotifies(t *testing.T) {
	ctx := context.Background()
	setSessionEventBus(nil)
	defer setSessionEventBus(nil)

	st := newFakeStorage()
	batch := storage.Batch{ID: "b1", Status: storage.BatchStatusPending}
	st.batches["b1"] = batch

	events := make(chan copilot.RawEvent, 1)
	events <- copilot.RawEvent{"type": "assistant.message"}
	close(events)

	cp := &fakeCopilot{events: events, sessionID: "s-bus-err"}
	nt := &fakeNotifier{}
	fb := &fakeBus{publishErr: errors.New("publish boom")}
	setSessionEventBus(fb)

	now := time.Date(2026, 2, 8, 21, 0, 0, 0, time.UTC)
	status, err := executeTask(
		ctx,
		cp,
		st,
		nil,
		nt,
		zap.NewNop(),
		"/repo",
		func() string { return "run-bus-err" },
		func() time.Time { return now },
		&batch,
		"task-bus-err",
		"prompt",
	)
	require.NoError(t, err)
	require.Equal(t, storage.TaskRunStatusSucceeded, status)
	require.Len(t, fb.published, 1)
	require.Len(t, nt.errors, 1)
	require.ErrorContains(t, nt.errors[0].err, "publish boom")
}

func TestExecuteTaskWithAssistantMessages_ReturnsAllAssistantMessages(t *testing.T) {
	ctx := context.Background()
	st := newFakeStorage()
	batch := storage.Batch{ID: "b1", Status: storage.BatchStatusPending}
	st.batches["b1"] = batch

	events := make(chan copilot.RawEvent, 4)
	events <- copilot.RawEvent{"type": "assistant.message", "data": map[string]any{"content": "first"}}
	events <- copilot.RawEvent{"type": "user.message", "data": map[string]any{"content": "user"}}
	events <- copilot.RawEvent{"type": "assistant.message", "data": map[string]any{"content": "second"}}
	events <- copilot.RawEvent{"type": "assistant.message", "data": map[string]any{"content": "third"}}
	close(events)

	cp := &fakeCopilot{events: events, sessionID: "s-assistant-messages"}
	nt := &fakeNotifier{}

	now := time.Date(2026, 2, 8, 18, 45, 0, 0, time.UTC)
	status, resp, err := executeTaskWithAssistantMessages(
		ctx,
		cp,
		st,
		nil,
		nt,
		zap.NewNop(),
		"/repo",
		func() string { return "run-assistant-messages" },
		func() time.Time { return now },
		&batch,
		"task-assistant-messages",
		"prompt",
	)
	require.NoError(t, err)
	require.Equal(t, storage.TaskRunStatusSucceeded, status)
	require.Equal(t, "first\nsecond\nthird", resp)

	run := st.runs["run-assistant-messages"]
	require.Equal(t, "s-assistant-messages", run.SessionID)
	require.Len(t, st.sessionEvents, 4)
}

func TestExecuteTaskWithStructuredOutput_StartSessionErrorSetsFailed(t *testing.T) {
	ctx := context.Background()
	st := newFakeStorage()
	batch := storage.Batch{ID: "b1", Status: storage.BatchStatusPending}
	st.batches["b1"] = batch

	cp := &fakeCopilot{startErr: errors.New("structured boom")}
	nt := &fakeNotifier{}

	now := time.Date(2026, 2, 8, 19, 0, 0, 0, time.UTC)
	status, resp, err := executeTaskWithStructuredOutput(
		ctx,
		cp,
		st,
		nil,
		nt,
		zap.NewNop(),
		"/repo",
		func() string { return "run-structured-fail" },
		func() time.Time { return now },
		&batch,
		"task-structured-fail",
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

func TestExecuteTask_Timeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	st := newFakeStorage()
	batch := storage.Batch{ID: "b-timeout", Status: storage.BatchStatusPending}
	st.batches["b-timeout"] = batch

	events := make(chan copilot.RawEvent)
	cp := &fakeCopilot{events: events, sessionID: "s-timeout"}
	nt := &fakeNotifier{}

	now := time.Date(2026, 2, 9, 10, 0, 0, 0, time.UTC)
	status, err := executeTask(
		ctx,
		cp,
		st,
		nil,
		nt,
		zap.NewNop(),
		"/repo",
		func() string { return "run-timeout" },
		func() time.Time { return now },
		&batch,
		"task-timeout",
		"prompt",
	)

	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, storage.TaskRunStatusTimedOut, status)

	run := st.runs["run-timeout"]
	require.Equal(t, storage.TaskRunStatusTimedOut, run.Status)
	require.Equal(t, &now, run.FinishedAt)
	require.True(t, cp.stopped)
	require.Equal(t, 1, cp.stopCalls)
}

func TestSetBatchStatus(t *testing.T) {
	ctx := context.Background()
	st := newFakeStorage()
	batch := storage.Batch{ID: "b1", Status: storage.BatchStatusPending}
	st.batches["b1"] = batch

	setBatchStatus(ctx, st, zap.NewNop(), nil, storage.BatchStatusInProgress)
	require.Equal(t, 0, st.updateBatchCalls)

	setBatchStatus(ctx, st, zap.NewNop(), &batch, storage.BatchStatusPending)
	require.Equal(t, 0, st.updateBatchCalls)

	setBatchStatus(ctx, st, zap.NewNop(), &batch, storage.BatchStatusInProgress)
	require.Equal(t, 1, st.updateBatchCalls)
	require.Equal(t, storage.BatchStatusInProgress, st.batches["b1"].Status)
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
	titles map[string]string
}

func (f *fakeTracker) AddComment(ctx context.Context, repoPath, ref string, text string) error {
	return nil
}

func (f *fakeTracker) FetchComments(ctx context.Context, repoPath, ref string) ([]tracker.Comment, error) {
	return []tracker.Comment{}, nil
}

func (f *fakeTracker) GetTitle(ctx context.Context, repoPath, ref string) (string, error) {
	if f.titles == nil {
		return "", nil
	}
	return f.titles[ref], nil
}

type fakeStorage struct {
	repositories     map[string]storage.Repository
	batches          map[string]storage.Batch
	runs             map[string]storage.TaskRun
	sessionEvents    []storage.SessionEvent
	updateBatchCalls int
}

func newFakeStorage() *fakeStorage {
	return &fakeStorage{
		repositories: map[string]storage.Repository{
			"repo-1": {ID: "repo-1", Path: "/repo"},
		},
		batches: make(map[string]storage.Batch),
		runs:    make(map[string]storage.TaskRun),
	}
}

func (f *fakeStorage) CreateRepository(ctx context.Context, repo *storage.Repository) error {
	if repo == nil {
		return errors.New("repo is nil")
	}
	f.repositories[repo.ID] = *repo
	return nil
}
func (f *fakeStorage) UpdateRepository(ctx context.Context, repo *storage.Repository) error {
	if repo == nil {
		return errors.New("repo is nil")
	}
	f.repositories[repo.ID] = *repo
	return nil
}
func (f *fakeStorage) GetRepository(ctx context.Context, id string) (*storage.Repository, error) {
	repo, ok := f.repositories[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return &repo, nil
}
func (f *fakeStorage) ListRepositories(ctx context.Context) ([]storage.Repository, error) {
	repos := make([]storage.Repository, 0, len(f.repositories))
	for _, r := range f.repositories {
		repos = append(repos, r)
	}
	return repos, nil
}
func (f *fakeStorage) DeleteRepository(ctx context.Context, id string) error {
	delete(f.repositories, id)
	return nil
}
func (f *fakeStorage) RepositoryHasActiveBatches(ctx context.Context, id string) (bool, error) {
	for _, b := range f.batches {
		if b.RepositoryID == id && b.Status != storage.BatchStatusDone && b.Status != storage.BatchStatusFailed {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeStorage) CreateAgent(ctx context.Context, agent *storage.Agent) error { return nil }
func (f *fakeStorage) UpdateAgent(ctx context.Context, agent *storage.Agent) error { return nil }
func (f *fakeStorage) GetAgent(ctx context.Context, id string) (*storage.Agent, error) {
	return nil, errors.New("not found")
}
func (f *fakeStorage) ListAgents(ctx context.Context) ([]storage.Agent, error) { return nil, nil }
func (f *fakeStorage) DeleteAgent(ctx context.Context, id string) error        { return nil }

func (f *fakeStorage) CreateBatch(ctx context.Context, batch *storage.Batch) error {
	if batch.RepositoryID == "" {
		batch.RepositoryID = "repo-1"
	}
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
	if b.RepositoryID == "" {
		b.RepositoryID = "repo-1"
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
func (f *fakeStorage) ListBatchesByRepository(ctx context.Context, repositoryID string, status storage.BatchStatus, limit int64) ([]storage.Batch, error) {
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

func (f *fakeStorage) ListTaskRuns(ctx context.Context, batchID string) ([]storage.TaskRunSummary, error) {
	out := []storage.TaskRunSummary{}
	for _, r := range f.runs {
		if r.BatchID == batchID {
			var total int64
			for _, evt := range f.sessionEvents {
				if evt.RunID == r.ID {
					total++
				}
			}
			out = append(out, storage.TaskRunSummary{TaskRun: r, TotalEvents: total})
		}
	}
	return out, nil
}
func (f *fakeStorage) ListTaskRunsByRepository(ctx context.Context, repositoryID string) ([]storage.TaskRunSummary, error) {
	out := []storage.TaskRunSummary{}
	for _, r := range f.runs {
		if r.RepositoryID == repositoryID {
			out = append(out, storage.TaskRunSummary{TaskRun: r})
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
	events    []notify.Event
	finished  []notifyFinished
	batchIdle []string
	errors    []notifyError
}

type fakeBus struct {
	published  []storage.SessionEvent
	sessionIDs []string
	publishErr error
}

func (f *fakeBus) Publish(ctx context.Context, sessionID string, evt storage.SessionEvent) error {
	f.sessionIDs = append(f.sessionIDs, sessionID)
	f.published = append(f.published, evt)
	return f.publishErr
}

func (f *fakeBus) Subscribe(ctx context.Context, sessionID string) (bus.Subscription, error) {
	return bus.Subscription{}, nil
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

func (f *fakeNotifier) NotifyEvent(ctx context.Context, event notify.Event) error {
	f.events = append(f.events, event)
	return nil
}

func (f *fakeNotifier) NotifyTaskFinished(ctx context.Context, batchID, runID, taskRef, taskName, status, commitHash string) error {
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
