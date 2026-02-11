package bus

import (
	"context"
	"errors"
	"sync"
	"time"

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

	b.mu.Lock()
	bucket, ok := b.subs[sessionID]
	if !ok {
		if b.cfg.MaxSessions > 0 && len(b.subs) >= b.cfg.MaxSessions {
			b.mu.Unlock()
			return Subscription{}, b.handleCapExceeded(sessionID, "sessions", ErrSessionLimitExceeded)
		}
		bucket = make(map[*subscriber]struct{})
		b.subs[sessionID] = bucket
	}
	if b.cfg.MaxSubscribersPerSession > 0 && len(bucket) >= b.cfg.MaxSubscribersPerSession {
		b.mu.Unlock()
		return Subscription{}, b.handleCapExceeded(sessionID, "subscribers", ErrSubscriberLimitExceeded)
	}

	sub := newSubscriber(b.cfg.BufferSize, b.cfg.IdleTimeout)
	bucket[sub] = struct{}{}
	b.mu.Unlock()

	sub.setCleanup(b.cleanupFunc(sessionID, sub))
	sub.startIdleTimer(func() {
		_ = sub.Close()
	})
	go sub.watchContext(ctx)

	return Subscription{
		Events:  sub.events,
		done:    sub.done,
		closeFn: sub.Close,
	}, nil
}

func (b *memoryBus) handleCapExceeded(sessionID, limit string, capErr error) error {
	b.cfg.Logger.Warn(
		"bus capacity exceeded",
		zap.String("policy", string(b.cfg.CapExceedPolicy)),
		zap.String("session_id", sessionID),
		zap.String("limit", limit),
		zap.Error(capErr),
	)

	// Both policies currently signal failure to the caller while leaving
	// existing subscribers untouched.
	return capErr
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
		sub.touch()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *memoryBus) handleDropLatest(sessionID string, sub *subscriber, evt storage.SessionEvent) error {
	select {
	case sub.events <- evt:
		sub.touch()
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
		sub.touch()
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

	idleTimeout time.Duration
	idleReset   chan struct{}

	closeOnce sync.Once
	cleanup   func() error
}

func newSubscriber(buffer int, idleTimeout time.Duration) *subscriber {
	s := &subscriber{
		events: make(chan storage.SessionEvent, buffer),
		done:   make(chan struct{}),
	}
	if idleTimeout > 0 {
		s.idleTimeout = idleTimeout
		s.idleReset = make(chan struct{}, 1)
	}
	return s
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

func (s *subscriber) startIdleTimer(onIdle func()) {
	if s.idleTimeout <= 0 {
		return
	}

	timer := time.NewTimer(s.idleTimeout)
	go func() {
		defer timer.Stop()
		for {
			select {
			case <-timer.C:
				onIdle()
				return
			case <-s.done:
				return
			case <-s.idleReset:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(s.idleTimeout)
			}
		}
	}()
}

func (s *subscriber) touch() {
	if s.idleTimeout <= 0 {
		return
	}
	select {
	case s.idleReset <- struct{}{}:
	default:
	}
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
