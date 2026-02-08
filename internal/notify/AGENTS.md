# Purpose
Send lifecycle notifications (task completion, batch idle/blocked, errors) through pluggable notifiers; Slack webhook is the default implementation.

# Exposed Interfaces
- `Notifier` interface with methods such as `NotifyTaskFinished`, `NotifyBatchIdle`, and `NotifyError`.
- Slack implementation posts JSON payloads with relevant identifiers and statuses; noop implementation when webhook is absent.

# Notes for Agents
- Keep payloads concise and redact secrets; include batch/run/task refs for traceability.
- Use HTTP client timeouts and zap logging for errors; do nothing quietly when webhook URL is empty.***
