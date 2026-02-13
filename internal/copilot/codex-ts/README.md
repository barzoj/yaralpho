# Codex Wrapper

This wrapper defines the CLI contract used by the Go `internal/copilot.Codex`
client.

## CLI Contract

### Required flags

- `--repo-path <path>`
- `--prompt <text>`

### Optional flags

- `--help` prints usage and exits with status `0`.

## Exit behavior

- Invalid or missing arguments: writes error details to `stderr`, exits non-zero.
- `--help`: writes usage to `stdout`, exits `0`.
- Valid required args: starts a Codex streamed run and writes events to `stdout`
  as NDJSON.

## stdout/stderr separation

- `stdout` is reserved for machine-readable event output only.
- Human-readable diagnostics and error messages go to `stderr`.
- During `--repo-path/--prompt` execution, each stdout line is one JSON object
  from the SDK stream, for example:

```json
{"type":"thread.started","thread_id":"..."}
{"type":"turn.started"}
{"type":"item.completed","item":{"type":"agent_message","text":"..."}}
{"type":"turn.completed","usage":{...}}
```

## Runtime behavior

- The wrapper starts `new Codex().startThread({ workingDirectory: <repo-path> })`.
- It runs `runStreamed(<prompt>)` and forwards each event unchanged.
- Any fatal setup/auth/stream error is emitted to `stderr` and exits non-zero.
- CLI resolution for the SDK backend:
  - Uses `YARALPHO_CODEX_CLI_PATH` if set.
  - Else uses `CODEX_CLI_PATH` if set.
  - Else resolves `codex` from `PATH`.
  - If none are available, SDK falls back to packaged binaries (may fail in standalone builds).

## Local verification

```bash
cd internal/copilot/codex-ts
npm run build
node dist/main.js --help
node dist/main.js --repo-path /tmp --prompt "hello"
```

## Packaging (Linux x64)

Build a standalone Linux x64 wrapper binary with a fixed output path:

```bash
cd internal/copilot/codex-ts
npm run package:linux-x64
```

This script performs:

1. `npm run build` to refresh `dist/main.js`
2. `mkdir -p bin` to ensure a stable output directory
3. `bun build --compile src/main.ts --outfile bin/codex-wrapper-linux-x64`

Expected artifact:

- `internal/copilot/codex-ts/bin/codex-wrapper-linux-x64`

Verification:

```bash
cd internal/copilot/codex-ts
npm run package:linux-x64
./bin/codex-wrapper-linux-x64 --help
```

Determinism note:

- Re-running `npm run package:linux-x64` always writes to the same target path and overwrites the previous artifact.

Prerequisite:

- Bun must be installed and available on `PATH` for `npm run package:linux-x64`.
