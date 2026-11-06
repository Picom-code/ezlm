# Remote catalog vs embedded catalog

Compared on 2026-11-06 (T01 step 5). This is a read-only comparison; no
catalog file was changed.

| | Remote | Embedded |
|---|---|---|
| URL / path | `https://picom-code.github.io/ezlm/internal/catalog/catalog.json` | `internal/catalog/catalog.json` |
| HTTP status | 200 | n/a (in the worktree) |
| Bytes | 73,739 | 73,739 |
| SHA-256 | `4bef13e184ae42523e963076a1037f0a873b8b24dee8178d621c107606886141` | `4bef13e184ae42523e963076a1037f0a873b8b24dee8178d621c107606886141` |
| Version | 8 | 8 |
| Models | 32 | 32 |
| `Last-Modified` | Wed, 04 Nov 2026 22:47:14 GMT | n/a |
| `ETag` | `"6aebb5f2-1200b"` (`0x1200b` = 73,739 bytes) | n/a |

## Result

**No differences.** The two files are byte-for-byte identical (`cmp` exit 0),
and a recursive walk of every JSON leaf found 0 paths only in the remote copy,
0 paths only in the embedded copy, and 0 changed values.

What the comparison covered:

- `version` (8 in both) and model count (32 in both).
- Model ids and their order in `models[]`: identical, 32 of 32, same order
  from `ternary-bonsai-2-27b` to `granite-4.2-3b`. No model present in only
  one catalog.
- Every scalar field of every model, including `name`, `family`, `vendor`,
  `country`, `license`, `params_b`, `active_params_b`, `moe`, `score`,
  `score_source`, `layers`, `kv_heads`, `head_dim`, `kv_layers`, `ctx_max`,
  `tool_calling`, `vision`, `native_1bit`, `no_kv_quant`, `repo`, `notes`
  and the optional `swa_*`, `draft` and `lazy_bytes` fields.
- Every quant of every model: `name`, `files` (names and order), `bytes`,
  `repo` and `mtp`. No quant was added, removed, reordered or changed.
- `tools` (absent in both).

So there is no changed field, changed quant or changed version to report.

## Cross-check of the URL the Go code actually uses

`internal/catalog/catalog.go:24` fetches
`https://raw.githubusercontent.com/picom-code/ezlm/main/internal/catalog/catalog.json`,
not the Pages URL. That URL also answers 200 with 73,739 bytes and SHA-256
`4bef13e184ae42523e963076a1037f0a873b8b24dee8178d621c107606886141`, so the
Pages copy and the copy EZLM downloads are the same bytes today.

## Method

```sh
curl -sS -D headers.txt -o /tmp/remote-catalog.json \
  https://picom-code.github.io/ezlm/internal/catalog/catalog.json
cmp /tmp/remote-catalog.json internal/catalog/catalog.json
python3 - <<'EOF'   # flatten both JSON trees to leaf paths and diff
...
EOF
```

The flattening step compares leaves by path, so a reordered key or a changed
value would show up even though `cmp` alone would already have caught any byte
difference.
