package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

	batch, err := w.storage.GetBatch(ctx, item.BatchID)
	if err != nil {
		_ = w.notifier.NotifyError(ctx, item.BatchID, "", item.TaskRef, err)
		return fmt.Errorf("get batch %s: %w", item.BatchID, err)
	}
	setBatchStatus(ctx, w.storage, w.logger, batch, storage.BatchStatusRunning)

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
		execInstruction := fmt.Sprintf("Work on task %s", item.TaskRef)
		execTask := NewExecutionTask(w.cfg, w.tracker, w.copilot, w.storage, w.notifier, w.logger, w.repoPath, execInstruction)
		execTask.newRunID = w.newRunID
		execTask.now = w.now

		status, _, finalErr := execTask.Execute(ctx, batch, item.TaskRef, "")
		if status != storage.TaskRunStatusSucceeded {
			return finalErr
		}

		verifyInstruction := fmt.Sprintf("Verify task %s", item.TaskRef)
		verifyTask := NewVerificationTask(w.cfg, execTask, verifyInstruction)
		verifyStatus, _, verifyErr := verifyTask.Execute(ctx, batch, item.TaskRef, "")
		if verifyStatus == storage.TaskRunStatusSucceeded {
			setBatchStatus(ctx, w.storage, w.logger, batch, storage.BatchStatusIdle)
			return nil
		}

		return verifyErr
	}

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

		execInstruction := fmt.Sprintf("Pick first ready task from epic %s and execute", item.TaskRef)
		execTask := NewExecutionTask(w.cfg, w.tracker, w.copilot, w.storage, w.notifier, w.logger, w.repoPath, execInstruction)
		execTask.newRunID = w.newRunID
		execTask.now = w.now

		status, _, finalErr := execTask.Execute(ctx, batch, child, item.TaskRef)
		runs = append(runs, storage.TaskRun{TaskRef: child})

		if status != storage.TaskRunStatusSucceeded {
			return finalErr
		}

		verifyInstruction := fmt.Sprintf("Verify task %s", child)
		verifyTask := NewVerificationTask(w.cfg, execTask, verifyInstruction)
		verifyStatus, _, verifyErr := verifyTask.Execute(ctx, batch, child, item.TaskRef)
		if verifyStatus != storage.TaskRunStatusSucceeded {
			return verifyErr
		}
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
