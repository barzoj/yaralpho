# Purpose
Define storage domain models and interfaces for batches, task runs, and session events. Keep persistence concerns abstract so implementations (e.g., MongoDB) can swap without touching callers.

# Exposed Interfaces
- `Storage` contract covering insert/get/list/update for batches and task runs, session event ingestion, and progress counts.
- Domain models include batch metadata, task run metadata, and raw copilot session events; JSON/BSON friendly fields expected.

# Notes for Agents
- MongoDB implementation will live in `internal/storage/mongo` and must honor the `Storage` interface only.
- Use contexts for all I/O; propagate zap logger from callers for structured logging.
- Raw copilot events should be stored unmodified; avoid logging secrets.***
