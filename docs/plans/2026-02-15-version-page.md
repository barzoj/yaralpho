# Version Page Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Add a dedicated Version view that fetches `/version`, handles loading/error/non-JSON responses, and wires navigation so users can open it from the shell without breaking batch/run deep links.

**Architecture:** Extend the single-page vanilla JS shell with a lightweight hash router (`#/version`) layered atop existing `?batch`/`?run` query handling. Version view reuses shared status/banner helpers (`setStatus`, `renderBreadcrumbs`) and isolates fetch/parse logic behind a view-specific renderer for testability.

**Tech Stack:** Vanilla JS + DOM APIs; hash routing; node:test for unit tests; existing fetchJSON helper and layout markup in `index.html`.

**Key Decisions:**

- **Routing integration:** Add a hash-based route for `#/version` while preserving query-parameter flows for batches/runs — avoids breaking existing deep links.
- **Fetch handling:** Reuse `fetchJSON` but guard for non-JSON by falling back to response text — ensures graceful display when content-type mismatches.
- **UI states:** Centralize loading/success/error status messages via `setStatus` and explicit view content sections — keeps status banner consistent across views.

---

## Supporting Documentation

- API reference: `GET /version` endpoint described in `README.md` (System section) — response carries build identifier.
- UI shell markup: `internal/app/ui/index.html` sidebar/nav + content slots.
- Existing patterns: `internal/app/ui/app.js` helpers (`setStatus`, `renderBreadcrumbs`, `fetchJSON`, view scaffolding) and exports used by tests (`RunList`, `RunLayout`).
- Test style: `internal/app/ui/runs_table.test.js` shows node:test + FakeDocument/Node setup for DOM-less rendering.

---

### Task 1: Add hash router skeleton and navigation entry

**Depends on:** None  
**Files:**

- Modify: `internal/app/ui/app.js` (router/nav setup)
- Test: `internal/app/ui/version_view.test.js` (router stubs)

**Purpose:** Introduce a minimal hash router that recognizes `#/version` while keeping existing query-based batch/run behavior intact, and render a sidebar nav item for Version.

**Context to rehydrate:**

- Router entry point currently in `start()` within `app.js`.
- Sidebar nav container is `<nav id="nav">` in `index.html`.

**Outcome:** Hash changes to `#/version` invoke a version view renderer; other states continue to use batch/run views; sidebar shows a Version link (non-breaking in existing flows).

**How to Verify:**  
Run: `node --test internal/app/ui/version_view.test.js`  
Expected: Router tests assert `#/version` path dispatches version renderer and default query handling remains unchanged.

**Acceptance Criteria:**

- [ ] Hash `#/version` triggers version view hook.
- [ ] Existing `?batch`/`?run` flows still call their renderers.
- [ ] Nav contains a Version link pointing to `#/version`.
- [ ] No other routes are modified.

**Not In Scope:** Implementing version fetch/render logic; styling changes.

---

### Task 2: Implement version view rendering with loading/error handling

**Depends on:** Task 1  
**Files:**

- Modify: `internal/app/ui/app.js` (version view renderer, status updates)

**Purpose:** Add a dedicated renderer that fetches `/version`, surfaces loading/success/error states, and renders returned data (JSON or plain text) safely.

**Context to rehydrate:**

- Shared helpers in `app.js`: `setStatus`, `clearContent`, `renderBreadcrumbs`, `fetchJSON`.
- Status banner is `#status`; content slot is `#content`; view title is `#view-title`.

**Outcome:** Visiting `#/version` shows a loading message, then either version details (pretty-printed when JSON) or an error message; non-JSON responses render as text with a fallback label.

**How to Verify:**  
Run: `node --test internal/app/ui/version_view.test.js`  
Expected: Tests cover success JSON render, text fallback, and HTTP error path updating status/content appropriately.

**Acceptance Criteria:**

- [ ] Loading state shown immediately when version view starts.
- [ ] Successful JSON response renders keys/values (stringified or structured).
- [ ] Non-JSON responses display raw text without throwing.
- [ ] HTTP error sets status to error and shows retry guidance or message.
- [ ] Breadcrumbs/title updated to “Version”.

**Not In Scope:** Persisting version data or caching across navigations.

---

### Task 3: Add focused tests for version view and router

**Depends on:** Task 2  
**Files:**

- Create: `internal/app/ui/version_view.test.js`

**Purpose:** Ensure routing and view rendering handle success/error/non-JSON states and do not regress batch/run behavior.

**Context to rehydrate:**

- Test patterns from `runs_table.test.js` (FakeDocument/Node).
- Exports from `app.js` should expose version/router hooks for testing.

**Outcome:** Automated tests validate the new view logic and routing decisions, guarding against regressions.

**How to Verify:**  
Run: `node --test internal/app/ui/version_view.test.js`  
Expected: All tests pass; assertions cover success JSON, text fallback, HTTP error, and router dispatch behavior.

**Acceptance Criteria:**

- [ ] Tests cover success, non-JSON, and error branches.
- [ ] Router dispatch test ensures `#/version` calls version renderer and default path preserves prior behavior.
- [ ] No reliance on real network calls (fetch mocked).

**Not In Scope:** Browser E2E tests or visual regression tooling.

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes |
| --------------------------- | ------ | ----- |
| Complete                    | ✓ | Router + view + tests cover acceptance criteria |
| Accurate                    | ✓ | Paths verified (`app.js`, `internal/app/ui/version_view.test.js`) |
| Commands valid              | ✓ | `node --test internal/app/ui/version_view.test.js` runs in repo |
| YAGNI                       | ✓ | Only routing/view/test work scoped to version |
| Minimal                     | ✓ | Three tasks, each ≤2 prod files |
| Not over-engineered         | ✓ | No new abstractions beyond router hook and view renderer |
| Key Decisions documented    | ✓ | Three decisions recorded |
| Supporting docs present     | ✓ | Links/notes listed |
| Context sections present    | ✓ | Included in each task |
| Budgets respected           | ✓ | ≤2 production files per task; outcomes singular |
| Outcome & Verify present    | ✓ | Provided per task |
| Acceptance Criteria present | ✓ | Provided per task |
| Rehydration context present | ✓ | Provided where needed |

### Rule‑of‑Five Passes

| Pass        | Changes Made |
| ----------- | ------------ |
| Draft       | Confirmed task ordering and dependencies |
| Correctness | Validated paths/commands and acceptance checks |
| Clarity     | Tightened notes in checklist table |
| Edge Cases  | Ensured non-JSON/error handling called out |
| Excellence  | Final polish of wording and scope boundaries |
