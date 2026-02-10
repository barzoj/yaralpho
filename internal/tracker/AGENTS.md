# Purpose
Identify whether a reference is an epic, list its child tasks, and access tracker comments using a tracker backend (beads CLI for this project).

# Exposed Interfaces
- `Tracker` interface with:
  - `IsEpic(ctx, ref) (bool, error)`
  - `ListChildren(ctx, ref) ([]string, error)`
  - `AddComment(ctx, ref, text) error`
  - `FetchComments(ctx, ref) ([]Comment, error)`

# Notes for Agents
- Beads implementation will shell out to `bd show` in the configured repo; trim whitespace and preserve child order.
- Comment methods shell out to `bd comments add` and `bd view --json`; multiline comments use temp files.
- Avoid logging sensitive repo details; propagate zap logger for structured messages.***
