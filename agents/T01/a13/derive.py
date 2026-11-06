#!/usr/bin/env python3
"""Derive the catalog numbers for one remote GGUF: attention layer count, KV
heads on those layers, head dim, context, experts and total parameter count
from the tensor shapes. Range requests only.

  agents/T01/a13/derive.py <repo> <file>
"""
import argparse
import os
import sys

sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "..", "scripts"))
import verify_catalog as vc  # noqa: E402


def tensors(url):
    """Yield (name, dims, ggml_type) for every tensor in the file header."""
    r = vc.Reader(url)
    if r.take(4) != b"GGUF":
        raise ValueError("not a GGUF file")
    r.u32()
    nt, nkv = r.u64(), r.u64()
    for _ in range(nkv):
        r.s()
        vc.value(r, r.u32())
    for _ in range(nt):
        name = r.s()
        nd = r.u32()
        dims = [r.u64() for _ in range(nd)]
        t = r.u32()
        r.u64()
        yield name, dims, t


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("repo")
    ap.add_argument("file")
    a = ap.parse_args()
    url = f"https://huggingface.co/{a.repo}/resolve/main/{a.file}"
    _, kv, _ = vc.header(url)
    arch = kv["general.architecture"]
    p = arch + "."
    heads = kv.get(p + "attention.head_count_kv")
    blocks = kv.get(p + "block_count")
    attn_layers = [i for i, h in enumerate(heads) if h] if isinstance(heads, list) else []
    print(f"arch {arch}  block_count {blocks}")
    print(f"head_count_kv: {len(attn_layers)} layers with cache, values "
          f"{sorted({h for h in heads if h}) if isinstance(heads, list) else heads}, indices {attn_layers}")
    print(f"key_length {kv.get(p + 'attention.key_length')} value_length {kv.get(p + 'attention.value_length')}")
    print(f"context_length {kv.get(p + 'context_length')}  rope.dimension_count {kv.get(p + 'rope.dimension_count')}")
    for k in sorted(kv):
        if any(s in k for s in ("expert_used_count", "expert_count", "shared_count", "nextn", "sliding", "ssm.")):
            v = kv[k]
            if isinstance(v, list):
                v = f"{len(v)} values, nonzero {sorted({x for x in v if x})}"
            print(f"{k} {v}")
    n = 0
    by_type = {}
    for name, dims, t in tensors(url):
        elems = 1
        for d in dims:
            elems *= d
        n += elems
        by_type[t] = by_type.get(t, 0) + elems
    print(f"total tensors elements {n:,} = {n / 1e9:.2f}B")
    print("by ggml_type:", {k: f"{v / 1e9:.2f}B" for k, v in sorted(by_type.items())})


if __name__ == "__main__":
    main()
