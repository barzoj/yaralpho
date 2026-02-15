# Mobile Hamburger Nav Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** On sub-768px viewports, replace the sidebar nav buttons with a hamburger button that opens an accessible dropdown containing the existing nav links without shifting content.

**Architecture:** Keep the existing `NAV_ITEMS` data and nav rendering, wrap it in a toggleable container controlled by a mobile-only hamburger button. Use CSS to hide the inline nav and show the button on small screens, with an absolutely positioned dropdown overlay that avoids layout shifts. JavaScript manages ARIA state, outside-click/escape close, focus trapping across the button and nav links, and resets on resize.

**Tech Stack:** Vanilla HTML/CSS, browser DOM APIs, Node 20 `node:test` with lightweight DOM fakes.

**Key Decisions:**

- **Hamburger as `<button>` with ARIA state:** Use a real button with `aria-expanded`, `aria-controls`, and `aria-label` for accessibility rather than a div or anchor.
- **CSS overlay dropdown:** Use an absolutely positioned dropdown container attached to the sidebar to avoid reflow and content shift when opening/closing on mobile.
- **JS-managed focus trap:** Trap focus between the hamburger button and nav links while open, closing on ESC/outside click and clearing on resize to desktop widths for consistent keyboard UX.

---

## Supporting Documentation

- WAI-ARIA menu button pattern: https://www.w3.org/WAI/ARIA/apg/patterns/menu-button/ — guidance on `aria-expanded`, `aria-controls`, and ESC handling.
- MDN `aria-expanded`: https://developer.mozilla.org/en-US/docs/Web/Accessibility/ARIA/Attributes/aria-expanded — how to reflect open state in controls.
- MDN `focus` and keyboard events: https://developer.mozilla.org/en-US/docs/Web/API/Element/focus — ensures deterministic focus trapping.
- Existing UI test pattern in `internal/app/ui/nav_layout.test.js` — use FakeDocument/FakeElement to avoid adding jsdom.

## Tasks

### Task 1: Add mobile nav markup and styles

**Depends on:** None  
**Files:**

- Modify: `internal/app/ui/index.html`

**Purpose:** Introduce a hamburger button, dropdown container, and responsive styles that hide the inline nav and show the button on sub-768px widths while keeping desktop layout unchanged.  
**Context to rehydrate:** Review current nav markup (`nav#nav` in `index.html`) and sidebar styles; confirm media queries near the 900px breakpoint.  
**Outcome:** Mobile view shows a hamburger button; dropdown markup exists (hidden by default) and overlays without shifting content.  
**How to Verify:** Run a local static serve (e.g., `python -m http.server 8000`) from `internal/app/ui`, open `/app` at <768px width, confirm the button is visible, nav list hidden until open, and page layout unchanged.  
**Acceptance Criteria:**

- [ ] Hamburger button renders at <768px; desktop layout unaffected.
- [ ] Dropdown container exists and is hidden by default; no layout shift when toggled.
- [ ] Nav links remain functional and retain existing styles when shown.
- [ ] Styles scoped to avoid affecting other components.

**Not In Scope:** JavaScript toggle logic, ARIA state updates, focus trapping (handled in Task 2).  
**Gotchas:** Keep CSS selectors narrow to avoid affecting `.nav-stack` on desktop; ensure z-index keeps overlay above main content.

### Task 2: Implement nav toggle logic with tests

**Depends on:** Task 1  
**Files:**

- Modify: `internal/app/ui/app.js`
- Create: `internal/app/ui/nav_menu.test.js`

**Purpose:** Add JS to manage the mobile dropdown: toggle open/close, sync ARIA attributes, handle outside click/ESC/resize, trap focus within the button and nav links, and expose helpers for testing.  
**Context to rehydrate:** Use `renderNav` (lines ~272) and module exports (lines ~2440) in `app.js`; mirror FakeDocument/FakeElement patterns from `nav_layout.test.js` for DOM tests.  
**Outcome:** Hamburger button toggles the dropdown; ARIA state updates; focus cycles within menu while open; ESC or outside click closes; resize to desktop resets state; tests cover happy path and closures.  
**How to Verify:** Run `node --test internal/app/ui/nav_menu.test.js` (all tests pass). Manually open `/app` at <768px, toggle the menu, ensure ESC/outside click close and tab focus cycles between button and first/last link.  
**Acceptance Criteria:**

- [ ] `renderNav` populates dropdown container without breaking desktop nav.
- [ ] `aria-expanded` and `aria-controls` reflect open state; dropdown has `aria-hidden` when closed.
- [ ] Outside click and ESC close the menu; resize to ≥768px closes and cleans listeners.
- [ ] Focus trapping keeps tab order inside button/nav links while open.
- [ ] `node --test internal/app/ui/nav_menu.test.js` passes.

**Not In Scope:** Visual redesign of nav links; navigation routing changes.  
**Gotchas:** Ensure listeners are cleaned to avoid duplicates across rerenders; guard for missing DOM nodes to keep tests deterministic.

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes                                                                 |
| --------------------------- | ------ | --------------------------------------------------------------------- |
| Complete                    | ✓      | Covers markup, styling, JS logic, and tests per issue acceptance.     |
| Accurate                    | ✓      | File paths verified in repo (`internal/app/ui/index.html`, `app.js`). |
| Commands valid              | ✓      | `node --test internal/app/ui/nav_menu.test.js` available with node 20.|
| YAGNI                       | ✓      | Only addresses mobile nav toggle; no extra UI beyond requirement.     |
| Minimal                     | ✓      | Two tasks, each scoped to ≤2 prod files.                              |
| Not over-engineered         | ✓      | Uses existing NAV_ITEMS and Fake DOM pattern; no new deps.            |
| Key Decisions documented    | ✓      | Three decisions recorded.                                             |
| Supporting docs present     | ✓      | Links to ARIA/MDN and existing test pattern.                          |
| Context sections present    | ✓      | Tasks include Purpose, Context, Not In Scope, Gotchas.                |
| Budgets respected           | ✓      | Tasks touch ≤2 prod files and single outcome each.                    |
| Outcome & Verify present    | ✓      | Each task lists outcome and verification.                             |
| Acceptance Criteria present | ✓      | Checklists included per task.                                         |
| Rehydration context present | ✓      | Context bullets provided where needed.                                |

### Rule‑of‑Five Passes

| Pass        | Changes Made                                                            |
| ----------- | ----------------------------------------------------------------------- |
| Draft       | Initial structure, tasks split by markup vs logic/tests.               |
| Correctness | Verified file paths/commands; clarified responsive breakpoint targets. |
| Clarity     | Tightened Purpose/Outcome wording; added context and Not In Scope.     |
| Edge Cases  | Added resize cleanup, listener cleanup, focus trap notes.              |
| Excellence  | Scoped CSS/Gotchas, ensured a11y/ARIA expectations explicit.           |
