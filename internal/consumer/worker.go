package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/barzoj/yaralpho/internal/copilot"
	"github.com/barzoj/yaralpho/internal/notify"
	"github.com/barzoj/yaralpho/internal/queue"
	"github.com/barzoj/yaralpho/internal/storage"
	"github.com/barzoj/yaralpho/internal/tracker"
	"go.uber.org/zap"
)

// QueueItem represents a single unit of work pulled by the consumer. Items are
// encoded as JSON strings before enqueueing to keep the queue dependency-free.
type QueueItem struct {
	BatchID string `json:"batch_id"`
	TaskRef string `json:"task_ref"`
}

// EncodeQueueItem serializes a QueueItem for enqueueing.
func EncodeQueueItem(item QueueItem) (string, error) {
	b, err := json.Marshal(item)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Worker consumes queue items and orchestrates task runs via Copilot while
// persisting progress and emitting notifications.
type Worker struct {
	queue    queue.Queue
	tracker  tracker.Tracker
	copilot  copilot.Client
	storage  storage.Storage
	notifier notify.Notifier
	repoPath string
	logger   *zap.Logger
	now      func() time.Time
	newRunID func() string
}

// NewWorker constructs a Worker with sensible defaults for logger, notifier,
// clock, and run ID generation.
func NewWorker(q queue.Queue, tr tracker.Tracker, cp copilot.Client, st storage.Storage, nt notify.Notifier, repoPath string, logger *zap.Logger) *Worker {
	if logger == nil {
		logger = zap.NewNop()
	}
	if nt == nil {
		nt = notify.Noop{}
	}

	return &Worker{
		queue:    q,
		tracker:  tr,
		copilot:  cp,
		storage:  st,
		notifier: nt,
		repoPath: strings.TrimSpace(repoPath),
		logger:   logger,
		now: func() time.Time {
			return time.Now().UTC()
		},
		newRunID: func() string {
			return fmt.Sprintf("run-%d", time.Now().UTC().UnixNano())
		},
	}
}

// Run blocks until the queue is closed or the context is cancelled. Errors
// while handling individual items are logged and processing continues.
func (w *Worker) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	for {
		raw, err := w.queue.Dequeue(ctx)
		if err != nil {
			if errors.Is(err, queue.ErrClosed) {
				w.logger.Info("queue closed; consumer exiting")
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			w.logger.Error("dequeue", zap.Error(err))
			return err
		}

		if err := w.handleItem(ctx, raw); err != nil {
			w.logger.Error("process queue item", zap.Error(err))
		}
	}
}

func (w *Worker) handleItem(ctx context.Context, raw string) error {
	var item QueueItem
	if err := json.Unmarshal([]byte(raw), &item); err != nil {
		return fmt.Errorf("decode queue item: %w", err)
	}
	item.BatchID = strings.TrimSpace(item.BatchID)
	item.TaskRef = strings.TrimSpace(item.TaskRef)
	if item.BatchID == "" || item.TaskRef == "" {
		return fmt.Errorf("queue item missing batch_id or task_ref")
	}

	batch, err := w.storage.GetBatch(ctx, item.BatchID)
	if err != nil {
		_ = w.notifier.NotifyError(ctx, item.BatchID, "", item.TaskRef, err)
		return fmt.Errorf("get batch %s: %w", item.BatchID, err)
	}
	w.setBatchStatus(ctx, batch, storage.BatchStatusRunning)

	runs, err := w.storage.ListTaskRuns(ctx, item.BatchID)
	if err != nil {
		w.logger.Warn("list task runs", zap.Error(err), zap.String("batch_id", item.BatchID))
	}

	runRef := item.TaskRef
	epicRef := ""
	prompt := fmt.Sprintf("Work on task %s", runRef)

	isEpic, err := w.tracker.IsEpic(ctx, item.TaskRef)
	if err != nil {
		_ = w.notifier.NotifyError(ctx, item.BatchID, "", item.TaskRef, err)
		w.setBatchStatus(ctx, batch, storage.BatchStatusFailed)
		return fmt.Errorf("tracker is-epic: %w", err)
	}

	if isEpic {
		children, err := w.tracker.ListChildren(ctx, item.TaskRef)
		if err != nil {
			_ = w.notifier.NotifyError(ctx, item.BatchID, "", item.TaskRef, err)
			w.setBatchStatus(ctx, batch, storage.BatchStatusFailed)
			return fmt.Errorf("tracker list children: %w", err)
		}

		child, ok := firstAvailableChild(children, runs)
		if !ok {
			w.setBatchStatus(ctx, batch, storage.BatchStatusIdle)
			_ = w.notifier.NotifyBatchIdle(ctx, item.BatchID)
			w.logger.Info("no remaining child tasks for epic", zap.String("epic", item.TaskRef))
			return nil
		}

		runRef = child
		epicRef = item.TaskRef
		prompt = fmt.Sprintf("Pick first ready task from epic %s and execute", epicRef)
	}

	runID := w.newRunID()

	sessionID, events, stop, err := w.copilot.StartSession(ctx, prompt, w.repoPath)
	if err != nil {
		finished := w.now()
		run := storage.TaskRun{
			ID:         runID,
			BatchID:    batch.ID,
			TaskRef:    runRef,
			EpicRef:    epicRef,
			SessionID:  "",
			StartedAt:  w.now(),
			FinishedAt: &finished,
			Status:     storage.TaskRunStatusFailed,
		}
		if createErr := w.storage.CreateTaskRun(ctx, &run); createErr != nil {
			w.logger.Error("record failed run", zap.Error(createErr), zap.String("run_id", run.ID))
		}

		w.setBatchStatus(ctx, batch, storage.BatchStatusFailed)
		_ = w.notifier.NotifyError(ctx, batch.ID, runID, runRef, err)
		return fmt.Errorf("start copilot session: %w", err)
	}
	defer stop()

	run := storage.TaskRun{
		ID:        runID,
		BatchID:   batch.ID,
		TaskRef:   runRef,
		EpicRef:   epicRef,
		SessionID: sessionID,
		StartedAt: w.now(),
		Status:    storage.TaskRunStatusRunning,
	}

	if err := w.storage.CreateTaskRun(ctx, &run); err != nil {
		w.setBatchStatus(ctx, batch, storage.BatchStatusFailed)
		_ = w.notifier.NotifyError(ctx, batch.ID, runID, runRef, err)
		return fmt.Errorf("create task run: %w", err)
	}

	status := storage.TaskRunStatusSucceeded
	var finalErr error

eventLoop:
	for {
		select {
		case <-ctx.Done():
			status = storage.TaskRunStatusStopped
			finalErr = ctx.Err()
			break eventLoop
		case evt, ok := <-events:
			if !ok {
				break eventLoop
			}

			err := w.storage.InsertSessionEvent(ctx, &storage.SessionEvent{
				BatchID:    batch.ID,
				RunID:      run.ID,
				SessionID:  sessionID,
				Event:      evt,
				IngestedAt: w.now(),
			})
			if err != nil {
				status = storage.TaskRunStatusFailed
				finalErr = err
				w.logger.Error("insert session event", zap.Error(err), zap.String("run_id", run.ID))
				break eventLoop
			}

			if isSessionIdleEvent(evt) {
				w.logger.Debug("copilot session idle event received; stopping run", zap.String("run_id", run.ID), zap.String("session_id", sessionID))
				break eventLoop
			}
		}
	}

	finished := w.now()
	run.Status = status
	run.FinishedAt = &finished

	if err := w.storage.UpdateTaskRun(ctx, &run); err != nil {
		w.logger.Error("update task run", zap.Error(err), zap.String("run_id", run.ID))
	}

	switch status {
	case storage.TaskRunStatusSucceeded:
		_ = w.notifier.NotifyTaskFinished(ctx, batch.ID, run.ID, run.TaskRef, string(run.Status), "")
		w.setBatchStatus(ctx, batch, storage.BatchStatusIdle)
	case storage.TaskRunStatusStopped:
		_ = w.notifier.NotifyBatchIdle(ctx, batch.ID)
		w.setBatchStatus(ctx, batch, storage.BatchStatusIdle)
	default:
		if finalErr == nil {
			finalErr = fmt.Errorf("task run failed")
		}
		_ = w.notifier.NotifyError(ctx, batch.ID, run.ID, run.TaskRef, finalErr)
		w.setBatchStatus(ctx, batch, storage.BatchStatusFailed)
	}

	return finalErr
}

func (w *Worker) setBatchStatus(ctx context.Context, batch *storage.Batch, status storage.BatchStatus) {
	if batch == nil {
		return
	}
	if batch.Status == status {
		return
	}
	batch.Status = status
	if err := w.storage.UpdateBatch(ctx, batch); err != nil {
		w.logger.Error("update batch status", zap.Error(err), zap.String("batch_id", batch.ID))
	}
}

func isSessionIdleEvent(evt copilot.RawEvent) bool {
	val, ok := evt["type"]
	if !ok {
		return false
	}

	eventType, ok := val.(string)
	if !ok {
		return false
	}

	return strings.EqualFold(eventType, "session.idle")
}

func firstAvailableChild(children []string, runs []storage.TaskRun) (string, bool) {
	seen := make(map[string]struct{}, len(runs))
	for _, r := range runs {
		seen[strings.TrimSpace(r.TaskRef)] = struct{}{}
	}

	for _, child := range children {
		child = strings.TrimSpace(child)
		if child == "" {
			continue
		}
		if _, ok := seen[child]; !ok {
			return child, true
		}
	}
	return "", false
}
