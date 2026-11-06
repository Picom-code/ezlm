#!/usr/bin/env python3
"""Render a model's GGUF chat template with jinja2, using llama.cpp's conventions.

Reads agents/T03/models/<id>.templates.json (written by fetch_templates.py),
renders the case list below and writes agents/T03/models/<id>.renders.json.

  ~/ezlm-agents/run/venv/bin/python agents/T03/a09/render.py [id ...]
"""
import json, os, re, sys
from jinja2 import Environment, Undefined
from jinja2.exceptions import TemplateError

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))  # agents/T03
MODELS = os.path.join(ROOT, "models")

BOS, EOS = "<bos>", "<eos>"


class TemplateRaise(Exception):
    """What a template's raise_exception() call turns into."""


def raise_exception(msg):
    raise TemplateRaise(str(msg))


def strftime_now(fmt):
    import datetime
    return datetime.datetime.now().strftime(fmt)


WEATHER = {
    "type": "function",
    "function": {
        "name": "get_weather",
        "description": "Get the current weather for a city.",
        "parameters": {
            "type": "object",
            "properties": {
                "city": {"type": "string", "description": "City name."},
                "unit": {"type": "string", "enum": ["celsius", "fahrenheit"]},
            },
            "required": ["city"],
        },
    },
}

TIME = {
    "type": "function",
    "function": {
        "name": "get_time",
        "description": "Get the current time in a city.",
        "parameters": {
            "type": "object",
            "properties": {"city": {"type": "string", "description": "City name."}},
            "required": ["city"],
        },
    },
}

USER = [{"role": "user", "content": "What is the weather in Paris?"}]
SYSTEM_USER = [{"role": "system", "content": "You are a helpful assistant."}] + USER
TOOL_CALL_MSG = {
    "role": "assistant",
    "content": "",
    "tool_calls": [
        {
            "id": "call_1",
            "type": "function",
            "function": {"name": "get_weather", "arguments": {"city": "Paris", "unit": "celsius"}},
        }
    ],
}
TOOL_RESULT = {"role": "tool", "tool_call_id": "call_1", "name": "get_weather",
               "content": "18 degrees and clear."}
PARALLEL_MSG = {
    "role": "assistant",
    "content": "",
    "tool_calls": [
        {"id": "call_1", "type": "function",
         "function": {"name": "get_weather", "arguments": {"city": "Paris"}}},
        {"id": "call_2", "type": "function",
         "function": {"name": "get_time", "arguments": {"city": "Paris"}}},
    ],
}
PARALLEL_RESULTS = [
    {"role": "tool", "tool_call_id": "call_1", "name": "get_weather", "content": "18 degrees and clear."},
    {"role": "tool", "tool_call_id": "call_2", "name": "get_time", "content": "09:30 local time."},
]
DEVELOPER = [{"role": "developer", "content": "Answer in one sentence."}] + USER

BASE = {"bos_token": BOS, "eos_token": EOS, "add_generation_prompt": True, "tools": None,
        "raise_exception": raise_exception, "strftime_now": strftime_now}

# name -> (messages, extra kwargs, one-line description)
CASES = [
    ("plain", USER, {}, "one user message"),
    ("system_user", SYSTEM_USER, {}, "system plus user"),
    ("tool_call", USER + [TOOL_CALL_MSG, TOOL_RESULT], {"tools": [WEATHER]},
     "tool definition, assistant tool call, tool result"),
    ("tool_call_string_args", USER + [
        {"role": "assistant", "content": "", "tool_calls": [
            {"id": "call_1", "type": "function",
             "function": {"name": "get_weather", "arguments": '{"city": "Paris"}'}}]},
        TOOL_RESULT], {"tools": [WEATHER]},
     "same, but arguments is an OpenAI JSON string"),
    ("parallel_tools", USER + [PARALLEL_MSG] + PARALLEL_RESULTS, {"tools": [WEATHER, TIME]},
     "two tool calls in one assistant message, two results"),
    ("thinking_on", USER, {"enable_thinking": True}, "enable_thinking true"),
    ("thinking_off", USER, {"enable_thinking": False}, "enable_thinking false"),
    ("thinking_var_on", USER, {"thinking": True}, "the `thinking` variable instead"),
    ("thinking_var_off", USER, {"thinking": False}, "the `thinking` variable instead"),
    ("gen_prompt_off", USER, {"add_generation_prompt": False}, "no generation prompt"),
    ("effort_none", USER, {"reasoning_effort": "none"}, "reasoning_effort=none"),
    ("effort_low", USER, {"reasoning_effort": "low"}, "reasoning_effort=low"),
    ("effort_medium", USER, {"reasoning_effort": "medium"}, "reasoning_effort=medium"),
    ("effort_high", USER, {"reasoning_effort": "high"}, "reasoning_effort=high"),
    ("effort_xhigh", USER, {"reasoning_effort": "xhigh"}, "reasoning_effort=xhigh"),
    ("effort_max", USER, {"reasoning_effort": "max"}, "reasoning_effort=max"),
    ("effort_nested", USER, {"reasoning": {"effort": "high"}}, "reasoning.effort=high"),
    ("effort_thinking_nested", USER, {"thinking": {"effort": "high"}}, "thinking.effort=high"),
    ("effort_with_thinking_on", USER, {"enable_thinking": True, "reasoning_effort": "high"},
     "enable_thinking true plus reasoning_effort=high"),
    ("developer_role", DEVELOPER, {}, "a developer role message"),
]

# The effort/thinking values a template could read, in llama.cpp's shapes.
IGNORED = ("reasoning_effort", "reasoning", "thinking")


def strip_generation(src):
    """jinja2 has no {% generation %} tag; llama.cpp's minja may. Drop the tags."""
    return re.sub(r"\{%-?\s*(?:end)?generation\s*-?%\}", "", src)


def render(env, src, messages, extra):
    tpl = env.from_string(src)
    ctx = dict(BASE)
    ctx.update(extra)
    ctx["messages"] = messages
    return tpl.render(**ctx)


def run(mid):
    tpath = os.path.join(MODELS, mid + ".templates.json")
    spec = json.load(open(tpath))
    src = spec["tokenizer.chat_template"]
    stripped = strip_generation(src)
    env = Environment(undefined=Undefined, extensions=["jinja2.ext.loopcontrols"])
    out = {
        "id": mid,
        "repo": spec["repo"],
        "file": spec["file"],
        "url": spec["url"],
        "template_source": "tokenizer.chat_template",
        "named_templates": sorted(spec["named_templates"]),
        "jinja2_version": __import__("jinja2").__version__,
        "environment": "jinja2, trim_blocks=False, lstrip_blocks=False, default Undefined (like minja: missing keys are falsy)",
        "preprocessing": [],
        "cases": {},
    }
    if stripped != src:
        out["preprocessing"].append(
            "removed {% generation %} / {% endgeneration %} tags: jinja2 has no such tag")

    # baseline for "did the input change the output" checks
    baseline, baseline_err = None, None
    try:
        baseline = render(env, stripped, USER, {})
    except Exception as e:  # noqa: BLE001 - recorded as data
        baseline_err = f"{type(e).__name__}: {e}"

    for name, messages, extra, desc in CASES:
        rec = {"description": desc, "kwargs": sorted(k for k in extra if k != "tools"),
               "tools": [t["function"]["name"] for t in extra.get("tools") or []]}
        try:
            text = render(env, stripped, messages, extra)
            rec["ok"] = True
            rec["output"] = text
            if baseline is not None and messages is USER:
                rec["same_as_plain_baseline"] = (text == baseline)
        except TemplateError as e:
            rec["ok"] = False
            rec["error"] = f"{type(e).__name__}: {e}"
        except TemplateRaise as e:
            rec["ok"] = False
            rec["error"] = f"raise_exception: {e}"
        except Exception as e:  # noqa: BLE001
            rec["ok"] = False
            rec["error"] = f"{type(e).__name__}: {e}"
        out["cases"][name] = rec

    # Which of the variables the template actually reads?
    out["variables_read"] = sorted(set(
        m for m in re.findall(r"[A-Za-z_][A-Za-z0-9_]*", stripped)
        if m in ("enable_thinking", "reasoning_effort", "reasoning", "thinking",
                 "preserve_thinking", "add_generation_prompt", "tools", "bos_token",
                 "eos_token", "messages", "developer")
    ))
    out["baseline_error"] = baseline_err
    open(os.path.join(MODELS, mid + ".renders.json"), "w").write(json.dumps(out, indent=1) + "\n")
    ok = sum(1 for c in out["cases"].values() if c["ok"])
    print(f"{mid}: {ok}/{len(out['cases'])} rendered, vars={out['variables_read']}")
    for name, rec in out["cases"].items():
        if not rec["ok"]:
            print(f"  FAIL {name}: {rec['error'][:160]}")
    return out


if __name__ == "__main__":
    ids = sys.argv[1:] or ["gemma-4-12b", "lfm3-1.7b"]
    for i in ids:
        run(i)
