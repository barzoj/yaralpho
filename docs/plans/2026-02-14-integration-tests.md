# Integration Tests Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Add deterministic integration tests that exercise the HTTP API and scheduler end-to-end using faked dependencies, covering happy path, retries, pause/resume, and batch restart flows.

**Architecture:** Build a dedicated `test/integration` package with a reusable harness that assembles an `app.App` using in-memory fakes for storage, tracker, notifier, copilot client, and worker. Drive the scheduler synchronously via `Tick` calls with a shortened interval, asserting HTTP responses and storage state transitions per scenario.

**Tech Stack:** Go 1.22 testing, `net/http/httptest` for API calls, in-memory fakes implementing internal interfaces, Zap test logger.

**Key Decisions:**

- **Test location:** Place integration suite in `test/integration` (separate package) to keep concerns isolated and avoid circular imports.
- **Dependency isolation:** Use purpose-built in-memory fakes for storage/tracker/notifier/copilot/worker instead of Mongo or real Copilot to keep runs fast and deterministic.
- **Scheduler control:** Invoke `scheduler.Tick` directly with a configurable, short interval rather than relying on background goroutines and sleeps to minimize test latency and flakiness.
- **State assertions:** Validate both HTTP responses and underlying storage state (batches, agents, runs) to ensure scheduler + API wiring correctness, not just surface outputs.

---

## Supporting Documentation

- `internal/app/app.go`, `scheduler_config.go`, `routes.go` — wiring and scheduler option derivation.
- `internal/app/batch_handlers.go`, `batch_restart_handler.go`, `batch_pause_handlers.go`, `repository_handlers.go` — HTTP behaviors exercised by tests.
- `internal/scheduler/scheduler.go`, `scheduler_test.go` — scheduler contract, fakes patterns, logging fields.
- `internal/storage/interfaces.go`, `internal/consumer/worker.go` — interfaces the fakes must satisfy.
- Go `httptest` package docs (std lib) — request/response testing patterns.

---

## Tasks

### Task 1: Create integration test package scaffold

**Depends on:** None
**Files:**
- Create: `test/integration/go.mod` (if needed for module path reuse) or package doc
- Create: `test/integration/doc.go`

**Purpose:** Establish the `test/integration` package boundary and shared imports so integration tests compile separately from application packages.

**Context to rehydrate:** Inspect existing module path in `go.mod` to mirror for nested package.

**Outcome:** An empty-but-compiling integration package exists and is discoverable by `go test ./test/integration/...`.

**How to Verify:**
Run: `go test ./test/integration/... -run TestDoesNotExist`
Expected: PASS with “no tests to run” and no import errors.

**Acceptance Criteria:**
- [ ] Package builds without modifying production code
- [ ] go test command above succeeds
- [ ] No dependency on real Mongo or external services

**Not In Scope:** Writing any actual tests.


### Task 2: Add deterministic fakes for integration harness

**Depends on:** Task 1
**Files:**
- Create: `test/integration/fakes.go`

**Purpose:** Provide in-memory implementations of storage, tracker, notifier, copilot client, and worker to drive scheduler behavior deterministically.

**Context to rehydrate:** Review interfaces in `internal/storage/interfaces.go`, `internal/tracker/interface.go`, `internal/notify/interfaces.go`, `internal/copilot/interfaces.go`, `internal/consumer/worker.go`.

**Outcome:** Fakes expose minimal state hooks (e.g., preset batches/agents, controllable worker outcomes) used by tests without external dependencies.

**How to Verify:**
Run: `go test ./test/integration/... -run TestFakesCompile`
Expected: PASS for a placeholder compilation test confirming fakes satisfy interfaces.

**Acceptance Criteria:**
- [ ] Fakes implement required interfaces without pulling real packages (Mongo, Slack, GitHub)
- [ ] Worker fake can be configured to succeed or fail with counter of attempts
- [ ] Storage fake tracks batch/item/agent statuses and run logs in-memory

**Not In Scope:** Scheduler invocation or HTTP routing.


### Task 3: Build reusable integration harness

**Depends on:** Task 2
**Files:**
- Create: `test/integration/harness.go`
- Modify: `test/integration/doc.go` (add helpers export comment)

**Purpose:** Assemble an `app.App` with fakes, short scheduler interval, and httptest server helpers to create repositories, agents, batches, and to invoke `Tick` deterministically.

**Context to rehydrate:** `internal/app/app.go`, `scheduler_config.go`, `routes.go`, and fakes from Task 2.

**Outcome:** Helper functions such as `newTestApp(t *testing.T, opts harnessOptions)` and `tickUntil` encapsulate setup/teardown and scheduler stepping.

**How to Verify:**
Run: `go test ./test/integration/... -run TestHarnessBoots -v`
Expected: Harness creates app, exposes HTTP client, and a single `Tick` advances without panic.

**Acceptance Criteria:**
- [ ] Harness overrides scheduler interval to <100ms without sleeping longer than needed
- [ ] Provides helper to register idle agent and seed batch items
- [ ] Cleans up httptest server and restores globals if modified


### Task 4: Test repository + task submission → batch pending

**Depends on:** Task 3
**Files:**
- Create: `test/integration/integration_test.go` (scenario tests)

**Purpose:** Validate that creating a repository and posting a batch via HTTP yields a `pending` batch with correctly initialized items.

**Context to rehydrate:** HTTP routes in `repository_handlers.go` and `batch_handlers.go`; harness helpers.

**Outcome:** A test like `TestIntegration_CreateBatchStartsPending` asserts HTTP 201 responses and storage state reflects pending batch/items.

**How to Verify:**
Run: `go test ./test/integration/... -run TestIntegration_CreateBatchStartsPending -v`
Expected: PASS; batch status pending, items pending with zero attempts.

**Acceptance Criteria:**
- [ ] HTTP 201 for repository and batch creation
- [ ] Batch stored with `BatchStatusPending`
- [ ] Items trimmed and non-empty, Attempts == 0


### Task 5: Test happy path processing to completion

**Depends on:** Task 4
**Files:**
- Modify: `test/integration/integration_test.go`

**Purpose:** Ensure scheduler claims pending work, marks items `in_progress` then `done`, and returns agent to idle when worker succeeds.

**Context to rehydrate:** Scheduler options defaults, worker fake success mode, storage status transitions.

**Outcome:** `TestIntegration_ProcessBatchSucceeds` drives `Tick` loops until done and asserts batch status `completed`, all items `done`, agent `idle`, and run history recorded.

**How to Verify:**
Run: `go test ./test/integration/... -run TestIntegration_ProcessBatchSucceeds -v`
Expected: PASS within <2s; no sleeps longer than scheduler interval.

**Acceptance Criteria:**
- [ ] Batch transitions pending → in_progress → completed
- [ ] Agent status returns to idle after processing
- [ ] At least one task run recorded per item with success flag


### Task 6: Test retry exhaustion marks batch failed

**Depends on:** Task 5
**Files:**
- Modify: `test/integration/integration_test.go`

**Purpose:** Validate failed items are retried up to `MaxRetries`, then batch status becomes `failed` and agent returns to idle.

**Context to rehydrate:** `scheduler.Options.MaxRetries`, worker fake failure mode, storage run attempt tracking.

**Outcome:** `TestIntegration_RetryExhaustionFailsBatch` configures worker to fail, ticks `MaxRetries+1` times, and asserts item `failed`, batch `failed`, attempts count matches, agent idle.

**How to Verify:**
Run: `go test ./test/integration/... -run TestIntegration_RetryExhaustionFailsBatch -v`
Expected: PASS; attempt count equals configured max retries, final status failed.

**Acceptance Criteria:**
- [ ] Worker invoked expected number of times
- [ ] Item status `failed`, batch status `failed`
- [ ] Agent status returns to idle after exhaustion


### Task 7: Test paused batch blocks scheduling until resume

**Depends on:** Task 6
**Files:**
- Modify: `test/integration/integration_test.go`

**Purpose:** Confirm a paused batch is skipped by scheduler ticks and resumes processing only after hitting the resume endpoint.

**Context to rehydrate:** `batch_pause_handlers.go`, `batch_restart_handler.go`, scheduler selection of pending batches.

**Outcome:** `TestIntegration_PauseThenResumeBatch` pauses a pending batch, runs several `Tick` cycles with no progress, resumes, then ensures items finish and batch completes.

**How to Verify:**
Run: `go test ./test/integration/... -run TestIntegration_PauseThenResumeBatch -v`
Expected: PASS; no attempts while paused, completion after resume within bounded ticks.

**Acceptance Criteria:**
- [ ] Pause endpoint returns 200 and sets batch status `paused`
- [ ] Scheduler skips paused batch during ticks
- [ ] After resume, batch completes and agent returns to idle


### Task 8: (Optional) Batch restart path smoke test

**Depends on:** Task 7
**Files:**
- Modify: `test/integration/integration_test.go`

**Purpose:** Provide a lightweight check that restarting a failed batch via HTTP resets item statuses and allows reprocessing.

**Context to rehydrate:** `batch_restart_handler.go` and storage status handling.

**Outcome:** Test ensures restart resets items to pending, ticks succeed, final status completed.

**How to Verify:**
Run: `go test ./test/integration/... -run TestIntegration_RestartFailedBatch -v`
Expected: PASS; statuses reset then complete.

**Acceptance Criteria:**
- [ ] Restart endpoint returns 200
- [ ] Items reset to pending/attempts zeroed
- [ ] Batch completes after rerun

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes                                     |
| --------------------------- | ------ | ----------------------------------------- |
| Complete                    | ✓      | Covers happy, retry, pause/resume, restart scenarios from task description |
| Accurate                    | ✓      | File paths verified via repo inspection (`internal/app`, `test/integration`) |
| Commands valid              | ✓      | go test commands align with package path `./test/integration/...` |
| YAGNI                       | ✓      | No extra features beyond required scenarios |
| Minimal                     | ✓      | Tasks split by scenario and harness creation |
| Not over‑engineered         | ✓      | Manual Tick loops avoid background daemons |
| Key Decisions documented    | ✓      | Four decisions listed |
| Supporting docs present     | ✓      | Linked key source files and stdlib docs |
| Context sections present    | ✓      | Each task includes purpose/context/outcome |
| Budgets respected           | ✓      | Tasks limited to ≤2 files and focused outcomes |
| Outcome & Verify present    | ✓      | Every task lists Outcome and How to Verify |
| Acceptance Criteria present | ✓      | Each task includes checklist items |
| Rehydration context present | ✓      | Included where prior work matters |

### Rule‑of‑Five Passes

| Pass        | Changes Made                             |
| ----------- | ---------------------------------------- |
| Draft       | Structured tasks, outcomes, verification |
| Correctness | Checked file paths and commands          |
| Clarity     | Simplified wording, added contexts       |
| Edge Cases  | Added pause/resume, restart coverage     |
| Excellence  | Tightened acceptance criteria, kept tests fast |

---
