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

// ralphSystemMessage is injected into every Copilot prompt to enforce unattended, non-interactive runs.
const ralphSystemMessage = "You are running unattended in Ralph mode. There is no human to answer questions. Do not ask clarifying questions. Make safe assumptions, prefer existing data fields, and avoid layout changes unless strictly necessary. Proceed autonomously"

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

	// eventInactivityTimeout guards against Copilot sessions that stop emitting
	// events without closing. When exceeded, the session is restarted up to
	// maxSessionRestarts times.
	eventInactivityTimeout time.Duration
	maxSessionRestarts     int
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
		eventInactivityTimeout: 5 * time.Minute,
		maxSessionRestarts:     1,
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

	isEpic, err := w.tracker.IsEpic(ctx, item.TaskRef)
	if err != nil {
		_ = w.notifier.NotifyError(ctx, item.BatchID, "", item.TaskRef, err)
		w.setBatchStatus(ctx, batch, storage.BatchStatusFailed)
		return fmt.Errorf("tracker is-epic: %w", err)
	}

	if !isEpic {
		prompt := fmt.Sprintf("Work on task %s", item.TaskRef)
		status, finalErr := w.executeTaskRun(ctx, batch, item.TaskRef, "", prompt)
		if status == storage.TaskRunStatusSucceeded {
			w.setBatchStatus(ctx, batch, storage.BatchStatusIdle)
			return nil
		}

		return finalErr
	}

	for {
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

		prompt := fmt.Sprintf("Pick first ready task from epic %s and execute", item.TaskRef)
		status, finalErr := w.executeTaskRun(ctx, batch, child, item.TaskRef, prompt)
		runs = append(runs, storage.TaskRun{TaskRef: child})

		if status != storage.TaskRunStatusSucceeded {
			return finalErr
		}
	}
}

func (w *Worker) executeTaskRun(ctx context.Context, batch *storage.Batch, runRef, epicRef, prompt string) (storage.TaskRunStatus, error) {
	runID := w.newRunID()
	run := storage.TaskRun{
		ID:        runID,
		BatchID:   batch.ID,
		TaskRef:   runRef,
		EpicRef:   epicRef,
		StartedAt: w.now(),
		Status:    storage.TaskRunStatusRunning,
	}

	status := storage.TaskRunStatusSucceeded
	var finalErr error

	if err := w.storage.CreateTaskRun(ctx, &run); err != nil {
		w.setBatchStatus(ctx, batch, storage.BatchStatusFailed)
		_ = w.notifier.NotifyError(ctx, batch.ID, runID, runRef, err)
		return storage.TaskRunStatusFailed, fmt.Errorf("create task run: %w", err)
	}

	attempt := 0

forSessions:
	for {
		if attempt > w.maxSessionRestarts {
			status = storage.TaskRunStatusFailed
			if finalErr == nil {
				finalErr = fmt.Errorf("copilot session exhausted restarts")
			}
			break
		}
		attempt++

		finalPrompt := ralphSystemMessage + "\n\n" + prompt
		sessionID, events, stop, err := w.copilot.StartSession(ctx, finalPrompt, w.repoPath)
		if err != nil {
			finished := w.now()
			run.SessionID = ""
			run.Status = storage.TaskRunStatusFailed
			run.FinishedAt = &finished
			_ = w.storage.UpdateTaskRun(ctx, &run)
			w.setBatchStatus(ctx, batch, storage.BatchStatusFailed)
			_ = w.notifier.NotifyError(ctx, batch.ID, runID, runRef, err)
			return storage.TaskRunStatusFailed, fmt.Errorf("start copilot session: %w", err)
		}
		w.logger.Info("copilot session started", zap.String("session_id", sessionID), zap.String("run_id", run.ID), zap.Int("attempt", attempt))

		run.SessionID = sessionID
		if err := w.storage.UpdateTaskRun(ctx, &run); err != nil {
			w.logger.Error("update task run session", zap.Error(err), zap.String("run_id", run.ID))
		}

		timer := time.NewTimer(w.eventInactivityTimeout)

		for {
			select {
			case <-ctx.Done():
				status = storage.TaskRunStatusStopped
				finalErr = ctx.Err()
				stop()
				timer.Stop()
				break forSessions
			case <-timer.C:
				w.logger.Warn("copilot session inactive; restarting", zap.String("run_id", run.ID), zap.String("session_id", sessionID), zap.Duration("timeout", w.eventInactivityTimeout))
				stop()
				timer.Stop()
				finalErr = fmt.Errorf("no copilot events for %s", w.eventInactivityTimeout)
				continue forSessions
			case evt, ok := <-events:
				if !ok {
					stop()
					timer.Stop()
					break forSessions
				}
				timer.Reset(w.eventInactivityTimeout)

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
					stop()
					timer.Stop()
					break forSessions
				}

				if isSessionErrorEvent(evt) {
					err := fmt.Errorf("copilot session error: %s", extractSessionErrorMessage(evt))
					status = storage.TaskRunStatusFailed
					finalErr = err
					w.logger.Error("copilot session error", zap.Error(err), zap.String("run_id", run.ID), zap.String("session_id", sessionID))
					if isAuthorizationError(evt) {
						w.setBatchStatus(ctx, batch, storage.BatchStatusBlockedAuth)
					}
					_ = w.notifier.NotifyError(ctx, batch.ID, run.ID, run.TaskRef, err)
					stop()
					timer.Stop()
					break forSessions
				}

				if isSessionIdleEvent(evt) {
					w.logger.Debug("copilot session idle event received; stopping run", zap.String("run_id", run.ID), zap.String("session_id", sessionID))
					stop()
					timer.Stop()
					break forSessions
				}
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
	case storage.TaskRunStatusStopped:
		_ = w.notifier.NotifyBatchIdle(ctx, batch.ID)
		w.setBatchStatus(ctx, batch, storage.BatchStatusIdle)
	default:
		if finalErr == nil {
			finalErr = fmt.Errorf("task run failed")
		}
		_ = w.notifier.NotifyError(ctx, batch.ID, run.ID, run.TaskRef, finalErr)
		if batch.Status != storage.BatchStatusBlockedAuth {
			w.setBatchStatus(ctx, batch, storage.BatchStatusFailed)
		}
	}

	return status, finalErr
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

func isSessionErrorEvent(evt copilot.RawEvent) bool {
	val, ok := evt["type"]
	if !ok {
		return false
	}

	eventType, ok := val.(string)
	if !ok {
		return false
	}

	return strings.EqualFold(eventType, "session.error")
}

func extractSessionErrorMessage(evt copilot.RawEvent) string {
	data, ok := evt["data"].(map[string]any)
	if !ok {
		return ""
	}

	if msg, ok := data["message"].(string); ok {
		return msg
	}

	if errVal, ok := data["error"].(string); ok {
		return errVal
	}

	return ""
}

func isAuthorizationError(evt copilot.RawEvent) bool {
	data, ok := evt["data"].(map[string]any)
	if !ok {
		return false
	}

	typ, ok := data["errorType"].(string)
	if !ok {
		return false
	}

	return strings.EqualFold(typ, "authorization")
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
