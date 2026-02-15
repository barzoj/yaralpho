# Mobile-friendly control plane layout Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Make the control plane usable on mobile by stacking repository cards, keeping tasks list scrollable horizontally, and ensuring restart actions remain reachable.

**Architecture:** Pure frontend changes in static HTML/CSS/JS; no backend contract updates. Leverage existing control-plane markup and helper builders, adding responsive classes and styling for narrow viewports.

**Tech Stack:** Vanilla HTML/CSS, browser DOM APIs in `internal/app/ui/app.js`, Node test harness in `internal/app/ui/control_plane.test.js`.

**Key Decisions:**

- **CSS-first for layout:** Use media queries and responsive utility classes instead of JS-driven layout changes — keeps render logic simple and testable.  
- **Non-intrusive DOM hooks:** Add minimal class hooks (`control-plane`, `control-card`, tasks scroll/table classes) reused by styles and tests — avoids changing data flows.  
- **Horizontal scrolling via wrappers:** Wrap tables in overflow containers rather than altering table structure — preserves existing table semantics and a11y.  

---

## Supporting Documentation

- MDN CSS Grid: https://developer.mozilla.org/en-US/docs/Web/CSS/CSS_grid_layout — patterns for responsive single-column grids.  
- MDN `overflow-x`: https://developer.mozilla.org/en-US/docs/Web/CSS/overflow-x — using overflow for horizontal scroll without clipping.  
- MDN Media Queries: https://developer.mozilla.org/en-US/docs/Web/CSS/Media_Queries/Using_media_queries — breakpoint behaviors.  

Notes:
- Use `@media (max-width: 768px)` to force single-column control-plane cards and ensure padding for tap targets.
- Apply `overflow-x: auto` with padding/border-radius on wrappers; keep table `min-width` to avoid squishing columns.

---

### Task 1: Add mobile-friendly control-plane layout hooks

**Depends on:** None  
**Files:**

- Modify: `internal/app/ui/index.html` (control-plane container/card class hooks, structure unchanged)
- Modify: `internal/app/ui/app.js:1318-1401` (ensure rendered cards/wrappers carry mobile classes)

**Purpose:** Provide responsive class hooks so CSS can stack control-plane cards and ensure card padding on narrow viewports.

**Context to rehydrate:**

- Read `internal/app/ui/index.html` for layout skeleton and existing CSS.
- Inspect `renderControlPlaneView` in `internal/app/ui/app.js` for card/container classes.

**Outcome:** Control-plane container and cards render with explicit layout classes enabling single-column stacking under 768px without changing data rendering.

**How to Verify:**  
Run: `node --test internal/app/ui/control_plane.test.js`  
Expected: tests pass; DOM includes `control-plane` container and `control-card` per repository.

**Acceptance Criteria:**

- [ ] Unit test(s): `internal/app/ui/control_plane.test.js` covers presence of layout classes.
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: Open control plane at ≤768px and observe cards stacked with intact content padding.
- [ ] Outputs match expectations from How to Verify.
- [ ] Files touched limited to listed paths; no data/API changes.

**Not In Scope:** Styling changes beyond control-plane layout; modifications to other views or navigation.

**Gotchas:** Ensure classes added do not regress desktop grid auto-fit behavior.

---

### Task 2: Make tasks table horizontally scrollable on mobile

**Depends on:** Task 1  
**Files:**

- Modify: `internal/app/ui/app.js:1210-1395` (add scroll wrappers/classes around batch and task tables)
- Modify: `internal/app/ui/index.html:70-525` (CSS for mobile overflow and spacing)
- Test: `internal/app/ui/control_plane.test.js` (assert tasks table wrapper class/structure)

**Purpose:** Prevent task lists and actions from clipping on narrow viewports by enabling horizontal scroll while keeping restart buttons reachable.

**Context to rehydrate:**

- Review `buildTasksContent` and `buildRestartAction` in `app.js`.
- Review existing `.control-table-wrap` and mobile media query styles in `index.html`.

**Outcome:** Tasks list renders inside an overflow container with `min-width` table, preserving action buttons; horizontal scroll works under 768px without clipping content.

**How to Verify:**  
Run: `node --test internal/app/ui/control_plane.test.js`  
Expected: New assertions pass verifying tasks wrapper class and restart button remains in actions cell.

**Acceptance Criteria:**

- [ ] Unit test(s): `internal/app/ui/control_plane.test.js` updated for scroll wrapper and actions visibility.
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: On ≤768px viewport, tasks table scrolls horizontally; restart button visible within scroll area.
- [ ] Outputs match expectations from How to Verify.
- [ ] Interface and implementation separation preserved; only DOM/class hooks changed.

**Not In Scope:** Changes to fetch logic, batch restart API, or pagination.

**Gotchas:** Fake DOM in tests ignores computed styles; assert class presence/structure rather than style effects.

---

### Task 3: Verify responsive behavior and restart accessibility

**Depends on:** Task 2  
**Files:**

- Modify: `internal/app/ui/control_plane.test.js` (add coverage for mobile wrappers/restart presence)

**Purpose:** Ensure automated tests guard against regressions: cards render, tasks scroll container exists, restart button accessible only for failed batches.

**Context to rehydrate:**

- Existing tests for control-plane rendering and restart flow in `control_plane.test.js`.
- Class hooks added in Tasks 1-2.

**Outcome:** Tests assert mobile-specific DOM hooks (control-plane wrapper, tasks scroll/table classes) and restart button remains reachable on failed batches.

**How to Verify:**  
Run: `node --test internal/app/ui/control_plane.test.js`  
Expected: All tests pass; new assertions validate mobile layout hooks.

**Acceptance Criteria:**

- [ ] Unit test(s): Updated `control_plane.test.js` assertions pass.
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: N/A (covered in earlier tasks).
- [ ] Outputs match expectations from How to Verify.

**Not In Scope:** Additional UI surfaces or navigation behavior.

**Gotchas:** Keep assertions resilient to unrelated text changes; target classes/elements.

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes |
| --------------------------- | ------ | ----- |
| Complete                    | ✓ | Covers stacking, scroll, restart accessibility across HTML/CSS/JS/tests. |
| Accurate                    | ✓ | File paths verified in repo (`index.html`, `app.js`, `control_plane.test.js`). |
| Commands valid              | ✓ | `node --test internal/app/ui/control_plane.test.js` exists and used in repo. |
| YAGNI                       | ✓ | No new endpoints or data shapes; strictly layout/hooks. |
| Minimal                     | ✓ | Three tasks, each within 2 prod files and single outcome. |
| Not over‑engineered         | ✓ | CSS/media query approach; no new layout JS. |
| Key Decisions documented    | ✓ | Three decisions recorded above. |
| Supporting docs present     | ✓ | MDN links for grid, overflow, media queries. |
| Context sections present    | ✓ | Each task includes Purpose and Context to rehydrate. |
| Budgets respected           | ✓ | Tasks ≤2 prod files, single verification, narrow scope. |
| Outcome & Verify present    | ✓ | Each task states outcome and exact command. |
| Acceptance Criteria present | ✓ | Checklists included per task. |
| Rehydration context present | ✓ | Context listed where prior work needed. |

### Rule‑of‑Five Passes

| Pass        | Changes Made |
| ----------- | ------------ |
| Draft       | Draft created; tasks scoped and budgets checked. |
| Correctness | Paths/commands verified against repo; no edits needed. |
| Clarity     | Language tightened around outcomes and scope boundaries. |
| Edge Cases  | Highlighted restart visibility and test resilience notes. |
| Excellence  | Polished supporting docs and acceptance criteria phrasing. |
