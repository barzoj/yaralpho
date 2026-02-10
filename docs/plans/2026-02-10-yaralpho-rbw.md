# yaralpho-rbw Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Restore safe, minimal defaults for execution prompts so configuration matches documented examples and unit tests while keeping verification behavior stable.

**Architecture:** Constrain changes to the config loader defaults; reuse existing prompt assembly logic in `consumer.ExecutionTask`/`VerificationTask` without altering runtime flow. Validate via existing unit tests.

**Tech Stack:** Go 1.25, standard library (`os`, `strings`), zap logger, testify.

**Key Decisions:**

- **Execution prompt default:** Use a short placeholder string (`"TODO: execution task prompt"`) instead of the injected multi-line agent instructions — prevents leaking operational instructions and aligns with config.example/test expectations.
- **Verification prompt default:** Keep the structured verification prompt unchanged — preserves deterministic verification automation already validated by tests.
- **Scope containment:** Limit modifications to `internal/config/config.go` and rely on existing tests (`internal/config/config_test.go`) for coverage — avoids touching consumer/runtime behavior.

---

## Supporting Documentation

- Go `os.LookupEnv` behavior (empty strings are not treated as set): https://pkg.go.dev/os#LookupEnv
- Go `strings.TrimSpace` semantics for default handling: https://pkg.go.dev/strings#TrimSpace
- Current config example for prompt defaults: `config.example.json` in repo root (shows placeholder prompts)
- Config loader tests to mirror: `internal/config/config_test.go`

---

## Design

**Section 1: Current observations and risks (self-validated).**  
The config loader seeds defaults for port, retries, and prompt strings, then panics if required keys remain empty. The execution prompt default currently embeds a long, instruction-heavy message (the live agent guidance), which diverges from `config.example.json` and the intended placeholder used by tests. Because the default is injected whenever `YARALPHO_EXECUTION_TASK_PROMPT` is unset, any deployment without an override inherits these verbose instructions, increasing prompt length, coupling to operational wording, and breaking unit expectations (`TestOptionalSlackNotRequired` expects the placeholder). The verification prompt remains an intentionally detailed structured-output guide and already aligns with test assertions. Given the loader trims env values but not defaults, the simplest remediation is to replace the execution default constant with the documented placeholder, letting the rest of the loader logic (token precedence, port default, retries default) remain untouched. No API surface or consumer behavior changes are required beyond the config package.

**Section 2: Planned changes and verification (self-validated).**  
We will replace `defaultExecutionTaskPrompt` in `internal/config/config.go` with the placeholder string used in `config.example.json`, keeping `%s`-format handling intact. No other functional code changes are expected; existing tests already enforce env precedence and fallback behavior, so rerunning `go test ./...` will validate correctness. If whitespace artifacts remain, we will trim via `strings.TrimSpace` in the constant definition rather than adding new runtime logic to keep risk low. Testing focuses on the config package plus a full suite run to ensure no regressions. No migrations or additional dependencies are required. Once implemented, the default configuration matches documentation, and future regressions will be caught by existing tests without expanding scope.

---

## Tasks

### Task 1: Restore execution prompt default to placeholder

**Depends on:** None  
**Files:**
- Modify: `internal/config/config.go` (constant definition)
- Test: `internal/config/config_test.go`

**Purpose:** Align default execution prompt with documented placeholder to remove operational instructions from defaults and satisfy unit expectations.

**Context to rehydrate:**
- Review `defaultExecutionTaskPrompt` in `internal/config/config.go`
- Compare with `config.example.json` and `TestOptionalSlackNotRequired`

**Outcome:** Default execution prompt equals `"TODO: execution task prompt"` when env/JSON are unset, matching docs and tests.

**How to Verify:**  
Run: `go test ./internal/config -run TestOptionalSlackNotRequired -v`  
Expected: PASS; execution prompt assertion equals placeholder.

**Acceptance Criteria:**
- [ ] Unit test(s): `TestOptionalSlackNotRequired` passes
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: N/A
- [ ] Outputs match expectations from How to Verify
- [ ] Interface and implementation separation maintained (config only)
- [ ] No additional files altered beyond listed ones

**Not In Scope:** Changing verification prompt or loader logic beyond the default constant.

**Gotchas:** Keep `%s` placeholder handling intact; avoid introducing trailing whitespace.

### Task 2: Run full test suite and summarize code review findings

**Depends on:** Task 1  
**Files:**
- Test: `./...` (command only)
- Notes: summary in commit message/final report (no repo doc changes)

**Purpose:** Confirm no regressions and document the addressed code smell (default prompt mismatch) plus any additional observations.

**Context to rehydrate:**
- Ensure Task 1 changes are present
- Prior test failure: `TestOptionalSlackNotRequired` mismatch

**Outcome:** Full test suite passes; summary notes capture the prompt-default regression and its fix.

**How to Verify:**  
Run: `go test ./...`  
Expected: All packages PASS.

**Acceptance Criteria:**
- [ ] Unit test(s): Entire suite passes
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: N/A
- [ ] Outputs match expectations from How to Verify
- [ ] Commit message references task and summarizes code review findings

**Not In Scope:** Adding new tests or features beyond the prompt default fix.

**Gotchas:** None; ensure clean working tree before commit.

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes |
| --------------------------- | ------ | ----- |
| Complete                    | ✓ | Addresses execution prompt default regression and test failure |
| Accurate                    | ✓ | Paths verified: `internal/config/config.go`, `internal/config/config_test.go`, `config.example.json` |
| Commands valid              | ✓ | `go test ./internal/config -run TestOptionalSlackNotRequired -v`, `go test ./...` |
| YAGNI                       | ✓ | No extra features beyond prompt default fix |
| Minimal                     | ✓ | Single constant change plus tests run |
| Not over‑engineered         | ✓ | Avoided new abstractions or config plumbing |
| Key Decisions documented    | ✓ | Three listed in header |
| Supporting docs present     | ✓ | Links and file references included |
| Context sections present    | ✓ | Purpose/Not In Scope/Gotchas provided per task |
| Budgets respected           | ✓ | Each task ≤2 files, single outcome |
| Outcome & Verify present    | ✓ | Each task includes Outcome and How to Verify |
| Acceptance Criteria present | ✓ | Checklists per task |
| Rehydration context present | ✓ | Included where prior work matters |

### Rule‑of‑Five Passes

| Pass        | Changes Made |
| ----------- | ------------ |
| Draft       | Initial structure, tasks, and verification record filled |
| Correctness | Confirmed paths/commands align with repo state |
| Clarity     | Simplified scope statements and outcomes |
| Edge Cases  | Noted placeholder handling and whitespace risk |
| Excellence  | Polished phrasing and ensured budgets/sections complete |
