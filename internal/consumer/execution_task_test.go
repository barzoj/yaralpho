package consumer

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/barzoj/yaralpho/internal/copilot"
	"github.com/barzoj/yaralpho/internal/notify"
	"github.com/barzoj/yaralpho/internal/storage"
	"github.com/barzoj/yaralpho/internal/tracker"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestExecutionTaskBuildsPromptWithComments(t *testing.T) {
	tr := &stubExecutionTracker{
		comments: []tracker.Comment{
			{Author: "Alice", Text: "hello"},
			{Author: "Bob", Text: "second"},
		},
	}

	var capturedPrompt string
	execCalls := 0

	task := NewExecutionTask(tr, nil, nil, notify.Noop{}, zap.NewNop(), "/repo", "base prompt")
	task.exec = func(ctx context.Context, cp copilot.Client, st storage.Storage, nt notify.Notifier, logger *zap.Logger, repoPath string, newRunID func() string, now func() time.Time, batch *storage.Batch, runRef, epicRef, prompt string) (storage.TaskRunStatus, error) {
		execCalls++
		capturedPrompt = prompt
		require.Equal(t, "/repo", repoPath)
		require.Equal(t, "task-1", runRef)
		require.Equal(t, "epic-1", epicRef)
		require.NotNil(t, newRunID)
		require.NotNil(t, now)
		return storage.TaskRunStatusSucceeded, nil
	}

	status, err := task.Execute(context.Background(), &storage.Batch{ID: "b1"}, "task-1", "epic-1")
	require.NoError(t, err)
	require.Equal(t, storage.TaskRunStatusSucceeded, status)
	require.Equal(t, 1, execCalls)

	require.Equal(t, []string{"task-1"}, tr.refs)
	require.Equal(t, "base prompt\n\nTracker comments:\n- Alice: hello\n- Bob: second", capturedPrompt)
}

func TestExecutionTaskWithoutCommentsUsesBasePrompt(t *testing.T) {
	tr := &stubExecutionTracker{}
	task := NewExecutionTask(tr, nil, nil, notify.Noop{}, zap.NewNop(), " /repo ", "solo prompt")

	var prompt string
	task.exec = func(ctx context.Context, cp copilot.Client, st storage.Storage, nt notify.Notifier, logger *zap.Logger, repoPath string, newRunID func() string, now func() time.Time, batch *storage.Batch, runRef, epicRef, p string) (storage.TaskRunStatus, error) {
		prompt = p
		require.Equal(t, "/repo", repoPath)
		return storage.TaskRunStatusSucceeded, nil
	}

	status, err := task.Execute(context.Background(), &storage.Batch{ID: "b2"}, "task-2", "")
	require.NoError(t, err)
	require.Equal(t, storage.TaskRunStatusSucceeded, status)
	require.Equal(t, "solo prompt", prompt)
	require.Equal(t, []string{"task-2"}, tr.refs)
}

func TestExecutionTaskPropagatesFetchError(t *testing.T) {
	tr := &stubExecutionTracker{err: errors.New("boom")}
	task := NewExecutionTask(tr, nil, nil, notify.Noop{}, zap.NewNop(), "/repo", "base")

	called := false
	task.exec = func(ctx context.Context, cp copilot.Client, st storage.Storage, nt notify.Notifier, logger *zap.Logger, repoPath string, newRunID func() string, now func() time.Time, batch *storage.Batch, runRef, epicRef, prompt string) (storage.TaskRunStatus, error) {
		called = true
		return storage.TaskRunStatusSucceeded, nil
	}

	status, err := task.Execute(context.Background(), &storage.Batch{ID: "b3"}, "task-3", "")
	require.Error(t, err)
	require.ErrorContains(t, err, "fetch tracker comments")
	require.Equal(t, storage.TaskRunStatusFailed, status)
	require.False(t, called)
	require.Equal(t, []string{"task-3"}, tr.refs)
}

type stubExecutionTracker struct {
	comments []tracker.Comment
	err      error
	refs     []string
}

func (s *stubExecutionTracker) IsEpic(ctx context.Context, ref string) (bool, error) {
	return false, nil
}

func (s *stubExecutionTracker) ListChildren(ctx context.Context, ref string) ([]string, error) {
	return nil, nil
}

func (s *stubExecutionTracker) AddComment(ctx context.Context, ref string, text string) error {
	return nil
}

func (s *stubExecutionTracker) FetchComments(ctx context.Context, ref string) ([]tracker.Comment, error) {
	s.refs = append(s.refs, ref)
	if s.err != nil {
		return nil, s.err
	}
	return s.comments, nil
}
