# Purpose
Provide a FIFO queue abstraction with a single consumer model to feed the worker loop.

# Exposed Interfaces
- `Queue` interface supporting `Enqueue`, `Dequeue(ctx)` (blocking, context-aware), and `Close`.
- In-memory implementation responsible for thread safety and logging enqueue/dequeue events.

# Notes for Agents
- Dequeue must unblock promptly on context cancellation or queue closure—no busy waiting.
- Keep implementation minimal; external queue systems are out of scope for this project.***
