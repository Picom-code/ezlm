"""Render the Gemma 4 chat template for the T03 capability matrix.

Uses the exact tokenizer.chat_template bytes from the GGUF header, Python
jinja2 with llama.cpp's context (bos/eos, messages, tools,
add_generation_prompt, raise_exception stub). Records OK output or the exact
exception for every case.
"""
import json, sys, traceback
from jinja2 import Environment, StrictUndefined, Undefined

MODELS = ["gemma-4-e4b", "gemma-4-e2b"]

BOS = "<bos>"
EOS = "<eos>"
TOOLS = [{
    "type": "function",
    "function": {
        "name": "get_weather",
        "description": "Get the current weather for a city.",
        "parameters": {
            "type": "object",
            "properties": {
                "city": {"type": "string", "description": "City name"},
                "unit": {"type": "string", "enum": ["celsius", "fahrenheit"]},
            },
            "required": ["city"],
        },
    },
}]

TOOL_CALLS = [{
    "id": "call_1",
    "type": "function",
    "function": {"name": "get_weather", "arguments": {"city": "Paris", "unit": "celsius"}},
}]
TOOL_RESULT = [{
    "role": "tool", "tool_call_id": "call_1", "name": "get_weather",
    "content": '{"temp_c": 18, "sky": "clear"}',
}]
PARALLEL_CALLS = [
    {"id": "call_1", "type": "function",
     "function": {"name": "get_weather", "arguments": {"city": "Paris"}}},
    {"id": "call_2", "type": "function",
     "function": {"name": "get_weather", "arguments": {"city": "Berlin"}}},
]
PARALLEL_RESULTS = [
    {"role": "tool", "tool_call_id": "call_1", "name": "get_weather", "content": '{"temp_c": 18}'},
    {"role": "tool", "tool_call_id": "call_2", "name": "get_weather", "content": '{"temp_c": 12}'},
]

BASE = [
    {"role": "user", "content": "What is the weather in Paris?"},
]

CASES = {
    "plain": dict(messages=BASE),
    "system_user": dict(messages=[
        {"role": "system", "content": "You are a helpful assistant."},
        {"role": "user", "content": "What is the weather in Paris?"},
    ]),
    "tool_call": dict(
        messages=[
            {"role": "user", "content": "What is the weather in Paris?"},
            {"role": "assistant", "content": "", "tool_calls": TOOL_CALLS},
            *TOOL_RESULT,
        ],
        tools=TOOLS,
    ),
    "parallel_tools": dict(
        messages=[
            {"role": "user", "content": "Weather in Paris and Berlin?"},
            {"role": "assistant", "content": "", "tool_calls": PARALLEL_CALLS},
            *PARALLEL_RESULTS,
        ],
        tools=TOOLS,
    ),
    "thinking_on": dict(messages=BASE, enable_thinking=True),
    "thinking_off": dict(messages=BASE, enable_thinking=False),
    "effort_none": dict(messages=BASE, reasoning_effort="none"),
    "effort_low": dict(messages=BASE, reasoning_effort="low"),
    "effort_medium": dict(messages=BASE, reasoning_effort="medium"),
    "effort_high": dict(messages=BASE, reasoning_effort="high"),
    "effort_xhigh": dict(messages=BASE, reasoning_effort="xhigh"),
    "effort_max": dict(messages=BASE, reasoning_effort="max"),
    "developer_role": dict(messages=[
        {"role": "developer", "content": "You are a helpful assistant."},
        {"role": "user", "content": "What is the weather in Paris?"},
    ]),
}
# Extra probes: alternative effort shapes and thinking-with-tools combinations.
EXTRA = {
    "plain_no_generation_prompt": dict(messages=BASE, add_generation_prompt=False),
    "reasoning_effort_high_add_generation_prompt": dict(
        messages=BASE, reasoning_effort="high", add_generation_prompt=True),
    "effort_nested_reasoning": dict(messages=BASE, reasoning={"effort": "high"}),
    "effort_nested_thinking": dict(messages=BASE, thinking={"effort": "high"}),
    "thinking_on_add_generation_prompt": dict(
        messages=BASE, enable_thinking=True, add_generation_prompt=True),
    "thinking_on_system_tool": dict(
        messages=[
            {"role": "system", "content": "You are a helpful assistant."},
            {"role": "user", "content": "What is the weather in Paris?"},
            {"role": "assistant", "content": "", "tool_calls": TOOL_CALLS},
            *TOOL_RESULT,
        ],
        tools=TOOLS, enable_thinking=True, add_generation_prompt=True),
    "thinking_on_with_reasoning_content": dict(
        messages=[
            {"role": "user", "content": "What is the weather in Paris?"},
            {"role": "assistant", "content": "",
             "reasoning_content": "I should call the weather tool.",
             "tool_calls": TOOL_CALLS},
            *TOOL_RESULT,
        ],
        tools=TOOLS, enable_thinking=True,
    ),
}


def make_env():
    # minja (llama.cpp) treats a missing object key as undefined, and undefined is
    # falsy there, so plain Undefined matches the server better than StrictUndefined.
    env = Environment(undefined=Undefined, keep_trailing_newline=True)
    env.filters["raise_exception"] = lambda msg: (_ for _ in ()).throw(Exception(msg))
    return env


def render(env, template, **kw):
    # llama.cpp always passes add_generation_prompt=true for chat completions.
    ctx = dict(bos_token=BOS, eos_token=EOS, add_generation_prompt=True)
    ctx.update(kw)
    ctx.setdefault("messages", [])
    ctx.setdefault("tools", None)
    # llama.cpp raises from inside the template through this hook.
    def raise_exception(msg):
        raise Exception(msg)
    ctx["raise_exception"] = raise_exception
    try:
        out = env.from_string(template).render(**ctx)
        return {"ok": True, "output": out}
    except Exception as e:
        return {"ok": False, "error_type": type(e).__name__, "error": str(e),
                "traceback_tail": traceback.format_exc().strip().splitlines()[-3:]}


def main():
    for mid in MODELS:
        tmpl = json.load(open(f"agents/T03/models/{mid}.templates.json"))
        src = tmpl["tokenizer.chat_template"]
        env = make_env()
        results = {}
        for name, kw in {**CASES, **EXTRA}.items():
            results[name] = render(env, src, **kw)
        row = {
            "id": mid,
            "template_vars": sorted({"messages", "tools", "add_generation_prompt",
                                     "bos_token", "eos_token", "enable_thinking",
                                     "preserve_thinking", "raise_exception"}),
            "cases": results,
        }
        with open(f"agents/T03/models/{mid}.renders.json", "w") as f:
            json.dump(row, f, indent=1, sort_keys=True)
            f.write("\n")
        ok = sum(1 for r in results.values() if r["ok"])
        print(f"{mid}: {ok}/{len(results)} rendered OK")


if __name__ == "__main__":
    main()
