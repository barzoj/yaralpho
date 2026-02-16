#!/usr/bin/env python3
"""
Sync a linear "plan notebook" (.ipynb) into Beads (bd), using *in-cell* service headers.

Model:
- Cell 0 is the Epic (bd issue type: epic by default).
- Cells 1..N are Tasks (bd issue type: task by default), all with --parent <epic_id>.
- Blocking dependency is linear: task[i] depends on task[i-1] (bd dep add <dependent> <dependency>).
- Notebook is the single source of truth. On each save:
  - Create missing issues
  - Update existing issues to match cell text
  - Delete issues under the epic that are no longer present in the notebook
  - Reset blockers for tasks under this epic (best-effort) and then rebuild the linear chain

Metadata is stored in the cell text, e.g. at the top of each cell:

  # beads-id: bd-xxxx
  # beads-status: synced

If beads-id is missing, it's a new task and will be created on sync.

Minimal dependencies: Python stdlib only. Requires `bd` CLI available in PATH.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import subprocess
from typing import Any, Dict, List, Optional, Tuple

# Allow one or more leading '#' so meta renders nicely as markdown headings (e.g., '#### beads-id: ...').
BEADS_ID_RE = re.compile(r"^\s*#+\s*beads-id\s*:\s*(\S+)\s*$", re.IGNORECASE)
BEADS_STATUS_RE = re.compile(r"^\s*#+\s*beads-status\s*:\s*(.+?)\s*$", re.IGNORECASE)
BEADS_SERVICE_LINE_RE = re.compile(r"^\s*#+\s*beads-[a-z0-9_-]+\s*:", re.IGNORECASE)


def run_bd(args: List[str], *, cwd: Optional[str] = None) -> Tuple[int, str, str]:
    """Run bd command, return (rc, stdout, stderr)."""
    p = subprocess.run(
        ["bd", *args],
        cwd=cwd,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )
    return p.returncode, p.stdout.strip(), p.stderr.strip()


def parse_bd_json(stdout: str) -> Any:
    """
    Beads docs recommend --json for programmatic access, but the shape can vary:
    - A single JSON object
    - A JSON array
    - JSON objects per line (JSONL)

    This parser is tolerant.
    """
    if not stdout:
        return None

    # 1) Try whole output as JSON
    try:
        return json.loads(stdout)
    except json.JSONDecodeError:
        pass

    # 2) Try JSON per line
    items = []
    for line in stdout.splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            items.append(json.loads(line))
        except json.JSONDecodeError:
            # If we hit non-JSON lines, keep going; at end, decide what to do
            continue
    if items:
        return items

    raise ValueError(f"bd --json output was not parseable JSON:\n{stdout[:1000]}")


def load_ipynb(path: str) -> Dict[str, Any]:
    with open(path, "r", encoding="utf-8") as f:
        return json.load(f)


def save_ipynb(path: str, nb: Dict[str, Any]) -> None:
    with open(path, "w", encoding="utf-8") as f:
        json.dump(nb, f, ensure_ascii=False, indent=1)
        f.write("\n")


def get_cell_source(cell: Dict[str, Any]) -> str:
    # ipynb uses either a string or list of lines
    src = cell.get("source", "")
    if isinstance(src, list):
        return "".join(src)
    return str(src)


def set_cell_source(cell: Dict[str, Any], text: str) -> None:
    # Store as string to keep it simple
    cell["source"] = text


def split_service_header(text: str) -> Tuple[List[str], List[str]]:
    """
    Returns (service_lines, rest_lines).
    Service lines are a contiguous block at the top where each line matches '# beads-...:'
    """
    lines = text.splitlines(keepends=True)
    service = []
    i = 0
    while i < len(lines) and BEADS_SERVICE_LINE_RE.match(lines[i]):
        service.append(lines[i])
        i += 1
    return service, lines[i:]


def extract_beads_id(text: str) -> Optional[str]:
    for line in text.splitlines():
        m = BEADS_ID_RE.match(line)
        if m:
            return m.group(1).strip()
    return None


def upsert_service_fields(
    text: str, *, beads_id: Optional[str], status: Optional[str]
) -> str:
    """
    Ensure service header block exists and has the desired beads-id and beads-status.
    - If beads_id is None -> leave beads-id as-is (do not delete).
    - If status is None -> leave beads-status as-is (do not delete).
    """
    service, rest = split_service_header(text)

    # Map existing service keys -> line index
    key_to_idx: Dict[str, int] = {}
    for idx, line in enumerate(service):
        if BEADS_ID_RE.match(line):
            key_to_idx["beads-id"] = idx
        elif BEADS_STATUS_RE.match(line):
            key_to_idx["beads-status"] = idx

    def set_line(key: str, value_line: str) -> None:
        if key in key_to_idx:
            service[key_to_idx[key]] = value_line
        else:
            service.append(value_line)

    if beads_id is not None:
        set_line("beads-id", f"#### beads-id: {beads_id}\n")

    if status is not None:
        set_line("beads-status", f"#### beads-status: {status}\n")

    # Keep a blank line between service block and content if content exists and
    # service exists but there isn't already a blank line.
    out = "".join(service)
    if service and rest:
        if not (out.endswith("\n\n") or (rest and rest[0].strip() == "")):
            out += "\n"
    out += "".join(rest)
    return out


def strip_service_header_for_payload(text: str) -> str:
    """Remove only the top contiguous beads service block; everything else remains."""
    _, rest = split_service_header(text)
    return "".join(rest).strip()


def derive_title_and_description(cell_text: str) -> Tuple[str, str]:
    """
    Extract title/description from notebook cell content.
    - Title prefers a line formatted as: '# $TITLE$: Actual Title'.
    - If no $TITLE$ line exists, fall back to first non-empty line (heading hashes removed).
    - Description is the remaining content (without service headers or the $TITLE$ line),
      preserving markdown structure.
    """
    payload = strip_service_header_for_payload(cell_text)
    lines = payload.splitlines(keepends=True)

    # 1) Prefer explicit $TITLE$ line
    title = ""
    description_lines = lines
    for idx, line in enumerate(lines):
        m = re.match(r"^\s*#\s*\$TITLE\$\s*:\s*(.*)$", line)
        if m:
            title = m.group(1).strip()
            description_lines = lines[:idx] + lines[idx + 1 :]
            break

    # 2) Fallback: first non-empty line (markdown heading removed)
    if not title:
        for idx, line in enumerate(lines):
            if line.strip():
                title = re.sub(r"^\s*#{1,6}\s*", "", line).strip()
                description_lines = lines[:idx] + lines[idx + 1 :]
                break

    if not title:
        title = "(untitled)"

    description = "".join(description_lines).strip()
    return title, description


def bd_create_issue(
    title: str, description: str, *, issue_type: str, parent: Optional[str], cwd: str
) -> str:
    args = [
        "create",
        title,
        "--type",
        issue_type,
        "--description",
        description,
        "--json",
    ]
    if parent:
        args.extend(["--parent", parent])
    rc, out, err = run_bd(args, cwd=cwd)
    if rc != 0:
        raise RuntimeError(f"bd create failed (rc={rc}): {err or out}")
    data = parse_bd_json(out)

    # Heuristic: return id from known common fields
    # (Exact schema can evolve; keep it tolerant.)
    if isinstance(data, dict):
        for k in ("id", "issue_id", "beads_id"):
            if k in data and isinstance(data[k], str):
                return data[k]
    # Sometimes it can be a list of results
    if isinstance(data, list):
        for item in data:
            if isinstance(item, dict):
                for k in ("id", "issue_id", "beads_id"):
                    if k in item and isinstance(item[k], str):
                        return item[k]

    raise RuntimeError(f"Could not find created issue id in bd output: {out[:500]}")


def bd_update_issue(issue_id: str, title: str, description: str, *, cwd: str) -> None:
    args = [
        "update",
        issue_id,
        "--title",
        title,
        "--description",
        description,
        "--json",
    ]
    rc, out, err = run_bd(args, cwd=cwd)
    if rc != 0:
        raise RuntimeError(f"bd update failed for {issue_id} (rc={rc}): {err or out}")


def bd_delete_issue(issue_id: str, *, cwd: str) -> None:
    # --force skips confirmation (docs: bd delete <id> -f --json) :contentReference[oaicite:0]{index=0}
    args = ["delete", issue_id, "--force", "--json"]
    rc, out, err = run_bd(args, cwd=cwd)
    if rc != 0:
        raise RuntimeError(f"bd delete failed for {issue_id} (rc={rc}): {err or out}")


def bd_list_children(epic_id: str, *, cwd: str) -> List[str]:
    """
    Get all issues with --parent epic_id.
    """
    rc, out, err = run_bd(["list", "--parent", epic_id, "--json"], cwd=cwd)
    if rc != 0:
        raise RuntimeError(f"bd list --parent failed (rc={rc}): {err or out}")

    data = parse_bd_json(out)
    ids: List[str] = []

    # Expect either array of dicts, or JSONL dicts
    if isinstance(data, list):
        for item in data:
            if isinstance(item, dict):
                _id = item.get("id") or item.get("issue_id")
                if isinstance(_id, str):
                    ids.append(_id)
    elif isinstance(data, dict):
        # If wrapped: {"issues":[...]}
        issues = data.get("issues")
        if isinstance(issues, list):
            for item in issues:
                if isinstance(item, dict):
                    _id = item.get("id") or item.get("issue_id")
                    if isinstance(_id, str):
                        ids.append(_id)
    return ids


def bd_blocked_map(*, cwd: str) -> Dict[str, List[str]]:
    """
    Best-effort: bd blocked --json returns blocked issues and their blockers. :contentReference[oaicite:1]{index=1}
    We use it to remove existing blockers before rebuilding the chain.
    """
    rc, out, err = run_bd(["blocked", "--json"], cwd=cwd)
    if rc != 0:
        # If blocked fails, we can still proceed (just won't remove old deps)
        return {}

    data = parse_bd_json(out)
    blocked: Dict[str, List[str]] = {}

    # The JSON shape isn't strictly documented, so be tolerant:
    # - list of {id, blockers:[{id...}]} or similar
    if isinstance(data, list):
        for item in data:
            if not isinstance(item, dict):
                continue
            bid = item.get("id") or item.get("issue_id")
            if not isinstance(bid, str):
                continue
            blockers = []
            bl = (
                item.get("blockers")
                or item.get("blocked_by")
                or item.get("dependencies")
            )
            if isinstance(bl, list):
                for b in bl:
                    if isinstance(b, dict):
                        b_id = b.get("id") or b.get("issue_id")
                        if isinstance(b_id, str):
                            blockers.append(b_id)
                    elif isinstance(b, str):
                        blockers.append(b)
            blocked[bid] = blockers

    elif isinstance(data, dict):
        # possible wrapper: {"blocked":[...]}
        arr = data.get("blocked") or data.get("issues")
        if isinstance(arr, list):
            for item in arr:
                if isinstance(item, dict):
                    bid = item.get("id") or item.get("issue_id")
                    if isinstance(bid, str):
                        blockers = []
                        bl = (
                            item.get("blockers")
                            or item.get("blocked_by")
                            or item.get("dependencies")
                        )
                        if isinstance(bl, list):
                            for b in bl:
                                if isinstance(b, dict):
                                    b_id = b.get("id") or b.get("issue_id")
                                    if isinstance(b_id, str):
                                        blockers.append(b_id)
                                elif isinstance(b, str):
                                    blockers.append(b)
                        blocked[bid] = blockers

    return blocked


def bd_dep_add(dependent: str, dependency: str, *, cwd: str) -> None:
    # Semantics documented: bd dep add <dependent> <dependency> :contentReference[oaicite:2]{index=2}
    rc, out, err = run_bd(["dep", "add", dependent, dependency, "--json"], cwd=cwd)
    if rc != 0:
        msg = err or out
        # Treat existing edges as success to keep operation idempotent.
        if msg and "UNIQUE constraint failed" in msg:
            return
        raise RuntimeError(
            f"bd dep add failed {dependent} <- {dependency} (rc={rc}): {msg}"
        )


def bd_dep_remove(dependent: str, dependency: str, *, cwd: str) -> None:
    rc, out, err = run_bd(["dep", "remove", dependent, dependency, "--json"], cwd=cwd)
    if rc != 0:
        # If the edge didn't exist, beads may return non-zero; treat as best-effort.
        return


def main() -> int:
    ap = argparse.ArgumentParser(description="Sync ipynb plan into Beads (bd).")
    ap.add_argument("notebook", help="Path to .ipynb")
    ap.add_argument(
        "--cwd",
        default=None,
        help="Working directory where bd is initialized (default: notebook dir)",
    )
    ap.add_argument(
        "--epic-type", default="epic", help="Beads issue type for the first cell"
    )
    ap.add_argument(
        "--task-type", default="task", help="Beads issue type for task cells"
    )
    ap.add_argument(
        "--force",
        action="store_true",
        help="Recreate epic and tasks, ignoring existing beads-id metadata in the notebook",
    )
    ap.add_argument(
        "--skip-empty",
        action="store_true",
        help="Skip task cells that have no content after headers",
    )
    ap.add_argument("--sync", action="store_true", help="Run `bd sync` at the end")
    args = ap.parse_args()

    nb_path = os.path.abspath(args.notebook)
    cwd = os.path.abspath(args.cwd) if args.cwd else os.path.dirname(nb_path)

    nb = load_ipynb(nb_path)
    cells = nb.get("cells", [])
    if not isinstance(cells, list) or not cells:
        raise SystemExit("Notebook has no cells.")

    # ---- 0) Parse cells
    cell_texts: List[str] = [get_cell_source(c) for c in cells]
    epic_text = cell_texts[0]
    epic_id = extract_beads_id(epic_text)

    # Force mode: ignore existing beads metadata and recreate everything
    if args.force:
        print("Force mode enabled: recreating epic and tasks, ignoring notebook beads-id metadata.")
        old_epic_id = epic_id
        if old_epic_id:
            try:
                old_children = bd_list_children(old_epic_id, cwd=cwd)
            except RuntimeError as e:
                print(f"Warning: could not list children of {old_epic_id}: {e}")
                old_children = []

            for child_id in old_children:
                try:
                    bd_delete_issue(child_id, cwd=cwd)
                    print(f"Deleted old task {child_id}")
                except RuntimeError as e:
                    print(f"Warning: could not delete old task {child_id}: {e}")

            try:
                bd_delete_issue(old_epic_id, cwd=cwd)
                print(f"Deleted old epic {old_epic_id}")
            except RuntimeError as e:
                print(f"Warning: could not delete old epic {old_epic_id}: {e}")

        # Ignore any beads-id in the notebook; we'll create a fresh epic and tasks.
        epic_id = None

    # ---- 1) Ensure epic exists
    epic_title, epic_desc = derive_title_and_description(epic_text)
    if not epic_id:
        epic_id = bd_create_issue(
            epic_title, epic_desc, issue_type=args.epic_type, parent=None, cwd=cwd
        )
        cell_texts[0] = upsert_service_fields(
            epic_text, beads_id=epic_id, status="synced"
        )
        print(f"Created epic {epic_id}")
    else:
        # Keep epic updated to notebook text (optional but consistent)
        bd_update_issue(epic_id, epic_title, epic_desc, cwd=cwd)
        cell_texts[0] = upsert_service_fields(
            epic_text, beads_id=epic_id, status="synced"
        )
        print(f"Using existing epic {epic_id}")

    # ---- 2) Delete beads tasks under epic that are no longer present in notebook
    beads_children = bd_list_children(epic_id, cwd=cwd)
    notebook_task_ids: List[str] = []
    task_cell_indices: List[int] = []

    for i in range(1, len(cell_texts)):
        txt = cell_texts[i]
        payload = strip_service_header_for_payload(txt)
        if args.skip_empty and not payload.strip():
            continue
        bid = extract_beads_id(txt)
        if bid and not args.force:
            notebook_task_ids.append(bid)
        task_cell_indices.append(i)

    if not args.force:
        notebook_task_id_set = set(notebook_task_ids)
        for child_id in beads_children:
            if child_id not in notebook_task_id_set:
                # Hard delete (with --force). Dependencies you said you'll handle manually for now.
                bd_delete_issue(child_id, cwd=cwd)
                print(f"Deleted orphan beads task under epic: {child_id}")

    # ---- 3) Create or update tasks, ensure parent is epic (by creating with --parent epic)
    ordered_task_ids: List[str] = []
    for idx in task_cell_indices:
        txt = cell_texts[idx]
        payload = strip_service_header_for_payload(txt)
        if args.skip_empty and not payload.strip():
            continue

        title, desc = derive_title_and_description(txt)
        bid = None if args.force else extract_beads_id(txt)

        if not bid:
            bid = bd_create_issue(
                title, desc, issue_type=args.task_type, parent=epic_id, cwd=cwd
            )
            print(f"Created task {bid} for cell {idx}")
        else:
            bd_update_issue(bid, title, desc, cwd=cwd)

        cell_texts[idx] = upsert_service_fields(txt, beads_id=bid, status="synced")
        ordered_task_ids.append(bid)

    # ---- 4) Reset blockers (best-effort) + rebuild linear chain
    # We remove current blockers for tasks we manage (from `bd blocked --json`),
    # then enforce chain: t[i] depends on t[i-1].
    blocked_map = bd_blocked_map(cwd=cwd)
    managed_set = set(ordered_task_ids)

    # Remove existing blockers for managed tasks (best-effort, only those currently blocked).
    for dependent, blockers in blocked_map.items():
        if dependent in managed_set:
            for dep in blockers:
                # Only remove blockers that are also in our managed set, to avoid breaking external constraints.
                if dep in managed_set:
                    bd_dep_remove(dependent, dep, cwd=cwd)

    # Explicitly drop our intended linear edges before re-adding, even if bd_blocked missed them.
    for i in range(1, len(ordered_task_ids)):
        dependent = ordered_task_ids[i]
        dependency = ordered_task_ids[i - 1]
        bd_dep_remove(dependent, dependency, cwd=cwd)

    # Now set the linear chain (safe even if already exists; add is idempotent in practice).
    for i in range(1, len(ordered_task_ids)):
        dependent = ordered_task_ids[i]
        dependency = ordered_task_ids[i - 1]
        bd_dep_add(dependent, dependency, cwd=cwd)
    print(f"Rebuilt linear dependency chain for {len(ordered_task_ids)} tasks.")

    # ---- 5) Write notebook back
    for i, c in enumerate(cells):
        set_cell_source(c, cell_texts[i])
    nb["cells"] = cells
    save_ipynb(nb_path, nb)
    print(f"Updated notebook: {nb_path}")

    if args.sync:
        rc, out, err = run_bd(["sync"], cwd=cwd)
        if rc != 0:
            raise RuntimeError(f"bd sync failed (rc={rc}): {err or out}")
        print("bd sync done.")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
