# UI Footer Placeholder Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Finalize the UI shell with a dedicated footer slot that can be populated by future content while keeping the existing pages and tests stable.

**Architecture:** Keep the single-page vanilla JS shell; add a semantic footer container in `index.html` and expose a minimal footer-update hook in `app.js` without altering routing or view logic. Footer content remains static by default and can be updated via the exported helper for later features.

**Tech Stack:** Vanilla HTML/CSS, DOM APIs in `internal/app/ui/app.js`, node:test for UI utilities.

**Key Decisions:**

- **Footer as static slot:** Add a semantic `<footer>` with an inner container and IDs for JS hooks—lightweight and non-invasive to existing layout.
- **Optional JS hook:** Provide a `setFooterContent` helper exported from `app.js` to allow future dynamic text without touching layout again.
- **No new routes:** Footer work should not affect hash routing or batch/run query handling—avoids regressions in navigation.

---

## Supporting Documentation

- UI shell structure and styling: `internal/app/ui/index.html` (sidebar, main content, footer block).
- UI behavior and helpers: `internal/app/ui/app.js` (`setStatus`, navigation, renderers; pattern for exported helpers).
- UI tests and patterns: `internal/app/ui/*.test.js` (node:test with fake DOM helpers).
- Project overview: `README.md` (UI instructions and endpoints).

---

### Task 1: Add footer slot markup and styling

**Depends on:** None  
**Files:**

- Modify: `internal/app/ui/index.html` (footer block, IDs/classes)

**Purpose:** Provide a semantic, styled footer container with an identifiable inner slot for future dynamic content while preserving existing layout spacing.

**Context to rehydrate:**

- Footer currently exists as plain text inside `.layout-footer`.
- CSS for `.layout-footer` is defined inline in `index.html`.

**Outcome:** Footer renders with structured markup (outer footer + inner container) and a clear placeholder message; layout spacing remains consistent on desktop/mobile.

**How to Verify:**  
Run: `node --test internal/app/ui/*.test.js` (ensures no regressions); then open the HTML locally and confirm the footer displays the placeholder text in the shell.

**Acceptance Criteria:**

- [ ] Footer uses semantic `<footer>` with inner slot identified by ID/class.
- [ ] Placeholder copy is present and readable.
- [ ] Layout spacing unchanged on desktop and responsive breakpoints.
- [ ] All UI tests continue to pass.

**Not In Scope:** Changing navigation, adding links, or introducing new APIs.

**Gotchas:** Keep contrast aligned with existing muted text tokens to avoid visual regression.

---

### Task 2: Expose footer update hook in app.js

**Depends on:** Task 1  
**Files:**

- Modify: `internal/app/ui/app.js` (exported helper or initialization hook)

**Purpose:** Add a small helper that can update the footer slot text and set the default footer message during boot without touching view-specific renderers.

**Context to rehydrate:**

- Shared helpers live at top-level of `app.js`; other exports follow `if (typeof module !== "undefined") module.exports = { ... }`.
- There is no current reference to the footer element in JS.

**Outcome:** `app.js` exposes a `setFooterContent` (or similar) function, invoked on startup to set default placeholder text; future code can reuse it to update footer content.

**How to Verify:**  
Run: `node --test internal/app/ui/*.test.js`  
Expected: Tests remain passing; manual check confirms footer text set by JS matches placeholder.

**Acceptance Criteria:**

- [ ] Helper safely no-ops if the footer element is missing.
- [ ] Default footer text is set during app initialization.
- [ ] Export includes the helper for testability/reuse.
- [ ] No changes to routing or view rendering behavior.

**Not In Scope:** Fetching dynamic footer content or adding analytics hooks.

**Gotchas:** Keep DOM lookups scoped and cached; avoid touching global state that could affect tests.

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes |
| --------------------------- | ------ | ----- |
| Complete                    | ✓ | Covers footer markup and JS hook per issue scope |
| Accurate                    | ✓ | Paths verified (`internal/app/ui/index.html`, `internal/app/ui/app.js`) |
| Commands valid              | ✓ | `node --test internal/app/ui/*.test.js` |
| YAGNI                       | ✓ | No new routes or API calls |
| Minimal                     | ✓ | Two tasks, each ≤2 prod files |
| Not over-engineered         | ✓ | Simple helper + markup only |
| Key Decisions documented    | ✓ | Three decisions |
| Supporting docs present     | ✓ | Listed relevant files/tests |
| Context sections present    | ✓ | Included per task |
| Budgets respected           | ✓ | ≤2 files per task; single outcome each |
| Outcome & Verify present    | ✓ | Included per task |
| Acceptance Criteria present | ✓ | Checklist per task |
| Rehydration context present | ✓ | Included where needed |

### Rule‑of‑Five Passes

| Pass        | Changes Made |
| ----------- | ------------ |
| Draft       | Initial tasks and decisions outlined |
| Correctness | Verified file paths and commands |
| Clarity     | Tightened Purpose/Outcome phrasing |
| Edge Cases  | Added no-op guard/gotchas |
| Excellence  | Polished wording and checklist |
