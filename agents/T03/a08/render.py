#!/usr/bin/env python3
"""Render the a08 models' GGUF chat templates with jinja2, following llama.cpp
b11960's conventions (common/chat.cpp + common/jinja/caps.cpp).

Reads agents/T03/models/<id>.templates.json, writes agents/T03/models/<id>.renders.json.
"""
import json
import os
import sys

import jinja2

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.abspath(os.path.join(HERE, "..", "..", ".."))
MODELS = os.path.join(ROOT, "agents", "T03", "models")


class TemplateRaise(Exception):
    """Stands in for llama.cpp's raise_exception(), which aborts the render."""


def _tojson(value):
    # llama.cpp's jinja engine serialises with nlohmann::json::dump(): compact,
    # no spaces, UTF-8 kept as-is. jinja2's own tojson adds spaces and escapes
    # non-ASCII, so override it to keep the renders faithful.
    return json.dumps(value, separators=(",", ":"), ensure_ascii=False)


def make_env():
    # llama.cpp's engine returns a falsy undefined for a missing identifier
    # (common/jinja/runtime.cpp:104, as_bool() == false), so plain jinja2
    # Undefined matches it better than StrictUndefined.
    env = jinja2.Environment(undefined=jinja2.Undefined, keep_trailing_newline=True)
    env.globals["raise_exception"] = lambda msg="": (_ for _ in ()).throw(TemplateRaise(msg))
    env.filters["tojson"] = _tojson
    return env


def render(env, source, ctx):
    try:
        out = env.from_string(source).render(**ctx)
        return {"ok": True, "output": out, "exception": None}
    except TemplateRaise as e:
        return {"ok": False, "output": None, "exception": "raise_exception: %s" % e}
    except Exception as e:  # jinja2 syntax / undefined errors
        return {"ok": False, "output": None,
                "exception": "%s: %s" % (type(e).__name__, e)}


# --- fixtures ---------------------------------------------------------------

TOOLS = [{
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
}]

TOOL_CALL = {
    "id": "call_1",
    "type": "function",
    "function": {"name": "get_weather", "arguments": {"city": "Paris", "unit": "c"}},
}
TOOL_CALL_2 = {
    "id": "call_2",
    "type": "function",
    "function": {"name": "get_weather", "arguments": {"city": "Berlin", "unit": "f"}},
}
TOOL_CALL_STR = {
    "id": "call_1",
    "type": "function",
    "function": {"name": "get_weather", "arguments": '{"city": "Paris", "unit": "c"}'},
}
TOOL_RESULT = {"role": "tool", "tool_call_id": "call_1",
               "content": '{"temp_c": 18, "conditions": "clear"}'}

MSGS_PLAIN = [{"role": "user", "content": "What is the capital of France?"}]
MSGS_SYSTEM = [{"role": "system", "content": "You are a concise assistant."}] + MSGS_PLAIN
MSGS_TOOL = [
    {"role": "user", "content": "What is the weather in Paris?"},
    {"role": "assistant", "content": "", "tool_calls": [TOOL_CALL]},
    TOOL_RESULT,
]
MSGS_TOOL_STR = [
    {"role": "user", "content": "What is the weather in Paris?"},
    {"role": "assistant", "content": "", "tool_calls": [TOOL_CALL_STR]},
    TOOL_RESULT,
]
MSGS_PARALLEL = [
    {"role": "user", "content": "What is the weather in Paris and Berlin?"},
    {"role": "assistant", "content": "", "tool_calls": [TOOL_CALL, TOOL_CALL_2]},
    {"role": "tool", "tool_call_id": "call_1", "content": '{"temp_c": 18}'},
    {"role": "tool", "tool_call_id": "call_2", "content": '{"temp_f": 64}'},
]
MSGS_DEVELOPER = [{"role": "developer", "content": "You are a concise assistant."}] + MSGS_PLAIN
MSGS_DEVELOPER_MID = MSGS_PLAIN[:1] + [
    {"role": "developer", "content": "Answer in one line."},
] + [{"role": "user", "content": "And in German?"}]
MSGS_SYSTEM_MID = MSGS_PLAIN + [
    {"role": "system", "content": "Answer in one line."},
    {"role": "user", "content": "And in German?"},
]
MSGS_NO_USER = [{"role": "assistant", "content": "", "tool_calls": [TOOL_CALL]}, TOOL_RESULT]


def cases():
    """(case name, template context). Mirrors the brief's list, plus extras."""
    # llama.cpp always sets bos_token/eos_token (chat.cpp:1068) and its caps
    # harness always passes tools, possibly empty (caps.cpp:57-63).
    base = {"add_generation_prompt": True, "bos_token": "", "eos_token": "", "tools": []}
    out = []

    def add(name, **ctx):
        c = dict(base)
        c.update(ctx)
        out.append((name, c))

    add("plain", messages=MSGS_PLAIN)
    add("system_user", messages=MSGS_SYSTEM)
    add("tool_call", messages=MSGS_TOOL, tools=TOOLS)
    add("parallel_tools", messages=MSGS_PARALLEL, tools=TOOLS)
    add("thinking_on", messages=MSGS_SYSTEM, enable_thinking=True)
    add("thinking_off", messages=MSGS_SYSTEM, enable_thinking=False)
    for level in ("none", "low", "medium", "high", "xhigh", "max"):
        add("effort_" + level, messages=MSGS_SYSTEM, enable_thinking=True,
            reasoning_effort=level)
    add("developer_role", messages=MSGS_DEVELOPER)

    # extras: shapes llama.cpp or agent apps really send
    add("extra_tool_call_string_args", messages=MSGS_TOOL_STR, tools=TOOLS)
    add("extra_developer_mid_history", messages=MSGS_DEVELOPER_MID)
    add("extra_system_mid_history", messages=MSGS_SYSTEM_MID)
    add("extra_no_user_query", messages=MSGS_NO_USER)
    add("extra_reasoning_strength", messages=MSGS_SYSTEM, enable_thinking=True,
        reasoning_strength="high")
    add("extra_reasoning_object", messages=MSGS_SYSTEM, enable_thinking=True,
        reasoning={"effort": "high"})
    add("extra_thinking_object", messages=MSGS_SYSTEM, enable_thinking=True,
        thinking={"effort": "high"})
    return out


def main():
    env = make_env()
    for mid in ("qwen3.5-4b", "qwen3.5-2b"):
        tpl_path = os.path.join(MODELS, mid + ".templates.json")
        doc = json.load(open(tpl_path))
        source = doc["templates"]["tokenizer.chat_template"]
        rendered = {}
        for name, ctx in cases():
            rendered[name] = render(env, source, ctx)

        # effort / thinking variables are only "honoured" if the render changes
        baseline = rendered["thinking_on"]["output"]
        for level in ("none", "low", "medium", "high", "xhigh", "max"):
            c = rendered["effort_" + level]
            c["changes_render"] = (c["output"] != baseline) if c["ok"] else None
        for name in ("extra_reasoning_strength", "extra_reasoning_object", "extra_thinking_object"):
            c = rendered[name]
            c["changes_render"] = (c["output"] != baseline) if c["ok"] else None

        out = {
            "id": mid,
            "repo": doc["repo"],
            "template_source": "tokenizer.chat_template",
            "engine": "jinja2 %s, llama.cpp b11960 common/jinja conventions" % jinja2.__version__,
            "baseline_for_changes": "thinking_on",
            "cases": rendered,
        }
        path = os.path.join(MODELS, mid + ".renders.json")
        with open(path, "w") as f:
            json.dump(out, f, indent=1, ensure_ascii=False)
            f.write("\n")
        ok = sum(1 for c in rendered.values() if c["ok"])
        print("%-12s %d/%d rendered ok -> %s" % (mid, ok, len(rendered), path))


if __name__ == "__main__":
    main()
