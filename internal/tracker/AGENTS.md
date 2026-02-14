# Purpose
Access tracker comments and titles using a tracker backend (beads CLI for this project).

# Exposed Interfaces
- `Tracker` interface with:
  - `AddComment(ctx, ref, text) error`
  - `FetchComments(ctx, ref) ([]Comment, error)`
  - `GetTitle(ctx, ref) (string, error)`

# Notes for Agents
- Beads implementation shells out to `bd comments add` and `bd view --json`; multiline comments use temp files.
- Avoid logging sensitive repo details; propagate zap logger for structured messages.
