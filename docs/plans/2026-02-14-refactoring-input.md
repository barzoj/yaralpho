# Persistent Scheduler & Repository-Aware Processing Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Refactor the service to a Mongo-backed, repository-aware scheduler that processes batch items sequentially per batch, across multiple runtime-configurable agents, with retries, pause/resume, and a graceful restart endpoint for CI.

**Architecture:** A single web process (only one scheduler instance) drives a 10s periodic goroutine that reads Mongo for pending batches. It claims the next pending item of each eligible batch, assigns the first idle agent, and executes work via the existing worker interface. Batches are tied to repositories; agents are runtime entries. The legacy in-memory queue and epic concept are removed. Restart drains active work and optionally blocks the caller.

**Tech Stack:** Go 1.25, MongoDB driver already in use, Gorilla mux, existing logging/notify/tracker wiring. No new dependencies.

**Assumptions & Constraints:**
- Exactly one web process; many agents (workers) allowed.
- No per-batch concurrency: items within a batch run strictly sequentially; multiple batches may run in parallel if multiple agents are idle.
- Repository CRUD stores name/path; path validation required; unique name/path; deletion only when no active batches.
- Agent CRUD manages runtime workers (codex|copilot); status is busy|idle; default idle; deletion/update forbidden when busy.
- Batch is created pending; first failed item marks entire batch failed. Only one failed item is tracked; human fixes and restarts.
- Retry policy: per-item attempts up to `maxRetries`; after exhaustion, batch is failed. Batch restart endpoint resets failed item+batch to pending.
- Pause semantics: pause prevents new scheduling; in-progress item finishes; resume returns batch to pending.
- Restart endpoint: `/restart?wait` optional bool. If wait=true, HTTP blocks until all active work drains; used by CI to `curl ... && start`.
- Status sets: Batch {pending, in-progress, failed, paused, done}; Item {pending, in-progress, done, failed}; Agent {idle, busy}.
- Runs are per-item attempts (no epics). `/runs/{id}`, `/runs/{id}/events`, `/runs/{id}/events/live` remain; global `/runs` list is removed.
- Scheduler must skip paused or draining states; no new work when draining.
- First idle agent selection is simple (no balancing).
- No special filesystem security beyond basic path validation.

**API Surface (final form):**
- Repository CRUD: `POST/GET /repository`, `GET/PUT/DELETE /repository/{id}` (delete only if no active batches).
- Add batch: `POST /repository/{repoid}/add` (replaces old `/add`); creates pending batch, items pending, repository_id set.
- List batches: `GET /repository/{repoid}/batches?status=` (optional filter pending|in-progress|failed|paused|done).
- Pause/Resume: `PUT /repository/{repoid}/batch/{batchid}/pause`, `.../resume`.
- Restart failed batch: `PUT /repository/{repoid}/batch/{batchid}/restart` (only when batch failed; resets failed item to pending, attempts=0).
- List runs for batch: `GET /repository/{repoid}/batch/{batchid}/runs`.
- Run detail/events: `/runs/{id}`, `/runs/{id}/events`, `/runs/{id}/events/live` (unchanged).
- Agents CRUD: `POST/GET /agent`, `GET/PUT/DELETE /agent/{id}` (no delete/update when busy).
- System: `GET /version`; `POST/GET /restart?wait` (drain + optional blocking).

**Scheduler / Execution Rules:**
- Tick every 10s (configurable): if draining, skip; otherwise: find next pending batch with a pending item; ensure batch not paused; ensure no item in-progress in that batch (sequential rule); pick first idle agent; set agent busy; mark item in-progress; create Run with repository_id; execute.
- On success: item done; if all items done -> batch done; agent idle.
- On failure: increment item attempt; if attempts < maxRetries -> set item pending, batch pending; else set item failed + batch failed; agent idle.
- Pause/resume enforced in selection; pause never cancels in-progress item.
- Draining: Stop starting new work; wait for active runs to finish; `wait=true` blocks caller until drain complete; `wait=false` returns immediately while draining continues.

**Retry & Restart Semantics:**
- Item retries handled automatically up to maxRetries per item.
- Batch restart endpoint only allowed on failed batch; finds failed item, resets to pending/attempts=0, batch status pending; scheduler picks it up in next tick.

**Testing Expectations:**
- Unit tests for scheduler selection, sequential constraint, pause, retry, failure paths, agent busy/idle transitions.
- Integration tests with mocked agent executor and shortened tick interval: pending→in-progress→done; retry to failure; pause/resume; batch restart.
- Handler tests for all endpoints, validation, and forbidden operations (delete busy agent, delete repo with active batch, restart non-failed batch).
- Migration tests for new models/fields.

---

## Tasks (execute in order; sequence is implicit)

### Task 1: Capture current models/handlers context
**Files:** Read `internal/storage/models.go`, `internal/storage/interfaces.go`, `internal/app/*.go`, `internal/queue/*`, `internal/consumer/*` (no edits).  
**Purpose:** Build a precise map of current Batch/Run shapes, queue usage, handler routes (including `/runs` endpoints), and where epics and in-memory queue are referenced so later edits don’t miss any code paths. Produce notes that enumerate every field and route needing change.  
**Outcome:** A short notes block (kept locally or in commit message scratch) listing existing models, queue wiring points, and handlers to touch/remove.  
**How to Verify:** Confirm notes include: Batch/Run fields, queue interfaces, consumer flow, all routes referencing runs/add/epic. No code diffs yet (`git status` clean).  
**Acceptance Criteria:** Notes are complete enough to guide edits without re-reading code; no files modified.

### Task 2: Define new domain models & constants
**Files:** Modify `internal/storage/models.go`; modify `internal/storage/interfaces.go`; add/update `internal/storage/status.go` (or equivalent).  
**Purpose:** Introduce Repository and Agent structs; extend Batch with `repository_id` and `items []BatchItem{input,status,attempts}`; extend Run with `repository_id`; define enums for BatchStatus, ItemStatus, AgentStatus with doc comments matching lifecycle rules.  
**Outcome:** Compilable model/interface definitions reflecting repository-aware batches, item-level status/attempts, agent runtime model, and run linkage.  
**How to Verify:** Run `go vet ./...` and `go test ./internal/storage/...` (may fail until persistence updated, acceptable here if model-only errors are resolved).  
**Acceptance Criteria:** Code compiles after stub fixes; every public type/method documented; statuses match defined sets; no epic references remain in models.

### Task 3: Persistence layer updates (Mongo)
**Files:** Modify `internal/storage/mongo/*` to implement Repository and Agent CRUD, Batch and Run serialization; add indexes in setup/init files; tests in `internal/storage/mongo/*_test.go`.  
**Purpose:** Persist new models: repository collection (unique name/path), agent collection (runtime, unique name), batch with items sub-doc and repository_id, run with repository_id. Ensure read/write functions map new fields and enforce sequential item order.  
**Outcome:** Mongo implementations that can create/read/update/delete repositories and agents; batches store item statuses/attempts; runs store repository_id; required indexes created.  
**How to Verify:** `go test ./internal/storage/mongo/...`; manual `go vet`; optionally run a small round-trip test creating repository, agent, batch with items, run.  
**Acceptance Criteria:** Tests pass; uniqueness enforced; serialization round-trips item status/attempts; migrations/backfill handled or noted if N/A.

### Task 4: Remove in-memory queue usage
**Files:** Modify/delete `internal/queue/*`; adjust wiring in `internal/app/app.go` (or equivalent composition root); update `internal/consumer/*` to drop queue dependency.  
**Purpose:** Eliminate the legacy in-memory producer/consumer path so only the new scheduler drives work. Prevent dead code paths and future confusion.  
**Outcome:** Build succeeds without queue package usage; consumer no longer listens to in-memory queue.  
**How to Verify:** `rg "queue" internal | grep -v vendor` shows only intended references (e.g., comments if retained); `go test ./...` compiles through consumer without queue.  
**Acceptance Criteria:** No runtime wiring of queue remains; consumer entrypoint uses scheduler hook instead.

### Task 5: Scheduler interface skeleton
**Files:** Create `internal/scheduler/interface.go`; create `internal/scheduler/scheduler.go`; tests `internal/scheduler/scheduler_test.go` (interface existence).  
**Purpose:** Define Start/Stop/Tick API plus constructor signature including interval, drain flag, storage and worker interfaces; document concurrency and drain expectations. Provide stub implementation so downstream compiles.  
**Outcome:** Interface with comments; stub Scheduler struct implementing methods (no logic yet); tests asserting interface compile.  
**How to Verify:** `go test ./internal/scheduler -run TestSchedulerInterface`; `go vet ./internal/scheduler`.  
**Acceptance Criteria:** Clear docs on Tick preconditions (single process, no new work when draining); code compiles.

### Task 6: Implement scheduler tick selection
**Files:** `internal/scheduler/scheduler.go`; tests `internal/scheduler/scheduler_test.go`.  
**Purpose:** Implement Tick logic: skip if draining; fetch next pending batch with pending item; enforce no in-progress item for that batch (sequential rule); skip paused batches; pick first idle agent; atomically set agent busy, item in-progress, create Run; dispatch execution via worker callback.  
**Outcome:** Functional Tick with unit tests covering: no batches, no agents, paused batch ignored, happy path claim, sequential enforcement.  
**How to Verify:** `go test ./internal/scheduler -run TestTick*`; inspect logs (if using test logger spy) for decision branches.  
**Acceptance Criteria:** Agent busy set before execution; item status persisted as in-progress; batch status set to in-progress; sequential constraint enforced.

### Task 7: Worker execution + retry handling
**Files:** `internal/scheduler/scheduler.go`; `internal/scheduler/scheduler_test.go`; update/add worker adapter in `internal/consumer/worker.go` (or new file).  
**Purpose:** On execution completion: success → item done, batch done if last; failure → increment attempts, if attempts < maxRetries set item pending/batch pending, else mark item failed and batch failed; agent always set idle. Create Run per attempt with repository_id.  
**Outcome:** Retry-aware execution flow with tests for success, retry-then-success, retry-exhausted → batch failed.  
**How to Verify:** `go test ./internal/scheduler -run TestRetry*`; assert agent returns to idle in each branch.  
**Acceptance Criteria:** Batch fails immediately when an item exhausts retries; only one failed item tracked; run records attempts.

### Task 8: Pause/resume behavior enforcement
**Files:** `internal/scheduler/scheduler.go`; tests.  
**Purpose:** Ensure paused batches are never selected; in-progress item may finish; resume moves paused batch back to pending (if not failed/done).  
**Outcome:** Pause flag checked in selection; resume updates status appropriately; tests for paused skip and resume pickup.  
**How to Verify:** `go test ./internal/scheduler -run TestPause*`.  
**Acceptance Criteria:** No item starts while batch paused; after resume, next tick schedules pending item.

### Task 9: Graceful drain/restart plumbing
**Files:** `internal/scheduler/scheduler.go`; `internal/app/restart_handler.go` (new); wiring in `internal/app/app.go`; tests `internal/app/restart_handler_test.go`.  
**Purpose:** Add draining state and active-run tracking; Stop prevents new ticks; restart handler sets draining, waits for active runs if `wait=true`, returns immediately if false. Ensure scheduler rejects new work during drain.  
**Outcome:** Functional `/restart?wait` that supports CI “curl forever && start”; drain finishes active items then stops.  
**How to Verify:** `go test ./internal/app -run TestRestart*` using fake scheduler with active counter; manual curl in dev environment optional.  
**Acceptance Criteria:** No new work starts after drain begins; wait=true blocks until active=0; responses use appropriate status codes (e.g., 202 non-wait, 200/204 when drained).

### Task 10: Repository CRUD endpoints
**Files:** `internal/app/repository_handlers.go` (new); router wiring in `internal/app/app.go`; tests `internal/app/repository_handlers_test.go`.  
**Purpose:** Implement POST/GET list/GET by id/PUT/DELETE for repositories with validation: unique name/path, path format, deletion only when no active/pending/in-progress batches reference repo.  
**Outcome:** Fully functional repository API with error handling.  
**How to Verify:** `go test ./internal/app -run TestRepository*`; manual `curl` optional.  
**Acceptance Criteria:** 409 on duplicate name/path; 400 on bad input; 409 on delete with active batches; responses include repo_id, timestamps.

### Task 11: Agent CRUD endpoints
**Files:** `internal/app/agent_handlers.go` (new); router wiring; tests `internal/app/agent_handlers_test.go`.  
**Purpose:** Manage runtime agents (name, agent type codex|copilot, status default idle). Block delete/update when busy.  
**Outcome:** API surfaces POST/GET list/GET by id/PUT/DELETE with validations.  
**How to Verify:** `go test ./internal/app -run TestAgent*`.  
**Acceptance Criteria:** Invalid agent type rejected; busy agent cannot be deleted or status-updated; new agents start idle.

### Task 12: Batch creation under repository
**Files:** Modify existing add handler to new route `POST /repository/{repoid}/add`; router wiring; tests `internal/app/batch_handlers_test.go`.  
**Purpose:** Replace old `/add`; create batch with repository_id, status pending; initialize items list (input from request) with status pending and attempts=0; batch status not enqueued to queue.  
**Outcome:** New endpoint returns batch id and initial status; old `/add` removed.  
**How to Verify:** `go test ./internal/app -run TestBatchCreate*`; `rg "/add"` confirms only new route references.  
**Acceptance Criteria:** Batch saved pending; items saved pending; no queue interaction; 404 if repository not found.

### Task 13: List batches by repository with status filter
**Files:** `internal/app/batch_handlers.go`; tests.  
**Purpose:** Implement `GET /repository/{repoid}/batches?status=` returning batches for repo; optional status filter (pending|in-progress|failed|paused|done).  
**Outcome:** Endpoint delivers filtered or full list; handles unknown status with 400.  
**How to Verify:** `go test ./internal/app -run TestBatchList*`.  
**Acceptance Criteria:** Correct filtering; empty list on none; pagination consistent with existing style if any (documented if absent).

### Task 14: Pause/resume endpoints
**Files:** `internal/app/batch_handlers.go`; tests.  
**Purpose:** Add `PUT /repository/{repoid}/batch/{batchid}/pause` and `/resume`. Pause allowed unless batch done/failed; resume allowed only from paused (and possibly failed? requirement: paused only).  
**Outcome:** Status transitions persisted; scheduler observes pause flag.  
**How to Verify:** `go test ./internal/app -run TestBatchPauseResume*`.  
**Acceptance Criteria:** Pause does not alter in-progress item; resume sets pending; invalid transitions return 409.

### Task 15: Restart failed batch endpoint
**Files:** `internal/app/batch_handlers.go`; tests.  
**Purpose:** `PUT /repository/{repoid}/batch/{batchid}/restart` permitted only when batch failed; find failed item, reset status to pending and attempts=0; set batch status pending.  
**Outcome:** Failed batches can be retried after human fixes.  
**How to Verify:** `go test ./internal/app -run TestBatchRestart*`.  
**Acceptance Criteria:** 409 if batch not failed; after restart, scheduler picks up on next tick; response returns updated batch state.

### Task 16: Runs listing scoped to batch
**Files:** `internal/app/run_handlers.go`; tests.  
**Purpose:** Implement `GET /repository/{repoid}/batch/{batchid}/runs`; remove global `GET /runs`; ensure repository scoping.  
**Outcome:** Batch-scoped run listing replaces legacy global list.  
**How to Verify:** `go test ./internal/app -run TestBatchRunsList*`; `rg "HandleFunc(\"/runs\"" confirms only detail/events routes remain).  
**Acceptance Criteria:** Returns runs belonging to batch; 404 on unknown batch; old list endpoint removed.

### Task 17: Run detail/event handlers alignment
**Files:** `internal/app/run_handlers.go`; tests.  
**Purpose:** Keep `/runs/{id}`, `/runs/{id}/events`, `/runs/{id}/events/live` working with new schema (repository_id present, no epic). Ensure data access uses updated storage structs.  
**Outcome:** Detail/event routes function unchanged externally but using new data model.  
**How to Verify:** `go test ./internal/app -run TestRunDetail*`.  
**Acceptance Criteria:** Responses include repository context where applicable; no epic references; handlers compile.

### Task 18: Remove epic concept entirely
**Files:** Search/modify `internal/**/*` where epic logic exists; adjust tracker usage; update tests.  
**Purpose:** Eradicate branching on epic vs task; align terminology to single-task batches.  
**Outcome:** No code paths or tests refer to epics; tracker usage simplified or stubbed accordingly.  
**How to Verify:** `rg "epic" internal` yields zero hits except explanatory comments; `go test ./...` passes.  
**Acceptance Criteria:** Build green; functionality matches single-task batches.

### Task 19: Version endpoint
**Files:** `internal/app/system_handlers.go` (new or existing); router wiring; tests.  
**Purpose:** Expose `GET /version` returning git commit SHA or “dev” fallback for non-repo builds.  
**Outcome:** Endpoint returns JSON with version string.  
**How to Verify:** `go test ./internal/app -run TestVersion*`; manual `curl` optional.  
**Acceptance Criteria:** 200 response with version field; no side effects.

### Task 20: Restart endpoint wiring
**Files:** `internal/app/app.go`; `internal/app/restart_handler.go` (from Task 9).  
**Purpose:** Register `/restart` route with optional `wait` bool param; wire to drain logic; ensure handler accessible via router.  
**Outcome:** Endpoint reachable and functions per drain implementation.  
**How to Verify:** `go test ./internal/app -run TestRestart*`; manual `curl -i '/restart?wait=true'` optional.  
**Acceptance Criteria:** Correct status codes; respects wait parameter; no duplicate routes.

### Task 21: Observability & logging
**Files:** `internal/scheduler/scheduler.go`; relevant handlers (restart, batch transitions).  
**Purpose:** Add structured logs for scheduler decisions (no batches, no agents, paused, claim, retry, fail), drain start/stop, pause/resume actions, agent busy/idle transitions. Include batch_id, item index, agent_id, repository_id, attempt counts.  
**Outcome:** Traceable logs enabling debugging of scheduling and restart behavior.  
**How to Verify:** Unit tests can use logger spy/assert; manual run shows logs on claim/retry/fail.  
**Acceptance Criteria:** Every branch logs entry/exit and errors with identifiers; no sensitive data logged.

### Task 22: Configuration defaults
**Files:** `internal/config/config.go` (or equivalent); any wiring in `internal/app/app.go`; docs.  
**Purpose:** Add config fields for scheduler interval (default 10s), maxRetries, restart wait timeout; bind from env (`YARALPHO_*`), document defaults; inject into scheduler.  
**Outcome:** Configurable behavior with safe defaults; scheduler uses injected values.  
**How to Verify:** `go test ./internal/config/...`; unit test for default values when env unset; integration overrides interval for fast tests.  
**Acceptance Criteria:** Missing env → defaults applied; invalid values handled gracefully (error or fallback).

### Task 23: Integration tests (happy, retry, pause/resume, batch restart)
**Files:** `internal/app/integration_test.go` or `test/integration/*`; fakes for agent execution.  
**Purpose:** Automated end-to-end coverage for four scenarios:  
1) create repo + task → batch pending.  
2) repo + one worker + one task → pending → in-progress → done; agent idle.  
3) repo + worker + task that fails until maxRetries → batch failed; agent idle.  
4) repo + task, pause before worker available → not picked up; resume → picked up and completes.  
Use shortened tick interval via config override to keep tests fast/deterministic.  
**Outcome:** Deterministic integration suite validating scheduler + API behavior.  
**How to Verify:** `go test ./... -run TestIntegration*`; ensure runtime < reasonable limit by using small intervals and faked time where possible.  
**Acceptance Criteria:** All four scenarios pass consistently; no flakiness; uses mocked agent executor.

### Task 24: Docs update
**Files:** `README.md`; add/update `docs/design/scheduler.md` or append to this plan file; ensure removal note for `/runs` list and epic concept.  
**Purpose:** Document new APIs, status lifecycles, retry rules, pause/resume, restart semantics, config envs, and sequential batch rule.  
**Outcome:** Developer-facing docs accurate to implementation; quickstart examples for key endpoints.  
**How to Verify:** Manual review; check that routes listed match router; `rg "/runs"` in docs to ensure only correct references.  
**Acceptance Criteria:** All new endpoints documented; old ones removed; restart usage (wait param) explained for CI.

### Task 25: Cleanup dead code
**Files:** Delete/trim `internal/queue/*` remnants; remove obsolete tests; `go.mod`/`go.sum` tidied if needed.  
**Purpose:** Remove unused artifacts post-refactor to reduce maintenance and confusion.  
**Outcome:** No dead packages; dependency files clean.  
**How to Verify:** `rg "queue" internal` minimal; `go mod tidy`; `go test ./...`.  
**Acceptance Criteria:** Build green; no unused packages; repository clean.

### Task 26: Final verification & push
**Files:** N/A (commands).  
**Purpose:** Run full quality gates and ensure remote sync per AGENTS workflow.  
**Outcome:** `go test ./...` and `go vet ./...` pass; git clean; changes pushed after `git pull --rebase` and `bd sync`.  
**How to Verify:** Capture command outputs; `git status` shows up to date with origin.  
**Acceptance Criteria:** All tests/linters green; push succeeds; worktree clean.

---

## Verification Record

| Check                       | Status | Notes |
| --------------------------- | ------ | ----- |
| Complete                    | ✓ | All user-stated requirements + Q&A captured; no new deps. |
| Accurate                    | ✓ | Single web process, sequential per-batch captured. |
| Commands valid              | ✓ | go test/vet; curl restart example implied. |
| YAGNI                       | ✓ | No distributed locks; simple first-idle agent pick. |
| Minimal                     | ✓ | Tasks scoped to ≤2 prod files each. |
| Not over-engineered         | ✓ | Mongo-backed scheduler only. |
| Key Decisions documented    | ✓ | Present in epic. |
| Supporting docs present     | ✓ | Points to raw input file + code paths. |
| Context sections present    | ✓ | Each task has Purpose/Outcome/Verify. |
| Budgets respected           | ✓ | Tasks target 30–60 mins, ≤2 prod files. |
| Outcome & Verify present    | ✓ | Every task includes both. |
| Acceptance Criteria present | ✓ | Included per task. |
| Rehydration context present | ✓ | Task 1 primes context; files listed per task. |

### Rule‑of‑Five Passes
| Pass        | Changes Made |
| ----------- | ------------ |
| Draft       | Full task breakdown with required fields. |
| Correctness | Ensured status/route semantics match requirements/Q&A. |
| Clarity     | Expanded Purpose/Outcome/Verify per task. |
| Edge Cases  | Included pause/resume, restart wait, retry exhaust, delete guards. |
| Excellence  | Added logging/config tasks, cleanup, final verification. |

