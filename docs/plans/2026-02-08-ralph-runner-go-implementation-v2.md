# Ralph Runner Go Implementation Plan v2

After _human approval_, use plan2beads to convert this plan to a beads epic, then use superpowers:subagent-driven-development for parallel execution.

**Goal:** Deliver “Ralph Runner” as a Go (1.21+) web server that: (1) starts from `cmd/main.go` with optional CLI flag for config path; (2) loads required configuration via an interface-driven, env-first loader (YARALPHO\_\* + auth token envs) with JSON fallback and panic on missing required keys; (3) exposes HTTP endpoints: POST /add (query params only) to enqueue issue refs; GET /batches, GET /batches/{id}, GET /batches/{id}/progress, GET /runs, GET /runs/{id}; (4) uses a producer/consumer queue with a single consumer instance to process tasks in order; (5) determines epic vs task via a tracker interface (beads implementation) and selects the first ready task for epics; (6) for each task, creates a fresh GitHub Copilot SDK session, auto-approves permissions, and streams every raw SDK event to storage; (7) stores all batch, run, and event data behind storage interfaces (Mongo implementation) with collection/index design; (8) logs everything with zap.Logger; (9) sends Slack notifications via notifier interface (Slack implementation) for task completion, batch idle/blocked, and errors; (10) maintains queue state and progress metrics (done/running/pending) but defers git/status transitions to the agent inside the Copilot loop.

**Architecture:** Simple HTTP service using gorilla/mux. `cmd/main.go` wires an `internal/app` that constructs interfaces (config, storage, queue, tracker, notifier, copilot client, consumer) and handlers. Producer/consumer model: producer enqueues items as they arrive; single consumer loop pulls from queue and processes tasks sequentially. Storage, tracker, copilot, notifier, and queue are all interface-first with swappable implementations (Mongo, beads, GitHub Copilot, Slack). Logging via zap injected into all components. No git snapshotting; runner only monitors queue and persists events/status snapshots.

**Tech Stack:** Go 1.21+, gorilla/mux, zap.Logger, MongoDB Go driver (implementation), GitHub Copilot Go SDK, Slack incoming webhook, bd CLI.

**Key Decisions:**

- **Interfaces-first:** Storage, tracker, copilot, queue, notifier all defined as contracts; concrete impls are swappable (Mongo, beads, GitHub Copilot, Slack).
- **Config strategy:** Env-first with JSON fallback; panic when required keys missing to avoid partial config; namespaced YARALPHO\_\*.
- **Queue model:** Single in-process consumer for determinism and simplicity; producer adds work immediately on POST /add.
- **Logging:** zap.Logger required in all modules for structured observability.
- **Copilot sessions:** One session per task with auto-approve permissions; raw events streamed unmodified to storage.
- **No git snapshotting:** Agent loop handles git/status changes; runner only enqueues, prompts, and records events and progress.

---

## Supporting Documentation

- gorilla/mux router: https://github.com/gorilla/mux
- MongoDB Go driver: https://pkg.go.dev/go.mongodb.org/mongo-driver/mongo
- GitHub Copilot Go SDK: https://github.com/github/copilot-sdk
- Slack incoming webhooks: https://api.slack.com/messaging/webhooks
- beads CLI usage: https://github.com/your-org/beads (bd show parsing)
- zap logger: https://pkg.go.dev/go.uber.org/zap

---

## Tasks

### Task 0: Establish AGENTS.md etiquette and scaffolding

**Depends on:** None
**Files:**

- Modify: `AGENTS.md`
- Create: `internal/storage/AGENTS.md`
- Create: `internal/queue/AGENTS.md`
- Create: `internal/consumer/AGENTS.md`
- Create: `internal/app/AGENTS.md`
- Create: `internal/copilot/AGENTS.md`
- Create: `internal/tracker/AGENTS.md`
- Create: `internal/notify/AGENTS.md`

**Purpose:** Document module purposes and interfaces for agents; root AGENTS describes packages and roles.
**Context to rehydrate:** Project root structure and architecture section.
**Outcome:** Root AGENTS lists internal packages with brief roles; each module AGENTS describes purpose, exposed interfaces, and internal notes.
**How to Verify:**
Run: `grep -n "AGENTS" AGENTS.md` and spot-check module AGENTS for content.
Expected: Entries present and accurate.
**Acceptance Criteria:**

- [ ] Root AGENTS updated with package list/roles
- [ ] Each module AGENTS created with purpose and interfaces
- [ ] No TODO placeholders left
      **Not In Scope:** Implementation details beyond brief notes.
      **Gotchas:** Keep concise; no secrets.

### Task 1: Initialize Go module and dependencies

**Depends on:** Task 0
**Files:**

- Create: `go.mod`
- Modify: `go.sum`

**Purpose:** Establish Go module with required dependencies (mux, zap, Mongo driver, Copilot SDK, Slack HTTP client, test helpers).
**Context to rehydrate:** None; fresh module.
**Outcome:** Module path `github.com/barzoj/yaralpho`; dependencies resolved.
**How to Verify:**
Run: `go list ./...`
Expected: Success.
**Acceptance Criteria:**

- [ ] go.mod/go.sum present and tidy
- [ ] go list ./... succeeds
      **Not In Scope:** App code.
      **Gotchas:** Avoid adding unused deps.

### Task 2: Config interface and loader (env-first, JSON fallback, panic on missing)

**Depends on:** Task 1
**Files:**

- Create: `internal/config/config.go`
- Create: `config.example.json`

**Purpose:** Define config interface (e.g., `Get(string) (string, error)`) and implement env-first loader with JSON fallback; panic if required keys absent.
**Context to rehydrate:** Goal and Architecture.
**Outcome:**

- Required envs: `YARALPHO_MONGODB_URI`, `YARALPHO_MONGODB_DB`, `YARALPHO_REPO_PATH`, `YARALPHO_BD_REPO`, `YARALPHO_PORT` (default 8080), `YARALPHO_SLACK_WEBHOOK_URL` (optional), `COPILOT_GITHUB_TOKEN` | `GH_TOKEN` | `GITHUB_TOKEN`, `RALPH_CONFIG` (path override).
- Env overrides JSON values; missing required ⇒ zap.Panic.
- No secrets logged.
  **How to Verify:**
  Run: `go test ./internal/config -v`
  Expected: Covers env override, panic on missing, optional Slack.
  **Acceptance Criteria:**
- [ ] Interface exported
- [ ] Env precedence works
- [ ] Panic on missing required keys after file fallback
- [ ] Example JSON documents all keys
      **Not In Scope:** CLI flags.
      **Gotchas:** Trim spaces; handle empty strings as missing.

### Task 2.1: Config documentation (production-ready)

**Depends on:** Task 2
**Files:**

- Modify: `README.md`

**Purpose:** Document all config/env keys, precedence, defaults, and panic behavior.
**Context to rehydrate:** Task 2 outputs.
**Outcome:** README section with table of vars, JSON example, and rules.
**How to Verify:**
Run: `grep -n "YARALPHO_MONGODB_URI" README.md`
Expected: Table present.
**Acceptance Criteria:**

- [ ] All variables listed with required/optional and defaults
- [ ] JSON example matches loader
- [ ] Panic behavior documented
      **Not In Scope:** Endpoint docs.
      **Gotchas:** Redact tokens.

### Task 3: Define storage interfaces and models

**Depends on:** Task 1
**Files:**

- Create: `internal/storage/models.go`
- Create: `internal/storage/interfaces.go`

**Purpose:** Specify domain models and storage contracts independent of Mongo.
**Context to rehydrate:** Requirements for batches/task_runs/session_events.
**Outcome:**

- Models: Batch (id, created_at, input_items, status created|running|idle|done|failed|blocked_auth, summary, session_name), TaskRun (run_id, batch_id, task_ref, epic_ref?, session_id, started/finished, status running|succeeded|failed|stopped, result commit_hash?, final_message?), SessionEvent (batch_id, run_id, session_id, event map[string]any, ingested_at).
- Interfaces: Storage with methods for batches, task runs, session events, progress counts. No Mongo types exposed.
  **How to Verify:**
  Run: `go test ./internal/storage -run TestInterfaces -v`
  Expected: Interfaces compile; models tagged JSON/BSON-friendly.
  **Acceptance Criteria:**
- [ ] Complete fields and status enums
- [ ] Interfaces cover insert/update/list/get/progress
- [ ] No concrete DB types leak
      **Not In Scope:** Mongo implementation.
      **Gotchas:** Use time.Time; document expectations in comments.

### Task 4: Mongo storage implementation (collections, indexes, wiring)

**Depends on:** Task 3
**Files:**

- Create: `internal/storage/mongo/collections.go`
- Create: `internal/storage/mongo/batches.go`
- Create: `internal/storage/mongo/task_runs.go`
- Create: `internal/storage/mongo/session_events.go`
- Create: `internal/storage/mongo/progress.go`
- Create: `internal/storage/mongo/client.go`
- Create: `internal/storage/AGENTS.md` (if not done in Task 0, update with Mongo impl notes)

**Purpose:** Implement storage interfaces using Mongo with indexes and zap logging.
**Context to rehydrate:** Interfaces from Task 3.
**Outcome:**

- Collection names: batches, task_runs, session_events.
- Indexes: batch_id, run_id, session_id+ingested_at, batch_id+status for progress.
- CRUD functions implementing interfaces with context timeouts and structured logs.
  **How to Verify:**
  Run: `go test ./internal/storage/mongo -v` (integration; skip if no MONGODB_URI).
  Expected: Inserts/updates/query work; indexes created.
  **Acceptance Criteria:**
- [ ] Implements interfaces fully
- [ ] Index creation on init
- [ ] Errors wrapped and logged
- [ ] Raw events preserved
      **Not In Scope:** Alternative DBs.
      **Gotchas:** Guard against nil context; handle optional Slack URL absence.

### Task 5: Tracker interface definition

**Depends on:** Task 1
**Files:**

- Create: `internal/tracker/interface.go`

**Purpose:** Define tracker contract to detect epic and list children.
**Context to rehydrate:** Requirements.
**Outcome:** Methods `IsEpic(ctx, ref) (bool, error)` and `ListChildren(ctx, ref) ([]string, error)` with doc comments.
**How to Verify:**
Run: `go test ./internal/tracker -run TestInterface -v`
Expected: Interface compiles.
**Acceptance Criteria:**

- [ ] Minimal methods as required
- [ ] No implementation details
      **Not In Scope:** Beads logic.
      **Gotchas:** None.

### Task 5.1: Beads tracker implementation

**Depends on:** Task 5, Task 2
**Files:**

- Create: `internal/tracker/beads.go`

**Purpose:** Implement tracker using `bd show <ref>` in configured repo.
**Context to rehydrate:** beads CLI docs.
**Outcome:** Exec wrapper with timeouts, parsing children, zap logging; uses config bd repo path.
**How to Verify:**
Run: `go test ./internal/tracker -run TestBeads* -v` (mock exec).
Expected: Epic detection true when children exist; order preserved.
**Acceptance Criteria:**

- [ ] Repo path honored
- [ ] Errors propagated/logged
- [ ] No secrets logged
      **Not In Scope:** Mutations.
      **Gotchas:** Trim whitespace; handle non-epic gracefully.

### Task 6: Notifier interface and Slack implementation

**Depends on:** Task 1, Task 2
**Files:**

- Create: `internal/notify/interface.go`
- Create: `internal/notify/slack.go`

**Purpose:** Notify task completion, batch idle/blocked, and errors via Slack; provide noop when webhook missing.
**Context to rehydrate:** Slack webhook docs.
**Outcome:** Interface methods `NotifyTaskFinished`, `NotifyBatchIdle`, `NotifyError`; Slack impl with zap logging and HTTP timeouts.
**How to Verify:**
Run: `go test ./internal/notify -v`
Expected: Payloads marshal; noop does nothing; errors logged.
**Acceptance Criteria:**

- [ ] Optional Slack URL handled
- [ ] Includes batch_id/run_id/task_ref/status/commit hash
- [ ] Errors surfaced
      **Not In Scope:** Threads.
      **Gotchas:** Redact secrets.

### Task 7: Queue interfaces and implementation

**Depends on:** Task 1
**Files:**

- Create: `internal/queue/interface.go`
- Create: `internal/queue/memory.go`
- Update: `internal/queue/AGENTS.md`

**Purpose:** Provide producer/consumer FIFO queue with single consumer.
**Context to rehydrate:** Lifecycle description.
**Outcome:** Interface for Enqueue/Dequeue/Close; in-memory implementation with blocking dequeue; zap logging.
**How to Verify:**
Run: `go test ./internal/queue -v`
Expected: FIFO order, blocking behavior works.
**Acceptance Criteria:**

- [ ] Thread-safe
- [ ] Context-aware dequeue
- [ ] Logs enqueue/dequeue
      **Not In Scope:** External queues.
      **Gotchas:** Avoid busy-wait; handle shutdown.

### Task 8: Copilot client interface

**Depends on:** Task 1
**Files:**

- Create: `internal/copilot/interface.go`

**Purpose:** Define contract for starting sessions and streaming events.
**Context to rehydrate:** Requirements.
**Outcome:** Interface with methods to start session (task prompt, repo), return session_id, event channel, stop func; includes auto-approve requirement.
**How to Verify:**
Run: `go test ./internal/copilot -run TestInterface -v`
Expected: Compiles.
**Acceptance Criteria:**

- [ ] Methods cover session start/events/stop
- [ ] No SDK leakage in signature
      **Not In Scope:** Implementation.
      **Gotchas:** None.

### Task 8.1: GitHub Copilot implementation

**Depends on:** Task 8, Task 2
**Files:**

- Create: `internal/copilot/github.go`

**Purpose:** Implement copilot interface using GitHub Copilot SDK with auto-approve permissions and raw event streaming.
**Context to rehydrate:** SDK docs.
**Outcome:** Session per task, permission handler auto-approves, events forwarded unmodified via channel, zap logging.
**How to Verify:**
Run: `go test ./internal/copilot -run TestGitHub* -v` (use fakes/build tags)
Expected: Session created; events emitted; auto-approve works.
**Acceptance Criteria:**

- [ ] Token presence checked before start
- [ ] Repo path passed
- [ ] Events not mutated
- [ ] Stop closes streams
      **Not In Scope:** Alternate providers.
      **Gotchas:** Handle SDK errors; buffer channels to avoid drops.

### Task 9: Consumer logic (worker)

**Depends on:** Tasks 5.1, 6, 7, 8.1, 3, 4
**Files:**

- Create: `internal/consumer/worker.go`
- Update: `internal/consumer/AGENTS.md`

**Purpose:** Consume queue items, classify epic vs task, craft prompt, start copilot session, stream events to storage, update run/batch records, send notifications.
**Context to rehydrate:** Interfaces.
**Outcome:** Worker loop that pulls from queue, creates TaskRun via storage, uses tracker to detect epic and choose first child, sets prompt accordingly, starts copilot session, pipes events to storage, sets statuses, notifies Slack.
**How to Verify:**
Run: `go test ./internal/consumer -run TestWorker -v` (fakes for deps)
Expected: Correct prompt selection, status updates, notifications, event storage calls.
**Acceptance Criteria:**

- [ ] Epic prompt vs task prompt applied
- [ ] Events persisted
- [ ] Status transitions recorded (runner-side bookkeeping)
- [ ] Errors logged and mark run failed
      **Not In Scope:** Git snapshots.
      **Gotchas:** Ensure stop on context cancel; handle tracker errors gracefully.

### Task 10: App wiring (main server assembly)

**Depends on:** Tasks 2, 4, 6, 7, 8.1, 9
**Files:**

- Create: `internal/app/app.go`
- Update: `internal/app/AGENTS.md`
- Modify: `cmd/main.go`

**Purpose:** Wire config, storage impl, queue, tracker, copilot, notifier, consumer, and HTTP server routes.
**Context to rehydrate:** Architecture section.
**Outcome:** `cmd/main.go` parses optional config path, constructs zap logger, builds app with DI, starts consumer goroutine, starts HTTP server with handlers.
**How to Verify:**
Run: `go test ./internal/app -v`
Then: `go run ./cmd/main.go &` and hit health.
Expected: Server starts; queue/consumer running.
**Acceptance Criteria:**

- [ ] DI uses interfaces
- [ ] Logger injected everywhere
- [ ] Graceful shutdown
      **Not In Scope:** Endpoint logic details.
      **Gotchas:** Ensure consumer starts once; close resources on exit.

### Task 11: HTTP endpoints (per-route tasks)

**Depends on:** Task 10
**Files:**

- Create: `internal/app/routes.go`
- Create: `internal/app/middleware.go`
- Create: `internal/app/handlers/add.go`
- Create: `internal/app/handlers/list_batches.go`
- Create: `internal/app/handlers/batch_detail.go`
- Create: `internal/app/handlers/batch_progress.go`
- Create: `internal/app/handlers/list_runs.go`
- Create: `internal/app/handlers/run_detail.go`

**Purpose:** Implement gorilla/mux routing and handlers.
**Context to rehydrate:** API contract.
**Outcome:**

- POST /add?items=...&session_name=... -> enqueue batch
- GET /batches -> list batches
- GET /batches/{id} -> batch metadata + run summaries
- GET /batches/{id}/progress -> progress counts
- GET /runs -> list runs
- GET /runs/{id} -> run detail (+ optional events page cap)
  Middleware for logging/recovery.
  **How to Verify:**
  Run: `go test ./internal/app -run TestHandlers -v`
  Then curl each endpoint against running server.
  Expected: Correct JSON, status codes, logging.
  **Acceptance Criteria:**
- [ ] Query parsing and validation
- [ ] Pagination where applicable
- [ ] JSON error responses
- [ ] Logging with request IDs
      **Not In Scope:** Auth on API.
      **Gotchas:** Limit event payload size; trim items.

### Task 12: Documentation and examples

**Depends on:** Tasks 2.1, 11
**Files:**

- Modify: `README.md`

**Purpose:** Document setup, env/config, endpoints, queue/consumer model, module layout, AGENTS etiquette, Slack behavior, event logging.
**Context to rehydrate:** Prior tasks.
**Outcome:** README sections for config, running server, curl examples, storage notes, logging notes, queue model.
**How to Verify:**
Run: `grep -n "POST /add" README.md`
Expected: Up-to-date instructions.
**Acceptance Criteria:**

- [ ] Env table matches Task 2.1
- [ ] Endpoint examples for all routes
- [ ] Notes on queue/consumer and zap logging
- [ ] Mention AGENTS usage
      **Not In Scope:** UI guides.
      **Gotchas:** Redact tokens.

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes                                                                                  |
| --------------------------- | ------ | -------------------------------------------------------------------------------------- |
| Complete                    | ✓      | Covers config, queue, tracker, copilot, storage, notifier, endpoints, AGENTS etiquette |
| Accurate                    | ✓      | Paths match module layout (cmd/main.go, internal/\*) and Go tooling                    |
| Commands valid              | ✓      | go list/test and curl checks align to handlers                                         |
| YAGNI                       | ✓      | Single consumer, no extra infra, no git snapshots or PR logic                          |
| Minimal                     | ✓      | Tasks scoped to small file sets and clear outcomes                                     |
| Not over-engineered         | ✓      | Interface-first with simple in-process queue; optional Slack noop                      |
| Key Decisions documented    | ✓      | Framework, interfaces, config strategy, queue model, logging, Copilot behavior         |
| Supporting docs present     | ✓      | mux, Mongo driver, Copilot SDK, Slack, beads, zap links                                |
| Context sections present    | ✓      | Purpose/Context provided where dependencies exist                                      |
| Budgets respected           | ✓      | Tasks limited in scope/time/files                                                      |
| Outcome & Verify present    | ✓      | Each task lists observable outcome and commands                                        |
| Acceptance Criteria present | ✓      | Checklists included per task                                                           |
| Rehydration context present | ✓      | Context notes included for dependent tasks                                             |

### Rule-of-Five Passes

| Pass        | Changes Made                                                                        |
| ----------- | ----------------------------------------------------------------------------------- |
| Draft       | Structured tasks around interfaces-first architecture and module layout             |
| Correctness | Aligned to no-git-snapshot requirement, single consumer, env-first config           |
| Clarity     | Added explicit purposes, outcomes, and handler breakdown                            |
| Edge Cases  | Noted panic on missing config, single-consumer shutdown, event caps, Slack optional |
| Excellence  | Polished wording, added AGENTS etiquette, reinforced zap logging everywhere         |
