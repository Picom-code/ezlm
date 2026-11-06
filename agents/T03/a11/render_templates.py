#!/usr/bin/env python3
"""Render the GGUF chat templates of the models assigned to agent a11 with
jinja2, using llama.cpp's request context, and record what renders and what
raises.

  ~/ezlm-agents/run/venv/bin/python agents/T03/a11/render_templates.py

Writes agents/T03/models/<id>.renders.json. Template text comes from the
.templates.json files written by fetch_templates.py.
"""
import json, os, sys, time
from datetime import datetime

import jinja2
from jinja2.sandbox import ImmutableSandboxedEnvironment

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.abspath(os.path.join(HERE, "..", "..", ".."))
OUT = os.path.join(ROOT, "agents", "T03", "models")

BOS = {"muse-glimmer-30b": "<|begin_of_text|>", "mistral-small-4-119b": "<s>"}
EOS = {"muse-glimmer-30b": "<|end_of_text|>", "mistral-small-4-119b": "</s>"}

TOOLS = [{
    "type": "function",
    "function": {
        "name": "get_weather",
        "description": "Current weather for a city.",
        "parameters": {
            "type": "object",
            "properties": {"city": {"type": "string", "description": "City name."}},
            "required": ["city"],
        },
    },
}]

USER = [{"role": "user", "content": "What is the weather in Paris?"}]
SYSTEM_USER = [
    {"role": "system", "content": "You are a terse assistant."},
    {"role": "user", "content": "What is the weather in Paris?"},
]
TOOL_CALL = [
    {"role": "user", "content": "What is the weather in Paris?"},
    {"role": "assistant", "content": "", "tool_calls": [{
        "id": "call_1", "type": "function",
        "function": {"name": "get_weather", "arguments": {"city": "Paris"}},
    }]},
    {"role": "tool", "tool_call_id": "call_1", "name": "get_weather",
     "content": '{"temp_c": 18}'},
]
TOOL_CALL_STRING_ARGS = [
    {"role": "user", "content": "What is the weather in Paris?"},
    {"role": "assistant", "content": "", "tool_calls": [{
        "id": "call_1", "type": "function",
        "function": {"name": "get_weather", "arguments": '{"city": "Paris"}'},
    }]},
    {"role": "tool", "tool_call_id": "call_1", "name": "get_weather",
     "content": '{"temp_c": 18}'},
]
PARALLEL_TOOLS = [
    {"role": "user", "content": "Compare the weather in Paris and Rome."},
    {"role": "assistant", "content": "", "tool_calls": [
        {"id": "call_1", "type": "function",
         "function": {"name": "get_weather", "arguments": {"city": "Paris"}}},
        {"id": "call_2", "type": "function",
         "function": {"name": "get_weather", "arguments": {"city": "Rome"}}},
    ]},
    {"role": "tool", "tool_call_id": "call_1", "name": "get_weather", "content": '{"temp_c": 18}'},
    {"role": "tool", "tool_call_id": "call_2", "name": "get_weather", "content": '{"temp_c": 24}'},
]
DEVELOPER_ROLE = [
    {"role": "developer", "content": "You are a terse assistant."},
    {"role": "user", "content": "What is the weather in Paris?"},
]

EFFORTS = ["none", "low", "medium", "high", "xhigh", "max", "minimal"]


def effort_ctx(effort):
    """Context for an OpenAI reasoning_effort value, as b11960 builds it.

    server-common.cpp:1497-1506: "none" turns thinking off and drops the kwarg,
    so the template falls back to its own default. Any other value reaches the
    template, and common/jinja/caps.cpp:36-40 (caps_apply_reasoning_effort)
    binds it to both reasoning_effort and reasoning_strength.
    """
    if effort == "none":
        return {"messages": USER, "enable_thinking": False}
    return {"messages": USER, "reasoning_effort": effort, "reasoning_strength": effort}


def base_cases():
    cases = [
        ("plain", {"messages": USER}),
        ("plain_no_gen_prompt", {"messages": USER, "add_generation_prompt": False}),
        ("system_user", {"messages": SYSTEM_USER}),
        ("tool_call", {"messages": TOOL_CALL, "tools": TOOLS}),
        ("tool_call_string_args", {"messages": TOOL_CALL_STRING_ARGS, "tools": TOOLS}),
        ("parallel_tools", {"messages": PARALLEL_TOOLS, "tools": TOOLS}),
        ("thinking_on", {"messages": USER, "enable_thinking": True}),
        ("thinking_off", {"messages": USER, "enable_thinking": False}),
        ("developer_role", {"messages": DEVELOPER_ROLE}),
    ]
    for e in EFFORTS:
        cases.append((f"effort_{e}", effort_ctx(e)))
    return cases


EXTRA = {
    "muse-glimmer-30b": [
        # this template names its effort variable reasoning_strength, and the
        # server sets both reasoning_effort and reasoning_strength
        # raw template variable only, without the server's caps_apply_reasoning_effort aliasing
        ("reasoning_effort_high_raw", {"messages": USER, "reasoning_effort": "high"}),
        ("reasoning_strength_high", {"messages": USER, "reasoning_strength": "high"}),
        ("reasoning_strength_none", {"messages": USER, "reasoning_strength": "none"}),
        ("reasoning_strength_xhigh", {"messages": USER, "reasoning_strength": "xhigh"}),
        ("reasoning_strength_garbage", {"messages": USER, "reasoning_strength": "REASONING_EFFORT_UNKNOWN"}),
        # shapes the template does not read
        ("thinking_var_on", {"messages": USER, "thinking": True}),
        ("reasoning_effort_dict", {"messages": USER, "reasoning": {"effort": "high"}}),
        # tool calls whose arguments are a JSON string (what many OpenAI clients send)
        ("tool_call_json_string_args_only", {"messages": TOOL_CALL_STRING_ARGS, "tools": TOOLS}),
    ],
    "mistral-small-4-119b": [
        ("reasoning_effort_none_kwarg", {"messages": USER, "reasoning_effort": "none"}),
        ("thinking_dict", {"messages": USER, "thinking": {"type": "enabled"}}),
        ("reasoning_effort_dict", {"messages": USER, "reasoning": {"effort": "high"}}),
        ("system_not_first", {"messages": [
            {"role": "user", "content": "Hi."},
            {"role": "system", "content": "You are a terse assistant."},
            {"role": "user", "content": "What is the weather in Paris?"},
        ]}),
    ],
}


def raise_exception(msg):
    raise Exception(msg)


def make_env():
    env = ImmutableSandboxedEnvironment(trim_blocks=True, lstrip_blocks=True)
    env.globals["raise_exception"] = raise_exception
    env.globals["strftime_now"] = lambda fmt="%Y-%m-%d": time.strftime(fmt)
    # transformers and llama.cpp render tojson with sorted keys and no ASCII escapes
    env.filters["tojson"] = lambda v: json.dumps(v, ensure_ascii=False, sort_keys=True)
    return env


def render(env, src, ctx):
    tmpl = env.from_string(src)
    return tmpl.render(**ctx)


def main():
    env = make_env()
    for mid in ("muse-glimmer-30b", "mistral-small-4-119b"):
        data = json.load(open(os.path.join(OUT, mid + ".templates.json")))
        src = data["templates"]["tokenizer.chat_template"]
        cases = base_cases() + EXTRA[mid]
        out = []
        for name, extra in cases:
            ctx = {
                "messages": extra.get("messages"),
                "tools": extra.get("tools", []),
                "add_generation_prompt": extra.get("add_generation_prompt", True),
                "bos_token": BOS[mid],
                "eos_token": EOS[mid],
            }
            for k, v in extra.items():
                if k not in ("messages", "tools", "add_generation_prompt"):
                    ctx[k] = v
            rec = {"name": name,
                   "vars": {k: v for k, v in extra.items()},
                   "ok": True, "output": None, "error": None}
            try:
                rec["output"] = render(env, src, ctx)
            except Exception as e:
                rec["ok"] = False
                rec["error"] = str(e)
            out.append(rec)

        # does the case change anything relative to a plain user turn?
        plain = next(c["output"] for c in out if c["name"] == "plain")
        for c in out:
            if c["ok"]:
                c["same_as_plain"] = c["output"] == plain
        raised = [c["name"] for c in out if not c["ok"]]
        doc = {
            "id": mid,
            "repo": data["repo"],
            "file": data["file"],
            "template_source": "tokenizer.chat_template",
            "gguf_source": data["source"],
            "rendered_at": datetime.now().astimezone().isoformat(timespec="seconds"),
            "python": sys.version.split()[0],
            "jinja2": jinja2.__version__,
            "environment": "jinja2 ImmutableSandboxedEnvironment(trim_blocks=True, lstrip_blocks=True); "
                           "raise_exception raises; strftime_now=time.strftime; "
                           "tojson=json.dumps(ensure_ascii=False, sort_keys=True); "
                           "bos/eos from the GGUF tokenizer metadata",
            "context_defaults": {"add_generation_prompt": True, "bos_token": BOS[mid],
                                 "eos_token": EOS[mid], "tools": []},
            "cases": out,
            "summary": {"total": len(out), "ok": len(out) - len(raised),
                        "raised": len(raised), "raised_cases": raised},
        }
        path = os.path.join(OUT, mid + ".renders.json")
        with open(path, "w") as f:
            json.dump(doc, f, indent=1, ensure_ascii=False)
            f.write("\n")
        print(f"{mid}: {len(out) - len(raised)}/{len(out)} rendered, raised: {raised}")


if __name__ == "__main__":
    main()
