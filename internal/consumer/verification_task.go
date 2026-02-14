package consumer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/barzoj/yaralpho/internal/config"
	"github.com/barzoj/yaralpho/internal/copilot"
	"github.com/barzoj/yaralpho/internal/notify"
	"github.com/barzoj/yaralpho/internal/storage"
	"github.com/barzoj/yaralpho/internal/tracker"
	"go.uber.org/zap"
)

// VerificationTask implements ExecutableTask by delegating to
// executeTaskWithStructuredOutput using the configured verification prompt.
type VerificationTask struct {
	cfg           config.Config
	executionTask *ExecutionTask
	instruction   string
	exec          func(ctx context.Context, cp copilot.Client, st storage.Storage, tr tracker.Tracker, nt notify.Notifier, logger *zap.Logger, repoPath string, newRunID func() string, now func() time.Time, batch *storage.Batch, runRef, parentRef, prompt string) (storage.TaskRunStatus, string, error)
}

// NewVerificationTask constructs a VerificationTask bound to an ExecutionTask
// reference and a verification prompt.
func NewVerificationTask(cfg config.Config, executionTask *ExecutionTask, instruction string) *VerificationTask {
	return &VerificationTask{
		cfg:           cfg,
		executionTask: executionTask,
		instruction:   strings.TrimSpace(instruction),
		exec:          executeTaskWithStructuredOutput,
	}
}

// Execute delegates to executeTaskWithStructuredOutput with the verification
// prompt and the execution task's dependencies.
func (t *VerificationTask) Execute(ctx context.Context, batch *storage.Batch, taskID, parentID string) (storage.TaskRunStatus, string, error) {
	if t.executionTask == nil {
		return storage.TaskRunStatusFailed, "", fmt.Errorf("execution task reference is nil")
	}
	if t.exec == nil {
		t.exec = executeTaskWithStructuredOutput
	}
	if t.cfg == nil {
		return storage.TaskRunStatusFailed, "", fmt.Errorf("config is nil")
	}

	basePrompt, err := t.cfg.Get(config.VerificationTaskPromptKey)
	if err != nil {
		return storage.TaskRunStatusFailed, "", fmt.Errorf("get verification task prompt: %w", err)
	}

	repoPath := strings.TrimSpace(t.executionTask.repoPath)
	logger := t.executionTask.logger
	if logger == nil {
		logger = zap.NewNop()
	}
	notifier := t.executionTask.notifier
	if notifier == nil {
		notifier = notify.Noop{}
	}
	newRunID := t.executionTask.newRunID
	if newRunID == nil {
		newRunID = func() string { return fmt.Sprintf("run-%d", time.Now().UTC().UnixNano()) }
	}
	now := t.executionTask.now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}

	prompt := buildPrompt(basePrompt, t.instruction)
	if strings.Contains(prompt, "%s") {
		prompt = fmt.Sprintf(prompt, taskID)
	}

	return t.exec(ctx, t.executionTask.copilot, t.executionTask.storage, t.executionTask.tracker, notifier, logger, repoPath, newRunID, now, batch, taskID, parentID, prompt)
}
