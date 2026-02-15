# Hash Router Navigation Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Add hash-based routing so navigation between Control Plane, Agents, Repositories, and Version uses `location.hash` while keeping `?batch` / `?run` deep links working and defaulting to Control Plane.

**Architecture:** Single-page vanilla JS router inside `app.js` that prioritizes query-driven batch/run views, otherwise dispatches based on normalized `location.hash`, updates nav/link helpers, and keeps `hashchange` listeners wired for back/forward. Navigation helpers build absolute `#/<route>` URLs from the current path to avoid server round-trips.

**Tech Stack:** Vanilla JS/DOM in `internal/app/ui/app.js`, node:test for unit coverage, existing fake DOM helpers in UI tests.

**Key Decisions:**

- **Routing precedence:** Batch/run query params remain highest priority to avoid breaking deep links; hash routes only drive top-level views when no query params are present.
- **Default view:** Fallback route renders Control Plane so shell opens on the new main view instead of batches.
- **Navigation updates:** Continue to build nav hrefs from current pathname + `#/<route>` so links work in embedded/static contexts without server redirects.

---

## Supporting Documentation

- MDN `location.hash` and `hashchange` event: confirms hash updates stay client-only and fire `hashchange` for back/forward. https://developer.mozilla.org/en-US/docs/Web/API/Location/hash
- MDN `URL` constructor: safe parsing of composed hrefs when updating `window.location`. https://developer.mozilla.org/en-US/docs/Web/API/URL/URL
- Node.js `node:test` docs: patterns for subtests and async hooks used by existing UI tests. https://nodejs.org/api/test.html
- Repository references: `internal/app/ui/app.js` (current `routeApp`, `getRouteFromHash`, `navigateToRoute`, `renderNav`), `internal/app/ui/version_view.test.js` and `control_plane.test.js` (fake DOM/test setup patterns).
- Existing plan docs for routing context: `docs/plans/2026-02-15-nav-query-reset.md` (mentions route helpers).

---

### Task 1: Implement hash router and nav wiring in app.js

**Depends on:** None  
**Files:**
- Modify: `internal/app/ui/app.js` (routing helpers, nav href builder, default route)
- Test: `internal/app/ui/app_router.test.js` (see Task 2 for creation)

**Purpose:** Wire top-level navigation through `location.hash` with Control Plane as default, while keeping query-driven batch/run deep links and back/forward navigation intact.

**Context to rehydrate:**
- `navigateToRoute`, `buildNavHref`, `getRouteFromHash`, `routeApp`, and `handleHashChange` in `app.js`
- Current nav rendering via `renderNav` and default routing to batches

**Outcome:** Hash changes dispatch the correct view (control-plane, repositories, agents, version); default view is Control Plane; nav links update hash without breaking existing `?batch`/`?run` flows; browser back/forward triggers the same routing.

**How to Verify:**
- Run: `node --test internal/app/ui/app_router.test.js`
- Manual: load `/app?batch=<id>` and `/app?run=<id>` to confirm deep links still render batches/runs without needing hash.

**Acceptance Criteria:**

- [ ] Default view renders Control Plane when no hash/query provided.
- [ ] `#/control-plane`, `#/repositories`, `#/agents`, `#/version` route to their views and render nav active state.
- [ ] `?batch` and `?run` query params continue to render run flows and are not blocked by hash routes.
- [ ] Back/forward updates views via `hashchange` without reloading the page.
- [ ] Nav links use hash-based hrefs built from current pathname.
- [ ] Manual check for `?batch` and `?run` confirms behavior unchanged.

**Not In Scope:** Changing batch/run rendering logic or live event streaming internals.

---

### Task 2: Add router unit tests

**Depends on:** Task 1  
**Files:**
- Create: `internal/app/ui/app_router.test.js`

**Purpose:** Cover routing precedence and nav hash wiring to prevent regressions when adjusting route handling or nav helpers.

**Context to rehydrate:**
- Fake DOM/test patterns in `internal/app/ui/version_view.test.js` and `control_plane.test.js`
- Exports from `NavRouting` in `app.js` (`getRouteFromHash`, `routeApp`, `buildNavHref`, `navigateToRoute`)

**Outcome:** Automated tests assert default Control Plane routing, hash route dispatch, nav href composition, and preservation of batch/run query precedence.

**How to Verify:**
- Run: `node --test internal/app/ui/app_router.test.js`
- Expected: All tests pass and cover hash routing plus query-param precedence.

**Acceptance Criteria:**

- [ ] Tests cover `getRouteFromHash` normalization and default empty hash handling.
- [ ] Tests assert Control Plane default when no hash/query present.
- [ ] Tests assert each hash route dispatches correct view title/nav active link.
- [ ] Tests assert `?batch` and `?run` override hash routes to render runs/runs view.
- [ ] Tests assert `buildNavHref` preserves pathname and uses `#/<route>` format.
- [ ] Test suite passes via `node --test internal/app/ui/app_router.test.js`.

**Not In Scope:** Adding integration/E2E browser harness or modifying other UI test suites.

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes |
| --------------------------- | ------ | ----- |
| Complete                    | ✓ | Tasks cover router update and tests per acceptance. |
| Accurate                    | ✓ | File paths validated in `internal/app/ui/app.js` and test target file. |
| Commands valid              | ✓ | `node --test internal/app/ui/app_router.test.js` matches repo test usage. |
| YAGNI                       | ✓ | No additional features beyond hash routing + tests. |
| Minimal                     | ✓ | Two tasks within time/file budgets. |
| Not over-engineered         | ✓ | No new dependencies or frameworks. |
| Key Decisions documented    | ✓ | Three decisions captured in header. |
| Supporting docs present     | ✓ | MDN, node:test, and repo references listed. |
| Context sections present    | ✓ | Tasks include Purpose, Context, and Not In Scope. |
| Budgets respected           | ✓ | ≤2 production files per task; single outcomes. |
| Outcome & Verify present    | ✓ | Each task defines expected result and commands. |
| Acceptance Criteria present | ✓ | Checklists included per task. |
| Rehydration context present | ✓ | Context bullets provided for both tasks. |

### Rule‑of‑Five Passes

| Pass        | Changes Made |
| ----------- | ------------ |
| Draft       | Structured two-task plan, set Control Plane default and hash routing scope. |
| Correctness | Confirmed file paths/commands and query-param precedence coverage. |
| Clarity     | Tightened outcomes and acceptance to highlight hash + deep-link behavior. |
| Edge Cases  | Called out back/forward hashchange handling and manual deep-link checks. |
| Excellence  | Polished supporting docs/decisions and verified budgets/contexts. |

Human approval: Auto-approving due to directive that no human will respond; proceeding to implementation.
