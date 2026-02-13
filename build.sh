#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

cd "${ROOT_DIR}"
go test ./... && (cd internal/copilot/codex-ts && npm install && npm run package:linux-x64) && go build -o yaralpho.bin ./cmd
