#!/usr/bin/env python3
"""Checks every model file in the catalog against a llama.cpp release, without
downloading the models: it reads only the GGUF headers (HTTP range requests).

For each quant it verifies that every file exists with the listed size, the
architecture and every tensor type are known to that llama.cpp build, split
files are complete, and the catalog's layer / KV-head / head-size / context
numbers (which size the memory plan) match the file's own metadata.

  scripts/verify_catalog.py [--tag b11960] [--catalog internal/catalog/catalog.json] [--only id,id] [--json]

Exit codes: 0 every file set is fine (warnings allowed), 1 at least one
mismatch, 2 the run could not be trusted (bad arguments, unreadable catalog,
or a network error while fetching). With --json one JSON object is printed
instead of the human report.
"""
import argparse, concurrent.futures as cf, json, re, struct, sys, time, urllib.error, urllib.request

UA = {"User-Agent": "ezlm-verify"}


def retry(f):
    def go(*a):
        for i in range(4):
            try:
                return f(*a)
            except urllib.error.HTTPError:
                raise  # a real answer (404, 401): don't retry
            except Exception:
                if i == 3:
                    raise
                time.sleep(3 * (i + 1))
    return go


@retry
def fetch(url, start, end):
    req = urllib.request.Request(url, headers={**UA, "Range": f"bytes={start}-{end}"})
    with urllib.request.urlopen(req, timeout=60) as r:
        return r.read()


@retry
def size_of(url):
    req = urllib.request.Request(url, method="HEAD", headers=UA)
    with urllib.request.urlopen(req, timeout=60) as r:
        return int(r.headers.get("Content-Length") or 0)


class Reader:
    """Sequential reader over a remote file, fetching chunks on demand."""

    def __init__(self, url, chunk=4 << 20):
        self.url, self.chunk, self.buf, self.base, self.pos = url, chunk, b"", 0, 0

    def take(self, n):
        while self.pos + n > self.base + len(self.buf):
            start = self.base + len(self.buf)
            more = fetch(self.url, start, start + max(self.chunk, n) - 1)
            if not more:
                raise EOFError("file ended inside the header")
            self.buf += more
            self.chunk = min(self.chunk * 2, 64 << 20)
        off = self.pos - self.base
        self.pos += n
        return self.buf[off:off + n]

    def u32(self): return struct.unpack("<I", self.take(4))[0]
    def u64(self): return struct.unpack("<Q", self.take(8))[0]
    def s(self): return self.take(self.u64()).decode("utf-8", "replace")


SCALARS = {0: "<B", 1: "<b", 2: "<H", 3: "<h", 4: "<I", 5: "<i", 6: "<f", 7: "<?", 10: "<Q", 11: "<q", 12: "<d"}


def value(r, t):
    if t in SCALARS:
        fmt = SCALARS[t]
        return struct.unpack(fmt, r.take(struct.calcsize(fmt)))[0]
    if t == 8:
        return r.s()
    if t == 9:
        et, n = r.u32(), r.u64()
        if et == 8 and n > 64:  # big string arrays (vocab, merges): skip, keep the count
            for _ in range(n):
                r.take(r.u64())
            return f"<{n} strings>"
        return [value(r, et) for _ in range(n)]
    raise ValueError(f"unknown GGUF value type {t}")


def header(url):
    r = Reader(url)
    if r.take(4) != b"GGUF":
        raise ValueError("not a GGUF file")
    ver = r.u32()
    nt, nkv = r.u64(), r.u64()
    kv = {}
    for _ in range(nkv):
        k = r.s()
        kv[k] = value(r, r.u32())
    types = set()
    for _ in range(nt):
        r.s()
        nd = r.u32()
        for _ in range(nd):
            r.u64()
        types.add(r.u32())
        r.u64()
    return ver, kv, types


class Finding:
    """One line of check output.

    level is "ok", "warn", "error" (a definite answer about the catalog) or
    "neterr" (the file could not be read, so the run is inconclusive).
    """

    __slots__ = ("level", "text")

    def __init__(self, level, text):
        self.level, self.text = level, text

    def __str__(self):
        return self.text


def failure_level(e):
    """A 404 or an unreadable header is a definite answer about the file;
    anything else (timeouts, resets) is a network problem."""
    if isinstance(e, (urllib.error.HTTPError, ValueError, EOFError, struct.error)):
        return "error"
    return "neterr"


def check_quant(m, q, archs, types):
    repo = q.get("repo") or m["repo"]
    out = []
    urls = [f"https://huggingface.co/{repo}/resolve/main/{f}" for f in q["files"]]
    try:
        total = sum(size_of(u) for u in urls)
    except Exception as e:
        return [Finding(failure_level(e), f"ERROR missing file: {e}")]
    if total != q["bytes"]:
        out.append(Finding("error", f"ERROR size {total} != catalog {q['bytes']}"))
    # llama.cpp finds the parts of a split model by name (<prefix>-0000i-of-0000N.gguf),
    # and EZLM saves each file under its own name: every part must follow the pattern.
    if len(urls) > 1:
        first = q["files"][0].rsplit("/", 1)[-1]
        mm = re.search(r"-00001-of-(\d{5})\.gguf$", first)
        for i, f in enumerate(q["files"]):
            want = first.replace("-00001-of-", f"-{i + 1:05d}-of-") if mm else None
            if not mm or f.rsplit("/", 1)[-1] != want or int(mm.group(1)) != len(urls):
                out.append(Finding("error", f"ERROR part {i + 1} is named {f.rsplit('/', 1)[-1]}, llama.cpp looks for {want}"))
                break
    kv0 = None
    for i, u in enumerate(urls):
        try:
            _, kv, ts = header(u)
        except Exception as e:
            out.append(Finding(failure_level(e), f"ERROR header of {q['files'][i]}: {e}"))
            continue
        bad = sorted(t for t in ts if str(t) not in types or types[str(t)] == "COUNT")
        if bad:
            out.append(Finding("error", f"ERROR tensor types unknown to llama.cpp: {bad} in {q['files'][i]}"))
        if i == 0:
            kv0 = kv
    if kv0 is None:
        return out
    arch = kv0.get("general.architecture")
    if arch not in archs:
        out.append(Finding("error", f"ERROR architecture '{arch}' unknown to llama.cpp"))
    n = kv0.get("split.count")
    if n is not None and n != len(urls):
        out.append(Finding("error", f"ERROR split.count {n} but catalog lists {len(urls)} files"))
    p = f"{arch}."
    blocks = kv0.get(p + "block_count")
    hkv = kv0.get(p + "attention.head_count_kv")
    klen = kv0.get(p + "attention.key_length")
    lora = kv0.get(p + "attention.kv_lora_rank")
    rope = kv0.get(p + "rope.dimension_count")
    ctx = kv0.get(p + "context_length")
    if blocks is not None and m.get("layers") and blocks != m["layers"]:
        # some files count the MTP/NextN layer in block_count
        nextn = kv0.get(p + "nextn_predict_layers") or 0
        if blocks - nextn != m["layers"]:
            out.append(Finding("warn", f"WARN layers {m['layers']} but file has block_count {blocks} (nextn {nextn})"))
    # Only undercounts matter: a plan that assumes less cache than llama.cpp
    # allocates can run out of memory. Overcounting is merely conservative.
    nextn = kv0.get(p + "nextn_predict_layers") or 0
    shared = kv0.get(p + "attention.shared_kv_layers") or 0
    pattern = kv0.get(p + "attention.sliding_window_pattern")
    heads = hkv if isinstance(hkv, list) else [hkv or 0] * (blocks or 0)
    if isinstance(pattern, int) or (pattern is None and kv0.get(p + "attention.sliding_window") and arch == "gpt-oss"):
        n = pattern if isinstance(pattern, int) else 2  # every n-th layer is full attention (gpt-oss alternates)
        pattern = [(i % n) < n - 1 for i in range(len(heads))]
    fai = kv0.get(p + "full_attention_interval")
    if fai and not isinstance(pattern, list):  # hybrid: linear attention except every fai-th layer
        heads = [h if (i + 1) % fai == 0 else 0 for i, h in enumerate(heads)]
    if isinstance(pattern, list) and len(pattern) == len(heads):
        own = len(heads) - shared  # trailing layers that share an earlier cache allocate none
        glob = [h for i, (h, w) in enumerate(zip(heads, pattern)) if not w and h and i < own]
        swa = [h for i, (h, w) in enumerate(zip(heads, pattern)) if w and h and i < own]
        if glob and (m.get("kv_layers") or 0) < len(glob):
            out.append(Finding("warn", f"WARN kv_layers {m.get('kv_layers')} < {len(glob)} full-attention layers"))
        if glob and m.get("kv_heads", 0) < max(glob):
            out.append(Finding("warn", f"WARN kv_heads {m.get('kv_heads')} < {max(glob)} on full-attention layers"))
        if klen and m.get("head_dim", 0) < klen:
            out.append(Finding("warn", f"WARN head_dim {m.get('head_dim')} < key_length {klen}"))
        if swa:
            win = kv0.get(p + "attention.sliding_window") or 0
            sk = kv0.get(p + "attention.key_length_swa") or klen or 0
            got = (m.get("swa_layers", 0), m.get("swa_kv_heads", 0), m.get("swa_head_dim", 0), m.get("swa_window", 0))
            if got[0] < len(swa) or got[1] < max(swa) or got[2] < sk or got[3] < win:
                out.append(Finding("warn", f"WARN window cache undercounted: catalog swa={got}, file has {len(swa)} layers x {max(swa)} heads x {sk} dim, window {win}"))
    elif lora:  # MLA: the cache holds the compressed latent plus the rope part, one "head"
        want = lora + (rope or 0)
        if m.get("head_dim", 0) < want:
            out.append(Finding("warn", f"WARN MLA head_dim {m.get('head_dim')} < kv_lora_rank+rope {want}"))
    else:
        attn = [h for h in heads if h]
        if attn and (m.get("kv_layers") or m.get("layers") or 0) < len(attn) - nextn:
            out.append(Finding("warn", f"WARN kv_layers {m.get('kv_layers')} < {len(attn) - nextn} attention layers"))
        if attn and m.get("kv_heads", 0) < max(attn):
            out.append(Finding("warn", f"WARN kv_heads {m.get('kv_heads')} < head_count_kv {max(attn)}"))
        if klen and m.get("head_dim", 0) < klen:
            out.append(Finding("warn", f"WARN head_dim {m.get('head_dim')} < key_length {klen}"))
    if ctx and m.get("ctx_max") and m["ctx_max"] > ctx * 4:
        out.append(Finding("warn", f"WARN ctx_max {m['ctx_max']} but file says context_length {ctx}"))
    if m.get("tool_calling") and not kv0.get("tokenizer.chat_template"):
        out.append(Finding("warn", "WARN tool_calling but the file has no chat template"))
    return out or [Finding("ok", "ok")]


def release(tag):
    """Architecture names and tensor type ids known to one llama.cpp build."""
    base = f"https://raw.githubusercontent.com/ggml-org/llama.cpp/{tag}"
    archs = set(re.findall(r'\{\s*LLM_ARCH_[A-Z0-9_]+\s*,\s*"([^"]+)"\s*\}', urllib.request.urlopen(base + "/src/llama-arch.cpp").read().decode()))
    enum = re.search(r"enum ggml_type\s*\{(.*?)\};", urllib.request.urlopen(base + "/ggml/include/ggml.h").read().decode(), re.S).group(1)
    types = {v: k for k, v in re.findall(r"GGML_TYPE_([A-Z0-9_]+)\s*=\s*(\d+)", enum)}
    if not archs or not types:
        raise ValueError("could not parse the llama.cpp sources")
    return archs, types


def die(a, code, msg):
    """Report a usage or network problem and exit; JSON mode stays JSON."""
    if a.json:
        print(json.dumps({"tag": a.tag, "catalog": a.catalog, "error": msg, "exit_code": code}))
    else:
        print(msg, file=sys.stderr)
    sys.exit(code)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--tag", default="b11960")
    ap.add_argument("--catalog", default="internal/catalog/catalog.json")
    ap.add_argument("--only", default="")
    ap.add_argument("--jobs", type=int, default=8)
    ap.add_argument("--json", action="store_true", help="print one JSON object instead of the human report")
    a = ap.parse_args()
    if a.jobs < 1:
        ap.error("--jobs must be at least 1")
    try:
        cat = json.load(open(a.catalog))
        models = cat["models"]
        known = {m["id"] for m in models}
    except (OSError, ValueError) as e:
        die(a, 2, f"cannot read catalog {a.catalog}: {e}")
    except (KeyError, TypeError) as e:
        die(a, 2, f"catalog {a.catalog} has no usable models list: {e}")
    only = set(filter(None, a.only.split(",")))
    if only - known:
        ap.error(f"--only names unknown model(s): {', '.join(sorted(only - known))}")
    jobs = [(m, q) for m in models if not only or m["id"] in only for q in m["quants"]]
    try:
        archs, types = release(a.tag)
    except Exception as e:
        die(a, 2, f"cannot read llama.cpp {a.tag}: {e}")
    if not a.json:
        print(f"llama.cpp {a.tag}: {len(archs)} architectures, {len(types)} tensor types; checking {len(jobs)} files sets", flush=True)
    order = {id(q): i for i, (_, q) in enumerate(jobs)}
    results, bad, net = [], 0, 0
    with cf.ThreadPoolExecutor(a.jobs) as ex:
        futs = {ex.submit(check_quant, m, q, archs, types): (m, q) for m, q in jobs}
        for f in cf.as_completed(futs):
            m, q = futs[f]
            try:
                res = f.result()
            except Exception as e:
                res = [Finding(failure_level(e), f"ERROR {e}")]
            errs = [r for r in res if r.level == "error"]
            nets = [r for r in res if r.level == "neterr"]
            bad += bool(errs or nets)
            net += bool(nets)
            results.append((order[id(q)], m["id"], q["name"], res))
            if not a.json:
                for r in res:
                    print(f"{m['id']:30s} {q['name']:24s} {r}", flush=True)
    code = 2 if net else 1 if bad else 0
    if a.json:
        rows = []
        for _, mid, qname, res in sorted(results):
            if any(r.level == "neterr" for r in res):
                status = "network_error"
            elif any(r.level == "error" for r in res):
                status = "error"
            elif any(r.level == "warn" for r in res):
                status = "warn"
            else:
                status = "ok"
            rows.append({"model": mid, "quant": qname, "status": status,
                         "findings": [{"level": r.level, "message": r.text} for r in res]})
        counts = {s: sum(1 for r in rows if r["status"] == s) for s in ("ok", "warn", "error", "network_error")}
        print(json.dumps({
            "tag": a.tag,
            "catalog": a.catalog,
            "architectures": len(archs),
            "tensor_types": len(types),
            "checked": len(jobs),
            "ok": code == 0,
            "exit_code": code,
            "counts": counts,
            "results": rows,
        }, indent=2))
    else:
        print(f"\n{bad} file set(s) with errors")
        if net:
            print(f"{net} file set(s) could not be read (network); rerun to confirm", file=sys.stderr)
    sys.exit(code)


if __name__ == "__main__":
    main()
