package bus

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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

func waitForClosed(t *testing.T, ch <-chan struct{}) {
	t.Helper()

	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for channel to close")
	}
}
