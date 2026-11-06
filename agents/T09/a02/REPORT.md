# T09 / a02: servers that ignore `Range`

**Fault:** a server answers a resume request (`Range: bytes=N-`) with `200 OK` and the
whole file. Two variants exist in the wild: `200` with the whole body (correct by
definition) and `206` with the whole body from byte 0 (a lying `Content-Range`).

**Branch:** `agent/T09-a02` · **Commit:** `2bf2306` · **Files:** `internal/engine/download.go`,
`internal/engine/download_range_test.go` (new, 4 tests).

## Reproduction

Before the fix, `Download` appended a `206` body to the existing `.part` file whenever the
status was `206`, without looking at `Content-Range`. A pre-seeded 1000-byte part file
plus a `206` that sends the whole 69,632-byte file produced:

| case | `want` | before | after |
|---|---|---|---|
| `200` + whole body | 69,632 | correct, 1 request | correct, 1 request |
| `206` + whole body, `Content-Range: bytes 0-69631/69632` | 0 | **69,632 → 70,632 bytes, corrupt, no error** | correct |
| same, sized | 69,632 | size error, **part file deleted** (all progress lost) | correct |
| `206` from byte 500 when byte 1000 was asked for | 69,632 | appended, size error, part file deleted | fails at once, part file kept |

The `200` path was already handled by the `StatusOK` branch (`have = 0` + `O_TRUNC`,
`download.go:159-162`), so the real gap was the `206` that lies about its range. That
variant matters for the same reason as the `200` one: Hugging Face's CDN can answer a
resume with a full-body `206`, and a silently corrupted multi-GB `.gguf` is far worse than
a failed download. Note that with `want == 0` there is no size check at all, so the
corruption reached `os.Rename` and became the final file.

## Findings

1. **`download.go:144` (was) — a `206` response was appended without checking
   `Content-Range`.** Severity **major**: silent corruption of the final model file when
   the caller passes `want == 0` (the size check is the only guard, `download.go:87-96`,
   and it is skipped then); lost progress when `want > 0`. Fixed.
2. **`download.go:141-162` — a `200` reply to a resume is already restarted correctly**
   (truncate, write from 0, progress total recomputed from the new `Content-Length`).
   Severity **none**; locked in by `TestDownloadRestartsWhenServerIgnoresRange`.
3. **Observation, not changed (out of slice): `download.go:166`** — a `416` returns
   `nil` ("already complete") without checking the part file's size. A stale part file
   that is *larger* than the server file is renamed as the model when `want == 0`. This
   needs the file to change on the server, so it belongs with a03/a11. Severity
   **minor** here.

## Fix (`download.go`, +31 lines)

- New helper `contentRangeStart` (`download.go:250`) parses the first offset from
  `Content-Range: bytes N-M/T`; it reports "unknown" for a missing or malformed header,
  which keeps the old append behaviour.
- In the `206` branch (`download.go:144-157`): if the header names an offset other than
  the one requested, the body is not a continuation.
  - start `0` means the server ignored `Range`: truncate the part file and write from 0,
    the same as the `200` path.
  - any other start means the body is neither the whole file nor the requested tail, so
    it cannot be appended or truncated safely: return `permanent` with a plain message
    (`download.go:152`). `Download` does not retry it and the part file is kept.

No behaviour change for servers that honour `Range`, for `200`, or for a `206` with no
`Content-Range` header.

## Tests (`internal/engine/download_range_test.go`)

- `TestDownloadRestartsWhenServerIgnoresRange` — `200` + whole body over a seeded part
  file; asserts the resume request really carried `Range`, one request, exact bytes.
- `TestDownloadRestartsWhenPartialContentIgnoresRange` — `206` + whole body, `want == 0`
  (the silent-corruption case). Failed before, passes after.
- `TestDownloadRestartsWhenPartialContentIgnoresRangeSized` — same with a known size.
  Failed before (size error, part deleted), passes after.
- `TestDownloadWrongContentRangeFailsFast` — `206` from an offset nobody asked for;
  asserts one request, a message naming the offset, and the part file preserved.

All four were run against the pre-fix code (one passed, three failed) and against the
fixed code (four passed). `go test -p 2 ./internal/engine/` passes (7.8 s),
`gofmt -l internal/engine/` is empty, `go vet ./internal/engine/` and `go build ./...` pass.

## Residual risk

- A `206` that reports the correct `Content-Range` but sends the whole body anyway is
  still appended and corrupts the file. Only a size or checksum check (a07) catches that;
  `Content-Range` is the only cheap signal.
- A `200` whose body is only the tail (no `Content-Length` check against `want`) is
  treated as the whole file. With `want > 0` the size check catches it; with `want == 0`
  it does not.
- Restarting on a `200`/full-body `206` throws away the bytes already on disk. On a
  server that ignores `Range` *and* drops the connection at a fixed offset, the download
  makes no net progress and gives up after five attempts
  (`download.go:69-80`) with "the connection to the download server keeps dropping". That
  is a clean failure, but the message blames the connection, not the server.

## Open questions

- Should a full-body restart be reported to the caller (progress jumps back to 0)? The
  progress callback sees `done` fall from `have` to 0. `internal/app` is read-only for
  this task; a13 owns what the caller sees.
- Is failing fast on a wrong `Content-Range` (instead of one blind retry without
  `Range`) the right trade-off? It keeps the part file and is deterministic, but a
  transient proxy bug would need the user to press download again.
