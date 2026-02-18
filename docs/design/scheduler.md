# Repository-aware scheduler

This document summarizes the current scheduling model, data shapes, and HTTP surface for repository-scoped batch execution. The in-memory queue used by the original runner has been removed; scheduler ticks dispatch work directly to the consumer worker.

## Purpose

- Keep batches scoped to a repository; items within a batch always run sequentially.
- Allow multiple repositories to progress in parallel when idle agents exist.
- Record every execution attempt (task run) with repository context and raw session events for replay.
- Provide a restart hook for failed batches and live event streaming for observability.

## Entities

- **Repository** – `{repository_id, name, path, created_at, updated_at}`. Stored in Mongo and managed via HTTP CRUD; `path` must be absolute and deletion is blocked when batches are active.
- **Batch** – `{batch_id, repository_id, items[], status, session_name?, summary?}`. Status: `pending|in_progress|paused|done|failed`.
- **Batch item** – `{input, status, attempts}`. Status: `pending|in_progress|done|failed`; `attempts` increments on each retry.
- **Agent** – `{agent_id, name, runtime (codex|copilot), status, created_at, updated_at}`. Status: `idle|busy`; updates/deletes are blocked when `busy`.
- **Task run** – `{run_id, batch_id, repository_id, task_ref, parent_ref?, session_id, status, started_at, finished_at?, result?}` with status `running|succeeded|failed|stopped`.
- **Session event** – `{run_id, batch_id, session_id, event, ingested_at}`; streamed via REST or WebSocket.

## Scheduling flow (expected behavior)

1. Pick the next batch that is **not paused** and has a **pending** item with no other item in progress for that batch.
2. Select the first **idle** agent; mark it `busy` before execution and back to `idle` afterward.
3. Create a **task run** for the item with `status=running` and `repository_id` populated.
4. On success: set item `done`; if all items are done, set batch `done`.
5. On failure: increment `attempts`; if `attempts < YARALPHO_MAX_RETRIES` set item back to `pending`, otherwise mark item and batch `failed`.
6. Failed batches can be reset via `/repository/{repoid}/batch/{batchid}/restart`, which sets the failed item to `pending` and `attempts=0` so the next tick can retry.

## Pause and resume

The `paused` status skips scheduling new work while allowing in-flight items to finish. HTTP endpoints `/repository/{repoid}/batch/{batchid}/pause` and `/.../resume` toggle this flag; pause returns `409` if a batch is done/failed or already paused, resume returns `409` when not paused.

## HTTP surface (implemented)

- `POST /repository` – create a repository (absolute path required).
- `GET /repository` – list repositories.
- `GET /repository/{id}` – repository detail.
- `PUT /repository/{id}` – update name/path (requires absolute path).
- `DELETE /repository/{id}` – delete when no active batches exist.
- `POST /repository/{repoid}/add` – create a pending batch for existing repository.
- `GET /batches`, `GET /batches/{id}`, `GET /batches/{id}/progress` – read batch state.
- `GET /repository/{repoid}/batches?status=` – list batches scoped to a repository with optional status filter.
- `PUT /repository/{repoid}/batch/{batchid}/pause` – pause a batch (no new items start).
- `PUT /repository/{repoid}/batch/{batchid}/resume` – return a paused batch to `pending`.
- `PUT /repository/{repoid}/batch/{batchid}/restart` – restart a failed batch.
- `POST|GET|PUT|DELETE /agent` – manage runtime agents (restricted when `busy`).
- `GET /repository/{repoid}/batch/{batchid}/runs` – runs for a batch.
- `GET /runs/{id}` – run detail (repository-aware).
- `GET /runs/{id}/events` – paginated events.
- `GET /runs/{id}/events/live` – WebSocket event stream with heartbeats and optional `last_ingested` cursor.
- `POST /restart?wait=true|false` – drain scheduler; with `wait=true` the request blocks until all active runs finish or `YARALPHO_RESTART_WAIT_TIMEOUT` elapses.

## Config knobs

- `YARALPHO_SCHEDULER_INTERVAL` (default `10s`) – tick cadence.
- `YARALPHO_MAX_RETRIES` (default `5`) – attempts per batch item before marking batch failed.
- `YARALPHO_RESTART_WAIT_TIMEOUT` (default `20m`) – max block time for `/restart?wait=true`.
- `YARALPHO_TASK_RUN_TIMEOUT` (default `20m`) – max total duration per task run across execution and verification.
- `YARALPHO_TASK_EXEC_TIMEOUT` (default `20m`) – max duration allowed for the execution phase of a task run.
- `YARALPHO_TASK_VERIFY_TIMEOUT` (default `20m`) – max duration allowed for the verification phase of a task run.

### Timeout behavior

- The worker wraps the entire task run (execution + verification, including retries) in `YARALPHO_TASK_RUN_TIMEOUT`; a deadline cancels the run, stops the Copilot session, and surfaces a timeout error.
- Worker execution and verification phases run under their respective timeouts; on deadline exceeded the worker stops the Copilot session, marks the run timed out, and returns the timeout error.
- The scheduler handles timeout errors like other failures: it increments `attempts`, releases the agent back to `idle`, and moves the item out of `in_progress` for retry until `YARALPHO_MAX_RETRIES` is reached.

_Notable removals_: the legacy `/add` queue entrypoint and global `/runs` list have been removed; work is now repository-scoped and sequential per batch. Epics are gone; runs carry `parent_ref` only when the tracker reports one, without special APIs.
