# Fix desktop nav stacking Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Ensure the desktop navigation renders as a vertical stack with consistent spacing without affecting existing views or the status banner.

**Architecture:** Keep the current vanilla HTML/JS structure, introduce nav container classes and CSS to force a column layout on desktop, and adjust the nav rendering helper to emit the new structure. Validate behavior via DOM-focused unit tests using the existing fake document pattern.

**Tech Stack:** Vanilla HTML/CSS/JS, node:test with custom fake DOM.

**Key Decisions:**

- **Layout approach:** Use a grid/flex column on the nav container instead of inline buttons — forces vertical stacking and predictable gaps on desktop.
- **Structure:** Add nav wrapper/list classes while preserving existing button-link styling — minimizes risk to other consumers and keeps anchor semantics.
- **Validation:** Cover nav layout with a dedicated node:test using the existing FakeDocument pattern — prevents regressions without needing a browser.

---

## Supporting Documentation

- [MDN: CSS Flexible Box Layout](https://developer.mozilla.org/en-US/docs/Web/CSS/CSS_flexible_box_layout) — reference for `flex-direction: column` and gap handling for vertical stacks.
- [MDN: CSS Grid Layout](https://developer.mozilla.org/en-US/docs/Web/CSS/CSS_grid_layout) — grid column patterns with `gap` for consistent spacing.
- [MDN: nav element](https://developer.mozilla.org/en-US/docs/Web/HTML/Element/nav) — semantics for navigation sections.
- Repository examples: `internal/app/ui/run_layout.test.js` and `internal/app/ui/version_view.test.js` show the fake DOM/testing pattern and module loading with `delete require.cache`.

## Plan

### Task 1: Add desktop nav container structure and styles

**Depends on:** None  
**Files:**

- Modify: `internal/app/ui/index.html` (style block and nav container markup)

**Purpose:** Introduce nav container/list classes and desktop CSS to force vertical stacking with consistent spacing while keeping existing sidebar/sticky behavior.

**Context to rehydrate:**

- Review current sidebar/nav markup in `internal/app/ui/index.html` (`.app-shell`, `.sidebar`, `nav#nav`).
- Confirm existing button styles (`.button-link`) and sidebar sticky layout.

**Outcome:** Desktop nav area uses a dedicated container/list class that applies a column layout and spacing so nav links stack vertically without wrapping.

**How to Verify:**  
Run: `rg "nav-list" internal/app/ui/index.html && rg "nav-stack" internal/app/ui/index.html`  
Expected: style definitions for the nav list/stack classes and nav markup referencing them.

**Acceptance Criteria:**

- [ ] Nav container/list classes added in markup and styled for column layout with gap on desktop.
- [ ] Sidebar sticky behavior remains defined (no changes to top offset).
- [ ] No changes to main/status content structure.
- [ ] Command above finds both style rules and class usage.

**Not In Scope:** Mobile/hamburger behavior (handled by yaralpho-ydb.10).  
**Gotchas:** Preserve existing `.button-link` styling; new classes should compose rather than replace it.

### Task 2: Render nav with new layout classes and add nav layout test

**Depends on:** Task 1  
**Files:**

- Modify: `internal/app/ui/app.js` (nav rendering helper)
- Create: `internal/app/ui/nav_layout.test.js`

**Purpose:** Update `renderNav` to emit the new nav structure/classes and lock in vertical stacking via tests so nav links stay in a single column and status/banner layout remains untouched.

**Context to rehydrate:**

- See `renderNav` and `NAV_ITEMS` in `internal/app/ui/app.js`.
- Review test scaffolding patterns in `internal/app/ui/run_layout.test.js` and `version_view.test.js` (FakeDocument, cache busting).

**Outcome:** `renderNav` outputs nav items within the new container/list structure with classes enabling column layout; automated test guards the DOM structure and ensures nav/status nodes remain separate.

**How to Verify:**  
Run: `node --test internal/app/ui/nav_layout.test.js`  
Expected: All tests pass, confirming nav DOM structure/classes and that the status element remains independent.

**Acceptance Criteria:**

- [ ] `renderNav` uses the new container/list classes without breaking existing routes.
- [ ] Test file exercises nav rendering and asserts class/structure suitable for vertical stacking.
- [ ] Test ensures status/banner element is not mutated or reparented by nav rendering.
- [ ] `node --test internal/app/ui/nav_layout.test.js` passes.

**Not In Scope:** Styling changes outside nav (e.g., content cards, tables).  
**Gotchas:** Clear module cache in the test before requiring `app.js` to pick up global DOM fakes; mirror existing FakeDocument patterns.

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes |
| --------------------------- | ------ | ----- |
| Complete                    | ✓ | Covers nav structure, helper, and test requirements. |
| Accurate                    | ✓ | File paths verified: `internal/app/ui/index.html`, `internal/app/ui/app.js`, new `internal/app/ui/nav_layout.test.js`. |
| Commands valid              | ✓ | `rg` and `node --test internal/app/ui/nav_layout.test.js` runnable in repo. |
| YAGNI                       | ✓ | Only nav layout + test; no extra styling or features. |
| Minimal                     | ✓ | Two implementation tasks, each within file/step budgets. |
| Not over‑engineered         | ✓ | Stays with vanilla DOM/CSS; no new deps. |
| Key Decisions documented    | ✓ | Three decisions recorded in header. |
| Supporting docs present     | ✓ | MDN links and repo references included. |
| Context sections present    | ✓ | Tasks include Purpose, Context, Not In Scope/Gotchas. |
| Budgets respected           | ✓ | ≤2 production files per task; single verification outcome each. |
| Outcome & Verify present    | ✓ | Each task specifies outcome and verification command. |
| Acceptance Criteria present | ✓ | Checklists provided per task. |
| Rehydration context present | ✓ | Context to rehydrate listed per task. |

### Rule‑of‑Five Passes

| Pass        | Changes Made |
| ----------- | ------------ |
| Draft       | Structured plan with two tasks covering markup/CSS and helper/test. |
| Correctness | Validated file paths/commands and alignment to requirements. |
| Clarity     | Clarified outcomes, verification steps, and scope boundaries. |
| Edge Cases  | Noted sticky sidebar preservation and mobile scope deferral. |
| Excellence  | Added supporting docs and key decision rationale. |
