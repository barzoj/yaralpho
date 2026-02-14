# yaralpho

Ralph Runner is a Go 1.25+ service that schedules repository-scoped batches of tasks across runtime agents (Codex or GitHub Copilot). Batches are processed sequentially within each repository while multiple repositories can run in parallel when idle agents are available. MongoDB stores repositories, batches, runs, and raw session events; a lightweight HTTP API plus a small UI surface observability.

## Agent providers

Startup agent selection is explicit via `--agent`, and defaults to `codex`.

- Allowed values: `codex` (default), `github`
- Invalid values fail fast at process start (exit code `2`)
- There is no silent fallback to another provider

Examples:

```bash
# Default (equivalent to --agent=codex)
go run ./cmd

# Explicit Codex provider
go run ./cmd --agent=codex

# GitHub Copilot provider
go run ./cmd --agent=github
```

### Codex wrapper requirements

The Codex provider launches a local wrapper binary. On startup it resolves the
wrapper path in this order:

1. `YARALPHO_CODEX_WRAPPER_PATH` (environment override)
2. `internal/copilot/codex-ts/bin/codex-wrapper-linux-x64` relative to the current process working directory

If the selected wrapper path is missing, is a directory, or is not executable,
startup fails immediately with an explicit error.

Build/package the Linux x64 wrapper:

```bash
cd internal/copilot/codex-ts
npm run package:linux-x64
```

`npm run package:linux-x64` uses `bun build --compile`, so Bun must be installed and on `PATH`.

### Session completion compatibility

To keep consumer behavior compatible with existing run loops, the Codex client
emits a synthetic terminal event with `type=session.idle` when the wrapper
process exits cleanly.

## Concepts and lifecycle

- **Repository** – identifies the codebase being worked on (name + filesystem path). Batches, runs, and session events are scoped to a repository.
- **Batch** – ordered list of task refs created under a repository. Items run strictly sequentially within a batch. Batch statuses: `pending`, `in_progress`, `paused`, `done`, `failed`.
- **Batch item** – a single task ref plus retry metadata. Item statuses: `pending`, `in_progress`, `done`, `failed`; `attempts` increments on each retry.
- **Agent** – runtime worker (`codex` or `copilot`) marked `idle` or `busy`. Agents can be created/updated/deleted only while idle.
- **Task run** – one execution attempt for a batch item; includes `repository_id` and optional `parent_ref` when a tracker identifies epics. Run statuses: `running`, `succeeded`, `failed`, `stopped`.
- **Session events** – raw Copilot stream events keyed by `session_id` and available via REST and WebSocket for replay/live tails.

Scheduling rules (enforced by the scheduler/worker layer):

- A batch’s items execute one at a time; no two items from the same batch run concurrently.
- The first idle agent is picked for the next eligible item. Agents are marked `busy` before execution and returned to `idle` afterward.
- Retries per item are capped by `YARALPHO_MAX_RETRIES` (default `5`). Exhausting retries marks the item and batch `failed` until `/restart` is invoked.
- `paused` batches are skipped until resumed (pause/resume endpoints are not yet exposed in HTTP; the status exists for future wiring).
- `/restart` on a failed batch resets the failed item to `pending` with attempts reset to 0.

## Configuration

The config loader is environment-first with an optional JSON fallback:

- Env vars override values from the JSON file.
- JSON is loaded from `config.json` by default; override the path with the `RALPH_CONFIG` env var (or pass a path to the loader).
- Blank/whitespace values are ignored.
- On startup the loader panics if any required keys are missing.
- Token precedence: `COPILOT_GITHUB_TOKEN` → `GH_TOKEN` → `GITHUB_TOKEN`.

| Key | Required? | Default | Description |
| --- | --- | --- | --- |
| `YARALPHO_MONGODB_URI` | yes | — | Mongo connection string. |
| `YARALPHO_MONGODB_DB` | yes | — | Mongo database name. |
| `YARALPHO_REPO_PATH` | yes | — | Filesystem path to the repo the runner operates in. |
| `YARALPHO_BD_REPO` | yes | — | Path to the beads (`bd`) repository used for tracking. |
| `YARALPHO_PORT` | no | `8080` | HTTP listen port. |
| `YARALPHO_SLACK_WEBHOOK_URL` | no | — | Slack webhook for notifications (noop when unset). |
| `YARALPHO_MAX_RETRIES` | no | `5` | Max attempts per batch item before the batch is marked `failed`. |
| `COPILOT_GITHUB_TOKEN` | yes* | — | Primary token for GitHub Copilot SDK. |
| `GH_TOKEN` | no* | — | Fallback token if `COPILOT_GITHUB_TOKEN` is unset. |
| `GITHUB_TOKEN` | no* | — | Secondary fallback if both above are unset. |
| `YARALPHO_EXECUTION_TASK_PROMPT` | no | built-in | Prompt template for execution agents (used by worker). |
| `YARALPHO_VERIFICATION_TASK_PROMPT` | no | built-in | Prompt template for verification agents. |
| `RALPH_CONFIG` | no | `config.json` | Path to JSON config file (env still wins). |

\*At least one of `COPILOT_GITHUB_TOKEN`, `GH_TOKEN`, or `GITHUB_TOKEN` must be set; the first non-empty in that order is used. Missing required keys cause a panic before the server starts.

### JSON example (fallback file)

```json
{
  "YARALPHO_MONGODB_URI": "mongodb://localhost:27017",
  "YARALPHO_MONGODB_DB": "ralph",
  "YARALPHO_REPO_PATH": "/abs/path/to/repo",
  "YARALPHO_BD_REPO": "/abs/path/to/bd/repo",
  "YARALPHO_PORT": "8080",
  "COPILOT_GITHUB_TOKEN": "ghp_REDACTED_token",
  "GH_TOKEN": "",
  "GITHUB_TOKEN": "",
  "YARALPHO_SLACK_WEBHOOK_URL": "https://hooks.slack.com/services/T000/B000/REDACTED",
  "YARALPHO_MAX_RETRIES": "5"
}
```

Place this file at `config.json` or point `RALPH_CONFIG` to it. Environment variables always take precedence over the JSON values.

## API

Base URL defaults to `http://localhost:8080`.

### System
- `GET /health` – liveness check.
- `GET /version` – build identifier (set via `-ldflags -X ...Version`).

### Agents
- `POST /agent` – create an agent. Body: `{ "name": "worker-1", "runtime": "codex|copilot" }`. Sets status to `idle`.
- `GET /agent` – list agents.
- `GET /agent/{id}` – fetch agent detail.
- `PUT /agent/{id}` – update `name` and `runtime` (only when status `idle`).
- `DELETE /agent/{id}` – delete an idle agent. Busy agents return `409`.

### Batches (repository-scoped)
- `POST /repository/{repoid}/add?items=ISSUE-1,ISSUE-2&session_name=label` – create a pending batch under an existing repository. Returns `batch_id`, `status`, `repository_id`.
- `GET /batches?limit=50` – list recent batches (global view).
- `GET /batches/{id}` – batch detail with embedded runs.
- `GET /batches/{id}/progress` – counts of pending/running/succeeded/failed/stopped for a batch.
- `PUT /repository/{repoid}/batch/{batchid}/restart` – reset the failed item in a failed batch to `pending`, `attempts=0`. Returns `409` if the batch is not failed or `404` if the repo/batch mismatch.

### Runs
- `GET /repository/{repoid}/batch/{batchid}/runs?limit=50` – runs for a batch scoped to its repository.
- `GET /runs/{id}` – run detail including `repository_id` and events (capped by `event_limit`, defaults apply in handler).
- `GET /runs/{id}/events?limit=100` – paged list of session events for a run.
- `GET /runs/{id}/events/live?last_ingested=<rfc3339>` – WebSocket stream of session events with heartbeats. Optional `last_ingested` resumes after a known cursor.

Notes:
- There is no global `/runs` list anymore; runs are accessed via batch-scoped and detail endpoints.
- Repository records must already exist in MongoDB; HTTP CRUD for repositories is not exposed yet.
- Pause/resume statuses exist but HTTP endpoints are not yet wired.
- The legacy "epic" concept has been removed; `parent_ref` in runs is only populated when a tracker indicates an epic but no special API surface remains.

## App UI (/app)

- `/app` renders the embedded UI; `/app?batch=<batch_id>` lists runs for that batch; `/app?run=<run_id>` shows raw events using `/runs/{id}/events`.
- Static assets are served from `/app/static/*` with content types set automatically.

## Status lifecycle quick reference

- **Batch:** `pending` → `in_progress` → (`done` | `failed`) with optional `paused` (skip scheduling) and manual `/restart` for failed batches.
- **Item:** `pending` → `in_progress` → (`done` | `failed`); `attempts` increments on retry.
- **Agent:** `idle` ↔ `busy`; updates/deletes are blocked when `busy`.
- **Run:** `running` → (`succeeded` | `failed` | `stopped`).

These statuses are persisted in Mongo and surfaced verbatim through the API responses.
