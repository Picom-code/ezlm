#!/usr/bin/env python3
"""Summarise the quant directories of a Hugging Face GGUF repo without
downloading weights: tree API only, one request per directory.

  quants.py <repo> [<repo> ...]
"""
import json, sys, urllib.request

UA = {"User-Agent": "ezlm-t02-a11"}


def get(url):
    req = urllib.request.Request(url, headers=UA)
    with urllib.request.urlopen(req, timeout=60) as r:
        return json.load(r)


def sizes(repo):
    top = get(f"https://huggingface.co/api/models/{repo}/tree/main")
    out = {}
    for e in top:
        if e["type"] == "directory":
            files = get(f"https://huggingface.co/api/models/{repo}/tree/main/{e['path']}")
            out[e["path"]] = [(f["path"], f.get("size") or 0) for f in files if f["type"] == "file"]
        elif e["path"].endswith(".gguf"):
            out.setdefault("<root>", []).append((e["path"], e.get("size") or 0))
    return out


for repo in sys.argv[1:]:
    print(f"== {repo}")
    try:
        d = sizes(repo)
    except Exception as e:
        print("  ERROR", e)
        continue
    for name, files in sorted(d.items(), key=lambda kv: sum(s for _, s in kv[1])):
        total = sum(s for _, s in files)
        first = files[0][0].split("/")[-1] if files else ""
        print(f"  {name:22s} {total/1e9:8.2f} GB  {len(files):2d} file(s)  {first}")
