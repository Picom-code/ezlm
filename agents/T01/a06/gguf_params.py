#!/usr/bin/env python3
"""Counts the parameters of a remote GGUF file from its tensor table.

  python3 gguf_params.py <repo> <file>

Reads only the header (range requests). Prints the element count per tensor
type so a catalog params_b can be checked against the file itself.
"""
import os, struct, sys

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "..", "..", "scripts"))
from verify_catalog import Reader, value  # noqa: E402

TYPES = {0: "f32", 1: "f16", 2: "q4_0", 3: "q4_1", 6: "q5_0", 7: "q5_1", 8: "q8_0",
         9: "q8_1", 10: "q2_k", 11: "q3_k", 12: "q4_k", 13: "q5_k", 14: "q6_k",
         15: "q8_k", 16: "iq2_xxs", 17: "iq2_xs", 18: "iq3_xxs", 19: "iq1_s",
         20: "iq4_nl", 21: "iq3_s", 22: "iq2_s", 23: "iq4_xs", 24: "i8", 25: "i16",
         26: "i32", 27: "i64", 28: "f64", 29: "iq1_m", 30: "bf16", 34: "tq1_0",
         35: "tq2_0", 39: "mxfp4", 40: "q1_0"}


def main():
    url = f"https://huggingface.co/{sys.argv[1]}/resolve/main/{sys.argv[2]}"
    r = Reader(url)
    if r.take(4) != b"GGUF":
        sys.exit("not a GGUF file")
    r.u32()
    nt, nkv = r.u64(), r.u64()
    kv = {}
    for _ in range(nkv):
        k = r.s()
        kv[k] = value(r, r.u32())
    total, by_type = 0, {}
    for _ in range(nt):
        r.s()
        n = 1
        for _ in range(r.u32()):
            n *= r.u64()
        t = r.u32()
        r.u64()
        total += n
        by_type[TYPES.get(t, str(t))] = by_type.get(TYPES.get(t, str(t)), 0) + n
    print(f"{sys.argv[1]}/{sys.argv[2]}")
    print(f"params total = {total:,} ({total / 1e9:.4f} B)")
    for k in sorted(by_type, key=lambda k: -by_type[k]):
        print(f"  {k:9s} {by_type[k]:>15,}  {100 * by_type[k] / total:5.2f}%")


if __name__ == "__main__":
    main()
