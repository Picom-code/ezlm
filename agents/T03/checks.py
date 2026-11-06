#!/usr/bin/env python3
"""Checks T03 matrix rows against llama.cpp b11960's chat template rules.

b11960 decides chat_template_caps by executing the template once and watching the
variables it reads (common/jinja/caps.cpp:132). Reasoning effort levels are only
listed when the template reads reasoning_effort and renders the level differently
from a sentinel render (common/jinja/caps.cpp:567-641); "none" never reaches the
template because the server turns it into enable_thinking=false
(tools/server/server-common.cpp:1497-1505). This script flags rows that claim
something those rules cannot produce.

  python3 agents/T03/checks.py [--models-dir agents/T03/models]

Exit code 0 when every row is consistent, 1 when a row contradicts the server.
Missing .renders.json / .templates.json files only reduce what can be checked.
"""
import argparse
import glob
import json
import os
import re
import sys

# levels b11960 probes, in order (common/jinja/caps.cpp:596). "none" is not one.
LEVELS = ["minimal", "low", "medium", "high", "xhigh", "max"]

ROW_REQUIRED = [
    "id", "repo", "template_source", "tool_call_format", "tools_supported",
    "thinking_toggle", "effort_levels_accepted", "effort_levels_rejected",
    "system_role", "developer_role", "known_template_errors", "notes",
]


def row_files(models_dir):
    out = []
    for path in sorted(glob.glob(os.path.join(models_dir, "*.json"))):
        name = os.path.basename(path)
        if name.endswith(".renders.json") or name.endswith(".templates.json"):
            continue
        if name == "matrix.json":
            continue
        out.append(path)
    return out


def load_json(path):
    try:
        with open(path, "r", encoding="utf-8") as f:
            return json.load(f)
    except (OSError, ValueError) as e:
        raise SystemExit("%s: %s" % (path, e))


def template_text(models_dir, model_id):
    path = os.path.join(models_dir, model_id + ".templates.json")
    if not os.path.exists(path):
        return None
    data = load_json(path)
    if not isinstance(data, dict) or not data:
        return None
    src = data.get("tokenizer.chat_template")
    if isinstance(src, str):
        return src
    for key in sorted(data):
        if key.startswith("tokenizer.chat_template") and isinstance(data[key], str):
            return data[key]
    return None


def renders(models_dir, model_id):
    path = os.path.join(models_dir, model_id + ".renders.json")
    if not os.path.exists(path):
        return None
    data = load_json(path)
    cases = {}
    for case in data.get("cases", []):
        if isinstance(case, dict) and "case" in case:
            cases[case["case"]] = case
    return cases


def check_row(row, path, models_dir):
    """Returns a list of (severity, message); severity is FAIL or WARN."""
    problems = []
    fail = lambda msg: problems.append(("FAIL", msg))
    warn = lambda msg: problems.append(("WARN", msg))

    model_id = row.get("id") or os.path.basename(path)[:-len(".json")]
    stem = os.path.basename(path)[:-len(".json")]
    if row.get("id") != stem:
        fail("id %r does not match file name %r" % (row.get("id"), stem))

    for key in ROW_REQUIRED:
        if key not in row:
            fail("missing required field %r" % key)

    accepted = row.get("effort_levels_accepted") or []
    rejected = row.get("effort_levels_rejected") or []
    if not isinstance(accepted, list) or not isinstance(rejected, list):
        fail("effort_levels_accepted/rejected must be lists")
        accepted, rejected = [], []

    # 1. only the levels b11960 probes can be accepted, and "none" is server-side.
    for level in accepted:
        if level == "none":
            fail("accepts effort 'none': b11960 handles none as enable_thinking=false "
                 "before the template (server-common.cpp:1497-1505), so it is not a "
                 "template level")
        elif level not in LEVELS:
            fail("accepts effort %r, which b11960 never probes (caps.cpp:596 probes %s)"
                 % (level, ", ".join(LEVELS)))
    for level in rejected:
        if level in accepted:
            fail("effort %r is both accepted and rejected" % level)

    src = template_text(models_dir, model_id)
    cases = renders(models_dir, model_id)

    if src is not None:
        reads_effort = bool(re.search(r"\breasoning_effort\b|\breasoning_strength\b", src))
        if accepted and not reads_effort:
            fail("accepts effort levels %s but the template never reads "
                 "reasoning_effort/reasoning_strength, so b11960 caps reports "
                 "supports_reasoning_effort=false (caps.cpp:567-592)"
                 % (accepted,))
        if row.get("tools_supported") and not re.search(r"\btools\b", src):
            fail("tools_supported=true but the template never reads 'tools'")
        if row.get("system_role") and "system" not in src:
            fail("system_role=true but the template never mentions a system role")
        if row.get("developer_role") and "developer" not in src:
            warn("developer_role=true but the template does not name 'developer'; "
                 "b11960 maps developer to system first (chat.cpp:1523)")
        toggle = str(row.get("thinking_toggle") or "none")
        if toggle != "none" and toggle not in src:
            fail("thinking_toggle=%r but the template never reads it" % toggle)
        if toggle == "none" and "enable_thinking" in src:
            fail("thinking_toggle='none' but the template reads enable_thinking")

    if cases is not None:
        baseline = None
        for key in ("system_user", "plain"):
            if cases.get(key, {}).get("ok"):
                baseline = cases[key]["output"]
                break
        if baseline is None:
            warn("no successful baseline render to compare effort levels against")
        else:
            distinct = {}
            for level in accepted:
                case = cases.get("effort_" + level)
                if not case:
                    fail("accepts effort %r but no effort_%s render was recorded" % (level, level))
                elif not case.get("ok"):
                    fail("accepts effort %r but the render raised: %s"
                         % (level, case.get("server_error") or case.get("error")))
                elif case.get("output") == baseline:
                    fail("accepts effort %r but it renders identically to the default; "
                         "b11960 caps drops it (caps.cpp:624-641)" % level)
                else:
                    distinct[level] = case["output"]
            for level in rejected:
                case = cases.get("effort_" + level)
                if case and case.get("ok"):
                    fail("rejects effort %r but the template rendered it without raising" % level)
            for level, out in distinct.items():
                for other, other_out in distinct.items():
                    if level < other and out == other_out:
                        fail("efforts %r and %r render identically; b11960 collapses "
                             "aliases to the highest (caps.cpp:626-641)" % (level, other))
            # underclaim: a level that renders differently but is not listed
            for level in LEVELS:
                case = cases.get("effort_" + level)
                if case and case.get("ok") and case.get("output") != baseline and level not in accepted:
                    fail("effort %r renders differently from the default but is not in "
                         "effort_levels_accepted" % level)
        for case_name in ("tool_call", "parallel_tools"):
            if row.get("tools_supported") and cases.get(case_name) and not cases[case_name].get("ok"):
                fail("tools_supported=true but %s raised: %s"
                     % (case_name, cases[case_name].get("server_error") or cases[case_name].get("error")))
        toggle = str(row.get("thinking_toggle") or "none")
        if toggle != "none":
            for case_name in ("thinking_on", "thinking_off"):
                if cases.get(case_name) and not cases[case_name].get("ok"):
                    fail("thinking toggle %r claimed but %s raised: %s"
                         % (toggle, case_name, cases[case_name].get("server_error") or cases[case_name].get("error")))
    else:
        warn("no .renders.json; effort levels were not verified against renders")

    return problems


def main():
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--models-dir", default=os.path.join("agents", "T03", "models"),
                    help="directory holding <id>.json rows (default: agents/T03/models)")
    args = ap.parse_args()

    files = row_files(args.models_dir)
    if not files:
        print("no matrix rows in %s" % args.models_dir)
        return 0

    failures = 0
    for path in files:
        row = load_json(path)
        problems = check_row(row, path, args.models_dir)
        model_id = row.get("id") or os.path.basename(path)[:-len(".json")]
        if not problems:
            print("ok   %s" % model_id)
            continue
        for severity, msg in problems:
            if severity == "FAIL":
                failures += 1
            print("%-4s %s: %s" % (severity, model_id, msg))

    print("%d row(s), %d failure(s)" % (len(files), failures))
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
