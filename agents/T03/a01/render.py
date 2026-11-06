#!/usr/bin/env python3
"""Render a model's chat template with jinja2 using llama.cpp's conventions.

Reads agents/T03/models/<id>.templates.json (written from the GGUF header) and
writes agents/T03/models/<id>.renders.json.

Usage: ~/ezlm-agents/run/venv/bin/python agents/T03/a01/render.py <id> [<id>...]
"""
import json
import os
import sys

import jinja2

HERE = os.path.dirname(os.path.abspath(__file__))
MODELS = os.path.join(os.path.dirname(HERE), "models")

WEATHER_TOOL = {
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
TIME_TOOL = {
    "type": "function",
    "function": {
        "name": "get_time",
        "description": "Get the current time in a city.",
        "parameters": {
            "type": "object",
            "properties": {"city": {"type": "string"}},
            "required": ["city"],
        },
    },
}


def assistant_call(name, arguments, call_id="call_1", content=None):
    return {
        "role": "assistant",
        "content": content,
        "tool_calls": [
            {
                "id": call_id,
                "type": "function",
                "function": {"name": name, "arguments": arguments},
            }
        ],
    }


def tool_result(content, call_id="call_1", name="get_weather"):
    return {"role": "tool", "content": content, "tool_call_id": call_id, "name": name}


PLAIN = [{"role": "user", "content": "Hello, world!"}]
SYSTEM_USER = [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "Hello, world!"},
]
TOOL_CALL = [
    {"role": "user", "content": "What is the weather in Paris?"},
    assistant_call("get_weather", {"city": "Paris"}),
    tool_result("22 degrees and sunny"),
]
PARALLEL = [
    {"role": "user", "content": "Weather in Paris and the time in Tokyo?"},
    {
        "role": "assistant",
        "content": "",
        "tool_calls": [
            {"id": "call_1", "type": "function",
             "function": {"name": "get_weather", "arguments": {"city": "Paris"}}},
            {"id": "call_2", "type": "function",
             "function": {"name": "get_time", "arguments": {"city": "Tokyo"}}},
        ],
    },
    tool_result("22 degrees and sunny", "call_1", "get_weather"),
    tool_result("09:30", "call_2", "get_time"),
]

CASES = [
    ("plain", {"messages": PLAIN}),
    ("system_user", {"messages": SYSTEM_USER}),
    ("tool_call", {"messages": TOOL_CALL, "tools": [WEATHER_TOOL]}),
    ("parallel_tools", {"messages": PARALLEL, "tools": [WEATHER_TOOL, TIME_TOOL]}),
    ("thinking_on", {"messages": PLAIN, "enable_thinking": True}),
    ("thinking_off", {"messages": PLAIN, "enable_thinking": False}),
    ("effort_none", {"messages": PLAIN, "enable_thinking": True, "reasoning_effort": "none"}),
    ("effort_low", {"messages": PLAIN, "enable_thinking": True, "reasoning_effort": "low"}),
    ("effort_medium", {"messages": PLAIN, "enable_thinking": True, "reasoning_effort": "medium"}),
    ("effort_high", {"messages": PLAIN, "enable_thinking": True, "reasoning_effort": "high"}),
    ("effort_xhigh", {"messages": PLAIN, "enable_thinking": True, "reasoning_effort": "xhigh"}),
    ("effort_max", {"messages": PLAIN, "enable_thinking": True, "reasoning_effort": "max"}),
    ("effort_minimal", {"messages": PLAIN, "enable_thinking": True, "reasoning_effort": "minimal"}),
    ("developer_role", {"messages": [
        {"role": "developer", "content": "Be terse."},
        {"role": "user", "content": "Hello, world!"},
    ]}),
    # extras that show how the variable is read
    ("effort_high_thinking_off", {"messages": PLAIN, "enable_thinking": False,
                                   "reasoning_effort": "high"}),
    ("effort_nested_reasoning", {"messages": PLAIN, "enable_thinking": True,
                                 "reasoning": {"effort": "high"}}),
    ("effort_nested_thinking", {"messages": PLAIN, "enable_thinking": True,
                                "thinking": {"effort": "high"}}),
    ("effort_unset", {"messages": PLAIN, "enable_thinking": True}),
]


def make_env():
    env = jinja2.Environment(
        autoescape=False,
        keep_trailing_newline=True,
        extensions=["jinja2.ext.loopcontrols"],
    )

    def raise_exception(msg):
        raise RuntimeError(str(msg))

    env.globals["raise_exception"] = raise_exception
    return env


def render(template_src, case):
    env = make_env()
    tmpl = env.from_string(template_src)
    ctx = {"add_generation_prompt": True}
    ctx.update(case)
    try:
        return {"ok": True, "output": tmpl.render(**ctx), "error": None}
    except Exception as e:  # noqa: BLE001 - we record the exact failure text
        return {"ok": False, "output": None, "error": f"{type(e).__name__}: {e}"}


def main(ids):
    env = make_env()
    for mid in ids:
        src_path = os.path.join(MODELS, f"{mid}.templates.json")
        data = json.load(open(src_path))
        templates = data["templates"]
        main_src = templates["__main__"]
        named = {k: v for k, v in templates.items() if k != "__main__"}
        out = {
            "id": mid,
            "repo": data["repo"],
            "file": data["file"],
            "template_source": "tokenizer.chat_template",
            "named_templates": sorted(named),
            "jinja2_version": jinja2.__version__,
            "llamacpp_conventions": {
                "raise_exception": "stub raises RuntimeError(message)",
                "tojson": "jinja2 built-in (separators ', ' and ': ', ensure_ascii=True; "
                          "minja uses the same separators but ensure_ascii=False)",
                "add_generation_prompt": True,
                "arguments": "tool_call arguments are objects (llama.cpp func_args_not_string)",
                "content_trim": "llama.cpp trims message content before rendering; the cases use "
                                "already-trimmed content",
            },
            "cases": {},
        }
        for name, case in CASES:
            res = render(main_src, case)
            res["case"] = name
            res["vars"] = {k: v for k, v in case.items() if k != "messages"}
            out["cases"][name] = res
        out["summary"] = {
            "ok": sum(1 for r in out["cases"].values() if r["ok"]),
            "raised": sum(1 for r in out["cases"].values() if not r["ok"]),
        }
        dst = os.path.join(MODELS, f"{mid}.renders.json")
        json.dump(out, open(dst, "w"), indent=1, ensure_ascii=False)
        print(f"{mid}: {out['summary']['ok']} ok, {out['summary']['raised']} raised -> {dst}")
        for name, r in out["cases"].items():
            if not r["ok"]:
                print(f"  {name}: RAISED {r['error']}")


if __name__ == "__main__":
    main(sys.argv[1:])
