#!/usr/bin/env python3
"""
Generate a batch creation payload from a Beads epic.

Outputs pretty JSON and a ready-to-run curl command for the yaralpho API:

  curl -X POST <base-url>/repository/<repo-id>/batches -H 'Content-Type: application/json' -d @- <<'EOF'
  {"items": [...], "session_name": "..."}
  EOF

The script queries `bd` for the epic title (used as default session name) and
child task IDs. Items default to task IDs; optionally include titles.
"""

from __future__ import annotations

import argparse
import json
import subprocess
from typing import Dict, List, Tuple


def run_bd(args: List[str], *, cwd: str | None = None) -> Tuple[int, str, str]:
    proc = subprocess.run(
        ["bd", *args],
        cwd=cwd,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    return proc.returncode, proc.stdout.strip(), proc.stderr.strip()


def parse_bd_json(stdout: str) -> object:
    if not stdout:
        return None
    try:
        return json.loads(stdout)
    except json.JSONDecodeError:
        pass

    items: List[object] = []
    for line in stdout.splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            items.append(json.loads(line))
        except json.JSONDecodeError:
            continue
    if items:
        return items
    raise ValueError(f"bd --json output was not parseable:\n{stdout[:500]}")


def get_epic_title(epic_id: str, *, cwd: str | None) -> str:
    rc, out, err = run_bd(["show", epic_id, "--json"], cwd=cwd)
    if rc != 0:
        raise RuntimeError(f"bd show {epic_id} failed (rc={rc}): {err or out}")
    data = parse_bd_json(out)
    # Try common shapes: list of issues or single issue dict
    issue: Dict[str, object] | None = None
    if isinstance(data, list) and data:
        first = data[0]
        issue = first if isinstance(first, dict) else None
    elif isinstance(data, dict):
        issue = data
    title = ""
    if issue:
        title = str(issue.get("title") or issue.get("name") or "").strip()
    return title or epic_id


def list_child_tasks(epic_id: str, *, cwd: str | None, limit: int) -> List[Tuple[str, str]]:
    """Return list of (id, title) child tasks for the epic."""
    rc, out, err = run_bd([
        "list",
        "--parent",
        epic_id,
        "--json",
        "--limit",
        str(limit),
    ], cwd=cwd)
    if rc != 0:
        raise RuntimeError(f"bd list --parent {epic_id} failed (rc={rc}): {err or out}")
    data = parse_bd_json(out)
    tasks: List[Tuple[str, str]] = []
    if isinstance(data, list):
        for item in data:
            if not isinstance(item, dict):
                continue
            tid = item.get("id") or item.get("issue_id")
            if not isinstance(tid, str):
                continue
            title = str(item.get("title") or item.get("name") or "").strip()
            tasks.append((tid, title))
    elif isinstance(data, dict):
        # Possible wrapper: {"issues":[...]}
        issues = data.get("issues")
        if isinstance(issues, list):
            for item in issues:
                if not isinstance(item, dict):
                    continue
                tid = item.get("id") or item.get("issue_id")
                if not isinstance(tid, str):
                    continue
                title = str(item.get("title") or item.get("name") or "").strip()
                tasks.append((tid, title))
    return tasks


def main() -> int:
    ap = argparse.ArgumentParser(description="Emit batch payload JSON from a Beads epic")
    ap.add_argument("epic", help="Beads epic ID (parent)")
    ap.add_argument("repository", help="Repository ID for the /repository/<id>/batches endpoint (required)")
    ap.add_argument(
        "--session-name",
        help="Optional session_name value; defaults to the epic title if omitted",
    )
    ap.add_argument(
        "--with-titles",
        action="store_true",
        help="Include task titles in each item as 'id: title'",
    )
    ap.add_argument(
        "--base-url",
        default="http://localhost:8080",
        help="API base URL (default: http://localhost:8080)",
    )
    ap.add_argument(
        "--limit",
        type=int,
        default=1000,
        help="Max child tasks to fetch (bd list --limit); default 1000",
    )
    ap.add_argument(
        "--cwd",
        default=None,
        help="Directory where bd is initialized (defaults to current working directory)",
    )
    args = ap.parse_args()

    epic_title = get_epic_title(args.epic, cwd=args.cwd)
    tasks = list_child_tasks(args.epic, cwd=args.cwd, limit=args.limit)
    if not tasks:
        raise SystemExit(f"No child tasks found under epic {args.epic}")

    items: List[str] = []
    for tid, title in tasks:
        if args.with_titles and title:
            items.append(f"{tid}: {title}")
        else:
            items.append(tid)

    session_name = args.session_name or epic_title
    payload = {"items": items, "session_name": session_name}

    json_body = json.dumps(payload, indent=2)
    url = args.base_url.rstrip("/") + f"/repository/{args.repository}/batches"

    print("# Batch payload JSON:\n")
    print(json_body)
    print("\n# Ready-to-run curl:\n")
    print("curl -X POST " + url + " \\")
    print("  -H 'Content-Type: application/json' \\")
    print("  -d @- <<'EOF'")
    print(json_body)
    print("EOF")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
