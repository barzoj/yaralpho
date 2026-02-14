# Purpose
Composition root for the Ralph Runner service: builds dependencies and exposes HTTP handlers.

# Exposed Interfaces
- Functions to construct the application given config and zap logger, wiring storage, tracker, notifier, copilot client, and event bus.
- HTTP server setup (gorilla/mux) with route registration for /repository/{repoid}/add, /batches, /batches/{id}, /batches/{id}/progress, /runs, /runs/{id}.

# Notes for Agents
- Configuration is env-first; honor optional config path flag from `cmd/main.go`.
- Ensure graceful shutdown closes storage client and copilot resources; scheduler/worker integration happens outside the app layer.
- Keep dependency graph interface-based to allow swapping implementations.***
