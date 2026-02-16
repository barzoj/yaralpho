#!/usr/bin/env bash
# List all bd issues whose IDs start with a given prefix and delete them.
# Usage: ./scripts/bd-delete-prefix.sh <prefix> [--yes]

set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "Usage: $0 <prefix> [--yes]" >&2
  exit 1
fi

PREFIX="$1"
AUTO_YES="${2-}"

if ! command -v bd >/dev/null 2>&1; then
  echo "bd CLI not found. Install beads first." >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required to parse bd output." >&2
  exit 1
fi

# Support both bd JSON formats: {items: [...]} and bare arrays.
mapfile -t IDS < <(bd list --all --json --limit 0 \
  | jq -r --arg p "$PREFIX" '
      (.items // .)[] | select(.id | startswith($p)) | .id
    ')

if [[ ${#IDS[@]} -eq 0 ]]; then
  echo "No issues found with prefix \"$PREFIX\"."
  exit 0
fi

echo "Issues matching prefix \"$PREFIX\":"
printf '  %s\n' "${IDS[@]}"
echo "Total: ${#IDS[@]}"

if [[ "$AUTO_YES" != "--yes" ]]; then
  read -r -p "Delete ALL of the above issues? Type 'delete' to confirm: " CONFIRM
  if [[ "$CONFIRM" != "delete" ]]; then
    echo "Aborted."
    exit 1
  fi
fi

echo "Deleting..."
bd delete "${IDS[@]}"

echo "Done. Run 'bd sync' to export changes to git if needed."
