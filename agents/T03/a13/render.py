#!/usr/bin/env python3
"""Render the Nemotron 3.5 chat templates for agent a13 of T03.

Reads tokenizer.chat_template from each model's GGUF header with HTTP range
requests (via scripts/verify_catalog.py), renders the T03 case list with
jinja2 following llama.cpp b11960's conventions, and writes:

  agents/T03/models/<id>.templates.json  raw templates, exact
  agents/T03/models/<id>.renders.json    per-case output or exception

Run from the worktree root:
  ~/ezlm-agents/run/venv/bin/python agents/T03/a13/render.py
"""
import json
import os
import sys
from urllib.request import urlopen

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.abspath(os.path.join(HERE, "..", "..", ".."))
sys.path.insert(0, os.path.join(ROOT, "scripts"))
import verify_catalog as v  # noqa: E402

OUT = os.path.join(ROOT, "agents", "T03", "models")

MODELS = {
    "nemotron-3.5-bolt-74b-a9b": {
        "repo": "nvidia/NVIDIA-Nemotron-3.5-Bolt-74B-A9B-24GB-GGUF",
        "file": "NVIDIA-Nemotron-3.5-Bolt-74B-A9B-24GB.gguf",
        "quant": "NVIDIA-24GB (IQ2_XXS QAD)",
    },
    "nemotron-3.5-lightning-30b-a3b": {
        "repo": "unsloth/NVIDIA-Nemotron-3.5-Lightning-30B-A3B-GGUF",
        "file": "NVIDIA-Nemotron-3.5-Lightning-30B-A3B-UD-IQ3_XXS.gguf",
        "quant": "UD-IQ3_XXS",
    },
}

WEATHER_TOOL = {
    "type": "function",
    "function": {
        "name": "get_weather",
        "description": "Get the current weather for a city.",
        "parameters": {
            "type": "object",
            "properties": {
                "city": {"type": "string", "description": "City name, for example Paris."},
                "units": {"type": "string", "enum": ["c", "f"]},
            },
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


def tool_call(name, args, call_id):
    # llama.cpp passes arguments as an object when caps.supports_object_arguments,
    # which is the case here (the template reads tool_call.arguments|items).
    return {
        "id": call_id,
        "type": "function",
        "function": {"name": name, "arguments": args},
    }


BASE = [{"role": "user", "content": "What is the weather in Paris?"}]

TOOL_HISTORY = [
    {"role": "user", "content": "What is the weather in Paris?"},
    {"role": "assistant", "content": "", "tool_calls": [tool_call("get_weather", {"city": "Paris", "units": "c"}, "call_1")]},
    {"role": "tool", "tool_call_id": "call_1", "content": '{"temp_c": 18, "conditions": "cloudy"}'},
]

PARALLEL_HISTORY = [
    {"role": "user", "content": "What is the weather and the time in Paris?"},
    {
        "role": "assistant",
        "content": "",
        "tool_calls": [
            tool_call("get_weather", {"city": "Paris", "units": "c"}, "call_1"),
            tool_call("get_time", {"city": "Paris"}, "call_2"),
        ],
    },
    {"role": "tool", "tool_call_id": "call_1", "content": '{"temp_c": 18}'},
    {"role": "tool", "tool_call_id": "call_2", "content": '{"local_time": "12:00"}'},
]


def cases(model_id):
    out = [
        ("plain", {"messages": BASE, "add_generation_prompt": True}),
        ("system_user", {"messages": [{"role": "system", "content": "You are a terse assistant."}] + BASE, "add_generation_prompt": True}),
        ("tool_call", {"messages": TOOL_HISTORY, "tools": [WEATHER_TOOL], "add_generation_prompt": True}),
        ("parallel_tools", {"messages": PARALLEL_HISTORY, "tools": [WEATHER_TOOL, TIME_TOOL], "add_generation_prompt": True}),
        ("thinking_on", {"messages": BASE, "add_generation_prompt": True, "enable_thinking": True}),
        ("thinking_off", {"messages": BASE, "add_generation_prompt": True, "enable_thinking": False}),
        ("developer_role", {"messages": [{"role": "developer", "content": "You are a terse assistant."}] + BASE, "add_generation_prompt": True}),
    ]
    for level in ("none", "low", "medium", "high", "xhigh", "max"):
        out.append((f"effort_{level}", {
            "messages": BASE,
            "add_generation_prompt": True,
            "enable_thinking": True,
            "reasoning_effort": level,
            "reasoning_strength": level,  # llama.cpp sets both from reasoning_effort
        }))
    # extra: bolt reads a boolean medium_effort, not the OAI string; record it.
    if model_id == "nemotron-3.5-bolt-74b-a9b":
        out.append(("extra_medium_effort_true", {
            "messages": BASE, "add_generation_prompt": True, "enable_thinking": True, "medium_effort": True,
        }))
        out.append(("extra_medium_effort_false", {
            "messages": BASE, "add_generation_prompt": True, "enable_thinking": True, "medium_effort": False,
        }))
    # extra: preserve_reasoning is a server kwarg that flips truncate_history_thinking.
    out.append(("extra_preserve_reasoning", {
        "messages": TOOL_HISTORY,
        "tools": [WEATHER_TOOL],
        "add_generation_prompt": True,
        "preserve_reasoning": True,
        "truncate_history_thinking": False,
    }))
    return out


def tojson(value):
    # llama.cpp's tojson is a plain JSON dump: no HTML escaping.
    return json.dumps(value, ensure_ascii=False)


def items(value):
    if isinstance(value, dict):
        return list(value.items())
    if isinstance(value, str):
        try:
            parsed = json.loads(value)
        except ValueError:
            return []
        return list(parsed.items()) if isinstance(parsed, dict) else []
    return list(enumerate(value))


def render(env, src, ctx):
    tmpl = env.from_string(src)
    return tmpl.render(**ctx)


def main():
    from jinja2 import Environment

    def raise_exception(msg):
        raise RuntimeError(str(msg))

    env = Environment(autoescape=False)
    env.filters["tojson"] = tojson
    env.filters["items"] = items
    env.globals["raise_exception"] = raise_exception
    env.globals["strftime_now"] = lambda fmt: "2026-11-06"
    os.makedirs(OUT, exist_ok=True)

    for mid, meta in MODELS.items():
        url = f"https://huggingface.co/{meta['repo']}/resolve/main/{meta['file']}"
        _, kv, _ = v.header(url)
        templates = {k: kv[k] for k in kv if k.startswith("tokenizer.chat_template")}
        if not templates:
            raise SystemExit(f"{mid}: no tokenizer.chat_template in the GGUF header")
        with open(os.path.join(OUT, f"{mid}.templates.json"), "w") as f:
            json.dump(templates, f, indent=1, ensure_ascii=False)
            f.write("\n")
        src = templates["tokenizer.chat_template"]
        results = []
        for name, ctx in cases(mid):
            base = {"bos_token": "", "eos_token": "", "tools": [], "add_generation_prompt": True}
            base.update(ctx)
            try:
                text = render(env, src, base)
                results.append({"case": name, "ok": True, "output": text})
            except Exception as e:  # noqa: BLE001 - record the exact failure
                results.append({"case": name, "ok": False, "error": f"{type(e).__name__}: {e}"})
        # hash of the plain output lets a15 spot silently-ignored variables
        plain = next(r["output"] for r in results if r["case"] == "plain")
        thinking = next(r["output"] for r in results if r["case"] == "thinking_on")
        for r in results:
            if r["ok"]:
                r["same_as_plain"] = r["output"] == plain
                r["same_as_thinking_on"] = r["output"] == thinking
        with open(os.path.join(OUT, f"{mid}.renders.json"), "w") as f:
            json.dump({
                "id": mid,
                "repo": meta["repo"],
                "file": meta["file"],
                "quant": meta["quant"],
                "template_source": "tokenizer.chat_template",
                "jinja2_version": __import__("jinja2").__version__,
                "llamacpp_conventions": {
                    "raise_exception": "stub raises RuntimeError(message)",
                    "tojson": "plain JSON dump, no HTML escaping (llama.cpp/minja style)",
                    "items": "mapping -> (key, value), string -> parsed JSON object items, list -> (index, value)",
                    "arguments": "tool_call arguments are objects (llama.cpp func_args_not_string)",
                    "context": "messages, tools, add_generation_prompt, enable_thinking, reasoning_effort/reasoning_strength, truncate_history_thinking, bos_token/eos_token",
                },
                "cases": results,
            }, f, indent=1, ensure_ascii=False)
            f.write("\n")
        bad = [r["case"] for r in results if not r["ok"]]
        print(f"{mid}: {len(results)} cases, {len(results) - len(bad)} rendered, failed={bad}")


if __name__ == "__main__":
    main()
