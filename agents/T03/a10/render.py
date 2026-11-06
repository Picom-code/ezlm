#!/usr/bin/env python3
"""Render the gpt-oss chat template with llama.cpp's Jinja conventions.

Reads agents/T03/models/<id>.templates.json, renders the required cases and
writes agents/T03/models/<id>.renders.json.  Usage:

  ~/ezlm-agents/run/venv/bin/python agents/T03/a10/render.py
"""
import datetime
import json
import pathlib
import sys

import jinja2

HERE = pathlib.Path(__file__).resolve().parent
MODELS = HERE.parent / "models"

TOOL = {
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

CALL = {
    "id": "call_1",
    "type": "function",
    "function": {"name": "get_weather", "arguments": {"city": "Paris", "unit": "c"}},
}
CALL2 = {
    "id": "call_2",
    "type": "function",
    "function": {"name": "get_time", "arguments": {"city": "Paris"}},
}


class RaiseException(Exception):
    pass


def tojson(value, indent=None, separators=None):
    """llama.cpp's jinja tojson: plain str, no HTML escaping.

    Python jinja2 3.1 returns Markup from |tojson, so concatenating it with
    plain text HTML-escapes the rest of the line. llama.cpp's own jinja
    (common/jinja/value.cpp) returns a plain string, so use json.dumps here.
    """
    kwargs = {"ensure_ascii": False}
    if indent is not None:
        kwargs["indent"] = indent
    if separators is not None:
        kwargs["separators"] = tuple(separators)
    else:
        kwargs["separators"] = (", " if not indent else ",", ": " if not indent else ": ")
    return json.dumps(value, **kwargs)


def render(template, **ctx):
    env = jinja2.Environment(trim_blocks=False, lstrip_blocks=False, keep_trailing_newline=True)
    env.globals["raise_exception"] = lambda msg: (_ for _ in ()).throw(RaiseException(msg))
    env.globals["strftime_now"] = lambda fmt: datetime.datetime.now().strftime(fmt)
    env.filters["tojson"] = tojson
    tmpl = env.from_string(template)
    return tmpl.render(**ctx)


def case(name, **ctx):
    return name, ctx


def cases():
    base = {"add_generation_prompt": True}
    yield case("plain", messages=[{"role": "user", "content": "Hello."}], **base)
    yield case(
        "system_user",
        messages=[
            {"role": "system", "content": "You are a terse assistant."},
            {"role": "user", "content": "Hello."},
        ],
        **base,
    )
    yield case(
        "tool_call",
        messages=[
            {"role": "user", "content": "What is the weather in Paris?"},
            {"role": "assistant", "content": "", "tool_calls": [CALL]},
            {"role": "tool", "content": "18 C, clear"},
        ],
        tools=[TOOL],
        **base,
    )
    yield case(
        "parallel_tools",
        messages=[
            {"role": "user", "content": "Weather and time in Paris?"},
            {"role": "assistant", "content": "", "tool_calls": [CALL, CALL2]},
            {"role": "tool", "content": "18 C, clear"},
            {"role": "tool", "content": "12:00"},
        ],
        tools=[TOOL, TOOL2],
        **base,
    )
    # The template names no thinking variable; try the usual candidates so the
    # renders file shows whether they change anything.
    for val in (True, False):
        yield case(f"thinking_{'on' if val else 'off'}", messages=[{"role": "user", "content": "Hi."}], enable_thinking=val, **base)
    for val in (True, False):
        yield case(f"thinking_kwarg_{'on' if val else 'off'}", messages=[{"role": "user", "content": "Hi."}], thinking=val, **base)
    for level in ("none", "minimal", "low", "medium", "high", "xhigh", "max"):
        yield case(f"effort_{level}", messages=[{"role": "user", "content": "Hi."}], reasoning_effort=level, **base)
    for level in ("low", "high"):
        yield case(f"effort_nested_{level}", messages=[{"role": "user", "content": "Hi."}], reasoning={"effort": level}, **base)
        yield case(f"effort_thinking_{level}", messages=[{"role": "user", "content": "Hi."}], thinking={"effort": level}, **base)
    yield case(
        "developer_role",
        messages=[
            {"role": "developer", "content": "Answer in one sentence."},
            {"role": "user", "content": "Hello."},
        ],
        **base,
    )
    yield case(
        "developer_role_second",
        messages=[
            {"role": "system", "content": "You are helpful."},
            {"role": "developer", "content": "Answer in one sentence."},
            {"role": "user", "content": "Hello."},
        ],
        **base,
    )
    # Extra probes: what breaks beyond the required list.
    yield case("no_generation_prompt", messages=[{"role": "user", "content": "Hello."}], add_generation_prompt=False)
    yield case(
        "tool_args_string",
        messages=[
            {"role": "user", "content": "What is the weather in Paris?"},
            {"role": "assistant", "content": "", "tool_calls": [
                {"id": "c", "type": "function", "function": {"name": "get_weather", "arguments": '{"city": "Paris"}'}}
            ]},
            {"role": "tool", "content": "18 C, clear"},
        ],
        tools=[TOOL],
        **base,
    )
    yield case(
        "tool_result_first",
        messages=[
            {"role": "user", "content": "Hi."},
            {"role": "tool", "content": "orphan"},
        ],
        tools=[TOOL],
        **base,
    )
    yield case("assistant_channel_tags", messages=[
        {"role": "user", "content": "Hi."},
        {"role": "assistant", "content": "<|channel|>final<|message|>Hello"},
    ], **base)


def main():
    ids = sys.argv[1:] or ["gpt-oss-120b", "gpt-oss-20b"]
    # b11960 patches the content channel-tag check in common_chat_templates_init
    # (common/chat.cpp) to "{%- if false %}" before rendering.
    patch_from = (
        '{%- if "<|channel|>analysis<|message|>" in message.content or '
        '"<|channel|>final<|message|>" in message.content %}'
    )
    for mid in ids:
        data = json.loads((MODELS / f"{mid}.templates.json").read_text())
        template = data["tokenizer.chat_template"]
        patched = template.replace(patch_from, "{%- if false %}")
        results = {}
        for name, ctx in cases():
            try:
                out = render(template, **ctx)
                results[name] = {"status": "ok", "output": out}
            except Exception as e:  # noqa: BLE001 - record the exact text
                results[name] = {"status": "raised", "error": f"{type(e).__name__}: {e}"}
        try:
            results["assistant_channel_tags_b11960"] = {
                "status": "ok",
                "output": render(patched, messages=[
                    {"role": "user", "content": "Hi."},
                    {"role": "assistant", "content": "<|channel|>final<|message|>Hello"},
                ], add_generation_prompt=True),
            }
        except Exception as e:  # noqa: BLE001
            results["assistant_channel_tags_b11960"] = {"status": "raised", "error": f"{type(e).__name__}: {e}"}
        (MODELS / f"{mid}.renders.json").write_text(
            json.dumps({
                "id": mid,
                "renderer": "python jinja2 3.1.6 with llama.cpp conventions "
                            "(raise_exception, strftime_now, minja-style plain-str tojson); "
                            "assistant_channel_tags_b11960 applies chat.cpp's patch",
                "cases": results,
            }, indent=1) + "\n"
        )
        bad = [k for k, v in results.items() if v["status"] != "ok"]
        print(f"{mid}: {len(results) - len(bad)} ok, {len(bad)} raised {bad}")


if __name__ == "__main__":
    main()
