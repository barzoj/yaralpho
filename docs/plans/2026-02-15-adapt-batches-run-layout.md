# Adapt batch and run views to new shell layout Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Adapt batch, runs list, and run detail views to the new shell layout so query-driven navigation preserves breadcrumbs/menu and live events remain intact.

**Architecture:** Continue using the existing DOM-driven UI in `internal/app/ui/app.js`, re-binding renderers to the new shell slots (status, content, breadcrumbs, view title) while keeping fetch + live stream flows untouched. Route selection remains query-driven (`?batch=`, `?run=`) and should coexist with the hash-based router introduced previously. Breadcrumb/back links rely on encoded query URLs so navigation stays consistent across shell sections.

**Tech Stack:** Browser DOM APIs, `fetch`/WebSocket, Node test runner (`node --test`).

**Key Decisions:**

- **Reuse existing helpers:** Keep `fetchJSON`, `buildTable`, and formatting helpers; only adjust mounting/slot usage to avoid unnecessary refactors.
- **Query-link navigation:** Generate links with encoded query params (`/app?batch=…`, `/app?batch=…&run=…`) to align with the router and ensure breadcrumbs/back links stay consistent.
- **Live events preservation:** Avoid changing live stream wiring; only adjust layout placement so the WebSocket/event merge logic remains stable.

---

## Supporting Documentation

- MDN `URLSearchParams` – ensure query-based routing aligns with the shell: https://developer.mozilla.org/en-US/docs/Web/API/URLSearchParams
- MDN `fetch` API – confirm error handling patterns for JSON/text responses: https://developer.mozilla.org/en-US/docs/Web/API/fetch
- MDN DOM `createElement`/tables – patterns for building table/anchor elements: https://developer.mozilla.org/en-US/docs/Web/API/Document/createElement

---

### Task 1: Map shell slots and startup binding

**Depends on:** None  
**Files:**

- Modify: `internal/app/ui/app.js` (root element lookups/startup routing)

**Purpose:** Align the entrypoint and shared helpers with the new shell layout slots (status/content/view-title/breadcrumbs) so all renderers mount into the correct containers.

**Context to rehydrate:**

- `internal/app/ui/app.js` (`statusEl`, `contentEl`, `viewTitle`, `breadcrumbsEl`, `start`)

**Outcome:** App startup binds to the correct shell nodes, clears content/status safely, and can render the default view without layout errors.

**How to Verify:** Load `/app` in the new shell and confirm no console errors and a visible batches list renders (manual).

**Acceptance Criteria:**

- [ ] Unit test(s): N/A (startup binding)
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: `/app` shows batches view without errors
- [ ] Outputs match expectations from How to Verify
- [ ] Interface and implementation separation maintained (helpers reused)

**Not In Scope:** Styling changes or HTML template alterations.

**Gotchas:** Ensure DOM lookups fail gracefully if slot IDs differ; avoid touching live event wiring here.

---

### Task 2: Adapt batches and runs list views to shell layout

**Depends on:** Task 1  
**Files:**

- Modify: `internal/app/ui/app.js` (`renderBatches`, `renderRuns`, helper links/breadcrumbs)
- Test: `internal/app/ui/runs_table.test.js` (update expectations if headers/links shift)

**Purpose:** Render batch and runs lists inside the new shell slots with accurate breadcrumbs and navigation that matches the router/hashes.

**Context to rehydrate:**

- `renderBatches`, `renderRuns`, `buildRunsTable`, `createLink`/`createButtonLink`

**Outcome:** `/app` lists batches; `/app?batch=…` lists runs with correct breadcrumbs/back links and status messaging.

**How to Verify:**  
Run: `node --test internal/app/ui/runs_table.test.js`  
Manual: open `/app?batch=<id>` and confirm breadcrumbs `Batches › <batch>` and status banner update on load/error.

**Acceptance Criteria:**

- [ ] Unit test(s): `internal/app/ui/runs_table.test.js`
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: `/app?batch=<id>` renders runs with breadcrumbs/menu intact
- [ ] Outputs match expectations from How to Verify
- [ ] Interface and implementation separation preserved (helpers reused)

**Not In Scope:** API shape changes or pagination behavior.

**Gotchas:** Preserve `deriveRepositoryIdForBatch` usage; ensure encoded query strings in links to avoid router conflicts.

---

### Task 3: Adapt run detail view to shell layout without breaking live events

**Depends on:** Task 2  
**Files:**

- Modify: `internal/app/ui/app.js` (`renderRunView`, breadcrumb/actions placement, status banner slotting)

**Purpose:** Render run detail inside the shell layout, keeping breadcrumbs, back links, and the live status banner consistent while leaving WebSocket/event handling unchanged.

**Context to rehydrate:**

- `renderRunView`, `createRunLayout`, `renderBreadcrumbs`, live stream helpers (`connectLiveStream`, `updateEventsUI`)

**Outcome:** `/app?run=<id>` shows run meta, breadcrumbs (`Batches › Batch <id> › Run <id>`), actions, and live event stream in the correct layout slots.

**How to Verify:**  
Manual: open `/app?run=<id>` with an active run; confirm breadcrumbs/actions present, events stream, and status banner updates; navigate back via buttons and breadcrumbs.

**Acceptance Criteria:**

- [ ] Unit test(s): N/A (live stream behavior)
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: `/app?run=<id>` shows live events and breadcrumbs/back links work
- [ ] Outputs match expectations from How to Verify
- [ ] Live stream logic untouched aside from layout placement

**Not In Scope:** Changing WebSocket endpoints, retry strategy, or merge logic.

**Gotchas:** Keep `resetLiveStream` invocation order; ensure layout headers include live status pill after slot changes.

---

### Task 4: Align tests with updated layout/navigation

**Depends on:** Task 2  
**Files:**

- Modify: `internal/app/ui/runs_table.test.js`
- Modify: `internal/app/ui/app.js` (test-facing exports if needed)

**Purpose:** Update characterization tests to reflect any table/header or link changes from the new layout usage.

**Context to rehydrate:**

- `buildRunsTable` exports and DOM shape expected in `runs_table.test.js`

**Outcome:** Tests pass and assert the updated columns/links without relying on old layout assumptions.

**How to Verify:**  
Run: `node --test internal/app/ui/runs_table.test.js`  
Expected: PASS

**Acceptance Criteria:**

- [ ] Unit test(s): `internal/app/ui/runs_table.test.js`
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: N/A
- [ ] Outputs match expectations from How to Verify
- [ ] Interface and implementation separation preserved

**Not In Scope:** Adding new test utilities or changing unrelated suites.

**Gotchas:** Keep fake DOM helpers aligned with actual DOM usage; avoid hard-coding slot IDs beyond what the module exports.

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes                                     |
| --------------------------- | ------ | ----------------------------------------- |
| Complete                    | ✓      | Covers batches list, runs list, run view, tests |
| Accurate                    | ✓      | Paths verified in repo (`internal/app/ui/app.js`, `internal/app/ui/runs_table.test.js`) |
| Commands valid              | ✓      | `node --test internal/app/ui/runs_table.test.js` |
| YAGNI                       | ✓      | No new APIs or layout assets planned      |
| Minimal                     | ✓      | Tasks focus on slot binding and navigation |
| Not over‑engineered         | ✓      | Reuse existing helpers; no new abstractions |
| Key Decisions documented    | ✓      | Three decisions captured                  |
| Supporting docs present     | ✓      | MDN references listed                     |
| Context sections present    | ✓      | All tasks include Purpose/Context sections |
| Budgets respected           | ✓      | Tasks limited to single file pair and single outcome |
| Outcome & Verify present    | ✓      | Each task has Outcome/How to Verify       |
| Acceptance Criteria present | ✓      | Checklists included per task              |
| Rehydration context present | ✓      | Included where prior knowledge needed     |

### Rule‑of‑Five Passes

| Pass        | Changes Made                             |
| ----------- | ---------------------------------------- |
| Draft       | Initial structure with 4 scoped tasks    |
| Correctness | Verified paths/commands and dependencies |
| Clarity     | Simplified outcomes and verification text |
| Edge Cases  | Added gotchas on live stream and encoding |
| Excellence  | Polished supporting docs and key decisions |

