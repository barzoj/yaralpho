# Purpose
Composition root for the Ralph Runner service: builds dependencies, starts the consumer, and exposes HTTP handlers.

# Exposed Interfaces
- Functions to construct the application given config and zap logger, wiring storage, queue, tracker, notifier, copilot client, and consumer.
- HTTP server setup (gorilla/mux) with route registration for /add, /batches, /batches/{id}, /batches/{id}/progress, /runs, /runs/{id}.

# Notes for Agents
- Configuration is env-first; honor optional config path flag from `cmd/main.go`.
- Start the consumer once and ensure graceful shutdown closes queue, storage client, and copilot resources.
- Keep dependency graph interface-based to allow swapping implementations.***
