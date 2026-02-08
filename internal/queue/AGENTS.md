# Purpose
Provide a FIFO queue abstraction with a single consumer model to feed the worker loop.

# Exposed Interfaces
- `Queue` interface supporting `Enqueue`, `Dequeue(ctx)` (blocking, context-aware), and `Close` (see `interface.go`).
- In-memory implementation (`memory.go`) that is thread safe, FIFO, and logs enqueue/dequeue events with zap.
- `Dequeue` must unblock on context cancellation or queue closure; `Close` makes subsequent `Enqueue` return an error and `Dequeue` return `ErrClosed` once drained.

# Notes for Agents
- Dequeue must unblock promptly on context cancellation or queue closure—no busy waiting.
- Keep implementation minimal; external queue systems are out of scope for this project.***
