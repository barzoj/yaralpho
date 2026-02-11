package streaming

import (
	"context"
	"time"

	"github.com/barzoj/yaralpho/internal/storage"
)

// SubscribeOptions controls how a subscriber receives session events.
// LastIngestedAt, when set, is exclusive and allows replay to start after
// the provided cursor while preserving ingest ordering. Buffer sets the
// subscriber channel capacity (0 uses the implementation default).
type SubscribeOptions struct {
	LastIngestedAt *time.Time
	Buffer         int
}

// Subscription represents a live stream of session events and a cleanup hook.
// Events must be delivered in ingest order for the session. Cancel must
// release all resources and close the Events channel once complete.
type Subscription struct {
	Events <-chan storage.SessionEvent
	Cancel func()
}

// SessionEventBus publishes session events and allows clients to subscribe to
// a session's stream. Implementations must preserve ingest ordering and honor
// context cancellation for both publish and subscribe calls.
type SessionEventBus interface {
	// Publish forwards a session event to the bus. Events should be delivered
	// to subscribers in the order they were ingested.
	Publish(ctx context.Context, event storage.SessionEvent) error

	// Subscribe attaches to a session's stream starting after LastIngestedAt
	// (exclusive) when provided. The returned subscription must emit events in
	// ingest order and stop when the context is done or Cancel is invoked.
	Subscribe(ctx context.Context, sessionID string, opts SubscribeOptions) (Subscription, error)
}
