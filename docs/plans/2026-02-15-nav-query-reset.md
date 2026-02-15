# Nav Query Param Reset Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Ensure navigation links drop stale ?batch/?run query parameters so routing always shows the selected page.

**Architecture:** Normalize navigation targets to hash-only URLs via a helper that clears search params and updates location/history without reloads; route resolution derives params on demand to avoid stale state. Guard behavior with node:test unit tests using a fake DOM harness mirroring existing UI tests. Keep changes isolated to router/nav code paths to avoid impacting other views.

**Tech Stack:** Vanilla JS, node:test, custom fake DOM utilities in `internal/app/ui`.

**Key Decisions:**

- **Navigation URL normalization:** Build nav hrefs with a helper that strips search params and rewrites to hash-only URLs — prevents stale batch/run params from persisting across sections.
- **Query parsing strategy:** Parse `window.location.search` at route time instead of capturing at module init — avoids stale params after navigation changes search.
- **History update mechanism:** Use `history.replaceState` fallback to `location.assign` when absent — keeps SPA flow without reload while still working in minimal test DOMs.

---

## Supporting Documentation

- MDN `URLSearchParams` for parsing query parameters reliably across browsers: https://developer.mozilla.org/en-US/docs/Web/API/URLSearchParams
- MDN `Location.hash` and `hashchange` behavior for hash-based routing: https://developer.mozilla.org/en-US/docs/Web/API/Window/hashchange_event
- MDN `history.replaceState` for updating URL without reload: https://developer.mozilla.org/en-US/docs/Web/API/History/replaceState
- Node.js `node:test` module (assert/test) used across repo: https://nodejs.org/api/test.html

## Tasks

### Task 1: Add router/nav regression tests for query clearing

**Depends on:** None  
**Files:**

- Create: `internal/app/ui/app_router.test.js`
- Modify: `internal/app/ui/app.js` (test harness exports reuse)
- Test: `internal/app/ui/app_router.test.js`

**Purpose:** Capture the current bug by asserting nav links clear ?batch/?run and routing still works from batch/run contexts.  
**Context to rehydrate:**

- Review existing fake DOM helpers in `nav_menu.test.js` for setup patterns.
- Note current module exports at bottom of `app.js` (VersionView/NavMenu).

**Outcome:** Failing test demonstrating that nav links retain query params and block hash routing.  
**How to Verify:** Run `node --test internal/app/ui/app_router.test.js` and observe failing assertions on URL normalization.  
**Acceptance Criteria:**

- [ ] Unit test covers navigation from URLs containing `?batch` and `?run`.
- [ ] Unit test asserts hash-only URLs are produced for each nav item.
- [ ] Unit test documents expected routing behavior post-click.
- [ ] No reliance on real DOM or network; uses fake DOM harness.
- [ ] One primary outcome: test fails with current code.

**Not In Scope:** Fixing nav dropdown accessibility; broader router refactors.  
**Gotchas:** Ensure fake `window.location` includes `pathname` for building absolute hrefs.

### Task 2: Normalize nav routing to drop query params

**Depends on:** Task 1  
**Files:**

- Modify: `internal/app/ui/app.js` (nav href builder, click handling, query parsing logic)
- Test: `internal/app/ui/app_router.test.js`

**Purpose:** Implement hash-only navigation and fresh query parsing so stale batch/run params no longer block routing.  
**Context to rehydrate:**

- Read `renderNav`, `getRouteFromHash`, and `routeApp` in `app.js`.
- Confirm nav menu bindings reuse `getNavLinks` for adding click handlers.

**Outcome:** Nav clicks rewrite URL to clean hash routes, and routing resolves based on current URL (no stale params).  
**How to Verify:** Run `node --test internal/app/ui/app_router.test.js`; expected PASS with normalized URLs and routing.  
**Acceptance Criteria:**

- [ ] Navigation links update URL without `?batch`/`?run`.
- [ ] `routeApp` derives params from current location, not cached module state.
- [ ] Hash routing still works when landing directly on run/batch URLs.
- [ ] No double navigation or regressions in existing nav menu behaviors.

**Not In Scope:** Changing nav layout or styling; modifying API fetch logic.  
**Gotchas:** Use `history.replaceState` when available to avoid reloads; fall back safely for environments lacking History API.

### Task 3: Full verification

**Depends on:** Task 2  
**Files:**

- Test: `internal/app/ui/app_router.test.js`

**Purpose:** Confirm behavior via automated tests and minimal manual check guidance.  
**Context to rehydrate:**

- Ensure test harness still loads `app.js` cleanly after implementation.

**Outcome:** Test suite passes; documented manual check flow for QA.  
**How to Verify:** Run `node --test internal/app/ui/app_router.test.js`; optionally open app and click nav from a `?batch=...&run=...` URL confirming hash-only navigation.  
**Acceptance Criteria:**

- [ ] Automated test pass.
- [ ] Manual check instructions present for QA.
- [ ] No leftover debug logs or test-only hooks in production code.
- [ ] Commit created referencing task ID.

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes                                     |
| --------------------------- | ------ | ----------------------------------------- |
| Complete                    | ✓      | Covers tests, implementation, verification|
| Accurate                    | ✓      | Paths/commands verified for app.js and test|
| Commands valid              | ✓      | `node --test internal/app/ui/app_router.test.js` |
| YAGNI                       | ✓      | Only nav query normalization work scoped  |
| Minimal                     | ✓      | 3 tasks, ≤2 prod files each               |
| Not over-engineered         | ✓      | Simple helpers + existing patterns        |
| Key Decisions documented    | ✓      | Three decisions recorded                  |
| Supporting docs present     | ✓      | MDN and node:test links listed            |
| Context sections present    | ✓      | Purpose/Context/Outcome provided per task |
| Budgets respected           | ✓      | Tasks ≤2 prod files and single outcomes   |
| Outcome & Verify present    | ✓      | Present per task                          |
| Acceptance Criteria present | ✓      | Present per task                          |
| Rehydration context present | ✓      | Included where needed                     |

### Rule-of-Five Passes

| Pass        | Changes Made                             |
| ----------- | ---------------------------------------- |
| Draft       | Initial structure with 3 tasks           |
| Correctness | Verified paths/commands; no content changes|
| Clarity     | Confirmed scope wording and outcomes      |
| Edge Cases  | Ensured Gotchas/Not In Scope recorded     |
| Excellence  | Final polish; ready for approval          |
