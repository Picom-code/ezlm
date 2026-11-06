#!/usr/bin/env python3
"""Dumps the GGUF header key/values of the first file of one quant of a model,
so the catalog numbers can be checked by hand against the file.

  python3 agents/T01/a12/dump_gguf.py <repo> <file> [key-prefix...]
"""
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[3] / "scripts"))
from verify_catalog import header  # noqa: E402


def main():
    repo, fname = sys.argv[1], sys.argv[2]
    prefixes = sys.argv[3:]
    url = f"https://huggingface.co/{repo}/resolve/main/{fname}"
    ver, kv, types = header(url)
    print(f"# {url}\n# gguf version {ver}, {len(types)} tensor types: {sorted(types)}")
    for k in sorted(kv):
        if prefixes and not any(k.startswith(p) for p in prefixes):
            continue
        v = kv[k]
        if isinstance(v, str) and len(v) > 200:
            print(f"{k} = <{len(v)} chars>")
        else:
            print(f"{k} = {v!r}")


if __name__ == "__main__":
    main()
