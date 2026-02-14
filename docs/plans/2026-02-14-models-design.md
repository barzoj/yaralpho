# Repository-Aware Models Design

**Supporting documentation:** Based on epic plan in `docs/plans/2026-02-14-refactoring-input.md` outlining repository-aware scheduler and CRUD surface. No external deps.

## Scope
Define domain models and constants for repositories, agents, batches, items, and runs to support repository-scoped scheduling and agent management.

## Entities
- **Repository:** `id`, `name`, `path`, timestamps. Name/path unique; path validated elsewhere.
- **Agent:** `id`, human-friendly `name`, `runtime` (codex|copilot), `status` (idle|busy), timestamps.
- **Batch:** `id`, `repository_id`, `status` (pending|in_progress|failed|paused|done), timestamps, optional `summary`/`session_name`, `items []BatchItem` preserving item order.
- **BatchItem:** `input` (task ref or path), `status` (pending|in_progress|done|failed), `attempts` (int, starts at 0).
- **Run (TaskRun):** `run_id`, `batch_id`, `repository_id`, `task_ref` (item input), optional `parent_ref` (epic/parent tracking), `session_id`, timing, `status` (running|succeeded|failed|stopped), optional `result`.
- **SessionEvent / BatchProgress:** unchanged aside from repository linkage via run/batch ids.

## Status Constants
- **BatchStatus:** pending, in_progress, failed, paused, done. Pending used for newly created and post-retry batches; in_progress while an item is executing.
- **ItemStatus:** pending, in_progress, done, failed. Attempts increment on each run.
- **AgentStatus:** idle, busy. Default idle; busy while executing an item.

## Interface Shape (storage.Storage)
Add repository and agent CRUD; keep batch/run/event operations; introduce batch listing by repository/status to support upcoming endpoints. Implementations may temporarily stub logic until persistence tasks land.

## Testing Plan
Models/interface must compile; `go test ./internal/storage/...` remains green after stubbing new methods in fakes/clients.
