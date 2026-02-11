package bus

import (
	"context"
	"testing"
	"time"

	"github.com/barzoj/yaralpho/internal/storage"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestSubscribeRegistersSubscriberWithBuffer(t *testing.T) {
	b := NewMemoryBus(Config{}).(*memoryBus)

	sub, err := b.Subscribe(context.Background(), "session-1")
	require.NoError(t, err)
	require.NotNil(t, sub.Events)
	require.NotNil(t, sub.Done())

	b.mu.Lock()
	bucket, ok := b.subs["session-1"]
	b.mu.Unlock()
	require.True(t, ok)
	require.Len(t, bucket, 1)
	require.Equal(t, defaultBufferSize, cap(sub.Events))
}

func TestCloseIdempotentAndCleansUp(t *testing.T) {
	b := NewMemoryBus(Config{}).(*memoryBus)

	sub, err := b.Subscribe(context.Background(), "session-close")
	require.NoError(t, err)

	require.NoError(t, sub.Close())
	require.NoError(t, sub.Close())

	waitForClosed(t, sub.Done())

	b.mu.Lock()
	_, ok := b.subs["session-close"]
	b.mu.Unlock()
	require.False(t, ok, "subscriber should be removed")

	_, ok = <-sub.Events
	require.False(t, ok, "events channel should be closed")
}

func TestContextCancelTriggersCleanup(t *testing.T) {
	b := NewMemoryBus(Config{}).(*memoryBus)
	ctx, cancel := context.WithCancel(context.Background())

	sub, err := b.Subscribe(ctx, "session-cancel")
	require.NoError(t, err)

	cancel()
	waitForClosed(t, sub.Done())

	b.mu.Lock()
	_, ok := b.subs["session-cancel"]
	b.mu.Unlock()
	require.False(t, ok, "subscriber should be removed after context cancel")

	_, ok = <-sub.Events
	require.False(t, ok, "events channel should be closed after context cancel")
}

func TestSessionIsolation(t *testing.T) {
	b := NewMemoryBus(Config{}).(*memoryBus)

	subA, err := b.Subscribe(context.Background(), "session-a")
	require.NoError(t, err)
	subB, err := b.Subscribe(context.Background(), "session-b")
	require.NoError(t, err)

	b.mu.Lock()
	lenA := len(b.subs["session-a"])
	lenB := len(b.subs["session-b"])
	b.mu.Unlock()
	require.Equal(t, 1, lenA)
	require.Equal(t, 1, lenB)

	require.NoError(t, subA.Close())

	b.mu.Lock()
	_, okA := b.subs["session-a"]
	lenB = len(b.subs["session-b"])
	b.mu.Unlock()
	require.False(t, okA)
	require.Equal(t, 1, lenB, "other session bucket should remain")

	require.NoError(t, subB.Close())
}

func TestPublishFanOutOrdering(t *testing.T) {
	b := NewMemoryBus(Config{}).(*memoryBus)

	sub1, err := b.Subscribe(context.Background(), "session-fanout")
	require.NoError(t, err)
	sub2, err := b.Subscribe(context.Background(), "session-fanout")
	require.NoError(t, err)

	evt1 := newEvent("session-fanout", "batch-1", "run-1")
	evt2 := newEvent("session-fanout", "batch-1", "run-2")

	require.NoError(t, b.Publish(context.Background(), "session-fanout", evt1))
	require.NoError(t, b.Publish(context.Background(), "session-fanout", evt2))

	require.Equal(t, evt1, readEvent(t, sub1.Events))
	require.Equal(t, evt1, readEvent(t, sub2.Events))
	require.Equal(t, evt2, readEvent(t, sub1.Events))
	require.Equal(t, evt2, readEvent(t, sub2.Events))
}

func TestPublishIsolationAcrossSessions(t *testing.T) {
	b := NewMemoryBus(Config{}).(*memoryBus)

	subA, err := b.Subscribe(context.Background(), "session-publish-a")
	require.NoError(t, err)
	subB, err := b.Subscribe(context.Background(), "session-publish-b")
	require.NoError(t, err)

	evtA := newEvent("session-publish-a", "batch-a", "run-a")
	evtB := newEvent("session-publish-b", "batch-b", "run-b")

	require.NoError(t, b.Publish(context.Background(), "session-publish-a", evtA))
	requireNoEvent(t, subB.Events)

	require.NoError(t, b.Publish(context.Background(), "session-publish-b", evtB))
	require.Equal(t, evtA, readEvent(t, subA.Events))
	require.Equal(t, evtB, readEvent(t, subB.Events))
}

func TestPublishBlockPolicyRespectsContext(t *testing.T) {
	b := NewMemoryBus(Config{BufferSize: 1, SlowConsumerPolicy: SlowConsumerPolicyBlock}).(*memoryBus)
	sub, err := b.Subscribe(context.Background(), "session-block")
	require.NoError(t, err)

	require.NoError(t, b.Publish(context.Background(), "session-block", newEvent("session-block", "batch-block", "run-1")))

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err = b.Publish(ctx, "session-block", newEvent("session-block", "batch-block", "run-2"))
	require.ErrorIs(t, err, context.DeadlineExceeded)

	require.Equal(t, "run-1", readEvent(t, sub.Events).RunID)
	requireNoEvent(t, sub.Events)
}

func TestPublishDropLatestDropsEvent(t *testing.T) {
	obs, logs := observer.New(zap.WarnLevel)
	b := NewMemoryBus(Config{
		BufferSize:         1,
		SlowConsumerPolicy: SlowConsumerPolicyDropLatest,
		Logger:             zap.New(obs),
	}).(*memoryBus)

	sub, err := b.Subscribe(context.Background(), "session-drop-latest")
	require.NoError(t, err)

	require.NoError(t, b.Publish(context.Background(), "session-drop-latest", newEvent("session-drop-latest", "batch-drop", "run-1")))
	err = b.Publish(context.Background(), "session-drop-latest", newEvent("session-drop-latest", "batch-drop", "run-2"))
	require.ErrorIs(t, err, ErrSlowConsumer)

	require.Equal(t, "run-1", readEvent(t, sub.Events).RunID)
	requireNoEvent(t, sub.Events)
	require.Equal(t, 1, logs.Len())
	require.Equal(t, zapcore.WarnLevel, logs.All()[0].Level)
	require.Equal(t, "slow consumer drop", logs.All()[0].Message)
}

func TestPublishDropOldestReplacesOldest(t *testing.T) {
	obs, logs := observer.New(zap.WarnLevel)
	b := NewMemoryBus(Config{
		BufferSize:         1,
		SlowConsumerPolicy: SlowConsumerPolicyDropOldest,
		Logger:             zap.New(obs),
	}).(*memoryBus)

	sub, err := b.Subscribe(context.Background(), "session-drop-oldest")
	require.NoError(t, err)

	require.NoError(t, b.Publish(context.Background(), "session-drop-oldest", newEvent("session-drop-oldest", "batch-drop", "run-1")))
	err = b.Publish(context.Background(), "session-drop-oldest", newEvent("session-drop-oldest", "batch-drop", "run-2"))
	require.ErrorIs(t, err, ErrSlowConsumer)

	require.Equal(t, "run-2", readEvent(t, sub.Events).RunID)
	requireNoEvent(t, sub.Events)
	require.Equal(t, 1, logs.Len())
	require.Equal(t, "slow consumer drop", logs.All()[0].Message)
}

func TestSubscribeMaxSessions(t *testing.T) {
	obs, logs := observer.New(zap.WarnLevel)
	b := NewMemoryBus(Config{
		MaxSessions:     1,
		CapExceedPolicy: CapExceedPolicyError,
		Logger:          zap.New(obs),
	}).(*memoryBus)

	_, err := b.Subscribe(context.Background(), "session-1")
	require.NoError(t, err)

	_, err = b.Subscribe(context.Background(), "session-2")
	require.ErrorIs(t, err, ErrSessionLimitExceeded)
	b.mu.Lock()
	require.Equal(t, 1, len(b.subs))
	b.mu.Unlock()
	require.Equal(t, 1, logs.Len())
	require.Equal(t, "bus capacity exceeded", logs.All()[0].Message)
}

func TestSubscribeMaxSubscribersDropPolicy(t *testing.T) {
	obs, logs := observer.New(zap.WarnLevel)
	b := NewMemoryBus(Config{
		MaxSubscribersPerSession: 1,
		CapExceedPolicy:          CapExceedPolicyDrop,
		Logger:                   zap.New(obs),
	}).(*memoryBus)

	first, err := b.Subscribe(context.Background(), "session-1")
	require.NoError(t, err)
	require.NotNil(t, first.Events)

	_, err = b.Subscribe(context.Background(), "session-1")
	require.ErrorIs(t, err, ErrSubscriberLimitExceeded)
	b.mu.Lock()
	require.Len(t, b.subs["session-1"], 1)
	b.mu.Unlock()
	require.Equal(t, 1, logs.Len())
	require.Equal(t, "bus capacity exceeded", logs.All()[0].Message)

	require.NoError(t, first.Close())
}

func TestIdleTimeoutPrunesSubscriber(t *testing.T) {
	b := NewMemoryBus(Config{
		IdleTimeout: 20 * time.Millisecond,
	}).(*memoryBus)

	sub, err := b.Subscribe(context.Background(), "session-idle")
	require.NoError(t, err)

	waitForClosed(t, sub.Done())

	b.mu.Lock()
	_, ok := b.subs["session-idle"]
	b.mu.Unlock()
	require.False(t, ok, "subscriber bucket should be pruned on idle timeout")

	_, open := <-sub.Events
	require.False(t, open, "events channel should be closed on idle")
}

func TestIdleTimeoutResetsOnPublish(t *testing.T) {
	b := NewMemoryBus(Config{
		IdleTimeout: 40 * time.Millisecond,
		BufferSize:  1,
	}).(*memoryBus)

	sub, err := b.Subscribe(context.Background(), "session-active")
	require.NoError(t, err)

	time.Sleep(25 * time.Millisecond)

	err = b.Publish(context.Background(), "session-active", newEvent("session-active", "batch-reset", "run-1"))
	require.NoError(t, err)

	select {
	case <-sub.Done():
		t.Fatalf("subscription closed before idle timeout after activity")
	case <-time.After(25 * time.Millisecond):
	}

	waitForClosed(t, sub.Done())
}

func waitForClosed(t *testing.T, ch <-chan struct{}) {
	t.Helper()

	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for channel to close")
	}
}

func newEvent(sessionID, batchID, runID string) storage.SessionEvent {
	return storage.SessionEvent{
		BatchID:    batchID,
		RunID:      runID,
		SessionID:  sessionID,
		IngestedAt: time.Now(),
	}
}

func readEvent(t *testing.T, ch <-chan storage.SessionEvent) storage.SessionEvent {
	t.Helper()

	select {
	case evt, ok := <-ch:
		if !ok {
			t.Fatalf("channel closed unexpectedly")
		}
		return evt
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for event")
		return storage.SessionEvent{}
	}
}

func requireNoEvent(t *testing.T, ch <-chan storage.SessionEvent) {
	t.Helper()

	select {
	case evt := <-ch:
		t.Fatalf("expected no event, got %+v", evt)
	default:
	}
}
