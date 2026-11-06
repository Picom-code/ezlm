#!/usr/bin/env python3
"""Render a model's chat template the way llama.cpp's server does.

Usage: render.py <model-id> [<model-id> ...]

Reads agents/T03/models/<id>.templates.json and writes
agents/T03/models/<id>.renders.json. The context mirrors what
common/chat.cpp (b11960) passes to jinja: messages, tools,
add_generation_prompt, enable_thinking, bos_token, eos_token, datetime,
date_string, and chat_template_kwargs (reasoning_effort and friends).

jinja2 is not byte-identical to llama.cpp's own jinja runtime, so the
output is a faithful but not guaranteed byte-exact rendering; JSON spacing
from `tojson` is the main known difference.
"""
import json
import os
import sys

from jinja2 import Environment, StrictUndefined


def llama_tojson(value, indent=None, ensure_ascii=False, separators=None, sort_keys=False):
    """llama.cpp's jinja `tojson` (common/jinja/value.cpp): Python-style JSON
    with `, ` and `: ` separators, ensure_ascii defaulting to false, and an
    optional `separators` pair."""
    if sort_keys:
        raise NotImplementedError("tojson sort_keys=true not implemented")
    kwargs = {"ensure_ascii": bool(ensure_ascii)}
    if indent is not None:
        kwargs["indent"] = int(indent)
    if separators:
        kwargs["separators"] = tuple(separators)
    return json.dumps(value, **kwargs)

HERE = os.path.dirname(os.path.abspath(__file__))
MODELS = os.path.join(os.path.dirname(HERE), "models")


def make_env():
    env = Environment(undefined=StrictUndefined, autoescape=False)

    def raise_exception(msg):
        raise ValueError(str(msg))

    env.globals["raise_exception"] = raise_exception
    # llama.cpp also exposes these globals
    env.globals["strftime_now"] = lambda fmt: "2026-11-06"
    env.filters["tojson"] = llama_tojson
    return env


TOOL_WEATHER = {
    "type": "function",
    "function": {
        "name": "get_weather",
        "description": "Get the current weather for a city.",
        "parameters": {
            "type": "object",
            "properties": {
                "city": {"type": "string", "description": "City name."},
                "unit": {"type": "string", "enum": ["c", "f"]},
            },
            "required": ["city"],
        },
    },
}

TOOL_TIME = {
    "type": "function",
    "function": {
        "name": "get_time",
        "description": "Get the current time in a timezone.",
        "parameters": {
            "type": "object",
            "properties": {"tz": {"type": "string"}},
            "required": ["tz"],
        },
    },
}

PLAIN = [{"role": "user", "content": "Hello, what is 2+2?"}]

SYSTEM_USER = [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "Hello, what is 2+2?"},
]

TOOL_CALL = [
    {"role": "user", "content": "What is the weather in Paris?"},
    {
        "role": "assistant",
        "content": "",
        "tool_calls": [
            {
                "id": "call_1",
                "type": "function",
                "function": {
                    "name": "get_weather",
                    "arguments": {"city": "Paris", "unit": "c"},
                },
            }
        ],
    },
    {"role": "tool", "content": '{"temp_c": 18, "condition": "cloudy"}', "tool_call_id": "call_1"},
]

PARALLEL_TOOLS = [
    {"role": "user", "content": "What is the weather in Paris and the time in Tokyo?"},
    {
        "role": "assistant",
        "content": "",
        "tool_calls": [
            {
                "id": "call_1",
                "type": "function",
                "function": {"name": "get_weather", "arguments": {"city": "Paris", "unit": "c"}},
            },
            {
                "id": "call_2",
                "type": "function",
                "function": {"name": "get_time", "arguments": {"tz": "Asia/Tokyo"}},
            },
        ],
    },
    {"role": "tool", "content": '{"temp_c": 18}', "tool_call_id": "call_1"},
    {"role": "tool", "content": '{"time": "13:00"}', "tool_call_id": "call_2"},
]

DEVELOPER = [
    {"role": "developer", "content": "You are terse."},
    {"role": "user", "content": "Hello, what is 2+2?"},
]


def base_context(messages, tools=None, **extra):
    ctx = {
        "messages": messages,
        "bos_token": "",
        "eos_token": "",
        "add_generation_prompt": True,
        "enable_thinking": True,
        "datetime": "Nov 06 2026",
        "date_string": "06 Nov 2026",
    }
    if tools is not None:
        ctx["tools"] = tools
    ctx.update(extra)
    return ctx


def cases():
    yield "plain", base_context(PLAIN)
    yield "system_user", base_context(SYSTEM_USER)
    yield "tool_call", base_context(TOOL_CALL, [TOOL_WEATHER])
    yield "parallel_tools", base_context(PARALLEL_TOOLS, [TOOL_WEATHER, TOOL_TIME])
    yield "thinking_on", base_context(PLAIN, enable_thinking=True)
    yield "thinking_off", base_context(PLAIN, enable_thinking=False)
    # the template does not name `thinking`; this documents that it is ignored
    yield "thinking_var_true", base_context(PLAIN, thinking=True)
    yield "thinking_var_false", base_context(PLAIN, thinking=False)
    for effort in ("none", "minimal", "low", "medium", "high", "xhigh", "max"):
        yield f"effort_{effort}", base_context(PLAIN, reasoning_effort=effort)
    # non-string shapes some OpenAI clients send
    yield "effort_reasoning_object", base_context(PLAIN, reasoning={"effort": "high"})
    yield "effort_thinking_object", base_context(PLAIN, thinking={"effort": "high"})
    yield "developer_role", base_context(DEVELOPER)
    yield "no_generation_prompt", base_context(PLAIN, add_generation_prompt=False)


def render_one(env, src, ctx):
    tmpl = env.from_string(src)
    return tmpl.render(**ctx)


def main(ids):
    env = make_env()
    for mid in ids:
        path = os.path.join(MODELS, f"{mid}.templates.json")
        data = json.load(open(path))
        out = {"id": mid, "source": data.get("url", ""), "renderer": "jinja2 (python)", "templates": {}}
        for name, src in data["templates"].items():
            results = {}
            for case_name, ctx in cases():
                try:
                    text = render_one(env, src, ctx)
                    results[case_name] = {"ok": True, "output": text}
                except Exception as e:  # noqa: BLE001 - record the exact failure
                    results[case_name] = {
                        "ok": False,
                        "error": f"{type(e).__name__}: {e}",
                    }
            out["templates"][name] = results
            ok = sum(1 for r in results.values() if r["ok"])
            print(f"{mid} {name}: {ok}/{len(results)} rendered")
        with open(os.path.join(MODELS, f"{mid}.renders.json"), "w") as f:
            json.dump(out, f, indent=2, ensure_ascii=False)
            f.write("\n")


if __name__ == "__main__":
    if len(sys.argv) < 2:
        sys.exit(__doc__)
    main(sys.argv[1:])
