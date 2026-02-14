# Live events last_ingested filtering & envelopes (yaralpho-b5i.8) Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Stream run session events over WebSocket with cursor-aware backfill and structured envelopes so clients receive ordered, non-duplicated updates without manual refresh.

**Architecture:** Reuse existing storage backfill for historical events and the in-memory session event bus for live fan-out; apply the `last_ingested` cursor exclusively to backfill and live filters; standardize envelope schema for event/error/heartbeat frames.

**Tech Stack:** Go 1.22+, gorilla/websocket, zap logger, in-memory bus + Mongo-backed storage.

**Key Decisions:**

- **Cursor handling:** Treat missing `last_ingested` as zero time; reject parse errors or future timestamps with structured WebSocket error envelopes to avoid silent drops.
- **Backfill ordering:** Sort storage events by `IngestedAt` and filter strictly after cursor to avoid duplicates while preserving ingest order before switching to live stream.
- **Envelope schema:** Use `event`/`error`/`heartbeat` types with RFC3339Nano cursor strings; heartbeat frames align with upcoming reliability tasks while keeping event envelopes backward compatible.

---

## Supporting Documentation

- gorilla/websocket (upgrade, read/write deadlines, close codes): https://pkg.go.dev/github.com/gorilla/websocket — use `SetWriteDeadline`, `CloseMessage`, and `FormatCloseMessage` for structured closes and pings/pongs.
- Go `time` parsing/formatting: https://pkg.go.dev/time#Parse and https://pkg.go.dev/time#Time.Format — RFC3339Nano ensures stable cursors.
- Existing run events handlers/tests: `internal/app/run_events_live_handler.go`, `internal/app/run_events_live_handler_test.go`, `internal/bus` fan-out behavior for live events.
- Storage model for session events: `internal/storage/models.go` (IngestedAt UTC timestamps) to ensure cursor comparisons use UTC and strict ordering.

---

### Task 1: Define envelope schema and cursor validation

**Depends on:** None  
**Files:**
- Modify: `internal/app/run_events_live_handler.go` (envelope types, cursor parsing, error envelopes)
- Modify: `internal/app/run_events_live_handler_test.go` (cursor/error envelope coverage)

**Purpose:** Establish explicit envelope types and strict cursor validation so clients know how errors are conveyed and cursors are applied consistently.

**Context to rehydrate:**
- Review current handler at `internal/app/run_events_live_handler.go` for parseCursor and envelope struct.
- Review existing cursor validation tests in `internal/app/run_events_live_handler_test.go`.

**Outcome:** WebSocket handler documents and emits `event`/`error` envelope types; missing cursor defaults to zero time, parse/future errors return error envelopes/close with clear reasons.

**How to Verify:**  
Run: `go test ./internal/app -run TestRunEventsLiveHandlerRejectsInvalidCursor|TestRunEventsLiveHandlerRejectsFutureCursor -v`  
Expected: Tests pass; responses include correct HTTP or WebSocket error signaling and updated envelope schema compiled.

**Acceptance Criteria:**
- [ ] Unit test(s): updated cursor/error coverage in `run_events_live_handler_test.go`
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: N/A
- [ ] Outputs match expectations from How to Verify
- [ ] Interface and implementation separation respected
- [ ] DAL types remain storage-agnostic

**Not In Scope:**
- Heartbeat timing or reconnect UX (covered by task 5.2).
- Frontend wiring to consume new envelope types.

**Gotchas:**
- Keep error strings within Close frame length limits (~123 chars) as existing helper does.

**Step 1: Write the failing test**

```go
// extend invalid/future cursor tests to assert error envelope or status
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/app -run TestRunEventsLiveHandlerRejectsInvalidCursor -v`  
Expected: FAIL on new assertions.

**Step 3: Write minimal implementation**

```go
// add envelopeTypeError, emit error envelope/close on invalid cursor
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/app -run TestRunEventsLiveHandlerRejectsInvalidCursor -v`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/app/run_events_live_handler.go internal/app/run_events_live_handler_test.go
git commit -m "yaralpho-b5i.8: add envelope schema and cursor validation"
```

---

### Task 2: Cursor-aware backfill ordering and dedupe

**Depends on:** Task 1  
**Files:**
- Modify: `internal/app/run_events_live_handler.go` (backfill loop, cursor update semantics)
- Modify: `internal/app/run_events_live_handler_test.go` (ordering/dedupe scenarios)

**Purpose:** Ensure backfill respects the cursor exclusivity and sends events in ingest order without duplicates before transitioning to live stream.

**Context to rehydrate:**
- Backfill loop and `seen` tracking in `run_events_live_handler.go`.
- Existing test `TestRunEventsLiveHandlerBackfillsFromCursor`.

**Outcome:** Backfill filters events strictly after cursor, emits envelopes with updated cursors, and skips duplicates deterministically; tests cover ordering and dedupe edge cases.

**How to Verify:**  
Run: `go test ./internal/app -run TestRunEventsLiveHandlerBackfillsFromCursor -v`  
Expected: Backfill test passes with new ordering/dedupe assertions.

**Acceptance Criteria:**
- [ ] Unit test(s): expanded backfill/dedupe coverage in `run_events_live_handler_test.go`
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: N/A
- [ ] Outputs match expectations from How to Verify
- [ ] Interface and implementation separation respected
- [ ] DAL types remain storage-agnostic

**Not In Scope:**
- Live bus filtering by cursor (handled via seen map and backfill handoff only).

**Gotchas:**
- Sort backfill events before filtering to maintain deterministic cursor progression.

**Step 1: Write the failing test**

```go
// extend backfill test to include out-of-order storage events and duplicate ingest times
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/app -run TestRunEventsLiveHandlerBackfillsFromCursor -v`  
Expected: FAIL on new assertions.

**Step 3: Write minimal implementation**

```go
// adjust sort/filter and cursor updates to match expected order and dedupe
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/app -run TestRunEventsLiveHandlerBackfillsFromCursor -v`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/app/run_events_live_handler.go internal/app/run_events_live_handler_test.go
git commit -m "yaralpho-b5i.8: fix backfill ordering and dedupe"
```

---

### Task 3: Live stream cursor continuity and envelope docs

**Depends on:** Task 2  
**Files:**
- Modify: `internal/app/run_events_live_handler.go` (live loop cursor enforcement, envelope doc comments)
- Modify: `internal/app/run_events_live_handler_test.go` (live stream cursor continuity test)

**Purpose:** Guarantee the live stream only emits events after the last sent cursor and document the envelope contract for clients consuming live updates.

**Context to rehydrate:**
- Live select loop and `seen` map updates in `run_events_live_handler.go`.
- Any new envelope constants from Task 1.

**Outcome:** Live stream enforces cursor monotonicity and skips duplicates, with doc comments clarifying envelope schema; tests assert no duplicate live send when backfill event is republished.

**How to Verify:**  
Run: `go test ./internal/app -run TestRunEventsLiveHandlerStreamsEvents -v`  
Expected: Live stream test passes with new assertions on cursor progression and dedupe.

**Acceptance Criteria:**
- [ ] Unit test(s): updated live streaming coverage in `run_events_live_handler_test.go`
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: N/A
- [ ] Outputs match expectations from How to Verify
- [ ] Interface and implementation separation respected
- [ ] DAL types remain storage-agnostic

**Not In Scope:**
- Heartbeat timers or reconnection policies (task 5.2).
- Frontend event handling.

**Gotchas:**
- Maintain read deadlines and close semantics when adding documentation or logging.

**Step 1: Write the failing test**

```go
// assert no duplicate live send when event matching last cursor is republished
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/app -run TestRunEventsLiveHandlerStreamsEvents -v`  
Expected: FAIL on new assertions.

**Step 3: Write minimal implementation**

```go
// enforce cursor monotonicity and document envelope types
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/app -run TestRunEventsLiveHandlerStreamsEvents -v`  
Expected: PASS

**Step 5: Commit**

```bash
git add internal/app/run_events_live_handler.go internal/app/run_events_live_handler_test.go
git commit -m "yaralpho-b5i.8: ensure live cursor continuity"
```

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes                                     |
| --------------------------- | ------ | ----------------------------------------- |
| Complete                    | ✓    | Covers cursor validation, backfill, live envelopes |
| Accurate                    | ✓    | File paths verified in repo                         |
| Commands valid              | ✓    | go test ./internal/app with targeted -run flags     |
| YAGNI                       | ✓    | Scopes to handler/tests only; defers heartbeats UX  |
| Minimal                     | ✓    | Three tasks, two prod files max each                |
| Not over-engineered         | ✓    | Reuses existing bus/storage; no new deps            |
| Key Decisions documented    | ✓    | Three decisions captured in header                  |
| Supporting docs present     | ✓    | Links to websocket/time/package references          |
| Context sections present    | ✓    | Each task includes Purpose and Context              |
| Budgets respected           | ✓    | ≤2 prod files per task; single outcome each         |
| Outcome & Verify present    | ✓    | Each task lists outcome and go test verification    |
| Acceptance Criteria present | ✓    | Checklists included per task                        |
| Rehydration context present | ✓    | Tasks 1-3 list context pointers                     |

### Rule-of-Five Passes

| Pass        | Changes Made                             |
| ----------- | ---------------------------------------- |
| Draft       | Structured three tasks, added budgets/verification |
| Correctness | Reviewed commands/paths; no content changes |
| Clarity     | Simplified outcomes/verify phrasing; kept scopes concise |
| Edge Cases  | Noted future/invalid cursor handling and duplicate ingest times |
| Excellence  | Final wording tightened; scopes and verifications aligned |
