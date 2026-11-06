# T08 / a01: rate limits

**Fault class:** GitHub rate limits on the update check: 403 with
`X-RateLimit-Remaining: 0`, 429 with `Retry-After`, and the secondary rate
limit (403 with `Retry-After`, no `X-RateLimit-Remaining: 0`).

**Verdict:** reproduced, real, fixed. Severity: major (it does not brick an
install, but a rate-limited user is stuck in an error state and every UI tick
asks GitHub again, which extends the limit).

## What the updater did

`Check` (`internal/update/update.go:56`) sent one request and turned any
non-200 into a generic message:

```go
if resp.StatusCode != 200 {
    info.Error = fmt.Sprintf("checking for updates failed (%s)", resp.Status)
    return info
}
```

No header was read, no retry, no memory of the limit. The app layer makes it
worse: `internal/app/settings.go:36` bypasses the one-hour cache whenever
`updCache.Error != ""`:

```go
if force || updCache.Checked.IsZero() || time.Since(updCache.Checked) > time.Hour || updCache.Error != "" {
```

So during a rate-limit window every `UpdateInfo` call (settings pane open,
`DownloadUpdate`, the Windows/macOS launch check) sent another request to
GitHub. The mac app calls `checkUpdate` from `AppModel.swift:708` and from
the settings view's `.task` (`Settings.swift:289`); the Windows app calls
`CheckUpdate` from `Shell.cs:592` and the button in `SettingsPane.cs:69`.
None of them retries in a tight loop, but each one costs a request against a
limit that is already exhausted, and the UI shows a raw HTTP status.

The asset path has its own rate-limit gap, outside the paths T08 owns:
`internal/engine/download.go:243` treats 403 as permanent (no retry) and
retries 429 with a 2/4/6/8 second backoff that ignores `Retry-After`
(`download.go:66-86`), so an asset behind a 60-second limit fails after
about 20 seconds with "the download server is having trouble". It cannot
install anything wrong (the digest check runs after the fetch), so this is
minor.

## Reproduction (failing before the fix)

`internal/update/ratelimit_test.go`, all against `httptest`, with
`EZLM_UPDATE_URL` pointed at the server and temp-only state. Before the fix:

```
--- FAIL: TestCheckPrimaryRateLimitBacksOff (0.00s)
    ratelimit_test.go:72: updater asked GitHub 5 times during one rate-limit window, want 1
--- FAIL: TestCheckRetryAfterSeconds (0.00s)
    ratelimit_test.go:98: updater asked GitHub 5 times after Retry-After: 90, want 1
--- FAIL: TestCheckSecondaryRateLimit (0.00s)
    ratelimit_test.go:127: updater asked GitHub 4 times during a secondary rate limit, want 1
--- FAIL: TestCheckRetriesShortRateLimit (0.00s)
    ratelimit_test.go:154: a transient 429 errored the check: "checking for updates failed (429 Too Many Requests)"
```

`TestCheckRateLimitRecovers` and `TestCheckPlainForbiddenIsNotARateLimit`
are guards rather than before/after cases: they passed trivially before the
fix (nothing was remembered) and now prove the window expires and that a
non-rate-limit 403 does not start one.

## Fix

All in `internal/update/update.go`, additive:

- `retryDelay` reads the response: 429 always counts; 403 counts only with
  `X-RateLimit-Remaining: 0` or a `Retry-After` header (the secondary
  limit). The wait comes from `Retry-After` (seconds or HTTP date), else
  `X-RateLimit-Reset`, else one minute, capped at one hour.
- A short wait (`Retry-After: 0`, up to 3 seconds) is retried once inside
  the same check, so a transient limit does not show up as an error in the
  app.
- A longer wait is remembered in `rateUntil` (mutex-guarded) and every
  `Check` inside the window returns a plain message without touching the
  network: "GitHub is limiting update checks, so EZLM will try again in 2
  minutes". That is what the app shows, so the app keeps working and the
  request count stops.
- `clearRateLimit` exists for tests; the test file resets the window at the
  start and in `t.Cleanup`, so the other test files in this package are not
  affected by ordering.

After:

```
ok  github.com/picom-code/ezlm/internal/update
```

`gofmt -l .` empty, `go build ./...`, `go vet ./...`, and
`GOOS=windows GOARCH=amd64 go build ./...` all pass.

## Residual risk

- The window is per process. Restarting EZLM forgets it, and a fresh
  process asks GitHub once more. One request is not a spin; a shared,
  persisted window would be a larger change.
- While rate-limited, `Info.Latest` is nil, so a release found by the
  previous check disappears from the banner until the window ends. Caching
  the last good `Info` in this package would keep the banner; that changes
  what the app shows and was left for the Lead to decide.
- The asset download's 429 handling is in `internal/engine`, not owned here.
  Worth a small follow-up: honour `Retry-After` in `engine.Download` and
  treat a 403 with rate-limit headers as retryable rather than permanent.

## Open questions

1. Should a rate-limited check return the previous good `Info` (with the
   calm message) instead of dropping `Latest`?
2. Is one in-check retry the right amount, or should the check retry until
   the 10-second context ends when `Retry-After` is small? One retry keeps
   the check fast; more retries would be more forgiving.
