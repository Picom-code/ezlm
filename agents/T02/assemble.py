#!/usr/bin/env python3
"""Merge the T02 tier proposals into catalog-proposed.json.

Run it from anywhere in the repo:

    python3 agents/T02/assemble.py

It reads every agents/T02/tiers/tier-*.json ({"tier": "<label>",
"models": [...]}), keeps the first entry for each id, drops the later ones,
sorts what is left by score and writes {"version": 1, "models": [...]} in the
shape of internal/catalog/catalog.json. It also prints a summary table.

Tier files are read in tier order (4, 6, ..., 96, 128plus), so "first" means
the smallest tier that proposed the id. A broken tier file, an entry without
an id, a duplicate id or an id that already exists in catalog.json is reported
as a warning and does not fail the run. --strict makes warnings exit 1.

Only the standard library is used, and nothing outside the tier files, the
output file and an optional catalog.json is read.
"""

import argparse
import json
import os
import re
import sys
import tempfile
from pathlib import Path

# Field order used by internal/catalog/catalog.json. Unknown fields keep their
# own order after the known ones, so a newer field is never dropped.
MODEL_KEYS = [
    "id", "name", "family", "vendor", "country", "license", "params_b",
    "moe", "score", "score_source", "layers", "kv_heads", "head_dim",
    "ctx_max", "tool_calling", "repo", "active_params_b", "kv_layers",
    "vision", "native_1bit", "no_kv_quant", "notes", "draft", "quants",
    "swa_layers", "swa_kv_heads", "swa_head_dim", "swa_window",
]
QUANT_KEYS = ["name", "repo", "files", "bytes", "mtp", "quality", "lazy_bytes", "args"]

TIER_GLOB = "tier-*.json"
OUT_NAME = "catalog-proposed.json"


class Findings:
    """Warnings collected during the run, printed once at the end."""

    def __init__(self):
        self.warnings = []

    def warn(self, message):
        self.warnings.append(message)


def tier_sort_key(label):
    """Order tier labels: 4, 6, ..., 96, 128plus, then anything else."""
    m = re.fullmatch(r"(\d+)(plus|\+)?", label.strip().lower())
    if m:
        return (0, int(m.group(1)), 1 if m.group(2) else 0, "")
    return (1, 0, 0, label.strip().lower())


def label_of(path):
    """The tier label a file name carries: tier-128plus.json -> 128plus."""
    stem = path.stem
    return stem[len("tier-"):] if stem.startswith("tier-") else stem


def human_size(n):
    """Bytes as GiB or MiB, the unit the memory tiers are quoted in."""
    if n >= 1 << 30:
        return "%.1f GiB" % (n / (1 << 30))
    if n >= 1 << 20:
        return "%.0f MiB" % (n / (1 << 20))
    return "%.0f KiB" % (n / (1 << 10))


def canonical(obj, order):
    """Reorder known fields, keep unknown ones in their own order."""
    out = {k: obj[k] for k in order if k in obj}
    for k, v in obj.items():
        if k not in out:
            out[k] = v
    return out


def canonical_model(m):
    m = canonical(m, MODEL_KEYS)
    if isinstance(m.get("quants"), list):
        m["quants"] = [canonical(q, QUANT_KEYS) if isinstance(q, dict) else q
                       for q in m["quants"]]
    return m


def check_entry(raw, path, index, findings):
    """Check one model entry, warn about what is missing, return a copy or None."""
    where = "%s: model #%d" % (path.name, index + 1)
    if not isinstance(raw, dict):
        findings.warn("%s is not an object; skipped" % where)
        return None
    m = dict(raw)
    mid = m.get("id")
    if not isinstance(mid, str) or not mid.strip():
        findings.warn("%s has no id; skipped" % where)
        return None
    m["id"] = mid.strip()
    if m["id"] != mid:
        findings.warn("%s: id %r had surrounding spaces; trimmed" % (path.name, mid))
    where = "%s: %s" % (path.name, m["id"])

    if not isinstance(m.get("name"), str) or not m["name"].strip():
        findings.warn("%s has no name" % where)
    if not isinstance(m.get("score_source"), str) or not m["score_source"].strip():
        findings.warn("%s has no score_source" % where)

    quants = m.get("quants")
    if not isinstance(quants, list) or not quants:
        findings.warn("%s has no quants" % where)
        return m
    checked = []
    for q in quants:
        if not isinstance(q, dict):
            findings.warn("%s: a quant is not an object; kept as-is" % where)
            checked.append(q)
            continue
        q = dict(q)
        if not isinstance(q.get("name"), str) or not q["name"].strip():
            findings.warn("%s: a quant has no name" % where)
        files = q.get("files")
        if not isinstance(files, list) or not files or not all(isinstance(f, str) for f in files):
            findings.warn("%s: quant %r has no file list" % (where, q.get("name")))
        if not isinstance(q.get("bytes"), int) or isinstance(q.get("bytes"), bool) or q["bytes"] < 0:
            findings.warn("%s: quant %r has no byte size" % (where, q.get("name")))
        checked.append(q)
    sizes = [q["bytes"] for q in checked if isinstance(q.get("bytes"), int)
             and not isinstance(q.get("bytes"), bool)]
    if len(sizes) == len(checked) and sizes != sorted(sizes):
        findings.warn("%s: quants are not sorted by size; sorted them in the merged file" % where)
        checked.sort(key=lambda q: q.get("bytes") or 0)
    m["quants"] = checked
    return m


def load_tier(path, findings):
    """Read one tier file, return (label, models) or None when unusable."""
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, ValueError) as e:
        findings.warn("%s: cannot read JSON (%s); skipped" % (path.name, e))
        return None
    if not isinstance(data, dict):
        findings.warn("%s: top level is %s, expected an object; skipped"
                      % (path.name, type(data).__name__))
        return None
    models = data.get("models")
    if not isinstance(models, list):
        findings.warn('%s: no "models" list; skipped' % path.name)
        return None
    label = data.get("tier")
    if not isinstance(label, str) or not label.strip():
        label = label_of(path)
        findings.warn('%s: no "tier" label, using "%s"' % (path.name, label))
    else:
        label = label.strip()
    if not models:
        findings.warn("%s: no models yet" % path.name)
    return label, models


def score_of(m):
    s = m.get("score")
    if isinstance(s, bool) or not isinstance(s, (int, float)):
        return 0.0
    return float(s)


def collect(tiers, findings, catalog_ids):
    """Merge the tier models, keeping the first entry for each id.

    Returns the kept models sorted by score, the duplicate ids, and the ids
    each tier file contributed to the output.
    """
    models, duplicates, seen, contributed = [], [], {}, {}
    for path, label, raws in tiers:
        contributed[path.name] = []
        for i, raw in enumerate(raws):
            m = check_entry(raw, path, i, findings)
            if m is None:
                continue
            mid = m["id"]
            if mid in seen:
                duplicates.append((mid, seen[mid], path.name))
                continue
            if mid in catalog_ids:
                findings.warn("%s: id already exists in internal/catalog/catalog.json" % mid)
            seen[mid] = path.name
            contributed[path.name].append(mid)
            models.append((m, label))
    models.sort(key=lambda pair: (-score_of(pair[0]), pair[0]["id"]))
    return models, duplicates, contributed


def tier_rows(tiers, kept, contributed):
    """One row per tier file: label, file, model count, quant count, score range."""
    by_id = {m["id"]: m for m, _ in kept}
    rows = []
    for path, label, _ in tiers:
        mine = [by_id[mid] for mid in contributed.get(path.name, [])]
        scores = [score_of(m) for m in mine]
        quants = sum(len(m.get("quants") or []) for m in mine)
        rng = "%.1f - %.1f" % (min(scores), max(scores)) if scores else "-"
        rows.append([label, path.name, str(len(mine)), str(quants), rng])
    return rows


def model_rows(kept):
    """One row per merged model, already sorted by score."""
    rows = []
    for rank, (m, label) in enumerate(kept, 1):
        quants = [q for q in (m.get("quants") or []) if isinstance(q, dict)]
        sized = [q for q in quants if isinstance(q.get("bytes"), int)
                 and not isinstance(q.get("bytes"), bool)]
        smallest = "-"
        if sized:
            q = min(sized, key=lambda q: q["bytes"])
            smallest = "%s %s" % (q.get("name", "?"), human_size(q["bytes"]))
        params = m.get("params_b")
        params = "%.1fB" % params if isinstance(params, (int, float)) and not isinstance(params, bool) else "-"
        active = "-"
        if m.get("moe") and isinstance(m.get("active_params_b"), (int, float)) \
                and not isinstance(m.get("active_params_b"), bool):
            active = "%.1fB" % m["active_params_b"]
        rows.append([str(rank), m["id"], "%.1f" % score_of(m), params, active,
                     label, str(len(quants)), smallest])
    return rows


def render(headers, rows, right=()):
    """Plain fixed-width table, no colour and no trailing spaces."""
    widths = [len(h) for h in headers]
    for row in rows:
        for i, cell in enumerate(row):
            widths[i] = max(widths[i], len(cell))
    out = []
    for row in [headers] + rows:
        cells = []
        for i, cell in enumerate(row):
            cells.append(cell.rjust(widths[i]) if i in right else cell.ljust(widths[i]))
        out.append("  ".join(cells).rstrip())
    return "\n".join(out)


def display_path(path):
    """A path relative to the current directory when that is shorter."""
    try:
        rel = os.path.relpath(path, Path.cwd())
    except ValueError:
        return str(path)
    return rel if len(rel) < len(str(path)) else str(path)


def find_tier_files(tiers_dir):
    if not tiers_dir.is_dir():
        return []
    files = sorted(tiers_dir.glob(TIER_GLOB), key=lambda p: tier_sort_key(label_of(p)))
    return files


def catalog_ids_of(path, findings):
    """Ids already in the shipped catalog, used to flag collisions."""
    if not path.is_file():
        return set()
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
        models = data.get("models")
        if not isinstance(models, list):
            findings.warn("%s: no models list; id collision check skipped" % path.name)
            return set()
        return {m.get("id") for m in models if isinstance(m, dict) and isinstance(m.get("id"), str)}
    except (OSError, ValueError) as e:
        findings.warn("%s: cannot read catalog for the id check (%s)" % (path.name, e))
        return set()


def write_output(path, kept):
    payload = {"version": 1, "models": [canonical_model(m) for m, _ in kept]}
    text = json.dumps(payload, indent=1, ensure_ascii=False) + "\n"
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, tmp = tempfile.mkstemp(dir=str(path.parent), prefix=".%s." % path.name, suffix=".tmp")
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as f:
            f.write(text)
        os.replace(tmp, path)
    except BaseException:
        try:
            os.unlink(tmp)
        except OSError:
            pass
        raise
    return len(text)


def main(argv=None):
    here = Path(__file__).resolve().parent
    repo = here.parents[1]
    ap = argparse.ArgumentParser(
        description="Merge agents/T02/tiers/tier-*.json into catalog-proposed.json.")
    ap.add_argument("--tiers", type=Path, default=here / "tiers",
                    help="directory of tier-*.json files (default: %(default)s)")
    ap.add_argument("--out", type=Path, default=here / OUT_NAME,
                    help="merged output file (default: %(default)s)")
    ap.add_argument("--catalog", type=Path, default=repo / "internal/catalog/catalog.json",
                    help="shipped catalog, used only to flag id collisions (default: %(default)s)")
    ap.add_argument("--strict", action="store_true", help="exit 1 when there are warnings")
    ap.add_argument("--dry-run", action="store_true", help="print the summary, write nothing")
    args = ap.parse_args(argv)

    findings = Findings()
    paths = find_tier_files(args.tiers)
    tiers = []
    for path in paths:
        loaded = load_tier(path, findings)
        if loaded is not None:
            tiers.append((path, loaded[0], loaded[1]))
    if not paths:
        findings.warn("no %s files in %s" % (TIER_GLOB, display_path(args.tiers)))

    kept, duplicates, contributed = collect(tiers, findings, catalog_ids_of(args.catalog, findings))
    total_quants = sum(len(m.get("quants") or []) for m, _ in kept)

    print("assemble: %d tier file(s)%s, %d model(s), %d quant(s), %d duplicate(s)"
          % (len(paths), "" if len(tiers) == len(paths) else " (%d usable)" % len(tiers),
             len(kept), total_quants, len(duplicates)))
    if not args.dry_run:
        print("out: %s" % display_path(args.out))
    print()

    if tiers:
        print(render(["tier", "file", "models", "quants", "score range"],
                     tier_rows(tiers, kept, contributed), right=(2, 3)))
        print()

    if kept:
        print(render(["rank", "id", "score", "params", "active", "tier", "quants",
                      "smallest quant"],
                     model_rows(kept), right=(0, 2, 3, 4, 6)))
        print()
    else:
        print("no models merged yet")
        print()

    if duplicates:
        print("duplicates (first one kept, the rest dropped)")
        rows = []
        for mid, kept_in, dropped_in in duplicates:
            rows.append([mid, kept_in, dropped_in])
        print(render(["id", "kept in", "also in"], rows))
        print()

    if findings.warnings:
        print("warnings (%d)" % len(findings.warnings))
        for i, message in enumerate(findings.warnings, 1):
            print(" %2d. %s" % (i, message))
        print()

    if args.dry_run:
        print("dry run: nothing written")
        return 0
    try:
        size = write_output(args.out, kept)
    except OSError as e:
        print("error: cannot write %s (%s)" % (display_path(args.out), e))
        return 1
    print("wrote %s (%d bytes)" % (display_path(args.out), size))
    if args.strict and findings.warnings:
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
