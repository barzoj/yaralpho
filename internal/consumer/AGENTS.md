# Purpose
Worker that executes scheduler-selected work items, classifies epics vs tasks, orchestrates copilot sessions, and records outcomes.

# Exposed Interfaces
- Entry point to process scheduler-issued `WorkItem` values given dependencies: `Tracker`, `CopilotClient`, `Storage`, and `Notifier`.
- Uses zap logger injected by the app; no new globals.

# Notes for Agents
- Work items are `{batch_id, task_ref}` structs; scheduler invokes `Process(ctx, WorkItem)`.
- Workflow: scheduler selects item → determine epic via tracker → choose prompt (task: "Work on task <ref>"; epic: "Pick first ready task from epic <epic> and execute") → create task run record → start copilot session → stream raw events to storage → update statuses → emit Slack/notify events on completion or error.
- Batch status is advanced to running when processing begins, idle on success/stop, failed on errors.
- Respect context cancellation for graceful shutdown; ensure resources from copilot sessions are closed.***
