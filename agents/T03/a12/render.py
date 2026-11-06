#!/usr/bin/env python3
"""Render a12's two chat templates with jinja2 using llama.cpp's conventions.

llama.cpp b11960 runs templates through common/jinja (minja). This script mirrors
the bits that matter for the capability matrix:

  * context: messages, tools, add_generation_prompt, bos_token, eos_token, plus
    whatever chat_template_kwargs the server forwards (enable_thinking,
    reasoning_effort / reasoning_strength, preserve_thinking, ...).
  * raise_exception() throws "Jinja Exception: <msg>", like minja.
  * tojson is minja's: Python json.dumps with default separators and no HTML
    escaping (jinja2's built-in tojson escapes <, >, & and ' as \\u00XX).
  * strftime_now is pinned to a fixed instant so renders are reproducible;
    llama.cpp uses the wall clock.

Writes agents/T03/models/<id>.renders.json.
"""
import json
import os
import sys
from datetime import datetime

import jinja2

HERE = os.path.dirname(os.path.abspath(__file__))
MODELS = os.path.join(HERE, "..", "models")

# Fixed clock for strftime_now: the template embeds the date in its default
# system prompt, so a moving clock would make every render differ.
FIXED_NOW = datetime(2026, 11, 6, 12, 0, 0)


def raise_exception(msg):
    # minja wraps the message; the server surfaces this text in its HTTP 500.
    raise RuntimeError("Jinja Exception: " + str(msg))


def strftime_now(fmt):
    return FIXED_NOW.strftime(fmt)


def minja_tojson(value):
    # minja: ensure_ascii=false, default separators, no HTML escaping.
    return json.dumps(value, ensure_ascii=False, default=str)


def make_env():
    env = jinja2.Environment(
        undefined=jinja2.Undefined,
        autoescape=False,
        keep_trailing_newline=True,
    )
    env.globals["raise_exception"] = raise_exception
    env.globals["strftime_now"] = strftime_now
    env.filters["tojson"] = minja_tojson
    return env


TOOL = {
    "type": "function",
    "function": {
        "name": "get_weather",
        "description": "Get the current weather for a city",
        "parameters": {
            "type": "object",
            "properties": {
                "city": {"type": "string", "description": "City name"},
            },
            "required": ["city"],
        },
    },
}

USER = {"role": "user", "content": "Hello"}
SYSTEM = {"role": "system", "content": "You are a helpful assistant."}
DEVELOPER = {"role": "developer", "content": "Be concise."}


def tool_call(call_id="call_0001", city="Paris"):
    return {
        "id": call_id,
        "type": "function",
        "function": {"name": "get_weather", "arguments": {"city": city}},
    }


ASSISTANT_TOOL = {
    "role": "assistant",
    "content": None,
    "tool_calls": [tool_call()],
}
ASSISTANT_PARALLEL = {
    "role": "assistant",
    "content": None,
    "tool_calls": [tool_call("call_0001", "Paris"), tool_call("call_0002", "Oslo")],
}
TOOL_RESULT = {"role": "tool", "content": "18C and sunny", "tool_call_id": "call_0001"}
TOOL_RESULT_2 = {"role": "tool", "content": "4C and rain", "tool_call_id": "call_0002"}

EFFORTS = ["none", "low", "medium", "high", "xhigh", "max"]


def cases():
    out = [
        ("plain", {"messages": [USER]}),
        ("system_user", {"messages": [SYSTEM, USER]}),
        ("tool_call", {"messages": [USER, ASSISTANT_TOOL, TOOL_RESULT], "tools": [TOOL]}),
        (
            "parallel_tools",
            {"messages": [USER, ASSISTANT_PARALLEL, TOOL_RESULT, TOOL_RESULT_2], "tools": [TOOL]},
        ),
        ("thinking_on", {"messages": [USER], "enable_thinking": True}),
        ("thinking_off", {"messages": [USER], "enable_thinking": False}),
        ("developer_role", {"messages": [SYSTEM, DEVELOPER, USER]}),
        ("consecutive_user_turns", {"messages": [USER, USER]}),
        # some clients echo history with tool arguments as a JSON string
        (
            "assistant_string_args",
            {
                "messages": [
                    USER,
                    {
                        "role": "assistant",
                        "content": None,
                        "tool_calls": [
                            {
                                "id": "call_0001",
                                "type": "function",
                                "function": {"name": "get_weather", "arguments": "{\"city\": \"Paris\"}"},
                            }
                        ],
                    },
                    TOOL_RESULT,
                ],
                "tools": [TOOL],
            },
        ),
        ("medium_effort_true", {"messages": [USER], "enable_thinking": True, "medium_effort": True}),
        ("medium_effort_false", {"messages": [USER], "enable_thinking": True, "medium_effort": False}),
        ("no_generation_prompt", {"messages": [USER], "add_generation_prompt": False}),
        # b11960's Ministral-3 handler rewrites assistant content into typed
        # blocks and adds a "thinking" block when reasoning_content is set.
        (
            "assistant_reasoning_content",
            {
                "messages": [
                    USER,
                    {"role": "assistant", "content": "Hi", "reasoning_content": "Let me think"},
                    USER,
                ]
            },
        ),
        (
            "assistant_typed_thinking",
            {
                "messages": [
                    USER,
                    {
                        "role": "assistant",
                        "content": [
                            {"type": "thinking", "thinking": "Let me think"},
                            {"type": "text", "text": "Hi"},
                        ],
                    },
                    USER,
                ]
            },
        ),
        (
            "assistant_reasoning_preserved",
            {
                "messages": [
                    USER,
                    {"role": "assistant", "content": "Hi", "reasoning_content": "Let me think"},
                    USER,
                ],
                "enable_thinking": True,
                "truncate_history_thinking": False,
            },
        ),
    ]
    for e in EFFORTS:
        out.append(
            (
                "effort_" + e,
                {"messages": [USER], "enable_thinking": True, "reasoning_effort": e},
            )
        )
    # alternative shapes some templates expect
    out.append(
        (
            "effort_high_reasoning_ns",
            {"messages": [USER], "enable_thinking": True, "reasoning": {"effort": "high"}},
        )
    )
    out.append(
        (
            "effort_high_thinking_ns",
            {"messages": [USER], "enable_thinking": True, "thinking": {"effort": "high"}},
        )
    )
    return out


def render_case(env, source, case):
    messages = case.get("messages", [])
    ctx = {
        "messages": messages,
        "tools": case.get("tools", []),
        "add_generation_prompt": case.get("add_generation_prompt", True),
        # llama.cpp fills these from the vocab; Devstral's GGUF uses <s>/</s>.
        "bos_token": "<s>",
        "eos_token": "</s>",
    }
    extra = {k: v for k, v in case.items() if k not in ("messages", "tools", "add_generation_prompt")}
    ctx.update(extra)
    record = {
        "case": case["name"] if "name" in case else None,
        "vars": extra,
        "add_generation_prompt": ctx["add_generation_prompt"],
        "tool_count": len(ctx["tools"]),
        "ok": False,
        "output": None,
        "exception": None,
    }
    try:
        tmpl = env.from_string(source)
        out = tmpl.render(**ctx)
        record["ok"] = True
        record["output"] = out
    except Exception as e:  # noqa: BLE001 - we want the exact text
        record["exception"] = "%s: %s" % (type(e).__name__, e)
    return record


def main():
    env = make_env()
    for mid in ("devstral-small-2-24b", "nemotron-3.5-super-vl-120b-a12b"):
        with open(os.path.join(MODELS, mid + ".templates.json")) as f:
            tpls = json.load(f)
        source = tpls["templates"]["tokenizer.chat_template"]
        rows = []
        for name, case in cases():
            case = dict(case, name=name)
            rows.append(render_case(env, source, case))
        doc = {
            "id": mid,
            "source": tpls["url"],
            "template": "tokenizer.chat_template",
            "renderer": "jinja2 (llama.cpp b11960 conventions; see agents/T03/a12/render.py)",
            "notes": [
                "strftime_now pinned to 2026-11-06T12:00:00; llama.cpp uses the wall clock",
                "bos_token/eos_token passed as <s>/</s>",
                "raise_exception text prefixed with 'Jinja Exception: ' like minja",
                "tojson matches minja (no HTML escaping, default separators)",
            ],
            "cases": rows,
        }
        path = os.path.join(MODELS, mid + ".renders.json")
        with open(path, "w") as f:
            json.dump(doc, f, indent=2, ensure_ascii=False)
            f.write("\n")
        ok = sum(1 for r in rows if r["ok"])
        print("%s: %d/%d rendered, %d raised -> %s" % (mid, ok, len(rows), len(rows) - ok, path))


if __name__ == "__main__":
    sys.exit(main())
