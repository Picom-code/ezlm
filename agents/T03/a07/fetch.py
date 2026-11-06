import json, sys, os
sys.path.insert(0, 'scripts')
import verify_catalog as v

MODELS = [
    ("gemma-4-e4b", "unsloth/gemma-4-E4B-it-GGUF", "gemma-4-E4B-it-UD-IQ2_M.gguf"),
    ("gemma-4-e2b", "unsloth/gemma-4-E2B-it-GGUF", "gemma-4-E2B-it-UD-IQ2_M.gguf"),
]
out = "agents/T03/models"
for mid, repo, file in MODELS:
    url = f"https://huggingface.co/{repo}/resolve/main/{file}"
    ver, kv, types = v.header(url)
    print(mid, "gguf version", ver, "keys", len(kv))
    tmpl = {}
    for k, val in kv.items():
        if k == "tokenizer.chat_template" or k.startswith("tokenizer.chat_template."):
            tmpl[k] = val
    print(" templates:", sorted(tmpl))
    for k, val in sorted(tmpl.items()):
        print(f"  {k}: {len(val)} chars")
    with open(os.path.join(out, mid + ".templates.json"), "w") as f:
        json.dump(tmpl, f, indent=1, sort_keys=True)
        f.write("\n")
    meta = {k: kv[k] for k in sorted(kv) if not k.startswith("tokenizer.ggml.") and k not in tmpl}
    with open(os.path.join(out, mid + ".gguf-meta.json"), "w") as f:
        json.dump({"url": url, "gguf_version": ver, "metadata": meta}, f, indent=1, sort_keys=True)
        f.write("\n")
