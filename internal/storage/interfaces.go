package storage

import "context"

// Storage defines persistence operations for batches, task runs, and session
// events. Implementations must remain agnostic to any specific database
// driver types and honor the provided context for cancellation and timeouts.
type Storage interface {
	CreateBatch(ctx context.Context, batch *Batch) error
	UpdateBatch(ctx context.Context, batch *Batch) error
	GetBatch(ctx context.Context, batchID string) (*Batch, error)
	ListBatches(ctx context.Context, limit int64) ([]Batch, error)

	CreateTaskRun(ctx context.Context, run *TaskRun) error
	UpdateTaskRun(ctx context.Context, run *TaskRun) error
	GetTaskRun(ctx context.Context, runID string) (*TaskRun, error)
	// ListTaskRuns returns runs for a batch. If batchID is empty, implementations
	// may return runs across all batches.
	ListTaskRuns(ctx context.Context, batchID string) ([]TaskRun, error)

	InsertSessionEvent(ctx context.Context, event *SessionEvent) error
	ListSessionEvents(ctx context.Context, sessionID string) ([]SessionEvent, error)

	GetBatchProgress(ctx context.Context, batchID string) (BatchProgress, error)
}
