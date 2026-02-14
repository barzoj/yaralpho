# Scheduler Logging & Observability Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Add structured, traceable logs for scheduler decisions and batch pause/resume/restart flows so operations can follow claims, retries, failures, and drain state without leaking secrets.

**Architecture:** Keep existing scheduler and HTTP handler shapes; add structured zap fields at each decision/transition point. Provide lightweight helpers for common fields to avoid duplication. Tests rely on zaptest/observer to assert log events without external sinks.

**Tech Stack:** Go 1.21, zap logger (`go.uber.org/zap`), zaptest/observer for log capture, existing in-memory fakes in tests.

**Key Decisions:**
- **Log levels:** Info for state changes (drain start/stop, pause/resume, claim success, worker success/fail), Debug for skips/eligibility decisions, Error only on unexpected persistence failures — balances signal vs. noise in production.
- **Field set:** Always include `batch_id`, `item_index`, `agent_id`, `repository_id`, `attempt`, `max_retries` where applicable to tie logs to domain entities.
- **Testing approach:** Use `zaptest/observer` with fake storage/worker to assert emitted logs instead of inspecting stdout — deterministic and fast.

---

## Supporting Documentation
- zap structured logging fields: `zap.String`, `zap.Int`, `zap.Error` — https://pkg.go.dev/go.uber.org/zap
- zaptest observer for capturing logs in tests — https://pkg.go.dev/go.uber.org/zap/zaptest/observer
- Existing batch pause/restart handlers for context: `internal/app/batch_pause_handlers.go`, `internal/app/batch_restart_handler.go`

---

### Task 1: Add scheduler log field helpers

**Depends on:** None  
**Files:**  
- Modify: `internal/scheduler/scheduler.go`  
- Test: `internal/scheduler/scheduler_test.go`

**Purpose:** Centralize common zap fields (batch, item index, agent, repo, attempts, draining) to keep subsequent instrumentation consistent and concise.

**Outcome:** Helper functions available in scheduler package returning reusable `[]zap.Field` bundles.

**How to Verify:**  
Run: `go test ./internal/scheduler -run TestLogFieldHelpers -v`  
Expected: Test passes verifying helper outputs.

**Acceptance Criteria:**  
- [ ] Helpers cover batch/item/agent/repo/attempt/max_retries/draining contexts.  
- [ ] Helpers avoid nil deref when inputs are nil or indexes missing.  
- [ ] Tests assert field keys/values match expectations.  
- [ ] No new production dependencies beyond zap.

**Not In Scope:** Emitting any new logs; only helper creation.  
**Gotchas:** Keep helpers unexported unless required by other packages.

Steps: write helper(s), add unit test with sample structs, ensure zero allocations beyond slices.

---

### Task 2: Instrument scheduler tick decision points

**Depends on:** Task 1  
**Files:**  
- Modify: `internal/scheduler/scheduler.go`  
- Test: `internal/scheduler/scheduler_test.go`

**Purpose:** Emit structured logs for drain state, no agents, paused batches, in-progress batches, empty batches, item claims, worker success/failure, retries exhausted, agent idle transitions.

**Outcome:** Each branch in `Tick` logs at the agreed level with identifiers; active run counters unchanged.

**How to Verify:**  
Run: `go test ./internal/scheduler -run TestSchedulerLogging -v`  
Expected: Observer captures logs for claim, retry, skip paths with correct levels and fields.

**Acceptance Criteria:**  
- [ ] Debug logs for draining, no idle agents, paused/empty/in-progress batches, no eligible batches.  
- [ ] Info log for dispatch/claim and worker success; Warn for worker error with remaining retries; Error (or Warn) when retries exhausted marking failed.  
- [ ] Logs include batch_id, agent_id, repository_id, item_index, attempt, max_retries as applicable.  
- [ ] Tests cover success path, retry path, exhausted path, and skip branches.  
- [ ] No sensitive data (task input payloads) emitted.

**Not In Scope:** Changing scheduling logic or max retry behavior.  
**Gotchas:** Ensure deferred counters/logs still run when errors return early.

Steps: apply helpers to Tick, inject repository_id from batch, add observer-based tests using fake storage/worker.

---

### Task 3: Log drain start/stop controls

**Depends on:** Task 2  
**Files:**  
- Modify: `internal/scheduler/scheduler.go`  
- Test: `internal/scheduler/scheduler_test.go`

**Purpose:** Record when draining mode is toggled via `SetDraining`, `Start`, or `Stop`, including current active count.

**Outcome:** Info-level log when draining enabled/disabled with active counts; tests confirm emission.

**How to Verify:**  
Run: `go test ./internal/scheduler -run TestDrainingLogs -v`  
Expected: Observer sees draining toggle logs with fields `draining` and `active`.

**Acceptance Criteria:**  
- [ ] Info log on SetDraining true/false, Stop triggers draining log.  
- [ ] Fields: draining (bool), active_count (int).  
- [ ] No behavior change to draining flag semantics.  
- [ ] Tests assert logs and no double-logging on no-op toggles.

**Not In Scope:** Implementing periodic Start ticker.  
**Gotchas:** Avoid noisy logs when SetDraining called repeatedly with same value.

---

### Task 4: Instrument batch pause/resume handlers

**Depends on:** Task 2  
**Files:**  
- Modify: `internal/app/batch_pause_handlers.go`  
- Test: `internal/app/batch_pause_handlers_test.go`

**Purpose:** Emit Info logs for pause/resume actions with batch_id and repository_id, and Warn on repository mismatch or missing batch.

**Outcome:** HTTP handlers log structured events for pause/resume endpoints; tests verify log presence and fields.

**How to Verify:**  
Run: `go test ./internal/app -run TestBatchPauseLogging -v`  
Expected: Tests assert observer records expected messages for pause and resume paths.

**Acceptance Criteria:**  
- [ ] Info log on successful pause/resume with batch_id, repository_id, actor (if available).  
- [ ] Warn log on repo mismatch or missing batch.  
- [ ] No secrets or request bodies logged.  
- [ ] Tests cover success and mismatch paths.

**Not In Scope:** Changing HTTP response codes.  
**Gotchas:** Ensure existing logger plumbing accessible in handlers.

---

### Task 5: Instrument batch restart handler

**Depends on:** Task 2  
**Files:**  
- Modify: `internal/app/batch_restart_handler.go`  
- Test: `internal/app/batch_restart_handler_test.go`

**Purpose:** Log restart attempts and outcomes (not found, repo mismatch, restart accepted) with identifiers.

**Outcome:** Structured Info/Warn logs around restart lifecycle visible in tests.

**How to Verify:**  
Run: `go test ./internal/app -run TestBatchRestartLogging -v`  
Expected: Observer captures restart success and mismatch logs with fields.

**Acceptance Criteria:**  
- [ ] Info log on restart accepted including batch_id, repository_id.  
- [ ] Warn on repo mismatch or missing batch.  
- [ ] Tests assert fields and levels.  
- [ ] No payload data logged.

**Not In Scope:** Changing restart semantics or scheduler integration.  
**Gotchas:** Reuse existing handler logger pattern for consistency.

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes |
| --------------------------- | ------ | ----- |
| Complete                    | ✓ | All required logging points and handlers covered by 5 tasks |
| Accurate                    | ✓ | File paths verified via ripgrep; scheduler/app handler files exist |
| Commands valid              | ✓ | go test package targets confirmed present |
| YAGNI                       | ✓ | Only logging/observability work included; no behavior changes |
| Minimal                     | ✓ | Tasks split by file ownership; ≤2 prod files each |
| Not over-engineered         | ✓ | No new deps; helpers confined to scheduler package |
| Key Decisions documented    | ✓ | Three decisions listed |
| Supporting docs present     | ✓ | zap/zaptest links |
| Context sections present    | ✓ | Tasks include Purpose/Not In Scope |
| Budgets respected           | ✓ | ≤2 prod files per task |
| Outcome & Verify present    | ✓ | Each task has Outcome/How to Verify |
| Acceptance Criteria present | ✓ | Each task lists checklist |
| Rehydration context present | ✓ | Context via file lists and package scoping |

### Rule-of-Five Passes

| Pass        | Changes Made |
| ----------- | ------------ |
| Draft       | Initial draft created |
| Correctness | Verified paths/commands, trimmed scope to logging only |
| Clarity     | Ensured Outcomes/Verify are explicit, levels clarified |
| Edge Cases  | Called out nil/duplicate draining toggles, no secrets in logs |
| Excellence  | Final polish for consistency and brevity |
