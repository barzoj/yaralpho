# WebSocket event stream handler Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Deliver a WebSocket endpoint that streams live session events for a run with validation, close codes, and cleanup.

**Architecture:** Reuse the in-memory session event bus already used by the consumer; expose it to the HTTP layer and stream events over `gorilla/websocket` from a run’s session ID with optional `last_ingested` filtering. Validation happens before upgrade; connections manage ping/pong and ensure subscription cancellation on close.

**Tech Stack:** Go 1.21, `github.com/gorilla/mux`, `github.com/gorilla/websocket`, Zap logging, existing in-memory bus.

**Key Decisions:**

- **WebSocket library:** Keep `gorilla/websocket` (already in go.sum) — mature, simple upgrade handling and ping/pong helpers versus hand-rolled hijacking.
- **Bus sharing:** Expose the consumer’s in-memory session event bus to the app and reuse it — avoids duplicating publish paths and preserves ordering.
- **Session lookup:** Use run → session lookup via storage before upgrade — ensures authorization/validation by existing run ownership.
- **Close codes:** Use standard RFC6455 codes (1008 policy violation for bad params, 1011 internal error) — clear semantics for clients over custom codes.
- **Last-ingested filter:** Apply in-stream filtering only to new events (no replay) — bus has no replay; keeps scope minimal while honoring parameter intent.

---

## Supporting Documentation

- Gorilla WebSocket server usage and ping/pong: https://pkg.go.dev/github.com/gorilla/websocket#hdr-Control_Messages — upgrade patterns and close handling.
- RFC 6455 close codes reference: https://www.rfc-editor.org/rfc/rfc6455.html#section-7.4.1 — choose 1008 for invalid params, 1011 for server errors.
- Gorilla mux path vars: https://pkg.go.dev/github.com/gorilla/mux#Vars — run ID extraction.
- Zap structured logging: https://pkg.go.dev/go.uber.org/zap — use fields for batch/run/session IDs and close reasons.

---

### Task 1: Expose shared session event bus

**Depends on:** None  
**Files:**

- Modify: `internal/consumer/task_helpers.go` (export bus setter)
- Modify: `internal/consumer/worker.go` (use injected bus if provided)

**Purpose:** Allow the HTTP layer to reuse the same session event bus that the consumer publishes to.

**Context to rehydrate:**

- Review `sessionEventBus` global usage in `internal/consumer/task_helpers.go`.
- Worker construction path in `internal/consumer/worker.go`.

**Outcome:** The consumer package provides an exported setter for the session event bus and honors a pre-set bus without recreating it.

**How to Verify:**  
Run: `go test ./internal/consumer -run TestExecuteTask_PublishesSessionEventsToBus -v`  
Expected: Test passes and bus publishes without panic.

**Acceptance Criteria:**

- [ ] Exported bus setter exists and is used before worker construction.
- [ ] Worker does not override a pre-configured bus.
- [ ] Unit test covering publish path still passes.
- [ ] No new global state beyond the shared bus.

**Not In Scope:** HTTP handlers and WebSocket logic.

**Gotchas:** Ensure zero-value logger handling stays intact when bus is injected.

### Task 2: Wire bus into App construction

**Depends on:** Task 1  
**Files:**

- Modify: `internal/app/app.go`

**Purpose:** Make the App hold a session event bus instance shared with the consumer for later WebSocket subscriptions.

**Context to rehydrate:**

- App struct and `Build/New` functions in `internal/app/app.go`.
- Current route registration (no changes yet).

**Outcome:** App initializes a session event bus once, passes it to the consumer setup, and stores it for HTTP handlers.

**How to Verify:**  
Run: `go test ./internal/app -run TestHandlers_AddAndList -v`  
Expected: Tests pass and app construction succeeds.

**Acceptance Criteria:**

- [ ] App struct holds a bus reference.
- [ ] Build/New set the consumer’s bus to the shared instance before worker creation.
- [ ] Existing handler tests continue to pass.

**Not In Scope:** WebSocket handler behavior.

**Gotchas:** Avoid nil-pointer issues if config injection fails; keep defaults stable.

### Task 3: Implement WebSocket handler and routing

**Depends on:** Task 2  
**Files:**

- Create: `internal/app/run_events_ws_handler.go`
- Modify: `internal/app/routes.go`

**Purpose:** Add a `/runs/{id}/events/live` WebSocket endpoint that validates input, upgrades, subscribes to the session event bus, filters by `last_ingested`, and streams events with proper close handling.

**Context to rehydrate:**

- Existing run detail/events handlers for validation patterns.
- Bus subscription semantics and `storage.SessionEvent` shape.
- Gorilla WebSocket upgrade defaults.

**Outcome:** Clients can connect to `/runs/{id}/events/live` (optionally `last_ingested=RFC3339`) and receive live session events; invalid params close with 1008; internal failures close with 1011; logging includes batch/run/session and close reason; subscription is cancelled on disconnect.

**How to Verify:**  
Run: `go test ./internal/app -run TestRunEventsLive -v`  
Expected: Tests cover successful upgrade/stream, invalid param close, and cleanup paths.

**Acceptance Criteria:**

- [ ] Route is registered and reachable.
- [ ] Missing/invalid IDs or bad timestamps result in 1008 closes with message.
- [ ] Successful upgrade attaches to bus and streams events in order, applying `last_ingested` filter to new events.
- [ ] Subscription cancel is invoked on close/cancel.
- [ ] Zap logs include batch_id, run_id, session_id, close_reason.

**Not In Scope:** Historical replay beyond filtering live events; envelope formatting beyond raw stored events.

**Gotchas:** WebSocket upgrader must not allow cross-site hijacking; ensure default origin check is acceptable for server-side use or add safe CheckOrigin if needed.

### Task 4: Add WebSocket handler tests

**Depends on:** Task 3  
**Files:**

- Create: `internal/app/run_events_ws_handler_test.go`

**Purpose:** Prove upgrade error handling, parameter validation, event streaming, and cleanup behaviors.

**Context to rehydrate:**

- Test fakes in `internal/app/handlers_test.go` (storage, queue).
- WebSocket dialer patterns in `net/http/httptest`.

**Outcome:** Tests assert success path (upgrade + streamed events + cancel on close) and failure path (bad params trigger 1008) with deterministic logs.

**How to Verify:**  
Run: `go test ./internal/app -run TestRunEventsLive -v`  
Expected: All new tests pass.

**Acceptance Criteria:**

- [ ] Test covers successful upgrade and event receive.
- [ ] Test covers invalid last_ingested or missing run id leading to 1008 close.
- [ ] Test asserts subscription cleanup.
- [ ] No flakiness from timing; use buffered channels and timeouts.

**Not In Scope:** Integration with UI; manual E2E.

**Gotchas:** Ensure WebSocket client closes to trigger server cleanup; avoid leaking goroutines.

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes |
| --------------------------- | ------ | ----- |
| Complete                    | ✗ | Pending checklist pass |
| Accurate                    | ✗ | Paths to be revalidated |
| Commands valid              | ✗ | To be confirmed |
| YAGNI                       | ✗ | Pending review |
| Minimal                     | ✗ | Pending review |
| Not over-engineered         | ✗ | Pending review |
| Key Decisions documented    | ✓ | 5 captured |
| Supporting docs present     | ✓ | Links listed |
| Context sections present    | ✓ | Purpose/Context/Outcome present |
| Budgets respected           | ✗ | To confirm per task |
| Outcome & Verify present    | ✓ | Provided per task |
| Acceptance Criteria present | ✓ | Included per task |
| Rehydration context present | ✓ | Included per dependent tasks |

### Rule-of-Five Passes

| Pass        | Changes Made |
| ----------- | ------------ |
| Draft       | Pending |
| Correctness | Pending |
| Clarity     | Pending |
| Edge Cases  | Pending |
| Excellence  | Pending |

