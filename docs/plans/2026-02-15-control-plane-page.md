# Control Plane Page Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Build a Control Plane view that groups batches by repository, shows batch items with statuses, links to run details, and safely triggers restart for failed batches.

**Architecture:** Extend the existing vanilla JS single-page UI (`internal/app/ui/app.js`) with a new `control-plane` route, reusing existing fetch helpers and routing; render repository sections with nested tables and guarded restart actions; rely on existing REST endpoints for repositories, batches, batch detail, runs, and restart.

**Tech Stack:** Vanilla JS in-browser UI (no framework), node:test for UI module tests, existing REST API handlers in Go.

**Key Decisions:**

- **Route name and placement:** Use `#/control-plane` with a new nav button—matches existing hash routing and keeps batch/run query handling intact.
- **Data fetching:** Reuse existing `fetchJSON`, `fetchRepositoriesList`, and `fetchBatchDetail` helpers; list batches per repository via `/repository/{repoid}/batches` to avoid new APIs.
- **Task listing source:** Use batch detail `items` for per-task status, fetched lazily per batch to keep initial payload small.
- **Restart guard:** Enable restart only when `batch.status === failed`, call existing `PUT /repository/{repoid}/batch/{batchid}/restart`, and refresh section + status banner so users see the result.
- **Scalability handling:** Render task sub-table inside a scrollable/collapsible container capped in height to handle 100-item batches without blowing the layout.

---

## Supporting Documentation

- UI entrypoint and routing: `internal/app/ui/app.js` (hash routing, nav, table builder, status banner patterns).
- API for repo batches: `internal/app/repository_batches_handler.go` → `GET /repository/{repoid}/batches?status=&limit=` returns `{ batches, count }`.
- Batch detail (items + status): `internal/app/routes.go` → `GET /batches/{id}` via `storage.Batch` (fields: `batch_id`, `repository_id`, `items[] {input,status,attempts}`, `status`, `session_name`).
- Restart endpoint: `internal/app/batch_restart_handler.go` → `PUT /repository/{repoid}/batch/{batchid}/restart` (only when batch status is `failed`; returns `batch_id`, `status`, `repository_id`).
- Runs listing per batch: `GET /repository/{repoid}/batch/{batchid}/runs?limit=` for linking to run details (existing in `app.js`).
- UI styling reference: `internal/app/ui/index.html` inline CSS (`.card`, `.table`, `.actions`, `.empty`, scrollable containers).
- Test patterns: `internal/app/ui/repositories_view.test.js`, `agents_view.test.js` (fake DOM, `mockJsonResponse`, `collectText`, `findFirstTag` helpers).

## Implementation Tasks

### Task 1: Author control plane test scaffold

**Depends on:** None  
**Files:**

- Create: `internal/app/ui/control_plane.test.js`

**Purpose:** Capture expected control-plane behavior (route wiring, repo/batch sections, task listing, restart guard) in tests before implementation.

**Context to rehydrate:**

- Review test helpers in `repositories_view.test.js` and `agents_view.test.js` for fake DOM and fetch mocking.

**Outcome:** Tests describe required structure and restart gating; they fail until the view is implemented.

**How to Verify:** Run `node --test internal/app/ui/control_plane.test.js` and confirm failures reflect missing control-plane implementation.

**Acceptance Criteria:**

- [ ] Tests assert nav entry/route `#/control-plane` renders repository sections.
- [ ] Tests assert batches render with Batch ID, Tasks count/labels, Status, Actions.
- [ ] Tests assert batch link navigates to runs URL.
- [ ] Tests assert restart is hidden/disabled unless status is `failed`.
- [ ] Tests assert task sub-table lists items with statuses and is scrollable/collapsible in structure.
- [ ] Verification command run and failure observed (pre-implementation).

**Not In Scope:** Implementing control plane UI or restart behavior.

**Gotchas:** Keep fixture payloads small but cover 100-item batching via generated data to assert scroll container presence.

### Task 2: Add control-plane route and layout shell

**Depends on:** Task 1  
**Files:**

- Modify: `internal/app/ui/app.js`

**Purpose:** Wire `#/control-plane` route and nav entry, render page shell with status + breadcrumbs, and stub data hooks.

**Context to rehydrate:**

- Routing helpers `getRouteFromHash`, `routeApp`, and `NAV_ITEMS` patterns.

**Outcome:** Visiting `#/control-plane` renders a control-plane card with placeholder content and uses status banner; nav highlights Control Plane.

**How to Verify:** Run `node --test internal/app/ui/control_plane.test.js` and confirm nav/route shell assertions pass (others may still fail).

**Acceptance Criteria:**

- [ ] Nav includes Control Plane and marks it active on `#/control-plane`.
- [ ] Route dispatches to new render function without breaking batch/run query handling.
- [ ] Shell shows loading state then placeholder content.
- [ ] No regressions in existing routes (batches, repositories, agents, version) per test expectations.
- [ ] Verification command run.

**Not In Scope:** Rendering repo/batch data or restart logic.

**Gotchas:** Keep hash parsing compatible with existing `?batch`/`?run` query overrides.

### Task 3: Render repositories with batch and task tables

**Depends on:** Task 2  
**Files:**

- Modify: `internal/app/ui/app.js`

**Purpose:** Populate control-plane view with repository sections, batch tables, and per-batch task sub-tables (scrollable/collapsible) using existing APIs.

**Context to rehydrate:**

- `fetchRepositoriesList`, `fetchJSON`, `fetchBatchDetail`, `buildTable`, `emptyState`, and CSS expectations from `index.html`.

**Outcome:** Control-plane shows each repository with batches (Batch ID link to runs, Tasks column with count/expand, Status, Actions placeholder) and task sub-table listing batch items with statuses; handles 100-item batches via scroll container.

**How to Verify:** Run `node --test internal/app/ui/control_plane.test.js` and confirm rendering and task table assertions pass.

**Acceptance Criteria:**

- [ ] Repositories load once; batches fetched per repository via `/repository/{id}/batches`.
- [ ] Batch rows show ID link to `/app?batch=<id>`, task count/label, status text, and actions container.
- [ ] Task sub-table lists batch items with status labels; supports at least 100 items via scrollable/collapsible container.
- [ ] Empty states shown when no repositories or batches exist.
- [ ] Verification command run.

**Not In Scope:** Restart action enablement/requests.

**Gotchas:** Avoid redundant fetches by caching batch detail where reused; guard against missing `items`.

### Task 4: Implement restart action guard and status updates

**Depends on:** Task 3  
**Files:**

- Modify: `internal/app/ui/app.js`

**Purpose:** Enable restart button only for failed batches, call restart endpoint, refresh batch detail, and reflect result in status banner.

**Context to rehydrate:**

- Restart endpoint contract in `batch_restart_handler.go`; status banner patterns in other views.

**Outcome:** Restart action appears/enabled only when batch status is `failed`, triggers PUT to restart endpoint, refreshes batch/task view, and updates status banner with success or error.

**How to Verify:** Run `node --test internal/app/ui/control_plane.test.js` ensuring restart guard and status updates assertions pass; optionally hit live endpoint against a failed batch.

**Acceptance Criteria:**

- [ ] Restart hidden/disabled unless `status === "failed"`.
- [ ] PUT `/repository/{repoid}/batch/{batchid}/restart` sent with proper IDs.
- [ ] UI refreshes batch status/items after restart response.
- [ ] Status banner shows success or error text reflecting restart result.
- [ ] Verification command run; manual restart path documented.

**Not In Scope:** Scheduler or backend restart semantics changes.

**Gotchas:** Avoid enabling restart while request in-flight; debounce repeated clicks.

### Task 5: Manual restart sanity check

**Depends on:** Task 4  
**Files:**

- None

**Purpose:** Validate restart flow against a real failed batch to ensure UI and backend align beyond unit tests.

**Context to rehydrate:**

- Ensure server running; have a known failed batch ID and repository ID.

**Outcome:** Restart action succeeds or surfaces backend error; status banner matches result.

**How to Verify:** In browser, navigate to `#/control-plane`, locate failed batch, trigger Restart, observe banner and refreshed status; optionally confirm via API `GET /batches/{id}` status pending.

**Acceptance Criteria:**

- [ ] Restart available only on failed batch.
- [ ] After restart, batch status reflects pending/in_progress on refresh.
- [ ] No JS errors in console during action.
- [ ] Banner communicates outcome.

**Not In Scope:** Automating manual run setup.

**Gotchas:** Ensure stale caches cleared before manual check.

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes |
| --------------------------- | ------ | ----- |
| Complete                    | ✓ | Covers route, rendering, restart, tests, manual check per issue |
| Accurate                    | ✓ | Paths verified (app.js, control_plane.test.js target, endpoints) |
| Commands valid              | ✓ | `node --test internal/app/ui/control_plane.test.js` exists and matches prior tests |
| YAGNI                       | ✓ | No backend changes planned; reuse existing APIs/helpers only |
| Minimal                     | ✓ | Five tasks, single-file edits per task, separate manual check |
| Not over-engineered         | ✓ | Vanilla JS only; no new abstractions or deps |
| Key Decisions documented    | ✓ | Five decisions recorded in header |
| Supporting docs present     | ✓ | Listed endpoints, files, and test references |
| Context sections present    | ✓ | Purpose/Context provided where needed |
| Budgets respected           | ✓ | ≤2 files per task, single outcome each, small step counts |
| Outcome & Verify present    | ✓ | Each task has explicit outcome and command |
| Acceptance Criteria present | ✓ | Checklist per task included |
| Rehydration context present | ✓ | Context notes on dependent tasks included |

### Rule-of-Five Passes

| Pass        | Changes Made |
| ----------- | ------------ |
| Draft       | Confirmed task ordering, dependencies, and file scopes; no structural changes needed |
| Correctness | Verified routes/endpoints/commands match codebase; ensured restart guard captured |
| Clarity     | Tightened task outcomes and verification wording for implementer readability |
| Edge Cases  | Checked scroll/100-item handling and restart gating included; no additions needed |
| Excellence  | Formatting/polish pass; plan ready for approval |
