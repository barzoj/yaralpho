package scheduler

import "context"

// Controller exposes the public scheduler contract used by HTTP handlers and
// service wiring. Implementations must be safe for single-process use; callers
// should ensure only one scheduler instance runs at a time.
type Controller interface {
	// Start begins periodic ticking until stopped or the context is canceled.
	Start(ctx context.Context) error
	// Stop halts periodic ticking and enables draining to prevent new work.
	Stop(ctx context.Context) error
	// Tick performs a single scheduling cycle. Callers must not drive Tick
	// concurrently from multiple processes; draining must short-circuit new work.
	Tick(ctx context.Context) error
	// SetDraining toggles draining mode, preventing new work when true.
	SetDraining(draining bool)
	// Draining reports whether draining mode is enabled.
	Draining() bool
	// ActiveCount returns the number of in-flight work items.
	ActiveCount() int
	// WaitForIdle blocks until all active work completes or the context is canceled.
	WaitForIdle(ctx context.Context) error
}

var _ Controller = (*Scheduler)(nil)
