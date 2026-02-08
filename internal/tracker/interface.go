package tracker

import "context"

// Tracker exposes read-only operations to reason about work items in an
// external tracker. Implementations should avoid leaking backend-specific
// types and must respect context cancellation for remote calls.
type Tracker interface {
	// IsEpic returns true when the provided reference points to an epic or
	// parent issue. It should only return an error for transport or parsing
	// failures, not when the reference simply is not an epic.
	IsEpic(ctx context.Context, ref string) (bool, error)

	// ListChildren returns ordered child task references for the given epic.
	// The slice should preserve the tracker-defined ordering (e.g., as shown
	// by `bd show`) and be empty when no children are present.
	ListChildren(ctx context.Context, ref string) ([]string, error)
}
