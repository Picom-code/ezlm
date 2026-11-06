# T11 a13: flakiness review of `internal/engine`

Slice: stretch. Run the existing `internal/engine` tests with `-count=20 -race`,
record flakes with their seed and output, and fix only what can be fixed inside
tests. No production code was changed.

Worktree: `~/ezlm-agents/work/subs/T11/a13` (branch `agent/T11-a13`).

## What I ran

```
go test -p 2 -race -count=20 ./internal/engine
```

Go 1.27.3, darwin/arm64. Baseline was `5213370` (clean tree).

The package has six tests: `TestFlavor`, `TestPickAssets`,
`TestPinnedReleaseCoversPlatforms` (pure), `TestDownloadNetErrorsArePlain`,
`TestDownloadFlakyConnectionFinishes`, `TestDownloadResumesAfterStall`,
`TestDownloadResumesAfterStallHTTP2`, `TestDownloadStatusHandling`, and
`TestLiveDownload` (skipped unless `EZLM_LIVE_URL` is set).

## Flake found: data race on the `stallTimeout` package global

Frequency: 2 race failures in the 20 iterations, both in the stall tests.
Raw log: `/tmp/a13_engine_race20.log` (not committed; stacks quoted below).
No `-shuffle` was used for the baseline run, so the order was source order
(`download_neterr_test.go`, then `download_stall_test.go`, ...).

```
WARNING: DATA RACE
Write at 0x000104bd4798 by goroutine 154:
  .../internal/engine.TestDownloadResumesAfterStall()
      .../internal/engine/download_stall_test.go:21
Previous read at 0x000104bd4798 by goroutine 148:
  .../internal/engine.fetch.func1()
      .../internal/engine/download.go:109
Goroutine 148 (finished) created at:
  .../internal/engine.fetch()
      .../internal/engine/download.go:108
  .../internal/engine.Download()
      .../internal/engine/download.go:58
  .../internal/engine.TestDownloadFlakyConnectionFinishes()
      .../internal/engine/download_neterr_test.go:80
--- FAIL: TestDownloadResumesAfterStall (2.31s)
    testing.go:1867: race detected during execution of test
```

The second report is the same variable written by
`TestDownloadResumesAfterStallHTTP2` (`download_stall_test.go:69` at the
time) against a watchdog goroutine started by the previous iteration's
`TestDownloadResumesAfterStall` (`download_stall_test.go:105`):

```
WARNING: DATA RACE
Write at 0x000104bd4798 by goroutine 168:
  .../internal/engine.TestDownloadResumesAfterStallHTTP2.func1()
      .../internal/engine/download_stall_test.go:69
Previous read at 0x000104bd4798 by goroutine 178:
  .../internal/engine.fetch.func1()
      .../internal/engine/download.go:109
--- FAIL: TestDownloadResumesAfterStallHTTP2 (2.34s)
    testing.go:1867: race detected during execution of test
```

### Root cause

`stallTimeout` (`internal/engine/download.go:21`) is a package global.
`fetch` starts a watchdog goroutine that reads it (`download.go:109`,
`download.go:116`). Both stall tests did:

```go
old := stallTimeout
stallTimeout = 300 * time.Millisecond
defer func() { stallTimeout = old }()
```

The write and the restore are unsynchronised with the watchdog goroutine of
a download started earlier in the same test binary (the goroutine has already
read the variable, so there is no happens-before edge). The race detector
correctly flags it between test runs; with `-count=1` it is timing dependent
and usually missed. This is a test bug: production code never writes
`stallTimeout` after init.

### Fix (test only)

`internal/engine/download_stall_test.go` now sets the short timeout once in
`TestMain`, before any test or download starts, and the two stall tests no
longer assign or restore it:

```go
func TestMain(m *testing.M) {
	stallTimeout = 300 * time.Millisecond
	os.Exit(m.Run())
}
```

That makes every read of `stallTimeout` ordered after the single write, and
keeps the 300 ms stall window the tests were written for. No production file
was touched.

## Verification after the fix

```
gofmt -l internal/engine     # empty
go vet ./internal/engine      # clean
go test -p 2 -race -count=20 -shuffle=on ./internal/engine
```

Result: `ok ... 148.906s`, exit 0, zero `DATA RACE` reports (the baseline
had two). A verbose shuffled run was used to record a seed and show the test
list:

```
-test.shuffle 1793981251028295000
--- PASS: TestDownloadResumesAfterStall (2.34s)
--- PASS: TestPinnedReleaseCoversPlatforms (0.00s)
--- PASS: TestDownloadFlakyConnectionFinishes (0.04s)
--- PASS: TestDownloadResumesAfterStallHTTP2 (2.35s)
--- PASS: TestDownloadNetErrorsArePlain (0.12s)
--- PASS: TestDownloadStatusHandling (2.50s)
--- SKIP: TestLiveDownload (0.00s)
--- PASS: TestFlavor (0.00s)
--- PASS: TestPickAssets (0.00s)
ok  	github.com/picom-code/ezlm/internal/engine	8.598s
```

To reproduce:

```
go test -p 2 -race -count=20 -shuffle=1793981251028295000 ./internal/engine
```

## Other observations (no change made)

- `retryStep` (`download.go:37`) is mutated the same way by `fastRetries`
  (`download_neterr_test.go:18-22`), but only the `Download` caller goroutine
  reads it (`download.go:84`), so no race was observed. If a future test
  starts `Download` in a goroutine while another test restores `retryStep`,
  it will have the same class of race. A `TestMain`-style set would remove
  the pattern; left alone to keep the diff minimal.
- `TestDownloadResumesAfterStallHTTP2` mutates the shared
  `dlClient.Transport.(*http.Transport).TLSClientConfig`
  (`download_stall_test.go`), then restores it. Tests run sequentially, so no
  race was observed in 20 iterations, but it is the same "global mutated from
  a test" pattern. A future parallel test would need its own client or a
  serialising lock.
- The stall tests assume the resumed request happens within 8 s / 15 s. The
  retry backoff after a stall is `fails*2*retryStep` (2 s with the default
  `retryStep`), which is the bulk of their ~2.3 s runtime; the margins are
  fine but they are wall-clock tests on a shared machine.
- `TestDownloadStatusHandling` uses the default 1 s `retryStep` and a 2.5 s
  context, expecting at least two 503 requests. It passed in every iteration,
  but it is the most timing-sensitive test in the package.

## Could not finish / next steps

- No non-race failures were seen in the baseline 20 iterations, so there is
  no other flake to record.
- If the package grows tests that run downloads in parallel, the `retryStep`
  and `dlClient.Transport` mutations above should move into `TestMain` or
  behind a helper too.
