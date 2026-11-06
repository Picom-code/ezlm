#!/usr/bin/env python3
"""Build agents/T02/tiers/tier-128plus.json.

Sizes and file lists come from the Hugging Face tree API, never from a
download; the entries themselves are written by hand below.
"""
import json, sys, urllib.request

UA = {"User-Agent": "ezlm-t02-a11"}
OUT = "agents/T02/tiers/tier-128plus.json"


def tree(repo, path):
    url = f"https://huggingface.co/api/models/{repo}/tree/main/{path}"
    req = urllib.request.Request(url, headers=UA)
    with urllib.request.urlopen(req, timeout=60) as r:
        return [e for e in json.load(r) if e["type"] == "file"]


def quant(repo, name, path, lazy=0, mtp=False):
    try:
        files = tree(repo, path)
    except urllib.error.HTTPError:
        # the quant is a single file at the repo root, not a directory
        files = [f for f in tree(repo, "") if name in f["path"].rsplit("/", 1)[-1] and f["path"].endswith(".gguf")]
    q = {"name": name, "files": [f["path"] for f in files],
         "bytes": sum(f.get("size") or 0 for f in files), "repo": repo}
    if lazy:
        q["lazy_bytes"] = lazy
    if mtp:
        q["mtp"] = True
    return q


TBL = 256 * 768022850  # per_layer_token_embd.weight elements in DeepSeek V4.1 Flash


def lazy(bpw):
    return int(TBL * bpw / 8)


MODELS = [
    {
        "id": "glm-5.3-flash",
        "name": "GLM-5.3 Flash",
        "family": "GLM-5.3",
        "vendor": "Z AI",
        "country": "CN",
        "license": "MIT",
        "params_b": 321.32,
        "moe": True,
        "score": 27.8,
        "score_source": "Artificial Analysis Intelligence Index v5.0 (artificialanalysis.ai/leaderboards/models, fetched 2026-11-06): 27.8, thinking at the default max effort.",
        "layers": 45,
        "kv_heads": 1,
        "head_dim": 512,
        "ctx_max": 1048576,
        "tool_calling": True,
        "repo": "zai-org/GLM-5.3-Flash",
        "active_params_b": 18.0,
        "kv_layers": 12,
        "vision": True,
        "native_1bit": False,
        "notes": "Released 2026-08-25. 320B total, 18B active (card). First GLM with hybrid sparse and linear attention: only every fourth layer keeps a KV cache (12 of 45), so the cache is about 24 KB/token f16 and long context is cheap. Natively multimodal, mmproj is in the GGUF repo. 1M context, MIT. llama.cpp arch glm5-next (b11960). At 128 GB the IQ2_XXS (101.8 GB) and Q2_K_XL (108.7 GB) files fit with room for 64K context (112.0 GB planned). IQ3_XXS (120.4 GB, 123.8 GB planned) needs a 160 GB machine, and Q3_K_XL (147.5 GB) a 192 GB one.",
        "quants": [("unsloth/GLM-5.3-Flash-GGUF", p) for p in
                   ("UD-IQ1_S", "UD-IQ1_M", "UD-IQ2_XXS", "UD-Q2_K_XL", "UD-IQ3_XXS", "UD-Q3_K_XL")],
    },
    {
        "id": "ornith-1.5-122b",
        "name": "Ornith 1.5 122B",
        "family": "Ornith 1.5",
        "vendor": "Ornith AI",
        "country": "US",
        "license": "MIT",
        "params_b": 125.09,
        "moe": True,
        "score": 26.0,
        "score_source": "ESTIMATE: not on the Artificial Analysis leaderboard on 2026-11-06. Placed from developer-reported evals on the model card (2026-10-16) against models that are on it: SWE-bench Pro 76.9 vs 43.9 for Qwen3.8-Flash-Next, SWE-bench Verified 84.5 vs 82.8, HLE with tools 48.0 vs 38.0, Terminal-Bench 2.1 81.2 vs 83.4. Qwen3.8-Flash-Next carries an ESTIMATE of 27.0 in the current catalog, so 26.0 is a conservative placement. Treat as UNVERIFIED.",
        "layers": 48,
        "kv_heads": 2,
        "head_dim": 256,
        "ctx_max": 262144,
        "tool_calling": True,
        "repo": "ornith-ai/Ornith-1.5-122B",
        "active_params_b": 6.0,
        "kv_layers": 12,
        "vision": True,
        "native_1bit": False,
        "notes": "Released 2026-10-16. 125B total, about 6B active (8 of 256 experts; estimated from the config). Built on Qwen3.5 and Gemma4 bases with a self-improvement RL loop. Hybrid attention, full attention every fourth layer (12 of 48), so the cache is about 24 KB/token f16. The card's benchmark numbers are developer-run with harness notes (Claude Code 2.1.126, OpenHands, 256K context), not independent. The card's MIT licence link is a 404, so the licence is UNVERIFIED. Vendor country is not stated on the card. GGUF repo includes mmproj. llama.cpp arch qwen35moe.",
        "quants": [("unsloth/Ornith-1.5-122B-GGUF", p) for p in
                   ("UD-IQ1_M", "UD-IQ2_XXS", "UD-Q2_K_XL", "UD-IQ3_XXS", "UD-Q3_K_XL",
                    "UD-Q4_K_XL", "UD-Q5_K_XL", "UD-Q6_K_XL")],
    },
    {
        "id": "minimax-m3.1-flash",
        "name": "MiniMax-M3.1-Flash",
        "family": "MiniMax M3.1",
        "vendor": "MiniMax",
        "country": "CN",
        "license": "minimax-community",
        "params_b": 428.0,
        "moe": True,
        "score": 29.06,
        "score_source": "Artificial Analysis Intelligence Index v5.0 (artificialanalysis.ai/leaderboards/models, fetched 2026-11-06): 29.06.",
        "layers": 60,
        "kv_heads": 4,
        "head_dim": 128,
        "ctx_max": 1048576,
        "tool_calling": True,
        "repo": "MiniMaxAI/MiniMax-M3.1-Flash",
        "active_params_b": 23.0,
        "kv_layers": 60,
        "vision": True,
        "native_1bit": False,
        "notes": "Released 2026-10-17. 428B total, 23B active (card). Native multimodal (text, image, video), 1M context, minimax-community licence. Card claims (developer-run): DeepSWE v1.1 73.7, OSWorld V2 61.7, CyberGym 82.2. Full attention on all 60 layers, 4 KV heads of 128, so the cache is about 123 KB/token f16 and 7.9 GB at 64K. The smallest file is UD-IQ1_M at 129.4 GB (139.3 GB planned), but the 0.85 quant floor skips it, so the first usable quant is IQ2_XXS at 144.9 GB planned: a 160 GB machine at 32K context, a 192 GB machine for IQ3_XXS (170.3 GB planned). llama.cpp arch minimax-m3. mmproj included; a DSpark draft file is published separately.",
        "quants": [("unsloth/MiniMax-M3.1-Flash-GGUF", p) for p in
                   ("UD-IQ1_M", "UD-IQ2_XXS", "UD-Q2_K_XL", "UD-IQ3_XXS", "UD-Q3_K_XL", "UD-Q4_K_XL")],
    },
    {
        "id": "glm-5.3",
        "name": "GLM-5.3",
        "family": "GLM-5.3",
        "vendor": "Z AI",
        "country": "CN",
        "license": "glm-5.3 (custom)",
        "params_b": 753.33,
        "moe": True,
        "score": 31.65,
        "score_source": "Artificial Analysis Intelligence Index v5.0 (artificialanalysis.ai/leaderboards/models, fetched 2026-11-06): 31.65 at max effort.",
        "layers": 78,
        "kv_heads": 1,
        "head_dim": 576,
        "ctx_max": 1048576,
        "tool_calling": True,
        "repo": "zai-org/GLM-5.3",
        "active_params_b": 32.0,
        "kv_layers": 78,
        "vision": False,
        "native_1bit": False,
        "notes": "Released 2026-08-25. 753B total, top-8 of 256 experts, about 32B active (estimated from the config; the card does not state it). MLA attention with a 576-wide compressed KV (512 latent plus 64 rope), so the cache is about 180 KB/token f16 and 11.5 GB at 64K. Card benchmarks (developer-run): Terminal-Bench 2.1 88.2, Terminal-Bench 3.0 28.3, DeepSWE 66.9, NL2Repo 58.0, Toolathlon Verified 73.0. Custom glm-5.3 licence. Needs 320 GB or more: the smallest usable file is UD-IQ2_M at 238.6 GB (253.3 GB with a 64K cache); IQ1_S and IQ1_M are below the 0.85 quant floor internal/fit applies to 150B+ models. llama.cpp arch glm-dsa. Text only.",
        "quants": [("unsloth/GLM-5.3-GGUF", p) for p in
                   ("UD-IQ1_S", "UD-IQ1_M", "UD-IQ2_M", "UD-Q2_K_XL", "UD-IQ3_XXS", "UD-IQ4_XS")],
    },
    {
        "id": "deepseek-v4.1-flash-1017",
        "name": "DeepSeek V4.1 Flash 1017",
        "family": "DeepSeek V4.1",
        "vendor": "DeepSeek",
        "country": "CN",
        "license": "MIT",
        "params_b": 763.21,
        "moe": True,
        "score": 32.59,
        "score_source": "Artificial Analysis Intelligence Index v5.0 (artificialanalysis.ai/leaderboards/models, fetched 2026-11-06): 32.59 at max effort.",
        "layers": 40,
        "kv_heads": 1,
        "head_dim": 512,
        "ctx_max": 1048576,
        "tool_calling": True,
        "repo": "deepseek-ai/DeepSeek-V4.1-Flash-1017",
        "active_params_b": 13.0,
        "kv_layers": 40,
        "vision": True,
        "native_1bit": False,
        "notes": "Released 2026-10-17. 763B total, of which 196.6B is a per-layer token lookup table (768M x 256, an engram) that can stay on disk with --lazy-mode: the resident body is about 122 GB at IQ1_S and 165 GB at IQ2_XXS. 40 layers, top-6 of 384 experts, about 13B active (estimated). 1M context, MIT, multimodal with mmproj in the GGUF repo. Card: HLE without tools 42.6, Chartography with tools 83.0. llama.cpp arch deepseek41. The first quant above the 0.85 floor is IQ2_XXS: 165.3 GB resident and 172.9 GB planned with a 64K cache, so 192 GB is the smallest machine, and IQ3_XXS (241.7 GB planned) needs 320 GB. Without lazy loading the smallest file is 233 GB.",
        "quants": [
            ("unsloth/DeepSeek-V4.1-Flash-1017-GGUF", "UD-IQ1_S", lazy(4.5)),
            ("unsloth/DeepSeek-V4.1-Flash-1017-GGUF", "UD-IQ1_M", lazy(4.5)),
            ("unsloth/DeepSeek-V4.1-Flash-1017-GGUF", "UD-IQ2_XXS", lazy(5.5)),
            ("unsloth/DeepSeek-V4.1-Flash-1017-GGUF", "UD-Q2_K_XL", lazy(5.5)),
            ("unsloth/DeepSeek-V4.1-Flash-1017-GGUF", "UD-IQ3_XXS", lazy(6.5625)),
        ],
    },
    {
        "id": "flint",
        "name": "Flint",
        "family": "Flint",
        "vendor": "Recursive",
        "country": "US",
        "license": "Apache-2.0",
        "params_b": 39.79,
        "moe": True,
        "score": 19.0,
        "score_source": "ESTIMATE: no Artificial Analysis entry on 2026-11-06. Placed at 19.0 from its 40B size and developer-reported coding evals on the model card (2026-10-21): Terminal-Bench 2.1 54.9, SWE-bench Pro v2 89.7, SWE-bench Verified 74.2. A general-intelligence index would likely be lower than its coding rank; treat as UNVERIFIED and do not compare with the general picks.",
        "layers": 49,
        "kv_heads": 4,
        "head_dim": 128,
        "ctx_max": 262144,
        "tool_calling": True,
        "repo": "RecursiveSI/flint",
        "active_params_b": 2.5,
        "kv_layers": 14,
        "swa_layers": 35,
        "swa_kv_heads": 4,
        "swa_head_dim": 128,
        "swa_window": 513,
        "vision": False,
        "native_1bit": False,
        "notes": "Released 2026-10-21. 39.8B total, about 2.5B active (card), built for coding agents. 35 sliding-window layers (window 513) and 14 full-attention layers, so the growing cache is about 29 KB/token f16 plus a fixed window cache; the small active set makes it fast with a long context. Coding specialist: the general score above is an ESTIMATE, the coding numbers are developer-run (Terminus 2 for Terminal-Bench, mini-swe-agent for SWE-bench). Apache-2.0. llama.cpp arch flint. Fits comfortably from 48 GB up, so this is the fast coding option for a 128 GB machine rather than a use of the tier's capacity.",
        "quants": [("unsloth/flint-GGUF", p) for p in
                   ("UD-Q4_K_XL", "UD-Q5_K_XL", "UD-Q6_K_XL")] + [("unsloth/flint-GGUF", "Q8_0")],
    },
]


def main():
    models = []
    for m in MODELS:
        entry = {k: v for k, v in m.items() if k != "quants"}
        entry["quants"] = []
        for spec in m["quants"]:
            if len(spec) == 2:
                repo, path = spec
                entry["quants"].append(quant(repo, path, path))
            else:
                repo, path, lb = spec
                entry["quants"].append(quant(repo, path, path, lb))
        entry["quants"].sort(key=lambda q: q["bytes"])
        models.append(entry)
        print(f"{entry['id']:28s} {len(entry['quants'])} quants, "
              f"{entry['quants'][0]['bytes'] / 1e9:7.2f} .. {entry['quants'][-1]['bytes'] / 1e9:7.2f} GB")
    doc = {"tier": "128plus", "models": models}
    with open(OUT, "w") as f:
        json.dump(doc, f, indent=2)
        f.write("\n")
    print("wrote", OUT)


if __name__ == "__main__":
    sys.exit(main())
