package consumer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/barzoj/yaralpho/internal/copilot"
	"github.com/barzoj/yaralpho/internal/notify"
	"github.com/barzoj/yaralpho/internal/storage"
	"go.uber.org/zap"
)

// VerificationTask implements ExecutableTask by delegating to
// executeTaskWithStructuredOutput using the configured verification prompt.
type VerificationTask struct {
	executionTask *ExecutionTask
	basePrompt    string
	exec          func(ctx context.Context, cp copilot.Client, st storage.Storage, nt notify.Notifier, logger *zap.Logger, repoPath string, newRunID func() string, now func() time.Time, batch *storage.Batch, runRef, epicRef, prompt string) (storage.TaskRunStatus, string, error)
}

// NewVerificationTask constructs a VerificationTask bound to an ExecutionTask
// reference and a verification prompt.
func NewVerificationTask(executionTask *ExecutionTask, basePrompt string) *VerificationTask {
	return &VerificationTask{
		executionTask: executionTask,
		basePrompt:    basePrompt,
		exec:          executeTaskWithStructuredOutput,
	}
}

// Execute delegates to executeTaskWithStructuredOutput with the verification
// prompt and the execution task's dependencies.
func (t *VerificationTask) Execute(ctx context.Context, batch *storage.Batch, taskID, epicID string) (storage.TaskRunStatus, string, error) {
	if t.executionTask == nil {
		return storage.TaskRunStatusFailed, "", fmt.Errorf("execution task reference is nil")
	}
	if t.exec == nil {
		t.exec = executeTaskWithStructuredOutput
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

	prompt := strings.TrimSpace(t.basePrompt)

	return t.exec(ctx, t.executionTask.copilot, t.executionTask.storage, notifier, logger, repoPath, newRunID, now, batch, taskID, epicID, prompt)
}
