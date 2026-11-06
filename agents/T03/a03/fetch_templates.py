#!/usr/bin/env python3
"""Fetch the GGUF header of one catalog model with HTTP range requests and save
every chat template it carries.

  fetch_templates.py <model-id> [--cache DIR]

Writes agents/T03/models/<id>.templates.json:
  {"id", "repo", "file", "url", "header_keys", "templates": {name: text}}

The whole GGUF is never downloaded: scripts/verify_catalog.Reader reads only
the metadata block at the front of the file.
"""
import argparse
import json
import os
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.abspath(os.path.join(HERE, "..", "..", ".."))


def catalog_entry(model_id):
    cat = json.load(open(os.path.join(ROOT, "internal/catalog/catalog.json")))
    for m in cat["models"]:
        if m["id"] == model_id:
            return m
    raise SystemExit(f"{model_id}: not in catalog")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("model_id")
    ap.add_argument("--out", default=os.path.join(ROOT, "agents/T03/models"))
    args = ap.parse_args()

    m = catalog_entry(args.model_id)
    q = m["quants"][0]
    repo = q.get("repo") or m["repo"]
    fname = q["files"][0]
    url = f"https://huggingface.co/{repo}/resolve/main/{fname}"

    sys.path.insert(0, os.path.join(ROOT, "scripts"))
    import verify_catalog as v

    _ver, kv, _types = v.header(url)

    tpl = {}
    for k, val in kv.items():
        if k == "tokenizer.chat_template" or k.startswith("tokenizer.chat_template."):
            name = "tokenizer.chat_template" if k == "tokenizer.chat_template" else k[len("tokenizer.chat_template."):]
            tpl[name] = val

    out = {
        "id": args.model_id,
        "repo": repo,
        "file": fname,
        "url": url,
        "header_keys": sorted(k for k in kv if k.startswith("tokenizer.chat_template")),
        "templates": tpl,
    }
    os.makedirs(args.out, exist_ok=True)
    path = os.path.join(args.out, f"{args.model_id}.templates.json")
    with open(path, "w") as f:
        json.dump(out, f, indent=2, sort_keys=True)
        f.write("\n")
    print(f"{args.model_id}: {len(tpl)} template(s) -> {path}")
    for name, text in tpl.items():
        print(f"  {name}: {len(text)} bytes")


if __name__ == "__main__":
    main()
