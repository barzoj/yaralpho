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

var executeTaskFunc = executeTaskWithAssistantMessages

// ExecutionTask implements ExecutableTask by assembling prompts and delegating to executeTask.
type ExecutionTask struct {
	cfg         config.Config
	tracker     tracker.Tracker
	copilot     copilot.Client
	storage     storage.Storage
	notifier    notify.Notifier
	logger      *zap.Logger
	repoPath    string
	instruction string
	newRunID    func() string
	now         func() time.Time
	exec        func(ctx context.Context, cp copilot.Client, st storage.Storage, nt notify.Notifier, logger *zap.Logger, repoPath string, newRunID func() string, now func() time.Time, batch *storage.Batch, runRef, epicRef, prompt string) (storage.TaskRunStatus, string, error)
}

// NewExecutionTask constructs an ExecutionTask with sensible defaults for logger,
// run ID generation, and clock.
func NewExecutionTask(cfg config.Config, tr tracker.Tracker, cp copilot.Client, st storage.Storage, nt notify.Notifier, logger *zap.Logger, repoPath string, instruction string) *ExecutionTask {
	if logger == nil {
		logger = zap.NewNop()
	}
	if nt == nil {
		nt = notify.Noop{}
	}

	return &ExecutionTask{
		cfg:         cfg,
		tracker:     tr,
		copilot:     cp,
		storage:     st,
		notifier:    nt,
		logger:      logger,
		repoPath:    strings.TrimSpace(repoPath),
		instruction: strings.TrimSpace(instruction),
		newRunID: func() string {
			return fmt.Sprintf("run-%d", time.Now().UTC().UnixNano())
		},
		now: func() time.Time {
			return time.Now().UTC()
		},
		exec: executeTaskFunc,
	}
}

// Execute fetches tracker comments, builds a comment-aware prompt, and delegates
// execution to executeTask.
func (t *ExecutionTask) Execute(ctx context.Context, batch *storage.Batch, taskID, epicID string) (storage.TaskRunStatus, string, error) {
	if t.cfg == nil {
		return storage.TaskRunStatusFailed, "", fmt.Errorf("config is nil")
	}

	basePrompt, err := t.cfg.Get(config.ExecutionTaskPromptKey)
	if err != nil {
		return storage.TaskRunStatusFailed, "", fmt.Errorf("get execution task prompt: %w", err)
	}

	// add taskID to the prompt if it's referenced
	if strings.Contains(basePrompt, "%s") {
		basePrompt = fmt.Sprintf(basePrompt, taskID)
	}

	comments, err := t.tracker.FetchComments(ctx, taskID)
	if err != nil {
		return storage.TaskRunStatusFailed, "", fmt.Errorf("fetch tracker comments: %w", err)
	}

	prompt := buildPrompt(basePrompt, t.instruction)
	if len(comments) > 0 {
		var b strings.Builder
		if prompt != "" {
			b.WriteString(prompt)
		}
		b.WriteString("\n\nTracker comments:\n")
		for _, c := range comments {
			b.WriteString("- ")
			if c.Author != "" {
				b.WriteString(c.Author)
				b.WriteString(": ")
			}
			b.WriteString(c.Text)
			b.WriteByte('\n')
		}
		prompt = strings.TrimSpace(b.String())
	}

	if t.exec == nil {
		t.exec = executeTaskFunc
	}
	if t.newRunID == nil {
		t.newRunID = func() string { return fmt.Sprintf("run-%d", time.Now().UTC().UnixNano()) }
	}
	if t.now == nil {
		t.now = func() time.Time { return time.Now().UTC() }
	}

	status, messages, err := t.exec(ctx, t.copilot, t.storage, t.notifier, t.logger, t.repoPath, t.newRunID, t.now, batch, taskID, epicID, prompt)
	return status, messages, err
}
