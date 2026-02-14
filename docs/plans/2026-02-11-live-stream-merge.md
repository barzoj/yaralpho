# Live stream merge Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Make the /app UI merge live WebSocket event envelopes into the existing events view in-order without duplicates while preserving current pagination/limit behavior.

**Architecture:** Keep the vanilla JS UI but extract pure merge helpers usable both in-browser and under Node’s built-in test runner; wire app.js to use the helper for all incoming envelopes. Tests run with `node --test` against the helper without needing DOM globals.

**Tech Stack:** Vanilla JS, Node built-in test runner, existing static asset serving.

**Key Decisions:**
- **Helper extraction:** Create a small UMD-style helper (`live_merge.js`) instead of touching the DOM-heavy app.js for tests — allows pure unit tests and keeps browser path simple.
- **Ordering key:** Sort by `ingested_at` (ISO) and use composite key (session/run/batch/ingested) for dedupe — matches backend cursor semantics and prevents duplicate renders.
- **Cursor update:** Prefer envelope cursor, else newest ingested_at — keeps last_ingested compatible with server expectations.
- **Limit preservation:** Do not change fetch limits or UI truncation messaging — avoids surprising pagination changes.
- **Export strategy:** UMD export (window + module.exports) — works for browser inclusion and Node tests without bundling.

---

## Supporting Documentation

- MDN WebSocket API: connection lifecycle and message handling — https://developer.mozilla.org/en-US/docs/Web/API/WebSocket
- Node.js test runner (node --test) for zero-dep unit tests — https://nodejs.org/api/test.html
- Array sorting and locale-independent date parsing via `new Date()` (ISO) — https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Date
- Prior UI plan for live events wiring — `docs/plans/2026-02-11-ui-live-events-wiring.md`
- Live events backfill/cursor behavior — `internal/app/run_events_live_handler.go`

---

### Task 1: Add pure merge helper with tests

**Depends on:** None  
**Files:**
- Create: `internal/app/ui/live_merge.js`
- Create (test): `internal/app/ui/live_merge.test.js`

**Purpose:** Provide a pure, testable function to merge incoming live envelopes into cached events with ordering and dedupe guarantees.

**Context to rehydrate:**
- Review `internal/app/ui/app.js` live stream handling (`handleLiveEnvelope`, `eventKeyFromEvent`, `getIngestedAt`).
- Review server envelope schema in `internal/app/run_events_live_handler.go`.

**Outcome:** A reusable helper exports `mergeLiveEnvelope(state, envelope)` (and supporting key/ingest helpers) with unit tests covering in-order, out-of-order, duplicate, and cursor updates.

**How to Verify:**
Run: `node --test internal/app/ui/live_merge.test.js`  
Expected: All tests pass.

**Acceptance Criteria:**
- [ ] Unit test(s): `internal/app/ui/live_merge.test.js` cover ordering, dedupe, cursor updates, total count behavior.
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: N/A
- [ ] Outputs match expectations from How to Verify
- [ ] Interface separation: helper has no DOM dependencies and can be required under Node.

**Not In Scope:** Wiring into app.js or HTML.

**Gotchas:** Ensure helper tolerates missing/invalid `ingested_at` by appending at end and keeping cursor unchanged.

### Task 2: Wire app.js to helper

**Depends on:** Task 1  
**Files:**
- Modify: `internal/app/ui/app.js`
- Test: `internal/app/ui/live_merge.test.js` (reuse)

**Purpose:** Use the pure helper for all live envelopes, maintaining existing initial render but improving merge correctness and dedupe.

**Context to rehydrate:**
- Task 1 helper API.
- Current `handleLiveEnvelope` logic and state shape in `app.js`.

**Outcome:** Live stream updates in app.js delegate to helper; state updates remain consistent (cursor, totalCount, seenKeys) and UI render path unchanged.

**How to Verify:**
Run: `node --test internal/app/ui/live_merge.test.js`  
Expected: Tests still pass (including any new cases needed for integration expectations).

**Acceptance Criteria:**
- [ ] Unit test(s): existing helper tests still pass; add cases if state shape changes.
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: N/A
- [ ] Outputs match expectations from How to Verify
- [ ] No DOM regressions; initial static render preserved.

**Not In Scope:** HTML script tag changes.

**Gotchas:** Preserve status messaging and totalCount handling; avoid re-render loops by only calling `updateEventsUI` when state mutated.

### Task 3: Load helper in UI

**Depends on:** Task 2  
**Files:**
- Modify: `internal/app/ui/index.html`

**Purpose:** Ensure browser loads the helper before app.js so live merging works at runtime.

**Context to rehydrate:**
- Script order in `index.html`.
- Served asset paths (`/app/static/...`).

**Outcome:** `live_merge.js` is served and available globally for app.js; page still functions for batches/runs.

**How to Verify:**
Run (manual): `go test ./internal/app` (coverage for embed paths) and open `/app?run=<id>` locally to confirm no console errors.  
Expected: Tests pass; browser console shows no 404 for live_merge.js and live events still render.

**Acceptance Criteria:**
- [ ] Unit test(s): N/A
- [ ] Integration test(s): `go test ./internal/app` (existing embed coverage)
- [ ] Manual or E2E check: Load `/app?run=<id>` and confirm no missing script errors.
- [ ] Outputs match expectations from How to Verify

**Not In Scope:** Styling or new UI controls.

**Gotchas:** Script must precede `app.js`; ensure embed path matches static serving.

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes                                     |
| --------------------------- | ------ | ----------------------------------------- |
| Complete                    | ✓      | Covers helper, wiring, and loading        |
| Accurate                    | ✓      | Paths verified via repo glob              |
| Commands valid              | ✓      | node --test and go test ./internal/app    |
| YAGNI                       | ✓      | Only helper + wiring + load               |
| Minimal                     | ✓      | Three small tasks within budgets          |
| Not over-engineered         | ✓      | Vanilla JS + Node test runner only        |
| Key Decisions documented    | ✓      | Five listed with rationale                |
| Supporting docs present     | ✓      | Links to MDN/Node runner + internal refs  |
| Context sections present    | ✓      | Each task includes Purpose/Context        |
| Budgets respected           | ✓      | ≤2 files per task; single outcome each    |
| Outcome & Verify present    | ✓      | Outcome + How to Verify per task          |
| Acceptance Criteria present | ✓      | Checklists included per task              |
| Rehydration context present | ✓      | Tasks list context where dependent        |

### Rule-of-Five Passes

| Pass        | Changes Made                             |
| ----------- | ---------------------------------------- |
| Draft       | Structure finalized with 3 tasks         |
| Correctness | Verified file paths/commands             |
| Clarity     | Simplified wording and scope             |
| Edge Cases  | Added notes on missing ingested_at       |
| Excellence  | Polished key decisions/supporting docs   |
