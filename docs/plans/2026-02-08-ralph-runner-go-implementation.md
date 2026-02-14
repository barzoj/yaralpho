# Ralph Runner Go Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use superpowers:subagent-driven-development for parallel execution.

**Goal:** Build “Ralph Runner,” a Go (1.21+) web service that: (1) accepts batch adds via POST /add query params, (2) expands issue refs, classifies epics via beads, and enqueues tasks, (3) for each task starts a fresh GitHub Copilot Go SDK session with auto-approve permissions and no interactive prompts, (4) streams and persists every SDK event verbatim to MongoDB, (5) snapshots git before/after to record commit hash changes, (6) updates task run and batch statuses, (7) emits Slack notifications via webhook for task completion, blocked auth, and batch idle, (8) exposes GET endpoints to list batches, batch runs, runs, and per-run details, plus a batch progress view, (9) enforces auth gate requiring COPILOT*GITHUB_TOKEN|GH_TOKEN|GITHUB_TOKEN before any session, (10) logs everything with zap.Logger (structured, leveled), and (11) reads config via JSON+YARALPHO*\* env with required variables for Mongo, Slack, repo paths, beads repo, and auth tokens, panicking if required config is absent.

**Architecture:** Single-process HTTP server with in-process FIFO queue and producer/consumer model. Modules: config loader (interface-driven, env-first, JSON fallback), storage (Mongo collections: batches, task_runs, session_events), tracker adapter interface (beads implementation for epic detection/children), notifier interface (Slack + noop), copilot wrapper (session lifecycle, events stream), runner (producer/consumer, status transitions, git snapshots, progress calc), server (gorilla/mux endpoints), logging (zap injected everywhere). Each task run uses its own Copilot session; events are captured and stored raw.

**Tech Stack:** Go 1.21+, gorilla/mux, MongoDB Go driver, GitHub Copilot Go SDK, Slack incoming webhook, bd CLI, zap.Logger.

**Key Decisions:**

- **Framework:** gorilla/mux for routing — matches user preference and is minimal/compatible.
- **Tracker:** beads CLI for epic detection — reuses existing project tooling; avoids custom maps.
- **Config:** Interface-based loader with JSON + env overrides (YARALPHO\_\*), env-first, panic on missing required config — enforces correctness.
- **Queue model:** In-process FIFO, single worker — MVP simplicity and deterministic sequencing.
- **Event logging:** Store raw Copilot SDK events unmodified — preserves fidelity for future analytics.
- **Logging:** Use zap.Logger with structured fields everywhere — consistent observability.

---

## Supporting Documentation

- gorilla/mux router: https://github.com/gorilla/mux (middleware, vars, subrouters)
- MongoDB Go driver: https://pkg.go.dev/go.mongodb.org/mongo-driver/mongo (contexts, indexes, timeouts)
- GitHub Copilot Go SDK: https://github.com/github/copilot-sdk (session creation, event streaming)
- Slack incoming webhooks: https://api.slack.com/messaging/webhooks (payload format, rate limits)
- beads CLI usage: https://github.com/your-org/beads (bd show, bd ready patterns) — confirm actual repo; plan assumes `bd show <ref>` outputs children for epics.
- zap logger: https://pkg.go.dev/go.uber.org/zap (structured logging patterns)

---

## Tasks

### Task 1: Initialize Go module and dependencies

**Depends on:** None
**Files:**

- Create: `go.mod`
- Modify: `go.sum`

**Purpose:** Establish Go module with required dependencies and zap logging.
**Context to rehydrate:** Inspect repository root; run `ls` to confirm no existing go.mod.
**Outcome:** Module initialized with gorilla/mux, MongoDB driver, Copilot SDK, Slack helper, zap, and test deps tidy.
**How to Verify:**
Run: `go list ./...`
Expected: command succeeds, module path correct.
**Acceptance Criteria:**

- [ ] go.mod present with correct module name `github.com/barzoj/yaralpho`
- [ ] Dependencies added without errors
- [ ] go list ./... succeeds
- [ ] No extraneous files generated
      **Not In Scope:** App code.
      **Gotchas:** Ensure zap version matches Go module proxy; use replace only if needed.

### Task 2: Config interface and loader (env-first, JSON fallback)

**Depends on:** Task 1
**Files:**

- Create: `internal/config/config.go`
- Create: `config.example.json`

**Purpose:** Define `Config` interface (e.g., `Get(key string) (string, error)`) with env-first lookup and JSON fallback, panicking if required vars missing; enumerate all required env vars.
**Context to rehydrate:** Goal section; supporting docs; env var list below.
**Outcome:**

- Interface `Config` with getter(s), implemented by struct that loads config file path from `RALPH_CONFIG` (default `config.json`).
- Required keys (env names override): `YARALPHO_MONGODB_URI`, `YARALPHO_MONGODB_DB`, `YARALPHO_SLACK_WEBHOOK_URL` (optional but logged if absent), `YARALPHO_REPO_PATH`, `YARALPHO_BD_REPO`, `YARALPHO_PORT` (default 8080), `COPILOT_GITHUB_TOKEN`|`GH_TOKEN`|`GITHUB_TOKEN`, `YARALPHO_CONFIG_PATH` (alias for RALPH_CONFIG if desired).
- Loader checks env first; if missing, reads JSON; if still missing for required keys (all except Slack), log panic and exit.
- Uses zap.Logger for structured errors; no secrets in logs.
  **How to Verify:**
  Run: `go test ./internal/config -v`
  Expected: Tests cover env override, missing required key panic, optional Slack handling, default config path.
  **Acceptance Criteria:**
- [ ] Interface exposed with env-first behavior
- [ ] Panic on missing required keys after file fallback
- [ ] Tokens resolved with precedence COPILOT_GITHUB_TOKEN > GH_TOKEN > GITHUB_TOKEN
- [ ] Example config lists all keys and types
      **Not In Scope:** CLI flags.
      **Gotchas:** Ensure JSON unmarshalling handles absent optional fields; avoid logging token values.

### Task 2.1: Config documentation (production-ready)

**Depends on:** Task 2
**Files:**

- Modify: `README.md`

**Purpose:** Document all env vars and config JSON schema clearly for production use.
**Context to rehydrate:** Output of Task 2; Goal requirements.
**Outcome:** README section with full variable table (names, required/optional, defaults), JSON example, precedence rules, and panic behavior description.
**How to Verify:**
Run: `grep -n "YARALPHO_MONGODB_URI" README.md`
Expected: Table and examples present and accurate.
**Acceptance Criteria:**

- [ ] All variables listed with meaning and defaults
- [ ] Example JSON matches loader
- [ ] Precedence (env over file) documented
- [ ] Panic on missing required config noted
      **Not In Scope:** Non-config docs.
      **Gotchas:** Redact tokens.

### Task 3: Define storage models (detailed)

**Depends on:** Task 1
**Files:**

- Create: `internal/storage/models.go`

**Purpose:** Specify Go structs for Mongo documents with clear field purposes and status enums.
**Context to rehydrate:** Requirements for collections and fields.
**Outcome:**

- Structs: `Batch` (batch_id uuid, created_at, input_items raw, status created|running|idle|done|failed|blocked_auth, summary, session_name), `TaskRun` (run_id uuid, batch_id, task_ref, epic_ref?, session_id, started_at/finished_at, status running|succeeded|failed|stopped, result.commit_hash, result.final_message), `SessionEvent` (batch_id, run_id, session_id, event raw map[string]any, ingested_at).
- Constants for statuses.
- BSON/JSON tags matching Mongo schema.
  **How to Verify:**
  Run: `go test ./internal/storage -run TestModels -v`
  Expected: Compiles; tags correct.
  **Acceptance Criteria:**
- [ ] All required fields present with types and tags
- [ ] Status enums defined
- [ ] Event payload uses map[string]any
- [ ] Time fields use time.Time and UTC expectation documented
      **Not In Scope:** Persistence functions.
      **Gotchas:** Avoid pointer time unless needed; ensure omitempty semantics appropriate.

### Task 4: Mongo collections and indexes definition

**Depends on:** Task 3
**Files:**

- Create: `internal/storage/collections.go`

**Purpose:** Define collection names, index specifications, and relationships between documents.
**Context to rehydrate:** Models from Task 3; Mongo index docs.
**Outcome:** Constants for collection names (`batches`, `task_runs`, `session_events`), index builders for batch_id, run_id, session_id+ingested_at, and compound batch_id+status where useful; comments documenting relations (task_runs reference batches by batch_id; session_events reference task_runs by run_id/session_id).
**How to Verify:**
Run: `go test ./internal/storage -run TestCollections -v`
Expected: Index definitions compile; names match plan.
**Acceptance Criteria:**

- [ ] Index models defined for all required lookups
- [ ] Collection names centralized
- [ ] Relations documented
      **Not In Scope:** CRUD logic.
      **Gotchas:** Mark indexes background/create if needed; unique constraints not required.

### Task 4.1: Mongo batch persistence functions

**Depends on:** Task 4
**Files:**

- Create: `internal/storage/batches.go`

**Purpose:** CRUD helpers for batches with timeouts and zap logging.
**Context to rehydrate:** Collections and models.
**Outcome:** Functions: `InsertBatch`, `UpdateBatchStatus`, `GetBatch`, `ListBatches` (paged). Uses context with timeout; logs errors via zap; returns typed errors.
**How to Verify:**
Run: `go test ./internal/storage -run TestBatches -v`
Expected: Integration tests (skip if no MONGODB_URI) cover insert/update/list.
**Acceptance Criteria:**

- [ ] Timeouts applied
- [ ] Status updates validated
- [ ] Pagination parameters respected
- [ ] Errors wrapped with context
      **Not In Scope:** Task run logic.
      **Gotchas:** Use bson for filters; ensure indexes created before use.

### Task 4.2: Mongo task run persistence functions

**Depends on:** Task 4
**Files:**

- Create: `internal/storage/task_runs.go`

**Purpose:** CRUD for task_runs including result updates.
**Context to rehydrate:** Models and indexes.
**Outcome:** Functions: `InsertTaskRun`, `UpdateTaskRunStatusAndResult`, `ListTaskRunsByBatch`, `GetTaskRun` (by run_id). Logging via zap, timeouts.
**How to Verify:**
Run: `go test ./internal/storage -run TestTaskRuns -v`
Expected: Integration tests cover insert/update/query.
**Acceptance Criteria:**

- [ ] Status transitions recorded
- [ ] Result fields upserted correctly
- [ ] Batch filter works
- [ ] Errors logged with context
      **Not In Scope:** Session events.
      **Gotchas:** Handle nil result map; avoid replacing fields unintentionally.

### Task 4.3: Mongo session event ingestion

**Depends on:** Task 4
**Files:**

- Create: `internal/storage/session_events.go`

**Purpose:** Append-only insertion of raw Copilot events with efficient indexing.
**Context to rehydrate:** Event logging requirement.
**Outcome:** Function `InsertSessionEvents(ctx, events []SessionEvent)` with ordered insert; indexes on session_id+ingested_at; zap logging for failures; optional batch_id/run_id filters.
**How to Verify:**
Run: `go test ./internal/storage -run TestSessionEvents -v`
Expected: Insert succeeds; ordering preserved.
**Acceptance Criteria:**

- [ ] Raw payload preserved
- [ ] Indexes applied
- [ ] Bulk insert supported
- [ ] Errors surfaced and logged
      **Not In Scope:** Query endpoints beyond simple filters.
      **Gotchas:** Ensure bson.D preserves map order if needed; handle large payload sizes with limits.

### Task 5: Tracker interface definition

**Depends on:** Task 2
**Files:**

- Create: `internal/tracker/tracker.go`

**Purpose:** Define tracker interface for epic detection and child enumeration.
**Context to rehydrate:** Requirements; beads usage.
**Outcome:** Interface with `IsIssueAnEpic(ctx, ref) (bool, error)` and `ListChildren(ctx, ref) ([]string, error)`; zap-logged errors; small doc comments.
**How to Verify:**
Run: `go test ./internal/tracker -run TestTrackerInterface -v`
Expected: Compile-time interface test.
**Acceptance Criteria:**

- [ ] Interface matches requirements
- [ ] No concrete dependencies leak
- [ ] Comments explain semantics
      **Not In Scope:** Implementation.
      **Gotchas:** Keep interface minimal.

### Task 5.1: Beads tracker implementation

**Depends on:** Task 5
**Files:**

- Create: `internal/tracker/beads.go`

**Purpose:** Implement tracker via `bd show <ref>` in configured repo to detect epics and list children.
**Context to rehydrate:** beads CLI docs; config.bd_repo.
**Outcome:** Uses exec.CommandContext with repo cwd; parses output to detect children; caches per process; logs via zap; returns errors on bd failure.
**How to Verify:**
Run: `go test ./internal/tracker -run TestBeads* -v` (fake command runner)
Expected: Epic detection based on children list; errors propagated.
**Acceptance Criteria:**

- [ ] Repo path honored
- [ ] Children order preserved
- [ ] Epic detection true when children exist
- [ ] Errors logged with context
      **Not In Scope:** Mutating tracker state.
      **Gotchas:** Timeouts on command; trim whitespace.

### Task 6: Notifier interface and Slack implementation

**Depends on:** Task 2
**Files:**

- Create: `internal/notify/notify.go`
- Create: `internal/notify/slack.go`

**Purpose:** Define notifier contract and Slack webhook implementation for task finished, batch idle/blocked, and errors with zap logging.
**Context to rehydrate:** Slack webhook payload format.
**Outcome:** Interface with methods `NotifyTaskFinished`, `NotifyBatchIdle`, `NotifyError`; Slack impl sends concise JSON; no-op impl when webhook missing; zap used for errors.
**How to Verify:**
Run: `go test ./internal/notify -v`
Expected: Slack payload marshals; no-op does nothing; errors surfaced.
**Acceptance Criteria:**

- [ ] Slack URL optional; no panic when absent
- [ ] HTTP client timeout set
- [ ] Payload includes batch_id, run_id, task_ref, status, commit hash
- [ ] Errors logged with context
      **Not In Scope:** Slack threads or attachments.
      **Gotchas:** Avoid leaking tokens in logs.

### Task 7: Copilot session creation wrapper

**Depends on:** Task 3
**Files:**

- Create: `internal/copilot/session.go`

**Purpose:** Wrap Copilot SDK to create a new session per task with repo path and prompt.
**Context to rehydrate:** Copilot SDK docs on session creation.
**Outcome:** Function/interface `StartSession(ctx, prompt, repoPath) (sessionID string, sessionHandle, error)` using SDK; zap logs session start/stop; validates token presence before call.
**How to Verify:**
Run: `go test ./internal/copilot -run TestSessionStart -v` (fakes/build tags)
Expected: Session created when token present; error when absent.
**Acceptance Criteria:**

- [ ] New session per invocation
- [ ] Token precedence enforced
- [ ] Repo path passed through
- [ ] Logging includes batch/run ids passed in
      **Not In Scope:** Event streaming.
      **Gotchas:** Ensure context cancellation closes session.

### Task 7.1: Copilot event streaming and storage hook

**Depends on:** Task 7
**Files:**

- Create: `internal/copilot/events.go`

**Purpose:** Subscribe to Copilot session events and forward raw payloads to storage layer.
**Context to rehydrate:** SDK event stream API.
**Outcome:** Helper that attaches to session, returns channel of raw events with metadata; uses zap logging for errors; does not mutate events.
**How to Verify:**
Run: `go test ./internal/copilot -run TestEventStream -v`
Expected: Events forwarded; channel closes on session end.
**Acceptance Criteria:**

- [ ] All event types passed through
- [ ] Backpressure handled (buffer or goroutine)
- [ ] Errors surfaced
- [ ] Logging contains session_id
      **Not In Scope:** Permission handling.
      **Gotchas:** Avoid dropping events; ensure channel closure on errors.

### Task 7.2: Copilot permission auto-approve

**Depends on:** Task 7
**Files:**

- Modify: `internal/copilot/session.go`

**Purpose:** Ensure permission handler auto-approves shell/write actions for autonomous runs.
**Context to rehydrate:** SDK permission hooks.
**Outcome:** Hook registered to auto-approve; tests assert handler invoked; zap logs approvals.
**How to Verify:**
Run: `go test ./internal/copilot -run TestPermissionAutoApprove -v`
Expected: Handler approves without prompt.
**Acceptance Criteria:**

- [ ] No interactive prompts required
- [ ] Approvals logged
- [ ] Errors from handler surfaced
      **Not In Scope:** Policy customization.
      **Gotchas:** Avoid infinite loops; ensure defaults set once.

### Task 8: Queue producer/consumer setup

**Depends on:** Tasks 4.1, 4.2, 6
**Files:**

- Create: `internal/runner/queue.go`

**Purpose:** Implement FIFO queue with producer/consumer goroutines; consumer waits for tasks and processes sequentially.
**Context to rehydrate:** Queue model in Goal.
**Outcome:** Thread-safe queue struct with enqueue/dequeue, condition wait; zap logs on push/pop; single consumer loop stub.
**How to Verify:**
Run: `go test ./internal/runner -run TestQueue -v`
Expected: Enqueue/dequeue order maintained; consumer wakes on new items.
**Acceptance Criteria:**

- [ ] Safe for concurrent producers
- [ ] Blocks when empty, resumes on push
- [ ] Context cancellation stops consumer
- [ ] Logging at enqueue/dequeue
      **Not In Scope:** Task processing.
      **Gotchas:** Avoid busy-wait; protect against panics.

### Task 8.1: Runner worker to process tasks

**Depends on:** Tasks 5.1, 7.1, 7.2, 8
**Files:**

- Create: `internal/runner/worker.go`

**Purpose:** Consume queue items, expand epics via tracker, start copilot session, stream events to storage, update statuses, send notifications.
**Context to rehydrate:** Runner requirements; storage APIs.
**Outcome:** Worker loop that for each queue item: checks auth token, creates task_run, starts session, streams events to storage, waits for completion/error, updates task_run and batch status, sends Slack notification.
**How to Verify:**
Run: `go test ./internal/runner -run TestWorker -v` (fakes for deps)
Expected: Status transitions correct; notifications called; events stored.
**Acceptance Criteria:**

- [ ] Blocks batch when token missing
- [ ] Epic expansion picks first not-yet-run child
- [ ] Events persisted as they arrive
- [ ] Errors handled and status set failed
- [ ] Logging with batch/run ids at each step
      **Not In Scope:** Git snapshot.
      **Gotchas:** Ensure session stop on context cancel; handle tracker errors gracefully.

### Task 8.2: Git snapshot and commit hash detection

**Depends on:** Task 8.1
**Files:**

- Create: `internal/runner/git.go`

**Purpose:** Snapshot repo HEAD before/after run and record commit hash changes.
**Context to rehydrate:** Git CLI usage; repo_path config.
**Outcome:** Helper that records initial HEAD, compares post-run HEAD; updates task_run result.commit_hash when changed; zap logs.
**How to Verify:**
Run: `go test ./internal/runner -run TestGitSnapshot -v` (use temp git repo)
Expected: Detects new commit; records hash.
**Acceptance Criteria:**

- [ ] Works with detached HEAD or branch
- [ ] No changes yields empty commit hash
- [ ] Errors surfaced/logged
      **Not In Scope:** Git push.
      **Gotchas:** Use repo_path from config; handle absence of git binary gracefully.

### Task 8.3: Batch progress computation logic

**Depends on:** Tasks 4.1, 4.2, 8.1
**Files:**

- Create: `internal/runner/progress.go`

**Purpose:** Compute per-batch progress counts: total tasks, completed, running, pending.
**Context to rehydrate:** Task_run statuses.
**Outcome:** Function `ComputeBatchProgress(batch_id)` using storage to count statuses; returns struct with counts; zap logs.
**How to Verify:**
Run: `go test ./internal/runner -run TestProgress -v`
Expected: Counts accurate based on seeded data.
**Acceptance Criteria:**

- [ ] Correct aggregation by status
- [ ] Handles empty batch
- [ ] Errors surfaced
      **Not In Scope:** HTTP exposure.
      **Gotchas:** Efficient queries; avoid full collection scan.

### Task 9: HTTP endpoint — POST /add

**Depends on:** Tasks 2, 8
**Files:**

- Create: `internal/server/add.go`
- Modify: `cmd/server/main.go`

**Purpose:** Implement /add handler parsing query params items (comma-separated) and session_name, creating batch, enqueueing when authed.
**Context to rehydrate:** API contract; queue producer.
**Outcome:** Handler validates params, creates batch via storage, enqueues items, returns JSON `{ batch_id, status }`; logs with zap.
**How to Verify:**
Run: `go test ./internal/server -run TestAddHandler -v`
Then: `go run ./cmd/server &` and `curl -X POST 'http://localhost:8080/add?items=ISSUE-1,ISSUE-2&session_name=demo'`
Expected: 200 with batch_id; status blocked_auth when token missing.
**Acceptance Criteria:**

- [ ] Query parsing robust (trim spaces)
- [ ] Empty items rejected with 400
- [ ] Auth gate enforced
- [ ] Logging includes batch_id
      **Not In Scope:** Other endpoints.
      **Gotchas:** Avoid blocking on enqueue; ensure JSON errors.

### Task 9.1: HTTP endpoint — GET /batches

**Depends on:** Tasks 4.1, 9
**Files:**

- Create: `internal/server/batches_list.go`

**Purpose:** List batches with status summaries, paginated.
**Context to rehydrate:** Storage ListBatches.
**Outcome:** Handler returns array of batches with id/status/created_at; supports limit/offset query params; logs with zap.
**How to Verify:**
Run: `go test ./internal/server -run TestListBatches -v`
Expected: 200 with list; pagination works.
**Acceptance Criteria:**

- [ ] Pagination defaults sane
- [ ] Errors surfaced as JSON
- [ ] Logging includes count and pagination
      **Not In Scope:** Runs details.
      **Gotchas:** Validate limits.

### Task 9.2: HTTP endpoint — GET /batches/{id}

**Depends on:** Tasks 4.1, 4.2, 9
**Files:**

- Create: `internal/server/batch_detail.go`

**Purpose:** Return batch metadata and run_ids summary for a batch.
**Context to rehydrate:** Storage GetBatch + ListTaskRunsByBatch.
**Outcome:** JSON with batch info and run_id/status list; zap logging.
**How to Verify:**
Run: `go test ./internal/server -run TestBatchDetail -v`
Expected: 200 with data; 404 on missing batch.
**Acceptance Criteria:**

- [ ] Combines batch and run summaries
- [ ] Proper 404 handling
- [ ] Logging includes batch_id
      **Not In Scope:** Session events.
      **Gotchas:** Efficient queries.

### Task 9.3: HTTP endpoint — GET /runs

**Depends on:** Tasks 4.2, 9
**Files:**

- Create: `internal/server/runs_list.go`

**Purpose:** List all runs (paged) with minimal fields.
**Context to rehydrate:** Storage list of runs.
**Outcome:** JSON array of run_id, batch_id, status, task_ref; pagination; zap logging.
**How to Verify:**
Run: `go test ./internal/server -run TestRunsList -v`
Expected: 200 with paged results.
**Acceptance Criteria:**

- [ ] Pagination supported
- [ ] Filters optional (batch_id?) documented if added
- [ ] Logging includes count
      **Not In Scope:** Events.
      **Gotchas:** Validate limits.

### Task 9.4: HTTP endpoint — GET /runs/{id}

**Depends on:** Tasks 4.2, 4.3, 9
**Files:**

- Create: `internal/server/run_detail.go`

**Purpose:** Return full run record and optionally recent events (paged or capped).
**Context to rehydrate:** Storage GetTaskRun and session events retrieval.
**Outcome:** JSON includes run info, result, maybe events slice with limit param; zap logging.
**How to Verify:**
Run: `go test ./internal/server -run TestRunDetail -v`
Expected: 200 with run; 404 when missing.
**Acceptance Criteria:**

- [ ] Events capped/paged to avoid overload
- [ ] Proper 404 handling
- [ ] Logging includes run_id
      **Not In Scope:** Mutations.
      **Gotchas:** Avoid large payloads; default limits.

### Task 9.5: HTTP endpoint — GET /batches/{id}/progress

**Depends on:** Tasks 8.3, 9
**Files:**

- Create: `internal/server/batch_progress.go`

**Purpose:** Expose batch progress counts (completed/running/pending) via API.
**Context to rehydrate:** Progress logic from Task 8.3.
**Outcome:** Handler returns JSON with counts; zap logging.
**How to Verify:**
Run: `go test ./internal/server -run TestBatchProgress -v`
Expected: 200 with counts.
**Acceptance Criteria:**

- [ ] Counts match storage data
- [ ] 404 when batch missing
- [ ] Logging includes batch_id
      **Not In Scope:** Run details.
      **Gotchas:** Limit query overhead.

### Task 10: Documentation and examples

**Depends on:** Tasks 2.1, 9.5
**Files:**

- Modify: `README.md`

**Purpose:** Document setup, env vars, config JSON example, API usage examples (all endpoints), Slack behavior, Mongo collections, logging expectations.
**Context to rehydrate:** Outputs of prior tasks.
**Outcome:** README shows installation steps, env/config variables, curl examples for all endpoints, expected Slack messages, Mongo collection layout, logging approach, and operational notes.
**How to Verify:**
Run: `grep -n "/batches" README.md`
Expected: Sections present; instructions consistent.
**Acceptance Criteria:**

- [ ] Env var list includes YARALPHO\_\* and token vars
- [ ] Curl examples for /add, /batches, /batches/{id}, /runs, /runs/{id}, /batches/{id}/progress
- [ ] Notes include event logging location and git snapshot behavior
- [ ] Slack optional noted and logging with zap mentioned
      **Not In Scope:** UI guides.
      **Gotchas:** Keep tokens redacted.

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes                                                                              |
| --------------------------- | ------ | ---------------------------------------------------------------------------------- |
| Complete                    | ✓      | Covers config, queue, tracker, copilot, logging, Mongo, Slack, endpoints, progress |
| Accurate                    | ✓      | Paths/commands reflect gorilla/mux split files and Go packages                     |
| Commands valid              | ✓      | go list/test targets and curl examples per endpoint                                |
| YAGNI                       | ✓      | Single worker, in-process queue, no auth UI, no PR/push                            |
| Minimal                     | ✓      | Tasks scoped to ≤2 files, interfaces first                                         |
| Not over-engineered         | ✓      | No external queue/worker pool; optional Slack noop                                 |
| Key Decisions documented    | ✓      | Framework, tracker, config strategy, queue model, event logging, logging stack     |
| Supporting docs present     | ✓      | mux, Mongo driver, Copilot SDK, Slack webhook, beads, zap links listed             |
| Context sections present    | ✓      | Purpose/Context present for tasks especially dependent ones                        |
| Budgets respected           | ✓      | Tasks limited in files/scope/time; split where large                               |
| Outcome & Verify present    | ✓      | Each task includes observable outcome and commands                                 |
| Acceptance Criteria present | ✓      | Checklists per task                                                                |
| Rehydration context present | ✓      | Context notes included where dependency matters                                    |

### Rule-of-Five Passes

| Pass        | Changes Made                                                                                 |
| ----------- | -------------------------------------------------------------------------------------------- |
| Draft       | Expanded tasks, added new splits for config/docs, Mongo, tracker, copilot, runner, endpoints |
| Correctness | Aligned env list, interfaces, collection/index names, and endpoint-per-file mapping          |
| Clarity     | Added detailed purposes/outcomes and logging expectations (zap)                              |
| Edge Cases  | Included token gate, panic on missing config, pagination, progress counts, event caps        |
| Excellence  | Polished wording, ensured budgets adherence, added supporting zap docs                       |
