# Runs list task ref & event count Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Expose task reference and total event count for each run in the runs list API and UI without regressing existing views.

**Architecture:** Extend storage to return run summaries that include event counts computed alongside deterministic sorting, surface these fields through the list runs handler, and render them in the UI table. Keep storage as the single source of truth for counts to avoid duplicated queries, and limit payloads via existing list caps.

**Tech Stack:** Go (net/http, MongoDB driver), Vanilla JS (DOM), Node test runner.

**Key Decisions:**

- **Storage shape:** Introduce a run summary struct with `total_events` instead of overloading `TaskRun` — keeps detail and summary concerns separate and avoids breaking existing callers.
- **Event counting:** Use Mongo aggregation grouped by `run_id` with a single query — more efficient and deterministic than per-run count queries.
- **Ordering:** Preserve `started_at` desc ordering in queries and response — aligns UI expectations and keeps stable pagination.
- **UI rendering:** Add explicit Task Ref and Total Events columns with default placeholders — ensures visibility even when data is missing and avoids layout regressions.
- **Testing scope:** Cover handler and UI rendering paths with targeted unit tests — minimizes reliance on integration environments while verifying schema changes.

---

## Supporting Documentation

- `internal/storage/interfaces.go`, `internal/storage/models.go` — current storage contracts and TaskRun fields.
- `internal/storage/mongo/task_runs.go` — ListTaskRuns implementation and sorting.
- `internal/storage/mongo/session_events.go` — session event persistence (source for event counts).
- `internal/app/list_runs_handler.go` — runs list HTTP handler shape.
- `internal/app/ui/app.js` — runs list rendering logic and table columns.
- Go MongoDB aggregation reference: https://pkg.go.dev/go.mongodb.org/mongo-driver/mongo#Collection.Aggregate (grouping and sorting).
- Go net/http handlers: https://pkg.go.dev/net/http — response writing patterns used in handlers.
- Node test runner: https://nodejs.org/api/test.html — existing UI tests rely on built-in `node:test`.

---

### Task 1: Define run summary shape with event counts

**Depends on:** None  
**Files:**

- Modify: `internal/storage/models.go`
- Modify: `internal/storage/interfaces.go`

**Purpose:** Add a summary struct that includes `task_ref` and `total_events` while keeping the existing TaskRun model intact for detail views.

**Context to rehydrate:**

- Read current `TaskRun` struct and `Storage.ListTaskRuns` signature.

**Outcome:** Storage exposes a `TaskRunSummary` (or similar) type with `TotalEvents` and `TaskRef` fields, and `ListTaskRuns` returns that summary type.

**How to Verify:**  
Run: `go test ./internal/storage/...`  
Expected: Tests compile against the new interface without failures.

**Acceptance Criteria:**

- [ ] Struct includes run id, batch id, task ref, status timestamps, and `total_events` JSON/BSON tags.
- [ ] Storage interface updated to return the summary type.
- [ ] No existing callers broken at compile time.
- [ ] Unit tests compile with the new shape.

**Not In Scope:** Computing counts; only type and interface definitions.

**Gotchas:** Ensure JSON/BSON tags are consistent (`total_events`) to match API output.

### Task 2: Implement Mongo run summaries with event counts

**Depends on:** Task 1  
**Files:**

- Modify: `internal/storage/mongo/task_runs.go`
- Modify: `internal/storage/mongo/client_test.go`

**Purpose:** Return run summaries with event counts using a single aggregation, sorted by `started_at` desc.

**Context to rehydrate:**

- Review existing find query sorting and session event collection fields.

**Outcome:** `ListTaskRuns` aggregates runs with their `total_events`, preserving deterministic ordering and respecting optional `batch_id` filter.

**How to Verify:**  
Run: `go test ./internal/storage/mongo/...` (with Mongo env vars set)  
Expected: ListTaskRuns integration test asserts task ref and total event counts.

**Acceptance Criteria:**

- [ ] Aggregation groups session events by `run_id` and merges counts into runs.
- [ ] Sorting remains `started_at` desc.
- [ ] Batch filter works and limits applied after sorting.
- [ ] Tests cover a run with events and without events.

**Not In Scope:** Changes to run detail queries or event listing.

**Gotchas:** Handle runs with zero events by defaulting count to 0; avoid N+1 queries.

### Task 3: Expose task ref and total events in runs list handler

**Depends on:** Task 2  
**Files:**

- Modify: `internal/app/list_runs_handler.go`
- Modify: `internal/app/handlers_test.go`

**Purpose:** Surface the new fields in the list runs API response with enforced ordering and limits.

**Context to rehydrate:**

- Review handler limit parsing and response structure.

**Outcome:** `/runs` response includes `task_ref` and `total_events` per run; tests assert presence and ordering.

**How to Verify:**  
Run: `go test ./internal/app/...`  
Expected: Handler test validates response fields and limit behavior.

**Acceptance Criteria:**

- [ ] Handler uses updated storage return type.
- [ ] Response JSON contains `task_ref` and `total_events`.
- [ ] Ordering by `started_at` desc maintained when trimming to limit.
- [ ] Tests cover runs with missing task ref (renders placeholder) and event counts.

**Not In Scope:** Changing run detail endpoint payload.

**Gotchas:** Ensure map keys in response use snake_case to match UI expectations.

### Task 4: Render task ref and total events in UI runs table

**Depends on:** Task 3  
**Files:**

- Modify: `internal/app/ui/app.js`
- Create: `internal/app/ui/runs_table.test.js`

**Purpose:** Display task reference and total event counts in the runs list table without breaking existing layout.

**Context to rehydrate:**

- Review `renderRuns` implementation and helper functions (formatDate, buildTable).

**Outcome:** Runs table shows Task Ref and Total Events columns mapped from API response, with placeholders for missing data.

**How to Verify:**  
Run: `node internal/app/ui/runs_table.test.js`  
Expected: Test asserts column headers and cell values for task ref and total events.

**Acceptance Criteria:**

- [ ] UI renders new columns with correct order and headers.
- [ ] Total events displays numeric value or fallback.
- [ ] No regressions to existing rows (run link, status, timestamps).
- [ ] Test covers mapping of response fields to table cells.

**Not In Scope:** Live events view or scrolling behavior.

**Gotchas:** Keep LIST_LIMIT usage unchanged; ensure numeric formatting uses locale-aware toString.

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes                                     |
| --------------------------- | ------ | ----------------------------------------- |
| Complete                    | ✓      | All scope items (storage, handler, UI, tests) captured |
| Accurate                    | ✓      | Paths validated via repo inspection       |
| Commands valid              | ✓      | go test and node test commands exist in repo |
| YAGNI                       | ✓      | Only fields and columns requested         |
| Minimal                     | ✓      | Four focused tasks within budgets         |
| Not over-engineered         | ✓      | Single aggregation; no extra APIs         |
| Key Decisions documented    | ✓      | Five listed                               |
| Supporting docs present     | ✓      | Linked repo files and docs                |
| Context sections present    | ✓      | Each task includes Purpose/Context        |
| Budgets respected           | ✓      | ≤2 prod files per task, single outcomes   |
| Outcome & Verify present    | ✓      | Each task includes outcome and verify cmd |
| Acceptance Criteria present | ✓      | Checklists included per task              |
| Rehydration context present | ✓      | Context bullets provided where needed     |

### Rule-of-Five Passes

| Pass        | Changes Made                             |
| ----------- | ---------------------------------------- |
| Draft       | Initial task breakdown and file lists    |
| Correctness | Added aggregation and ordering details   |
| Clarity     | Added placeholders and command expectations |
| Edge Cases  | Noted zero-event runs and missing task refs |
| Excellence  | Polished acceptance criteria and gotchas |

