# Bus limits and idle pruning (yaralpho-b5i.5) Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Add configurable subscriber/session limits and idle pruning to the in-memory bus so streaming consumers shed idle subscribers and enforce caps with explicit policy outcomes.

**Architecture:** Extend `internal/bus` to track subscriber counts per session and total buckets, enforce caps during subscription creation based on policy, and run per-subscription idle timers that invoke the existing cleanup path to close channels and remove buckets. Keep publish path unchanged aside from timer resets and lightweight logging for cap hits.

**Tech Stack:** Go, `context`, `time`, channels, `zap` logging, existing `internal/bus` memory implementation and tests.

**Key Decisions:**

- **Cap enforcement timing:** Enforce max sessions and max subscribers per session at subscribe time — simplest to avoid mid-stream evictions and keeps publish path lean.
- **Cap exceed policy:** Support `error` (fail subscribe) and `drop` (reject new subscriber but keep existing ones) mapped to logs/returned error — avoids silent overload and matches acceptance.
- **Idle pruning strategy:** Use per-subscription idle timer reset on publish and cancelled on close — isolates idle detection without global sweeps and reuses existing cleanup to prevent leaks.
- **Instrumentation:** Rely on zap logger for cap-hit/idle-prune visibility (no metrics backend yet) — consistent with current slow-consumer logging.

---

## Supporting Documentation

- Go contexts for cancellation and timeouts: https://pkg.go.dev/context, https://pkg.go.dev/time
- Channel patterns and cancellation: https://go.dev/doc/effective_go#channels
- Current bus implementation and tests: `internal/bus/bus.go`, `internal/bus/memory_bus.go`, `internal/bus/memory_bus_test.go`
- Session event model used by bus: `internal/storage/models.go`
- Epic context: beads issue `yaralpho-b5i.5` (limits and idle pruning)

## Workplan

### Task 1: Rehydrate bus behavior and define limits contract

**Depends on:** None  
**Files:**
- Read: `internal/bus/bus.go`
- Read: `internal/bus/memory_bus.go`
- Read: `internal/bus/memory_bus_test.go`

**Purpose:** Confirm current subscriber lifecycle, cleanup, and slow-consumer policy to anchor limit/idle design against existing patterns.  
**Context to rehydrate:** Review the listed files focusing on subscriber creation, cleanup, and logging.  
**Outcome:** Documented understanding of current bus semantics and where to insert caps/idle timers without breaking ordering.  
**How to Verify:** Notes captured in this plan; no code changes.  
**Acceptance Criteria:**
- [ ] Subscriber creation/cleanup flow understood (Close, Done, watchContext)
- [ ] Current logging and slow-consumer policy behaviors noted
- [ ] No code modifications performed
- [ ] Plan updated with insertion points for caps and timers
- [ ] Scope limited to discovery
**Not In Scope:** Implementing limits or timers.  
**Gotchas:** Watch for shared locks and panic recovery in publish to avoid introducing deadlocks.

### Task 2: Add configuration for caps and policies

**Depends on:** Task 1  
**Files:**
- Modify: `internal/bus/bus.go`
- Modify: `internal/bus/memory_bus.go`
- Test: `internal/bus/memory_bus_test.go`

**Purpose:** Introduce config knobs for max subscribers per session, max sessions overall, idle timeout, and cap-exceed policy with normalized defaults.  
**Context to rehydrate:** Task 1 notes on config usage; existing `Config` and `normalizeConfig` helpers.  
**Outcome:** Bus config exposes new fields with sensible defaults, normalized in `normalizeConfig`, and memory bus consumes them.  
**How to Verify:**  
Run: `go test ./internal/bus/...`  
Expected: Tests compile and cover new config defaults.  
**Acceptance Criteria:**
- [ ] New config fields for max sessions, max subscribers per session, idle timeout, and cap policy
- [ ] Defaults set to conservative non-breaking values
- [ ] Memory bus initializes using normalized config
- [ ] gofmt applied; `go test ./internal/bus/...` passes
- [ ] No functional change beyond config exposure
**Not In Scope:** Enforcing limits or timers; only configuration and wiring.  
**Gotchas:** Keep zero-value backwards compatible; avoid breaking existing callers.

### Task 3: Enforce caps on Subscribe

**Depends on:** Task 2  
**Files:**
- Modify: `internal/bus/memory_bus.go`
- Test: `internal/bus/memory_bus_test.go`

**Purpose:** Prevent new subscribers when per-session or total session caps are hit based on policy (error/drop) with logging.  
**Context to rehydrate:** Task 2 config fields; current `Subscribe` flow and cleanup.  
**Outcome:** Subscribe enforces caps before registering subscriber, emitting logs and returning errors/drop behavior per policy; existing subscribers unaffected.  
**How to Verify:**  
Run: `go test ./internal/bus/... -run Test.*Cap -v` (new tests)  
Expected: Tests covering cap-hit paths pass with expected errors/logs.  
**Acceptance Criteria:**
- [ ] Cap checks performed before bucket insertion
- [ ] Policy `error` returns error and leaves state unchanged
- [ ] Policy `drop` logs and rejects only the new subscriber without touching existing ones
- [ ] go test command above passes
- [ ] No change to publish ordering
**Not In Scope:** Idle timer behavior.  
**Gotchas:** Avoid holding locks across logging that might block; ensure cleanup remains idempotent.

### Task 4: Implement idle pruning with timers

**Depends on:** Task 3  
**Files:**
- Modify: `internal/bus/memory_bus.go`
- Test: `internal/bus/memory_bus_test.go`

**Purpose:** Add per-subscriber idle timers that close and clean up subscriptions after configured inactivity, resetting on publish activity.  
**Context to rehydrate:** Task 3 state handling; subscriber lifecycle (`Close`, `watchContext`), config idle timeout.  
**Outcome:** Subscribers start idle timers on creation, reset on each publish to that subscriber; idle expiration triggers cleanup and removes empty buckets.  
**How to Verify:**  
Run: `go test ./internal/bus/... -run Test.*Idle -v`  
Expected: Idle timer tests confirm channels close and buckets removed after timeout, with publishes resetting timers.  
**Acceptance Criteria:**
- [ ] Idle timer uses configured duration (disabled when zero)
- [ ] Timer reset on publish to subscriber
- [ ] Timeout triggers subscriber close and bucket removal without leaks (Done closes, channel closed)
- [ ] go test command above passes
- [ ] Logging emitted when idle prune occurs
**Not In Scope:** Changing publish fan-out logic beyond timer resets.  
**Gotchas:** Prevent race between timer expiry and explicit Close/context cancel; guard with done channel select.

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes |
| --------------------------- | ------ | ----- |
| Complete                    | ✓      | Covers caps config, enforcement, and idle pruning with tests |
| Accurate                    | ✓      | Paths verified (`internal/bus/*`, `internal/storage/models.go`) |
| Commands valid              | ✓      | `go test ./internal/bus/...` exercises code and tests |
| YAGNI                       | ✓      | Scope limited to bus caps/idle timers; no new metrics backend |
| Minimal                     | ✓      | Four tasks within file/time budgets and single outcomes |
| Not over-engineered         | ✓      | Per-sub timers and subscribe-time caps keep publish hot path unchanged |
| Key Decisions documented    | ✓      | Four decisions captured with rationale |
| Supporting docs present     | ✓      | Links and internal references listed |
| Context sections present    | ✓      | Each task has Purpose and Context; Not In Scope added where needed |
| Budgets respected           | ✓      | Max two production files per task; tests scoped |
| Outcome & Verify present    | ✓      | Each task states outcome and verification commands |
| Acceptance Criteria present | ✓      | Checklists included per task |
| Rehydration context present | ✓      | Tasks include context pointers where dependent |

### Rule-of-Five Passes

| Pass        | Changes Made |
| ----------- | ------------ |
| Draft       | Initial structure with tasks, decisions, verification scaffold |
| Correctness | Verified paths/commands and budgets; ensured defaults/backwards compatibility noted |
| Clarity     | Tightened outcomes/verification wording and Not In Scope notes |
| Edge Cases  | Added idle-disable case (zero timeout) and race/gotcha notes |
| Excellence  | Polished supporting docs and rationale phrasing |
