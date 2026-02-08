# Purpose
Identify whether a reference is an epic and list its child tasks using a tracker backend (beads CLI for this project).

# Exposed Interfaces
- `Tracker` interface with `IsEpic(ctx, ref) (bool, error)` and `ListChildren(ctx, ref) ([]string, error)`.

# Notes for Agents
- Beads implementation will shell out to `bd show` in the configured repo; trim whitespace and preserve child order.
- Avoid logging sensitive repo details; propagate zap logger for structured messages.***
