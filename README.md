# yaralpho

Ralph Runner is a Go 1.21+ service that ingests work items, classifies them, and drives a Copilot-powered agent loop with storage, tracking, queueing, and Slack notifications.

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
  "COPILOT_GITHUB_TOKEN": "ghp_redacted_token",
  "GH_TOKEN": "",
  "GITHUB_TOKEN": "",
  "YARALPHO_SLACK_WEBHOOK_URL": "https://hooks.slack.com/services/T000/B000/REDACTED"
}
```

Place this file at `config.json` or point `RALPH_CONFIG` to it. Environment variables always take precedence over the JSON values.
