#!/usr/bin/env python3
"""Merge every agents/T12/rows/*.csv into agents/T12/strings.csv.

The 16 T12 agents each write their own rows file, so at consolidation the
Lead runs this once to get a single table. Rows are deduped by
(file, line, current_text), sorted by file then line, and a count per owner
task is printed.

Usage:
  python3 agents/T12/merge_rows.py              # rows/ -> strings.csv
  python3 agents/T12/merge_rows.py --strict     # nonzero exit on bad rows
  python3 agents/T12/merge_rows.py --rows DIR --out FILE

Behaviour worth knowing:
  - files are read in name order, so the first agent to report a duplicate
    wins the row; empty fields are then filled from the later copies;
  - a duplicate whose problem, proposed_text or severity disagrees with the
    first copy is reported as a conflict and the first copy is kept;
  - the line column may be "42", "42-45", "42,44" or empty; sorting uses
    the first number in it;
  - a missing or short row is skipped and counted, never fatal.
"""
import argparse
import csv
import os
import re
import sys
from collections import Counter, OrderedDict

COLUMNS = ["file", "line", "current_text", "problem",
           "proposed_text", "owner_task", "severity"]
SEVERITIES = ("blocker", "major", "minor")

# header spellings the agents may use, mapped to the canonical column name
ALIASES = {
    "file": "file", "path": "file", "filename": "file",
    "line": "line", "lines": "line", "line_no": "line", "lineno": "line",
    "current_text": "current_text", "current": "current_text",
    "text": "current_text", "current_string": "current_text",
    "problem": "problem", "issue": "problem",
    "proposed_text": "proposed_text", "proposed": "proposed_text",
    "fix": "proposed_text", "suggestion": "proposed_text",
    "owner_task": "owner_task", "owner": "owner_task", "task": "owner_task",
    "severity": "severity", "sev": "severity",
}


def canon(name):
    key = re.sub(r"[\s\-]+", "_", name.strip().lower())
    return ALIASES.get(key, key)


def norm_path(value):
    value = value.strip().replace("\\", "/")
    while value.startswith("./"):
        value = value[2:]
    return value


def clean_cell(value):
    """Trim a cell, including the stray quotes a hand-written CSV leaves."""
    value = value.strip()
    if len(value) >= 2 and value[0] == value[-1] == '"':
        value = value[1:-1].strip()
    return value


def norm_text(value):
    return " ".join(value.split())


def line_key(value):
    """Sort key for a line cell: numbers first, then ranges, then the rest."""
    value = value.strip()
    if not value:
        return (0, 0, "")
    m = re.search(r"\d+", value)
    if m:
        return (1, int(m.group()), value)
    return (2, 0, value)


def read_rows(path):
    """Yield (row dict, line number in the file, problem or None)."""
    with open(path, newline="", encoding="utf-8-sig") as f:
        reader = csv.reader(f)
        header = None
        for lineno, cells in enumerate(reader, start=1):
            if not cells or all(not c.strip() for c in cells):
                continue
            if header is None:
                names = [canon(c) for c in cells]
                if "current_text" in names or "file" in names:
                    header = names
                    continue
                # no header at all: assume the documented column order
                header = list(COLUMNS)
            row = {}
            for i, name in enumerate(header):
                if name in COLUMNS and i < len(cells):
                    row[name] = clean_cell(cells[i])
            if not row.get("file"):
                yield row, lineno, "no file"
                continue
            if not row.get("current_text"):
                yield row, lineno, "no current_text"
                continue
            yield row, lineno, None


def fill(base, extra):
    """Fill empty fields of base from a duplicate; return conflicting names."""
    conflicts = []
    for name in COLUMNS:
        if name in ("file", "line", "current_text"):
            continue
        mine, theirs = base.get(name, ""), extra.get(name, "")
        if not mine:
            base[name] = theirs
        elif theirs and norm_text(theirs) != norm_text(mine):
            conflicts.append(name)
    return conflicts


def main(argv=None):
    here = os.path.dirname(os.path.abspath(__file__))
    ap = argparse.ArgumentParser(description="Merge agents/T12/rows/*.csv.")
    ap.add_argument("--rows", default=os.path.join(here, "rows"),
                    help="directory of per-agent CSVs (default: agents/T12/rows)")
    ap.add_argument("--out", default=os.path.join(here, "strings.csv"),
                    help="output CSV (default: agents/T12/strings.csv)")
    ap.add_argument("--strict", action="store_true",
                    help="exit 2 if any row was skipped or any duplicate conflicts")
    ap.add_argument("--show-conflicts", type=int, default=10,
                    help="how many duplicate conflicts to print (default: 10)")
    args = ap.parse_args(argv)

    if not os.path.isdir(args.rows):
        print(f"no rows directory: {args.rows}", file=sys.stderr)
        return 1
    out_abs = os.path.abspath(args.out)
    inputs = sorted(
        os.path.join(args.rows, n) for n in os.listdir(args.rows)
        if n.lower().endswith(".csv") and os.path.isfile(os.path.join(args.rows, n))
        and os.path.abspath(os.path.join(args.rows, n)) != out_abs
    )
    if not inputs:
        print(f"no *.csv in {args.rows}; wrote header only to {args.out}",
              file=sys.stderr)
        os.makedirs(os.path.dirname(out_abs), exist_ok=True)
        with open(out_abs, "w", newline="", encoding="utf-8") as f:
            csv.writer(f, lineterminator="\n").writerow(COLUMNS)
        return 1

    merged = OrderedDict()   # key -> row
    display = {}             # key -> (file, line) as the agent wrote them
    sources = {}             # key -> [agent file names]
    skipped = []             # (path, lineno, reason)
    conflicts = []           # (key, field, kept, dropped, sources)
    per_file = Counter()

    for path in inputs:
        name = os.path.basename(path)
        for row, lineno, problem in read_rows(path):
            if problem:
                skipped.append((name, lineno, problem))
                continue
            row["file"] = norm_path(row["file"])
            key = (row["file"].casefold(), line_key(row["line"]),
                   norm_text(row["current_text"]).casefold())
            if key not in merged:
                merged[key] = dict(row)
                display[key] = (row["file"], row["line"])
                sources[key] = [name]
                per_file[name] += 1
                continue
            sources[key].append(name)
            for field in fill(merged[key], row):
                conflicts.append((key, field, merged[key][field], row[field],
                                  list(sources[key])))

    rows = list(merged.values())
    rows.sort(key=lambda r: (r["file"].casefold(), line_key(r["line"]),
                             norm_text(r["current_text"]).casefold()))

    os.makedirs(os.path.dirname(out_abs), exist_ok=True)
    with open(out_abs, "w", newline="", encoding="utf-8") as f:
        w = csv.writer(f, lineterminator="\n")
        w.writerow(COLUMNS)
        for row in rows:
            w.writerow([row.get(c, "") for c in COLUMNS])

    owners = Counter(r.get("owner_task") or "(none)" for r in rows)
    severities = Counter(r.get("severity") or "(none)" for r in rows)

    print(f"merged {len(rows)} rows from {len(inputs)} files -> {args.out}")
    dupes = sum(len(v) - 1 for v in sources.values())
    print(f"dropped {dupes} duplicate rows, skipped {len(skipped)} malformed")
    print("per owner task:")
    for owner, n in sorted(owners.items(), key=lambda kv: (-kv[1], kv[0])):
        print(f"  {owner:<10} {n:>4}")
    print("per severity: " + ", ".join(
        f"{s} {n}" for s, n in sorted(severities.items(), key=lambda kv: (-kv[1], kv[0]))))
    bad_sev = [r for r in rows if r.get("severity") not in SEVERITIES]
    if bad_sev:
        print(f"WARN {len(bad_sev)} rows have a severity outside "
              f"{'/'.join(SEVERITIES)}", file=sys.stderr)
    for name, lineno, reason in skipped:
        print(f"WARN skip {name}:{lineno}: {reason}", file=sys.stderr)
    for key, field, kept, dropped, srcs in conflicts[:args.show_conflicts]:
        where = display.get(key) or (key[0], key[1][2])
        print(f"CONFLICT {where[0]}:{where[1]} {field}: "
              f"kept {kept!r} from {srcs[0]}, dropped {dropped!r} from "
              f"{srcs[-1]}", file=sys.stderr)
    if len(conflicts) > args.show_conflicts:
        print(f"... and {len(conflicts) - args.show_conflicts} more conflicts",
              file=sys.stderr)

    if args.strict and (skipped or conflicts):
        return 2
    return 0


if __name__ == "__main__":
    sys.exit(main())
