# T08 / a02: redirect chains in the updater

Slice: 301/302/307/308 chains, redirect loops, a redirect to another host, and a
redirect on the asset whose target changes between the check and the download.

Test file: `internal/update/redirect_test.go` (all cases use `httptest`; no real
network, no model, no real home).

## Findings

### F1 (major) The updater follows a redirect from https to plain http

Before this change the update check ran on `http.DefaultClient` (`internal/update/update.go:66`,
formerly `resp, err := http.DefaultClient.Do(req)`) and the installer download ran on
`dlClient` (`internal/engine/download.go:25-32`). Neither sets `CheckRedirect`, so Go's
default policy applies: follow up to ten hops, and accept a hop that drops from `https`
to `http`. Nothing in the updater looks at the scheme of the address the bytes came from.

So a captive portal, a company proxy or anything that can answer for the host can redirect
the release list, or the installer, to plain `http`. Both are then read and trusted. For the
installer the only content control left is GitHub's published digest, and
`verifyDigest` returns nil when the digest is empty (`internal/engine/engine.go:321-324`,
"GitHub did not publish one; size was already checked"). A swapped asset of the same size
therefore installs and is launched.

Evidence, pre-fix (`go test -run TestCheckRefusesPlaintextRedirect` against the old code):

```
--- FAIL: TestCheckRefusesPlaintextRedirect (0.00s)
    redirect_test.go:101: Check accepted a release list that came over plain http:
      {Current:0.1.0 Available:true Latest:0xc000... Error:}
```

### F2 (major) A redirect loop on the installer is retried five times, fifty requests, 20 seconds

`engine.Download` retries a failed attempt up to five times with backoff
(`internal/engine/download.go:57-86`). A loop is not a `permanent` error, so each of the five
attempts walks Go's ten-hop limit again. The download reports a generic connection error
after ~20 s.

Evidence, pre-fix (`TestDownloadStopsRedirectLoop`):

```
--- FAIL: TestDownloadStopsRedirectLoop (20.02s)
    redirect_test.go:241: the loop was followed 50 times, want at most 11
```

The check path is bounded by its 10 s context and its 10 hops, so a loop there is only a
misleading message, not a spin (F3).

### F3 (minor) A redirect problem was reported as "couldn't reach GitHub"

`Check` collapsed every transport error into
`"couldn't reach GitHub to check for updates"` (`internal/update/update.go:129-136`), which
sends the user to check their network for a server-side redirect problem. Now a chain that
leaves https and a chain that never ends each get their own sentence.

### F4 (minor, residual) The asset's redirect target can change between check and download

`Check` stores `browser_download_url` and `Download` fetches it later, so the target can be
different. The controls are the size and the published digest, both applied after the bytes
arrive: same-size swapped bytes are caught by the digest (`TestDownloadRedirectTargetChanged`
passes before and after) and the file is removed. When the release publishes no digest, a
swapped asset of the same size installs silently. That is a04's missing-digest fault; a02
only records it here.

## What changed (`internal/update/update.go`)

- `httpClient` (line 46-49): one client for the whole updater, with `CheckRedirect`.
- `redirectPolicy` (line 59-68): at most ten hops, a hop must keep a host, and a chain that
  started on https must stay on https. Two sentinel errors (`errInsecureRedirect`,
  `errTooManyRedirects`) so callers can name the fault.
- `resolveAsset` (line 80-103): follows the installer's redirect chain with a one-byte
  `Range` GET and returns the address the bytes actually come from. A policy violation is a
  hard error; any other probe failure returns the original URL, so a flaky probe never
  blocks a download that would have worked.
- `Download` (line 179-183): resolves the asset first, then hands the resolved address to
  `engine.Download`. Nothing is written before the chain is approved.
- `Check` (line 116-136): uses `httpClient`, and names the redirect fault in `Info.Error`.

Nothing outside `internal/update/**` was touched, so `engine`'s own redirect handling is
unchanged (see residual risk).

## Tests (`internal/update/redirect_test.go`)

| Test | Before | After |
|---|---|---|
| `TestCheckFollowsRedirectChain` (301, 302, 307, 308) | pass | pass |
| `TestCheckRefusesPlaintextRedirect` | **fail** (F1) | pass |
| `TestCheckStopsRedirectLoop` (bounded hops) | pass | pass |
| `TestResolveAssetCases` (cross-host ok, loop, https to http) | **fail** (loop followed) | pass |
| `TestDownloadFollowsCrossHostRedirect` (install still works) | pass | pass |
| `TestDownloadRedirectTargetChanged` (digest catches swap, file removed) | pass | pass |
| `TestDownloadStopsRedirectLoop` (<= 11 hops, < 5 s) | **fail** (50 hops, 20 s) | pass |

Before/after was measured by temporarily restoring the two old lines (`http.DefaultClient.Do`
in `Check`, no `resolveAsset` in `Download`) and running the same file; both runs are quoted
above. `TestCheckRefusesPlaintextRedirect` swaps `http.DefaultClient` as well as
`httpClient`, so the same test exercises the old path and the new one.

Hermetic: `TMPDIR` is set to `t.TempDir()` for the download tests, servers are `httptest`,
no real network, no real home.

Gates: `gofmt -l .` empty, `go build ./...`, `go vet ./...`, `go test -p 2 ./...`,
`GOOS=windows GOARCH=amd64|arm64 go build ./...`, `GOOS=linux go build ./...` all pass.

## Residual risk

- `engine.Download` still follows a redirect on the address `resolveAsset` returned. GitHub's
  CDN normally answers directly, so the second chain is usually empty, but a chain that
  starts at the resolved address is not checked. Fixing that means a `CheckRedirect` on
  `dlClient` in `internal/engine/download.go:25-32`, which is outside T08's paths.
- The probe is one extra request (headers plus one byte) per download. On a metered
  connection that is negligible, but it is a request the updater did not make before.
- The probe follows whatever `browser_download_url` says at download time, including to a
  host the check never saw. Content is still covered by size and digest; host pinning is
  open (below).
- A loop or an insecure hop now fails the download outright. There is no fallback to the
  original URL, which is the intent, but it means an over-eager proxy that rewrites a
  redirect to http blocks updates until it is fixed.

## Open questions

1. Should the asset be pinned to a host allowlist (`github.com`, `objects.githubusercontent.com`,
   `release-assets.githubusercontent.com`)? GitHub adds CDN hostnames over time, so a wrong
   list bricks updates; the digest covers content, so I left it open.
2. Should the check refuse a chain longer than, say, two hops for the API rather than ten? Ten
   is Go's default and no GitHub endpoint uses more than two today.
3. `internal/update/fakegithub_test.go` (a15) did not exist in this worktree when I wrote the
   tests, so the servers here are local helpers (`releaseList`, `hop`). They can move to the
   shared fixture once a15 lands.
