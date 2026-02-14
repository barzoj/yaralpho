package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/barzoj/yaralpho/internal/bus"
	"github.com/barzoj/yaralpho/internal/config"
	"github.com/barzoj/yaralpho/internal/copilot"
	"github.com/barzoj/yaralpho/internal/notify"
	"github.com/barzoj/yaralpho/internal/storage"
	"github.com/barzoj/yaralpho/internal/tracker"
	"go.uber.org/zap"
)

// WorkItem represents a single unit of work selected by the scheduler and
// executed directly without an intermediary buffer.
type WorkItem struct {
	BatchID string `json:"batch_id"`
	TaskRef string `json:"task_ref"`
}

// Worker executes scheduler-selected work items and orchestrates task runs via
// Copilot while persisting progress and emitting notifications.
type Worker struct {
	cfg      config.Config
	tracker  tracker.Tracker
	copilot  copilot.Client
	storage  storage.Storage
	notifier notify.Notifier
	repoPath string
	logger   *zap.Logger
	now      func() time.Time
	newRunID func() string
	bus      bus.Bus
	titleMap map[string]string
}

// NewWorker constructs a Worker with sensible defaults for logger, notifier,
// clock, and run ID generation.
func NewWorker(tr tracker.Tracker, cp copilot.Client, st storage.Storage, nt notify.Notifier, cfg config.Config, repoPath string, logger *zap.Logger) *Worker {
	if logger == nil {
		logger = zap.NewNop()
	}
	if nt == nil {
		nt = notify.Noop{}
	}
	if sessionEventBus == nil {
		setSessionEventBus(bus.NewMemoryBus(bus.Config{Logger: logger}))
	}

	return &Worker{
		cfg:      cfg,
		tracker:  tr,
		copilot:  cp,
		storage:  st,
		notifier: nt,
		bus:      sessionEventBus,
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

// Process executes a single work item selected by the scheduler using the
// direct execution path.
func (w *Worker) Process(ctx context.Context, item WorkItem) error {
	item.BatchID = strings.TrimSpace(item.BatchID)
	item.TaskRef = strings.TrimSpace(item.TaskRef)
	if item.BatchID == "" || item.TaskRef == "" {
		return fmt.Errorf("work item missing batch_id or task_ref")
	}

	taskName := w.taskTitle(ctx, item.TaskRef)
	w.notifyTaskEvent(ctx, notify.Event{Type: "task_received", BatchID: item.BatchID, TaskRef: item.TaskRef, TaskName: taskName, Status: "pending"})

	batch, err := w.storage.GetBatch(ctx, item.BatchID)
	if err != nil {
		_ = w.notifier.NotifyError(ctx, item.BatchID, "", item.TaskRef, err)
		return fmt.Errorf("get batch %s: %w", item.BatchID, err)
	}
	setBatchStatus(ctx, w.storage, w.logger, batch, storage.BatchStatusInProgress)
	w.notifyTaskEvent(ctx, notify.Event{Type: "task_started", BatchID: item.BatchID, TaskRef: item.TaskRef, TaskName: taskName, Status: "running"})

	return w.handleSingleTask(ctx, batch, item)
}

func (w *Worker) handleSingleTask(ctx context.Context, batch *storage.Batch, item WorkItem) error {
	retries, hasRetries := w.parseMaxRetries()
	attempts := 0
	for {
		attempts++
		w.notifyTaskEvent(ctx, notify.Event{Type: "attempt_started", BatchID: item.BatchID, TaskRef: item.TaskRef, TaskName: w.taskTitle(ctx, item.TaskRef), Status: "in_progress", Attempt: attempts})
		execInstruction := fmt.Sprintf("Work on task %s", item.TaskRef)
		verifyInstruction := fmt.Sprintf("Verify task %s", item.TaskRef)
		verifyStatus, agentResp, assistantOutput, output, err := w.executeAndVerify(ctx, batch, item.TaskRef, execInstruction, verifyInstruction)
		if err != nil {
			return err
		}

		if agentResp.Status == "failure" {
			if w.shouldRetry(ctx, batch, item.BatchID, item.TaskRef, agentResp, assistantOutput, attempts, retries, hasRetries) {
				continue
			}
		}

		if verifyStatus == storage.TaskRunStatusSucceeded {
			w.notifyTaskEvent(ctx, notify.Event{Type: "verification_succeeded", BatchID: item.BatchID, TaskRef: item.TaskRef, TaskName: w.taskTitle(ctx, item.TaskRef), Status: "succeeded", Attempt: attempts, Details: formatAgentResponseText(agentResp, output)})
			setBatchStatus(ctx, w.storage, w.logger, batch, storage.BatchStatusPending)
			return nil
		}

		return fmt.Errorf("verification task did not succeed: %s", output)
	}
}

func (w *Worker) executeAndVerify(ctx context.Context, batch *storage.Batch, taskRef, execInstruction, verifyInstruction string) (storage.TaskRunStatus, AgentStructuredResponse, string, string, error) {
	execTask := NewExecutionTask(w.cfg, w.tracker, w.copilot, w.storage, w.notifier, w.logger, w.repoPath, execInstruction)
	execTask.newRunID = w.newRunID
	execTask.now = w.now

	status, assistantOutput, finalErr := execTask.Execute(ctx, batch, taskRef)
	if status != storage.TaskRunStatusSucceeded {
		return status, AgentStructuredResponse{}, assistantOutput, "", finalErr
	}

	verifyTask := NewVerificationTask(w.cfg, execTask, verifyInstruction)
	verifyStatus, output, verifyErr := verifyTask.Execute(ctx, batch, taskRef)
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

	baseDetails := formatAgentResponseText(agentResp, "")
	w.notifyTaskEvent(ctx, notify.Event{Type: "verification_failed", BatchID: batchID, TaskRef: taskRef, TaskName: w.taskTitle(ctx, taskRef), Status: "failed", Attempt: attempts, MaxAttempts: retries, Details: baseDetails})

	if !hasRetries {
		return false
	}

	if attempts >= retries {
		w.logger.Info("max verification attempts reached; marking batch as failed", zap.String("batch_id", batchID), zap.Int("attempts", attempts), zap.Int("max_retries", retries))
		setBatchStatus(ctx, w.storage, w.logger, batch, storage.BatchStatusFailed)
		details := appendDetailLine(baseDetails, "Next: no retries remaining")
		w.notifyTaskEvent(ctx, notify.Event{Type: "verification_failed_max_retries", BatchID: batchID, TaskRef: taskRef, TaskName: w.taskTitle(ctx, taskRef), Status: "failed", Attempt: attempts, MaxAttempts: retries, Details: details})
		_ = w.notifier.NotifyError(ctx, batchID, "", taskRef, fmt.Errorf("verification failed after %d attempts: %s", attempts, agentResp.Reason))
		return false
	}

	retryDetails := appendDetailLine(baseDetails, fmt.Sprintf("Next: retrying (%d attempts remaining)", retries-attempts))
	w.notifyTaskEvent(ctx, notify.Event{Type: "verification_failed_retrying", BatchID: batchID, TaskRef: taskRef, TaskName: w.taskTitle(ctx, taskRef), Status: "retrying", Attempt: attempts, MaxAttempts: retries, Details: retryDetails})
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

func (w *Worker) taskTitle(ctx context.Context, taskRef string) string {
	taskRef = strings.TrimSpace(taskRef)
	if taskRef == "" || w.tracker == nil {
		return ""
	}
	if w.titleMap != nil {
		if title, ok := w.titleMap[taskRef]; ok {
			return title
		}
	}

	title, err := w.tracker.GetTitle(ctx, taskRef)
	if err != nil {
		w.logger.Warn("fetch task title", zap.Error(err), zap.String("task_ref", taskRef))
		return ""
	}
	title = strings.TrimSpace(title)
	if w.titleMap == nil {
		w.titleMap = make(map[string]string)
	}
	w.titleMap[taskRef] = title
	return title
}

func formatAgentResponseText(resp AgentStructuredResponse, rawOutput string) string {
	lines := []string{}
	reason := strings.TrimSpace(resp.Reason)
	if reason != "" {
		lines = append(lines, fmt.Sprintf("Reason: %s", reason))
	}
	details := strings.TrimSpace(resp.Details)
	if details != "" {
		lines = append(lines, fmt.Sprintf("Details: %s", details))
	}
	if len(lines) > 0 {
		return strings.Join(lines, "\n")
	}
	if raw := strings.TrimSpace(rawOutput); raw != "" {
		return fmt.Sprintf("Response: %s", raw)
	}
	return "Agent response unavailable."
}

func appendDetailLine(base, line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return base
	}
	if strings.TrimSpace(base) == "" {
		return line
	}
	return fmt.Sprintf("%s\n%s", base, line)
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
