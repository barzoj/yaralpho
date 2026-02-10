package consumer

import (
	"context"
	"testing"

	"github.com/barzoj/yaralpho/internal/storage"
	"github.com/stretchr/testify/require"
)

func TestExecutableTaskInterfaceSignature(t *testing.T) {
	var task ExecutableTask = &fakeExecutableTask{}

	status, err := task.Execute(context.Background(), &storage.Batch{ID: "batch-1"}, "task-1", "epic-1")
	require.NoError(t, err)
	require.Equal(t, storage.TaskRunStatusSucceeded, status)
}

type fakeExecutableTask struct{}

func (f *fakeExecutableTask) Execute(ctx context.Context, batch *storage.Batch, taskID, epicID string) (storage.TaskRunStatus, error) {
	_ = ctx
	_ = batch
	_ = taskID
	_ = epicID
	return storage.TaskRunStatusSucceeded, nil
}
