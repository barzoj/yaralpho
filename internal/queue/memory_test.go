package queue

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestFIFOOrdering(t *testing.T) {
	q := NewMemoryQueue(zap.NewNop())

	require.NoError(t, q.Enqueue("a"))
	require.NoError(t, q.Enqueue("b"))
	require.NoError(t, q.Enqueue("c"))

	ctx := context.Background()

	v1, err := q.Dequeue(ctx)
	require.NoError(t, err)
	require.Equal(t, "a", v1)

	v2, err := q.Dequeue(ctx)
	require.NoError(t, err)
	require.Equal(t, "b", v2)

	v3, err := q.Dequeue(ctx)
	require.NoError(t, err)
	require.Equal(t, "c", v3)
}

func TestDequeueBlocksUntilEnqueue(t *testing.T) {
	q := NewMemoryQueue(zap.NewExample())

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := make(chan string, 1)
	go func() {
		v, err := q.Dequeue(ctx)
		if err == nil {
			done <- v
		}
	}()

	time.Sleep(50 * time.Millisecond)
	require.NoError(t, q.Enqueue("ready"))

	select {
	case got := <-done:
		require.Equal(t, "ready", got)
	case <-ctx.Done():
		t.Fatalf("dequeue timed out: %v", ctx.Err())
	}
}

func TestDequeueRespectsContextCancellation(t *testing.T) {
	q := NewMemoryQueue(zap.NewNop())

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	_, err := q.Dequeue(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestClosePreventsFurtherUse(t *testing.T) {
	q := NewMemoryQueue(zap.NewNop())

	q.Close()

	_, err := q.Dequeue(context.Background())
	require.ErrorIs(t, err, ErrClosed)

	err = q.Enqueue("x")
	require.ErrorIs(t, err, ErrClosed)
}

func TestDrainThenClosed(t *testing.T) {
	q := NewMemoryQueue(zap.NewNop())

	require.NoError(t, q.Enqueue("a"))
	require.NoError(t, q.Enqueue("b"))
	q.Close()

	v1, err := q.Dequeue(context.Background())
	require.NoError(t, err)
	require.Equal(t, "a", v1)

	v2, err := q.Dequeue(context.Background())
	require.NoError(t, err)
	require.Equal(t, "b", v2)

	_, err = q.Dequeue(context.Background())
	require.ErrorIs(t, err, ErrClosed)
}
