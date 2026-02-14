# Mongo Persistence Updates for Repository-Aware Scheduler

## Direction and Options (validated solo)
We need Mongo persistence that matches the new repository/agent/batch/run shapes introduced in the refactor plan. Two viable approaches emerged: (A) evolve the existing collections in place, keeping document shapes stable and expanding indexes; (B) create new collections (e.g., `repos`, `agents_v2`, `batches_v2`) and migrate data. Option A wins because the schema changes are additive (new fields, stricter defaults) and existing test data is minimal; migration overhead would slow delivery without reducing risk. We will keep the current collection names but harden uniqueness and add query indexes that the scheduler and new handlers will rely on (repository_id filters, status filters, idle agents lookup). Sequential item order is preserved by storing items as an ordered array and never mutating with positional operators that reorder. Conflicts surface as `storage.ErrConflict` from duplicate key errors to align with handler semantics.

## Persistence Design
- **Repositories**: Document fields `repository_id`, `name`, `path`, timestamps. Indexes: unique on id, name, path. CRUD setters stamp `UpdatedAt`, default timestamps when absent. Delete returns `mongo.ErrNoDocuments` when missing; callers will guard via `RepositoryHasActiveBatches`.
- **Agents**: Fields `agent_id`, `name`, `runtime`, `status`, timestamps. Default `status=idle` on create. Indexes: unique id and name; non-unique runtime for filtering; consider status+runtime compound in follow-up if scheduler queries need it.
- **Batches**: Fields `batch_id`, `repository_id`, timestamps, `items` array of `{input,status,attempts}`, `status`, optional summary/session_name. Defaults: batch status pending, item status pending, attempts min 0. Indexes: unique id; repository_id; compound repository_id+status for listing by repo/status; created_at desc for recency. We keep full batch document replacement via `$set` to retain item order.
- **TaskRuns**: Fields `run_id`, `batch_id`, `repository_id`, `task_ref`, optional `parent_ref`, `session_id`, timestamps, status, result. On create, if repository_id is empty but batch_id is set, we resolve the batch to backfill repository_id before insert. Updates also backfill repository_id if missing. Indexes: unique id; batch_id; batch_id+status; repository_id; repository_id+started_at for repo-scoped listing.
- **SessionEvents / Progress**: unchanged except they now benefit from run.repository_id presence when aggregating.

## Validation and Tests
- Apply defaults and conflict handling inside storage layer; upstream handlers add path/runtime validation.
- Integration tests (`internal/storage/mongo/client_test.go`) will cover round-trip for repository/agent CRUD, batch with item statuses/attempts, run repository_id propagation, and new indexes via duplicate insert checks. Tests skip when Mongo env vars are absent.

