# Task Execution Timeout Enforcement Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Enforce a configurable execution timeout so worker runs stop Copilot sessions promptly, record timed-out task runs, and surface errors for scheduler retries.

**Architecture:** Wrap the execution phase in a context derived from `YARALPHO_TASK_EXEC_TIMEOUT`, propagate deadline cancellations down to the Copilot session runner, and persist a dedicated timed-out run status while emitting stop signals to the client. Error returns should bubble to the scheduler without masking, keeping retry logic intact.

**Tech Stack:** Go, Copilot client integration, Mongo-backed storage, Zap logging, Go `context`/`time` utilities.

**Key Decisions:**

- **Timeout scope:** Apply timeout only around the execution task (pre-verification) to avoid hiding verification time while still bounding Copilot sessions.
- **Run status:** Introduce a `timed_out` task run status instead of overloading `stopped` to make observability and retries explicit.
- **Error propagation:** Return `context.DeadlineExceeded` to caller so scheduler retry logic can treat timeouts as failures eligible for retry.

---

## Supporting Documentation

- Go `context.WithTimeout`: https://pkg.go.dev/context#WithTimeout — canonical pattern for deadline-bound contexts and `Done` channel semantics.
- Go `time.ParseDuration`: https://pkg.go.dev/time#ParseDuration — parsing `TaskExecTimeoutKey` values like `20m` safely.
- Copilot client stop hook: existing `StartSession` contract returns a stop func; ensure it is invoked on deadline to free resources.

---

### Task 1: Add timed-out task run status

**Depends on:** None  
**Files:**

- Modify: `internal/storage/models.go` (add new status constant, doc string)
- Modify: `internal/consumer/worker_test.go` (align fakes/expectations if status strings referenced)

**Purpose:** Introduce an explicit `timed_out` status to distinguish deadline cancellations from voluntary stops or generic failures.

**Context to rehydrate:** Review `storage.TaskRunStatus` definitions and any tests asserting status enums (see `internal/consumer/worker_test.go`).

**Outcome:** Storage layer exposes `TaskRunStatusTimedOut` constant usable by consumer code and tests compile against it.

**How to Verify:**
Run: `go test ./internal/storage -run TestDummy || true` (no tests expected; compile check)  
Expected: Package compiles with new constant.

**Acceptance Criteria:**

- [ ] New `TaskRunStatusTimedOut` constant defined with comment.
- [ ] Build references compile without unused/undefined identifier errors.
- [ ] No other statuses removed or renamed.
- [ ] No storage interface changes needed beyond status constant.

**Not In Scope:** Database migrations or API changes.

**Gotchas:** None expected; ensure string value matches naming conventions.

---

### Task 2: Apply execution timeout context and stop sessions on deadline

**Depends on:** Task 1  
**Files:**

- Modify: `internal/consumer/worker.go` (wrap execution phase with timeout from config)
- Modify: `internal/consumer/task_helpers.go` (treat deadline as timed-out status and stop Copilot session on cancel path)

**Purpose:** Enforce configured timeout around execution, stopping Copilot sessions when the deadline hits and tagging runs as timed out.

**Context to rehydrate:** Check `TaskExecTimeoutKey` defaults in `internal/config/config.go`; inspect `executeTask` loop in `task_helpers.go` for context cancellation handling; see `executeAndVerify` call chain in `worker.go`.

**Outcome:** Execution uses a derived timeout context; on deadline exceeded, the stop hook fires, run status is `timed_out`, and error bubbles up.

**How to Verify:**
Run: `go test ./internal/consumer -run TestExecuteTask_Timeout -count=1`  
Expected: New timeout-focused test passes, asserting stop invoked, status set to `timed_out`, and error returned.

**Acceptance Criteria:**

- [ ] Execution phase uses `context.WithTimeout` based on `TaskExecTimeoutKey` value.
- [ ] On deadline exceeded, Copilot stop function called and run status persisted as `timed_out`.
- [ ] Error returned to caller equals `context.DeadlineExceeded` (or wraps it).
- [ ] Batch status handling remains consistent (no regressions to happy path).

**Not In Scope:** Verification phase timeouts or scheduler retry policy changes.

**Gotchas:** Avoid swallowing original context; ensure cancel func is deferred to prevent leaks.

---

### Task 3: Add timeout test coverage

**Depends on:** Tasks 1, 2  
**Files:**

- Modify: `internal/consumer/worker_test.go` (add timeout test using short context and blocking event stream)
- Modify: `internal/consumer/task_helpers.go` (only if small test helper tweaks required; otherwise keep to two files budget)

**Purpose:** Prove timeout behavior by simulating a hung Copilot stream and asserting stop, status, and error propagation.

**Context to rehydrate:** Use existing fakes (`fakeCopilot`, `fakeStorage`, `closedChan`) in `worker_test.go` to craft a blocking channel; reference new `TaskRunStatusTimedOut`.

**Outcome:** Tests fail without timeout handling and pass with new logic, guarding against regressions.

**How to Verify:**
Run: `go test ./internal/consumer -run TestExecuteTask_Timeout -count=1`  
Expected: Timeout test passes alongside existing suite.

**Acceptance Criteria:**

- [ ] New test exercises deadline, stop call, and status persistence.
- [ ] Test is deterministic (uses controlled timers/channels, no sleeps longer than necessary).
- [ ] Existing consumer tests remain green.

**Not In Scope:** Broader integration or e2e tests.

**Gotchas:** Ensure blocking channel does not leak goroutines; close channels after use in test to avoid deadlocks.

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes                                     |
| --------------------------- | ------ | ----------------------------------------- |
| Complete                    | ✓      | Covers timeout context, stop hook, status, tests |
| Accurate                    | ✓      | File paths verified via repo inspection    |
| Commands valid              | ✓      | go test commands scoped to consumer/stage  |
| YAGNI                       | ✓      | Only adds timeout handling and test        |
| Minimal                     | ✓      | Three small tasks, limited files           |
| Not over-engineered         | ✓      | Uses existing context and config           |
| Key Decisions documented    | ✓      | Three decisions captured                   |
| Supporting docs present     | ✓      | Links to Go context/time docs              |
| Context sections present    | ✓      | Purpose/Not In Scope/Gotchas included      |
| Budgets respected           | ✓      | ≤2 files per task, single outcome          |
| Outcome & Verify present    | ✓      | Each task states outcome and verification  |
| Acceptance Criteria present | ✓      | Checklists included per task               |
| Rehydration context present | ✓      | Added where dependencies exist             |

### Rule-of-Five Passes

| Pass        | Changes Made                             |
| ----------- | ---------------------------------------- |
| Draft       | Initial structure with 3 tasks and docs  |
| Correctness | Verified paths/commands align to codebase |
| Clarity     | Simplified wording and outcomes          |
| Edge Cases  | Called out deadline/error propagation     |
| Excellence  | Added gotchas and concise verifications   |

