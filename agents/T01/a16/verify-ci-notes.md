# verify_catalog.py: CI mode

`scripts/verify_catalog.py` can now run unattended. The default human report
is unchanged; two things were added: a `--json` flag and exit codes that
distinguish "checked and clean" from "checked and found a problem" from "could
not check".

## Flags

| Flag | Default | Meaning |
|---|---|---|
| `--json` | off | Print one JSON object on stdout instead of the human report. Nothing else is printed on stdout in this mode. |
| `--tag` | `b11960` | llama.cpp release tag. bNNNN tags and semantic tags such as `v0.9.0` both work. |
| `--catalog` | `internal/catalog/catalog.json` | Catalog to check. |
| `--only` | all models | Comma-separated model ids. An id that is not in the catalog is now a usage error. |
| `--jobs` | 8 | Worker threads. Must be at least 1. |

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Every file set was read and is fine. Warnings do not fail the run. |
| 1 | At least one definite mismatch: missing file, wrong byte size, bad split naming, unknown architecture or tensor type, unreadable header. |
| 2 | The run could not be trusted: bad arguments, unreadable or malformed catalog, unknown `--only` id, or a network error while fetching. |

Code 2 wins over 1 when both happen, because a partial read means the clean
parts were not fully checked either.

The line between 1 and 2 is the kind of failure:

- A 404/401, a file that is not GGUF, a truncated header, or a size or
  metadata mismatch is a definite answer about the catalog, so exit 1.
  Hugging Face answers 401 rather than 404 for a repo that does not exist, so
  a missing repo still lands in this bucket.
- Timeouts, connection resets and 5xx answers after the existing retries mean
  the file could not be read, so exit 2.

A run with exit 0 and warnings still has `"status": "warn"` rows in the JSON,
so CI can surface them without failing.

## JSON output

`--json` prints one object, with keys in this order:

```json
{
  "tag": "b11960",
  "catalog": "internal/catalog/catalog.json",
  "architectures": 166,
  "tensor_types": 44,
  "checked": 13,
  "ok": true,
  "exit_code": 0,
  "counts": {"ok": 13, "warn": 0, "error": 0, "network_error": 0},
  "results": [
    {
      "model": "granite-4.2-3b",
      "quant": "Q2_K",
      "status": "ok",
      "findings": [{"level": "ok", "message": "ok"}]
    }
  ]
}
```

- `results` is in catalog order (model, then quant), not completion order, so
  the output is stable between runs.
- `status` is one of `ok`, `warn`, `error`, `network_error`.
- `findings[].level` is `ok`, `warn`, `error` or `neterr`; `message` is the
  same text the human report prints.
- `exit_code` repeats the process exit code inside the document, so a log
  artifact is self-describing.
- On a usage or network failure before the check starts, the object is
  shorter: `{"tag": ..., "catalog": ..., "error": "...", "exit_code": 2}`.
- Argparse usage errors (for example `--jobs 0`) still print argparse's own
  text and exit 2; they happen before `--json` is parsed.

## CI recipes

Fail the job on a mismatch or an inconclusive run:

```sh
python3 scripts/verify_catalog.py --tag b11960 --jobs 2 --json > verify.json
code=$?
case $code in
  0) echo "catalog matches $TAG" ;;
  1) echo "catalog mismatch, see verify.json" >&2 ;;
  2) echo "verifier could not complete, rerun" >&2 ;;
esac
exit $code
```

Read the counts with `jq`:

```sh
jq -r '.counts, (.results[] | select(.status != "ok") | "\(.model) \(.quant) \(.status)")' verify.json
```

Gate one model while editing it:

```sh
python3 scripts/verify_catalog.py --only granite-4.2-3b --jobs 2 --json | jq -e '.ok'
```

## Tested

| Command | Exit | Result |
|---|---|---|
| `--only granite-4.2-3b --jobs 2` | 0 | same human lines as before, `0 file set(s) with errors` |
| `--only granite-4.2-3b --jobs 2 --json` | 0 | valid JSON, `counts.ok` 8, `ok` true |
| `--only granite-4.2-3b --catalog <copy with bytes+1> --json` | 1 | `status` `error`, `ERROR size ... != catalog ...` |
| `--only granite-4.2-3b --catalog <copy with a nonexistent repo>` | 1 | `ERROR missing file: HTTP Error 401: Unauthorized` |
| `--tag no-such-tag-xyz` | 2 | `cannot read llama.cpp no-such-tag-xyz: HTTP Error 404` |
| `--catalog <file containing {}> --json` | 2 | `catalog ... has no usable models list: 'models'` |
| `--only nope` | 2 | argparse: `--only names unknown model(s): nope` |

Both `verify-b11960.txt` and `verify-latest.txt` in this folder were produced
by the script; the human output did not change when `--json` was added.
