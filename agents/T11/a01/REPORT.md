# T11 a01: `internal/catalog` tests

Branch `agent/T11-a01`. Worktree `~/ezlm-agents/work/subs/T11/a01`.
New file only: `internal/catalog/catalog_test.go`. No non-test file touched.

## Coverage

| | statements |
|---|---|
| before | 0.0% (no test files) |
| after | **100.0%** |

```
go test -p 2 -cover ./internal/catalog
ok  github.com/picom-code/ezlm/internal/catalog  0.4s  coverage: 100.0% of statements
```

Per function: `KVBytesPerToken`, `SWABytes`, `FileURL`, `Draft.URL`, `parse`,
`sortModels`, `Find` all 100%; `Load` 100% after the embedded-panic test.

## What is covered

- **Loading** (`TestLoad*`): `EZLM_CATALOG` local file wins and does not hit the
  network; malformed, model-less and missing local files fall back to the
  embedded catalog; remote newer wins and is sorted; older, equal, version-0,
  model-less, malformed, 500 and 404 responses are ignored; unreachable remote
  falls back; the 30 minute cache serves the cached copy without a fetch and
  refetches after expiry; a cached embedded catalog also suppresses refetches;
  an invalid embedded catalog panics as documented.
- **Validation** (`TestEmbeddedCatalogValidates`): every shipped model has a
  slug id (unique), name/family/license/org-repo, positive score and params,
  usable layer/head/context numbers, `0 <= kv_layers <= layers`, valid country
  code, complete SWA and draft fields, non-zero KV cost. Every quant has a
  unique name, bytes, at least one relative `.gguf` file, a valid quality
  override and a Hugging Face URL. The list is sorted by score.
- **Ranking** (`TestParse`): score-descending sort, file order kept on ties,
  version preserved, `tools` pins preserved byte for byte.
- **Quant selection helpers**: `FileURL` (model repo, quant repo override,
  subdirectory file), `Draft.URL`, `KVBytesPerToken` (f16, q8_0, q4_0,
  unknown/empty type, `kv_layers` override), `SWABytes` (no SWA, incomplete
  SWA, window plus one batch, context cap, zero context).
- **Malformed JSON / missing fields** (`TestParse` table, `TestParseDecodesEveryField`):
  truncated input, trailing data, `{}`, `[]`-shaped model list, `null`, wrong
  field type; every documented JSON field decoded, including the optional ones.
- **Version handling**: embedded is the floor, remote must be strictly newer;
  version 0 and absent version never replace it.

## Bugs found (skipped tests)

1. **`TestParseAcceptsQuantWithoutFiles`** (major, owner: catalog/app).
   `parse` accepts a quant with `"files": []`. `internal/app/app.go:1103`
   (`Installed`) indexes `q.Files[0]` without a length check, so a catalog
   shipped that way panics the Installed list. Evidence: the test parses such a
   catalog and shows `len(Files) == 0`; the index expression is at
   `internal/app/app.go:1103` (also `internal/app/app.go:1109` builds the path
   from `q.Files[0]`). Fix in `parse` (reject a model with a quantless/empty
   quant) or guard in `Installed`. The shipped `catalog.json` is clean.
2. **`TestParseRejectsDuplicateIDs`** (minor, owner: catalog).
   `parse` accepts two models with the same id. `Find` (catalog.go:208) returns
   the first, so `Use`/`Installed` silently resolve the duplicate to one copy;
   with the score sort, which copy wins depends on scores. The shipped catalog
   has no duplicates.

## Open questions

- `Load` holds `cacheMu` across the 4 second remote fetch, so concurrent
  `Load` calls serialize behind a slow network. Latency only, not tested.
- The remote path decodes with `json.Decoder.Decode` (accepts trailing bytes)
  while the local/embedded path uses `json.Unmarshal` (rejects them). Lenient,
  not a bug today.
- `Catalog.Tools` is only applied through `app.useCatalogPins`; a remote
  catalog without `tools` clears overrides, which matches the documented
  "built-in pins apply" behaviour.
