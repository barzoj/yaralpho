# Fix Desktop Nav Stacking Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Align the desktop navigation into a vertical stack with consistent spacing while keeping mobile behavior and existing pages unaffected.

**Architecture:** Keep the current sidebar/nav container but tighten its markup and CSS for a columnar layout; adjust `renderNav` to enforce vertical stacking and active-state semantics without impacting the mobile dropdown. Validate layout via DOM-focused node tests and a manual desktop viewport check against the design.

**Tech Stack:** HTML/CSS, vanilla JS (DOM), node:test.

**Key Decisions:**

- **Layout strategy:** Use CSS grid/column classes on the sidebar nav container instead of inline styles — minimal change and consistent gaps across viewports.
- **Nav rendering:** Continue generating links in `renderNav` with centralized NAV_ITEMS — avoids duplication and keeps routing unchanged.
- **Validation:** Rely on `node --test` DOM fakes plus a manual desktop viewport check — fast feedback and visual assurance that spacing and banner alignment remain correct.

---

## Supporting Documentation

- MDN CSS Grid Layout — https://developer.mozilla.org/en-US/docs/Web/CSS/CSS_grid_layout (reference for column stacking and gap control).
- MDN `display: flex` and column direction — https://developer.mozilla.org/en-US/docs/Web/CSS/flex-direction (verify flex/grid choices for vertical nav).
- WAI-ARIA Authoring Practices: Navigation — https://www.w3.org/WAI/ARIA/apg/patterns/landmarks/examples/navigation.html (ensure nav and aria-current usage).
- Node.js `node:test` module — https://nodejs.org/api/test.html (for structuring DOM-focused tests).
- Chrome DevTools device toolbar — https://developer.chrome.com/docs/devtools/device-mode/ (desktop viewport manual verification shortcut).

---

### Task 1: Solidify sidebar markup for vertical nav stack

**Depends on:** None  
**Files:**

- Modify: `internal/app/ui/index.html` (sidebar/nav container markup and classes)

**Purpose:** Ensure the desktop nav container uses a column-friendly structure and class hooks so links naturally stack with consistent spacing.

**Context to rehydrate:** Review current sidebar structure in `internal/app/ui/index.html` around the `<aside>` navigation block.

**Outcome:** Sidebar markup exposes a stable nav container that applies the stacking classes and leaves mobile dropdown hooks intact.

**How to Verify:**  
Run: `rg "nav-stack" internal/app/ui/index.html`  
Expected: Sidebar nav container includes the stacking class on the outer nav wrapper used for desktop rendering.

**Acceptance Criteria:**

- [ ] Unit test(s): N/A (markup only)
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: Desktop load shows nav container rendered (performed in Task 4)
- [ ] Outputs match expectations from How to Verify
- [ ] Interface and implementation separation — DOM hooks preserved for JS render
- [ ] DAL layer tasks do not leak database-specific types; interface-based design for testability and flexibility (N/A)

**Not In Scope:** Changing nav link labels, routes, or mobile dropdown behavior.

**Gotchas:** Keep existing `nav-dropdown` container for mobile; avoid altering header/footer layout.

### Task 2: Enforce vertical stacking in nav render logic

**Depends on:** Task 1  
**Files:**

- Modify: `internal/app/ui/app.js` (renderNav and nav menu layout helpers)

**Purpose:** Make `renderNav` apply stacking-friendly class names and container usage so desktop nav links render as a vertical column while preserving mobile menu behavior and ARIA states.

**Context to rehydrate:**

- Review `renderNav`, `ensureNavMenuBindings`, and related nav constants in `internal/app/ui/app.js`.
- Confirm NAV_ITEMS definitions and active route handling.

**Outcome:** `renderNav` produces vertically stacked nav links on desktop, keeps button styles, and maintains active-state/ARIA without disturbing mobile dropdown logic.

**How to Verify:**  
Run: `node --test internal/app/ui/nav_layout.test.js`  
Expected: PASS showing nav-stack/nav-list classes applied and active link retained; no horizontal layout regressions.

**Acceptance Criteria:**

- [ ] Unit test(s): `internal/app/ui/nav_layout.test.js`
- [ ] Integration test(s): `internal/app/ui/nav_menu.test.js` (regression guard for menu toggling)
- [ ] Manual or E2E check: N/A (covered in Task 4)
- [ ] Outputs match expectations from How to Verify
- [ ] Interface and implementation separation - class names sourced from render logic, not scattered constants
- [ ] DAL layer tasks do not leak database-specific types; interface-based design for testability and flexibility (N/A)

**Not In Scope:** Changing routing or nav items.

**Gotchas:** Ensure desktop rendering does not depend on the mobile dropdown `data-open` attribute; keep existing focus management intact.

### Task 3: Expand nav layout test coverage

**Depends on:** Task 2  
**Files:**

- Modify: `internal/app/ui/nav_layout.test.js` (add expectations for vertical stacking and non-wrapping)

**Purpose:** Strengthen automated coverage to assert vertical stacking, spacing classes, and that status banner positioning remains unaffected.

**Context to rehydrate:** Review existing fake DOM helpers and nav assertions in `internal/app/ui/nav_layout.test.js`.

**Outcome:** Tests fail if nav renders horizontally or drops required classes/structure; active-state and status placement remain guarded.

**How to Verify:**  
Run: `node --test internal/app/ui/nav_layout.test.js`  
Expected: PASS with new assertions; failure would indicate horizontal layout or missing class hooks.

**Acceptance Criteria:**

- [ ] Unit test(s): `internal/app/ui/nav_layout.test.js` (updated)
- [ ] Integration test(s): `internal/app/ui/nav_menu.test.js` (no regressions)
- [ ] Manual or E2E check: N/A
- [ ] Outputs match expectations from How to Verify
- [ ] Interface and implementation separation - tests validate DOM contract, not implementation internals
- [ ] DAL layer tasks do not leak database-specific types; interface-based design for testability and flexibility (N/A)

**Not In Scope:** Adding new nav routes or altering NAV_ITEMS.

**Gotchas:** Keep fake DOM aligned with production nav structure; avoid brittle ordering assertions beyond class/structure essentials.

### Task 4: Manual desktop verification

**Depends on:** Task 3  
**Files:**

- Test: `internal/app/ui/nav_layout.test.js` (already executed)

**Purpose:** Confirm visually that desktop nav stacks vertically with consistent spacing and that the status banner alignment remains intact in the real DOM/CSS.

**Context to rehydrate:**

- Launch the app via `./run_ralph.sh` or `./build.sh && ./run_ralph.sh` (if needed) and open `/app`.
- Set browser width ≥ 1024px using DevTools device toolbar.

**Outcome:** Desktop view shows nav links in a single vertical column with even gaps; no horizontal wrapping; status banner and main content remain aligned.

**How to Verify:**  
Run: `node --test internal/app/ui/nav_layout.test.js && node --test internal/app/ui/nav_menu.test.js`  
Then: Load `/app` at ≥1024px width; visually confirm vertical nav column and aligned status banner.

**Acceptance Criteria:**

- [ ] Unit test(s): `internal/app/ui/nav_layout.test.js`
- [ ] Integration test(s): `internal/app/ui/nav_menu.test.js`
- [ ] Manual or E2E check: Desktop viewport manual check performed
- [ ] Outputs match expectations from How to Verify
- [ ] Interface and implementation separation - UI verified via behavior, not internal DOM tricks
- [ ] DAL layer tasks do not leak database-specific types; interface-based design for testability and flexibility (N/A)

**Not In Scope:** Mobile hamburger/menu redesign (covered by dependent task 10), content changes beyond nav layout.

**Gotchas:** Verify at desktop breakpoints only; mobile behavior is validated separately by existing tests and task 10.

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes                                     |
| --------------------------- | ------ | ----------------------------------------- |
| Complete                    | ✓      | Covers markup, render logic, tests, and manual desktop check; dependencies noted. |
| Accurate                    | ✓      | Paths verified in repo; nav_layout/nav_menu tests located in `internal/app/ui`. |
| Commands valid              | ✓      | `node --test internal/app/ui/nav_layout.test.js internal/app/ui/nav_menu.test.js` passes. |
| YAGNI                       | ✓      | Only tasks needed for desktop nav stacking; no new routes/features. |
| Minimal                     | ✓      | Four small tasks; each within file/time budgets. |
| Not over‑engineered         | ✓      | Reuses existing NAV_ITEMS and menu bindings; minimal structural changes. |
| Key Decisions documented    | ✓      | Three decisions captured in header. |
| Supporting docs present     | ✓      | MDN, WAI-ARIA, node:test, and DevTools references listed. |
| Context sections present    | ✓      | Each task includes Purpose and Context (and scope notes where relevant). |
| Budgets respected           | ✓      | ≤2 production files per task; single outcome per verification. |
| Outcome & Verify present    | ✓      | Every task states Outcome and How to Verify. |
| Acceptance Criteria present | ✓      | Checklists included for all tasks. |
| Rehydration context present | ✓      | Tasks with dependencies include context reminders. |

### Rule‑of‑Five Passes

| Pass        | Changes Made                             |
| ----------- | ---------------------------------------- |
| Draft       | Structured header, supporting docs, four scoped tasks, and verification record. |
| Correctness | Verified file paths/commands; checklist updated to ✓. |
| Clarity     | Simplified task scopes and dependencies; ensured context and scope notes are explicit. |
| Edge Cases  | Confirmed budgets, dependency order, and mobile scope exclusion while focusing on desktop. |
| Excellence  | Polished outcomes/verification language and doc references for execution readiness. |
