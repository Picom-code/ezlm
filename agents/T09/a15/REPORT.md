# T09 a15: memory and CPU for a 20 GB download

Stretch task: measure what a full-speed 20 GB model download costs in memory and
CPU, and prove that neither grows with the file size. No fault was found in
`download.go`, so this slice adds a test and numbers, not a fix.

## What was measured

`TestDownload20GBMemoryAndCPU` in `internal/engine/download_bench_test.go`
downloads 20,000,000,000 bytes from an in-process `httptest` server that writes
as fast as loopback and the disk allow, with a `Content-Length` so the client
knows the total. The client is the real `Download` with `want` set, so the
exact-size check and the rename path are exercised too.

The test checks that the file has the right size, spot-checks three 1 MiB blocks
against what the server sent, and that progress was reported monotonically and
ended at 100%. It then asserts:

- heap after a GC grew by less than 64 MiB;
- peak RSS grew by less than 256 MiB;
- total bytes allocated during the transfer stayed under a quarter of the file.

`BenchmarkDownload20GB` runs the same measurement for repeat runs:

```sh
go test -p 1 -run '^$' -bench BenchmarkDownload20GB -benchtime=1x ./internal/engine/
```

The test is `-short`-guarded because it writes 20 GB (about 27 s here), so
`go test -short ./internal/engine/` stays cheap. A run needs roughly 20 GB of
free disk in the temp directory.

## Numbers

Measured on this Mac (Apple silicon, 11 cores, macOS, Go 1.27.3), worktree
`agent/T09-a15`, commit with this test, while other T09 agents were running:

| Metric | Result |
|---|---|
| File size | 20.0 GB (20,000,000,000 bytes) |
| Wall time | 26.5 s |
| Throughput | 756 MB/s |
| CPU (user+sys, whole process) | 6.83 s, 26% of one core |
| Heap after GC, before | 1.4 MiB |
| Heap after GC, after | 1.4 MiB |
| Peak RSS, before | 14.2 MiB |
| Peak RSS, after | 15.9 MiB |
| Bytes allocated during the transfer | 1.1 MiB |
| Progress callbacks | 102, every ~200 ms, monotonic, final `done == total` |

A second run through the benchmark, minutes later, was faster (the machine is
shared):

| Metric | Result |
|---|---|
| Wall time | 19.1 s |
| Throughput | 1045 MB/s |
| CPU | 6.81 s |
| Peak RSS growth | 1.9 MiB |
| Allocations | 366 allocs, 3.7 MB total |

Throughput varies with the other agents on this Mac; CPU, memory and
allocation stay flat across runs. The 3.7 MB in the benchmark includes the
benchmark harness (temp dir, stats, per-iteration setup); the plain test
measured 1.1 MiB. Both are constant and unrelated to the 20 GB size.

CPU covers both ends of the transfer: the fake server runs in the same process
as the client, so the client alone is cheaper than 6.83 s. Even so, the number
is far from a problem: 20 GB costs well under one core-second per GB.

## Reading

- Memory does not scale with the file. The only per-transfer buffer is the
  1 MiB read buffer in `fetch` (`internal/engine/download.go:164`); the file
  itself never sits in memory. Peak RSS grew 1.7 MiB over a 20 GB download and
  the heap was unchanged after a GC, which is consistent with a constant
  working set.
- Allocation does not scale with the file. 1.1 MiB was allocated in total for
  20 GB, i.e. about the one 1 MiB buffer plus small fixed overhead. The read
  loop reuses the buffer and writes it straight to the file, so there is no
  per-chunk garbage for the GC to collect.
- CPU is proportional to bytes but cheap: ~0.34 s per GB for the whole
  process (client and server). No busy polling is involved; the only timer is
  the stall watchdog, which ticks every `stallTimeout/9` (5 s at the default
  45 s) and does nothing on a healthy transfer.
- Progress is fine for a 20 GB file: 102 callbacks at roughly the intended
  200 ms cadence, totals known from `Content-Length`, and the final callback
  reports 100%.

## Before/after

There was no bug to reproduce. The code before this slice already had the flat
profile; the new test and the table above are the evidence. Before the test,
the claim "memory stays flat on multi-GB downloads" was unproven.

## Fix

None in `download.go`. Files added:

- `internal/engine/download_bench_test.go`: the 20 GB test and benchmark.
- `internal/engine/download_bench_unix_test.go`: peak RSS and process CPU from
  `getrusage` (`//go:build unix`; macOS reports bytes, Linux kilobytes).
- `internal/engine/download_bench_other_test.go`: zero-value fallbacks so the
  test files still compile on Windows.

The CPU metrics in `runtime/metrics` (`/cpu/classes/user:cpu-seconds`) were
tried first and read zero on this Go/macOS build, hence `getrusage`.

## Residual risk

- The fake server is in-process, so throughput (756 MB/s) and CPU include the
  server and the loopback stack. They are an upper bound for the client, not a
  prediction of a real CDN download, which is limited by the network.
- Peak RSS only grows over a process lifetime (`ru_maxrss`). When the full
  suite runs first, the "before" value already includes earlier tests; the
  delta is then a lower bound, never an overstatement.
- The 20 GB test runs in the normal (non-`-short`) suite and takes ~27 s. The
  brief asked for `-short`-guarding; if the suite must stay fast, run
  `go test -short` or move it behind a build tag.
- The benchmark writes 20 GB per iteration; use `-benchtime=1x`.

## Open questions

- Should the client read in 1 MiB chunks, or is 256 KiB better for the stall
  watchdog on slow links? Not measurable here: at 756 MB/s the watchdog sees
  data every ~1.4 ms, so a 1 MiB buffer is nowhere near the stall threshold.
- `progress` is reported from the download goroutine; a slow UI callback would
  throttle the transfer. `internal/app` is out of scope here (a13).
