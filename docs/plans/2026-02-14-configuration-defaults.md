# Configuration Defaults (yaralpho-hgo.22) Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Make scheduler interval, max retries, and restart wait timeout configurable with safe defaults and wiring.

**Architecture:** Extend the existing env-first config loader with new keys and defaults, expose typed values at wiring sites, and pass them into scheduler/restart handling without changing underlying scheduler semantics. Keep configuration parsing centralized in `internal/config` and inject values at the application boundary.

**Tech Stack:** Go 1.25, zap logging, env/JSON-backed config, gorilla/mux, existing scheduler package.

**Key Decisions:**

- **Env-first configuration with JSON fallback**: Reuse current loader to avoid duplicating config parsing logic while honoring environment precedence.
- **Duration parsing at the edge**: Parse interval/timeout strings when wiring components rather than embedding parsing into the config map, keeping `config.Config` interface stable.
- **Conservative defaults**: 10s scheduler interval and 30s restart wait timeout to balance responsiveness with load and CI wait behavior; retain existing default maxRetries=5.

---

## Supporting Documentation

- Go `time.ParseDuration` for duration strings (e.g., "10s", "1m"): https://pkg.go.dev/time#ParseDuration
- Go `os.LookupEnv` precedence and trimming rules used in current config loader: https://pkg.go.dev/os#LookupEnv
- Zap structured logging patterns for config emission: https://pkg.go.dev/go.uber.org/zap

---

## Tasks

### Task 1: Add config keys and defaults for scheduler interval/max retries/restart wait

**Depends on:** None  
**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

**Purpose:** Introduce new env keys (`YARALPHO_SCHEDULER_INTERVAL`, `YARALPHO_RESTART_WAIT_TIMEOUT`) with documented defaults and ensure maxRetries default remains explicit.

**Context to rehydrate:**
- Current config defaults and env precedence in `internal/config/config.go`
- Existing tests for defaults in `internal/config/config_test.go`

**Outcome:** Config loader recognizes the new keys, applies defaults (10s interval, 30s wait timeout, 5 maxRetries), and tests cover env override and fallback behavior.

**How to Verify:**  
Run: `go test ./internal/config -run TestEnvOverridesAndTokenPrecedence|TestOptionalSlackNotRequired`  
Expected: Tests pass with assertions for new keys and defaults.

**Acceptance Criteria:**
- [ ] Unit tests updated to cover new keys and defaults
- [ ] Env override precedence maintained (env over JSON)
- [ ] Defaults applied when env unset
- [ ] Loggable keys updated to include new non-secret values
- [ ] No changes to Config interface shape

**Not In Scope:** Wiring scheduler start logic or restart handler behavior.

**Gotchas:** Duration strings must be valid `time.ParseDuration` inputs; keep stored values as strings to preserve existing Config contract.

### Task 2: Wire restart wait timeout into restart handler

**Depends on:** Task 1  
**Files:**
- Modify: `internal/app/restart_handler.go`
- Modify: `internal/app/restart_handler_test.go`

**Purpose:** Enforce a configurable timeout for wait-mode restarts to prevent indefinite blocking.

**Context to rehydrate:**
- Restart handler behavior in `internal/app/restart_handler.go`
- Scheduler fake tests in `internal/app/restart_handler_test.go`

**Outcome:** `wait=true` requests use a timeout derived from config (default 30s) when waiting for draining; timeouts return 408 with clear messaging.

**How to Verify:**  
Run: `go test ./internal/app -run TestRestartHandler`  
Expected: Tests cover timeout path and pass.

**Acceptance Criteria:**
- [ ] Restart handler uses configured timeout value
- [ ] Timeout returns 408 with informative body
- [ ] Non-wait behavior unchanged (202)
- [ ] Existing tests still pass

**Not In Scope:** Scheduler drain implementation beyond existing behavior.

**Gotchas:** Ensure context cancellation from request still respected; avoid leaking goroutines when timeout triggers.

### Task 3: Inject scheduler config values into scheduler construction

**Depends on:** Task 1  
**Files:**
- Modify: `internal/app/app.go` (or scheduler wiring file once available)
- Test: `internal/app/app_test.go` (add/adjust to cover options)

**Purpose:** Ensure scheduler receives interval and maxRetries from config when instantiated so runtime matches configuration.

**Context to rehydrate:**
- App construction path in `internal/app/app.go`
- Scheduler options defaults in `internal/scheduler/scheduler.go`

**Outcome:** Scheduler is constructed with options derived from config strings (parsed durations and ints) with validation and fallbacks.

**How to Verify:**  
Run: `go test ./internal/app -run TestSchedulerConfigOptions` (add new test)  
Expected: Scheduler receives configured interval/maxRetries values or defaults on invalid input.

**Acceptance Criteria:**
- [ ] Interval parsed from config and passed to scheduler; defaults to 10s on invalid/empty
- [ ] MaxRetries parsed as int with fallback to default on invalid/empty
- [ ] Errors surfaced for irrecoverably invalid config values
- [ ] Logging includes chosen interval/maxRetries for visibility

**Not In Scope:** Implementing scheduler Start loop or tick scheduling cadence.

**Gotchas:** Keep parsing errors non-fatal by falling back to defaults where safe; avoid changing Config interface.

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes |
| --------------------------- | ------ | ----- |
| Complete                    | ✓ | Covers scheduler interval, maxRetries, restart wait timeout wiring/tests |
| Accurate                    | ✓ | Paths verified in repo for config/app/restart files |
| Commands valid              | ✓ | go test targets exist (`./internal/config`, `./internal/app`) |
| YAGNI                       | ✓ | No extra endpoints or config surfaces beyond task ask |
| Minimal                     | ✓ | Three tasks, each limited to two production files |
| Not over-engineered         | ✓ | Parsing kept at wiring layer; Config interface unchanged |
| Key Decisions documented    | ✓ | Three decisions captured in header |
| Supporting docs present     | ✓ | Go time/env/zap references listed |
| Context sections present    | ✓ | Each task has Purpose and Context to rehydrate |
| Budgets respected           | ✓ | Tasks limited to ≤2 prod files and single outcomes |
| Outcome & Verify present    | ✓ | Every task lists outcome and how to verify |
| Acceptance Criteria present | ✓ | Checklists provided per task |
| Rehydration context present | ✓ | Included where prior context needed |

### Rule-of-Five Passes

| Pass        | Changes Made |
| ----------- | ------------ |
| Draft       | Added tasks, files, dependencies, and verification steps |
| Correctness | Verified commands, file paths, and defaults align with existing code |
| Clarity     | Tightened task outcomes and acceptance criteria wording |
| Edge Cases  | Called out invalid duration parsing fallback and timeout handling |
| Excellence  | Ensured supporting docs and key decisions are succinct |
