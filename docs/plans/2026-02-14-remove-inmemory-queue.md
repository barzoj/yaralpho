# Remove in-memory queue usage Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Ensure the service no longer wires or references the legacy in-memory queue; scheduler-driven worker path is the only execution path.

**Architecture:** Single web process with scheduler selecting work from Mongo-backed batches and invoking the consumer Worker directly. No producer/consumer queue objects exist; routing and wiring bypass any queue setup. Documentation reflects scheduler-centric flow.

**Tech Stack:** Go 1.25, gorilla/mux HTTP routing, zap logging, Mongo storage, scheduler + consumer worker.

**Key Decisions:**

- **Execution path:** Scheduler-to-worker direct invocation — removes in-memory queue complexity and aligns with sequential batch processing.
- **Docs source of truth:** AGENTS.md and design docs updated to reflect scheduler flow — reduces confusion for future contributors.
- **Verification strategy:** Static search plus `go test ./...` — ensures no residual queue dependencies and build stays green.

---

## Supporting Documentation

- Gorilla mux router: https://github.com/gorilla/mux — route declarations and handler wiring.
- Go testing package: https://pkg.go.dev/testing — writing and running Go tests.
- Go sync.WaitGroup: https://pkg.go.dev/sync#WaitGroup — relevant for scheduler/worker coordination.
- Zap logging: https://pkg.go.dev/go.uber.org/zap — structured logging used across the service.

---

### Task 1: Update docs to reflect scheduler-only execution

**Depends on:** None  
**Files:**
- Modify: `AGENTS.md`
- Modify: `docs/design/scheduler.md`

**Purpose:** Remove outdated queue references in contributor docs so newcomers follow the scheduler-driven model.

**Context to rehydrate:** Review current scheduler design summary in `docs/design/scheduler.md`.

**Outcome:** Top-level docs describe scheduler-driven execution with no in-memory queue; legacy queue references removed or clarified as historical.

**How to Verify:**  
Run: `rg "queue" AGENTS.md docs/design/scheduler.md`  
Expected: Only historical or clarified references remain; primary guidance points to scheduler.

**Acceptance Criteria:**
- [ ] AGENTS.md no longer lists queue as wired dependency.
- [ ] docs/design/scheduler.md emphasizes scheduler path and absence of queue.
- [ ] Language clearly states scheduler/worker path is canonical.
- [ ] No new queue terminology introduced elsewhere in these files.

**Not In Scope:** Changing historical plan documents beyond brief clarifications in current docs.

**Gotchas:** Preserve existing contributor guidance sections; avoid removing unrelated notes.

**Step 1: Write the failing test**

```bash
rg "queue" AGENTS.md docs/design/scheduler.md
```

**Step 2: Run test to verify it fails**

Run: `rg "queue" AGENTS.md docs/design/scheduler.md`  
Expected: Shows outdated queue mentions.

**Step 3: Write minimal implementation**

Update docs to replace/remove queue references with scheduler wording.

**Step 4: Run test to verify it passes**

Run: `rg "queue" AGENTS.md docs/design/scheduler.md`  
Expected: Only clarified/historical mentions remain.

**Step 5: Commit**

```bash
git add AGENTS.md docs/design/scheduler.md
git commit -m "yaralpho-hgo.4: update docs for scheduler-only execution"
```

---

### Task 2: Purge residual queue wiring in code if any

**Depends on:** Task 1  
**Files:**
- Inspect: `internal/app/app.go`
- Inspect: `internal/consumer/*` (no edits expected; split follow-up if more than two files require changes)
- Test: `go test ./...`

**Purpose:** Ensure application wiring and consumer code contain no queue dependencies; scheduler/worker path remains sole execution flow.

**Context to rehydrate:** Inspect `internal/app/app.go` wiring and `internal/consumer` worker entrypoint.

**Outcome:** Codebase free of queue imports or wiring; consumer invoked solely by scheduler.

**How to Verify:**  
Run: `rg "queue" internal`  
Run: `go test ./...`  
Expected: No queue references outside harmless variable names; tests pass without queue packages.

**Acceptance Criteria:**
- [ ] No queue package imports or wiring in internal code.
- [ ] Consumer entrypoint paths (worker, scheduler) compile without queue constructs.
- [ ] `go test ./...` succeeds.
- [ ] Only intentional non-queue usages (e.g., local variable names) remain.

**Not In Scope:** Introducing new scheduler features or altering worker semantics.

**Gotchas:** Avoid renaming test helpers unless they materially imply queue wiring; keep diffs minimal.

**Step 1: Write the failing test**

```bash
rg "queue" internal
```

**Step 2: Run test to verify it fails**

Run: `rg "queue" internal`  
Expected: Any queue wiring hits identified.

**Step 3: Write minimal implementation**

If any queue wiring is found, fix it within at most two files or split a follow-up task.

**Step 4: Run test to verify it passes**

Run: `rg "queue" internal`  
Expected: No queue wiring hits remain.  
Run: `go test ./...`  
Expected: All tests pass.

**Step 5: Commit**

```bash
git add internal
git commit -m "yaralpho-hgo.4: remove leftover queue wiring"
```

---

### Task 3: Final verification and cleanup

**Depends on:** Task 2  
**Files:**
- Modify: none (verification only)
- Test: `go test ./...`

**Purpose:** Confirm build health and absence of queue references after cleanup.

**Context to rehydrate:** Results from Tasks 1–2.

**Outcome:** Verified clean codebase with scheduler-only path; no residual queue references.

**How to Verify:**  
Run: `rg "queue" internal`  
Run: `go test ./...`  
Expected: No queue wiring hits; tests green.

**Acceptance Criteria:**
- [ ] Searches confirm no queue wiring remains.
- [ ] Full test suite passes.
- [ ] Working tree clean post-verification.

**Not In Scope:** Additional feature changes.

**Gotchas:** Ensure working tree is clean before final verification to avoid false positives.

**Step 1: Write the failing test**

```bash
rg "queue" internal
```

**Step 2: Run test to verify it fails**

Run: `rg "queue" internal`  
Expected: No unintended hits (if any, address before proceeding).

**Step 3: Write minimal implementation**

Address any remaining findings if present.

**Step 4: Run test to verify it passes**

Run: `rg "queue" internal`  
Run: `go test ./...`  
Expected: Clean search; tests pass.

**Step 5: Commit**

```bash
git add .
git commit -m "yaralpho-hgo.4: verify scheduler-only path"
```

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes                                     |
| --------------------------- | ------ | ----------------------------------------- |
| Complete                    | ✓      | Covers doc update, code scan, verification |
| Accurate                    | ✓      | Paths validated (AGENTS.md, scheduler doc, internal/*) |
| Commands valid              | ✓      | rg/go test commands runnable              |
| YAGNI                       | ✓      | Only docs, scan, verify steps included    |
| Minimal                     | ✓      | Tasks scoped to ≤2 files per skill budget |
| Not over-engineered         | ✓      | No new components added                   |
| Key Decisions documented    | ✓      | Three decisions captured                  |
| Supporting docs present     | ✓      | Links included                            |
| Context sections present    | ✓      | Tasks include Purpose/Context             |
| Budgets respected           | ✓      | File/step limits noted                    |
| Outcome & Verify present    | ✓      | Each task lists Outcome/How to Verify     |
| Acceptance Criteria present | ✓      | Checklists included per task              |
| Rehydration context present | ✓      | Context sections provided where needed    |

### Rule‑of‑Five Passes

| Pass        | Changes Made                             |
| ----------- | ---------------------------------------- |
| Draft       | Outline and tasks defined                |
| Correctness | Paths/commands verified; budgets checked |
| Clarity     | Clarified Task 2 scope/edits expectation |
| Edge Cases  | Confirmed no new features; budget noted  |
| Excellence  | Polished wording and consistency         |
