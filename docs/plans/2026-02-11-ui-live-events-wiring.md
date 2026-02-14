# UI live events wiring Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Wire the run detail UI to the live WebSocket stream so new session events appear without page refresh while preserving the initial static render.

**Architecture:** Reuse the existing run detail view: fetch the run and its events, derive the latest ingested timestamp, then open a single WebSocket to `/runs/{id}/events/live?last_ingested=<cursor>` to receive envelopes. Maintain an in-memory event list keyed by ingest time to dedupe and re-render via the existing `renderEventsList` path. Surface connection lifecycle via status/console logging and close the socket on navigation/unload to avoid leaks.

**Tech Stack:** Vanilla JS DOM, browser WebSocket API, Go HTTP server with gorilla/websocket (backend already implemented).

**Key Decisions:**

- **Transport:** Use the existing `/runs/{id}/events/live` WebSocket endpoint with `last_ingested` cursor — matches server contract and avoids inventing new APIs like SSE.
- **Deduplication:** Key events by `ingested_at` (server ordering) while keeping the full payload — prevents duplicate renders from backfill plus stream without changing backend schema.
- **Lifecycle:** Create one socket per run view and close on navigation/unload with console/status logging — avoids multiple concurrent sockets and provides observable telemetry without adding new logging infra.

---

## Supporting Documentation

- `internal/app/run_events_live_handler.go`: WebSocket envelope schema (`type` event/error/heartbeat, `cursor`, `event`), `last_ingested` RFC3339Nano parsing, heartbeat interval, dedupe key, error handling.
- `internal/app/run_events_handler.go`: Existing REST fetch for events with limit/truncation metadata consumed by the UI.
- `internal/app/ui/app.js`: Run view flow (`renderRunView`, `renderEventsList`, `fetchJSON`, status helpers, rendering pipeline for events).
- MDN WebSocket API: https://developer.mozilla.org/en-US/docs/Web/API/WebSocket — browser API shape for connect/close/message/error handling.
- Gorilla WebSocket docs: https://pkg.go.dev/github.com/gorilla/websocket — server framing and close/heartbeat behaviors to align client expectations.

---

## Workplan

### Task 1: Capture cursor for live stream bootstrap

**Depends on:** None  
**Files:**
- Modify: `internal/app/ui/app.js` (run view event fetch path)

**Purpose:** Preserve the latest `ingested_at` timestamp from the initial events fetch to seed the WebSocket `last_ingested` cursor and prevent duplicate backfill.

**Context to rehydrate:**
- `renderRunView` and `renderEventsList` in `internal/app/ui/app.js`
- Envelope parsing rules in `internal/app/run_events_live_handler.go`

**Outcome:** The run view stores the newest ingested timestamp (or empty when none) immediately after initial events load for later WebSocket connection.

**How to Verify:** Load `/app?batch=<batch>&run=<run>` with existing events; confirm (via console log to be added) that `last_ingested` equals the newest `ingested_at` value or is blank when no events exist.

**Acceptance Criteria:**
- [ ] Unit test(s): N/A (no JS test harness)
- [ ] Integration test(s): N/A (not applicable)
- [ ] Manual or E2E check: Console shows derived cursor matching latest event timestamp or blank when none
- [ ] Outputs match expectations from How to Verify
- [ ] Interface and implementation separation: Cursor derivation isolated from socket wiring
- [ ] DAL layer tasks do not leak database-specific types: N/A

**Not In Scope:** Changing backend cursor parsing or event storage ordering.

**Gotchas:** `ingested_at` must be used as-is (RFC3339Nano) to satisfy server validation of `last_ingested`.

### Task 2: Establish WebSocket connection and merge live events

**Depends on:** Task 1  
**Files:**
- Modify: `internal/app/ui/app.js` (add WebSocket wiring and event merge logic)

**Purpose:** Open a WebSocket after initial fetch, handle event/heartbeat/error envelopes, dedupe by ingest time, and re-render using the existing event rendering pipeline without requiring page refresh.

**Context to rehydrate:**
- Envelope types and cursor rules in `internal/app/run_events_live_handler.go`
- UI rendering helpers (`renderEventsList`, `filterRenderableEvents`) in `internal/app/ui/app.js`

**Outcome:** When the run view is open, live events received over WebSocket append in-order without duplicates and immediately appear in the events list while preserving the initial static render.

**How to Verify:** With the server running and a run producing events, open `/app?batch=<batch>&run=<run>`; emit a new session event (e.g., trigger task execution) and observe the UI increment the shown event count without duplicate renders; heartbeats should not change the list.

**Acceptance Criteria:**
- [ ] Unit test(s): N/A (no JS test harness)
- [ ] Integration test(s): N/A (manual stream validation)
- [ ] Manual or E2E check: New events appear live without refresh and without duplication; heartbeats do not render
- [ ] Outputs match expectations from How to Verify
- [ ] Interface and implementation separation: WebSocket handler encapsulated from render functions
- [ ] DAL layer tasks do not leak database-specific types: N/A

**Not In Scope:** Auto-reconnect/backoff strategies or UI controls for socket state.

**Gotchas:** Maintain a single open socket per view; ensure message `type` guards so only envelopeTypeEvent mutates state.

### Task 3: Telemetry and lifecycle cleanup for live stream

**Depends on:** Task 2  
**Files:**
- Modify: `internal/app/ui/app.js` (socket lifecycle and logging helpers)

**Purpose:** Provide observable connect/disconnect/error logs and cleanly close the WebSocket when navigating away or on error to prevent leaks and dangling subscriptions.

**Context to rehydrate:**
- Connection/close/error handlers in the new WebSocket wiring (Task 2)
- Browser `beforeunload`/view-switch logic in `renderRunView`

**Outcome:** The UI logs connection lifecycle (open, error, close) with run/batch context and disposes the socket on navigation or page unload; status messaging surfaces failures without breaking the static render.

**How to Verify:** Open a run view and observe console logs for connect; close the tab or navigate back and see a close log without errors; simulate server close/error (e.g., stop backend or block upgrade) and confirm error log plus graceful UI fallback.

**Acceptance Criteria:**
- [ ] Unit test(s): N/A (no JS test harness)
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: Console shows connect/disconnect/error logs; socket closes on navigation/unload without thrown errors
- [ ] Outputs match expectations from How to Verify
- [ ] Interface and implementation separation: Logging helpers isolated from render logic
- [ ] DAL layer tasks do not leak database-specific types: N/A

**Not In Scope:** Persistent telemetry sinks beyond console/status messaging.

**Gotchas:** Ensure close handlers do not race with message handlers; avoid retry loops that could re-open sockets during teardown.

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes                                     |
| --------------------------- | ------ | ----------------------------------------- |
| Complete                    | ✓      | Covers cursor capture, WS wiring, logging/cleanup |
| Accurate                    | ✓      | Paths and endpoint `/runs/{id}/events/live` verified in code |
| Commands valid              | ✓      | Manual verification steps align with available UI/server |
| YAGNI                       | ✓      | No new APIs or reconnect logic added |
| Minimal                     | ✓      | Three single-file tasks within budgets |
| Not over-engineered         | ✓      | Reuses existing render path; no new frameworks |
| Key Decisions documented    | ✓      | Three decisions with rationale |
| Supporting docs present     | ✓      | Linked server/JS files and official docs |
| Context sections present    | ✓      | Each task includes Purpose and Context to rehydrate |
| Budgets respected           | ✓      | Each task touches one file and single outcome |
| Outcome & Verify present    | ✓      | Outcome and How to Verify per task |
| Acceptance Criteria present | ✓      | Checklists included with N/A noted |
| Rehydration context present | ✓      | Added for tasks depending on prior work |

### Rule‑of‑Five Passes

| Pass        | Changes Made                             |
| ----------- | ---------------------------------------- |
| Draft       | Initial structure with 3 tasks and scopes |
| Correctness | Verified endpoints/paths and acceptance alignment |
| Clarity     | Simplified outcomes/verification wording |
| Edge Cases  | Added dedupe/single-socket gotchas and cursor validation note |
| Excellence  | Polished key decisions, supporting docs, and budgets alignment |
