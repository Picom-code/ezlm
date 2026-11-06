#!/usr/bin/env python3
"""Render the GGUF chat templates for T03 agent a06 (bonsai-27b-1bit, qwen3.5-9b).

The templates are read from the GGUF header with HTTP range requests (the
Reader class from scripts/verify_catalog.py) and stored verbatim in
agents/T03/models/<id>.templates.json. Rendering follows llama.cpp's own
scripts/jinja/jinja-tester.py at tag b11960: ImmutableSandboxedEnvironment
with trim_blocks, lstrip_blocks, loopcontrols, a raise_exception global and a
json.dumps tojson filter.

  ~/ezlm-agents/run/venv/bin/python agents/T03/a06/render.py --fetch
  ~/ezlm-agents/run/venv/bin/python agents/T03/a06/render.py
"""
import argparse, json, os, sys

import jinja2
from jinja2.sandbox import ImmutableSandboxedEnvironment
from jinja2.ext import loopcontrols

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__)))))
MODELS = {
    "bonsai-27b-1bit": {
        "repo": "prism-ml/Bonsai-27B-gguf",
        "file": "Bonsai-27B-Q1_0.gguf",
    },
    "qwen3.5-9b": {
        "repo": "unsloth/Qwen3.5-9B-GGUF",
        "file": "Qwen3.5-9B-UD-IQ2_XXS.gguf",
    },
}
UA = {"User-Agent": "ezlm-t03-a06"}


def gguf_header(url):
    """Return the GGUF metadata dict, reading only the header with range requests."""
    sys.path.insert(0, os.path.join(ROOT, "scripts"))
    import verify_catalog as v
    v.UA = UA
    ver, kv, _types = v.header(url)
    return ver, kv


def fetch_templates():
    os.makedirs(os.path.join(ROOT, "agents/T03/models"), exist_ok=True)
    for mid, m in MODELS.items():
        url = f"https://huggingface.co/{m['repo']}/resolve/main/{m['file']}"
        ver, kv = gguf_header(url)
        tpls = {k: v for k, v in kv.items() if k.startswith("tokenizer.chat_template")}
        if "tokenizer.chat_template" not in tpls:
            raise SystemExit(f"{mid}: no tokenizer.chat_template in {url}")
        out = {
            "id": mid,
            "repo": m["repo"],
            "file": m["file"],
            "url": url,
            "gguf_version": ver,
            "source": "GGUF metadata, HTTP range request, header only",
            "templates": dict(sorted(tpls.items())),
        }
        path = os.path.join(ROOT, f"agents/T03/models/{mid}.templates.json")
        with open(path, "w") as f:
            json.dump(out, f, indent=1)
            f.write("\n")
        print(f"{mid}: {len(tpls)} template(s), {sum(len(v) for v in tpls.values())} bytes -> {path}")


TOOLS = [{
    "type": "function",
    "function": {
        "name": "get_weather",
        "description": "Get the current weather for a city.",
        "parameters": {
            "type": "object",
            "properties": {"city": {"type": "string", "description": "City name."}},
            "required": ["city"],
        },
    },
}]


def tool_call(name="get_weather", city="Paris", cid="call_1"):
    return {
        "id": cid,
        "type": "function",
        "function": {"name": name, "arguments": {"city": city}},
    }


def tool_result(content='{"temp_c": 21}', cid="call_1"):
    return {"role": "tool", "content": content, "tool_call_id": cid}


# Each case: (messages, extra template variables). add_generation_prompt is
# always passed explicitly so the record shows what the server would do.
def cases():
    user = [{"role": "user", "content": "What is the weather in Paris?"}]
    sysuser = [{"role": "system", "content": "You are a helpful assistant."},
               {"role": "user", "content": "What is 2 + 2?"}]
    tc = [
        {"role": "user", "content": "What is the weather in Paris?"},
        {"role": "assistant", "content": "", "tool_calls": [tool_call()]},
        tool_result(),
    ]
    par = [
        {"role": "user", "content": "What is the weather in Paris and in Rome?"},
        {"role": "assistant", "content": "", "tool_calls": [
            tool_call(city="Paris", cid="call_1"),
            tool_call(city="Rome", cid="call_2"),
        ]},
        tool_result('{"temp_c": 21}', "call_1"),
        tool_result('{"temp_c": 25}', "call_2"),
    ]
    out = [
        ("plain", user, {"add_generation_prompt": True}),
        ("system_user", sysuser, {"add_generation_prompt": True}),
        ("tool_call", tc, {"add_generation_prompt": True, "tools": TOOLS}),
        ("parallel_tools", par, {"add_generation_prompt": True, "tools": TOOLS}),
        ("thinking_on", user, {"add_generation_prompt": True, "enable_thinking": True}),
        ("thinking_off", user, {"add_generation_prompt": True, "enable_thinking": False}),
        ("thinking_on_alt_var", user, {"add_generation_prompt": True, "thinking": True}),
        ("thinking_off_alt_var", user, {"add_generation_prompt": True, "thinking": False}),
        ("no_generation_prompt", user, {"add_generation_prompt": False}),
        ("developer_role", [{"role": "developer", "content": "You are terse."}] + user,
         {"add_generation_prompt": True}),
        ("system_not_first", [{"role": "user", "content": "Hi"},
                              {"role": "system", "content": "You are terse."}],
         {"add_generation_prompt": True}),
        ("empty_messages", [], {"add_generation_prompt": True}),
        ("tool_call_string_args", [
            {"role": "user", "content": "What is the weather in Paris?"},
            {"role": "assistant", "content": "", "tool_calls": [{
                "id": "call_1", "type": "function",
                "function": {"name": "get_weather", "arguments": '{"city": "Paris"}'},
            }]},
            tool_result(),
        ], {"add_generation_prompt": True, "tools": TOOLS}),
    ]
    for level in ("none", "low", "medium", "high", "xhigh", "max"):
        out.append((f"effort_{level}", user,
                    {"add_generation_prompt": True, "reasoning_effort": level}))
    for shape, extra in (("reasoning_nested", {"reasoning": {"effort": "high"}}),
                         ("thinking_nested", {"thinking": {"effort": "high"}})):
        out.append((f"effort_high_{shape}", user,
                    {"add_generation_prompt": True, **extra}))
    return out


def make_env():
    def raise_exception(text):
        raise RuntimeError(text)

    env = ImmutableSandboxedEnvironment(
        trim_blocks=True,
        lstrip_blocks=True,
        extensions=[loopcontrols],
    )
    env.filters["tojson"] = (
        lambda x, indent=None, separators=None, sort_keys=False, ensure_ascii=False:
        json.dumps(x, indent=indent, separators=separators,
                   sort_keys=sort_keys, ensure_ascii=ensure_ascii)
    )
    env.globals["raise_exception"] = raise_exception
    return env


def render_all():
    env = make_env()
    for mid in MODELS:
        tpath = os.path.join(ROOT, f"agents/T03/models/{mid}.templates.json")
        with open(tpath) as f:
            tdata = json.load(f)
        results = []
        for name, messages, extra in cases():
            template = env.from_string(tdata["templates"]["tokenizer.chat_template"])
            ctx = {"messages": messages, "tools": [], "bos_token": "", "eos_token": ""}
            ctx.update(extra)
            rec = {
                "case": name,
                "template": "tokenizer.chat_template",
                "context": {k: v for k, v in extra.items() if k != "messages"},
            }
            try:
                text = template.render(ctx)
                rec["ok"] = True
                rec["output"] = text
            except Exception as e:  # noqa: BLE001 - record the exact failure
                rec["ok"] = False
                rec["error_type"] = type(e).__name__
                rec["error"] = str(e)
            results.append(rec)
        out = {
            "id": mid,
            "repo": MODELS[mid]["repo"],
            "file": MODELS[mid]["file"],
            "template_source": "tokenizer.chat_template",
            "jinja2": jinja2.__version__,
            "conventions": "llama.cpp b11960 scripts/jinja/jinja-tester.py: ImmutableSandboxedEnvironment(trim_blocks, lstrip_blocks, loopcontrols), raise_exception raises RuntimeError, tojson = json.dumps(ensure_ascii=False)",
            "cases": results,
        }
        opath = os.path.join(ROOT, f"agents/T03/models/{mid}.renders.json")
        with open(opath, "w") as f:
            json.dump(out, f, indent=1)
            f.write("\n")
        ok = sum(1 for r in results if r["ok"])
        print(f"{mid}: {ok}/{len(results)} rendered, {len(results) - ok} raised -> {opath}")


if __name__ == "__main__":
    ap = argparse.ArgumentParser()
    ap.add_argument("--fetch", action="store_true", help="re-read the GGUF headers first")
    a = ap.parse_args()
    if a.fetch:
        fetch_templates()
    render_all()
