#!/usr/bin/env python3
"""Fetch the chat templates for a12's two models from GGUF headers (range requests only)."""
import json, sys
sys.path.insert(0, 'scripts')
import verify_catalog as v

MODELS = {
    "devstral-small-2-24b": "https://huggingface.co/unsloth/Devstral-Small-2-24B-Instruct-2512-GGUF/resolve/main/Devstral-Small-2-24B-Instruct-2512-UD-IQ3_XXS.gguf",
    "nemotron-3.5-super-vl-120b-a12b": "https://huggingface.co/unsloth/NVIDIA-Nemotron-3.5-Super-VL-120B-A12B-GGUF/resolve/main/UD-IQ1_M/NVIDIA-Nemotron-3.5-Super-VL-120B-A12B-UD-IQ1_M-00001-of-00002.gguf",
}

for mid, url in MODELS.items():
    _, kv, _ = v.header(url)
    tpl = {k: kv[k] for k in sorted(kv) if 'chat_template' in k}
    out = {"id": mid, "url": url, "gguf_version": 3, "templates": tpl}
    path = f"agents/T03/models/{mid}.templates.json"
    with open(path, "w") as f:
        json.dump(out, f, indent=2, ensure_ascii=False)
        f.write("\n")
    print(mid, list(tpl), sum(len(x) for x in tpl.values()), "chars ->", path)
