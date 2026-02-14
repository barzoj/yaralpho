# Consumer Bus Publish Integration Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Publish each persisted Copilot session event to the in-memory bus so live subscribers receive updates during task execution.

**Architecture:** Inject the streaming bus into the consumer worker and execution path; publish events immediately after storage persistence while keeping run lifecycle unchanged; handle bus publish errors with structured logging and notifier signaling without blocking event ingestion.

**Tech Stack:** Go, zap logger, internal bus package, consumer worker/helpers, storage interfaces.

**Key Decisions:**

- **Bus injection scope:** Inject `bus.Bus` into consumer worker and execution task to keep publish logic close to event ingestion while avoiding global state — follows existing dependency pattern and eases testing with fakes.
- **Publish failure handling:** Log and emit notifier event on publish error but do not stop the run loop — avoids disrupting persistence while still surfacing telemetry.
- **Buffer semantics:** Use existing bus configuration defaults (buffered channels, slow-consumer policies) without additional consumer-specific tuning — leverages previously validated bus behavior and keeps this change focused on integration.

---

## Supporting Documentation

- Internal bus design: `internal/bus/bus.go`, `internal/bus/memory_bus.go` — publish/subscribe contracts, slow-consumer policies, error types.
- Consumer run loop and event persistence: `internal/consumer/task_helpers.go`, `internal/consumer/execution_task.go` — where session events are saved and notified.
- Storage models: `internal/storage/models.go` — `SessionEvent` fields required for bus publish.
- Testing patterns: `internal/consumer/execution_task_test.go`, `internal/consumer/config_stub_test.go` — existing use of fakes and dependency injection for consumer components.

## Work Plan

### Task 1: Inject bus into consumer execution path

**Depends on:** None  
**Files:**
- Modify: `internal/consumer/worker.go` (wire bus dependency into worker struct/constructor)
- Modify: `internal/consumer/execution_task.go` (propagate bus into execution task struct/ctor and exec function signature)

**Purpose:** Ensure consumer components receive a bus dependency so execution logic can publish events without global lookups.

**Context to rehydrate:**
- Review worker constructor defaults in `internal/consumer/worker.go`.
- Check execution task wiring in `internal/consumer/execution_task.go`.

**Outcome:** Worker and execution task accept a `bus.Bus` dependency that is initialized with sane defaults and exposed to execution helpers.

**How to Verify:**  
Run: `go test ./internal/consumer/...`  
Expected: Tests compile and pass with updated signatures (may be failing until Task 3 adds new tests).

**Acceptance Criteria:**
- [ ] Worker struct and `NewWorker` include a bus field initialized safely.
- [ ] ExecutionTask stores bus reference and passes it to underlying execute function.
- [ ] Existing tests compile with the new dependency injected.
- [ ] No additional global state introduced.
- [ ] Interface/implementation separation maintained; bus passed via dependency injection.

**Not In Scope:** Bus publish logic itself (covered in Task 2).

**Gotchas:** Keep constructor defaults aligned with existing patterns (zap.NewNop, notify.Noop); ensure nil bus handling is explicitly guarded.

### Task 2: Publish persisted session events to bus

**Depends on:** Task 1  
**Files:**
- Modify: `internal/consumer/task_helpers.go`

**Purpose:** Emit each persisted session event to the bus immediately after storage insertion, surfacing publish failures deterministically.

**Context to rehydrate:**
- Event loop in `executeTask` within `task_helpers.go`.
- Bus publish semantics in `internal/bus/memory_bus.go`.

**Outcome:** After storing a session event, the consumer publishes it to the bus with batch/run/session identifiers; publish errors are logged and forwarded to notifier without stopping the run loop.

**How to Verify:**  
Run: `go test ./internal/consumer/...`  
Expected: Event publish path exercised by tests from Task 3; no regressions in existing behavior.

**Acceptance Criteria:**
- [ ] Publish invoked for every persisted event with correct session/run/batch IDs.
- [ ] Publish errors logged with context (session/run/batch) and forwarded to notifier.
- [ ] Publish failures do not interrupt storage persistence loop.
- [ ] Idle/stop behavior of executeTask remains unchanged.

**Not In Scope:** Websocket handler consumption or backfill logic.

**Gotchas:** Avoid double-closing channels; respect context cancellation semantics already present in the loop.

### Task 3: Add integration-style test coverage for bus publish

**Depends on:** Task 2  
**Files:**
- Modify: `internal/consumer/worker_test.go`
- Modify: `internal/consumer/config_stub_test.go` (reuse config/test helpers as needed)

**Purpose:** Verify that bus publish is invoked with persisted events and that publish errors are surfaced via logs/notifier without breaking runs.

**Context to rehydrate:**
- Existing execution task tests for structured output.
- Bus error types in `internal/bus/bus.go`.

**Outcome:** Tests assert publish fan-out call count and error handling, preventing regression of the new integration.

**How to Verify:**  
Run: `go test ./internal/consumer/... -run TestExecutionTaskBus` (or similar new test name)  
Expected: Tests pass and fail if publish is not called or errors aren’t handled as expected.

**Acceptance Criteria:**
- [ ] Fake bus captures published events and validates identifiers.
- [ ] Test covers error path (bus returns error) and asserts notifier/log behavior.
- [ ] No flakes: tests deterministic without timing dependencies.
- [ ] Documentation/comments in test explain scenario briefly.

**Not In Scope:** Websocket client behavior or UI updates.

**Gotchas:** Keep test fakes minimal; avoid race conditions by using buffered channels or sync primitives.

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes |
| --------------------------- | ------ | ----- |
| Complete                    | ✓ | Covers bus injection, publish, and tests |
| Accurate                    | ✓ | Paths verified against current tree |
| Commands valid              | ✓ | `go test ./internal/consumer/...` exists |
| YAGNI                       | ✓ | Scoped to publish integration only |
| Minimal                     | ✓ | Three focused tasks within budgets |
| Not over-engineered         | ✓ | Reuses existing bus defaults and patterns |
| Key Decisions documented    | ✓ | Three decisions captured |
| Supporting docs present     | ✓ | Links to bus, consumer, storage files |
| Context sections present    | ✓ | Each task includes purpose/context |
| Budgets respected           | ✓ | ≤2 prod files per task, single outcome |
| Outcome & Verify present    | ✓ | Each task includes outcome and command |
| Acceptance Criteria present | ✓ | Checklists per task |
| Rehydration context present | ✓ | Context steps listed where needed |

### Rule-of-Five Passes

| Pass        | Changes Made |
| ----------- | ------------ |
| Draft       | Confirmed task decomposition and budgets; no structural changes |
| Correctness | Verified file paths, dependencies, and test commands |
| Clarity     | Tightened outcomes and verification phrasing |
| Edge Cases  | Noted publish-error handling to avoid run interruption |
| Excellence  | Polished wording and consistency across tasks |
