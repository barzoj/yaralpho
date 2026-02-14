# Event Viewer Chat UX Implementation Plan

After human approval, use plan2beads to convert this plan to a beads epic, then use superpowers:subagent-driven-development for parallel execution.

**Goal:** Redesign the run events viewer to render session events as a chat-like conversation with emojis per event type, hiding raw JSON behind an expandable toggle, and ignoring streaming delta events.

**Architecture:** Extend the existing vanilla JS UI in internal/app/ui to transform API events into a chat timeline. Map event types to friendly labels, icons, and concise summaries derived from the event payload. Provide an expandable details panel for raw JSON. Avoid backend changes; rely solely on current /runs/:id/events responses.

**Tech Stack:** Vanilla JS/DOM, HTML/CSS in internal/app/ui, current API endpoints (`/runs/:id/events`).

**Key Decisions:**
- **Presentation:** Chat bubbles with emoji per event type — improves scanability vs. plain JSON dump.
- **Delta filtering:** Drop assistant.message_delta and assistant.reasoning_delta in UI — reduces noise while preserving final messages.
- **Details toggle:** Collapsible JSON per event — keeps view compact while retaining full fidelity for debugging.
- **Time ordering:** Preserve API order and show ingested_at timestamps — matches server chronology without re-sorting.

---

## Supporting Documentation
- Event types reference: see generated enum in go module [github.com/github/copilot-sdk/go@v0.1.23/generated_session_events.go](../../go/pkg/mod/github.com/github/copilot-sdk/go@v0.1.23/generated_session_events.go).
- Current UI code: [internal/app/ui/app.js](../internal/app/ui/app.js) and [internal/app/ui/index.html](../internal/app/ui/index.html).
- Sample payloads: [run-1770648229581217369.json](../../run-1770648229581217369.json) — use jq to inspect, e.g., `jq '.events[] | select(.event.type=="assistant.intent") | .event.data' run-1770648229581217369.json`.
- API shape: `/runs/:id/events?limit=` returns `events` array with `event` and `ingested_at`; no change required.

---

## Plan Verification Checklist (to run before approval)
- Complete event coverage (all non-delta types present in enum and sample log), no speculative types.
- Friendly labels + emoji defined for each event type, including default/fallback.
- Raw JSON hidden behind toggle but accessible.
- Noise filtered: assistant.message_delta and assistant.reasoning_delta skipped client-side.
- Loading/empty/error states preserved from current UI.
- Tasks respect file-touch budget (<=2 prod files each) and verification commands are runnable.

---

## Tasks

### Task 1: Define event map and filtering
**Depends on:** None
**Files:**
- Modify: internal/app/ui/app.js

**Purpose:** Centralize event type metadata (emoji, label, summary extractor, visibility) and filter out streaming delta events.
**Context to rehydrate:** Review current renderEventsList in app.js and enum in generated_session_events.go.
**Outcome:** A JS map of event types to display info; UI skips assistant.message_delta and assistant.reasoning_delta.
**How to Verify:** Run `node -e "console.log(require('./internal/app/ui/app.js') ? 'load ok' : 'fail')"` (or load /app run view) and confirm delta events no longer appear.
**Acceptance Criteria:**
- [ ] Map includes all event types seen in sample log and enum (except deltas ignored).
- [ ] Unknown types fall back to generic emoji/label.
- [ ] Filtering is performed before rendering.
- [ ] No regression to batch/run list views.

### Task 2: Chat layout scaffolding
**Depends on:** Task 1
**Files:**
- Modify: internal/app/ui/index.html
- Modify: internal/app/ui/static.go (if bundling CSS) or existing stylesheet block

**Purpose:** Add structural containers and CSS for chat bubbles, metadata rows, and expand/collapse affordances.
**Context to rehydrate:** Inspect current DOM structure in index.html; note classes used in app.js ("events", "event").
**Outcome:** Chat-ready DOM/CSS classes available for JS to populate; maintains responsiveness.
**How to Verify:** Open /app run page in browser; ensure layout shows bubble styling even with placeholder content.
**Acceptance Criteria:**
- [ ] Chat items align vertically with clear timestamp and emoji.
- [ ] Buttons/links for "Show details" exist and are keyboard-focusable.
- [ ] No layout breakage on existing tables or meta grid.
- [ ] Works on narrow viewport (basic responsiveness).

### Task 3: Render user.message and system/pending messages
**Depends on:** Task 2
**Files:**
- Modify: internal/app/ui/app.js

**Purpose:** Render `user.message`, `system.message` (if present), and `pending_messages.modified` as chat bubbles with appropriate emoji and readable text.
**Context to rehydrate:** Use sample user message content from run log; note transformedContent field.
**Outcome:** User/system events show friendly label, emoji, text (prefer `content`, else `transformedContent`).
**How to Verify:** Load run view; first user.message displays as chat bubble with timestamp and text, JSON hidden behind toggle.
**Acceptance Criteria:**
- [ ] Emoji + label displayed; content uses plaintext when available.
- [ ] Toggle reveals raw JSON block.
- [ ] Handles missing content gracefully (shows placeholder like “(no content)”).

### Task 4: Render assistant.intent
**Depends on:** Task 2
**Files:**
- Modify: internal/app/ui/app.js

**Purpose:** Show the model’s inferred intent with emoji and short text from `data.intent`.
**Context to rehydrate:** See assistant.intent sample in run log.
**Outcome:** Assistant intent appears as a concise note in timeline.
**How to Verify:** Load run; intent event shows label and intent string, JSON collapsible.
**Acceptance Criteria:**
- [ ] Uses `data.intent` when present; placeholder if absent.
- [ ] Timestamp shown; toggle works.

### Task 5: Render assistant.reasoning (non-delta)
**Depends on:** Task 2
**Files:**
- Modify: internal/app/ui/app.js

**Purpose:** Display reasoning text as a chat bubble (markdown/plain) and hide deltas.
**Context to rehydrate:** Use assistant.reasoning sample from log.
**Outcome:** Only complete reasoning events render; deltas skipped by Task 1 filter.
**How to Verify:** Run view; reasoning bubble appears with content preview and toggle.
**Acceptance Criteria:**
- [ ] Renders `data.content` (markdown allowed but displayed as preformatted/plain to avoid XSS).
- [ ] Respects filter for reasoning_delta.

### Task 6: Render assistant.message (final responses)
**Depends on:** Task 2
**Files:**
- Modify: internal/app/ui/app.js

**Purpose:** Render assistant final messages with emoji and decrypted/plain content; ignore message_delta.
**Context to rehydrate:** Sample assistant.message with plaintext content; others may have encryptedContent only.
**Outcome:** Bubble shows `content` if present, else indicates encrypted/hidden.
**How to Verify:** Load run; assistant messages visible; no streaming deltas shown.
**Acceptance Criteria:**
- [ ] Uses `content` when available; fallback text when only `encryptedContent` present.
- [ ] Toggle exposes raw JSON.

### Task 7: Render assistant.turn_start / assistant.turn_end
**Depends on:** Task 2
**Files:**
- Modify: internal/app/ui/app.js

**Purpose:** Show turn boundaries with concise status (start/end) and turnId.
**Context to rehydrate:** Samples include `turnId` field.
**Outcome:** Timeline shows markers for turns.
**How to Verify:** Load run; turn markers appear with emoji and turnId.
**Acceptance Criteria:**
- [ ] Labels “Turn start”/“Turn end” with turnId.
- [ ] Minimal styling; toggle shows full event JSON.

### Task 8: Render assistant.usage & session.usage_info
**Depends on:** Task 2
**Files:**
- Modify: internal/app/ui/app.js

**Purpose:** Summarize token/cost info in readable chips (input/output tokens, cache reads, cost) using usage events.
**Context to rehydrate:** Use samples with cost, inputTokens, outputTokens, currentTokens, tokenLimit.
**Outcome:** Usage events render as compact metric rows with emoji.
**How to Verify:** Load run; usage events display metrics; JSON toggle works.
**Acceptance Criteria:**
- [ ] Shows key metrics (input/output tokens, cache read/write, cost/duration if present).
- [ ] Handles missing fields gracefully.

### Task 9: Render tool.execution_start / tool.execution_complete
**Depends on:** Task 2
**Files:**
- Modify: internal/app/ui/app.js

**Purpose:** Show tool calls with name, callId, arguments, and result summary.
**Context to rehydrate:** Samples with `toolName`, `toolCallId`, `arguments`, `result.content`.
**Outcome:** Start events show tool + args; complete events show result summary and success/failure status.
**How to Verify:** Load run; tool invocations visible with args and results; JSON toggle works.
**Acceptance Criteria:**
- [ ] Arguments summarized (stringify small objects, otherwise “see details”).
- [ ] Result content rendered when present; error surfaced if error exists.

### Task 10: Render skill.invoked
**Depends on:** Task 2
**Files:**
- Modify: internal/app/ui/app.js

**Purpose:** Show invoked skill name/description as a chat bubble.
**Context to rehydrate:** Sample contains skill doc in `data.content`.
**Outcome:** Skill invocation appears with label and first lines of content; toggle shows full JSON.
**How to Verify:** Load run; skill bubble present.
**Acceptance Criteria:**
- [ ] Displays skill name if parsable; otherwise generic label.
- [ ] Truncates long content with “Show details” toggle.

### Task 11: Render session.idle
**Depends on:** Task 2
**Files:**
- Modify: internal/app/ui/app.js

**Purpose:** Mark idle heartbeat events with simple emoji/label.
**Context to rehydrate:** Sample idle event with minimal data.
**Outcome:** Idle markers appear unobtrusively.
**How to Verify:** Load run; idle event shows as small status line.
**Acceptance Criteria:**
- [ ] Timestamp shown; minimal text.

### Task 12: Fallback/unknown event handling
**Depends on:** Task 1
**Files:**
- Modify: internal/app/ui/app.js

**Purpose:** Ensure any unrecognized event types render with generic emoji/label and accessible JSON toggle.
**Context to rehydrate:** Use enum as source; anticipate future types.
**Outcome:** No hard failures when new types appear.
**How to Verify:** Simulate with injected fake event in local data; verify UI renders generic entry.
**Acceptance Criteria:**
- [ ] Safe rendering path for unknown types.

### Task 13: QA pass and regression checks
**Depends on:** Tasks 1-12
**Files:**
- Modify: internal/app/ui/app.js (test harness tweaks if needed)

**Purpose:** Verify UX end-to-end, including loading/empty/error states and truncation notice handling.
**Context to rehydrate:** Existing renderBatches/renderRuns flows; events_truncated note.
**Outcome:** Confidence that new chat UI does not break existing flows.
**How to Verify:**
- Load /app with no batch/run → empty states unchanged.
- Load run-1770648229581217369.json via backend (or mock) → chat renders; delta events absent.
- Check events_truncated note still shows when flag true.
**Acceptance Criteria:**
- [ ] No JS errors in console.
- [ ] Empty/error states preserved.
- [ ] Truncation note displayed when events_truncated true.

---

## Notes on Event Payloads (from run-1770648229581217369.json)
- user.message: `data.content` plain text; `transformedContent` available.
- assistant.intent: `data.intent` string; ephemeral true.
- assistant.reasoning: `data.content` markdown/plain; reasoningId present; ephemeral true.
- assistant.message: `data.content` sometimes plain; `encryptedContent` often present — show hint when only encrypted.
- assistant.turn_start / assistant.turn_end: `data.turnId` present.
- assistant.usage: tokens/cost/duration/providerCallId; initiator field.
- session.usage_info: currentTokens, tokenLimit, messagesLength.
- tool.execution_start: toolName/toolCallId/arguments.
- tool.execution_complete: result.content/detailedContent or error; shares toolCallId.
- skill.invoked: `data.content` contains skill doc markdown.
- session.idle: no extra fields.
- pending_messages.modified: minimal data; can suppress or mark as pending state.
- assistant.message_delta & assistant.reasoning_delta: streaming noise — ignore in UI.

---

## Out of Scope
- Backend/API changes, pagination, or event storage adjustments.
- Cryptographic handling of encryptedContent; we only display placeholder.
- Live streaming; UI remains static after load.

---

## Next Step
Request human approval before converting this plan to beads tasks.
