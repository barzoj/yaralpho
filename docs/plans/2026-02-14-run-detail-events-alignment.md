# Run Detail & Event Handlers Alignment (Task yaralpho-hgo.17)

**Goal**: Keep `/runs/{id}`, `/runs/{id}/events`, and `/runs/{id}/events/live` functional on the repository-aware schema. Responses should surface repository context and avoid any epic references while continuing to stream or list events by session.

**Current state**: Detail handler returns a run plus capped events; events handler returns events only; live handler streams session events with heartbeat envelopes. Runs now include `repository_id`; epics are being removed. Events are keyed by `session_id` and currently provide batch/run IDs but no repository metadata.

**Decisions** (recommended):
- Keep route shapes unchanged to avoid client breakage; attach repository context in payloads where we already return metadata (detail + events responses).
- Preserve event fetching by `session_id` but carry the run’s `repository_id` in HTTP responses so callers can navigate back to repository-scoped views without relying on epics.
- Maintain existing event limits/truncation behavior; no shape changes to websocket envelopes beyond the embedded event.

**Implementation sketch**:
- `runDetailHandler`: validate `run_id`, fetch run, fetch session events; include `repository_id` in the JSON response (alongside the existing run object) and ensure no code references epics.
- `runEventsHandler`: fetch run then events; include `repository_id` (and reuse run metadata) in response together with truncation flags.
- Add focused handler tests using `httptest.NewRecorder` (no real sockets) to assert repository context presence and epic-free payloads.

**Testing plan**: Unit-test new handlers with in-memory fakes; avoid websocket/network reliance. Target `go test ./internal/app -run TestRunDetail*` and `-run TestRunEvents*` when environment permits. Log inability to run network-bound websocket tests in constrained environments.
