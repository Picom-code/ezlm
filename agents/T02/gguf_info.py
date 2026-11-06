#!/usr/bin/env python3
"""Print the GGUF header fields an EZLM catalog entry needs.

The header is read with HTTP range requests, so a 40 GB model costs about
16 MB of download and one to two seconds. Nothing is written to disk and no
weights are fetched. Plain python3, standard library only.

    python3 gguf_info.py <repo> <file> [--json] [--parts]

Add --parts for a file named -NNNNN-of-NNNNN.gguf to read every part and
merge the tensor types and parameter counts.

Examples:

    python3 gguf_info.py ggml-org/gpt-oss-20b-GGUF gpt-oss-20b-MXFP4.gguf
    python3 gguf_info.py unsloth/gemma-4-12b-it-GGUF \\
        gemma-4-12b-it-UD-IQ2_M.gguf --json

The "catalog:" block is what internal/catalog/catalog.go stores: layers,
kv_heads, head_dim, kv_layers, ctx_max and the sliding-window fields. The
rules match scripts/verify_catalog.py, so an entry built from this output
passes that checker. Raw metadata and the tensor type histogram are printed
underneath for anything the block does not cover.

Exit status is 0 on success, 1 on a bad file or a failed request.
"""
import argparse
import json
import re
import struct
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

HF = "https://huggingface.co"
UA = {"User-Agent": "ezlm-gguf-info", "Accept-Encoding": "identity"}
SCALARS = {0: ("<B", 1), 1: ("<b", 1), 2: ("<H", 2), 3: ("<h", 2),
           4: ("<I", 4), 5: ("<i", 4), 6: ("<f", 4), 7: ("<?", 1),
           10: ("<Q", 8), 11: ("<q", 8), 12: ("<d", 8)}

# ggml_type ids from llama.cpp b11960 (ggml/include/ggml.h). Ids 4, 5, 31,
# 32, 33, 36, 37 and 38 were removed but can still appear in old files.
TYPES = {
    0: "F32", 1: "F16", 2: "Q4_0", 3: "Q4_1", 4: "Q4_2", 5: "Q4_3",
    6: "Q5_0", 7: "Q5_1", 8: "Q8_0", 9: "Q8_1", 10: "Q2_K", 11: "Q3_K",
    12: "Q4_K", 13: "Q5_K", 14: "Q6_K", 15: "Q8_K", 16: "IQ2_XXS",
    17: "IQ2_XS", 18: "IQ3_XXS", 19: "IQ1_S", 20: "IQ4_NL", 21: "IQ3_S",
    22: "IQ2_S", 23: "IQ4_XS", 24: "I8", 25: "I16", 26: "I32", 27: "I64",
    28: "F64", 29: "IQ1_M", 30: "BF16", 31: "Q4_0_4_4", 32: "Q4_0_4_8",
    33: "Q4_0_8_8", 34: "TQ1_0", 35: "TQ2_0", 36: "IQ4_NL_4_4",
    37: "IQ4_NL_4_8", 38: "IQ4_NL_8_8", 39: "MXFP4", 40: "NVFP4",
    41: "Q1_0", 42: "Q2_0",
}

# Arrays larger than this are skipped instead of decoded: the tokenizer
# vocabulary and merges lists are the bulk of a header and we never need them.
MAX_ARRAY = 4096


def retry(f):
    def go(*a):
        for i in range(4):
            try:
                return f(*a)
            except urllib.error.HTTPError:
                raise  # a real answer (404, 401, 416): don't retry
            except (RuntimeError, ValueError, EOFError):
                raise  # a bad header, not a flaky connection
            except Exception:
                if i == 3:
                    raise
                time.sleep(2 * (i + 1))
    return go


class Remote:
    """A remote file read through HTTP range requests, buffered in chunks."""

    def __init__(self, url, chunk=16 << 20, timeout=60):
        self.url = url
        self.final = None       # CDN URL after the Hugging Face redirect
        self.chunk = chunk
        self.timeout = timeout
        self.buf = b""
        self.base = 0           # file offset of buf[0]
        self.pos = 0
        self.requests = 0
        self.fetched = 0
        self.total = None       # file size from Content-Range

    @retry
    def _fetch(self, start, end):
        req = urllib.request.Request(self.final or self.url,
                                     headers={**UA, "Range": f"bytes={start}-{end}"})
        with urllib.request.urlopen(req, timeout=self.timeout) as r:
            if self.final is None:
                self.final = r.geturl()
            if r.status == 200:
                # The server ignored Range. Read only what was asked for so a
                # 20 GB file cannot be pulled into memory by accident.
                if start > 0:
                    raise RuntimeError("server ignored the Range header")
                data = r.read(end - start + 1)
                self.total = int(r.headers.get("Content-Length") or len(data))
            else:
                data = r.read()
                cr = r.headers.get("Content-Range") or ""
                if "/" in cr:
                    try:
                        self.total = int(cr.rsplit("/", 1)[1])
                    except ValueError:
                        pass
        self.requests += 1
        self.fetched += len(data)
        return data

    def take(self, n):
        if n > 256 << 20:
            raise ValueError(f"absurd length {n} in the header")
        while self.pos + n > self.base + len(self.buf):
            start = self.base + len(self.buf)
            more = self._fetch(start, start + max(self.chunk, n) - 1)
            if not more:
                raise EOFError("file ended inside the header")
            self.buf += more
            self.chunk = min(self.chunk * 2, 64 << 20)
        off = self.pos - self.base
        self.pos += n
        return self.buf[off:off + n]


class Reader:
    def __init__(self, remote):
        self.r = remote
        self.v1 = False

    def take(self, n):
        return self.r.take(n)

    def u32(self):
        return struct.unpack("<I", self.take(4))[0]

    def u64(self):
        return struct.unpack("<Q", self.take(8))[0]

    def count(self):
        return self.u32() if self.v1 else self.u64()

    def string(self):
        return self.take(self.count()).decode("utf-8", "replace")


def skip_value(r, t):
    """Advance over a value whose contents we do not need."""
    if t in SCALARS:
        r.take(SCALARS[t][1])
    elif t == 8:
        r.take(r.count())
    elif t == 9:
        et, n = r.u32(), r.count()
        for _ in range(n):
            skip_value(r, et)
    else:
        raise ValueError(f"unknown GGUF value type {t}")


def value(r, t):
    if t in SCALARS:
        fmt = SCALARS[t][0]
        return struct.unpack(fmt, r.take(struct.calcsize(fmt)))[0]
    if t == 8:
        return r.string()
    if t == 9:
        et, n = r.u32(), r.count()
        if n > MAX_ARRAY:
            for _ in range(n):
                skip_value(r, et)
            return f"<{n} values>"
        return [value(r, et) for _ in range(n)]
    raise ValueError(f"unknown GGUF value type {t}")


def header(url, timeout=60):
    """Parse the metadata and tensor table. Returns (version, kv, tensors, reader)."""
    remote = Remote(url, timeout=timeout)
    r = Reader(remote)
    if r.take(4) != b"GGUF":
        raise ValueError("not a GGUF file (bad magic)")
    ver = r.u32()
    if ver not in (1, 2, 3):
        raise ValueError(f"unsupported GGUF version {ver}")
    r.v1 = ver == 1
    n_tensors, n_kv = r.count(), r.count()
    kv = {}
    for _ in range(n_kv):
        key = r.string()
        kv[key] = value(r, r.u32())
    tensors = []
    for _ in range(n_tensors):
        name = r.string()
        dims = [r.u64() for _ in range(r.u32())]
        tensors.append((name, dims, r.u32()))
        r.u64()  # data offset, not needed
    return ver, kv, tensors, remote


def _max(vals):
    return max(vals) if vals else 0


def derive(kv, arch, tensors):
    """Work out the catalog fields from raw metadata.

    Returns (catalog, params, hints, notes). catalog holds the values that go
    into internal/catalog/catalog.go; notes explains any guess made here.
    """
    p = arch + "."
    notes = []

    def g(name, default=None):
        return kv.get(p + name, default)

    blocks = g("block_count") or 0
    nextn = g("nextn_predict_layers") or 0
    # Some converters count the MTP/NextN layers in block_count. The catalog
    # stores the number of ordinary transformer layers, and so does
    # scripts/verify_catalog.py, so drop them here.
    layers = blocks - nextn if blocks else 0
    n = layers or blocks

    head_count = g("attention.head_count")
    if isinstance(head_count, list):
        head_count = _max(head_count)
    hkv = g("attention.head_count_kv")
    klen = g("attention.key_length")
    klen_swa = g("attention.key_length_swa")
    sliding = g("attention.sliding_window")
    pattern = g("attention.sliding_window_pattern")
    shared = g("attention.shared_kv_layers") or 0
    interval = g("full_attention_interval")
    lora = g("attention.kv_lora_rank")
    rope = g("rope.dimension_count")

    if isinstance(hkv, list):
        heads = list(hkv)
        if len(heads) == blocks and nextn:
            heads = heads[:n]
        elif len(heads) == n:
            pass
        elif len(heads) != blocks:
            notes.append(f"head_count_kv has {len(heads)} entries for {blocks} blocks; "
                         "padded with its last value")
            heads = (heads + [heads[-1] if heads else 0] * n)[:n]
        else:
            heads = heads[:n]
    elif isinstance(hkv, int):
        heads = [hkv] * n
    else:
        heads = [0] * n
        notes.append("no attention.head_count_kv; no KV cache fields derived")

    if isinstance(pattern, list):
        if len(pattern) == blocks and nextn:
            pattern = pattern[:n]
        elif len(pattern) != n:
            pattern = None

    # Which layers are sliding-window (True) and which are full attention.
    swa = [False] * len(heads)
    if isinstance(pattern, list) and len(pattern) == len(heads):
        swa = [bool(x) for x in pattern]
    elif isinstance(pattern, int) and pattern > 1:
        # Every n-th layer is full attention; the rest keep a short window.
        swa = [(i % pattern) < pattern - 1 for i in range(len(heads))]
    elif interval:
        # Hybrid (qwen3.5/3.8, gated deltanet): only every interval-th layer
        # has attention at all, the others hold no KV cache.
        heads = [h if (i + 1) % interval == 0 else 0 for i, h in enumerate(heads)]
    elif isinstance(hkv, list) and klen_swa and len({h for h in heads if h}) > 1:
        # Gemma without sliding_window_pattern: the full-attention layers use
        # the larger head size and fewer KV heads than the local ones.
        low = min(h for h in heads if h)
        swa = [h != low for h in heads]
        notes.append("gemma-style head_count_kv list without sliding_window_pattern: "
                     "treated the lower-head layers as full attention")
    elif arch == "gpt-oss" and sliding:
        swa = [(i % 2) == 0 for i in range(len(heads))]
        notes.append("gpt-oss: alternating layers treated as sliding-window")

    global_layers = [i for i, h in enumerate(heads) if h and not swa[i]]
    swa_layers = [i for i, h in enumerate(heads) if h and swa[i]]
    kv_heads = _max([heads[i] for i in global_layers])
    swa_heads = _max([heads[i] for i in swa_layers])

    if not klen and head_count:
        klen = (g("embedding_length") or 0) // head_count
        if klen:
            notes.append(f"no attention.key_length; used embedding_length/head_count = {klen}")
    head_dim = klen or 0
    swa_dim = klen_swa or klen or 0

    if lora:  # MLA: the cache is the compressed latent plus the rope part
        kv_heads = 1
        head_dim = lora + (rope or 0)
        notes.append(f"MLA: head_dim = kv_lora_rank {lora} + rope {rope or 0}")

    ctx = g("context_length") or 0
    if isinstance(ctx, list):
        ctx = _max(ctx)
    experts = g("expert_count") or 0
    if isinstance(experts, list):
        experts = _max(experts)
    used = g("expert_used_count") or 0
    if isinstance(used, list):
        # Some Nemotron files store this per layer.
        used = _max(used)
        notes.append(f"expert_used_count is per layer; using the maximum {used}")

    catalog = {
        "layers": layers,
        "kv_heads": kv_heads,
        "head_dim": head_dim,
        "kv_layers": len(global_layers),
        "ctx_max": ctx,
        "moe": experts > 0,
    }
    if swa_layers:
        catalog.update({
            "swa_layers": len(swa_layers),
            "swa_kv_heads": swa_heads,
            "swa_head_dim": swa_dim,
            "swa_window": sliding or 0,
        })
    if shared:
        catalog["shared_kv_layers"] = shared
        owned = sum(1 for i in global_layers if i < n - shared)
        catalog["kv_layers_owned"] = owned
        notes.append(f"shared_kv_layers {shared}: only {owned} of {len(global_layers)} "
                     "full-attention layers allocate their own cache")
    if nextn:
        notes.append(f"block_count {blocks} includes {nextn} nextn/MTP layers; "
                     f"layers is {layers}")
    if experts:
        catalog["experts"] = experts
        catalog["experts_active"] = used

    total = sum(_prod(d) for _, d, _ in tensors)
    expert = sum(_prod(d) for n, d, _ in tensors if "exps" in n)
    embd = sum(_prod(d) for n, d, _ in tensors if "embd" in n or "embed" in n)
    params = {
        "total_b": round(total / 1e9, 2),
        "expert_b": round(expert / 1e9, 2),
        "dense_b": round((total - expert) / 1e9, 2),
    }
    if experts and used:
        # A token touches `used` of `experts` expert matrices and one row of
        # each embedding table, so leave the embedding tables out. The card
        # number still differs a little; this is a sanity check, not a source.
        active = (total - expert - embd) + expert * used / experts
        params["active_b_est"] = round(active / 1e9, 2)

    names = [n for n, _, _ in tensors]
    hints = {
        "chat_template": bool(kv.get("tokenizer.chat_template")),
        "chat_template_tools": "tools" in str(kv.get("tokenizer.chat_template", "")),
        "vision_tensors": any(n.startswith("v.") or "mmproj" in n or "vision" in n
                              for n in names),
    }
    return catalog, params, hints, notes


def _prod(vals):
    out = 1
    for v in vals:
        out *= v
    return out


def build_url(repo, file, revision):
    if repo.startswith(("http://", "https://")):
        return repo
    quote = lambda s: urllib.parse.quote(s, safe="")
    repo_q = "/".join(quote(p) for p in repo.strip("/").split("/"))
    file_q = "/".join(quote(p) for p in file.strip("/").split("/"))
    return f"{HF}/{repo_q}/resolve/{quote(revision)}/{file_q}"


def human(info, show_metadata):
    out = []
    add = out.append
    add(f"repo:       {info['repo']}")
    add(f"file:       {info['file']}")
    add(f"revision:   {info['revision']}")
    add(f"url:        {info['url']}")
    if info.get("bytes") is not None:
        add(f"file size:  {info['bytes']} bytes")
    add(f"gguf:       v{info['gguf_version']}, {info['tensor_count']} tensors, "
        f"{info['metadata_count']} metadata keys")
    add(f"arch:       {info['arch']}")
    add(f"name:       {info['name']}")
    split = info.get("split") or {}
    if split:
        add(f"split:      {split.get('no', 1)} of {split.get('count', 1)}"
            + (f" ({split['tensors_count']} tensors across parts)"
               if split.get("tensors_count") else ""))
    else:
        add("split:      1 of 1")
    if info.get("parts"):
        add("parts:")
        for part in info["parts"]:
            add(f"  {part['file']:<58} {part['bytes'] or 0:>13} bytes  "
                f"{part['tensors']} tensors")

    add("catalog:")
    cat = info["catalog"]
    for key in ("layers", "kv_heads", "head_dim", "kv_layers", "ctx_max",
                "swa_layers", "swa_kv_heads", "swa_head_dim", "swa_window",
                "shared_kv_layers", "kv_layers_owned", "experts", "experts_active"):
        if cat.get(key) is not None:
            add(f"  {key:<15} {cat[key]}")
    add(f"  {'moe':<15} {'true' if cat['moe'] else 'false'}")

    add("params:")
    for key in ("total_b", "expert_b", "dense_b", "active_b_est"):
        if key in info["params"]:
            add(f"  {key:<15} {info['params'][key]}")

    add("tensor types:")
    for name, count in sorted(info["tensor_types"].items(), key=lambda x: (-x[1], x[0])):
        add(f"  {name:<15} {count}")

    hints = info["hints"]
    add("hints:")
    add(f"  chat_template  {'yes' if hints['chat_template'] else 'no'}"
        + (" (mentions tools)" if hints["chat_template_tools"] else ""))
    add(f"  vision_tensors {'yes' if hints['vision_tensors'] else 'no'}")

    if show_metadata:
        add("metadata:")
        for key in sorted(info["metadata"]):
            val = info["metadata"][key]
            text = repr(val)
            if len(text) > 120:
                text = text[:117] + "..."
            add(f"  {key} = {text}")

    for note in info["notes"]:
        add(f"note:       {note}")
    rd = info["read"]
    add(f"read:       {rd['seconds']:.2f} s, {rd['requests']} range request(s), "
        f"{rd['fetched_bytes'] / 1e6:.1f} MB fetched")
    return "\n".join(out)


def part_names(file, count):
    """Return the names of all parts of a split file, or None."""
    m = re.search(r"-(\d{5})-of-(\d{5})\.gguf$", file)
    if not m or int(m.group(2)) != count:
        return None
    return [re.sub(r"-\d{5}-of-\d{5}\.gguf$", f"-{i:05d}-of-{count:05d}.gguf", file)
            for i in range(1, count + 1)]


def main():
    ap = argparse.ArgumentParser(
        description="Print the GGUF metadata a catalog entry needs, using range requests.")
    ap.add_argument("repo", help="Hugging Face repo (org/name) or a full file URL")
    ap.add_argument("file", nargs="?", default="", help="file inside the repo")
    ap.add_argument("--revision", default="main", help="branch, tag or commit (default: main)")
    ap.add_argument("--json", action="store_true", help="print one JSON object")
    ap.add_argument("--no-metadata", action="store_true", help="hide the raw metadata block")
    ap.add_argument("--parts", action="store_true",
                    help="for a split model, read every part and merge tensor types and "
                         "parameter counts")
    ap.add_argument("--timeout", type=float, default=60, help="per-request timeout in seconds")
    args = ap.parse_args()

    if not args.repo.startswith(("http://", "https://")) and not args.file:
        ap.error("give a file inside the repo, or a full URL")

    url = build_url(args.repo, args.file, args.revision)
    t0 = time.time()
    try:
        ver, kv, tensors, remote = header(url, args.timeout)
    except Exception as e:
        print(f"gguf_info: {url}: {e}", file=sys.stderr)
        return 1

    reads = [(args.file, ver, kv, tensors, remote)]
    extra_notes = []
    split_count = kv.get("split.count") or 1
    if args.parts and split_count > 1 and not args.repo.startswith(("http://", "https://")):
        names = part_names(args.file, split_count)
        if names:
            for name in names:
                if name == args.file:
                    continue
                u = build_url(args.repo, name, args.revision)
                try:
                    v, k, t, rm = header(u, args.timeout)
                except Exception as e:
                    extra_notes.append(f"part {name} could not be read: {e}")
                    continue
                reads.append((name, v, k, t, rm))
        else:
            extra_notes.append("file name does not end in -NNNNN-of-NNNNN.gguf; "
                               "could not find the other parts")
    elif split_count > 1:
        extra_notes.append(f"split file; add --parts to merge tensor types and parameter "
                           f"counts from all {split_count} parts")

    # The first part carries the full metadata; a later part may carry most of
    # the tensors, so take metadata from whichever part has the most keys.
    _, _, meta, _, primary = max(reads, key=lambda x: len(x[2]))
    all_tensors = [t for _, _, _, ts, _ in reads for t in ts]
    elapsed = time.time() - t0

    arch = meta.get("general.architecture") or "unknown"
    catalog, params, hints, notes = derive(meta, arch, all_tensors)
    notes.extend(extra_notes)
    types = {}
    for _, _, t in all_tensors:
        name = TYPES.get(t, f"TYPE_{t}")
        types[name] = types.get(name, 0) + 1
        if t not in TYPES:
            notes.append(f"unknown tensor type id {t}")

    split = {}
    if "split.no" in meta or "split.count" in meta:
        split = {
            "no": (meta.get("split.no") or 0) + 1,
            "count": meta.get("split.count") or 1,
        }
        if "split.tensors.count" in meta:
            split["tensors_count"] = meta["split.tensors.count"]

    info = {
        "schema": 1,
        "repo": args.repo if not args.repo.startswith(("http://", "https://")) else "",
        "file": args.file,
        "revision": args.revision,
        "url": url,
        "bytes": primary.total,
        "gguf_version": ver,
        "tensor_count": len(all_tensors),
        "metadata_count": len(meta),
        "arch": arch,
        "name": meta.get("general.name") or "",
        "split": split,
        "catalog": catalog,
        "params": params,
        "tensor_types": types,
        "hints": hints,
        "metadata": meta,
        "notes": notes,
        "read": {
            "seconds": round(elapsed, 3),
            "requests": sum(r.requests for _, _, _, _, r in reads),
            "fetched_bytes": sum(r.fetched for _, _, _, _, r in reads),
        },
    }
    if len(reads) > 1:
        info["parts"] = [{"file": name, "bytes": rm.total, "tensors": len(ts)}
                         for name, _, _, ts, rm in reads]

    if args.json:
        print(json.dumps(info, indent=2, ensure_ascii=False))
    else:
        print(human(info, not args.no_metadata))
    return 0


if __name__ == "__main__":
    sys.exit(main())
