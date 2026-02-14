package storage

import "errors"

// Sentinel errors used by storage implementations for common API semantics.
var (
	// ErrConflict indicates a unique constraint violation (e.g., name/path already exists).
	ErrConflict = errors.New("conflict")
)
