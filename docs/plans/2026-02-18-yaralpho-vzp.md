# Task Run Timeout Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Enforce a configurable per-task run timeout (default 20m) covering execution + verification, stopping stuck sessions and surfacing failures for scheduler retries.

**Architecture:** Add a new config surface `YARALPHO_TASK_RUN_TIMEOUT` with a 20m default, and wrap the worker’s end-to-end task handling in a shared context derived from it so all execution/verification phases inherit a single deadline. On timeout, record runs as timed out, stop the copilot session, free the agent, and bubble an error to scheduler to trigger existing retry/failure flows.

**Tech Stack:** Go 1.22, context timeouts, zap logging, existing consumer/worker + copilot clients, beads tracker config surface, Go tests with fakes.

**Key Decisions:**

- **Single run-level timeout vs per-phase only:** Add `YARALPHO_TASK_RUN_TIMEOUT` so execution + verification share one ceiling; keeps retries predictable and mirrors acceptance criteria.
- **Context origin:** Derive run-timeout context in worker before attempt loop so both phases and retries respect the same deadline; per-phase timeouts remain as inner limits.
- **Session stop handling:** Use existing stop callbacks on timeout to kill copilot sessions and avoid leaked agents; rely on deferred stop + timeout status for durability.
- **Config default:** Keep 20m default to match current exec/verify defaults and acceptance expectation, allowing overrides via env/JSON.

---

## Supporting Documentation

- Go `context.WithTimeout`: https://pkg.go.dev/context#WithTimeout — use to wrap run lifecycle and ensure cancel propagation.
- Current timeouts + config keys: `internal/config/config.go`, `README.md` env table, `config.example.json` — shows existing exec/verify defaults (20m) to align new run timeout.
- Consumer timeout logging helper: `internal/consumer/task_helpers.go` (`withTimeoutMetadata`, `logTimeoutEvent`) — reuse for run-level telemetry.
- Scheduler/worker retry flow: `internal/scheduler/scheduler.go`, `internal/consumer/worker.go` — shows how errors mark attempts and free agents.
- Prior timeout plan/context: `docs/design/scheduler.md` (exec/verify timeout notes) — ensure new run timeout documented alongside existing values.

---

## Tasks

### Task 1: Add run-timeout configuration surface

**Depends on:** None  
**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

**Purpose:** Expose a configurable `YARALPHO_TASK_RUN_TIMEOUT` (default 20m) for whole-task runtime with tested defaults.

**Context to rehydrate:**
- Review current timeout keys and defaults in `internal/config/config.go`.

**Outcome:** New config key exists with 20m default and is covered by config tests.

**How to Verify:**  
Run: `go test ./internal/config -run Timeout -count=1`  
Expected: Tests pass and assert run-timeout default and key inclusion.

**Acceptance Criteria:**
- [ ] Unit test(s): `internal/config/config_test.go` covers run-timeout default and key lists.
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: N/A
- [ ] Outputs match expectations from How to Verify.

**Not In Scope:** Changing exec/verify timeout semantics.

**Gotchas:** Keep loggable/env override key lists aligned.

### Task 2: Update config example and README with run-timeout env

**Depends on:** Task 1  
**Files:**
- Modify: `config.example.json`
- Modify: `README.md`

**Purpose:** Surface the new run-timeout key and default in user-facing configuration examples and env tables.

**Context to rehydrate:**
- Existing timeout entries in README env table and `config.example.json`.

**Outcome:** README and example config list `YARALPHO_TASK_RUN_TIMEOUT` with 20m default and run-level description.

**How to Verify:**  
Run: `grep -n \"YARALPHO_TASK_RUN_TIMEOUT\" README.md config.example.json`  
Expected: Entries present with correct default and description.

**Acceptance Criteria:**
- [ ] Unit test(s): N/A
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: Entries updated as described.
- [ ] Outputs match expectations from How to Verify.

**Not In Scope:** Design doc updates.

**Gotchas:** Keep wording consistent with acceptance criteria.

### Task 3: Document run-timeout behavior in design notes

**Depends on:** Task 2  
**Files:**
- Modify: `docs/design/scheduler.md`

**Purpose:** Capture run-timeout behavior alongside existing exec/verify timeout design notes.

**Context to rehydrate:**
- Timeout section in `docs/design/scheduler.md`.

**Outcome:** Design doc describes run-timeout key, default, and scope across execution + verification.

**How to Verify:**  
Run: `grep -n \"run timeout\" docs/design/scheduler.md`  
Expected: Section describes `YARALPHO_TASK_RUN_TIMEOUT` and behavior.

**Acceptance Criteria:**
- [ ] Unit test(s): N/A
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: Doc updated.
- [ ] Outputs match expectations from How to Verify.

**Not In Scope:** Wider design refactors.

**Gotchas:** Keep language aligned with README.

### Task 4: Apply run-timeout context to worker execution flow

**Depends on:** Task 1  
**Files:**
- Modify: `internal/consumer/worker.go`
- Modify: `internal/consumer/task_helpers.go`

**Purpose:** Enforce run-level timeout across execution + verification (including retries) via a single deadline context with timeout metadata.

**Context to rehydrate:**
- `handleSingleTask` attempt loop and `executeAndVerify` contexts in `worker.go`.
- Timeout helpers in `task_helpers.go`.

**Outcome:** Worker derives run-scoped context from `YARALPHO_TASK_RUN_TIMEOUT`, propagates through attempts, and converts deadline hits into timed-out runs and errors.

**How to Verify:**  
Run: `go test ./internal/consumer -run Timeout -count=1`  
Expected: New timeout tests pass and existing tests remain green.

**Acceptance Criteria:**
- [ ] Unit test(s): Consumer timeout test asserts run-level timeout triggers timed-out run and error propagation.
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: N/A
- [ ] Outputs match expectations from How to Verify.

**Not In Scope:** Changing per-phase timeout values or retry math.

**Gotchas:** Ensure attempts stop when parent context times out; avoid double-cancel by deferring cancel after loops.

### Task 5: Ensure timeout stops sessions and frees agents

**Depends on:** Task 4  
**Files:**
- Modify: `internal/consumer/task_helpers.go`
- Modify: `internal/consumer/worker_test.go`

**Purpose:** Validate timeout path stops copilot sessions, records `TaskRunStatusTimedOut`, notifies appropriately, and surfaces errors so scheduler retry logic engages.

**Context to rehydrate:**
- `executeTask` event loop status handling in `task_helpers.go`.
- Worker tests and fakes in `worker_test.go`.

**Outcome:** Timeout path is covered by tests confirming session stop, timed-out status, and returned error.

**How to Verify:**  
Run: `go test ./internal/consumer -run Timeout -count=1`  
Expected: Timeout-focused test passes and proves session stop/error propagation.

**Acceptance Criteria:**
- [ ] Unit test(s): New worker timeout test covers session stop + status.
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: N/A
- [ ] Outputs match expectations from How to Verify.

**Not In Scope:** Scheduler logic changes (relies on existing error handling).

**Gotchas:** Keep fakes deterministic to avoid flakiness.

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes                                     |
| --------------------------- | ------ | ----------------------------------------- |
| Complete                    | ✓      | Covers config surface, worker timeout flow, tests, docs. |
| Accurate                    | ✓      | Paths verified in repo for listed files. |
| Commands valid              | ✓      | go test targets exist; grep commands scoped to files. |
| YAGNI                       | ✓      | Only adds run-timeout feature and docs; no extra scope. |
| Minimal                     | ✓      | Tasks split by file budget; no parallel doc/code mix. |
| Not over-engineered         | ✓      | Uses existing timeout helpers and contexts. |
| Key Decisions documented    | ✓      | Four decisions captured with rationale. |
| Supporting docs present     | ✓      | Links/notes included for context and APIs. |
| Context sections present    | ✓      | Each task lists Purpose, Context, Outcome, Verify. |
| Budgets respected           | ✓      | ≤2 production files per task; single outcome each. |
| Outcome & Verify present    | ✓      | Each task lists Outcome + How to Verify. |
| Acceptance Criteria present | ✓      | Acceptance checklists per task. |
| Rehydration context present | ✓      | Context bullets provided where dependent. |

### Rule-of-Five Passes

| Pass        | Changes Made                             |
| ----------- | ---------------------------------------- |
| Draft       | Structured tasks by file budget and dependencies. |
| Correctness | Commands and paths validated against repo. |
| Clarity     | Simplified task outcomes/verification wording. |
| Edge Cases  | Highlighted agent release and timeout error propagation. |
| Excellence  | Polished wording and consistency across tasks. |
