package consumer

import (
	"context"

	"github.com/barzoj/yaralpho/internal/storage"
)

// ExecutableTask represents a runnable task within the consumer workflow.
type ExecutableTask interface {
	Execute(ctx context.Context, batch *storage.Batch, taskID string) (storage.TaskRunStatus, string, error)
}
