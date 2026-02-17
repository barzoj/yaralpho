# Propagate Timeout to Scheduler Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Ensure scheduler treats worker timeouts like failures by releasing agents, incrementing attempts, and moving items out of in_progress using cleanup contexts.

**Architecture:** Keep scheduler logic single-responsibility by isolating cleanup from work execution; use uncanceled contexts for persistence on timeout while retaining existing retry/failure semantics; expand tests to cover timeout cleanup. No new dependencies or workflows introduced.

**Tech Stack:** Go 1.21+, zap logging, testify assertions, existing scheduler/consumer/storage packages.

**Key Decisions:**

- **Cleanup context:** Use `context.WithoutCancel` when persisting status updates after worker returns to ensure cleanup succeeds even if the work context timed out — avoids leaving agents/items stuck.
- **Error semantics:** Continue returning the worker error (including timeout) so upstream orchestration can observe failures while persisting retry state — aligns with current failure flow.
- **Test surface:** Add scheduler-level regression tests that simulate context timeout and storage honoring ctx cancellation — ensures attempts increment and resources release under timeout conditions.

---

## Supporting Documentation

- Go `context.WithoutCancel` (https://pkg.go.dev/context#WithoutCancel): allows reuse of values without cancellation for cleanup flows.
- Go context cancellation patterns (https://go.dev/blog/context-and-cancellation): guidance on cleanup after timeouts.
- Zap structured logging (https://pkg.go.dev/go.uber.org/zap): ensure consistent fields for retries/attempts.
- Testify `require`/`assert` (https://pkg.go.dev/github.com/stretchr/testify/require): used for deterministic scheduler expectations.

---

## Tasks

### Task 1: Capture timeout cleanup gap with a regression test

**Depends on:** None  
**Files:**

- Modify: `internal/scheduler/scheduler_test.go` (new test + helper storage honoring ctx cancellation)

**Purpose:** Reproduce timeout-induced cancellation preventing cleanup so we have a failing test before code changes.

**Context to rehydrate:**

- Review scheduler Tick flow in `internal/scheduler/scheduler.go` (error handling + updates).
- Look at existing fakeStorage patterns in the same test file.

**Outcome:** A scheduler test fails pre-fix, showing agent/item remain in_progress when worker returns `context.DeadlineExceeded` and storage respects ctx cancellation.

**How to Verify:**  
Run: `go test ./internal/scheduler -run TestSchedulerTick_TimeoutCleansUp -count=1`  
Expected (pre-fix): FAIL indicating cleanup did not persist (attempts/agent/item stuck).

**Acceptance Criteria:**

- [ ] Unit test added covering timeout path with ctx cancellation-aware storage.
- [ ] Test asserts attempts incremented and statuses reset/failed per max retries expectation.
- [ ] Test currently fails against main branch without code changes.
- [ ] Logging fields remain consistent (batch/agent/attempt).
- [ ] No production code changed in this task.

**Not In Scope:**

- Altering scheduler tick cadence or draining semantics.
- Changing worker timeout configuration logic.

### Task 2: Ensure scheduler cleanup persists after timeouts

**Depends on:** Task 1  
**Files:**

- Modify: `internal/scheduler/scheduler.go` (cleanup context and logging)
- Modify: `internal/scheduler/scheduler_test.go` (ensure regression passes)

**Purpose:** Use an uncanceled context for cleanup so agents are released, attempts increment, and items leave in_progress even when worker timeouts cancel the original context.

**Context to rehydrate:**

- Task 1 test expectations.
- Current error-handling branch in Tick (attempt increment, status transitions, agent release).

**Outcome:** Scheduler uses `context.WithoutCancel` (or equivalent) for persistence after worker completion; timeout errors follow the same failure flow and leave no stuck resources.

**How to Verify:**  
Run: `go test ./internal/scheduler -run TestSchedulerTick_TimeoutCleansUp -count=1`  
Expected: PASS with attempts incremented, agent idle, item pending/failed per retry logic; other scheduler tests still pass.

**Acceptance Criteria:**

- [ ] Cleanup persists even when `ctx.Err()` is `context.DeadlineExceeded`.
- [ ] Agent status returns to idle after timeout.
- [ ] Batch/item status follows existing retry/failure thresholds.
- [ ] Attempts counter increments on timeout.
- [ ] All scheduler tests pass.

**Not In Scope:**

- Introducing new retry policies or max retries configuration changes.
- Modifying worker behavior beyond scheduler integration.

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes                                     |
| --------------------------- | ------ | ----------------------------------------- |
| Complete                    | ✓      | Covers regression + fix + verification.   |
| Accurate                    | ✓      | Paths verified in repo.                   |
| Commands valid              | ✓      | go test command scoped to scheduler pkg.  |
| YAGNI                       | ✓      | Only timeout cleanup addressed.           |
| Minimal                     | ✓      | Two tasks, minimal files.                 |
| Not over-engineered         | ✓      | Reuse existing patterns; no new deps.     |
| Key Decisions documented    | ✓      | Three decisions listed.                   |
| Supporting docs present     | ✓      | Context links provided.                   |
| Context sections present    | ✓      | Purpose/Context included per task.        |
| Budgets respected           | ✓      | Each task ≤2 files, single outcome.       |
| Outcome & Verify present    | ✓      | Outcome/How to Verify per task.           |
| Acceptance Criteria present | ✓      | Checklist per task.                       |
| Rehydration context present | ✓      | Provided where dependent.                 |

### Rule‑of‑Five Passes

| Pass        | Changes Made                             |
| ----------- | ---------------------------------------- |
| Draft       | Initial structure with 2 tasks.          |
| Correctness | Paths/commands checked; outcomes aligned |
| Clarity     | Simplified scope/not-in-scope notes.     |
| Edge Cases  | Added cleanup context emphasis.          |
| Excellence  | Polished wording and verification table. |
