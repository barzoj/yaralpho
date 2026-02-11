package bus

import (
	"context"
	"errors"

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
)

// Config controls memory bus behavior and instrumentation.
type Config struct {
	BufferSize         int
	SlowConsumerPolicy SlowConsumerPolicy
	Logger             *zap.Logger
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
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}
	return cfg
}
