# Purpose
Worker loop that consumes queue items, classifies epics vs tasks, orchestrates copilot sessions, and records outcomes.

# Exposed Interfaces
- Entry point to start/stop the consumer given dependencies: `Queue`, `Tracker`, `CopilotClient`, `Storage`, and `Notifier`.
- Uses zap logger injected by the app; no new globals.

# Notes for Agents
- Workflow: dequeue item → determine epic via tracker → choose prompt → create task run record → start copilot session → stream raw events to storage → update statuses → emit Slack/notify events on completion or error.
- Respect context cancellation for graceful shutdown; ensure resources from copilot sessions are closed.***
