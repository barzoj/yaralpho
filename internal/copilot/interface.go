package copilot

import "context"

// RawEvent represents an unmodified payload emitted by the Copilot provider
// while a session is running. Implementations must forward events exactly as
// received so downstream storage can persist the full record.
type RawEvent map[string]any

// Client starts Copilot sessions for task runs without exposing provider-
// specific types. Implementations must auto-approve any permission prompts
// surfaced by the underlying SDK and keep the returned events raw.
type Client interface {
	// StartSession begins a Copilot session using the supplied prompt and
	// repository path. The returned channel streams RawEvent values until the
	// session ends or stop is called. The stop function should be safe to call
	// multiple times and must release any SDK resources.
	StartSession(ctx context.Context, prompt, repoPath string) (sessionID string, events <-chan RawEvent, stop func(), err error)
}
