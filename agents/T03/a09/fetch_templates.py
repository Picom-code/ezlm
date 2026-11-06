#!/usr/bin/env python3
"""Save the raw chat templates from each model's GGUF header.

Only the header is fetched (HTTP range requests), never the weights:
scripts/verify_catalog.py already has a Reader that walks the GGUF key-value
section, so this imports it.

  ~/ezlm-agents/run/venv/bin/python agents/T03/a09/fetch_templates.py [id ...]

Writes agents/T03/models/<id>.templates.json.
"""
import json, os, sys

HERE = os.path.dirname(os.path.abspath(__file__))            # agents/T03/a09
OUT = os.path.join(os.path.dirname(HERE), "models")          # agents/T03/models
REPO = os.path.dirname(os.path.dirname(os.path.dirname(HERE)))  # worktree root
sys.path.insert(0, os.path.join(REPO, "scripts"))
import verify_catalog as v  # noqa: E402

# id -> (repo, file), taken from internal/catalog/catalog.json: `repo` of the
# first quant, which is the file EZLM downloads first.
MODELS = {
    "gemma-4-12b": ("unsloth/gemma-4-12b-it-GGUF", "gemma-4-12b-it-UD-IQ2_M.gguf"),
    "lfm3-1.7b": ("LiquidAI/LFM3-1.7B-GGUF", "LFM3-1.7B-QAD-Q4_0.gguf"),
}


def fetch(mid):
    repo, name = MODELS[mid]
    url = f"https://huggingface.co/{repo}/resolve/main/{name}"
    ver, kv, _types = v.header(url)  # header() returns (version, key-values, tensor types)
    named = {k[len("tokenizer.chat_template."):]: kv[k]
             for k in kv if k.startswith("tokenizer.chat_template.")}
    out = {
        "id": mid,
        "repo": repo,
        "file": name,
        "url": url,
        "gguf_version": ver,
        "tokenizer.chat_template": kv.get("tokenizer.chat_template"),
        "named_templates": named,
        "bos_token": kv.get("tokenizer.ggml.bos_token_id"),
        "eos_token": kv.get("tokenizer.ggml.eos_token_id"),
    }
    os.makedirs(OUT, exist_ok=True)
    with open(os.path.join(OUT, mid + ".templates.json"), "w") as f:
        f.write(json.dumps(out, indent=1) + "\n")
    print(f"{mid}: {len(out['tokenizer.chat_template'] or '')} bytes, "
          f"named templates {sorted(named)}")


if __name__ == "__main__":
    for model_id in sys.argv[1:] or sorted(MODELS):
        fetch(model_id)
