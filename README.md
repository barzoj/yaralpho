# yaralpho

Ralph Runner is a Go 1.21+ service that ingests work items, classifies them, and drives a Copilot-powered agent loop with storage, tracking, queueing, and Slack notifications.

## How it works

- **Queue + single consumer:** `/add` enqueues work into an in-memory FIFO queue; one consumer pulls items sequentially, so only one task runs at a time and order is preserved.
- **Classification:** the consumer asks the tracker (beads CLI) whether an item is an epic; for epics it picks the first ready child to run.
- **Copilot sessions:** every task run opens a fresh GitHub Copilot SDK session, auto-approves prompts/permissions, and streams raw events to storage.
- **Storage + progress:** Mongo stores batches, runs, and session events; batch progress counts pending/running/done from storage.
- **Logging & notifications:** zap logging is used end-to-end with request IDs from middleware. Slack notifications are sent when a webhook is set; when unset the Slack notifier is a noop.
- **Agent etiquette:** follow AGENTS guides (root `AGENTS.md` + `internal/app/AGENTS.md`) when extending handlers or wiring.

## Configuration

The config loader is **environment-first** with an optional JSON fallback:

- Env vars override values from the JSON file.
- JSON is loaded from `config.json` by default; override the path with the `RALPH_CONFIG` env var (or pass a path to the loader).
- Blank/whitespace values are ignored.
- On startup the loader **panics** if any required keys are missing.
- Token precedence: `COPILOT_GITHUB_TOKEN` → `GH_TOKEN` → `GITHUB_TOKEN`.

### Variables

| Key | Required? | Default | Description |
| --- | --- | --- | --- |
| `YARALPHO_MONGODB_URI` | yes | — | Mongo connection string. |
| `YARALPHO_MONGODB_DB` | yes | — | Mongo database name. |
| `YARALPHO_REPO_PATH` | yes | — | Filesystem path to the repo the runner operates in. |
| `YARALPHO_BD_REPO` | yes | — | Path to the beads (`bd`) repository used for tracking. |
| `YARALPHO_PORT` | no | `8080` | HTTP listen port. |
| `YARALPHO_SLACK_WEBHOOK_URL` | no | — | Slack webhook for notifications. |
| `COPILOT_GITHUB_TOKEN` | yes* | — | Primary token for GitHub Copilot SDK. |
| `GH_TOKEN` | no* | — | Fallback token if `COPILOT_GITHUB_TOKEN` is unset. |
| `GITHUB_TOKEN` | no* | — | Secondary fallback if both above are unset. |
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
  "YARALPHO_SLACK_WEBHOOK_URL": "https://hooks.slack.com/services/T000/B000/REDACTED"
}
```

Place this file at `config.json` or point `RALPH_CONFIG` to it. Environment variables always take precedence over the JSON values.

## API

Base URL defaults to `http://localhost:8080`.

### POST /add
- Query: `items` (comma-separated issue refs), `session_name` (optional label, defaults to `default`).
- Example:
```bash
curl -X POST "http://localhost:8080/add?items=PROJ-1,PROJ-2&session_name=demo"
```
Response: `{"batch_id":"batch-1700000000000000000"}`

### GET /batches
- Optional `limit` (default 50, max 200).
```bash
curl "http://localhost:8080/batches?limit=20"
```

### GET /batches/{id}
```bash
curl "http://localhost:8080/batches/batch-1700000000000000000"
```

### GET /batches/{id}/progress
```bash
curl "http://localhost:8080/batches/batch-1700000000000000000/progress"
```
Returns counts of pending/running/done tasks for the batch.

### GET /runs
- Optional `batch_id` to filter; `limit` (default 50, max 200) slices the returned list.
```bash
curl "http://localhost:8080/runs?batch_id=batch-1700000000000000000&limit=25"
```

### GET /runs/{id}
- Optional `event_limit` caps returned session events (default 50, max 200). Response includes `events_truncated` and `session_event_cap` to signal the cap used.
```bash
curl "http://localhost:8080/runs/run-123?event_limit=100"
```

### GET /runs/{id}/events
- Optional `limit` caps returned session events (default 50, max 200). Response includes `events_truncated` and `event_limit_used` to signal whether the list was truncated.
```bash
curl "http://localhost:8080/runs/run-123/events?limit=100"
```

## App UI (/app)

- Serve the embedded HTML/JS UI at `/app`; no build step is required.
- `/app` lists batches; `/app?batch=<batch_id>` lists runs for that batch; `/app?run=<run_id>` shows raw events using `/runs/{id}/events`.
- Static assets are served from `/app/static/*` with content types set automatically.
