# Mobile Hamburger Navigation Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Implement an accessible mobile hamburger nav that overlays dropdown links without shifting content and closes on escape or outside click.

**Architecture:** Reuse the existing nav dropdown container and toggle button; drive visibility via data attributes and ARIA state so desktop layout remains unchanged. Keep logic in vanilla JS (app.js) with document-level listeners for outside click, escape, focus trap, and resize reset. Use CSS already present in index.html for mobile overlay, only adjusting markup/attributes as needed.

**Tech Stack:** Vanilla HTML/CSS/JS, Node 20+ test runner (`node --test`).

**Key Decisions:**

- **Dropdown overlay via data attributes:** Use `data-open` + `aria-hidden` on the existing `.nav-dropdown` to avoid new containers and keep desktop static layout. — Minimal changes preserve styling.
- **JS-managed focus + dismissal:** Handle outside click, escape, and tab wrap in JS rather than CSS tricks. — Ensures accessibility and deterministic behavior across browsers.
- **Tests with fake DOM:** Use node:test with lightweight fake DOM objects to validate nav behaviors. — Fast, headless validation without external deps.

---

## Supporting Documentation

- WAI-ARIA Authoring Practices – Disclosure / Menu Button patterns: https://www.w3.org/WAI/ARIA/apg/patterns/menu-button/ (ARIA expectations for toggles, focus management, escape/outside click).
- MDN `aria-expanded` and `aria-controls`: https://developer.mozilla.org/en-US/docs/Web/Accessibility/ARIA/Attributes/aria-expanded (state semantics for toggles).
- MDN `Element.contains()` and focus trapping basics: https://developer.mozilla.org/en-US/docs/Web/API/Node/contains (used for outside click detection).

Notes: Mobile breakpoint is 768px (`NAV_MOBILE_BREAKPOINT`); dropdown visibility is controlled via `data-open` + `aria-hidden`; focus trap cycles tab order within nav links.

---

### Task 1: Align nav markup for mobile hamburger

**Depends on:** None  
**Files:**

- Modify: `internal/app/ui/index.html` (nav toggle markup/attributes, dropdown container)

**Purpose:** Ensure the HTML markup and ARIA wiring support the hamburger toggle and overlay behavior without affecting desktop layout.

**Context to rehydrate:** Open `internal/app/ui/index.html` around the sidebar navigation and mobile media queries.

**Outcome:** Nav toggle and dropdown container expose correct ARIA attributes and structure for mobile overlay without shifting content.

**How to Verify:** Run `node --test internal/app/ui/nav_menu.test.js` (structural assertions cover attributes/visibility).

**Acceptance Criteria:**

- [ ] Nav toggle has `aria-expanded` and `aria-controls` pointing at dropdown.
- [ ] Dropdown defaults to hidden with `data-open="false"` and `aria-hidden="true"` on mobile.
- [ ] Desktop layout unchanged.
- [ ] Unit test: `internal/app/ui/nav_menu.test.js`.

**Not In Scope:** Styling changes beyond minimal attribute/structure needs.

---

### Task 2: Enhance nav menu logic for mobile interactions

**Depends on:** Task 1  
**Files:**

- Modify: `internal/app/ui/app.js` (nav toggle handlers, focus trap, dismissal, resize reset)

**Purpose:** Ensure mobile hamburger toggles open/close state, traps focus, and dismisses on escape or outside click while keeping nav links functional.

**Context to rehydrate:** Review `NavMenu` helpers in `app.js` (`setNavMenuVisibility`, `openNavMenu`, `closeNavMenu`, `handleNavKeydown`, `ensureNavMenuBindings`).

**Outcome:** Mobile nav opens/closes via toggle, escape, outside click, or link click; focus cycles within menu; desktop resize resets state.

**How to Verify:** Run `node --test internal/app/ui/nav_menu.test.js` (behavioral assertions for open/close, focus trap, resize).

**Acceptance Criteria:**

- [ ] `data-open`/`aria-hidden` and `aria-expanded` reflect open state.
- [ ] Outside click and Escape close the menu; resize to desktop clears open state.
- [ ] Focus trap cycles first/last nav links on Tab/Shift+Tab when open.
- [ ] Nav links close the menu and still navigate via `navigateToRoute`.
- [ ] Unit test: `internal/app/ui/nav_menu.test.js`.

**Gotchas:** Avoid leaving document listeners attached after close; ensure resize handler doesn’t reopen menu on desktop.

---

### Task 3: Strengthen nav menu tests

**Depends on:** Task 2  
**Files:**

- Modify: `internal/app/ui/nav_menu.test.js` (add/adjust cases for hamburger interactions)

**Purpose:** Add coverage for hamburger open/close flows, focus trapping, outside click, escape, and resize reset to guard against regressions.

**Context to rehydrate:** Existing fake DOM test helpers in `nav_menu.test.js`; NavMenu exports from `app.js`.

**Outcome:** Tests assert ARIA state, dismissal behaviors, focus trap, and desktop reset across the nav menu.

**How to Verify:** Run `node --test internal/app/ui/nav_menu.test.js`.

**Acceptance Criteria:**

- [ ] Tests cover open/close toggling, outside click, Escape, focus wrap, and desktop resize reset.
- [ ] Fake DOM includes nav toggle + dropdown + nav items wired to NavMenu.
- [ ] All assertions pass via `node --test internal/app/ui/nav_menu.test.js`.

**Not In Scope:** Broader UI integration or E2E browser tests.

---

### Task 4: Validate nav menu behavior

**Depends on:** Task 3  
**Files:**

- Test: `internal/app/ui/nav_menu.test.js`

**Purpose:** Execute automated verification to confirm hamburger nav behavior meets acceptance criteria.

**Context to rehydrate:** Ensure dependencies installed; tests use Node’s built-in runner (`node --test`).

**Outcome:** Test suite passes for nav menu logic, confirming mobile overlay accessibility and dismissal behavior.

**How to Verify:** Run `node --test internal/app/ui/nav_menu.test.js`.

**Acceptance Criteria:**

- [ ] Command exits 0.
- [ ] Output shows all nav menu tests passing.
- [ ] No regressions in nav menu logic observed.

**Not In Scope:** Manual visual QA (can be done separately at /app on mobile width).

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes                                     |
| --------------------------- | ------ | ----------------------------------------- |
| Complete                    | ✓      | Covers markup, JS logic, tests, and verification for mobile nav. |
| Accurate                    | ✓      | Paths validated: index.html, app.js, nav_menu.test.js. |
| Commands valid              | ✓      | node --test internal/app/ui/nav_menu.test.js. |
| YAGNI                       | ✓      | Limited to nav overlay; no extra styling/features. |
| Minimal                     | ✓      | Four tasks, each ≤2 files and single outcome. |
| Not over-engineered         | ✓      | Reuses existing dropdown/toggle wiring. |
| Key Decisions documented    | ✓      | Three decisions captured. |
| Supporting docs present     | ✓      | WAI-ARIA and MDN references listed. |
| Context sections present    | ✓      | Purpose/context included where needed. |
| Budgets respected           | ✓      | File/time/outcome budgets observed. |
| Outcome & Verify present    | ✓      | Each task has explicit outcome and command. |
| Acceptance Criteria present | ✓      | Unit test criteria per task recorded. |
| Rehydration context present | ✓      | Context notes provided for dependent tasks. |

### Rule-of-Five Passes

| Pass        | Changes Made                             |
| ----------- | ---------------------------------------- |
| Draft       | Structured tasks, goals, decisions, and supporting docs. |
| Correctness | Verified paths/commands and acceptance alignment. |
| Clarity     | Tightened scope and phrasing for outcomes/verify. |
| Edge Cases  | Captured outside click, escape, resize behaviors in criteria. |
| Excellence  | Ready for execution; no further polish needed. |
