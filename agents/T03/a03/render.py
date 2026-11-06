#!/usr/bin/env python3
"""Render a GGUF chat template the way llama.cpp b11960 does.

  render.py <model-id>

Reads agents/T03/models/<id>.templates.json (written by fetch_templates.py),
renders every case the T03 brief asks for with jinja2, and writes
agents/T03/models/<id>.renders.json.

llama.cpp conventions reproduced here (common/chat.cpp @ b11960):
  * the context always carries messages, bos_token, eos_token, enable_thinking,
    and add_generation_prompt only when it is true;
  * tools is present only when the request has tools;
  * reasoning_effort is an extra template variable; the server sets
    enable_thinking=false and drops reasoning_effort when the client sends
    "none" (tools/server/server-common.cpp);
  * tool call arguments arrive as a JSON string unless the template reads
    them as an object (common_chat.cpp func_args_not_string), so both shapes
    are rendered;
  * tojson is a plain JSON dump (nlohmann), not jinja2's HTML-safe one;
  * raise_exception aborts the render with its message.
"""
import json
import os
import sys

import jinja2

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.abspath(os.path.join(HERE, "..", "..", ".."))
MODELS = os.path.join(ROOT, "agents/T03/models")

TOOL = {
    "type": "function",
    "function": {
        "name": "get_weather",
        "description": "Get the current weather for a city.",
        "parameters": {
            "type": "object",
            "properties": {
                "city": {"type": "string", "description": "City name."},
                "units": {"type": "string", "enum": ["celsius", "fahrenheit"]},
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


def tool_call(name, args, call_id):
    """args is a dict; render.py decides whether to pass it as a dict or a JSON string."""
    return {
        "id": call_id,
        "type": "function",
        "function": {"name": name, "arguments": args},
    }


def with_arg_shape(tool_calls, shape):
    out = json.loads(json.dumps(tool_calls))
    if shape == "string":
        for tc in out:
            tc["function"]["arguments"] = json.dumps(tc["function"]["arguments"])
    return out


def cases(arg_shape):
    tc1 = tool_call("get_weather", {"city": "Paris", "units": "celsius"}, "call_1")
    tc2 = tool_call("get_time", {"city": "Paris"}, "call_2")
    return [
        ("plain", {"messages": [{"role": "user", "content": "Hello"}]}),
        ("system_user", {"messages": [
            {"role": "system", "content": "You are a helpful assistant."},
            {"role": "user", "content": "Hello"},
        ]}),
        ("tool_call", {"messages": [
            {"role": "user", "content": "What is the weather in Paris?"},
            {"role": "assistant", "content": "", "tool_calls": with_arg_shape([tc1], arg_shape)},
            {"role": "tool", "content": '{"temperature_c": 18, "conditions": "clear"}', "tool_call_id": "call_1"},
        ], "tools": [TOOL]}),
        ("parallel_tools", {"messages": [
            {"role": "user", "content": "What is the weather and the time in Paris?"},
            {"role": "assistant", "content": "", "tool_calls": with_arg_shape([tc1, tc2], arg_shape)},
            {"role": "tool", "content": '{"temperature_c": 18}', "tool_call_id": "call_1"},
            {"role": "tool", "content": '{"local_time": "12:00"}', "tool_call_id": "call_2"},
        ], "tools": [TOOL, TOOL2]}),
        ("thinking_on", {"messages": [{"role": "user", "content": "Hello"}],
                         "enable_thinking": True, "add_generation_prompt": True}),
        ("thinking_off", {"messages": [{"role": "user", "content": "Hello"}],
                          "enable_thinking": False, "add_generation_prompt": True}),
    ]


EFFORTS = ["none", "low", "medium", "high", "xhigh", "max"]


def effort_cases():
    out = []
    for level in EFFORTS:
        out.append((f"effort_{level}", {
            "messages": [{"role": "user", "content": "Hello"}],
            "enable_thinking": True,
            "add_generation_prompt": True,
            "reasoning_effort": level,
        }))
    # unknown level: llama.cpp caps probes with this to find the template fallback
    out.append(("effort_unknown", {
        "messages": [{"role": "user", "content": "Hello"}],
        "enable_thinking": True,
        "add_generation_prompt": True,
        "reasoning_effort": "REASONING_EFFORT_UNKNOWN",
    }))
    # b11960 server: reasoning_effort "none" turns thinking off and is not passed on
    out.append(("effort_none_server", {
        "messages": [{"role": "user", "content": "Hello"}],
        "enable_thinking": False,
        "add_generation_prompt": True,
    }))
    return out


def developer_case():
    return [("developer_role", {"messages": [
        {"role": "developer", "content": "You are a helpful assistant."},
        {"role": "user", "content": "Hello"},
    ]})]


def make_env():
    env = jinja2.Environment(undefined=jinja2.Undefined)
    env.filters["tojson"] = lambda v, **kw: json.dumps(v, ensure_ascii=False, sort_keys=kw.get("sort_keys", False))
    env.globals["raise_exception"] = lambda msg: (_ for _ in ()).throw(RuntimeError(str(msg)))
    env.globals["strftime_now"] = lambda fmt="%Y-%m-%d": "2026-11-06"
    return env


def render(env, source, ctx):
    try:
        return True, env.from_string(source).render(**ctx), None
    except Exception as e:  # noqa: BLE001 - the exception text is the finding
        return False, None, f"{type(e).__name__}: {e}"


def main():
    model_id = sys.argv[1]
    path = os.path.join(MODELS, f"{model_id}.templates.json")
    data = json.load(open(path))
    template = data["templates"]["tokenizer.chat_template"]
    env = make_env()

    all_cases = []
    for arg_shape in ("string", "object"):
        for name, ctx in cases(arg_shape):
            tool_case = name in ("tool_call", "parallel_tools")
            if arg_shape == "object" and not tool_case:
                continue
            c = dict(ctx)
            c.setdefault("bos_token", "")
            c.setdefault("eos_token", "")
            all_cases.append((name, arg_shape if tool_case else None, c))
    for name, ctx in effort_cases():
        c = {"bos_token": "", "eos_token": ""}
        c.update(ctx)
        all_cases.append((name, None, c))
    for name, ctx in developer_case():
        c = {"bos_token": "", "eos_token": ""}
        c.update(ctx)
        all_cases.append((name, None, c))

    results = []
    for name, arg_shape, ctx in all_cases:
        ok, out, err = render(env, template, ctx)
        case = name if arg_shape is None else f"{name}__args_{arg_shape}"
        results.append({
            "case": case,
            "arg_shape": arg_shape,
            "ok": ok,
            "output": out,
            "error": err,
            "context": ctx,
        })

    by_name = {r["case"]: r for r in results}
    effort_outputs = {r["case"]: r["output"] for r in results if r["case"].startswith("effort_")}
    distinct = {o for o in effort_outputs.values() if o is not None}

    summary = {
        "total": len(results),
        "ok": sum(1 for r in results if r["ok"]),
        "failed": sum(1 for r in results if not r["ok"]),
        "effort_distinct_renders": len(distinct),
        "thinking_on_off_differ": (
            by_name["thinking_on"]["output"] != by_name["thinking_off"]["output"]
            if by_name["thinking_on"]["ok"] and by_name["thinking_off"]["ok"] else None
        ),
    }

    out = {
        "id": model_id,
        "repo": data["repo"],
        "file": data["file"],
        "url": data["url"],
        "template_source": "tokenizer.chat_template",
        "llamacpp_tag": "b11960",
        "cases": results,
        "summary": summary,
    }
    out_path = os.path.join(MODELS, f"{model_id}.renders.json")
    with open(out_path, "w") as f:
        json.dump(out, f, indent=2, sort_keys=True)
        f.write("\n")
    print(f"{model_id}: {summary['ok']}/{summary['total']} ok, {summary['failed']} failed -> {out_path}")
    for r in results:
        if not r["ok"]:
            print(f"  FAIL {r['case']}: {r['error']}")


if __name__ == "__main__":
    main()
