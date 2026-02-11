# Purpose
Browser UI for viewing batches/runs and live session events without page refresh.

# Exposed Surfaces
- Static assets served from `/app` (index.html, styles) and UI logic in `app.js`.
- Live event stream helpers: `live_merge.js` for cursor-based dedupe/ordering and `live_reconnect.js` for reconnect/backoff status.
- Consumes REST endpoints `/batches`, `/runs`, `/runs/{id}`, `/runs/{id}/events` and WebSocket `/runs/{id}/events/live?last_ingested=…`.

# Notes for Agents
- Reconnect policy: exponential backoff with jitter (`base 1s`, `max 15s`, `jitter 0.2`, `max 5 attempts`); status banner shows disconnect/retry reason, and exhaustion offers a Refresh action.
- Client always seeds state from REST events then streams via WebSocket; envelopes carry `cursor` so merges stay gapless.
- Keep status messaging and reconnect UX aligned with `live_reconnect.js` tests when adjusting timing or limits.
