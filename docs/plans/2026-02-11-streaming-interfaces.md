# Streaming interfaces (yaralpho-b5i.2) Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Define streaming interfaces under `internal/streaming` so session events can be published and subscribed to with ordering and cancellation guarantees.

**Architecture:** Provide Go interfaces for a session event bus that wrap publish/subscribe semantics over channels, preserve `storage.SessionEvent` ordering by `IngestedAt`, and allow subscribers to start after a cursor while supporting context-driven cleanup.

**Tech Stack:** Go, `context`, channels, `time`, `internal/storage.SessionEvent`.

**Key Decisions:**

- **Payload type:** Reuse `storage.SessionEvent` for bus traffic — avoids duplication and stays aligned with persistence schema.
- **Subscription control:** Subscriptions accept `context.Context` and return an unsubscribe function — ensures cancellation and resource cleanup without leaking goroutines.
- **Cursor handling:** Expose `SubscribeOptions` with an optional `LastIngestedAt` (exclusive) and buffer size hint — lets callers resume from a timestamp while keeping ordering explicit.

---

## Supporting Documentation

- Go contexts for cancellation: https://pkg.go.dev/context
- Go channel concurrency patterns: https://go.dev/doc/effective_go#channels
- Existing session event flow and types: `internal/storage/models.go`, `internal/storage/interfaces.go`, `internal/consumer/task_helpers.go`, `internal/app/run_events_handler.go`.
- Streaming module notes: `internal/streaming/AGENTS.md` (preserve ordering, clean subscriptions).

## Workplan

### Task 1: Rehydrate session event shape and flow

**Depends on:** None  
**Files:**
- Read: `internal/storage/models.go`
- Read: `internal/storage/interfaces.go`
- Read: `internal/consumer/task_helpers.go`
- Read: `internal/app/run_events_handler.go`

**Purpose:** Confirm event payload fields and ordering semantics to design interfaces that align with existing storage and consumer logic.  
**Context to rehydrate:** Review the listed files to recall SessionEvent structure and how events are ingested/persisted.  
**Outcome:** Clear reference for SessionEvent fields and ordering expectations to inform interface signatures.  
**How to Verify:** Notes captured in this plan; no code changes.  
**Acceptance Criteria:**
- [ ] Understanding of `storage.SessionEvent` fields and ordering by `IngestedAt`
- [ ] Awareness of how consumers currently insert and list events
- [ ] No code changes made
- [ ] Plan updated with any nuances found
- [ ] Scope limited to interface design inputs
**Not In Scope:** Implementing or modifying storage or consumer behavior.  
**Gotchas:** None observed yet; look for any non-obvious ordering or filtering behaviors.

### Task 2: Define streaming interfaces for session events

**Depends on:** Task 1  
**Files:**
- Create: `internal/streaming/interfaces.go`

**Purpose:** Introduce interfaces describing a session event bus with publish/subscribe, cursor-aware streaming, and cleanup hooks.  
**Context to rehydrate:** Revisit Task 1 notes and `internal/streaming/AGENTS.md` for ordering/cleanup guidance.  
**Outcome:** `internal/streaming/interfaces.go` contains Go interfaces (`SessionEventBus`, `Subscription`, `SubscribeOptions`) aligned with `storage.SessionEvent`, compiling without errors.  
**How to Verify:**
Run: `go test ./internal/streaming/...`  
Expected: Tests pass (or package builds if no tests).  
**Acceptance Criteria:**
- [ ] Interfaces for publish/subscribe accept `context.Context`
- [ ] Publish uses `storage.SessionEvent` payload
- [ ] Subscribe returns a channel of `storage.SessionEvent` plus unsubscribe/cleanup hook
- [ ] Subscribe options include optional cursor and buffer sizing
- [ ] gofmt applied; package compiles via `go test ./internal/streaming/...`
**Not In Scope:** Concrete implementations or wiring into HTTP/WebSocket handlers.  
**Gotchas:** Ensure comments describe ordering and cleanup expectations to guide implementers.

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes                                                                                  |
| --------------------------- | ------ | -------------------------------------------------------------------------------------- |
| Complete                    | ✓      | Addresses defining streaming interfaces with ordering/cursor considerations            |
| Accurate                    | ✓      | Paths verified (`internal/streaming`, `internal/storage`, consumer/app handlers exist) |
| Commands valid              | ✓      | `go test ./internal/streaming/...` builds the new package                              |
| YAGNI                       | ✓      | Plan limited to interfaces only                                                        |
| Minimal                     | ✓      | Two tasks cover discovery and interface definition                                     |
| Not over-engineered         | ✓      | No implementation or extra features beyond interfaces                                  |
| Key Decisions documented    | ✓      | Three decisions recorded                                                               |
| Supporting docs present     | ✓      | Links and code references listed                                                       |
| Context sections present    | ✓      | Purpose, Context, Not In Scope included                                                |
| Budgets respected           | ✓      | Tasks under file/time/step limits                                                      |
| Outcome & Verify present    | ✓      | Each task lists Outcome and How to Verify                                              |
| Acceptance Criteria present | ✓      | Checklists included per task                                                           |
| Rehydration context present | ✓      | Context noted for tasks needing prior knowledge                                        |

### Rule-of-Five Passes

| Pass        | Changes Made                                        |
| ----------- | --------------------------------------------------- |
| Draft       | Initial structure, tasks, and decisions captured    |
| Correctness | Verified paths/commands; clarified options wording  |
| Clarity     | Simplified outcomes and verification language       |
| Edge Cases  | Added cursor/buffer options and cleanup expectations |
| Excellence  | Polished supporting docs and acceptance checklists  |
