# T09 / a01: Hugging Face 302 to CDN redirects, including a changed target on resume

## Fault

Hugging Face answers `https://huggingface.co/<repo>/resolve/main/<file>` with a
302 to a signed, expiring CDN URL. The target differs on every request. A
download that drops partway is resumed from the `.part` file, and the resume
goes back through the original URL, which hands out a *new* target. The resume
must follow the new target with its `Range` header intact, append to the part
file and finish with the right bytes.

## Reproduced

Yes, as a scenario; **no defect in the current code**. Four `httptest` cases in
`internal/engine/download_redirect_test.go` exercise it:

1. `TestDownloadFollowsRedirectToCDN` - HF 302 on one host, CDN on another
   host; whole file arrives, progress total is the full size, one request per
   server.
2. `TestDownloadResumeFollowsChangedRedirect` - CDN cuts the connection after
   a third of the file; the retry hits the original URL, HF issues `?sig=2`
   (different from `?sig=1`), and the CDN receives `Range: bytes=<cut>-`
   together with the new signature. File bytes match exactly.
3. `TestDownloadResumeAcrossRestartFollowsNewRedirect` - a pre-existing
   `.part` file (app restart) resumes through a freshly issued target and only
   fetches the missing half.
4. `TestDownloadFollowsRelativeRedirect` - same-host relative `Location`.

Before this change there was no redirect coverage at all; the tests pass on the
unmodified `download.go`.

## Why it already works

- `fetch` sets `Range` on the request *before* `dlClient.Do`. Go's
  `http.Client` follows redirects and its header copier forwards every
  non-sensitive initial header, so `Range` reaches the CDN. Only
  `Authorization`, `WWW-Authenticate`, `Cookie`, `Cookie2`,
  `Proxy-Authorization` and `Proxy-Authenticate` are restricted, and only
  across hosts (`net/http/client.go`, `makeHeadersCopier`).
- `Download` never caches the redirect target: every attempt calls `fetch`
  with the original URL, so an expired or changed target is re-resolved
  naturally. There is no stale CDN URL to reuse.
- `resp.ContentLength` is the length of the final (post-redirect) response,
  and the `have + ContentLength` total is correct for a 206.

## Fix

None needed. `internal/engine/download.go` is unchanged. The tests are a
regression guard: they fail if a future change adds a `CheckRedirect` that
drops `Range`, caches the first target, or stops re-requesting the original
URL.

## Evidence (before/after)

- Before: no redirect test; `go test -run Redirect ./internal/engine/` runs
  nothing.
- After: 4 tests pass; the changed-target test asserts
  `hfRange[1] == "bytes=<cut>-"` and that the second CDN hit carries a
  different `sig` from the first.
- Whole package: `gofmt -l .` empty, `go vet ./internal/engine/` clean,
  `go test -p 2 -count=1 ./internal/engine/` ok (9.6 s).

## Severity

None for the fault itself. Minor (coverage): the redirect path had no test
before, so a regression in header forwarding or target resolution would have
gone unnoticed.

## Residual risk / open questions

- Go's client has no cookie jar here, so `Set-Cookie` on a 302 is not carried
  to the CDN. Hugging Face's redirect does not need it today; if the CDN ever
  starts requiring a redirect cookie, this would fail. Minor.
- A changed target can point at a re-uploaded file of the same size; the
  resume would append old and new bytes and the size check would not notice.
  Detecting that needs `ETag`/`Content-Range` validation (a03's fault), not
  redirect handling.
- Transparent gzip (`Accept-Encoding` added by the transport) combined with
  `Range` could make offsets refer to compressed bytes. Not seen with GGUF on
  the HF CDN; not redirect-specific.
- The stall watchdog starts before the redirect chain; a redirect hop slower
  than `stallTimeout` counts as a stall. Real HF redirects are sub-second, so
  this is theoretical (a04 owns slow-but-alive links).
