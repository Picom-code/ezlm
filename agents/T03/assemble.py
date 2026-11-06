#!/usr/bin/env python3
"""Merges the per-model T03 rows into agents/T03/matrix.json.

Every agent writes agents/T03/models/<id>.json for the models it rendered. This
script collects them, sorts by id, reports which catalog models are still
missing and which rows are duplicated, and prints a compact table.

  agents/T03/assemble.py                 # write matrix.json, print the table
  agents/T03/assemble.py --check         # write nothing, exit 1 if incomplete
  agents/T03/assemble.py --out /tmp/x.json --quiet

It runs cleanly with zero, one or many model files: with none it writes
{"version": 1, "models": []} and reports every catalog model as missing.
"""
import argparse
import glob
import json
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.normpath(os.path.join(HERE, "..", ".."))
DEFAULT_MODELS = os.path.join(HERE, "models")
DEFAULT_CATALOG = os.path.join(ROOT, "internal", "catalog", "catalog.json")
DEFAULT_OUT = os.path.join(HERE, "matrix.json")

# The row keys the brief defines. Extra keys are kept; missing ones are warned
# about once, so a half-written row still lands in the matrix.
REQUIRED = [
    "id",
    "repo",
    "template_source",
    "tool_call_format",
    "tools_supported",
    "thinking_toggle",
    "effort_levels_accepted",
    "effort_levels_rejected",
    "system_role",
    "developer_role",
    "known_template_errors",
    "notes",
]

# Files that live in models/ but are not rows.
SKIP_SUFFIX = (".renders.json", ".templates.json")


def row_files(models_dir):
    out = []
    for path in sorted(glob.glob(os.path.join(models_dir, "*.json"))):
        name = os.path.basename(path)
        if name.startswith("_") or name.endswith(SKIP_SUFFIX):
            continue
        out.append(path)
    return out


def catalog_ids(path):
    """Ids from the catalog, in catalog order. Missing catalog is not fatal."""
    try:
        with open(path) as f:
            data = json.load(f)
    except FileNotFoundError:
        print(f"warning: no catalog at {path}; skipping the missing-model report")
        return None
    models = data.get("models", data if isinstance(data, list) else [])
    ids = []
    for m in models:
        if isinstance(m, dict) and isinstance(m.get("id"), str):
            ids.append(m["id"])
    return ids


def load_rows(paths, warnings):
    rows, seen = [], {}
    for path in paths:
        name = os.path.basename(path)
        try:
            with open(path) as f:
                row = json.load(f)
        except json.JSONDecodeError as exc:
            warnings.append(f"{name}: not valid JSON ({exc}); skipped")
            continue
        if not isinstance(row, dict):
            warnings.append(f"{name}: not a JSON object; skipped")
            continue
        rid = row.get("id")
        if not isinstance(rid, str) or not rid:
            warnings.append(f"{name}: no string id; skipped")
            continue
        missing = [k for k in REQUIRED if k not in row]
        if missing:
            warnings.append(f"{name}: missing {', '.join(missing)}")
        row["_file"] = name
        if rid in seen:
            warnings.append(
                f"{name}: duplicate row for {rid}, already read from {seen[rid]['_file']}; "
                "keeping the first"
            )
            continue
        seen[rid] = row
        rows.append(row)
    return rows


def cell(value):
    if value is True:
        return "yes"
    if value is False:
        return "no"
    if value is None:
        return "-"
    if isinstance(value, (list, tuple)):
        return ",".join(str(v) for v in value) if value else "-"
    return str(value)


def table(rows, width=22):
    head = ["id", "tools", "thinking", "effort ok", "effort bad", "effort ignored",
            "sys", "dev"]
    body = [
        [
            r["id"],
            cell(r.get("tools_supported")),
            cell(r.get("thinking_toggle")),
            cell(r.get("effort_levels_accepted")),
            cell(r.get("effort_levels_rejected")),
            cell(r.get("effort_levels_ignored")),
            cell(r.get("system_role")),
            cell(r.get("developer_role")),
        ]
        for r in rows
    ]
    widths = [len(h) for h in head]
    for line in body:
        widths = [max(w, len(c)) for w, c in zip(widths, line)]
    widths[0] = min(widths[0], width)
    fmt = "  ".join("{:<%d}" % w for w in widths)
    out = [fmt.format(*head), fmt.format(*["-" * w for w in widths])]
    for line in body:
        line = list(line)
        if len(line[0]) > widths[0]:
            line[0] = line[0][: widths[0] - 1] + "~"
        out.append(fmt.format(*line))
    return "\n".join(out)


def main():
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--models-dir", default=DEFAULT_MODELS)
    ap.add_argument("--catalog", default=DEFAULT_CATALOG)
    ap.add_argument("--out", default=DEFAULT_OUT)
    ap.add_argument("--check", action="store_true",
                    help="write nothing; exit 1 if any catalog model is missing or a row is duplicated")
    ap.add_argument("--quiet", action="store_true", help="only print the table")
    args = ap.parse_args()

    warnings = []
    paths = row_files(args.models_dir)
    if not os.path.isdir(args.models_dir):
        warnings.append(f"no models directory at {args.models_dir}")
    rows = load_rows(paths, warnings)
    rows.sort(key=lambda r: r["id"])

    catalog = catalog_ids(args.catalog)
    ids = {r["id"] for r in rows}
    missing = [c for c in (catalog or []) if c not in ids]
    extra = [r["id"] for r in rows if catalog is not None and r["id"] not in catalog]

    dupes = [w for w in warnings if "duplicate row" in w]

    if not args.check:
        os.makedirs(os.path.dirname(os.path.abspath(args.out)) or ".", exist_ok=True)
        payload = {
            "version": 1,
            "models": [{k: v for k, v in r.items() if k != "_file"} for r in rows],
        }
        with open(args.out, "w") as f:
            json.dump(payload, f, indent=1, sort_keys=False)
            f.write("\n")

    if not args.quiet:
        for w in warnings:
            print("warning: " + w)

    if rows:
        print(table(rows))
    else:
        print("(no model rows yet)")

    total = len(catalog) if catalog is not None else None
    print()
    if total:
        print(f"{len(rows)}/{total} catalog models have a row")
    else:
        print(f"{len(rows)} rows")
    if missing:
        print(f"missing ({len(missing)}): {', '.join(missing)}")
    if extra:
        print(f"not in catalog ({len(extra)}): {', '.join(extra)}")
    if dupes:
        print(f"duplicates: {len(dupes)}")
    if not args.check:
        print(f"wrote {args.out}")

    if args.check and (missing or dupes or extra):
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
