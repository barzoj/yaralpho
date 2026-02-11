package bus

import (
	"context"
	"errors"
	"sync"

	"github.com/barzoj/yaralpho/internal/storage"
	"go.uber.org/zap"
)

// NewMemoryBus constructs an in-memory Bus using the provided configuration.
func NewMemoryBus(cfg Config) Bus {
	cfg = normalizeConfig(cfg)
	return &memoryBus{
		subs: make(map[string]map[*subscriber]struct{}),
		cfg:  cfg,
	}
}

type memoryBus struct {
	mu   sync.Mutex
	subs map[string]map[*subscriber]struct{}
	cfg  Config
}

func (b *memoryBus) Publish(ctx context.Context, sessionID string, evt storage.SessionEvent) error {
	if ctx == nil {
		ctx = context.Background()
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	subs := b.snapshotSubscribers(sessionID)
	if len(subs) == 0 {
		return nil
	}

	var firstErr error
	for _, sub := range subs {
		if err := b.publishToSubscriber(ctx, sessionID, sub, evt); err != nil && !errors.Is(err, ErrSubscriptionClosed) && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

func (b *memoryBus) Subscribe(ctx context.Context, sessionID string) (Subscription, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	sub := newSubscriber(b.cfg.BufferSize)

	b.mu.Lock()
	bucket, ok := b.subs[sessionID]
	if !ok {
		bucket = make(map[*subscriber]struct{})
		b.subs[sessionID] = bucket
	}
	bucket[sub] = struct{}{}
	b.mu.Unlock()

	sub.setCleanup(b.cleanupFunc(sessionID, sub))
	go sub.watchContext(ctx)

	return Subscription{
		Events:  sub.events,
		done:    sub.done,
		closeFn: sub.Close,
	}, nil
}

func (b *memoryBus) cleanupFunc(sessionID string, sub *subscriber) func() error {
	return func() error {
		b.mu.Lock()
		defer b.mu.Unlock()

		if bucket, ok := b.subs[sessionID]; ok {
			delete(bucket, sub)
			if len(bucket) == 0 {
				delete(b.subs, sessionID)
			}
		}

		close(sub.events)
		return nil
	}
}

func (b *memoryBus) snapshotSubscribers(sessionID string) []*subscriber {
	b.mu.Lock()
	defer b.mu.Unlock()

	bucket := b.subs[sessionID]
	subs := make([]*subscriber, 0, len(bucket))
	for sub := range bucket {
		subs = append(subs, sub)
	}

	return subs
}

func (b *memoryBus) publishToSubscriber(ctx context.Context, sessionID string, sub *subscriber, evt storage.SessionEvent) (err error) {
	select {
	case <-sub.done:
		return ErrSubscriptionClosed
	default:
	}

	defer func() {
		if r := recover(); r != nil {
			err = ErrSubscriptionClosed
		}
	}()

	switch b.cfg.SlowConsumerPolicy {
	case SlowConsumerPolicyDropLatest:
		return b.handleDropLatest(sessionID, sub, evt)
	case SlowConsumerPolicyDropOldest:
		return b.handleDropOldest(ctx, sessionID, sub, evt)
	default:
		return b.handleBlocking(ctx, sub, evt)
	}
}

func (b *memoryBus) handleBlocking(ctx context.Context, sub *subscriber, evt storage.SessionEvent) error {
	select {
	case sub.events <- evt:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *memoryBus) handleDropLatest(sessionID string, sub *subscriber, evt storage.SessionEvent) error {
	select {
	case sub.events <- evt:
		return nil
	default:
		b.logDrop(sessionID, evt, "drop_latest")
		return ErrSlowConsumer
	}
}

func (b *memoryBus) handleDropOldest(ctx context.Context, sessionID string, sub *subscriber, evt storage.SessionEvent) error {
	select {
	case sub.events <- evt:
		return nil
	default:
	}

	select {
	case <-sub.events:
	default:
	}

	b.logDrop(sessionID, evt, "drop_oldest")

	select {
	case sub.events <- evt:
		return ErrSlowConsumer
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *memoryBus) logDrop(sessionID string, evt storage.SessionEvent, reason string) {
	b.cfg.Logger.Warn(
		"slow consumer drop",
		zap.String("policy", string(b.cfg.SlowConsumerPolicy)),
		zap.String("session_id", sessionID),
		zap.String("run_id", evt.RunID),
		zap.String("batch_id", evt.BatchID),
		zap.String("reason", reason),
	)
}

type subscriber struct {
	events chan storage.SessionEvent
	done   chan struct{}

	closeOnce sync.Once
	cleanup   func() error
}

func newSubscriber(buffer int) *subscriber {
	return &subscriber{
		events: make(chan storage.SessionEvent, buffer),
		done:   make(chan struct{}),
	}
}

func (s *subscriber) setCleanup(fn func() error) {
	s.cleanup = fn
}

func (s *subscriber) Close() error {
	var err error
	s.closeOnce.Do(func() {
		if s.cleanup != nil {
			err = s.cleanup()
		}
		close(s.done)
	})
	return err
}

func (s *subscriber) watchContext(ctx context.Context) {
	if ctx == nil {
		return
	}

	select {
	case <-ctx.Done():
		_ = s.Close()
	case <-s.done:
	}
}
