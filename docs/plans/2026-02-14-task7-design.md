## Task 7: Worker execution and retry handling

### Goals
- Ensure scheduler finalizes batch/item/agent state after worker execution, including retries and terminal failure.
- Record task runs with repository context so downstream queries remain scoped.
- Keep sequential per-batch execution and idle-agent gating intact.

### Approaches considered
1) **Synchronous result via error-only contract (recommended):** Keep `Worker.Process` returning an error to indicate failure, and let the scheduler interpret nil as success. Minimal surface change and aligns with current worker signature; relies on scheduler to handle all state transitions and retries.  
2) **Structured result type:** Have `Worker.Process` return a typed result (status, metadata), giving richer context (e.g., fatal vs retryable). More future-proof but cascades interface changes across app wiring not yet landed.  
3) **Async queue with callbacks:** Dispatch work and let callbacks update state. Adds complexity and shared-state hazards without current infra need.

Recommendation: option 1 for incremental fit while leaving room to extend the worker interface later.

### Design
- Add `maxRetries` configuration on scheduler (default 5). Before dispatch, claim batch item and agent busy as today. After `Process` returns, always set agent back to idle (warn on failure to persist).
- Track attempts on the batch item: increment after each execution attempt (success or failure) so retries are counted.  
  - **Success:** mark item `done`; if all items done -> batch `done`, else batch `pending` for next tick.  
  - **Failure:** increment attempts, set item `pending` when attempts < maxRetries (batch `pending`), else set item `failed` and batch `failed` (stop further work).
- Persist batch updates even when worker fails; propagate worker error from Tick for visibility.
- Add helper to detect completion across items.
- Worker/task run plumbing: include `batch.RepositoryID` on all created `TaskRun` records (running and failure pre-session) to keep runs repository-aware.

### Testing
- New scheduler tests:  
  - **TestRetrySuccess:** single attempt success leaves agent idle, item done, batch pending/done accordingly, attempts incremented.  
  - **TestRetryThenSuccess:** first failure increments attempts and leaves item pending/batch pending; second success completes item; agent idle after each.  
  - **TestRetryExhausted:** failures until attempts==maxRetries mark item failed and batch failed; agent idle.  
  - Adjust existing happy-path test expectations for idle agent and attempt increments.  
Run `go test ./internal/scheduler -run TestRetry` to verify.***
