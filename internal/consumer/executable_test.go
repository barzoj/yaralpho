package consumer

import (
	"context"
	"testing"

	"github.com/barzoj/yaralpho/internal/storage"
	"github.com/stretchr/testify/require"
)

func TestExecutableTaskInterfaceSignature(t *testing.T) {
	var task ExecutableTask = &fakeExecutableTask{}

	status, resp, err := task.Execute(context.Background(), &storage.Batch{ID: "batch-1"}, "task-1")
	require.NoError(t, err)
	require.Equal(t, storage.TaskRunStatusSucceeded, status)
	require.Empty(t, resp)
}

type fakeExecutableTask struct{}

func (f *fakeExecutableTask) Execute(ctx context.Context, batch *storage.Batch, taskID string) (storage.TaskRunStatus, string, error) {
	_ = ctx
	_ = batch
	_ = taskID
	return storage.TaskRunStatusSucceeded, "", nil
}
