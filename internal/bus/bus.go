package bus

import (
	"context"
	"errors"
	"time"

	"github.com/barzoj/yaralpho/internal/storage"
	"go.uber.org/zap"
)

const defaultBufferSize = 64

// SlowConsumerPolicy defines how the bus should behave when subscribers lag.
type SlowConsumerPolicy string

const (
	SlowConsumerPolicyBlock      SlowConsumerPolicy = "block"
	SlowConsumerPolicyDropLatest SlowConsumerPolicy = "drop_latest"
	SlowConsumerPolicyDropOldest SlowConsumerPolicy = "drop_oldest"
)

var (
	// ErrSlowConsumer signals that a subscriber buffer was full and the slow-consumer
	// policy was applied.
	ErrSlowConsumer = errors.New("bus: slow consumer buffer full")
	// ErrSubscriptionClosed indicates an attempt to publish to a subscription that
	// has already been cleaned up.
	ErrSubscriptionClosed = errors.New("bus: subscription closed")
	// ErrSessionLimitExceeded indicates the bus has reached the configured max sessions.
	ErrSessionLimitExceeded = errors.New("bus: max sessions exceeded")
	// ErrSubscriberLimitExceeded indicates the session has reached the configured max subscribers.
	ErrSubscriberLimitExceeded = errors.New("bus: max subscribers per session exceeded")
)

// Config controls memory bus behavior and instrumentation.
type Config struct {
	BufferSize               int
	SlowConsumerPolicy       SlowConsumerPolicy
	CapExceedPolicy          CapExceedPolicy
	MaxSessions              int
	MaxSubscribersPerSession int
	IdleTimeout              time.Duration
	Logger                   *zap.Logger
}

// Bus publishes session events and allows subscriptions.
type Bus interface {
	Publish(ctx context.Context, sessionID string, evt storage.SessionEvent) error
	Subscribe(ctx context.Context, sessionID string) (Subscription, error)
}

// Subscription exposes a stream of session events and a cleanup hook.
type Subscription struct {
	Events <-chan storage.SessionEvent

	done    <-chan struct{}
	closeFn func() error
}

// Done signals when the subscription has been closed and cleaned up.
func (s Subscription) Done() <-chan struct{} {
	return s.done
}

// Close releases subscription resources. It is safe to call multiple times.
func (s Subscription) Close() error {
	if s.closeFn == nil {
		return nil
	}
	return s.closeFn()
}

func normalizeConfig(cfg Config) Config {
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = defaultBufferSize
	}
	if cfg.SlowConsumerPolicy == "" {
		cfg.SlowConsumerPolicy = SlowConsumerPolicyBlock
	}
	if cfg.CapExceedPolicy == "" {
		cfg.CapExceedPolicy = CapExceedPolicyError
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}
	return cfg
}

// CapExceedPolicy defines how the bus handles subscription attempts that would
// exceed configured limits.
type CapExceedPolicy string

const (
	CapExceedPolicyError CapExceedPolicy = "error"
	CapExceedPolicyDrop  CapExceedPolicy = "drop"
)
