# Sticky Run Header & Scrollable Events Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Keep run header visible while events scroll inside a dedicated container, preserving existing actions/status layout.

**Architecture:** Adjust run view DOM to introduce a sticky header block and a scroll-constrained events section; CSS handles stickiness/overflow while JS renders into the new container and sizes it to the viewport.

**Tech Stack:** Vanilla HTML/CSS/JS (no build), node:test for unit tests.

**Key Decisions:**

- **Layout approach:** Use explicit run layout containers (header + events section) instead of repurposing existing nodes — avoids regressions and keeps structure testable.
- **Stickiness offset:** Sticky header uses a computed CSS variable from measured page header/status heights — prevents overlap with existing sticky top header.
- **Scroll sizing:** Inline JS sizing with minimum height fallback — keeps header visible and events scrollable across viewport sizes without relying on complex CSS calc chains.

---

## Supporting Documentation

- Existing UI files: `internal/app/ui/index.html` (CSS + entry), `internal/app/ui/app.js` (run rendering), live helpers `internal/app/ui/live_merge.js`, `internal/app/ui/live_reconnect.js`.
- API surface used: `/runs/{id}`, `/runs/{id}/events?limit=...` (see README section "App UI (/app)").
- Sticky positioning reference: [MDN position: sticky](https://developer.mozilla.org/en-US/docs/Web/CSS/position#sticky).
- Overflow/scroll sizing reference: [MDN overflow-y](https://developer.mozilla.org/en-US/docs/Web/CSS/overflow-y).

---

## Work Plan

### Task 1: Add run view layout containers and styles

**Depends on:** None

**Files:**
- Modify: `internal/app/ui/index.html` (CSS additions)

**Purpose:** Introduce explicit run header and events section styling, including sticky header and scrollable area classes.

**Context to rehydrate:** Review current CSS in `internal/app/ui/index.html` around `.events` and layout basics.

**Outcome:** New CSS classes support sticky run header and scrollable events area without altering existing global layout.

**How to Verify:**
Run: `grep -n "run-header" internal/app/ui/index.html`
Expected: CSS definitions for run header stickiness and `.events-scroll` overflow/height.

**Acceptance Criteria:**
- [ ] Sticky header CSS present with top offset variable.
- [ ] `.events-scroll` styles set overflow-y and height constraints.
- [ ] No removal of existing styles (only additive/minimal adjustments).
- [ ] Manual/E2E: N/A (visual check handled in later tasks).

**Not In Scope:** JS wiring and tests.

**Gotchas:** Keep z-index modest to avoid covering global header.

### Task 2: Render run view into sticky header + scroll container

**Depends on:** Task 1

**Files:**
- Modify: `internal/app/ui/app.js`

**Purpose:** Build run view DOM using header section and events scroll container; size scroll area and keep header visible during live updates.

**Context to rehydrate:**
- `renderRunView` structure and `updateEventsUI` usage.
- Live stream helpers `mergeLiveEnvelope`, `updateEventsUI` expectations.

**Outcome:** Run header (actions + metadata) stays outside the scrollable events container; events render and live-update inside `.events-scroll` with sizing applied.

**How to Verify:**
Run: `node --test internal/app/ui/run_layout.test.js`
Expected: Tests for layout structure and scroll container sizing pass.

**Acceptance Criteria:**
- [ ] Run header node not nested in events scroll container.
- [ ] Events render inside dedicated scroll container and live updates reuse the same container (no duplicates).
- [ ] Scroll container height/overflow configured at render time.
- [ ] Status/action layout preserved (buttons still visible with header).

**Not In Scope:** Auto-follow/scroll mode (separate task yaralpho-b5i.19).

**Gotchas:** Maintain existing event rendering/dedupe logic; avoid altering live reconnect controller.

### Task 3: Add unit tests for layout and scroll container

**Depends on:** Task 2

**Files:**
- Create: `internal/app/ui/run_layout.test.js`

**Purpose:** Validate DOM structure (header outside scroll area, events inside) and scroll container sizing configuration.

**Context to rehydrate:** New helpers exported from `app.js` for testing; node:test usage in other UI tests.

**Outcome:** Automated coverage ensuring structure and overflow setup stay intact.

**How to Verify:**
Run: `node --test internal/app/ui/run_layout.test.js`
Expected: All tests pass.

**Acceptance Criteria:**
- [ ] Test asserts header is separate from events scroll container.
- [ ] Test asserts `.events-scroll` receives overflow/height configuration.
- [ ] Tests run with node:test without additional tooling.

**Not In Scope:** Visual regression or E2E tests.

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes                                     |
| --------------------------- | ------ | ----------------------------------------- |
| Complete                    | ✓      | All scope items mapped to tasks           |
| Accurate                    | ✓      | Paths verified via repo inspection        |
| Commands valid              | ✓      | Uses node --test + grep                   |
| YAGNI                       | ✓      | Focused on layout/scroll only             |
| Minimal                     | ✓      | Three small tasks, limited files          |
| Not over-engineered         | ✓      | No new deps; simple DOM helpers           |
| Key Decisions documented    | ✓      | Three decisions listed                    |
| Supporting docs present     | ✓      | Links and file refs included              |
| Context sections present    | ✓      | Purpose/rehydrate provided where needed   |
| Budgets respected           | ✓      | ≤2 files/task, small steps                |
| Outcome & Verify present    | ✓      | Each task lists outcome and verify        |
| Acceptance Criteria present | ✓      | Checklists per task                       |
| Rehydration context present | ✓      | Added where prior context needed          |

### Rule-of-Five Passes

| Pass        | Changes Made                             |
| ----------- | ---------------------------------------- |
| Draft       | Initial structure + tasks                |
| Correctness | Checked file paths/commands              |
| Clarity     | Added purposes/outcomes/context          |
| Edge Cases  | Noted z-index/gotchas and scope limits   |
| Excellence  | Ensured verification record completeness |
