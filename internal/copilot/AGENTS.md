# Purpose
Abstract interaction with the GitHub Copilot SDK (or other providers) to start sessions and stream raw events for each task run.

# Exposed Interfaces
- `Client` interface (name TBD) to start a session for a task, returning session metadata, event channel/stream, and a stop function.
- API must not leak SDK-specific types to callers.

# Notes for Agents
- GitHub implementation will auto-approve permission requests and must forward events unmodified to storage.
- Validate presence of authentication tokens before starting a session; log with zap but do not print secrets.***
