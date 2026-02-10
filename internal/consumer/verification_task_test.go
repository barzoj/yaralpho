package consumer

import (
	"context"
	"testing"
	"time"

	"github.com/barzoj/yaralpho/internal/config"
	"github.com/barzoj/yaralpho/internal/copilot"
	"github.com/barzoj/yaralpho/internal/notify"
	"github.com/barzoj/yaralpho/internal/storage"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestVerificationTaskDelegatesToStructuredExecution(t *testing.T) {
	cfg := stubConfig{
		config.ExecutionTaskPromptKey:    "exec base",
		config.VerificationTaskPromptKey: "verify base",
	}
	execTask := NewExecutionTask(cfg, &stubExecutionTracker{}, nil, nil, notify.Noop{}, zap.NewNop(), " /repo ", "exec instruction")

	var (
		capturedPrompt string
		runRef         string
		epicRef        string
	)

	task := NewVerificationTask(cfg, execTask, " verify prompt ")
	task.exec = func(ctx context.Context, cp copilot.Client, st storage.Storage, nt notify.Notifier, logger *zap.Logger, repoPath string, newRunID func() string, now func() time.Time, batch *storage.Batch, run, epic, prompt string) (storage.TaskRunStatus, string, error) {
		capturedPrompt = prompt
		runRef = run
		epicRef = epic

		require.Equal(t, "/repo", repoPath)
		require.NotNil(t, newRunID)
		require.NotNil(t, now)
		require.Equal(t, "batch-verify", batch.ID)
		require.Equal(t, notify.Noop{}, nt)

		return storage.TaskRunStatusSucceeded, "structured response", nil
	}

	status, resp, err := task.Execute(context.Background(), &storage.Batch{ID: "batch-verify"}, "task-verify", "epic-verify")
	require.NoError(t, err)
	require.Equal(t, storage.TaskRunStatusSucceeded, status)
	require.Equal(t, "structured response", resp)
	require.Equal(t, "verify base\n\nverify prompt", capturedPrompt)
	require.Equal(t, "task-verify", runRef)
	require.Equal(t, "epic-verify", epicRef)
}

func TestVerificationTaskRequiresExecutionReference(t *testing.T) {
	cfg := stubConfig{config.VerificationTaskPromptKey: "prompt"}
	task := NewVerificationTask(cfg, nil, "prompt")

	status, resp, err := task.Execute(context.Background(), &storage.Batch{ID: "batch"}, "task", "")
	require.Error(t, err)
	require.Equal(t, storage.TaskRunStatusFailed, status)
	require.Empty(t, resp)
}
