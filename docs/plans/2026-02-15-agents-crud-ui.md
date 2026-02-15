# Agents CRUD Page Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Add an Agents CRUD page that lists agents (id, name, runtime, status, timestamps) and supports create/edit/delete with validation, busy guards, error surfacing, and post-mutation refetches using the `/agent` endpoints.

**Architecture:** Extend the existing vanilla JS `/app` single-page UI (hash + query routing) to introduce an `agents` route and view built in `internal/app/ui/app.js`; reuse shared helpers for DOM/status handling and the existing fetchJSON wrapper for API calls. Adopt pessimistic refetch after each mutation instead of local state diffing to keep data authoritative and error handling consistent.

**Tech Stack:** Plain DOM + fetch in `internal/app/ui/app.js`; Node.js tests via `node --test` with fake DOM helpers.

**Key Decisions:**

- **Routing via hash + nav:** Add `#/agents` route and nav entry to align with existing hash-based routing (`#/version`) and avoid query collisions with batch/run parameters.
- **Pessimistic refetch after mutations:** Always re-query `/agent` after create/edit/delete to avoid stale lists and simplify state handling (no local cache invalidation logic).
- **Runtime validation bound to server contract:** Restrict runtime selector to `codex|copilot` since `agent_handlers.go` rejects other values and defaults to lowercase.
- **Busy guard client + server:** Disable edit/delete when status is `busy` and handle `409` responses to surface server-side busy/duplicate constraints.
- **Single file UI surface:** Keep UI changes contained to `internal/app/ui/app.js` to minimize touchpoints and match existing UI patterns; export a focused AgentsView API for tests.

---

## Supporting Documentation

- API contract: README “Agents” section (`POST/GET/PUT/DELETE /agent`, busy agents return `409`; body `{name, runtime}`).
- Handler behavior: `internal/app/agent_handlers.go` (valid runtimes codex/copilot, trims/lowercases, busy guard on update/delete, 409 on conflicts).
- Data shape: `internal/storage/models.go` (`Agent` fields agent_id, name, runtime, status, created_at, updated_at; statuses idle|busy).
- Existing UI patterns: `internal/app/ui/app.js` (hash routing with `getRouteFromHash`/`routeApp`, nav builder, status handling via `setStatus`/`renderBreadcrumbs`/`buildTable`).
- Test scaffolding: `internal/app/ui/version_view.test.js` (fake DOM + fetch stubs; `module.exports` usage for view hooks).

---

### Task 1: Add Agents route, nav entry, and API helpers

**Depends on:** None  
**Files:**

- Modify: `internal/app/ui/app.js` (add Agents route/nav wiring and fetch helpers for /agent)

**Purpose:** Introduce an `agents` view entry point and shared API helpers so subsequent UI work can render and refresh agent data.

**Context to rehydrate:**

- Review `routeApp`/`getRouteFromHash`/`renderNav` patterns in `app.js`.
- Note existing `fetchJSON`, `setStatus`, `renderBreadcrumbs`, and `buildTable` helpers for consistency.

**Outcome:** Navigating to `#/agents` or selecting Agents nav renders the Agents view shell using new helpers that fetch `/agent` list data.

**How to Verify:**  
Run: `node --test internal/app/ui/agents_view.test.js` (agents routing/nav tests)  
Expected: Tests covering route dispatch and nav label pass.

**Acceptance Criteria:**

- [ ] Hash route `#/agents` maps to Agents view.
- [ ] Nav highlights Agents when active.
- [ ] Fetch helper targets `/agent` and returns parsed agents list/errors.
- [ ] No regressions to existing routes (`batches`, `version`).

**Not In Scope:** Rendering forms or mutation handlers (covered in Task 2).

**Gotchas:** Ensure hash routing does not conflict with existing `batch`/`run` query handling (keep current precedence).

---

### Task 2: Implement Agents CRUD UI with validation and busy guard

**Depends on:** Task 1  
**Files:**

- Modify: `internal/app/ui/app.js` (render list table, create/edit forms, delete controls, status handling)

**Purpose:** Provide full CRUD interactions: display agents table, create new agent, edit name/runtime for idle agents, delete idle agents, surface API errors, and refetch after each mutation.

**Context to rehydrate:**

- Agent validation rules in `agent_handlers.go` (name required, runtime codex|copilot, busy agents blocked).
- Agent fields from `storage.Agent` for table columns and busy guard logic.

**Outcome:** Agents view shows id/name/runtime/status/timestamps; create/edit/delete actions validate inputs, disable edit/delete when status is `busy`, show API errors inline, and refresh the list after successful mutations.

**How to Verify:**  
Run: `node --test internal/app/ui/agents_view.test.js`  
Expected: Tests asserting list rendering, validation errors, busy guard on edit/delete, and refetch after mutations pass.

**Acceptance Criteria:**

- [ ] Table displays `agent_id`, `name`, `runtime`, `status`, `created_at`, `updated_at`.
- [ ] Create form requires name/runtime; rejects invalid runtime; shows server error text.
- [ ] Edit/delete controls are disabled for busy agents and handle `409` responses gracefully.
- [ ] After create/edit/delete, list is refetched and status bar updates.
- [ ] Status bar surfaces API failures with actionable text.

**Not In Scope:** Pagination or filtering beyond existing list limit; runtime localization/styling changes.

**Gotchas:** Avoid mutating prior state when refetching; ensure buttons re-enable after failed requests.

---

### Task 3: Add Agents view tests

**Depends on:** Task 2  
**Files:**

- Create: `internal/app/ui/agents_view.test.js`
- Modify: `internal/app/ui/app.js` (export Agents view/test hooks)

**Purpose:** Cover routing, rendering, validation, busy guards, and refetch behavior to prevent regressions.

**Context to rehydrate:**

- Test patterns from `version_view.test.js` (FakeDocument/FakeElement helpers, fetch stubs).
- Agents view behaviors added in Tasks 1–2.

**Outcome:** Automated tests exercise list rendering, create/edit/delete flows, validation errors, busy guards, and refetch triggers.

**How to Verify:**  
Run: `node --test internal/app/ui/agents_view.test.js`  
Expected: All agents view tests pass.

**Acceptance Criteria:**

- [ ] Tests cover success and error paths for list/create/edit/delete.
- [ ] Busy agents cannot be edited/deleted in tests.
- [ ] Refetch assertions ensure mutations update the table.
- [ ] No leaks into global state between tests (module cache cleared per test).

**Not In Scope:** End-to-end browser testing; styling snapshots.

**Gotchas:** Ensure module exports include Agents view helpers without breaking existing exports.

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes |
| --------------------------- | ------ | ----- |
| Complete                    | ✓ | Covers nav, CRUD UI, and tests per issue |
| Accurate                    | ✓ | Paths checked (`internal/app/ui/app.js`, new `agents_view.test.js`) |
| Commands valid              | ✓ | `node --test internal/app/ui/agents_view.test.js` matches repo pattern |
| YAGNI                       | ✓ | No extra pagination/styling beyond ask |
| Minimal                     | ✓ | Single UI file plus one new test file |
| Not over-engineered         | ✓ | Pessimistic refetch, no state manager added |
| Key Decisions documented    | ✓ | Five explicit choices recorded |
| Supporting docs present     | ✓ | README + handlers + models + UI/test refs |
| Context sections present    | ✓ | Every task has Purpose/Context/Not In Scope where needed |
| Budgets respected           | ✓ | ≤2 files per task, single outcome each |
| Outcome & Verify present    | ✓ | Each task lists outcome and verification command |
| Acceptance Criteria present | ✓ | Checklists included per task |
| Rehydration context present | ✓ | Context bullets provided for dependent tasks |

### Rule‑of‑Five Passes

| Pass        | Changes Made |
| ----------- | ------------ |
| Draft       | Confirmed task sizing/ordering; no structural changes needed |
| Correctness | Validated file paths/commands and acceptance alignment |
| Clarity     | Tightened outcomes/acceptance language for readability |
| Edge Cases  | Emphasized busy guard/reset behaviors in notes |
| Excellence  | Ensured supporting docs/key decisions polished and concise |
