# Repositories CRUD Page Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Implement the Repositories CRUD page with validation hints and conflict handling so users can list, create, edit, and delete repositories with accurate feedback.

**Architecture:** Vanilla JS single-page view in `internal/app/ui/app.js` using existing fetch helpers and status banner; hash-based routing drives the view while preserving existing batch/run query handling. Tests rely on `node:test` with fake DOM elements and mocked fetch to validate UI behaviors.

**Tech Stack:** Vanilla JS DOM APIs, `node:test`, existing fetchJSON helpers and status rendering in `internal/app/ui/app.js`.

**Key Decisions:**

- **Validation strategy:** Enforce absolute path and non-empty name client-side before API calls — reduces needless network traffic and surfaces errors early.
- **Conflict handling:** Treat 409 delete responses (active batches) as user-facing errors shown in status banner — aligns with API semantics and keeps UX transparent.
- **Data refresh:** Reload repositories list after every mutation (create/edit/delete) — ensures UI consistency without manual state patching.

---

## Supporting Documentation

- Existing repositories UI implementation and helpers: `internal/app/ui/app.js` (RepositoriesView, fetch helpers).
- Existing tests and fake DOM scaffolding: `internal/app/ui/repositories_view.test.js`.
- Repository API handlers and data model for behavior reference: `internal/storage/mongo/repositories.go`, `internal/app/repository_handlers.go` (path requirements, conflict codes).
- Batch/restart interactions for downstream dependencies: `internal/app/ui/app.js` control plane and batch helpers (for Task 7 dependency awareness).

---

### Task 1: Wire repositories route and table rendering

**Depends on:** None  
**Files:**
- Modify: `internal/app/ui/app.js`
- Test: `internal/app/ui/repositories_view.test.js`

**Purpose:** Ensure the repositories route renders the table with repository_id/name/path/timestamps and shows the absolute-path hint on load.

**Context to rehydrate:**
- Review `RepositoriesView.routeApp` and table builders in `internal/app/ui/app.js`.
- Run existing tests in `internal/app/ui/repositories_view.test.js` to see current expectations.

**Outcome:** Visiting `#/repositories` loads repository data, renders the table, and displays the absolute path requirement hint.

**How to Verify:**  
Run: `node --test internal/app/ui/repositories_view.test.js`  
Expected: Test for rendering list and path hint passes.

**Acceptance Criteria:**
- [ ] Unit test: list render + hint (repositories_view.test.js)
- [ ] Integration/E2E: N/A
- [ ] Manual check: Route `#/repositories` shows list and hint
- [ ] Outputs match How to Verify expectations
- [ ] Interface vs implementation kept separated via helpers

**Not In Scope:** Mutation flows (create/edit/delete).

**Gotchas:** None known.

---

### Task 2: Implement create repository flow with validation

**Depends on:** Task 1  
**Files:**
- Modify: `internal/app/ui/app.js`
- Test: `internal/app/ui/repositories_view.test.js`

**Purpose:** Validate name + absolute path before POST and refresh the list after successful creation.

**Context to rehydrate:**
- Form state helpers and status banner usage in `internal/app/ui/app.js`.
- Validation expectations in tests.

**Outcome:** Submitting the create form with valid inputs posts to `/repository`, clears the form, reloads the table, and shows success; invalid inputs show inline status errors without calling the API.

**How to Verify:**  
Run: `node --test internal/app/ui/repositories_view.test.js`  
Expected: Create flow test passes (POST called once, table refreshed, status shows created).

**Acceptance Criteria:**
- [ ] Unit test: create flow (repositories_view.test.js)
- [ ] Integration/E2E: N/A
- [ ] Manual check: Invalid path shows absolute-path error without network call
- [ ] Outputs match How to Verify expectations
- [ ] Validation occurs client-side before fetch

**Not In Scope:** Edit/delete handling.

**Gotchas:** None known.

---

### Task 3: Implement edit flow with status feedback

**Depends on:** Task 2  
**Files:**
- Modify: `internal/app/ui/app.js`
- Test: `internal/app/ui/repositories_view.test.js`

**Purpose:** Allow selecting a repository, editing name/path with validation, persisting via PUT, and refreshing the list with updated timestamps.

**Context to rehydrate:**
- Editing state management in `internal/app/ui/app.js` (setEditing, loadRepositories).
- PUT expectations in tests.

**Outcome:** Selecting a row populates the edit form; submitting with valid data sends PUT to `/repository/{id}`, reloads the table, and shows “Repository updated”.

**How to Verify:**  
Run: `node --test internal/app/ui/repositories_view.test.js`  
Expected: Edit portion of the edit/delete test passes (PUT called once, updated text rendered, success status).

**Acceptance Criteria:**
- [ ] Unit test: edit flow (repositories_view.test.js)
- [ ] Integration/E2E: N/A
- [ ] Manual check: Status banner shows update success; invalid path errors inline without PUT
- [ ] Outputs match How to Verify expectations
- [ ] Editing state clears when record disappears

**Not In Scope:** Delete conflict handling.

**Gotchas:** Ensure editing state resets if repository disappears after refresh.

---

### Task 4: Handle delete with active-batch conflict

**Depends on:** Task 3  
**Files:**
- Modify: `internal/app/ui/app.js`
- Test: `internal/app/ui/repositories_view.test.js`

**Purpose:** Surface API conflicts (409 for active batches), keep item selected state consistent, and refresh list after successful delete.

**Context to rehydrate:**
- Delete action wiring in `internal/app/ui/app.js`.
- Conflict scenario in tests.

**Outcome:** First DELETE returning 409 shows user-facing error mentioning conflict/active batches; subsequent successful DELETE refreshes the list, clears editing selection, and shows success.

**How to Verify:**  
Run: `node --test internal/app/ui/repositories_view.test.js`  
Expected: Delete portion of the edit/delete test passes (two DELETE calls, conflict status text contains active/conflict, list empty after success).

**Acceptance Criteria:**
- [ ] Unit test: delete conflict + success (repositories_view.test.js)
- [ ] Integration/E2E: N/A
- [ ] Manual check: Conflict message visible; second delete clears row
- [ ] Outputs match How to Verify expectations
- [ ] Status banner reflects both conflict and success paths

**Not In Scope:** Batch restart actions (covered in Task 7).

**Gotchas:** Avoid double-disabling buttons; ensure status uses error style on conflict.

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes |
| --------------------------- | ------ | ----- |
| Complete                    | ✓ | CRUD listing + create/edit/delete covered across 4 tasks |
| Accurate                    | ✓ | File paths verified via `internal/app/ui/app.js`, `internal/app/ui/repositories_view.test.js` |
| Commands valid              | ✓ | `node --test internal/app/ui/repositories_view.test.js` exists and runs |
| YAGNI                       | ✓ | No extra features beyond CRUD/validation/conflict handling |
| Minimal                     | ✓ | 4 tasks, each within 1 prod file + 1 test file |
| Not over-engineered         | ✓ | Reuse existing fetch/status helpers; no new abstractions |
| Key Decisions documented    | ✓ | Three decisions listed in header |
| Supporting docs present     | ✓ | References to handlers, storage, tests, and UI |
| Context sections present    | ✓ | All tasks include Purpose/Context; Not In Scope when needed |
| Budgets respected           | ✓ | Tasks under 2 prod files and single outcome each |
| Outcome & Verify present    | ✓ | Each task has explicit outcome and verification command |
| Acceptance Criteria present | ✓ | Checklist per task with unit/manual markers |
| Rehydration context present | ✓ | Context to rehydrate included where dependent |

### Rule-of-Five Passes

| Pass        | Changes Made |
| ----------- | ------------ |
| Draft       | Structured four tasks aligned to CRUD flows and budgets |
| Correctness | Verified file paths/commands and acceptance criteria map to tests |
| Clarity     | Tightened wording for outcomes and verification steps |
| Edge Cases  | Highlighted conflict handling and editing-state reset considerations |
| Excellence  | Polished supporting docs and notes; self-approval recorded (no human reviewer available) |
