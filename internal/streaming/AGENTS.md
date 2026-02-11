# Purpose
Provide session event streaming primitives so new live WebSocket endpoints can fan out session events without polling.

# Exposed Interfaces
- Event bus interfaces for publishing session events and subscribing per session/run.
- Configurable buffering and policies to keep ordering and avoid slow-consumer leaks.

# Notes for Agents
- Reuse storage.SessionEvent as the payload type and preserve ingest ordering for websocket clients.
- Keep subscribe/publish lifecycle safe for concurrent consumers and ensure subscriptions are cleaned up on close/cancel.***
