package notify

import "context"

// Event captures a notifier-friendly description of lifecycle activity so that
// implementations can render rich messages without losing context.
type Event struct {
	Type          string // machine-friendly event type, e.g. task_received, attempt_started
	BatchID       string
	RunID         string
	TaskRef       string
	ParentTaskRef string
	Status        string
	Details       string
	Attempt       int
	MaxAttempts   int
	CommitHash    string
}

// Notifier reports lifecycle events (task completion, idle batches, errors)
// to an external channel such as Slack. Implementations should be resilient
// to transient failures and respect context cancellation.
type Notifier interface {
	NotifyEvent(ctx context.Context, event Event) error
	NotifyTaskFinished(ctx context.Context, batchID, runID, taskRef, status, commitHash string) error
	NotifyBatchIdle(ctx context.Context, batchID string) error
	NotifyError(ctx context.Context, batchID, runID, taskRef string, err error) error
}

// Noop implements Notifier but discards all notifications. It is used when no
// webhook has been configured; all methods return nil without side effects.
type Noop struct{}

func (Noop) NotifyEvent(ctx context.Context, event Event) error {
	return nil
}

func (Noop) NotifyTaskFinished(ctx context.Context, batchID, runID, taskRef, status, commitHash string) error {
	return nil
}

func (Noop) NotifyBatchIdle(ctx context.Context, batchID string) error {
	return nil
}

func (Noop) NotifyError(ctx context.Context, batchID, runID, taskRef string, err error) error {
	return nil
}
