#!/usr/bin/env python3
"""Read tokenizer.chat_template* keys from a GGUF header with HTTP range requests
and write them verbatim to agents/T03/models/<id>.templates.json.

  ~/ezlm-agents/run/venv/bin/python agents/T03/a11/fetch_templates.py
"""
import json, os, sys

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.abspath(os.path.join(HERE, "..", "..", ".."))
sys.path.insert(0, os.path.join(ROOT, "scripts"))
import verify_catalog as v

MODELS = [
    ("muse-glimmer-30b", "unsloth/Muse-Glimmer-30B-GGUF", "Muse-Glimmer-30B-UD-IQ2_XXS.gguf"),
    ("mistral-small-4-119b", "unsloth/Mistral-Small-4-119B-2603-GGUF",
     "Mistral-Small-4-119B-2603-UD-IQ1_M.gguf"),
]

outdir = os.path.join(ROOT, "agents", "T03", "models")
os.makedirs(outdir, exist_ok=True)

for mid, repo, fname in MODELS:
    url = f"https://huggingface.co/{repo}/resolve/main/{fname}"
    ver, kv, types = v.header(url)
    keys = sorted(k for k in kv if k.startswith("tokenizer.chat_template"))
    data = {"id": mid, "repo": repo, "file": fname, "gguf_version": ver,
            "source": url, "templates": {k: kv[k] for k in keys},
            "other_keys": sorted(k for k in kv if not k.startswith("tokenizer.chat_template"))}
    path = os.path.join(outdir, mid + ".templates.json")
    with open(path, "w") as f:
        json.dump(data, f, indent=1, ensure_ascii=False)
        f.write("\n")
    print(mid, "gguf v%d" % ver, "templates:", keys)
    for k in keys:
        print("   ", k, len(kv[k]), "bytes")
