#!/usr/bin/env python3
"""Render one GGUF chat template with llama.cpp-style inputs.

Usage: render.py <model-id> [--templates path] [--out path]

The templates JSON is {name: template_string}; each is rendered against the
cases in CASES. Undefined variables behave like llama.cpp's jinja runtime
(empty render, `is defined` false), and raise_exception() aborts the render
with the template's own message.
"""
import argparse, json, os, sys

import jinja2


class RaiseException(Exception):
    pass


def raise_exception(msg):
    raise RaiseException(str(msg))


ENV = jinja2.Environment(undefined=jinja2.Undefined, autoescape=False, keep_trailing_newline=True)
ENV.globals["raise_exception"] = raise_exception

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
        "description": "Get the current time for a city.",
        "parameters": {
            "type": "object",
            "properties": {"city": {"type": "string"}},
            "required": ["city"],
        },
    },
}

TOOL_CALL = {
    "id": "call_1",
    "type": "function",
    "function": {"name": "get_weather", "arguments": {"city": "Paris"}},
}
TOOL_CALL2 = {
    "id": "call_2",
    "type": "function",
    "function": {"name": "get_time", "arguments": {"city": "Paris"}},
}
TOOL_RESULT = {"role": "tool", "tool_call_id": "call_1", "content": json.dumps({"temp_c": 21})}

BASE = [{"role": "user", "content": "What is the weather in Paris?"}]

# name -> (messages, extra context vars, tools)
CASES = [
    ("plain", [{"role": "user", "content": "Hello, how are you?"}], {}, []),
    ("system_user", [
        {"role": "system", "content": "You are a terse assistant."},
        {"role": "user", "content": "Hello, how are you?"},
    ], {}, []),
    ("tool_call", [
        {"role": "user", "content": "What is the weather in Paris?"},
        {"role": "assistant", "content": "", "tool_calls": [TOOL_CALL]},
        TOOL_RESULT,
    ], {}, [TOOL]),
    ("parallel_tools", [
        {"role": "user", "content": "Weather and time in Paris?"},
        {"role": "assistant", "content": "", "tool_calls": [TOOL_CALL, TOOL_CALL2]},
        TOOL_RESULT,
        {"role": "tool", "tool_call_id": "call_2", "content": json.dumps({"time": "12:00"})},
    ], {}, [TOOL, TOOL2]),
    ("tool_call_string_args", [
        {"role": "user", "content": "What is the weather in Paris?"},
        {"role": "assistant", "content": "", "tool_calls": [
            {"id": "call_1", "type": "function",
             "function": {"name": "get_weather", "arguments": json.dumps({"city": "Paris"})}},
        ]},
        TOOL_RESULT,
    ], {}, [TOOL]),
    ("parallel_string_args", [
        {"role": "user", "content": "Weather and time in Paris?"},
        {"role": "assistant", "content": "", "tool_calls": [
            {"id": "call_1", "type": "function",
             "function": {"name": "get_weather", "arguments": json.dumps({"city": "Paris"})}},
            {"id": "call_2", "type": "function",
             "function": {"name": "get_time", "arguments": json.dumps({"city": "Paris"})}},
        ]},
        TOOL_RESULT,
        {"role": "tool", "tool_call_id": "call_2", "content": json.dumps({"time": "12:00"})},
    ], {}, [TOOL, TOOL2]),
    ("tool_call_json_format", [
        {"role": "user", "content": "What is the weather in Paris?"},
        {"role": "assistant", "content": "", "tool_calls": [TOOL_CALL]},
        TOOL_RESULT,
    ], {"tool_call_format": "json"}, [TOOL]),
    ("thinking_on", BASE, {"enable_thinking": True}, []),
    ("thinking_off", BASE, {"enable_thinking": False}, []),
    ("thinking_var_on", BASE, {"thinking": True}, []),
    ("thinking_var_off", BASE, {"thinking": False}, []),
    ("no_generation_prompt", BASE, {"add_generation_prompt": False}, []),
    ("effort_none", BASE, {"reasoning_effort": "none"}, []),
    ("effort_low", BASE, {"reasoning_effort": "low"}, []),
    ("effort_medium", BASE, {"reasoning_effort": "medium"}, []),
    ("effort_high", BASE, {"reasoning_effort": "high"}, []),
    ("effort_xhigh", BASE, {"reasoning_effort": "xhigh"}, []),
    ("effort_max", BASE, {"reasoning_effort": "max"}, []),
    ("effort_nested", BASE, {"reasoning": {"effort": "high"}}, []),
    ("system_image", [
        {"role": "system", "content": [{"type": "image_url", "image_url": {"url": "data:image/png;base64,AAAA"}}]},
        {"role": "user", "content": "What is this?"},
    ], {}, []),
    ("no_user_query", [
        {"role": "assistant", "content": "Hello."},
    ], {}, []),
    ("system_not_first", [
        {"role": "user", "content": "Hello."},
        {"role": "system", "content": "Late system."},
        {"role": "user", "content": "Again."},
    ], {}, []),
    ("developer_role", [
        {"role": "developer", "content": "You are a terse assistant."},
        {"role": "user", "content": "Hello, how are you?"},
    ], {}, []),
]


def render_one(source, messages, extra, tools):
    ctx = {
        "messages": messages,
        "tools": tools,
        "add_generation_prompt": True,
        "enable_thinking": True,
        "bos_token": "<s>",
        "eos_token": "</s>",
    }
    ctx.update(extra)
    try:
        out = ENV.from_string(source).render(**ctx)
        return {"status": "ok", "output": out}
    except Exception as e:  # noqa: BLE001 - record every failure verbatim
        return {"status": "raised", "exception": f"{type(e).__name__}: {e}"}


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("model")
    ap.add_argument("--templates")
    ap.add_argument("--out")
    a = ap.parse_args()
    here = os.path.dirname(os.path.abspath(__file__))
    tpath = a.templates or os.path.join(here, "..", "models", a.model + ".templates.json")
    outpath = a.out or os.path.join(here, "..", "models", a.model + ".renders.json")
    templates = json.load(open(tpath))
    results = {}
    for tname, source in templates.items():
        cases = {}
        for cname, msgs, extra, tools in CASES:
            cases[cname] = render_one(source, msgs, extra, tools)
        results[tname] = cases
    json.dump({"model": a.model, "cases": results}, open(outpath, "w"), indent=1, ensure_ascii=False)
    ok = sum(1 for t in results.values() for c in t.values() if c["status"] == "ok")
    raised = sum(1 for t in results.values() for c in t.values() if c["status"] == "raised")
    print(f"{a.model}: {ok} rendered, {raised} raised -> {outpath}")
    for cname, c in results.get("tokenizer.chat_template", {}).items():
        detail = c.get("exception") if c["status"] == "raised" else c["output"].replace("\n", "\\n")[:110]
        print(f"  {cname:22s} {c['status']:7s} {detail}")


if __name__ == "__main__":
    sys.exit(main())
