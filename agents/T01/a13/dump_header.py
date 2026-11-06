#!/usr/bin/env python3
"""Print the GGUF metadata of one remote file (range requests only), with big
arrays summarised so the output stays readable.

  agents/T01/a13/dump_header.py <repo> <file> [--template]

Used by agent a13 to check the catalog fields that verify_catalog.py does not
compare (params, moe, vision, tool calling, kv_layers, ctx_max).
"""
import argparse
import os
import sys

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "..", "scripts"))
import verify_catalog as vc  # noqa: E402


def show(v, limit=12):
    if isinstance(v, list):
        if len(v) > limit:
            return f"[{len(v)} values: {v[:limit]} ...]"
        return v
    if isinstance(v, str) and len(v) > 400:
        return v[:400] + f" ... <{len(v)} chars>"
    return v


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("repo")
    ap.add_argument("file")
    ap.add_argument("--template", action="store_true", help="print tokenizer.chat_template in full")
    a = ap.parse_args()
    url = f"https://huggingface.co/{a.repo}/resolve/main/{a.file}"
    ver, kv, types = vc.header(url)
    print(f"# {a.repo}/{a.file}  gguf v{ver}")
    for k in sorted(kv):
        v = kv[k]
        if k == "tokenizer.chat_template":
            v = v if a.template else f"<{len(v)} chars>"
        print(f"{k} = {show(v)!r}")


if __name__ == "__main__":
    main()
