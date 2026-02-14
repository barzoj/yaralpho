# Codex Provider Integration Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Add first-class Codex provider support (default agent) using a standalone Linux x64 wrapper binary while preserving existing GitHub Copilot behavior.

**Architecture:** Introduce provider selection at app composition time (`--agent`, default `codex`) and implement a new `internal/copilot.Codex` client that spawns a local wrapper process. The wrapper, implemented in TypeScript with the Codex SDK, streams raw events as NDJSON on stdout; Go forwards these as `RawEvent`, synthesizes a terminal `session.idle` event on child exit, and keeps lifecycle semantics compatible with current consumer/storage flow.

**Tech Stack:** Go 1.x (`os/exec`, `bufio`, `encoding/json`, `flag`), TypeScript/Node.js, Codex TypeScript SDK, standalone binary packaging for Linux x64 (`pkg`), existing test stack (`go test`).

**Supporting Documentation**

- Codex SDK streaming sample (primary implementation reference): https://github.com/openai/codex/blob/main/sdk/typescript/samples/basic_streaming.ts
  - Use the same event subscription/streaming style; do not re-shape payloads unless required for stable top-level `type`.
- Codex SDK package docs (API shape and auth expectations): https://www.npmjs.com/package/@openai/codex-sdk
  - Validate exact constructor/session API before writing wrapper.
- Go `os/exec` docs (process lifecycle, context cancellation, stdio pipes): https://pkg.go.dev/os/exec
  - Use `exec.CommandContext` and explicit `Wait()` to avoid process leaks.
- Go `bufio.Scanner` docs (line scanning and token limit caveats): https://pkg.go.dev/bufio#Scanner
  - Increase scanner buffer to handle large JSON lines.
- Go JSON docs (`map[string]any` decode, tolerant parsing): https://pkg.go.dev/encoding/json
  - Decode each stdout line into `copilot.RawEvent`.
- Node CLI args parsing (stable basic parsing with `process.argv`): https://nodejs.org/api/process.html#processargv
  - Keep wrapper arguments explicit and minimal (`--repo-path`, `--prompt`).
- `pkg` packaging docs (single-file executable build): https://www.npmjs.com/package/pkg
  - Pin tool version in project scripts for reproducibility.

**Key Decisions:**

- **Provider selection location:** `cmd` flag + `app.BuildWithOptions` — keeps CLI explicit while preserving compatibility for tests/callers using existing `Build`.
- **Transport contract:** NDJSON over child `stdout` — simple streaming IPC, language-agnostic, easy to persist as raw map payloads.
- **End-of-session signal:** Go synthesizes `{"type":"session.idle"}` when child exits — preserves current consumer stop condition (`isSessionIdleEvent`) without touching storage/UI paths.
- **Failure policy:** hard-fail when selected provider is unavailable — matches explicit requirement and avoids hidden fallback behavior.
- **Packaging target:** Linux x64 standalone executable for first release — smallest useful production scope with clear future extension path.

---

### Task 1: Add Agent Selection Surface in CLI

**Depends on:** None  
**Files:**
- Modify: `cmd/main.go`
- Test: `cmd/main_test.go` (create if missing)

**Purpose:** Make provider selection explicit and deterministic at process start.

**Context to rehydrate:**
- Read `cmd/main.go`
- Run `go test ./cmd/...`

**Subagent Input (detailed execution brief):**
- Add `--agent` flag in `cmd/main.go` with default `codex`.
- Accept only `codex` and `github`; invalid values must print to stderr and exit code `2`.
- Keep existing `--config` and `--debug-level` behavior unchanged.
- Pass the parsed value into app construction (Task 2 adds the new app API).
- Create `cmd/main_test.go` with table-driven tests for accepted/invalid values and default behavior.

**Outcome:** CLI can parse/validate agent choice and defaults to Codex.

**How to Verify:**
Run: `go test ./cmd/... -v`  
Expected: tests pass, including default=`codex` and invalid-agent rejection.

**Acceptance Criteria:**
- [ ] Unit test(s): `cmd/main_test.go` verify parsing and validation.
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: `go run ./cmd --agent=invalid` exits with status 2.
- [ ] Outputs match expectations from How to Verify.
- [ ] Interface and implementation separation - one task for interface definition, one or more for concrete implementation(s) (if applicable)
- [ ] DAL layer tasks do not leak database-specific types; interface-based design for testability and flexibility (if applicable)

**Not In Scope:**
- Adding config/env-based fallback for agent selection.

**Gotchas:**
- Keep flag parsing side effects minimal so tests remain deterministic.

### Task 2: Introduce App Build Options and Provider Factory

**Depends on:** Task 1  
**Files:**
- Modify: `internal/app/app.go`
- Test: `internal/app/app_test.go`

**Purpose:** Centralize provider selection in composition root without breaking existing call sites.

**Context to rehydrate:**
- Read `internal/app/app.go`
- Read current `Build` usages with `rg -n "app.Build\\("`

**Subagent Input (detailed execution brief):**
- Add `type BuildOptions struct { Agent string }` in `internal/app/app.go`.
- Add `BuildWithOptions(ctx, logger, cfg, opts)`; keep existing `Build` as compatibility wrapper to `BuildWithOptions(..., BuildOptions{})`.
- In `BuildWithOptions`, resolve agent:
  - empty => `codex`
  - `codex` => `copilot.NewCodex(logger)`
  - `github` => `copilot.NewGitHub(logger)`
  - otherwise return error.
- Keep all non-copilot wiring unchanged.
- Extend `internal/app/app_test.go` with agent selection tests using existing stubs/fakes.

**Outcome:** App can construct either provider based on explicit option while defaulting to Codex.

**How to Verify:**
Run: `go test ./internal/app -v`  
Expected: tests confirm default codex, explicit github, and invalid option error.

**Acceptance Criteria:**
- [ ] Unit test(s): `internal/app/app_test.go` for provider selection.
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: `go test ./internal/app -run Build -v` succeeds.
- [ ] Outputs match expectations from How to Verify.
- [ ] Interface and implementation separation - one task for interface definition, one or more for concrete implementation(s) (if applicable)
- [ ] DAL layer tasks do not leak database-specific types; interface-based design for testability and flexibility (if applicable)

**Not In Scope:**
- Changing worker/consumer execution logic.

### Task 3: Add Codex Client Stub (`codex.go`)

**Depends on:** None  
**Files:**
- Create: `internal/copilot/codex.go`
- Test: `internal/copilot/codex_test.go`

**Purpose:** Establish provider shape and constructor before subprocess behavior.

**Context to rehydrate:**
- Read `internal/copilot/interface.go`
- Read `internal/copilot/github.go` for lifecycle semantics

**Subagent Input (detailed execution brief):**
- Create `type Codex struct` with logger and injectable command runner hooks suitable for testing.
- Add `func NewCodex(logger *zap.Logger) *Codex`.
- Implement `StartSession` no-op stub returning explicit `not implemented` error for now.
- Add compile-time assertion `var _ Client = (*Codex)(nil)`.
- Add tests for constructor defaults and stub error contract.

**Outcome:** Codex provider exists in codebase and is selectable by app.

**How to Verify:**
Run: `go test ./internal/copilot -run Codex -v`  
Expected: tests pass and interface implementation compiles.

**Acceptance Criteria:**
- [ ] Unit test(s): `internal/copilot/codex_test.go`.
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: full package compile includes new provider.
- [ ] Outputs match expectations from How to Verify.
- [ ] Interface and implementation separation - one task for interface definition, one or more for concrete implementation(s) (if applicable)
- [ ] DAL layer tasks do not leak database-specific types; interface-based design for testability and flexibility (if applicable)

**Gotchas:**
- Keep exported surface minimal to avoid lock-in before subprocess behavior lands.

### Task 4: Scaffold `codex-ts` Project Metadata

**Depends on:** None  
**Files:**
- Create: `internal/copilot/codex-ts/package.json`
- Create: `internal/copilot/codex-ts/tsconfig.json`

**Purpose:** Establish a reproducible TypeScript project baseline with deterministic build scripts.

**Context to rehydrate:**
- Read `internal/copilot/AGENTS.md`
- Read plan supporting docs links for Codex SDK + `pkg`

**Subagent Input (detailed execution brief):**
- Create `package.json` with scripts:
  - `build` (`tsc -p tsconfig.json`)
  - `start` (`node dist/main.js`)
  - placeholder `package:linux-x64` (implemented fully in Task 7)
- Add dependencies: `@openai/codex-sdk`.
- Add dev dependencies: `typescript`, `@types/node`, `pkg` (pinned major versions).
- Add `tsconfig.json` targeting Node runtime compatible with packaging.
- Keep scope limited to metadata only (no wrapper logic in this task).

**Outcome:** `npm install` and `npm run build` are possible once source file exists.

**How to Verify:**
Run: `cd internal/copilot/codex-ts && npm install`  
Expected: dependencies install without unresolved package/script errors.

**Acceptance Criteria:**
- [ ] Unit test(s): N/A
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: `npm run` lists expected scripts.
- [ ] Outputs match expectations from How to Verify.
- [ ] Interface and implementation separation - one task for interface definition, one or more for concrete implementation(s) (if applicable)
- [ ] DAL layer tasks do not leak database-specific types; interface-based design for testability and flexibility (if applicable)

**Not In Scope:**
- Creating runtime wrapper source file.

### Task 5: Add Wrapper CLI Skeleton and Usage Contract

**Depends on:** Task 4  
**Files:**
- Create: `internal/copilot/codex-ts/src/main.ts`
- Create: `internal/copilot/codex-ts/README.md`

**Purpose:** Define the command-line contract before integrating SDK streaming.

**Context to rehydrate:**
- Read `internal/copilot/codex-ts/package.json`
- Read Node argv docs in supporting documentation

**Subagent Input (detailed execution brief):**
- Implement minimal CLI parser in `src/main.ts` for:
  - `--repo-path <path>`
  - `--prompt <text>`
- Validate required args; print errors to stderr and exit non-zero.
- Add simple `--help` usage output.
- Ensure `npm run build` produces `dist/main.js`.
- Document CLI contract and expected stdout/stderr separation in README.

**Outcome:** Wrapper executable contract exists and is testable without SDK runtime.

**How to Verify:**
Run: `cd internal/copilot/codex-ts && npm run build && node dist/main.js --help`  
Expected: usage output appears; missing args exit non-zero with stderr message.

**Acceptance Criteria:**
- [ ] Unit test(s): N/A
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: both required flags are validated.
- [ ] Outputs match expectations from How to Verify.
- [ ] Interface and implementation separation - one task for interface definition, one or more for concrete implementation(s) (if applicable)
- [ ] DAL layer tasks do not leak database-specific types; interface-based design for testability and flexibility (if applicable)

### Task 6: Implement Codex SDK Streaming in Wrapper

**Depends on:** Task 5  
**Files:**
- Modify: `internal/copilot/codex-ts/src/main.ts`
- Modify: `internal/copilot/codex-ts/README.md`

**Purpose:** Turn wrapper into an NDJSON stream producer consumable by Go.

**Context to rehydrate:**
- Read `internal/copilot/codex-ts/src/main.ts`
- Read Codex sample: `https://github.com/openai/codex/blob/main/sdk/typescript/samples/basic_streaming.ts`

**Subagent Input (detailed execution brief):**
- Create Codex client/session following current official sample pattern.
- Use parsed `repo-path` and `prompt` to start the run.
- For each SDK event:
  - serialize as one JSON object per line
  - write only to stdout
- Never emit non-JSON content to stdout.
- Route diagnostics/errors exclusively to stderr.
- Exit non-zero on auth/init/stream fatal errors.
- Update README with streaming event contract and examples.

**Outcome:** Wrapper emits newline-delimited JSON events for transport layer.

**How to Verify:**
Run: `cd internal/copilot/codex-ts && npm run build && node dist/main.js --repo-path /tmp --prompt "hello"`  
Expected: stdout emits JSON lines (or stderr provides clear auth/setup error).

**Acceptance Criteria:**
- [ ] Unit test(s): N/A (CLI-level wrapper).
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: stdout contains only JSON objects (one per line).
- [ ] Outputs match expectations from How to Verify.
- [ ] Interface and implementation separation - one task for interface definition, one or more for concrete implementation(s) (if applicable)
- [ ] DAL layer tasks do not leak database-specific types; interface-based design for testability and flexibility (if applicable)

**Not In Scope:**
- Binary packaging behavior.

### Task 7: Package Linux x64 Standalone Wrapper Artifact

**Depends on:** Task 6  
**Files:**
- Modify: `internal/copilot/codex-ts/package.json`
- Modify: `internal/copilot/codex-ts/README.md`

**Purpose:** Produce deterministic standalone executable for Go runtime invocation.

**Context to rehydrate:**
- Read `internal/copilot/codex-ts/package.json`
- Read `internal/copilot/codex-ts/README.md`

**Subagent Input (detailed execution brief):**
- Finalize `package:linux-x64` script using `pkg`.
- Output path must be fixed: `internal/copilot/codex-ts/bin/codex-wrapper-linux-x64`.
- Document exact build sequence and artifact expectations.
- Ensure script behavior is deterministic across reruns.

**Outcome:** Linux x64 executable artifact exists at known path.

**How to Verify:**
Run: `cd internal/copilot/codex-ts && npm run package:linux-x64 && ./bin/codex-wrapper-linux-x64 --help`  
Expected: binary runs and shows usage/arg guidance without crash.

**Acceptance Criteria:**
- [ ] Unit test(s): N/A
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: output binary is executable and callable.
- [ ] Outputs match expectations from How to Verify.
- [ ] Interface and implementation separation - one task for interface definition, one or more for concrete implementation(s) (if applicable)
- [ ] DAL layer tasks do not leak database-specific types; interface-based design for testability and flexibility (if applicable)

**Gotchas:**
- `pkg` can break on dynamic imports; keep imports static.

### Task 8: Implement Wrapper Path Resolution in Go Codex Client

**Depends on:** Task 3, Task 7  
**Files:**
- Modify: `internal/copilot/codex.go`
- Test: `internal/copilot/codex_test.go`

**Purpose:** Resolve wrapper path safely and fail fast with actionable errors.

**Context to rehydrate:**
- Read `internal/copilot/codex.go`
- Read logging patterns in `internal/copilot/github.go`

**Subagent Input (detailed execution brief):**
- Implement resolver precedence:
  1. `YARALPHO_CODEX_WRAPPER_PATH` env override
  2. repo-relative default `internal/copilot/codex-ts/bin/codex-wrapper-linux-x64`
  3. descriptive error when unresolved
- Validate executable exists and has executable permission.
- Add structured logs without leaking prompt or credentials.
- Add tests for override precedence and failure messages.

**Outcome:** Go client either finds runnable wrapper or fails explicitly.

**How to Verify:**
Run: `go test ./internal/copilot -run CodexPath -v`  
Expected: precedence and failure-path tests pass.

**Acceptance Criteria:**
- [ ] Unit test(s): path resolution cases in `internal/copilot/codex_test.go`.
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: env override path wins when set.
- [ ] Outputs match expectations from How to Verify.
- [ ] Interface and implementation separation - one task for interface definition, one or more for concrete implementation(s) (if applicable)
- [ ] DAL layer tasks do not leak database-specific types; interface-based design for testability and flexibility (if applicable)

### Task 9: Implement Process Launch and Stdout Event Pump

**Depends on:** Task 8  
**Files:**
- Modify: `internal/copilot/codex.go`
- Test: `internal/copilot/codex_test.go`

**Purpose:** Add subprocess transport for raw event streaming.

**Context to rehydrate:**
- Read `internal/copilot/codex.go`
- Read `internal/copilot/interface.go`

**Subagent Input (detailed execution brief):**
- Replace stub `StartSession` with real process launch using `exec.CommandContext`.
- Pass `--repo-path` and `--prompt` to wrapper.
- Capture stdout and stderr pipes.
- Parse stdout with line scanner (expanded buffer) and decode each line into `RawEvent`.
- Forward events to buffered output channel with cancellation-aware sends.
- Drain stderr concurrently and log each line.
- Return session id and idempotent `stop` callback.

**Outcome:** Codex StartSession streams raw events from wrapper process.

**How to Verify:**
Run: `go test ./internal/copilot -run CodexStartSessionStreams -v`  
Expected: fake-wrapper test confirms events are forwarded in order.

**Acceptance Criteria:**
- [ ] Unit test(s): stream-forwarding tests in `internal/copilot/codex_test.go`.
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: fake wrapper output appears as `RawEvent` values.
- [ ] Outputs match expectations from How to Verify.
- [ ] Interface and implementation separation - one task for interface definition, one or more for concrete implementation(s) (if applicable)
- [ ] DAL layer tasks do not leak database-specific types; interface-based design for testability and flexibility (if applicable)

**Gotchas:**
- Scanner default token limit must be increased for large events.

### Task 10: Add Lifecycle Completion and Synthetic Idle Event

**Depends on:** Task 9  
**Files:**
- Modify: `internal/copilot/codex.go`
- Test: `internal/copilot/codex_test.go`

**Purpose:** Preserve consumer run-completion behavior with explicit idle signal.

**Context to rehydrate:**
- Read `internal/consumer/worker.go` (`isSessionIdleEvent`)
- Read `internal/consumer/task_helpers.go` event loop

**Subagent Input (detailed execution brief):**
- On normal child exit, emit exactly one synthetic `{"type":"session.idle"}`.
- Ensure event channel closes exactly once.
- Ensure `stop` is idempotent and safe before/after child termination.
- Add tests for:
  - natural exit => single idle + close
  - repeated stop calls => no panic/double-close
  - context cancellation => graceful shutdown path

**Outcome:** Consumer stops naturally for Codex provider without changes.

**How to Verify:**
Run: `go test ./internal/copilot -run CodexIdle -v`  
Expected: one idle event then channel closure.

**Acceptance Criteria:**
- [ ] Unit test(s): idle lifecycle tests in `internal/copilot/codex_test.go`.
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: consumer-style loop exits after idle event.
- [ ] Outputs match expectations from How to Verify.
- [ ] Interface and implementation separation - one task for interface definition, one or more for concrete implementation(s) (if applicable)
- [ ] DAL layer tasks do not leak database-specific types; interface-based design for testability and flexibility (if applicable)

### Task 11: Harden JSON/Event Error Handling in Go Pump

**Depends on:** Task 9  
**Files:**
- Modify: `internal/copilot/codex.go`
- Test: `internal/copilot/codex_test.go`

**Purpose:** Make event transport resilient to malformed output while preserving completion semantics.

**Context to rehydrate:**
- Read event pump implementation in `internal/copilot/codex.go`
- Read logging style in `internal/copilot/github.go`

**Subagent Input (detailed execution brief):**
- On malformed JSON line:
  - log warning (include byte length, avoid full payload dump for very long lines)
  - skip the line and continue processing
- On scanner hard error:
  - log error
  - continue to termination path that still emits idle event
- Add tests for malformed-line skip and scanner error behavior.

**Outcome:** Transport continues through malformed lines and still terminates correctly.

**How to Verify:**
Run: `go test ./internal/copilot -run CodexMalformed -v`  
Expected: malformed lines skipped; valid lines and idle event still emitted.

**Acceptance Criteria:**
- [ ] Unit test(s): malformed/error tests in `internal/copilot/codex_test.go`.
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: no deadlock when bad line appears mid-stream.
- [ ] Outputs match expectations from How to Verify.
- [ ] Interface and implementation separation - one task for interface definition, one or more for concrete implementation(s) (if applicable)
- [ ] DAL layer tasks do not leak database-specific types; interface-based design for testability and flexibility (if applicable)

**Not In Scope:**
- Wrapper-side retries or backoff.

### Task 12: Wire CLI Agent Into App Build Callsite

**Depends on:** Task 1, Task 2  
**Files:**
- Modify: `cmd/main.go`
- Test: `cmd/main_test.go`

**Purpose:** Complete runtime path from CLI argument to selected provider instance.

**Context to rehydrate:**
- Read `cmd/main.go`
- Read `internal/app/app.go` (`BuildWithOptions`)

**Subagent Input (detailed execution brief):**
- Replace `app.Build(...)` usage with `app.BuildWithOptions(..., BuildOptions{Agent: agent})`.
- Normalize agent value as needed before call.
- Extend tests to assert selected value flows into build path and invalid values still fail.

**Outcome:** Process startup selects provider exactly as specified by `--agent`.

**How to Verify:**
Run: `go test ./cmd/... ./internal/app -v`  
Expected: both packages pass; selection path covered by tests.

**Acceptance Criteria:**
- [ ] Unit test(s): callsite coverage in `cmd/main_test.go` and app tests.
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: `--agent=github` path remains usable.
- [ ] Outputs match expectations from How to Verify.
- [ ] Interface and implementation separation - one task for interface definition, one or more for concrete implementation(s) (if applicable)
- [ ] DAL layer tasks do not leak database-specific types; interface-based design for testability and flexibility (if applicable)

### Task 13: Update Operational Documentation and Config Examples

**Depends on:** Task 7, Task 10, Task 12  
**Files:**
- Modify: `README.md`
- Modify: `config.example.json`

**Purpose:** Document codex-first operation and binary-wrapper prerequisites.

**Context to rehydrate:**
- Read `README.md`
- Read `config.example.json`

**Subagent Input (detailed execution brief):**
- Document:
  - `--agent` values and default (`codex`)
  - hard-fail behavior when wrapper missing
  - wrapper build/package command and default binary path
  - env override for wrapper path
  - session idle compatibility behavior
- Keep config example limited to keys actually consumed by code.

**Outcome:** Operators can run Codex provider without reading implementation files.

**How to Verify:**
Run: `rg -n "agent|codex|wrapper|session.idle|YARALPHO_CODEX_WRAPPER_PATH" README.md config.example.json`  
Expected: required operational details are present and accurate.

**Acceptance Criteria:**
- [ ] Unit test(s): N/A
- [ ] Integration test(s): N/A
- [ ] Manual or E2E check: README steps are executable as written.
- [ ] Outputs match expectations from How to Verify.
- [ ] Interface and implementation separation - one task for interface definition, one or more for concrete implementation(s) (if applicable)
- [ ] DAL layer tasks do not leak database-specific types; interface-based design for testability and flexibility (if applicable)

### Task 14: Full Verification Gate and Regression Sweep

**Depends on:** Task 3, Task 7, Task 10, Task 12, Task 13  
**Files:**
- Modify: None
- Test: Existing package tests + wrapper build artifacts

**Purpose:** Prove end-to-end correctness and catch regressions before landing.

**Context to rehydrate:**
- Read `git status --short`
- Read changed verification commands in tasks above

**Subagent Input (detailed execution brief):**
- Run and collect outputs:
  - `go test ./internal/copilot -v`
  - `go test ./internal/app -v`
  - `go test ./internal/consumer -v`
  - `go test ./cmd/... -v`
  - `cd internal/copilot/codex-ts && npm run build && npm run package:linux-x64`
- Execute smoke checks:
  - binary invocation `./bin/codex-wrapper-linux-x64 --help`
  - invalid CLI agent behavior `go run ./cmd --agent=invalid`
- If failures occur, open follow-up issue(s) and do not mark epic complete.

**Outcome:** Integration is verified and release-ready for Linux x64 Codex path.

**How to Verify:**
Run: commands above in sequence  
Expected: all targeted tests pass and wrapper packages successfully.

**Acceptance Criteria:**
- [ ] Unit test(s): all modified package tests pass.
- [ ] Integration test(s): N/A (explicitly recorded in run notes).
- [ ] Manual or E2E check: wrapper binary runs and invalid-agent path fails fast.
- [ ] Outputs match expectations from How to Verify.
- [ ] Interface and implementation separation - one task for interface definition, one or more for concrete implementation(s) (if applicable)
- [ ] DAL layer tasks do not leak database-specific types; interface-based design for testability and flexibility (if applicable)

**Not In Scope:**
- UI/event-view rendering changes.
- Non-Linux or non-x64 packaging.

---

## Parallelization and Conflict Map

- Can run in parallel:
  - Task 3 (Go stub) and Task 4 (TS scaffold)
  - Task 8 (Go path resolution) can begin after Task 3 and Task 7 complete, independent of Task 1/2 CLI work
- Must remain sequential:
  - Task 4 -> Task 5 -> Task 6 -> Task 7 (wrapper metadata -> CLI skeleton -> SDK stream -> package)
  - Task 8 -> Task 9 -> Task 10/11 (transport lifecycle and hardening)
  - Task 1/2 -> Task 12 (CLI wiring finalization)
- File conflict hotspots:
  - `cmd/main.go` shared by Tasks 1 and 12
  - `internal/copilot/codex.go` shared by Tasks 3, 8, 9, 10, 11
  - `internal/copilot/codex_test.go` shared by Tasks 3, 8, 9, 10, 11

---

## Verification Record

### Plan Verification Checklist

| Check | Status | Notes |
| --- | --- | --- |
| Complete | ✓ | Covers requested Tasks 1–4 plus wiring, packaging, lifecycle, and verification gate. |
| Accurate | ✓ | Paths validated against repository structure (`cmd/main.go`, `internal/app/app.go`, `internal/copilot/*`, `docs/plans/*`). |
| Commands valid | ✓ | Commands align with repo tooling (`go test`, npm build/package in new wrapper dir). |
| YAGNI | ✓ | Limited scope to provider integration + transport; no UI or schema changes. |
| Minimal | ✓ | No config-system redesign; keep existing interface and consumer flow. |
| Not over‑engineered | ✓ | Single-process wrapper bridge, simple NDJSON transport, Linux x64 only. |
| Key Decisions documented | ✓ | 5 decisions with rationale included. |
| Supporting docs present | ✓ | Official docs and sample links listed with actionable notes. |
| Context sections present | ✓ | Every task has purpose; scoped tasks include Not In Scope/Gotchas where useful. |
| Budgets respected | ✓ | Tasks scoped to single outcome, typically 1–2 prod files, executable in one run. |
| Outcome & Verify present | ✓ | Each task contains explicit outcome and verification command/expectation. |
| Acceptance Criteria present | ✓ | Checklist present on all tasks with explicit N/A where appropriate. |
| Rehydration context present | ✓ | Each task includes context rehydration commands/files. |

### Rule‑of‑Five Passes

| Pass | Changes Made |
| --- | --- |
| Draft | Created 14 bite-sized tasks with dependencies, file lists, subagent inputs, and verification gates. |
| Correctness | Corrected touched file paths to existing repo layout and separated sequencing for shared-file tasks. |
| Clarity | Added “Subagent Input” blocks and explicit outcomes to make execution context-free. |
| Edge Cases | Added malformed JSON handling, scanner buffer caveat, idempotent stop/double-idle race checks. |
| Excellence | Added parallelization/conflict map and tightened scope boundaries and hard-fail policy wording. |
