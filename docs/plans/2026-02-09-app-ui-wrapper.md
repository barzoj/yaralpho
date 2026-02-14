# App UI Wrapper Implementation Plan

After _human approval_, use plan2beads to convert this plan to a beads epic, then use `superpowers:subagent-driven-development` for parallel execution.

**Goal:** Serve a minimal `/app` web UI that consumes existing APIs to list batches, list runs by batch, and display raw events for a run with basic navigation and loading/error states.

**Architecture:** Static HTML/JS served from the Go app via embedded assets. Client-side fetches hit existing JSON endpoints (`/batches`, `/runs`, `/runs/{id}`, new events endpoint) with simple DOM rendering—no build pipeline or SSR. Router exposes `/app` (HTML) plus `/app/static` for assets.

**Tech Stack:** Go (net/http, gorilla/mux, `embed`), vanilla HTML/CSS/JS (ES2015+), existing JSON APIs.

**Key Decisions:**

- **Embed static assets via `//go:embed`:** Keeps deploy simple and avoids extra build steps; assets ship with binary.
- **Separate events endpoint:** Add `/runs/{id}/events` to return full event streams so UI can request all events without server truncation.
- **Vanilla JS over framework:** Small surface, no stateful routing needed; minimizes dependencies and build tooling.
- **Table-first UX:** Use simple tables and links between views to stay readable and fast to implement; styling minimal but structured.
- **Query-string driven navigation:** `/app`, `/app?batch=ID`, `/app?run=ID` to mirror requirements and keep stateless navigation.

---

## Supporting Documentation

- Go embed package: https://pkg.go.dev/embed — reference for embedding static assets.
- Gorilla mux PathPrefix/FileServer pattern: https://pkg.go.dev/github.com/gorilla/mux#Router.PathPrefix — serving static files.
- Fetch API reference: https://developer.mozilla.org/en-US/docs/Web/API/Fetch_API/Using_Fetch — client-side data loading.
- Existing API handlers: internal/app/routes.go, listBatchesHandler, listRunsHandler, runDetailHandler for shapes and limits.
- Data models: internal/storage/models.go for Batch, TaskRun, SessionEvent fields to display.

## Tasks

### Task 1: Add events endpoint to return full session events

**Depends on:** None

**Files:**

- Modify: `internal/app/routes.go`
- Add: `internal/app/run_events_handler.go`
- Modify: `internal/app/handlers_test.go`

**Purpose:** Allow clients (UI) to fetch all session events for a run without truncation limits.

**Outcome:** GET `/runs/{id}/events` returns the full ordered event list (optionally capped by query limit) with proper errors for missing runs.

**How to Verify:**

- Run: `go test ./internal/app -run TestRunEventsHandler`
- Expected: New test passes; handler returns 200 with events when run exists, 404 when run is missing.

**Acceptance Criteria:**

- [ ] Endpoint registered at `/runs/{id}/events`
- [ ] Uses storage.ListSessionEvents to fetch events
- [ ] Honors optional `limit` query with sane max, default to all
- [ ] Returns 404 for unknown run, 400 for empty id
- [ ] Tests cover success and not-found cases

**Not In Scope:** UI consumption; auth.

**Gotchas:** Keep limit parsing consistent with existing helpers.

---

### Task 2: Embed static UI assets and wire router

**Depends on:** Task 1 (needs routes context to avoid conflicts)

**Files:**

- Add: `internal/app/ui/static.go`
- Modify: `internal/app/routes.go`

**Purpose:** Serve `/app` HTML and `/app/static/*` assets from embedded filesystem.

**Outcome:** Visiting `/app` returns HTML page; static assets load from embedded FS via `/app/static/...` with correct content-type.

**How to Verify:**

- Run: `go test ./internal/app -run TestAppUIRoutes`
- Manual: `go run ./cmd` then curl `http://localhost:8080/app` returns HTML including “Ralph UI”.

**Acceptance Criteria:**

- [ ] `//go:embed` captures HTML/JS/CSS files
- [ ] Router serves index at `/app` and static assets at `/app/static/`
- [ ] Content-Type headers set correctly
- [ ] Tests validate handlers respond 200 and serve embedded content

**Not In Scope:** UI rendering logic, styling polish.

**Gotchas:** Ensure PathPrefix doesn’t shadow API routes; strip prefix correctly.

---

### Task 3: Implement frontend logic for batches/runs/events views

**Depends on:** Task 2

**Files:**

- Add: `internal/app/ui/index.html`
- Add: `internal/app/ui/app.js`

**Purpose:** Provide client-side navigation and rendering for batches list, runs list, and run events using existing APIs.

**Outcome:** `/app` loads a table of batches with links to batch view; batch view lists runs; run view shows raw event JSON blocks. Loading/error states included.

**How to Verify:**

- Manual: Start server, open `/app`; navigate between views; ensure API calls succeed and content updates.
- Optional: `npm` not required; plain browser test.

**Acceptance Criteria:**

- [ ] Query params `batch` and `run` drive view selection
- [ ] Fetches `/batches` with sane limit, renders table with session names and status
- [ ] Fetches `/runs?batch_id=` when batch view selected
- [ ] Fetches `/runs/{id}/events` when run view selected and renders JSON per event
- [ ] Shows loading and error messages per view
- [ ] No build tooling required (vanilla JS); styling inline or minimal in HTML

**Not In Scope:** Advanced rendering, pagination controls, filters, auth.

**Gotchas:** Handle missing session_name gracefully; keep fetch limits aligned with server defaults.

---

### Task 4: Wire minimal tests and linting guardrails

**Depends on:** Tasks 1-3

**Files:**

- Modify: `internal/app/handlers_test.go`
- Modify: `internal/app/ui/static.go` (test hooks)

**Purpose:** Ensure new handlers and embedded assets are covered by basic tests to prevent regressions.

**Outcome:** Handler tests exercise `/app` HTML serving and events endpoint; go test passes.

**How to Verify:**

- Run: `go test ./internal/app -run TestRunEventsHandler -run TestAppUIRoutes`
- Expected: Tests green.

**Acceptance Criteria:**

- [ ] Tests cover happy path for `/runs/{id}/events`
- [ ] Tests cover `/app` serving HTML from embed FS
- [ ] Tests run in CI without external deps

**Not In Scope:** Frontend integration/e2e tests.

**Gotchas:** Use httptest server; avoid brittle HTML assertions (check marker strings).

---

### Task 5: Doc update and sanity walkthrough

**Depends on:** Tasks 1-4

**Files:**

- Modify: `README.md`
- Modify: `docs/plans/2026-02-09-app-ui-wrapper.md` (verification record)

**Purpose:** Document the `/app` UI usage and record verification steps.

**Outcome:** README includes brief `/app` section; plan file contains verification record updates after checks.

**How to Verify:**

- Manual: Review README section; ensure commands accurate.

**Acceptance Criteria:**

- [ ] README mentions `/app` endpoint and basic navigation
- [ ] Plan verification record filled after checklist and rule-of-five passes

**Not In Scope:** Detailed UI guide.

**Gotchas:** Keep instructions concise; mention no build step required.

---

## Verification Record

### Plan Verification Checklist

| Check                       | Status | Notes                                                                          |
| --------------------------- | ------ | ------------------------------------------------------------------------------ |
| Complete                    | ✓      | Covers batches list, runs list, run events UI, new events endpoint, docs/tests |
| Accurate                    | ✓      | File paths mapped to current layout; uses existing handlers and storage APIs   |
| Commands valid              | ✓      | go test targets existing package; manual curl/go run steps feasible            |
| YAGNI                       | ✓      | No build tooling, minimal styling, no auth/workflows beyond request            |
| Minimal                     | ✓      | Small number of tasks with inline styling; no extra endpoints beyond events    |
| Not over-engineered         | ✓      | Vanilla JS, embedded assets, reuse existing APIs                               |
| Key Decisions documented    | ✓      | Five decisions captured in header                                              |
| Supporting docs present     | ✓      | Links to embed, mux, fetch, handlers, models                                   |
| Context sections present    | ✓      | Each task has Purpose and scope notes                                          |
| Budgets respected           | ✓      | Tasks touch ≤2 prod files each and have single outcomes                        |
| Outcome & Verify present    | ✓      | Every task lists outcome plus how to verify                                    |
| Acceptance Criteria present | ✓      | Checklists included per task                                                   |
| Rehydration context present | ✓      | Not needed beyond file references; tasks independent                           |

### Rule-of-Five Passes

| Pass        | Changes Made                                                                                         |
| ----------- | ---------------------------------------------------------------------------------------------------- |
| Draft       | Confirmed task ordering, budgets, and minimal file set; no structural changes needed                 |
| Correctness | Validated file lists, commands, and endpoint paths; no corrections needed                            |
| Clarity     | Reviewed language for brevity; kept tasks concise, no content changes                                |
| Edge Cases  | Confirmed empty/error states captured in UI task; events endpoint handles missing/limit cases        |
| Excellence  | Added inline styling constraint and emphasized minimal manual verification; no further polish needed |

### Execution Verification (2026-02-09)

- Updated README with `/app` usage and `/runs/{id}/events` API details.
- Plan verification record refreshed post-docs; no additional code changes required.
- Tests not run (doc-only changes).
