# Mobile-friendly Control Plane Layout Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Make the control plane view usable on sub-768px screens by stacking repo cards, preserving status/actions, and enabling horizontal task table scrolling without clipping.

**Architecture:** Keep the vanilla JS single-page layout; add responsive CSS tokens in `index.html` and apply lightweight class hooks in `app.js` so control plane cards and task tables adapt at mobile breakpoints. Use non-invasive wrappers to avoid altering data flow or existing routing.

**Tech Stack:** Vanilla JS + DOM APIs, inline CSS in `index.html`, node:test for UI logic validation.

**Key Decisions:**

- **Layout strategy:** Use CSS grid with gap tokens and a mobile breakpoint that collapses to a single column — avoids JS layout toggles and keeps accessibility stable.
- **Table overflow:** Wrap control plane tables in an overflow-x container with sticky padding rather than modifying table markup — minimizes logic changes and keeps semantics intact.
- **Action accessibility:** Keep restart buttons within a flex row that wraps on narrow screens — ensures tap targets remain reachable without stacking complex controls.

---

## Supporting Documentation

- MDN CSS Grid: https://developer.mozilla.org/en-US/docs/Web/CSS/CSS_grid_layout — reference for responsive grid layouts.
- MDN Media Queries: https://developer.mozilla.org/en-US/docs/Web/CSS/Media_Queries/Using_media_queries — breakpoint usage for sub-768px adjustments.
- MDN overflow-x/scroll: https://developer.mozilla.org/en-US/docs/Web/CSS/overflow-x — patterns for horizontal scroll containers.
- WAI touch target size guidance: https://www.w3.org/WAI/WCAG21/Techniques/general/G215 — to keep restart buttons accessible on mobile.
- Existing UI patterns: `internal/app/ui/app.js` (ControlPlaneView, buildTable), `internal/app/ui/index.html` (base layout styles), `internal/app/ui/control_plane.test.js` (current expectations for scrollable tasks and restart button behavior).

---

### Task 1: Add responsive control plane styles

**Depends on:** None  
**Files:**

- Modify: `internal/app/ui/index.html:7-470` (add control-plane/card/table wrappers and mobile media rules)

**Purpose:** Introduce CSS classes that stack control plane cards on narrow screens and add horizontal scroll wrappers for task tables without JS changes.

**Context to rehydrate:**

- Review existing `.app-shell` grid and media queries in `index.html`.
- Note current control plane markup added by `ControlPlaneView` (cards, tables).

**Outcome:** Control plane container uses responsive grid spacing; below 768px cards span full width with padded content; tables sit inside an overflow-x wrapper with touch-friendly spacing.

**How to Verify:**  
Manual: Open `/app#/control-plane` in devtools with <768px width; observe single-column cards, padded content, and horizontally scrollable task table without clipping.

**Acceptance Criteria:**

- [ ] Unit test(s): N/A (style-only change)
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: `/app#/control-plane` at <768px shows stacked cards and horizontal scroll for tasks
- [ ] Outputs match expectations from How to Verify
- [ ] Interface and implementation separation maintained (CSS hooks only)
- [ ] DAL layer unchanged (N/A)

**Not In Scope:** JS routing changes, data fetching logic, or altering table semantics.

**Gotchas:** Preserve existing color tokens and avoid affecting other views; keep media rules scoped to control-plane classes to prevent nav regressions.

### Task 2: Wire control plane markup to responsive classes and scroll wrappers

**Depends on:** Task 1  
**Files:**

- Modify: `internal/app/ui/app.js:1200-1405` (ControlPlaneView card/table rendering and wrappers)
- Modify: `internal/app/ui/app.js:1370-1398` (buildTable wrapper hooks if needed)

**Purpose:** Apply new class hooks to control plane cards and wrap batch tables in overflow containers so mobile scroll works without clipping actions.

**Context to rehydrate:**

- ControlPlaneView rendering loop and `buildTable` helper.
- Restart action rendering in `buildRestartAction`.

**Outcome:** Control plane cards carry responsive class names; batch tables render inside an overflow-x container with preserved restart buttons; tasks list still lazy-loads and vertical scroll remains capped.

**How to Verify:**  
Run: `node --test internal/app/ui/control_plane.test.js`  
Manual: Narrow viewport to <768px; confirm batch table scrolls horizontally and restart buttons remain visible.

**Acceptance Criteria:**

- [ ] Unit test(s): `internal/app/ui/control_plane.test.js` updated and passing
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: Control plane table scrolls horizontally; restart button reachable on mobile
- [ ] Outputs match expectations from How to Verify
- [ ] Interface/implementation separation intact; helper signatures unchanged
- [ ] No data flow changes (fetch/restart)

**Not In Scope:** Changing restart eligibility logic or batch fetch pagination.

**Gotchas:** Ensure wrappers do not interfere with existing FakeElement table rendering in tests; keep table semantics for accessibility.

### Task 3: Extend control plane tests for mobile layout behaviors

**Depends on:** Task 2  
**Files:**

- Modify: `internal/app/ui/control_plane.test.js:144-199` (add assertions for overflow wrapper/classes)

**Purpose:** Lock in responsive behavior by asserting horizontal scroll container presence and card class hooks without relying on real DOM layout.

**Context to rehydrate:**

- Existing tests for restart visibility and task scroll container.
- Fake DOM utilities (`findFirst`, `collectText`) available in test file.

**Outcome:** Tests assert control plane tables render within overflow-x wrapper and card containers carry responsive class names, preventing regressions.

**How to Verify:**  
Run: `node --test internal/app/ui/control_plane.test.js`

**Acceptance Criteria:**

- [ ] Unit test(s): `internal/app/ui/control_plane.test.js` assertions updated for overflow wrapper and card classes
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: N/A
- [ ] Outputs match expectations from How to Verify
- [ ] Interface boundaries unchanged; uses public render functions only
- [ ] No brittle DOM coupling beyond class hooks

**Not In Scope:** Visual snapshot testing or cross-browser matrix.

**Gotchas:** Keep assertions resilient to unrelated style changes by checking class presence/overflow flags, not pixel values.

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes |
| --------------------------- | ------ | ----- |
| Complete                    | ✓ | Covers CSS hooks, JS wiring, tests, manual check |
| Accurate                    | ✓ | Paths verified: `internal/app/ui/index.html`, `internal/app/ui/app.js`, `internal/app/ui/control_plane.test.js` |
| Commands valid              | ✓ | `node --test internal/app/ui/control_plane.test.js` |
| YAGNI                       | ✓ | Only layout and test hardening; no routing/data changes |
| Minimal                     | ✓ | Three small tasks, max two files each |
| Not over-engineered         | ✓ | CSS + wrappers only; no JS layout engine |
| Key Decisions documented    | ✓ | Three decisions listed |
| Supporting docs present     | ✓ | MDN/WAI links captured |
| Context sections present    | ✓ | Purpose, Context, Outcome, Verify included |
| Budgets respected           | ✓ | Tasks limited in scope/files/outcomes |
| Outcome & Verify present    | ✓ | Each task states outcome and verification |
| Acceptance Criteria present | ✓ | Each task lists checklist |
| Rehydration context present | ✓ | Included where prior work matters |

### Rule-of-Five Passes

| Pass        | Changes Made |
| ----------- | ------------ |
| Draft       | Initial structure with 3 tasks and dependencies |
| Correctness | Verified file paths/commands; clarified outcomes |
| Clarity     | Simplified outcomes and gotchas; added scope notes |
| Edge Cases  | Added manual mobile check and restart button accessibility notes |
| Excellence  | Ensured decisions, supporting docs, and budgets are explicit |
