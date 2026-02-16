# yaralpho

Ralph Runner is a Go 1.25+ service that schedules repository-scoped batches of tasks across runtime agents (Codex or GitHub Copilot). Batches are processed sequentially within each repository while multiple repositories can run in parallel when idle agents are available. MongoDB stores repositories, batches, runs, and raw session events; a lightweight HTTP API plus a small UI surface observability.

## Agent providers

Providers are now selected per agent record via the `runtime` field (`codex|copilot`). The service always starts with the Codex client available; each scheduled item runs on whichever idle agent the scheduler selects, using that agent’s runtime.

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

| Key                                 | Required? | Default       | Description                                                               |
| ----------------------------------- | --------- | ------------- | ------------------------------------------------------------------------- |
| `YARALPHO_MONGODB_URI`              | yes       | —             | Mongo connection string.                                                  |
| `YARALPHO_MONGODB_DB`               | yes       | —             | Mongo database name.                                                      |
| `YARALPHO_REPO_PATH`                | yes       | —             | Filesystem path to the repo the runner operates in.                       |
| `YARALPHO_PORT`                     | no        | `8080`        | HTTP listen port.                                                         |
| `YARALPHO_SLACK_WEBHOOK_URL`        | no        | —             | Slack webhook for notifications (noop when unset).                        |
| `YARALPHO_MAX_RETRIES`              | no        | `5`           | Max attempts per batch item before the batch is marked `failed`.          |
| `YARALPHO_SCHEDULER_INTERVAL`       | no        | `10s`         | Interval between scheduler ticks that claim the next eligible batch item. |
| `YARALPHO_RESTART_WAIT_TIMEOUT`     | no        | `20m`         | Maximum time `/restart?wait=true` will block while draining active runs.  |
| `YARALPHO_EXECUTION_TASK_PROMPT`    | no        | built-in      | Prompt template for execution agents (used by worker).                    |
| `YARALPHO_VERIFICATION_TASK_PROMPT` | no        | built-in      | Prompt template for verification agents.                                  |
| `RALPH_CONFIG`                      | no        | `config.json` | Path to JSON config file (env still wins).                                |

GitHub Copilot still requires an access token, but the SDK reads it directly from `COPILOT_GITHUB_TOKEN` (or `GH_TOKEN` / `GITHUB_TOKEN`) without going through the config loader.

### JSON example (fallback file)

```json
{
	"YARALPHO_MONGODB_URI": "mongodb://localhost:27017",
	"YARALPHO_MONGODB_DB": "ralph",
	"YARALPHO_REPO_PATH": "/abs/path/to/repo",
	"YARALPHO_PORT": "8080",
	"YARALPHO_SLACK_WEBHOOK_URL": "https://hooks.slack.com/services/T000/B000/REDACTED",
	"YARALPHO_MAX_RETRIES": "5"
}
```

`bd` commands now execute in the repository path recorded on each `Repository` object, so tracking is per-repo without a global beads path.

Place this file at `config.json` or point `RALPH_CONFIG` to it. Environment variables always take precedence over the JSON values.

## API

Base URL defaults to `http://localhost:8080`.

### System

- `GET /health` – liveness check.
- `GET /version` – build identifier (set via `-ldflags -X ...Version`).
- `POST /restart?wait=true|false` – puts the scheduler into draining mode. `wait=true` blocks (up to `YARALPHO_RESTART_WAIT_TIMEOUT`) until all active runs finish; otherwise returns `202 Accepted` immediately with the active run count.

### Repositories

- `POST /repository` – create a repository. Body: `{ "name": "repo-1", "path": "/abs/path" }` (path must be absolute). 409 on name/path conflict.
- `GET /repository` – list repositories.
- `GET /repository/{id}` – repository detail.
- `PUT /repository/{id}` – update name/path (absolute path required). 409 on duplicate.
- `DELETE /repository/{id}` – remove an idle repository (409 if any active batches exist).

### Agents

- `POST /agent` – create an agent. Body: `{ "name": "worker-1", "runtime": "codex|copilot" }`. Sets status to `idle`.
- `GET /agent` – list agents.
- `GET /agent/{id}` – fetch agent detail.
- `PUT /agent/{id}` – update `name` and `runtime` (only when status `idle`).
- `DELETE /agent/{id}` – delete an idle agent. Busy agents return `409`.

### Batches (repository-scoped)

- `POST /repository/{repoid}/add` – create a pending batch under an existing repository from JSON body `{ "items": ["ISSUE-1","ISSUE-2"], "session_name": "label" }`. Returns `batch_id`, `status`, `repository_id`.
- `GET /batches?limit=50` – list recent batches (global view).
- `GET /repository/{repoid}/batches?status=pending|in_progress|paused|done|failed&limit=50` – batches scoped to a repository with optional status filter; defaults to `pending|in_progress|paused|done|failed`.
- `GET /batches/{id}` – batch detail with embedded runs.
- `GET /batches/{id}/progress` – counts of pending/running/succeeded/failed/stopped for a batch.
- `PUT /repository/{repoid}/batch/{batchid}/pause` – mark a batch paused (skips new items; in-flight work completes). 409 if done/failed/already paused.
- `PUT /repository/{repoid}/batch/{batchid}/resume` – return a paused batch to `pending` so the scheduler can pick it up again.
- `PUT /repository/{repoid}/batch/{batchid}/restart` – reset the failed item in a failed batch to `pending`, `attempts=0`. Returns `409` if the batch is not failed or `404` if the repo/batch mismatch.

### Runs

- `GET /repository/{repoid}/batch/{batchid}/runs?limit=50` – runs for a batch scoped to its repository (no global `/runs` list).
- `GET /runs/{id}?event_limit=10000` – run detail plus capped events slice; `event_limit` caps returned events (default `10000`, max `100000`).
- `GET /runs/{id}/events?limit=10000` – paged list of session events for a run (default `10000`, max `100000`); response includes `events_truncated` and `event_limit_used`.
- `GET /runs/{id}/events/live?last_ingested=<rfc3339>` – WebSocket stream of session events with heartbeats. Optional `last_ingested` resumes after a known cursor.

Notes:

- There is no global `/runs` list; use batch-scoped runs + run detail.
- Repository CRUD is exposed; repository paths must be absolute and deletion is blocked while batches are active.
- Pause/resume endpoints are available; paused batches skip new items but allow in-flight work to finish.
- `/restart?wait=true` drains the scheduler for deploy/CI workflows; caller blocks until idle or `YARALPHO_RESTART_WAIT_TIMEOUT` elapses.
- The legacy "epic" concept has been removed; `parent_ref` in runs is only populated when a tracker indicates an epic but no special API surface remains.

### Quickstart (curl)

Create at least one agent before scheduling work; agents are global and the scheduler assigns them to repository batches automatically.

```bash
# 1) Create an agent (global; scheduler assigns it to repos)
curl -s -X POST http://localhost:8080/agent \
  -H 'Content-Type: application/json' \
  -d '{"name":"worker-1","runtime":"copilot"}' | tee /tmp/agent.json

# 2) Create a repository (absolute path required)
curl -s -X POST http://localhost:8080/repository \
  -H 'Content-Type: application/json' \
  -d '{"name":"demo","path":"/abs/path/to/repo"}' | tee /tmp/repo.json

# 3) Add a batch under that repository
repo_id=$(jq -r '.repository_id' /tmp/repo.json)
curl -s -X POST http://localhost:8080/repository/$repo_id/add \
  -H 'Content-Type: application/json' \
  -d '{"items":["ISSUE-1","ISSUE-2"],"session_name":"demo-run"}' | tee /tmp/batch.json

# 4) Watch progress and runs
batch_id=$(jq -r '.batch_id' /tmp/batch.json)
curl -s "http://localhost:8080/batches/$batch_id/progress"
curl -s "http://localhost:8080/repository/$repo_id/batch/$batch_id/runs?limit=50"

# 5) Pause/resume if needed
curl -s -X PUT "http://localhost:8080/repository/$repo_id/batch/$batch_id/pause"
curl -s -X PUT "http://localhost:8080/repository/$repo_id/batch/$batch_id/resume"

# 6) CI-safe drain before restart/deploy (blocks until idle or timeout)
curl -s -X POST "http://localhost:8080/restart?wait=true"
```

## App UI (/app)

- `/app` renders the embedded UI; `/app?batch=<batch_id>` lists runs for that batch; `/app?run=<run_id>` shows raw events using `/runs/{id}/events`.
- Static assets are served from `/app/static/*` with content types set automatically.

## Status lifecycle quick reference

- **Batch:** `pending` → `in_progress` → (`done` | `failed`) with optional `paused` (skip scheduling) and manual `/restart` for failed batches.
- **Item:** `pending` → `in_progress` → (`done` | `failed`); `attempts` increments on retry.
- **Agent:** `idle` ↔ `busy`; updates/deletes are blocked when `busy`.
- **Run:** `running` → (`succeeded` | `failed` | `stopped`).

These statuses are persisted in Mongo and surfaced verbatim through the API responses.
