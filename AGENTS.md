# Agent Instructions

This project uses **bd** (beads) for issue tracking. Run `bd onboard` to get started.

## Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --status in_progress  # Claim work
bd close <id>         # Complete work
bd sync               # Sync with git
```

## Landing the Plane (Session Completion)

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
   bd sync
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** - Clear stashes, prune remote branches
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds

## Issue Tracking

This project uses **bd (beads)** for issue tracking.
Run `bd prime` for workflow context, or install hooks (`bd hooks install`) for auto-injection.

**Quick reference:**
- `bd ready` - Find unblocked work
- `bd create "Title" --type task --priority 2` - Create issue
- `bd close <id>` - Complete work
- `bd sync` - Sync with git (run at session end)

For full workflow details: `bd prime`

For GitHub Copilot users:
Add the same content to .github/copilot-instructions.md

## Internal Package Map

- `internal/app` – Composition root; wires config, logger, storage, scheduler, tracker, notifier, copilot, consumer, and HTTP handlers.
- `internal/config` – Env-first configuration loader (YARALPHO_*), optional JSON fallback, panic on missing required keys.
- `internal/storage` – Domain models and storage interfaces for batches, task runs, and session events; Mongo implementation lives in `internal/storage/mongo`.
- `internal/consumer` – Worker invoked by the scheduler; classifies epics vs tasks via tracker, runs copilot sessions, persists events/status, and triggers notifications.
- `internal/copilot` – Interface for starting copilot sessions and streaming raw events; GitHub Copilot SDK implementation auto-approves permissions.
- `internal/tracker` – Tracker contract to detect epics and list child tasks; beads CLI-backed implementation.
- `internal/notify` – Notifier interface for task/batch lifecycle events; Slack webhook implementation with noop fallback.
