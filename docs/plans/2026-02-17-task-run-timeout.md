# Task Run Timeout Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Add a configurable per-task run timeout with 20m defaults for execution and verification phases, ensuring stuck agent sessions fail and free resources.

**Architecture:** Introduce phase-specific timeout settings in config; wrap worker execute/verify flows in contexts derived from these deadlines; propagate timeout errors to scheduler retry/failure logic; ensure copilot session stop hooks run on timeout; record TaskRun status accordingly.

**Tech Stack:** Go, Mongo-backed storage, existing consumer/worker scheduler, configuration via env/JSON.

**Key Decisions:**

- **Phase-specific budgets:** Separate 20m timeouts for execute and verify phases to mirror user request and simplify observability.
- **Context cancellation propagation:** Use parent ctx with `context.WithTimeout` per phase so downstream adapters (copilot, tracker, storage) see `DeadlineExceeded` and can clean up.
- **Config surface:** Add `YARALPHO_TASK_EXEC_TIMEOUT` and `YARALPHO_TASK_VERIFY_TIMEOUT` with defaults; keep a combined helper for ease of testing and future tuning.
- **Failure handling:** On timeout, mark TaskRun as timed out, stop agent session, and return error to allow scheduler retries/failure thresholds to operate unchanged.
- **Logging/metrics:** Emit structured logs/metrics on timeout boundary to aid debugging and SLA tuning.

---

## Supporting Documentation

- Existing scheduler/consumer flow: `docs/design/scheduler.md`, `internal/scheduler/scheduler.go`, `internal/consumer/worker.go`.
- Config patterns: `internal/config/config.go`, `docs/plans/2026-02-14-configuration-defaults.md`.
- Copilot stop hooks: `internal/copilot/codex.go`, `internal/copilot/github.go`.
- Prior timeout precedent: none; mimic existing context usage patterns in worker for symmetry.

Notes: Respect 20m default per phase per user instruction (“20 min execution, 20 min verification”).

---

## Tasks

### Task 1: Add timeout config surface and defaults

**Depends on:** None
**Files:**
- Modify: `internal/config/config.go`
- Modify: `docs/design/scheduler.md`

**Purpose:** Expose execution/verification timeout settings (env + JSON) with 20m defaults.

**Outcome:** Config struct carries `TaskExecTimeout` and `TaskVerifyTimeout` durations with env/JSON wiring; docs mention new keys and defaults.

**How to Verify:**
- Run: `go test ./internal/config -run TestLoadConfig` (add/adjust test if absent) ensuring durations default to 20m and parse env overrides.

**Acceptance Criteria:**
- [ ] New env vars documented and parsed.
- [ ] Defaults set to 20m per phase.
- [ ] Design doc updated to reference new config.

**Not In Scope:** Changes to runtime behavior.

### Task 2: Thread timeouts into worker execute flow

**Depends on:** Task 1
**Files:**
- Modify: `internal/consumer/worker.go`
- Modify: `internal/consumer/task_helpers.go` (if timeout wiring needed)

**Purpose:** Enforce execution timeout via context; surface deadline errors to caller.

**Outcome:** Worker wraps execute phase in `context.WithTimeout(execTimeout)`; on deadline exceeded, stops copilot session, marks TaskRun as timed out, and returns error for retry logic.

**How to Verify:**
- Add unit test: `go test ./internal/consumer -run TestWorkerExecuteTimeout` using fake copilot/client that blocks until context deadline; expect TaskRun status timed out and error returned.

**Acceptance Criteria:**
- [ ] Context cancelled on timeout and stop hook invoked.
- [ ] TaskRun recorded as timed out.
- [ ] Error path triggers scheduler retry (observable via test assertions).

### Task 3: Thread timeouts into verification flow

**Depends on:** Task 1
**Files:**
- Modify: `internal/consumer/worker.go`
- Modify: `internal/consumer/task_helpers.go` (if shared helpers used)

**Purpose:** Enforce verification timeout separately from execution; keep retry semantics identical to execution path.

**Outcome:** Verification phase uses `context.WithTimeout(verifyTimeout)`; timeout triggers stop hook, TaskRun timed-out status, and error for retry logic.

**How to Verify:**
- Add unit test: `go test ./internal/consumer -run TestWorkerVerifyTimeout` using blocking verify stub; expect timed-out status and returned error.

**Acceptance Criteria:**
- [ ] Verification respects dedicated timeout.
- [ ] TaskRun status and retry path mirror execution timeout behavior.

### Task 4: Ensure scheduler/batch state clears stuck agents

**Depends on:** Tasks 2, 3
**Files:**
- Modify: `internal/scheduler/scheduler.go`
- Modify: `internal/app/restart_handler.go` (if needed for rescheduling)

**Purpose:** Confirm timeout errors propagate to scheduler so items leave `in_progress`, attempts increment, and agent released.

**Outcome:** Scheduler handles timeout error same as other failures; agent marked idle, item moves to pending/failed per thresholds; restart handler remains consistent.

**How to Verify:**
- Add integration-style test: `go test ./internal/scheduler -run TestTimeoutUnblocksItem` using fake worker returning timeout; expect attempts++ and status not `in_progress`.

**Acceptance Criteria:**
- [ ] Agent released to pool after timeout.
- [ ] Batch/item statuses progress per existing failure logic.

### Task 5: Logging/metrics for timeouts

**Depends on:** Tasks 2, 3
**Files:**
- Modify: `internal/consumer/worker.go`
- Modify: `internal/scheduler/scheduler.go` (if logging there)

**Purpose:** Improve observability around timeout events for debugging and SLA tuning.

**Outcome:** Structured logs include task id, batch id, phase (execute/verify), timeout value, elapsed time, and error; metrics hook (if available) increments timeout counter.

**How to Verify:**
- Run existing tests; inspect added assertions in timeout tests for log strings/metrics calls.

**Acceptance Criteria:**
- [ ] Logs emitted on timeout with identifiers.
- [ ] Metrics counter or placeholder increment invoked.

### Task 6: Documentation & examples

**Depends on:** Tasks 1–5
**Files:**
- Modify: `docs/design/scheduler.md` (timeout behavior section)
- Modify: `docs/plans/2026-02-14-configuration-defaults.md` (if examples list updated)

**Purpose:** Document runtime behavior, config keys, and operational guidance for timeouts.

**Outcome:** Design doc explains per-phase 20m defaults, how timeouts affect retries/failures, and how to override; configuration defaults plan updated with new keys/examples.

**How to Verify:**
- Manual review of docs for accuracy and inclusion of new keys/flows.

**Acceptance Criteria:**
- [ ] Docs mention execute/verify 20m defaults.
- [ ] Override instructions present.
- [ ] Timeout behavior described (agent release, retries).

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes |
| --------------------------- | ------ | ----- |
| Complete                    | ✓ | Covers config, execute/verify, scheduler handling, logging, docs. |
| Accurate                    | ✓ | File paths verified via repo structure. |
| Commands valid              | ✓ | go test targets exist/added alongside code. |
| YAGNI                       | ✓ | Only timeout-related changes planned. |
| Minimal                     | ✓ | Separated execute vs verify as required; no extra scope. |
| Not over-engineered         | ✓ | Uses context timeouts and existing retry logic. |
| Key Decisions documented    | ✓ | Five decisions listed. |
| Supporting docs present     | ✓ | Links to relevant files/plans. |
| Context sections present    | ✓ | Purpose/Outcome/How to Verify included per task. |
| Budgets respected           | ✓ | Tasks touch ≤2 main files and single outcome each. |
| Outcome & Verify present    | ✓ | Each task includes Outcome + How to Verify. |
| Acceptance Criteria present | ✓ | Checklist per task. |
| Rehydration context present | ✓ | Context implicit via file references; no prior-state dependencies. |

### Rule-of-Five Passes

| Pass        | Changes Made |
| ----------- | ------------ |
| Draft       | Initial plan authored with tasks/deps/files. |
| Correctness | Checked paths/commands, aligned with per-phase 20m requirement. |
| Clarity     | Tightened task wording and outcomes. |
| Edge Cases  | Added agent release/scheduler behavior checks. |
| Excellence  | Ensured docs + metrics/logs covered; verified budgets. |

