package queue

import (
	"context"
	"sync"

	"go.uber.org/zap"
)

// MemoryQueue provides an in-memory FIFO queue with a single consumer. It is
// thread-safe and supports blocking dequeue with context cancellation.
type MemoryQueue struct {
	mu     sync.Mutex
	cond   *sync.Cond
	items  []string
	closed bool
	logger *zap.Logger
}

// NewMemoryQueue constructs a MemoryQueue using the provided logger. If logger
// is nil, logging is disabled via zap.NewNop().
func NewMemoryQueue(logger *zap.Logger) *MemoryQueue {
	if logger == nil {
		logger = zap.NewNop()
	}
	mq := &MemoryQueue{logger: logger}
	mq.cond = sync.NewCond(&mq.mu)
	return mq
}

// Enqueue adds an item to the tail of the queue. Returns ErrClosed if the
// queue has been closed.
func (q *MemoryQueue) Enqueue(item string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return ErrClosed
	}

	q.items = append(q.items, item)
	q.logger.Debug("queue enqueue", zap.Int("size", len(q.items)))
	q.cond.Signal()
	return nil
}

// Dequeue blocks until an item is available, the queue is closed, or the
// context is cancelled. It returns ErrClosed when the queue is closed and empty.
func (q *MemoryQueue) Dequeue(ctx context.Context) (string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return "", err
	}

	for len(q.items) == 0 && !q.closed {
		waitDone := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				q.mu.Lock()
				q.cond.Broadcast()
				q.mu.Unlock()
			case <-waitDone:
			}
		}()

		q.cond.Wait()
		close(waitDone)

		if err := ctx.Err(); err != nil {
			return "", err
		}
	}

	if len(q.items) == 0 && q.closed {
		return "", ErrClosed
	}

	item := q.items[0]
	q.items = q.items[1:]
	q.logger.Debug("queue dequeue", zap.Int("remaining", len(q.items)))
	return item, nil
}

// Close marks the queue as closed and unblocks any waiting dequeuers. After
// closure, Enqueue will return ErrClosed, and Dequeue will return ErrClosed
// once all buffered items are drained.
func (q *MemoryQueue) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return
	}
	q.closed = true
	q.logger.Debug("queue closed")
	q.cond.Broadcast()
}
