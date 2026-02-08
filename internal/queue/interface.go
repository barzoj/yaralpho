package queue

import (
	"context"
	"errors"
)

// ErrClosed indicates operations attempted on a queue that has been closed.
var ErrClosed = errors.New("queue closed")

// Queue exposes a simple FIFO contract with a single consumer. Dequeue blocks
// until an item is available, the queue is closed, or the provided context is
// cancelled.
type Queue interface {
	Enqueue(item string) error
	Dequeue(ctx context.Context) (string, error)
	Close()
}
