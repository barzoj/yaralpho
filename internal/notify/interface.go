package notify

import "context"

// Notifier reports lifecycle events (task completion, idle batches, errors)
// to an external channel such as Slack. Implementations should be resilient
// to transient failures and respect context cancellation.
type Notifier interface {
	NotifyTaskFinished(ctx context.Context, batchID, runID, taskRef, status, commitHash string) error
	NotifyBatchIdle(ctx context.Context, batchID string) error
	NotifyError(ctx context.Context, batchID, runID, taskRef string, err error) error
}

// Noop implements Notifier but discards all notifications. It is used when no
// webhook has been configured; all methods return nil without side effects.
type Noop struct{}

func (Noop) NotifyTaskFinished(ctx context.Context, batchID, runID, taskRef, status, commitHash string) error {
	return nil
}

func (Noop) NotifyBatchIdle(ctx context.Context, batchID string) error {
	return nil
}

func (Noop) NotifyError(ctx context.Context, batchID, runID, taskRef string, err error) error {
	return nil
}
