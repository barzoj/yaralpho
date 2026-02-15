# Mobile-friendly repositories layout Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Make the repositories page usable on small screens by stacking forms, providing horizontal scroll for tables, and maintaining accessible navigation.

**Architecture:** Extend existing static HTML/CSS and vanilla JS rendering in `internal/app/ui` by adding responsive utility classes and lightweight DOM wrappers that do not alter data flow or endpoints.

**Tech Stack:** Vanilla JS + DOM APIs, inline CSS in `index.html`, node:test for UI view tests.

**Key Decisions:**

- **Responsive layout approach:** Use CSS grid/stack helpers and a scroll wrapper around tables—avoids JS media queries and keeps semantics intact.
- **Table scroll behavior:** Apply `overflow-x: auto` with `position: sticky` headers—improves mobile usability without changing column structure.
- **ARIA/labels:** Add aria-labels to forms and containers rather than changing form fields—minimizes DOM churn and keeps tests stable.
- **Testing strategy:** Rely on node:test DOM fakes and structural assertions—matches existing UI test style and avoids browser deps.
- **CSS placement:** Keep new tokens in `index.html` alongside existing view styles—single source for UI tokens and no extra assets.

---

## Supporting Documentation

- MDN Responsive design basics: https://developer.mozilla.org/en-US/docs/Learn/CSS/CSS_layout/Responsive_Design — confirm use of `max-width` breakpoints and stacking via `display: grid;` with `auto-fit`.
- MDN Scroll containers: https://developer.mozilla.org/en-US/docs/Web/CSS/overflow-x — use `overflow-x: auto` and padding to avoid clipped shadows.
- MDN Sticky table headers: https://developer.mozilla.org/en-US/docs/Web/CSS/position#sticky — apply `position: sticky; top: 0;` to `<th>` rows for horizontal scroll contexts.
- WAI-ARIA Authoring Practices: https://www.w3.org/WAI/ARIA/apg/practices/names-and-descriptions/ — ensure `aria-label` on forms/sections for screen readers.
- Existing repo UI patterns: `internal/app/ui/nav_layout.test.js`, `control_plane.test.js`, `repositories_view.test.js` for DOM helpers and class naming expectations.

---

## Tasks

### Task 1: Add responsive styles for repositories section

**Depends on:** None  
**Files:**
- Modify: `internal/app/ui/index.html` (CSS: add repo-specific responsive classes, scroll wrapper styles)

**Purpose:** Provide CSS helpers so repository forms stack on small screens and tables scroll horizontally without clipping content.

**Context to rehydrate:**
- Review existing breakpoints in `index.html` (@media 768px, 900px)
- Inspect current `.form-grid`, `.actions`, and table styles

**Outcome:** New `.stack-sm` and `.scroll-card` styles exist; repository layout uses full-width stacked forms and a scrollable table wrapper under 768px.

**How to Verify:**  
Run: `node --test internal/app/ui/repositories_view.test.js`  
Expected: Tests pass; responsive class assertions updated to new styles.

**Acceptance Criteria:**
- [ ] CSS defines reusable stack helper for mobile (<768px) covering form grids and cards
- [ ] Scroll wrapper styles include horizontal scroll with padding and sticky headers support
- [ ] No visual regressions for desktop grid; existing tokens untouched outside repo styles
- [ ] Manual: narrow viewport shows stacked cards and horizontal scroll bar visible

**Not In Scope:** Changing data fetching or table columns; nav/menu layout handled elsewhere.

**Gotchas:** Ensure sticky header works with scroll container (`position: sticky` requires `th` background and z-index).

### Task 2: Update repositories view markup for responsiveness and accessibility

**Depends on:** Task 1  
**Files:**
- Modify: `internal/app/ui/app.js` (RepositoriesView layout/classes/aria)

**Purpose:** Apply new CSS helpers to repositories view, wrapping table in a scroll container and labeling forms for small screens.

**Context to rehydrate:**
- `renderRepositoriesView` in `app.js`
- Existing button/input class usage from Task 11 (form styling)

**Outcome:** Repositories view uses stacked layout classes on forms, wraps table in a scrollable div with sticky header support, and adds aria-labels for create/edit sections.

**How to Verify:**  
Run: `node --test internal/app/ui/repositories_view.test.js`  
Expected: DOM structure includes scroll wrapper and aria labels; tests assert stacking classes.

**Acceptance Criteria:**
- [ ] Create/edit forms apply stack/width classes and aria-labels for section identification
- [ ] Table lives in a scrollable container with sticky header styling hook
- [ ] No regression to CRUD behaviors or status messaging
- [ ] Manual: On <768px, forms span full width and table scrolls horizontally without clipping buttons

**Not In Scope:** Changing API calls or validation rules.

**Gotchas:** Keep button ordering stable for tests; ensure new wrapper does not drop existing `card` styling.

### Task 3: Expand repositories view tests for mobile layout

**Depends on:** Task 2  
**Files:**
- Modify: `internal/app/ui/repositories_view.test.js` (assert responsive classes/wrappers)

**Purpose:** Capture mobile-focused structure to guard against regressions in layout and accessibility.

**Context to rehydrate:**
- Existing repositories view tests and DOM helper functions (FakeElement, collectText)
- New classes/aria added in Task 2

**Outcome:** Tests assert scroll wrapper presence, sticky-ready headers, stacked form classes, and aria labels without breaking existing CRUD assertions.

**How to Verify:**  
Run: `node --test internal/app/ui/repositories_view.test.js`  
Expected: New mobile layout tests pass alongside existing suites.

**Acceptance Criteria:**
- [ ] Tests cover scroll wrapper and header stickiness attributes
- [ ] Tests cover stacked form classes/aria-labels
- [ ] Existing CRUD tests still pass and remain unchanged in intent
- [ ] Manual: N/A (covered by automated assertions)

**Not In Scope:** Snapshot tests or visual regression tooling.

**Gotchas:** Maintain deterministic FakeDocument structures; avoid brittle class order assertions—prefer includes.

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes                                                     |
| --------------------------- | ------ | --------------------------------------------------------- |
| Complete                    | ✓      | Covers CSS, markup, and tests for repositories mobile UX  |
| Accurate                    | ✓      | Paths verified: `internal/app/ui/index.html`, `app.js`, `repositories_view.test.js` |
| Commands valid              | ✓      | `node --test internal/app/ui/repositories_view.test.js`   |
| YAGNI                       | ✓      | Only repositories view touched; other views unchanged     |
| Minimal                     | ✓      | Three focused tasks within file budget                    |
| Not over-engineered         | ✓      | Pure CSS/DOM tweaks; no new libs                          |
| Key Decisions documented    | ✓      | Five listed above                                         |
| Supporting docs present     | ✓      | MDN/WAI references listed                                 |
| Context sections present    | ✓      | Each task has Purpose and Context to rehydrate            |
| Budgets respected           | ✓      | ≤2 prod files per task; single outcome each               |
| Outcome & Verify present    | ✓      | Outcome + How to Verify in every task                     |
| Acceptance Criteria present | ✓      | Checklists included per task                              |
| Rehydration context present | ✓      | Context lists per task                                    |

### Rule-of-Five Passes

| Pass        | Changes Made                                           |
| ----------- | ------------------------------------------------------ |
| Draft       | Structured tasks, dependencies, files, verification    |
| Correctness | Checked paths/commands and dependency ordering         |
| Clarity     | Simplified outcomes and acceptance wording             |
| Edge Cases  | Added sticky header/gotchas and aria considerations    |
| Excellence  | Polished supporting docs and budgets alignment         |
