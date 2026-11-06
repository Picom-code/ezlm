#!/usr/bin/env python3
"""Renders a model's GGUF chat template with jinja2, the way llama.cpp b11960
does (messages/tools/add_generation_prompt plus the template's own variables,
and a raise_exception stub that raises). Writes agents/T03/models/<id>.renders.json.

  agents/T03/a02/render.py <id> [<id> ...]
"""
import json
import sys
from pathlib import Path

import jinja2

ROOT = Path(__file__).resolve().parents[1]  # agents/T03
MODELS = ROOT / "models"


class TemplateError(Exception):
    """Mirrors llama.cpp's jinja raise_exception: the message reaches the client."""


def raise_exception(msg):
    raise TemplateError(str(msg))


def env_for(src):
    env = jinja2.Environment(undefined=jinja2.Undefined, autoescape=False)
    env.globals["raise_exception"] = raise_exception
    # llama.cpp exposes these; templates that use them must not fail for unrelated reasons.
    env.globals["strftime_now"] = lambda fmt="%Y-%m-%d": "2026-11-06"
    env.globals["tojson"] = env.filters["tojson"]
    return env.from_string(src)


TOOL = {
    "type": "function",
    "function": {
        "name": "get_weather",
        "description": "Get the current weather for a city.",
        "parameters": {
            "type": "object",
            "properties": {"city": {"type": "string", "description": "City name"}},
            "required": ["city"],
        },
    },
}
TOOL2 = {
    "type": "function",
    "function": {
        "name": "get_time",
        "description": "Get the current local time for a city.",
        "parameters": {
            "type": "object",
            "properties": {"city": {"type": "string"}},
            "required": ["city"],
        },
    },
}

CALL = {
    "id": "call_1",
    "type": "function",
    "function": {"name": "get_weather", "arguments": {"city": "Paris"}},
}
CALL2 = {
    "id": "call_2",
    "type": "function",
    "function": {"name": "get_time", "arguments": {"city": "Paris"}},
}

PLAIN = [{"role": "user", "content": "What is the weather in Paris?"}]

SYSTEM_USER = [
    {"role": "system", "content": "You are a terse weather assistant."},
    {"role": "user", "content": "What is the weather in Paris?"},
]

TOOL_CALL = [
    {"role": "user", "content": "What is the weather in Paris?"},
    {"role": "assistant", "content": "", "tool_calls": [CALL]},
    {"role": "tool", "content": '{"temp_c": 14, "sky": "cloudy"}', "tool_call_id": "call_1"},
]

PARALLEL_TOOLS = [
    {"role": "user", "content": "Weather and local time in Paris?"},
    {"role": "assistant", "content": "", "tool_calls": [CALL, CALL2]},
    {"role": "tool", "content": '{"temp_c": 14}', "tool_call_id": "call_1"},
    {"role": "tool", "content": '{"time": "12:00"}', "tool_call_id": "call_2"},
]

DEVELOPER = [
    {"role": "developer", "content": "You are a terse weather assistant."},
    {"role": "user", "content": "What is the weather in Paris?"},
]


def cases():
    out = [
        ("plain", {"messages": PLAIN, "add_generation_prompt": True}),
        ("system_user", {"messages": SYSTEM_USER, "add_generation_prompt": True}),
        ("tool_call", {"messages": TOOL_CALL, "tools": [TOOL], "add_generation_prompt": True}),
        ("parallel_tools", {"messages": PARALLEL_TOOLS, "tools": [TOOL, TOOL2], "add_generation_prompt": True}),
        ("thinking_on", {"messages": PLAIN, "add_generation_prompt": True, "enable_thinking": True}),
        ("thinking_off", {"messages": PLAIN, "add_generation_prompt": True, "enable_thinking": False}),
    ]
    for level in ("none", "low", "medium", "high", "xhigh", "max"):
        out.append((
            "effort_" + level,
            {"messages": PLAIN, "add_generation_prompt": True, "enable_thinking": True,
             "reasoning_effort": level},
        ))
    out.append(("developer_role", {"messages": DEVELOPER, "add_generation_prompt": True}))
    # Extra probes: shapes llama.cpp leaves to chat_template_kwargs, and the
    # "thinking" spelling some templates use instead of enable_thinking.
    out.append(("probe_thinking_var_true", {"messages": PLAIN, "add_generation_prompt": True, "thinking": True}))
    out.append(("probe_thinking_var_false", {"messages": PLAIN, "add_generation_prompt": True, "thinking": False}))
    out.append(("probe_effort_obj_low", {"messages": PLAIN, "add_generation_prompt": True,
                                         "reasoning": {"effort": "low"}}))
    out.append(("probe_thinking_obj_low", {"messages": PLAIN, "add_generation_prompt": True,
                                            "thinking": {"effort": "low"}}))
    out.append(("probe_add_gen_false", {"messages": PLAIN, "add_generation_prompt": False}))
    out.append(("probe_system_user_tools", {"messages": SYSTEM_USER, "tools": [TOOL],
                                            "add_generation_prompt": True}))
    out.append(("probe_system_mid", {"messages": [
        {"role": "user", "content": "Hi"},
        {"role": "system", "content": "Be terse."},
        {"role": "user", "content": "Weather in Paris?"},
    ], "add_generation_prompt": True}))
    out.append(("probe_no_user", {"messages": [
        {"role": "assistant", "content": "Hi, ask me about the weather."},
    ], "add_generation_prompt": True}))
    out.append(("probe_empty_messages", {"messages": [], "add_generation_prompt": True}))
    out.append(("probe_unknown_role", {"messages": [
        {"role": "user", "content": "Hi"},
        {"role": "narrator", "content": "The wind rose."},
    ], "add_generation_prompt": True}))
    return out


def render_one(src, kwargs):
    tmpl = env_for(src)
    try:
        text = tmpl.render(**kwargs)
        return {"ok": True, "output": text}
    except TemplateError as e:
        return {"ok": False, "error": str(e), "error_type": "raise_exception"}
    except Exception as e:  # jinja2 UndefinedError, TypeError, ...
        return {"ok": False, "error": f"{type(e).__name__}: {e}", "error_type": "jinja2"}


def main(ids):
    for mid in ids:
        data = json.loads((MODELS / f"{mid}.templates.json").read_text())
        templates = data["templates"]
        rows = {}
        for name, src in sorted(templates.items()):
            case_results = {}
            for case, kwargs in cases():
                case_results[case] = render_one(src, kwargs)
            rows[name] = case_results
        out = {"id": mid, "source": data["url"], "cases": rows}
        (MODELS / f"{mid}.renders.json").write_text(json.dumps(out, indent=1) + "\n")
        passed = sum(1 for c in rows.values() for r in c.values() if r["ok"])
        total = sum(len(c) for c in rows.values())
        print(f"{mid}: {passed}/{total} cases rendered, wrote {mid}.renders.json")


if __name__ == "__main__":
    main(sys.argv[1:])
