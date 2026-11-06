#!/usr/bin/env python3
"""T03 slice a05: read chat templates from GGUF headers and render them with
jinja2 using llama.cpp b11960 conventions.

  agents/T03/a05/render.py [model-id ...]

Writes agents/T03/models/<id>.templates.json and <id>.renders.json.
Range requests only: the weights are never downloaded.
"""
import json
import os
import re
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.abspath(os.path.join(HERE, "..", "..", ".."))
sys.path.insert(0, os.path.join(ROOT, "scripts"))

from jinja2 import ChainableUndefined, Environment  # noqa: E402
from jinja2.exceptions import TemplateError  # noqa: E402

import verify_catalog as vc  # noqa: E402

MODELS = os.path.join(ROOT, "agents", "T03", "models")
CATALOG = os.path.join(ROOT, "internal", "catalog", "catalog.json")

# llama.cpp b11960: common/jinja/caps.cpp caps_apply_reasoning_effort() binds a
# flat `reasoning_effort` (and alias `reasoning_strength`); the server turns an
# OAI "none" into enable_thinking=false and drops the kwarg.
EFFORT_LEVELS = ["none", "minimal", "low", "medium", "high", "xhigh", "max"]

# llama.cpp b11960 common/chat.cpp: to_json_oaicompat() renders function
# arguments as a JSON string; workaround::func_args_not_string() parses it to an
# object when the template's caps say supports_object_arguments.
TOOL = {
    "type": "function",
    "function": {
        "name": "get_weather",
        "description": "Get the current weather for a city",
        "parameters": {
            "type": "object",
            "properties": {"city": {"type": "string", "description": "City name"}},
            "required": ["city"],
        },
    },
}
CALL = {"type": "function", "id": "call_1",
        "function": {"name": "get_weather", "arguments": '{"city": "Paris"}'}}
CALL2 = {"type": "function", "id": "call_2",
         "function": {"name": "get_weather", "arguments": '{"city": "Berlin"}'}}


def catalog_entry(model_id):
    cat = json.load(open(CATALOG))
    for m in cat["models"]:
        if m["id"] == model_id:
            q = m["quants"][0]
            return m, (q.get("repo") or m["repo"]), q["files"][0]
    raise SystemExit(f"{model_id} not in catalog")


def load_templates(model_id):
    """Read tokenizer.chat_template* from the GGUF header (range requests)."""
    m, repo, file = catalog_entry(model_id)
    url = f"https://huggingface.co/{repo}/resolve/main/{file}"
    _, kv, _ = vc.header(url)
    templates = {k: kv[k] for k in sorted(kv) if k == "tokenizer.chat_template"
                 or k.startswith("tokenizer.chat_template.")}
    doc = {"id": model_id, "repo": repo, "file": file,
           "gguf": {"architecture": kv.get("general.architecture"),
                    "chat_template_keys": sorted(templates)},
           "templates": templates}
    path = os.path.join(MODELS, f"{model_id}.templates.json")
    os.makedirs(MODELS, exist_ok=True)
    json.dump(doc, open(path, "w"), indent=1, ensure_ascii=False)
    open(path, "a").write("\n")
    return m, repo, file, templates


def env():
    e = Environment(undefined=ChainableUndefined, autoescape=False,
                    keep_trailing_newline=True)

    def raise_exception(msg=""):
        raise TemplateError(f"raise_exception: {msg}")

    e.globals["raise_exception"] = raise_exception
    return e


def render(template, ctx):
    try:
        return True, env().from_string(template).render(**ctx), None
    except Exception as exc:  # record the exact failure text
        return False, None, f"{type(exc).__name__}: {exc}"


def cases(template):
    """Case name -> template context, following llama.cpp b11960 defaults."""
    base = {"messages": [{"role": "user", "content": "What is the weather in Paris?"}],
            "add_generation_prompt": True, "enable_thinking": True,
            "bos_token": "<bos>", "eos_token": "<end_of_turn>"}
    tool_msgs = [
        {"role": "user", "content": "What is the weather in Paris?"},
        {"role": "assistant", "content": "", "tool_calls": [CALL]},
        {"role": "tool", "content": '{"temp_c": 21}', "tool_call_id": "call_1"},
    ]
    parallel_msgs = [
        {"role": "user", "content": "Weather in Paris and Berlin?"},
        {"role": "assistant", "content": "", "tool_calls": [CALL, CALL2]},
        {"role": "tool", "content": '{"temp_c": 21}', "tool_call_id": "call_1"},
        {"role": "tool", "content": '{"temp_c": 19}', "tool_call_id": "call_2"},
    ]
    out = {
        "plain": dict(base),
        "system_user": dict(base, messages=[
            {"role": "system", "content": "You are a terse weather bot."},
            {"role": "user", "content": "What is the weather in Paris?"}]),
        "tool_call": dict(base, messages=tool_msgs, tools=[TOOL]),
        "parallel_tools": dict(base, messages=parallel_msgs, tools=[TOOL]),
        "thinking_on": dict(base, enable_thinking=True),
        "thinking_off": dict(base, enable_thinking=False),
        "developer_role": dict(base, messages=[
            {"role": "developer", "content": "Answer in Celsius."},
            {"role": "user", "content": "What is the weather in Paris?"}]),
        "add_generation_prompt_off": dict(base, add_generation_prompt=False),
        "assistant_reasoning_history": dict(base, messages=[
            {"role": "user", "content": "What is the weather in Paris?"},
            {"role": "assistant", "content": "It is mild.",
             "reasoning_content": "Check the forecast first."},
            {"role": "user", "content": "And in Berlin?"}]),
    }
    for level in EFFORT_LEVELS:
        out[f"effort_{level}"] = dict(base, reasoning_effort=level)
    # shapes some templates use instead of the flat b11960 names
    out["effort_nested_reasoning"] = dict(base, reasoning={"effort": "high"})
    out["effort_nested_thinking"] = dict(base, thinking={"effort": "high"})
    out["thinking_var_on"] = dict(base, thinking=True)
    out["thinking_var_off"] = dict(base, thinking=False)
    # object arguments, as b11960 sends after workaround::func_args_not_string()
    obj_call = {"type": "function", "id": "call_1",
                "function": {"name": "get_weather", "arguments": {"city": "Paris"}}}
    out["tool_call_object_args"] = dict(
        base, messages=[tool_msgs[0],
                         {"role": "assistant", "content": "", "tool_calls": [obj_call]},
                         tool_msgs[2]], tools=[TOOL])
    return out


def vars_referenced(template):
    """Template variables the template actually reads (identifier match, so
    `reasoning_content` does not count as `reasoning`)."""
    names = {"enable_thinking", "thinking", "reasoning_effort", "reasoning_strength",
             "reasoning", "effort", "preserve_thinking", "clear_thinking",
             "truncate_history_thinking", "drop_thinking", "tools",
             "add_generation_prompt", "messages", "bos_token", "eos_token"}
    code = re.sub(r"\{#.*?#\}", "", template, flags=re.S)      # comments
    code = re.sub(r"'(?:[^'\\]|\\.)*'|\"(?:[^\"\\]|\\.)*\"", "", code)  # strings
    return sorted(names & set(re.findall(r"[A-Za-z_][A-Za-z0-9_]*", code)))


def main(ids):
    for model_id in ids:
        m, repo, file, templates = load_templates(model_id)
        tmpl = templates["tokenizer.chat_template"]
        rows = {}
        for name, ctx in cases(tmpl).items():
            ok, out, err = render(tmpl, ctx)
            rows[name] = {"ok": ok, "context": {k: v for k, v in ctx.items()
                                                 if k not in ("bos_token", "eos_token")},
                          "prompt": out, "error": err}
        doc = {"id": model_id, "repo": repo, "file": file,
               "template_source": "tokenizer.chat_template",
               "jinja2": True,
               "llamacpp_defaults": {"enable_thinking": True, "add_generation_prompt": True},
               "vars_referenced": vars_referenced(tmpl),
               "renders": rows}
        path = os.path.join(MODELS, f"{model_id}.renders.json")
        json.dump(doc, open(path, "w"), indent=1, ensure_ascii=False)
        open(path, "a").write("\n")
        ok = sum(1 for r in rows.values() if r["ok"])
        print(f"{model_id}: {ok}/{len(rows)} cases rendered, vars={doc['vars_referenced']}")


if __name__ == "__main__":
    main(sys.argv[1:] or ["bonsai-8b-1bit", "gemma-4-26b-a4b"])
