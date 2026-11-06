#!/usr/bin/env python3
"""Dumps the metadata of one remote GGUF file (range requests only).

  python3 dump_header.py <repo> <file> [--template]

Reuses scripts/verify_catalog.py's reader so the numbers come from the same code
path the audit uses.
"""
import os, sys

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "..", "..", "scripts"))
from verify_catalog import header  # noqa: E402

SKIP = {
    "tokenizer.ggml.tokens", "tokenizer.ggml.merges", "tokenizer.ggml.token_type",
    "tokenizer.ggml.scores", "tokenizer.chat_template",
}


def main():
    repo, name = sys.argv[1], sys.argv[2]
    show_template = "--template" in sys.argv
    url = f"https://huggingface.co/{repo}/resolve/main/{name}"
    ver, kv, types = header(url)
    print(f"# {repo}/{name}  gguf v{ver}  tensors={len(types)}")
    for k in sorted(kv):
        if k in SKIP:
            continue
        v = kv[k]
        if isinstance(v, list) and len(v) > 16:
            v = f"[{len(v)} items] {v[:8]}..."
        print(f"{k} = {v!r}")
    print(f"# tensor types: {sorted(types)}")
    t = kv.get("tokenizer.chat_template")
    if show_template and t:
        print("# --- tokenizer.chat_template ---")
        print(t)


if __name__ == "__main__":
    main()
