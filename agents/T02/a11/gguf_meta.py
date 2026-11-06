#!/usr/bin/env python3
"""Print the GGUF metadata the catalog needs, using range requests only.

  gguf_meta.py <repo> <file> [<repo> <file> ...]
"""
import sys, os

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "..", "..", "scripts"))
import verify_catalog as vc  # noqa: E402

WANT = ("block_count", "attention.head_count", "attention.head_count_kv", "attention.key_length",
        "attention.value_length", "attention.kv_lora_rank", "rope.dimension_count", "context_length",
        "attention.sliding_window", "attention.sliding_window_pattern", "attention.shared_kv_layers",
        "full_attention_interval", "nextn_predict_layers", "expert_count", "expert_used_count",
        "embedding_length", "feed_forward_length", "split.count", "attention.layer_norm_rms_epsilon")


def show(repo, path):
    url = f"https://huggingface.co/{repo}/resolve/main/{path}"
    ver, kv, types = vc.header(url)
    arch = kv.get("general.architecture")
    print(f"== {repo}/{path}")
    print(f"   version {ver} arch {arch} name {kv.get('general.name')} types {sorted(types)}")
    for k in sorted(kv):
        short = k[len(arch) + 1:] if k.startswith(f"{arch}.") else k
        if short in WANT or k in ("tokenizer.chat_template", "general.file_type"):
            val = kv[k]
            if k == "tokenizer.chat_template":
                val = f"<{len(val)} chars>" if isinstance(val, str) else val
            print(f"   {k} = {val}")


args = sys.argv[1:]
for i in range(0, len(args), 2):
    try:
        show(args[i], args[i + 1])
    except Exception as e:
        print(f"ERROR {args[i]}/{args[i+1]}: {e}")
