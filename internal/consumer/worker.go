package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/barzoj/yaralpho/internal/config"
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
	cfg      config.Config
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
func NewWorker(q queue.Queue, tr tracker.Tracker, cp copilot.Client, st storage.Storage, nt notify.Notifier, cfg config.Config, repoPath string, logger *zap.Logger) *Worker {
	if logger == nil {
		logger = zap.NewNop()
	}
	if nt == nil {
		nt = notify.Noop{}
	}

	return &Worker{
		cfg:      cfg,
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

	w.notifyTaskEvent(ctx, notify.Event{Type: "task_received", BatchID: item.BatchID, TaskRef: item.TaskRef, Status: "pending"})

	batch, err := w.storage.GetBatch(ctx, item.BatchID)
	if err != nil {
		_ = w.notifier.NotifyError(ctx, item.BatchID, "", item.TaskRef, err)
		return fmt.Errorf("get batch %s: %w", item.BatchID, err)
	}
	setBatchStatus(ctx, w.storage, w.logger, batch, storage.BatchStatusRunning)
	w.notifyTaskEvent(ctx, notify.Event{Type: "task_started", BatchID: item.BatchID, TaskRef: item.TaskRef, Status: "running"})

	runs, err := w.storage.ListTaskRuns(ctx, item.BatchID)
	if err != nil {
		w.logger.Warn("list task runs", zap.Error(err), zap.String("batch_id", item.BatchID))
	}

	isEpic, err := w.tracker.IsEpic(ctx, item.TaskRef)
	if err != nil {
		_ = w.notifier.NotifyError(ctx, item.BatchID, "", item.TaskRef, err)
		setBatchStatus(ctx, w.storage, w.logger, batch, storage.BatchStatusFailed)
		return fmt.Errorf("tracker is-epic: %w", err)
	}

	if !isEpic {
		return w.handleSingleTask(ctx, batch, item)
	}

	return w.handleEpic(ctx, batch, item, runs)
}

func (w *Worker) handleSingleTask(ctx context.Context, batch *storage.Batch, item QueueItem) error {
	retries, hasRetries := w.parseMaxRetries()
	attempts := 0
	for {
		attempts++
		w.notifyTaskEvent(ctx, notify.Event{Type: "attempt_started", BatchID: item.BatchID, TaskRef: item.TaskRef, Status: "in_progress", Attempt: attempts, Details: fmt.Sprintf("attempt=%d", attempts)})
		execInstruction := fmt.Sprintf("Work on task %s", item.TaskRef)
		verifyInstruction := fmt.Sprintf("Verify task %s", item.TaskRef)
		verifyStatus, agentResp, assistantOutput, output, err := w.executeAndVerify(ctx, batch, item.TaskRef, "", execInstruction, verifyInstruction)
		if err != nil {
			return err
		}

		if agentResp.Status == "failure" {
			if w.shouldRetry(ctx, batch, item.BatchID, item.TaskRef, agentResp, assistantOutput, attempts, retries, hasRetries) {
				continue
			}
		}

		if verifyStatus == storage.TaskRunStatusSucceeded {
			w.notifyTaskEvent(ctx, notify.Event{Type: "verification_succeeded", BatchID: item.BatchID, TaskRef: item.TaskRef, Status: "succeeded", Attempt: attempts, Details: fmt.Sprintf("attempt=%d", attempts)})
			setBatchStatus(ctx, w.storage, w.logger, batch, storage.BatchStatusIdle)
			return nil
		}

		return fmt.Errorf("verification task did not succeed: %s", output)
	}
}

func (w *Worker) handleEpic(ctx context.Context, batch *storage.Batch, item QueueItem, runs []storage.TaskRun) error {
	retries, hasRetries := w.parseMaxRetries()

	for {
		children, err := w.tracker.ListChildren(ctx, item.TaskRef)
		if err != nil {
			_ = w.notifier.NotifyError(ctx, item.BatchID, "", item.TaskRef, err)
			setBatchStatus(ctx, w.storage, w.logger, batch, storage.BatchStatusFailed)
			return fmt.Errorf("tracker list children: %w", err)
		}

		child, ok := firstAvailableChild(children, runs)
		if !ok {
			setBatchStatus(ctx, w.storage, w.logger, batch, storage.BatchStatusIdle)
			_ = w.notifier.NotifyBatchIdle(ctx, item.BatchID)
			w.logger.Info("no remaining child tasks for epic", zap.String("epic", item.TaskRef))
			return nil
		}

		attempts := 0
		runRecorded := false
		for {
			attempts++
			w.notifyTaskEvent(ctx, notify.Event{Type: "attempt_started", BatchID: item.BatchID, TaskRef: child, ParentTaskRef: item.TaskRef, Status: "in_progress", Attempt: attempts, Details: fmt.Sprintf("attempt=%d epic=%s", attempts, item.TaskRef)})
			execInstruction := fmt.Sprintf("Pick first ready task from epic %s and execute", item.TaskRef)
			verifyInstruction := fmt.Sprintf("Verify task %s", child)
			verifyStatus, agentResp, assistantOutput, output, err := w.executeAndVerify(ctx, batch, child, item.TaskRef, execInstruction, verifyInstruction)
			if err != nil {
				return err
			}

			if !runRecorded {
				runs = append(runs, storage.TaskRun{TaskRef: child})
				runRecorded = true
			}

			if agentResp.Status == "failure" {
				if w.shouldRetry(ctx, batch, item.BatchID, child, agentResp, assistantOutput, attempts, retries, hasRetries) {
					continue
				}
			}

			if verifyStatus == storage.TaskRunStatusSucceeded {
				w.notifyTaskEvent(ctx, notify.Event{Type: "verification_succeeded", BatchID: item.BatchID, TaskRef: child, ParentTaskRef: item.TaskRef, Status: "succeeded", Attempt: attempts, Details: fmt.Sprintf("attempt=%d epic=%s", attempts, item.TaskRef)})
				break
			}

			return fmt.Errorf("verification task did not succeed: %s", output)
		}
	}
}

func (w *Worker) executeAndVerify(ctx context.Context, batch *storage.Batch, taskRef, parentRef, execInstruction, verifyInstruction string) (storage.TaskRunStatus, AgentStructuredResponse, string, string, error) {
	execTask := NewExecutionTask(w.cfg, w.tracker, w.copilot, w.storage, w.notifier, w.logger, w.repoPath, execInstruction)
	execTask.newRunID = w.newRunID
	execTask.now = w.now

	status, assistantOutput, finalErr := execTask.Execute(ctx, batch, taskRef, parentRef)
	if status != storage.TaskRunStatusSucceeded {
		return status, AgentStructuredResponse{}, assistantOutput, "", finalErr
	}

	verifyTask := NewVerificationTask(w.cfg, execTask, verifyInstruction)
	verifyStatus, output, verifyErr := verifyTask.Execute(ctx, batch, taskRef, parentRef)
	if verifyErr != nil {
		w.logger.Warn("verification task execution error", zap.Error(verifyErr), zap.String("task_ref", taskRef))
		return verifyStatus, AgentStructuredResponse{}, assistantOutput, output, verifyErr
	}

	var agentResp AgentStructuredResponse
	if err := json.Unmarshal([]byte(output), &agentResp); err != nil {
		w.logger.Warn("unmarshal agent response", zap.Error(err), zap.String("output", output))
	} else {
		w.logger.Info("agent structured response", zap.String("status", agentResp.Status), zap.String("reason", agentResp.Reason), zap.String("details", agentResp.Details))
	}

	return verifyStatus, agentResp, assistantOutput, output, nil
}

func (w *Worker) shouldRetry(ctx context.Context, batch *storage.Batch, batchID, taskRef string, agentResp AgentStructuredResponse, assistantOutput string, attempts, retries int, hasRetries bool) bool {
	w.logger.Info("agent indicated task failure", zap.String("task_ref", taskRef), zap.String("reason", agentResp.Reason), zap.String("details", agentResp.Details))
	if err := w.tracker.AddComment(ctx, taskRef, fmt.Sprintf("Verification failed: %s\nDetails: %s\nAssistant output: %s", agentResp.Reason, agentResp.Details, assistantOutput)); err != nil {
		w.logger.Warn("add tracker comment for failed verification", zap.Error(err), zap.String("task_ref", taskRef))
	}

	info := fmt.Sprintf("attempt=%d reason=%s details=%s", attempts, agentResp.Reason, agentResp.Details)
	w.notifyTaskEvent(ctx, notify.Event{Type: "verification_failed", BatchID: batchID, TaskRef: taskRef, Status: "failed", Attempt: attempts, MaxAttempts: retries, Details: info})

	if !hasRetries {
		return false
	}

	if attempts >= retries {
		w.logger.Info("max verification attempts reached; marking batch as failed", zap.String("batch_id", batchID), zap.Int("attempts", attempts), zap.Int("max_retries", retries))
		setBatchStatus(ctx, w.storage, w.logger, batch, storage.BatchStatusFailed)
		w.notifyTaskEvent(ctx, notify.Event{Type: "verification_failed_max_retries", BatchID: batchID, TaskRef: taskRef, Status: "failed", Attempt: attempts, MaxAttempts: retries, Details: info})
		_ = w.notifier.NotifyError(ctx, batchID, "", taskRef, fmt.Errorf("verification failed after %d attempts: %s", attempts, agentResp.Reason))
		return false
	}

	w.notifyTaskEvent(ctx, notify.Event{Type: "verification_failed_retrying", BatchID: batchID, TaskRef: taskRef, Status: "retrying", Attempt: attempts, MaxAttempts: retries, Details: fmt.Sprintf("%s retry_in=%d", info, retries-attempts)})
	return true
}

func (w *Worker) notifyTaskEvent(ctx context.Context, event notify.Event) {
	if w.notifier == nil {
		return
	}

	if event.Type == "" {
		event.Type = "task_event"
	}

	if err := w.notifier.NotifyEvent(ctx, event); err != nil {
		w.logger.Warn("notify task event", zap.Error(err), zap.String("event_type", event.Type), zap.String("batch_id", event.BatchID), zap.String("task_ref", event.TaskRef))
	}
}

func (w *Worker) parseMaxRetries() (int, bool) {
	value, err := w.cfg.Get(config.MaxRetriesKey)
	if err != nil {
		return 0, false
	}

	maxRetries, err := strconv.Atoi(value)
	if err != nil {
		w.logger.Warn("invalid max retries config value; must be an integer", zap.Error(err), zap.String("value", value))
		maxRetries = 3
	}

	return maxRetries, true
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

func buildPrompt(basePrompt, fallback string) string {
	basePrompt = strings.TrimSpace(basePrompt)
	fallback = strings.TrimSpace(fallback)

	switch {
	case basePrompt != "" && fallback != "":
		return fmt.Sprintf("%s\n\n%s", basePrompt, fallback)
	case basePrompt != "":
		return basePrompt
	default:
		return fallback
	}
}
