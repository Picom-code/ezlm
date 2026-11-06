#!/usr/bin/env python3
"""Validate EZLM catalog proposals for T02.

The proposal files are the tier files written by the T02 tier agents
(``agents/T02/tiers/tier-<NN>.json``) and the assembled
``agents/T02/catalog-proposed.json``. They use the schema of
``internal/catalog/catalog.json`` (see ``internal/catalog/catalog.go`` and
``agents/T02/SCHEMA.md``).

The checker is offline on purpose: it validates types, ranges, cross-field
rules, id uniqueness and quant ordering. Real file sizes and GGUF headers
still need ``scripts/verify_catalog.py``, which fetches them from Hugging Face.

Usage:
    python3 agents/T02/validate_proposal.py agents/T02/tiers/tier-08.json
    python3 agents/T02/validate_proposal.py agents/T02/tiers/tier-*.json
    python3 agents/T02/validate_proposal.py --lenient internal/catalog/catalog.json

Exit codes:
    0  every entry passed (warnings are allowed)
    1  at least one entry failed
    2  a file could not be read or parsed, or the arguments were invalid

Output is one PASS/FAIL line per entry, with each error and warning on its own
line. ``--json`` prints the same result as one JSON object.
"""

import argparse
import json
import math
import re
import sys
from pathlib import Path

# Tier labels accepted in a tier file's top-level "tier" field.
TIERS = {"4", "6", "8", "12", "16", "24", "32", "48", "64", "96", "128plus"}

# Draft types llama-server accepts (fit.specFor; catalog.go's comment lists
# only the first three, but fit.go also accepts draft-eagle3).
DRAFT_TYPES = {"draft-mtp", "draft-dflash", "draft-dspark", "draft-eagle3"}

REPO_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9][A-Za-z0-9._-]*$")
ID_RE = re.compile(r"^[a-z0-9]+(?:[.-][a-z0-9]+)*$")
COUNTRY_RE = re.compile(r"^[A-Z]{2}$")
DATE_RE = re.compile(r"\b(?:19|20)\d{2}\b")
ESTIMATE_RE = re.compile(r"(?i)^(?:UNVERIFIED\s+)?ESTIMATE\b")
SOURCE_RE = re.compile(
    r"(?i)(artificial\s*analysis|\bAA\b|lmarena|livebench|aider|swe-bench|"
    r"open\s*llm\s*leaderboard|terminal-bench|gpqa|mmlu|humaneval|bfcl|"
    r"tau-bench|tau2|ifeval|matharena|designarena|webdev|eq-bench|"
    r"hle\b|benchmark|leaderboard)"
)

# Fields that catalog.go decodes without omitempty. A missing one leaves a Go
# zero value that the fit planner uses, so they are hard errors.
CORE_FIELDS = {
    "id",
    "name",
    "family",
    "license",
    "params_b",
    "active_params_b",
    "moe",
    "score",
    "score_source",
    "layers",
    "kv_heads",
    "head_dim",
    "kv_layers",
    "ctx_max",
    "tool_calling",
    "repo",
    "quants",
}

# Fields the T02 brief asks every tier agent to fill even though catalog.go
# would default them (omitempty). Required in the default strict mode.
BRIEF_FIELDS = {"vendor", "country", "vision", "native_1bit", "notes"}

MODEL_FIELDS = (
    CORE_FIELDS
    | BRIEF_FIELDS
    | {"no_kv_quant", "draft", "swa_layers", "swa_kv_heads", "swa_head_dim", "swa_window"}
)
QUANT_FIELDS = {"name", "repo", "files", "bytes", "mtp", "quality", "lazy_bytes", "args"}
DRAFT_FIELDS = {"repo", "file", "bytes", "type"}
SWA_FIELDS = ("swa_layers", "swa_kv_heads", "swa_head_dim", "swa_window")

MAX_BYTES = 1 << 40  # 1 TiB: no single GGUF quant set is larger today.


def is_int(v):
    """True for a JSON integer, not for a bool (bool is an int in Python)."""
    return isinstance(v, int) and not isinstance(v, bool)


def is_num(v):
    """True for a JSON number, not for a bool."""
    return isinstance(v, (int, float)) and not isinstance(v, bool)


def fmt(v):
    """Short, readable rendering of a value for an error message."""
    s = repr(v)
    return s if len(s) <= 120 else s[:117] + "..."


def add_unknown(obj, allowed, where, errors, warnings, allow_extra):
    for key in sorted(set(obj) - allowed):
        msg = "%s: unknown field %r (not in catalog.go)" % (where, key)
        (warnings if allow_extra else errors).append(msg)


def check_string(value, where, errors, maxlen, allow_empty=False):
    """Type/emptiness/length check for a string field. Returns the string."""
    if not isinstance(value, str):
        errors.append("%s must be a string, got %s" % (where, type(value).__name__))
        return None
    if not value.strip():
        if not allow_empty:
            errors.append("%s must not be empty" % where)
        return value
    if len(value) > maxlen:
        errors.append("%s is %d chars, limit is %d" % (where, len(value), maxlen))
    if value != value.strip():
        errors.append("%s has leading or trailing whitespace: %s" % (where, fmt(value)))
    return value


def check_int(value, where, errors, lo=None, hi=None):
    if not is_int(value):
        errors.append("%s must be an integer, got %s" % (where, fmt(value)))
        return None
    if lo is not None and value < lo:
        errors.append("%s is %d, must be >= %d" % (where, value, lo))
    if hi is not None and value > hi:
        errors.append("%s is %d, must be <= %d" % (where, value, hi))
    return value


def check_num(value, where, errors, lo=None, hi=None):
    if not is_num(value):
        errors.append("%s must be a number, got %s" % (where, fmt(value)))
        return None
    if not math.isfinite(value):
        errors.append("%s must be a finite number, got %s" % (where, fmt(value)))
        return None
    if lo is not None and value < lo:
        errors.append("%s is %s, must be >= %s" % (where, fmt(value), lo))
    if hi is not None and value > hi:
        errors.append("%s is %s, must be <= %s" % (where, fmt(value), hi))
    return value


def check_repo(value, where, errors):
    """A repo is the bare Hugging Face id 'org/name', not a URL."""
    s = check_string(value, where, errors, 200)
    if not s:
        return s
    if not REPO_RE.match(s):
        errors.append("%s is %r; expected a Hugging Face repo like 'org/name'" % (where, s))
    low = s.lower()
    if "://" in s or low.startswith("huggingface.co/") or "resolve/" in low:
        errors.append(
            "%s is a URL; catalog.go builds the URL from repo + file, so store only 'org/name'"
            % where
        )
    if s.endswith("/") or s.startswith("/"):
        errors.append("%s must not start or end with '/'" % where)
    if s.endswith(".git"):
        errors.append("%s must not end with '.git'" % where)
    if any(c.isspace() for c in s):
        errors.append("%s contains whitespace" % where)
    return s


def check_file_path(value, where, errors):
    """Check one GGUF path as stored in the catalog (relative, .gguf)."""
    s = check_string(value, where, errors, 400)
    if not s:
        return s
    if s.startswith("/") or s.startswith("\\"):
        errors.append("%s is absolute; catalog files are relative to the repo root" % where)
    if "\\" in s:
        errors.append("%s uses backslashes; use '/' separators" % where)
    if ".." in s.split("/"):
        errors.append("%s contains '..'" % where)
    if any(part in ("", ".") for part in s.split("/")):
        errors.append("%s has an empty or '.' path component" % where)
    if not s.lower().endswith(".gguf"):
        errors.append("%s is not a .gguf file" % where)
    if "?" in s or "#" in s:
        errors.append("%s looks like a URL, not a repo-relative file name" % where)
    low = s.lower()
    if "://" in s or low.startswith("huggingface.co/") or "/resolve/" in low:
        errors.append("%s is a URL; store only the repo-relative file name" % where)
    if any(c.isspace() for c in s):
        errors.append("%s contains whitespace" % where)
    return s


def check_split_files(files, where, errors):
    """Split GGUF files must follow llama.cpp's -0000i-of-0000N naming."""
    if len(files) < 2:
        return
    first = files[0].rsplit("/", 1)[-1]
    m = re.search(r"-00001-of-(\d{5})\.gguf$", first)
    if not m:
        errors.append(
            "%s lists %d files but the first is %r, not '<name>-00001-of-%05d.gguf'"
            % (where, len(files), first, len(files))
        )
        return
    parts = int(m.group(1))
    if parts != len(files):
        errors.append(
            "%s first file says %05d parts but %d files are listed" % (where, parts, len(files))
        )
    for i, f in enumerate(files):
        base = f.rsplit("/", 1)[-1]
        want = first.replace("-00001-of-", "-%05d-of-" % (i + 1))
        if base != want:
            errors.append(
                "%s part %d is %r; llama.cpp looks for %r" % (where, i + 1, base, want)
            )
            break


def check_score_source(value, where, errors, warnings):
    s = check_string(value, where, errors, 2000)
    if not s:
        return
    if ESTIMATE_RE.match(s):
        if len(s) < 40:
            errors.append(
                "%s starts with ESTIMATE but does not explain the basis (too short)" % where
            )
        return
    if not DATE_RE.search(s):
        errors.append(
            "%s must name the source and the date (a 4-digit year), or start with ESTIMATE"
            % where
        )
    if not SOURCE_RE.search(s) and "http" not in s.lower():
        warnings.append("%s does not name a known benchmark source" % where)


def check_quant(q, i, model_id, params_b, opts, errors, warnings, names, files_seen):
    where = "%s: quants[%d]" % (model_id, i)
    if not isinstance(q, dict):
        errors.append("%s must be an object" % where)
        return None
    if "name" in q:
        name = check_string(q["name"], where + ".name", errors, 80)
        if name is not None:
            where = "%s (%s)" % (where, name)
            if name in names:
                errors.append("%s: duplicate quant name %r" % (where, name))
            else:
                names.add(name)
            if "MTP" in name.upper() and q.get("mtp") is not True:
                warnings.append("%s: name mentions MTP but mtp is not true" % where)
    else:
        errors.append("%s: missing required field 'name'" % where)

    if "files" in q:
        files = q["files"]
        if not isinstance(files, list):
            errors.append("%s.files must be a list" % where)
            files = []
        elif not files:
            errors.append("%s.files must not be empty" % where)
        for j, f in enumerate(files):
            check_file_path(f, "%s.files[%d]" % (where, j), errors)
        if files and all(isinstance(f, str) for f in files):
            check_split_files(files, where + ".files", errors)
            seen = set()
            for f in files:
                if f in seen:
                    errors.append("%s.files lists %r twice" % (where, f))
                seen.add(f)
                if f in files_seen:
                    warnings.append("%s.files %r is also used by %s" % (where, f, files_seen[f]))
                else:
                    files_seen[f] = where
    else:
        errors.append("%s: missing required field 'files'" % where)

    if "bytes" in q:
        b = check_int(q["bytes"], where + ".bytes", errors, 1, MAX_BYTES)
        if b is not None:
            if b < (1 << 20):
                errors.append("%s.bytes is %d, too small for a model file" % (where, b))
            elif b < (100 << 20):
                warnings.append("%s.bytes is %d, smaller than 100 MB" % (where, b))
            if params_b:
                ratio = b / (params_b * 1e9)
                if ratio < 0.10 or ratio > 3.0:
                    warnings.append(
                        "%s.bytes is %.2f bytes/param, outside the usual 0.10-3.0"
                        % (where, ratio)
                    )
    else:
        errors.append("%s: missing required field 'bytes'" % where)

    if "repo" in q:
        check_repo(q["repo"], where + ".repo", errors)
    if "mtp" in q and not isinstance(q["mtp"], bool):
        errors.append("%s.mtp must be a boolean" % where)
    if "quality" in q:
        check_num(q["quality"], where + ".quality", errors, hi=1)
        if is_num(q["quality"]) and q["quality"] <= 0:
            errors.append("%s.quality must be > 0 (omit it to use the default)" % where)
    if "lazy_bytes" in q:
        lb = check_int(q["lazy_bytes"], where + ".lazy_bytes", errors, 0)
        if lb is not None and is_int(q.get("bytes")) and lb >= q["bytes"]:
            errors.append(
                "%s.lazy_bytes %d must be smaller than bytes %d" % (where, lb, q["bytes"])
            )
    if "args" in q:
        args = q["args"]
        if not isinstance(args, list):
            errors.append("%s.args must be a list of strings" % where)
        else:
            for j, a in enumerate(args):
                check_string(a, "%s.args[%d]" % (where, j), errors, 200)

    add_unknown(q, QUANT_FIELDS, where, errors, warnings, opts.allow_extra)
    return q


def check_draft(d, model_id, errors, warnings, opts):
    where = model_id + ": draft"
    if not isinstance(d, dict):
        errors.append("%s must be an object" % where)
        return
    for f in sorted(DRAFT_FIELDS):
        if f not in d:
            errors.append("%s: missing required field %r" % (where, f))
    if "repo" in d:
        check_repo(d["repo"], where + ".repo", errors)
    if "file" in d:
        check_file_path(d["file"], where + ".file", errors)
    if "bytes" in d:
        check_int(d["bytes"], where + ".bytes", errors, 1, MAX_BYTES)
    if "type" in d:
        t = check_string(d["type"], where + ".type", errors, 40)
        if t and t not in DRAFT_TYPES:
            errors.append(
                "%s.type is %r; allowed: %s" % (where, t, ", ".join(sorted(DRAFT_TYPES)))
            )
    add_unknown(d, DRAFT_FIELDS, where, errors, warnings, opts.allow_extra)


def validate_model(m, index, opts, seen_ids, existing_ids, existing_is_input):
    errors = []
    warnings = []
    if not isinstance(m, dict):
        return {
            "index": index,
            "id": "",
            "pass": False,
            "errors": ["entry[%d]: model must be an object, got %s" % (index, type(m).__name__)],
            "warnings": [],
        }

    mid = m.get("id")
    label = mid if isinstance(mid, str) and mid.strip() else "entry[%d]" % index

    required = set(CORE_FIELDS)
    if not opts.lenient:
        required |= BRIEF_FIELDS
    for f in sorted(required):
        if f not in m:
            errors.append("%s: missing required field %r" % (label, f))

    add_unknown(m, MODEL_FIELDS, label, errors, warnings, opts.allow_extra)

    if "id" in m:
        s = check_string(mid, label + ".id", errors, 64)
        if s:
            if not ID_RE.match(s):
                errors.append(
                    "%s.id is %r; expected lowercase words separated by '-' or '.', "
                    "e.g. 'gemma-4-12b'" % (label, s)
                )
            if s in seen_ids:
                errors.append("%s: duplicate id (first seen at entry[%d])" % (label, seen_ids[s]))
            else:
                seen_ids[s] = index
            if existing_ids and not existing_is_input and s in existing_ids:
                errors.append(
                    "%s: id %r already exists in internal/catalog/catalog.json" % (label, s)
                )

    if "name" in m:
        check_string(m["name"], label + ".name", errors, 80)
    if "family" in m:
        check_string(m["family"], label + ".family", errors, 80)
    if "vendor" in m:
        check_string(m["vendor"], label + ".vendor", errors, 80)
    if "country" in m:
        s = check_string(m["country"], label + ".country", errors, 2)
        if s and not COUNTRY_RE.match(s):
            errors.append("%s.country is %r; expected ISO 3166-1 alpha-2, e.g. 'US'" % (label, s))
    if "license" in m:
        s = check_string(m["license"], label + ".license", errors, 200)
        if s and "UNVERIFIED" in s.upper():
            warnings.append("%s.license is marked UNVERIFIED" % label)

    params_b = None
    if "params_b" in m:
        params_b = check_num(m["params_b"], label + ".params_b", errors, hi=10000)
        if params_b is not None and params_b <= 0:
            errors.append("%s.params_b must be > 0" % label)
    active = None
    if "active_params_b" in m:
        active = check_num(m["active_params_b"], label + ".active_params_b", errors, hi=10000)
        if active is not None and active <= 0:
            errors.append("%s.active_params_b must be > 0" % label)

    moe = m.get("moe")
    if "moe" in m and not isinstance(moe, bool):
        errors.append("%s.moe must be a boolean" % label)
        moe = None
    if params_b is not None and active is not None:
        if active > params_b * (1 + 1e-9):
            errors.append(
                "%s.active_params_b %s is larger than params_b %s"
                % (label, fmt(active), fmt(params_b))
            )
        elif moe is True and active >= params_b:
            errors.append(
                "%s.moe is true but active_params_b %s is not smaller than params_b %s"
                % (label, fmt(active), fmt(params_b))
            )

    if "score" in m:
        check_num(m["score"], label + ".score", errors, hi=100)
        if is_num(m["score"]) and m["score"] <= 0:
            errors.append("%s.score must be > 0" % label)
    if "score_source" in m:
        check_score_source(m["score_source"], label + ".score_source", errors, warnings)

    layers = None
    if "layers" in m:
        layers = check_int(m["layers"], label + ".layers", errors, 1, 1000)
    if "kv_heads" in m:
        check_int(m["kv_heads"], label + ".kv_heads", errors, 1, 512)
    if "head_dim" in m:
        check_int(m["head_dim"], label + ".head_dim", errors, 1, 4096)
    kv_layers = None
    if "kv_layers" in m:
        kv_layers = check_int(m["kv_layers"], label + ".kv_layers", errors, 1, 1000)
        if kv_layers is not None and layers is not None and kv_layers > layers:
            errors.append(
                "%s.kv_layers %d is larger than layers %d" % (label, kv_layers, layers)
            )
    ctx = None
    if "ctx_max" in m:
        ctx = check_int(m["ctx_max"], label + ".ctx_max", errors, 2048, 1 << 24)
        if ctx is not None and ctx >= 2048 and ctx % 1024:
            warnings.append("%s.ctx_max %d is not a multiple of 1024" % (label, ctx))
    if "tool_calling" in m and not isinstance(m["tool_calling"], bool):
        errors.append("%s.tool_calling must be a boolean" % label)
    if "repo" in m:
        check_repo(m["repo"], label + ".repo", errors)
    for f in ("vision", "native_1bit", "no_kv_quant"):
        if f in m and not isinstance(m[f], bool):
            errors.append("%s.%s must be a boolean" % (label, f))
    if "notes" in m:
        check_string(m["notes"], label + ".notes", errors, 4000)

    if "quants" in m:
        quants = m["quants"]
        if not isinstance(quants, list):
            errors.append("%s.quants must be a list" % label)
        elif not quants:
            errors.append("%s.quants must not be empty" % label)
        else:
            names = set()
            files_seen = {}
            prev = None
            for i, q in enumerate(quants):
                check_quant(q, i, label, params_b, opts, errors, warnings, names, files_seen)
                if isinstance(q, dict) and is_int(q.get("bytes")):
                    if prev is not None and q["bytes"] < prev:
                        errors.append(
                            "%s: quants[%d].bytes %d is smaller than quants[%d].bytes %d "
                            "(quants must be sorted by size, smallest first)"
                            % (label, i, q["bytes"], i - 1, prev)
                        )
                    prev = q["bytes"]

    if "draft" in m:
        check_draft(m["draft"], label, errors, warnings, opts)

    swa_present = [f for f in SWA_FIELDS if f in m]
    if swa_present:
        for f in SWA_FIELDS:
            if f not in m:
                errors.append("%s: sliding-window fields must be set together; missing %r" % (label, f))
        swa_layers = swa_win = None
        if "swa_layers" in m:
            swa_layers = check_int(m["swa_layers"], label + ".swa_layers", errors, 1, 1000)
        if "swa_kv_heads" in m:
            check_int(m["swa_kv_heads"], label + ".swa_kv_heads", errors, 1, 512)
        if "swa_head_dim" in m:
            check_int(m["swa_head_dim"], label + ".swa_head_dim", errors, 1, 4096)
        if "swa_window" in m:
            swa_win = check_int(m["swa_window"], label + ".swa_window", errors, 128, 1 << 24)
        if swa_layers is not None and layers is not None and swa_layers > layers:
            errors.append("%s.swa_layers %d is larger than layers %d" % (label, swa_layers, layers))
        if swa_layers is not None and kv_layers is not None and layers is not None:
            if swa_layers + kv_layers > layers:
                errors.append(
                    "%s.swa_layers + kv_layers = %d, larger than layers %d"
                    % (label, swa_layers + kv_layers, layers)
                )
        if swa_win is not None and ctx is not None and swa_win > ctx:
            errors.append("%s.swa_window %d is larger than ctx_max %d" % (label, swa_win, ctx))

    return {
        "index": index,
        "id": label,
        "pass": not errors,
        "errors": errors,
        "warnings": warnings,
    }


def fatal(msg):
    print(msg, file=sys.stderr)
    raise SystemExit(2)


def load_json(path):
    try:
        text = path.read_text(encoding="utf-8")
    except OSError as e:
        fatal("error: cannot read %s: %s" % (path, e))
    try:
        return json.loads(text)
    except json.JSONDecodeError as e:
        fatal("error: %s is not valid JSON: %s" % (path, e))


def load_existing_ids(path, explicit):
    """Return (ids, version) of the current catalog, or (None, None)."""
    try:
        doc = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as e:
        if explicit:
            fatal("error: cannot read existing catalog %s: %s" % (path, e))
        return None, None
    if not isinstance(doc, dict) or not isinstance(doc.get("models"), list):
        if explicit:
            fatal("error: %s has no top-level 'models' list" % path)
        return None, None
    ids = set()
    for m in doc["models"]:
        if isinstance(m, dict) and isinstance(m.get("id"), str):
            ids.add(m["id"])
    version = doc.get("version") if is_int(doc.get("version")) else None
    return ids, version


def find_existing(args):
    """Return (path, ids, version) for the current catalog, or (None, None, None)."""
    if args.no_existing:
        return None, None, None
    if args.existing:
        p = Path(args.existing)
        if not p.is_file():
            fatal("error: --existing %s is not a file" % p)
        ids, version = load_existing_ids(p, True)
        return p, ids, version
    here = Path(__file__).resolve()
    candidates = [
        Path.cwd() / "internal/catalog/catalog.json",
        here.parents[2] / "internal/catalog/catalog.json",
    ]
    for p in candidates:
        if p.is_file():
            ids, version = load_existing_ids(p, False)
            return p, ids, version
    return None, None, None


def validate_file(path, opts, existing_ids, existing_version):
    doc = load_json(path)
    file_errors = []
    file_warnings = []
    tier = None
    version = None

    if isinstance(doc, list):
        models = doc
        file_warnings.append("%s: top level is a list; expected an object with a 'models' key" % path)
    elif isinstance(doc, dict):
        models = doc.get("models")
        if "tier" in doc:
            t = doc["tier"]
            if not isinstance(t, str):
                file_errors.append("%s: top-level 'tier' must be a string" % path)
            else:
                tier = t
                if t not in TIERS:
                    file_errors.append(
                        "%s: tier %r is not one of %s" % (path, t, ", ".join(sorted(TIERS)))
                    )
        if "version" in doc:
            v = doc["version"]
            if not is_int(v) or v < 1:
                file_errors.append("%s: top-level 'version' must be an integer >= 1" % path)
            else:
                version = v
        for key in sorted(set(doc) - {"version", "models", "tier", "tools"}):
            file_warnings.append("%s: unknown top-level field %r" % (path, key))
        if tier is None and version is None:
            file_warnings.append(
                "%s: neither 'tier' nor 'version' is set; is this a tier file?" % path
            )
    else:
        file_errors.append("%s: top level must be an object or a list" % path)
        models = None

    if models is None:
        file_errors.append("%s: missing top-level 'models' list" % path)
        models = []
    elif not isinstance(models, list):
        file_errors.append("%s: 'models' must be a list" % path)
        models = []
    elif not models:
        file_errors.append("%s: 'models' is empty" % path)

    m = re.match(r"^tier-(4|6|8|12|16|24|32|48|64|96|128plus)\.json$", path.name)
    if tier and m and m.group(1) != tier:
        file_warnings.append(
            "%s: file name says tier %r but the 'tier' field says %r"
            % (path, m.group(1), tier)
        )

    existing_is_input = False
    if existing_ids:
        try:
            existing_is_input = path.resolve() == Path(opts.existing_path).resolve()
        except OSError:
            existing_is_input = False

    # catalog.Load only accepts a remote catalog when its version is higher
    # than the embedded one (catalog.go:174); a proposal must be bumped.
    if version is not None and existing_version is not None and not existing_is_input:
        if version <= existing_version:
            file_warnings.append(
                "%s: version %d is not higher than the embedded catalog's %d; "
                "catalog.Load ignores it (catalog.go:174)" % (path, version, existing_version)
            )

    seen_ids = {}
    results = [
        validate_model(model, i, opts, seen_ids, existing_ids, existing_is_input)
        for i, model in enumerate(models)
    ]

    return {
        "path": str(path),
        "tier": tier,
        "version": version,
        "results": results,
        "file_errors": file_errors,
        "file_warnings": file_warnings,
    }


def print_text(report, show_warnings=True):
    print("\n%s%s" % (report["path"], "  (tier %s)" % report["tier"] if report["tier"] else ""))
    for w in report["file_warnings"]:
        print("  warn: %s" % w)
    for e in report["file_errors"]:
        print("  error: %s" % e)
    for r in report["results"]:
        status = "PASS" if r["pass"] else "FAIL"
        print("  [%s] %s" % (status, r["id"]))
        if show_warnings:
            for w in r["warnings"]:
                print("         warn: %s" % w)
        for e in r["errors"]:
            print("         error: %s" % e)
    n = len(report["results"])
    passed = sum(1 for r in report["results"] if r["pass"])
    failed = n - passed
    warns = sum(len(r["warnings"]) for r in report["results"]) + len(report["file_warnings"])
    print(
        "  %d entr%s: %d passed, %d failed, %d warning%s"
        % (n, "y" if n == 1 else "ies", passed, failed, warns, "" if warns == 1 else "s")
    )
    if report["file_errors"]:
        print("  file errors: %d" % len(report["file_errors"]))


def main(argv=None):
    ap = argparse.ArgumentParser(
        description="Validate EZLM catalog proposals against internal/catalog/catalog.go.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__.split("Usage:", 1)[1] if "Usage:" in __doc__ else None,
    )
    ap.add_argument("paths", nargs="+", metavar="FILE", help="proposal JSON file(s)")
    ap.add_argument(
        "--lenient",
        action="store_true",
        help="only require fields catalog.go decodes; vendor/country/vision/native_1bit/notes "
        "become optional (used to audit the current catalog.json)",
    )
    ap.add_argument(
        "--allow-extra",
        action="store_true",
        help="downgrade unknown fields from errors to warnings",
    )
    ap.add_argument(
        "--existing",
        metavar="PATH",
        help="catalog whose ids proposals must not collide with "
        "(default: internal/catalog/catalog.json, auto-detected)",
    )
    ap.add_argument("--no-existing", action="store_true", help="skip the id collision check")
    ap.add_argument("--json", action="store_true", help="print machine-readable JSON")
    args = ap.parse_args(argv)

    existing_path, existing_ids, existing_version = find_existing(args)
    args.existing_path = str(existing_path) if existing_path else ""

    reports = []
    for raw in args.paths:
        p = Path(raw)
        if not p.is_file():
            fatal("error: %s is not a file" % p)
        reports.append(validate_file(p, args, existing_ids, existing_version))

    ok = all(
        not r["file_errors"] and all(m["pass"] for m in r["results"]) for r in reports
    )

    if args.json:
        out = {
            "ok": ok,
            "existing_catalog": args.existing_path or None,
            "files": [
                {
                    "path": r["path"],
                    "tier": r["tier"],
                    "version": r["version"],
                    "file_errors": r["file_errors"],
                    "file_warnings": r["file_warnings"],
                    "models": r["results"],
                }
                for r in reports
            ],
        }
        print(json.dumps(out, indent=2))
    else:
        if existing_path:
            print(
                "existing catalog: %s (%d ids)"
                % (existing_path, len(existing_ids or ()))
            )
        for r in reports:
            print_text(r)
        total = sum(len(r["results"]) for r in reports)
        passed = sum(1 for r in reports for m in r["results"] if m["pass"])
        failed = total - passed
        file_errors = sum(len(r["file_errors"]) for r in reports)
        print(
            "\n%d file%s, %d entr%s: %d passed, %d failed, %d file error%s"
            % (
                len(reports),
                "" if len(reports) == 1 else "s",
                total,
                "y" if total == 1 else "ies",
                passed,
                failed,
                file_errors,
                "" if file_errors == 1 else "s",
            )
        )
        print("OK" if ok else "FAILED")
    return 0 if ok else 1


if __name__ == "__main__":
    try:
        sys.exit(main())
    except SystemExit:
        raise
    except BrokenPipeError:
        sys.exit(1)
