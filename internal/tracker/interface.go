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
	// AddComment adds a tracker comment to the given reference.
	AddComment(ctx context.Context, repoPath string, ref string, text string) error

	// FetchComments returns tracker comments for the given reference, ordered
	// as provided by the backend. Implementations should return an empty
	// slice when no comments exist.
	FetchComments(ctx context.Context, repoPath string, ref string) ([]Comment, error)

	// GetTitle returns the issue title for the given reference.
	GetTitle(ctx context.Context, repoPath string, ref string) (string, error)
}
