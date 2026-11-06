# T08 / a08: huge assets, slow drip and proxies

Slice: a very large `Content-Length`, a body delivered a few bytes at a time (stall watchdog), and a
proxy that strips headers. Test file: `internal/update/slow_test.go`. Fix: `internal/update/update.go`
only (T08 owns it; `internal/engine/download.go` belongs to T09, so nothing there was touched).

All four tests drive `update.Download` / `update.Check` through `httptest` servers. They set `TMPDIR`,
`HOME` and `EZLM_HOME` to a `t.TempDir()`, so `Download`'s `os.TempDir()/EZLM-update` writes stay in
the test's folder.

## Findings

### F1 (major, fixed): an asset declared far too big is downloaded anyway

`Download` passed the release's asset `Size` straight to `engine.Download` as the expected size
(`internal/update/update.go:117` before the change) and never bounded it. GitHub's `size` is just a
number from the API; a release (or a compromised/odd API response) that declares 40 GB makes the
updater stream 40 GB into `os.TempDir()/EZLM-update` before the size check at the end of
`engine.Download` can object. On a small disk that fills the volume first; the error the user sees is
only "the downloaded file came out the wrong size" after the fact. An installer is tens of MB, so
anything near a gigabyte is already wrong.

Reproduction: `TestDownloadRejectsHugeAsset` serves a 14-byte body, the release declares
`Size: 40 << 30`, and the cap is lowered to 1 MiB for the test. Before the fix the test failed with
`error does not explain the size: the downloaded file came out the wrong size (14 bytes instead of
42949672960)` and the server saw one request. After the fix the test passes: no request is made and
the error says the file is far bigger than an EZLM installer should be.

Fix: `maxAssetSize` (1 GiB) at `internal/update/update.go:199`, checked before the temp dir is even
created (`internal/update/update.go:113`). The check runs before any network traffic and before any
file is opened, so it costs nothing.

Residual risk: the cap uses the size GitHub reports. If `size` is 0 and the response's
`Content-Length` is huge, the cap cannot see it; `engine.Download` only uses `Content-Length` for
progress, not for a bound. That case is only caught at the end by the digest or the size check, and
when both `size` and `digest` are missing the file is accepted. That is the missing-digest hole
(a04), not fixed here.

### F2 (major, fixed): a slow drip defeats the stall watchdog

`engine`'s watchdog (`internal/engine/download.go`, `stallTimeout`) only fires when *nothing* arrives
for 45 s; every byte resets the clock. A server (or a proxy buffering a trickle) that sends one byte
every 44 s keeps a download alive for as long as it likes: a 100 MB installer at that rate is over a
century, and the app sits on "Downloading EZLM" with a frozen progress bar. The updater had no
throughput floor of its own.

Reproduction: `TestDownloadCutsSlowDrip` serves 200 bytes one at a time every 50 ms (20 B/s) with a
correct `Content-Length` and digest, and a 15 s context. Before the fix `Download` returned success
after 10.4 s ("Download accepted a 20 B/s drip"); the whole file had dribbled in and was installed.
After the fix it fails cleanly in ~0.26 s with "the update download was too slow".

Fix: `Download` wraps the caller's context and the progress callback in a windowed speed check
(`internal/update/update.go:129`). Each `slowWindow` (30 s) window, the bytes received in that window
must be at least `slowFloor` (16 KiB/s); otherwise the request is cancelled and `Download` returns a
plain "too slow" error. Windows reset every period, so an initial burst cannot buy minutes of drip.
The `.part` file is kept, so the next attempt resumes. The defaults live at
`internal/update/update.go:205`; tests override both.

Note: the progress callback is now always non-nil, so `engine.Download` reports progress even when
the caller passed nil. That is harmless and is what the check needs.

Residual risk: a drip slower than 16 KiB/s but in short bursts that happen to land just over the floor
each window still passes; the floor is a trade-off so genuinely slow connections (modem, congested
Wi-Fi) are not cut. If a window is cut, the partial file stays on disk until the next attempt (same as
any other failed attempt).

### F3 (none, regression test): a header-stripping proxy is tolerated

A corporate/security proxy may answer a `Range` request with `200 OK`, strip `Content-Length`, `ETag`
and `Accept-Ranges`, and chunk the body. The updater copes: a 200 response truncates and restarts,
and the final size and SHA-256 are still checked. `TestDownloadThroughHeaderStrippingProxy` seeds a
partial `.part` (so the request carries `Range`), sends it through a proxy that always returns the
full body with no length header, and asserts the installed bytes and digest match.
`TestCheckThroughHeaderStrippingProxy` does the same for the release list JSON with the API response
headers stripped; `Check` still selects the asset. Both passed before the change and still pass, so
they are regression cover, not a bug claim.

Residual risk (reported, not fixed): if a proxy strips `Content-Length` *and* the release metadata
has no size and no digest, a truncated body is accepted. If the proxy returns `206` with a body from
the wrong offset and no `Content-Range`, the append corrupts the file; the size check catches it when
`size` is known, otherwise only the digest can. Both are a03/a04 territory.

## Before/after summary

| Test | Before | After |
|---|---|---|
| `TestDownloadRejectsHugeAsset` | downloads, then "wrong size" error | refused before any request |
| `TestDownloadCutsSlowDrip` | installed 200 B at 20 B/s in 10.4 s | stopped in 0.26 s with "too slow" |
| `TestDownloadThroughHeaderStrippingProxy` | pass | pass |
| `TestCheckThroughHeaderStrippingProxy` | pass | pass |

## Checks run

`gofmt -l .` (empty), `go build ./...`, `go vet ./...`, `go test -p 2 ./...` all pass in the
worktree.

## Open questions

- Is 1 GiB the right cap for the asset size? It is 10x the current installers; it exists to catch
  nonsense, not to police growth. If the apps ever ship a universal bundle, revisit.
- Should the updater reject an asset whose metadata has no `size` at all? That would close the
  Content-Length hole from F1 but overlaps a04's missing-digest fix; left to the Lead.
- A slow-drip cut leaves a `.part` behind. If the next attempt starts from a proxy that keeps
  trickling, the user sees repeated "too slow" errors rather than one clear failure. A retry count
  for the slow case would be a small follow-up.
