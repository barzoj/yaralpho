package tracker

import (
	"context"
	"time"
)

// Comment represents a tracker comment with basic metadata.
type Comment struct {
	ID        string    `json:"id"`
	Author    string    `json:"author"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

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

	// AddComment adds a tracker comment to the given reference.
	AddComment(ctx context.Context, ref string, text string) error

	// FetchComments returns tracker comments for the given reference, ordered
	// as provided by the backend. Implementations should return an empty
	// slice when no comments exist.
	FetchComments(ctx context.Context, ref string) ([]Comment, error)
}
