#!/usr/bin/env python3
"""Fetch the GGUF header chat templates for a08's models (range requests only)
and write agents/T03/models/<id>.templates.json."""
import json
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.abspath(os.path.join(HERE, "..", "..", ".."))
sys.path.insert(0, os.path.join(ROOT, "scripts"))
import verify_catalog as v  # noqa: E402

MODELS = {
    "qwen3.5-4b": ("Qwen/Qwen3.5-4B", "unsloth/Qwen3.5-4B-GGUF",
                    "Qwen3.5-4B-UD-IQ2_XXS.gguf", "UD-IQ2_XXS"),
    "qwen3.5-2b": ("Qwen/Qwen3.5-2B", "unsloth/Qwen3.5-2B-GGUF",
                    "Qwen3.5-2B-UD-IQ2_XXS.gguf", "UD-IQ2_XXS"),
}

OUT = os.path.join(ROOT, "agents", "T03", "models")


def main():
    for mid, (repo, qrepo, fname, quant) in MODELS.items():
        url = "https://huggingface.co/%s/resolve/main/%s" % (qrepo, fname)
        ver, kv, _types = v.header(url)
        tpl = {k: val for k, val in kv.items()
               if k == "tokenizer.chat_template" or k.startswith("tokenizer.chat_template.")}
        doc = {
            "id": mid,
            "repo": repo,
            "quant_repo": qrepo,
            "quant": quant,
            "file": fname,
            "url": url,
            "gguf_version": ver,
            "arch": kv.get("general.architecture"),
            "templates": tpl,
        }
        path = os.path.join(OUT, mid + ".templates.json")
        with open(path, "w") as f:
            json.dump(doc, f, indent=1, ensure_ascii=False)
            f.write("\n")
        print(mid, "->", path, len(tpl), "template(s)", [len(t) for t in tpl.values()])


if __name__ == "__main__":
    main()
