#!/usr/bin/env python3
"""Renders one model's chat template under llama.cpp's conventions and records
every case as OK (with the exact output) or raised (with the exact text).

  agents/T03/a15/render.py <model-id>

Reads agents/T03/models/<id>.templates.json, writes <id>.renders.json next to it.
"""
import json
import os
import sys

from jinja2 import Environment, StrictUndefined, UndefinedError
from jinja2.exceptions import TemplateError as JinjaTemplateError

HERE = os.path.dirname(os.path.abspath(__file__))
MODELS = os.path.normpath(os.path.join(HERE, "..", "models"))


class TemplateError(Exception):
    """What llama.cpp's raise_exception() does: abort the render with a message."""


def raise_exception(msg):
    raise TemplateError(str(msg))


def strftime_now(fmt="%Y-%m-%d"):
    return "2026-11-06"


def env():
    e = Environment(trim_blocks=False, lstrip_blocks=False, keep_trailing_newline=True)
    e.globals["raise_exception"] = raise_exception
    e.globals["strftime_now"] = strftime_now
    return e


def render(template, ctx):
    """Render once. Returns (True, text) or (False, exception text)."""
    try:
        out = env().from_string(template).render(**ctx)
        return True, out
    except TemplateError as exc:
        return False, str(exc)
    except UndefinedError as exc:
        return False, "UndefinedError: " + str(exc)
    except JinjaTemplateError as exc:
        return False, type(exc).__name__ + ": " + str(exc)
    except Exception as exc:  # noqa: BLE001 - we want the text, not a crash
        return False, type(exc).__name__ + ": " + str(exc)


TOOLS = [
    {
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
]

TOOL_CALL = {
    "id": "call_1",
    "type": "function",
    "function": {
        "name": "get_weather",
        "arguments": {"city": "Oslo", "unit": "celsius"},
    },
}

TOOL_CALL_2 = {
    "id": "call_2",
    "type": "function",
    "function": {
        "name": "get_weather",
        "arguments": {"city": "Lima", "unit": "fahrenheit"},
    },
}


def cases():
    """(name, messages, tools, extra context) in the order the brief lists them."""
    user = [{"role": "user", "content": "What is the weather in Oslo?"}]
    return [
        ("plain", [{"role": "user", "content": "Hello, what is 2+2?"}], None, {}),
        (
            "system_user",
            [
                {"role": "system", "content": "You are a terse assistant."},
                {"role": "user", "content": "What is 2+2?"},
            ],
            None,
            {},
        ),
        (
            "tool_call",
            [
                {"role": "system", "content": "You are a helpful assistant."},
                {"role": "user", "content": "What is the weather in Oslo?"},
                {"role": "assistant", "content": "", "tool_calls": [TOOL_CALL]},
                {
                    "role": "tool",
                    "tool_call_id": "call_1",
                    "content": '{"temp_c": 4, "sky": "cloudy"}',
                },
            ],
            TOOLS,
            {},
        ),
        (
            "parallel_tools",
            [
                {"role": "user", "content": "Weather in Oslo and Lima?"},
                {
                    "role": "assistant",
                    "content": "",
                    "tool_calls": [TOOL_CALL, TOOL_CALL_2],
                },
                {"role": "tool", "tool_call_id": "call_1", "content": '{"temp_c": 4}'},
                {"role": "tool", "tool_call_id": "call_2", "content": '{"temp_f": 68}'},
            ],
            TOOLS,
            {},
        ),
        ("thinking_on", user, None, {"enable_thinking": True}),
        ("thinking_off", user, None, {"enable_thinking": False}),
        ("thinking_off_gen_prompt_off", user, None,
         {"enable_thinking": False, "add_generation_prompt": False}),
    ]


EFFORTS = ["none", "low", "medium", "high", "xhigh", "max"]


def effort_cases():
    user = [{"role": "user", "content": "Explain why the sky is blue."}]
    out = []
    for effort in EFFORTS:
        out.append((f"effort_{effort}", user, None, {"reasoning_effort": effort}))
    return out


def effort_shape_cases():
    """The other shapes a template might read: reasoning.effort and thinking.effort."""
    user = [{"role": "user", "content": "Explain why the sky is blue."}]
    out = []
    for effort in EFFORTS:
        out.append(
            (f"reasoning_effort_dotted_{effort}", user, None,
             {"reasoning": {"effort": effort}})
        )
        out.append(
            (f"thinking_effort_{effort}", user, None,
             {"thinking": {"effort": effort}})
        )
    return out


def extra_cases():
    user = [{"role": "user", "content": "What is 2+2?"}]
    string_args = dict(TOOL_CALL)
    string_args = {
        "id": "call_1",
        "type": "function",
        "function": {"name": "get_weather", "arguments": '{"city": "Oslo"}'},
    }
    return [
        (
            "developer_role",
            [
                {"role": "developer", "content": "You are a terse assistant."},
                {"role": "user", "content": "What is 2+2?"},
            ],
            None,
            {},
        ),
        (
            "developer_role_late",
            [
                {"role": "user", "content": "What is 2+2?"},
                {"role": "developer", "content": "Be brief."},
                {"role": "user", "content": "And 3+3?"},
            ],
            None,
            {},
        ),
        (
            "system_role_late",
            [
                {"role": "user", "content": "What is 2+2?"},
                {"role": "system", "content": "Be brief."},
                {"role": "user", "content": "And 3+3?"},
            ],
            None,
            {},
        ),
        (
            "system_and_developer",
            [
                {"role": "system", "content": "You are a terse assistant."},
                {"role": "developer", "content": "Answer in one word."},
                {"role": "user", "content": "What is 2+2?"},
            ],
            None,
            {},
        ),
        (
            "tool_call_string_args",
            [
                {"role": "user", "content": "What is the weather in Oslo?"},
                {"role": "assistant", "content": "", "tool_calls": [string_args]},
                {"role": "tool", "tool_call_id": "call_1", "content": "{}"},
            ],
            TOOLS,
            {},
        ),
        (
            "tool_call_no_name",
            [
                {"role": "user", "content": "What is the weather in Oslo?"},
                {
                    "role": "assistant",
                    "content": "",
                    "tool_calls": [
                        {"type": "function", "function": {"arguments": {"city": "Oslo"}}}
                    ],
                },
                {"role": "tool", "tool_call_id": "call_1", "content": "{}"},
            ],
            TOOLS,
            {},
        ),
        ("no_messages", [], None, {}),
        ("effort_high_thinking_off", user, None,
         {"reasoning_effort": "high", "enable_thinking": False}),
        ("effort_max_thinking_off", user, None,
         {"reasoning_effort": "max", "enable_thinking": False}),
        ("plain_gen_prompt_off", user, None, {"add_generation_prompt": False}),
    ]


def main():
    if len(sys.argv) != 2:
        sys.exit("usage: render.py <model-id>")
    model_id = sys.argv[1]
    templates = json.load(open(os.path.join(MODELS, model_id + ".templates.json")))
    if "tokenizer.chat_template" not in templates:
        sys.exit(model_id + ": no tokenizer.chat_template")

    base = {"messages": [], "tools": None, "add_generation_prompt": True}
    out = {}
    for name, messages, tools, extra in (
        cases() + effort_cases() + effort_shape_cases() + extra_cases()
    ):
        ctx = dict(base)
        ctx.update(extra)
        ctx["messages"] = messages
        ctx["tools"] = tools
        ok, payload = render(templates["tokenizer.chat_template"], ctx)
        out[name] = {
            "ok": ok,
            "output": payload if ok else None,
            "error": None if ok else payload,
            "context": {"enable_thinking": extra.get("enable_thinking"),
                        "reasoning_effort": extra.get("reasoning_effort"),
                        "add_generation_prompt": ctx["add_generation_prompt"]},
        }
    dest = os.path.join(MODELS, model_id + ".renders.json")
    with open(dest, "w") as f:
        json.dump({"id": model_id, "template_source": "tokenizer.chat_template",
                   "cases": out}, f, indent=1)
    ok = sum(1 for v in out.values() if v["ok"])
    print(f"{model_id}: {ok}/{len(out)} cases rendered, {len(out) - ok} raised -> {dest}")
    for k, v in out.items():
        print(("  ok   " if v["ok"] else "  raise") + "  " + k +
              ("" if v["ok"] else "  :: " + v["error"][:120]))


if __name__ == "__main__":
    main()
