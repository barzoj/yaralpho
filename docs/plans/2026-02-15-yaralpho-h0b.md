# yaralpho-h0b Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Add a restart path that drains in-flight work and then stops the app (scheduler loop and HTTP server) when `/restart?wait=true` is called, while keeping `/restart?wait=false` behavior unchanged.

**Architecture:** Extend the app run loop to expose a shutdown request that cancels the run context and triggers the existing graceful shutdown path, and wire the restart handler to invoke it only after the scheduler reports idle. Keep draining as the guard that prevents new ticks and rely on context cancellation to stop loops and the HTTP server.

**Tech Stack:** Go, net/http, gorilla/mux, zap logger.

**Key Decisions:**

- **Shutdown trigger mechanism:** Store the app run context cancel func and expose a guarded shutdown request rather than adding a separate signal channel—keeps shutdown reuse with existing Run logic.
- **Draining + cancel ordering:** Always SetDraining before waiting; only cancel after WaitForIdle succeeds so no new work starts and shutdown happens after drain.
- **Scheduler Stop contract:** Keep Stop idempotent and draining-safe, reusing SetDraining without introducing extra goroutines to minimize state surface.

---

## Supporting Documentation

- Go net/http graceful shutdown: https://pkg.go.dev/net/http#Server.Shutdown (shutdown waits for in-flight handlers).
- Go context cancellation: https://pkg.go.dev/context#WithCancel (share cancel func to stop loops).
- sync.WaitGroup semantics: https://pkg.go.dev/sync#WaitGroup (Wait blocks until all done).
- atomic.Bool usage: https://pkg.go.dev/sync/atomic#Bool (draining flag already uses this type).

---

## Tasks

### Task 1: Add app shutdown hook

**Depends on:** None  
**Files:**

- Modify: `internal/app/app.go`
- Test: `internal/app/app_test.go`

**Purpose:** Allow the app to be shut down on demand by canceling the run context while reusing the existing graceful shutdown flow.

**Context to rehydrate:**

- Read `App.Run` loop in `internal/app/app.go` for current shutdown behavior.

**Outcome:** App exposes a safe shutdown request that cancels the run context once and causes the scheduler loop and HTTP server to stop gracefully.

**How to Verify:**  
Run: `go test ./internal/app -run TestAppShutdownRequested`  
Expected: Test passes showing Run exits after shutdown request.

**Acceptance Criteria:**

- [ ] Unit test covers shutdown request exiting Run.
- [ ] Manual/E2E: N/A (not required here).
- [ ] Outputs match expectations from How to Verify.
- [ ] Interface and implementation separation preserved (shutdown via App method, not handlers).
- [ ] No DB/provider leakage.

**Not In Scope:** Restart handler changes; scheduler draining semantics.

**Gotchas:** Guard shutdown with sync.Once to avoid double cancel from repeated calls.

### Task 2: Make scheduler Stop drain-safe

**Depends on:** Task 1  
**Files:**

- Modify: `internal/scheduler/scheduler.go`
- Modify: `internal/scheduler/interface.go`
- Test: `internal/app/app_test.go` (fakes) or new scheduler test file if needed

**Purpose:** Ensure Stop enables draining and is safe to call during shutdown so no new ticks start.

**Context to rehydrate:**

- Review `Scheduler.Stop` and draining guard in `Scheduler.Tick`.

**Outcome:** Scheduler Stop sets draining, is idempotent, and advertises the contract through the controller interface for shutdown orchestration.

**How to Verify:**  
Run: `go test ./internal/scheduler ./internal/app -run TestSchedulerStop.* -v`  
Expected: Tests pass showing Stop sets draining and cooperates with shutdown paths.

**Acceptance Criteria:**

- [ ] Unit test(s) assert Stop sets draining and is safe.
- [ ] Manual/E2E: N/A.
- [ ] Outputs match expectations from How to Verify.
- [ ] Draining prevents new ticks after Stop.

**Not In Scope:** Changing Tick scheduling cadence or worker logic.

**Gotchas:** Keep Stop nil-safe for tests using nil scheduler references.

### Task 3: Wire restart handler to trigger shutdown after drain

**Depends on:** Task 1, Task 2  
**Files:**

- Modify: `internal/app/restart_handler.go`
- Test: `internal/app/restart_handler_test.go`

**Purpose:** `/restart?wait=true` should drain in-flight runs, then request app shutdown; `/restart?wait=false` should keep current draining-only behavior.

**Context to rehydrate:**

- Existing restart handler behavior and tests.

**Outcome:** Wait=true path sets draining, waits for idle, triggers app shutdown, and returns 200 with drained status; wait=false still returns 202 immediately and keeps process running.

**How to Verify:**  
Run: `go test ./internal/app -run TestRestartHandler -v`  
Expected: Tests cover wait=true shutdown, wait timeout, and wait=false unchanged behavior.

**Acceptance Criteria:**

- [ ] Unit test covers wait=true draining + shutdown.
- [ ] Unit test covers wait=false 202 behavior unchanged.
- [ ] Manual/E2E: N/A.
- [ ] No new work scheduled after draining set.

**Not In Scope:** Batch restart routes.

**Gotchas:** Avoid calling shutdown on failed WaitForIdle or bad wait param.

### Task 4: Integration test for restart-triggered shutdown

**Depends on:** Task 3  
**Files:**

- Modify: `internal/app/app_test.go`
- Test only (no production file beyond app.go already touched)

**Purpose:** Prove that `/restart?wait=true` drains a fake scheduler and causes Run to exit, while wait=false does not shut down immediately.

**Context to rehydrate:**

- App Run loop and restart handler behavior from prior tasks.

**Outcome:** Integration test drives HTTP server with fake scheduler, sets active runs >0, verifies wait=true blocks until idle then Run exits; wait=false returns 202 and Run continues.

**How to Verify:**  
Run: `go test ./internal/app -run TestRestart.*Shutdown -v`  
Expected: Tests pass demonstrating shutdown on wait=true and continued run on wait=false.

**Acceptance Criteria:**

- [ ] Integration test asserts Run exits after wait=true drain.
- [ ] Integration test asserts wait=false keeps app running while draining.
- [ ] Outputs match expectations from How to Verify.
- [ ] No goroutine leaks in test (wg waits).

**Not In Scope:** Real scheduler ticking or worker processing.

**Gotchas:** Ensure test HTTP server uses ephemeral port to avoid conflicts.

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes                                     |
| --------------------------- | ------ | ----------------------------------------- |
| Complete                    | ✓      | Addresses wait=true shutdown, wait=false drain, scheduler stop. |
| Accurate                    | ✓      | Paths verified: app.go, scheduler.go/interface.go, restart handler/tests. |
| Commands valid              | ✓      | go test targets exist or will be added under internal/app and scheduler. |
| YAGNI                       | ✓      | Scope limited to shutdown wiring and tests. |
| Minimal                     | ✓      | Four small tasks, ≤2 prod files each. |
| Not over-engineered         | ✓      | Reuses Run cancel + draining without new services. |
| Key Decisions documented    | ✓      | Three decisions captured in header. |
| Supporting docs present     | ✓      | Links to net/http shutdown, context, WaitGroup, atomic. |
| Context sections present    | ✓      | Tasks include Purpose/Context and Not In Scope. |
| Budgets respected           | ✓      | Tasks fit time/file/outcome budgets. |
| Outcome & Verify present    | ✓      | Each task lists outcome and exact verify commands. |
| Acceptance Criteria present | ✓      | Acceptance checklists per task. |
| Rehydration context present | ✓      | All tasks include context bullets. |

### Rule-of-Five Passes

| Pass        | Changes Made                             |
| ----------- | ---------------------------------------- |
| Draft       |                                          |
| Correctness |                                          |
| Clarity     |                                          |
| Edge Cases  |                                          |
| Excellence  |                                          |
