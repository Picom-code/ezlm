#!/usr/bin/env python3
"""Runs a catalog model through a real llama-server without downloading it.

Each GGUF file is recreated as a sparse file: its real header (metadata and
tensor table, fetched with HTTP range requests) followed by zeros up to its
exact size. llama.cpp memory-maps the weights, so it builds and executes the
model's full graph (every layer type, quant kernel and expert router) on
all-zero weights. The output is meaningless; what matters is that the
architecture loads and a forward pass runs without errors in this build.
Sparse files take almost no disk and the zero pages cost almost no RAM.

  scripts/run_zero_weights.py --server <llama-server> --model <id> [--quant <name>]
"""
import argparse, json, os, shutil, subprocess, sys, tempfile, time, urllib.error, urllib.request

sys.path.insert(0, os.path.dirname(__file__))
from verify_catalog import Reader, size_of, value  # noqa: E402


def header_bytes(url):
    """The exact bytes of a GGUF header, up to the end of the tensor table."""
    r = Reader(url)
    assert r.take(4) == b"GGUF"
    r.u32()
    nt, nkv = r.u64(), r.u64()
    for _ in range(nkv):
        r.s()
        value(r, r.u32())
    for _ in range(nt):
        r.s()
        for _ in range(r.u32()):
            r.u64()
        r.u32()
        r.u64()
    return r.buf[:r.pos]


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--server", required=True)
    ap.add_argument("--model", required=True)
    ap.add_argument("--quant", default="")  # default: the smallest file set
    ap.add_argument("--catalog", default=os.path.join(os.path.dirname(__file__), "..", "internal", "catalog", "catalog.json"))
    ap.add_argument("--port", type=int, default=18600)
    ap.add_argument("--timeout", type=int, default=900)
    ap.add_argument("--extra", default="", help="more llama-server flags, space-separated")
    a = ap.parse_args()
    cat = json.load(open(a.catalog))
    m = next(x for x in cat["models"] if x["id"] == a.model)
    q = next(x for x in m["quants"] if x["name"] == a.quant) if a.quant else min(m["quants"], key=lambda x: x["bytes"])
    repo = q.get("repo") or m["repo"]
    tmp = tempfile.mkdtemp(prefix="ezlm-zero-")
    srv = None
    try:
        t0 = time.time()
        paths = []
        for f in q["files"]:
            url = f"https://huggingface.co/{repo}/resolve/main/{f}"
            p = os.path.join(tmp, os.path.basename(f))
            with open(p, "wb") as out:
                out.write(header_bytes(url))
            os.truncate(p, size_of(url))
            paths.append(p)
        logical = sum(os.path.getsize(p) for p in paths)
        print(f"{m['id']} {q['name']}: {len(paths)} sparse file(s), {logical / 1e9:.1f} GB logical, headers in {time.time() - t0:.0f}s", flush=True)
        log = open(os.path.join(tmp, "server.log"), "w")
        srv = subprocess.Popen([a.server, "-m", paths[0], "--host", "127.0.0.1", "--port", str(a.port), "-c", "4096",
                                "-ngl", "0", "--device", "none", "-np", "1", "--alias", m["id"], "--jinja"] + a.extra.split(),
                               stdout=log, stderr=subprocess.STDOUT)
        t1 = time.time()
        while time.time() - t1 < a.timeout:
            if srv.poll() is not None:
                print(f"FAIL: llama-server exited with code {srv.returncode}:\n" + open(log.name).read()[-1500:])
                return 1
            try:
                with urllib.request.urlopen(f"http://127.0.0.1:{a.port}/health", timeout=5) as r:
                    if r.status == 200:
                        break
            except Exception:
                pass
            time.sleep(2)
        else:
            print("FAIL: never became healthy")
            return 1
        print(f"  loaded in {time.time() - t1:.0f}s", flush=True)
        msgs = [{"role": "user", "content": "Hi"}]
        tpl = json.dumps({"messages": msgs}).encode()
        with urllib.request.urlopen(urllib.request.Request(f"http://127.0.0.1:{a.port}/apply-template", tpl, {"Content-Type": "application/json"}), timeout=60) as r:
            rendered = json.load(r).get("prompt", "")
        if not rendered:
            print("FAIL: chat template rendered nothing")
            return 1
        print(f"  chat template renders ({len(rendered)} chars)", flush=True)
        body = json.dumps({"model": m["id"], "messages": [{"role": "user", "content": "Hi"}], "max_tokens": 3}).encode()
        req = urllib.request.Request(f"http://127.0.0.1:{a.port}/v1/chat/completions", body, {"Content-Type": "application/json"})
        t2 = time.time()
        try:
            with urllib.request.urlopen(req, timeout=a.timeout) as r:
                out = json.load(r)
        except urllib.error.HTTPError as e:
            msg = e.read().decode(errors="replace")
            if e.code == 500 and "does not match the expected" in msg:
                # Zero weights emit noise that can't follow a structured output format
                # (e.g. gpt-oss harmony). Generation itself is checked raw below.
                raw = json.dumps({"prompt": rendered, "n_predict": 3}).encode()
                with urllib.request.urlopen(urllib.request.Request(f"http://127.0.0.1:{a.port}/completion", raw, {"Content-Type": "application/json"}), timeout=a.timeout) as r:
                    n = json.load(r).get("tokens_predicted", 0)
                print(f"  generated {n} token(s) from the rendered template in {time.time() - t2:.0f}s (output-format check needs real weights)")
                return 0 if n > 0 else 1
            print(f"FAIL: chat request answered HTTP {e.code}: {msg[:600]}")
            # Does plain generation work without the chat template and output parser?
            raw = json.dumps({"prompt": "Hi", "n_predict": 3}).encode()
            try:
                with urllib.request.urlopen(urllib.request.Request(f"http://127.0.0.1:{a.port}/completion", raw, {"Content-Type": "application/json"}), timeout=a.timeout) as r:
                    print(f"  raw /completion works: {json.load(r).get('tokens_predicted')} token(s)")
            except Exception as e2:
                print(f"  raw /completion also failed: {e2}")
            print("  server log tail:\n" + open(log.name).read()[-1200:])
            return 1
        n = out.get("usage", {}).get("completion_tokens", 0)
        print(f"  chat template applied and {n} token(s) generated in {time.time() - t2:.0f}s (zero weights, so the text is noise)")
        return 0 if n > 0 else 1
    finally:
        if srv and srv.poll() is None:
            srv.terminate()
            try:
                srv.wait(20)
            except subprocess.TimeoutExpired:
                srv.kill()
        shutil.rmtree(tmp, ignore_errors=True)


if __name__ == "__main__":
    sys.exit(main())
