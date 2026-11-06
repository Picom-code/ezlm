# T05 a15: bug hunt in `internal/scan`

Scope: read every parser in `internal/scan` (this snapshot: `gpu_discrete.go`,
`display_windows.go`, `apps.go`, `apps_windows.go`, `scan.go`, `scan_darwin.go`,
`quick.go`), looking for the failure modes a fixture corpus misses: index out of
range, silent zero on a parse error, unit confusion (MB/MiB/GB), unsigned
underflow, 32-bit truncation, wrong regex anchoring.

Test file: `internal/scan/parser_bugs_test.go`. One test per bug; each one was
run before its fix and failed (evidence below). External tools are replaced with
a shell script on `PATH` in a temp dir, so no host GPU or tool is needed.

## Fixed

1. **Unsigned underflow in `amdScan` free memory** (major)
   `internal/scan/gpu_discrete.go:159` (before the fix):
   `MemFree: total - used`. When `rocm-smi` reports more VRAM in use than the
   card's total (an APU that counts shared memory, or a stale total), the
   subtraction wraps to about 18 EB. That value flows into `Hardware.GPUUsable`
   and the planner then believes any model fits.
   Test: `TestAMDScanClampsFreeMemory`. Before: `free memory 18446744073172680704
   is more than the card's 2147483648: the subtraction wrapped`. Fix: clamp
   `used` to `total`.

2. **`amdScan` returned cards in map order** (minor)
   `internal/scan/gpu_discrete.go:164`: `for _, c := range cards` over a map is
   randomized, so `GPUs[0]` was a random card and the order changed between
   scans (dashboard flicker; `displayScan` and callers that take `GPUs[0]` get a
   random card). nvidia-smi and the Windows display scan both return the
   biggest card first.
   Test: `TestAMDScanSortsCardsBySize` (repeats the parse, because map order is
   random). Before: `cards out of order: AMD Radeon RX 6600 before AMD Radeon
   RX 7900 XTX`. Fix: `sort.Slice` by `MemTotal` descending.

3. **`parseGPUUsage` counted PID 0** (minor)
   `internal/scan/gpu_discrete.go:135`: the Windows GPU process counters
   include `pid_0_...` (System Idle). It was summed into the usage map and then
   listed as an app holding graphics memory; it can never be closed, so it is
   noise in Model mode.
   Test: `TestParseGPUUsageSkipsPIDZero`. Before: `PID 0 was counted as a
   process holding graphics memory`. Fix: require `pid > 0`.

4. **`friendlyName` split a non-ASCII name mid-rune** (minor)
   `internal/scan/apps.go:353` (before the fix):
   `strings.ToUpper(b[:1]) + b[1:]`. With a program whose name starts outside
   ASCII, `b[:1]` is half a rune and `strings.ToUpper` turns it into U+FFFD, so
   the app list showed `�nïcode`. The same pattern was in `osName`
   (`internal/scan/scan.go:192`) for a distro name.
   Test: `TestFriendlyNameKeepsNonASCII`. Before: `friendlyName("/Applications/
   Ünïcode/Ünïcode") = "�nïcode"`. Fix: a `upperFirst` helper that decodes the
   first rune, used by both call sites.

5. **Mobile Intel Arc cards read as built-in graphics** (minor)
   `internal/scan/gpu_discrete.go` `discreteRe`: the card pattern ended in
   `[ab]\d{2,3}\b`, so "Arc(TM) A770M" did not match (the `M` is a word
   character). A 16 GB A770M then took the built-in branch, which gives it no
   memory bandwidth, so the planner under-rated the card.
   Test: `TestUsableAdaptersMobileArc`. Before: `A770M read as built-in
   graphics (bandwidth 0), want the discrete card's 560`. Fix: allow the mobile
   suffix, `[ab]\d{2,3}m?\b`. "Arc(TM) 140V" is still built-in.

## Found, not fixed (skipped test)

6. **A MIG-enabled NVIDIA card is dropped entirely** (major)
   `internal/scan/gpu_discrete.go:24-28`: with MIG on, `nvidia-smi
   --query-gpu=memory.total,memory.free` returns `[N/A]` for the whole card.
   Both `ParseUint` calls fail, the row is skipped, `len(h.GPUs) == 0` makes
   `nvidiaScan` return false, and a machine with an A100 falls back to the CPU
   backend. `TestNvidiaScanMIGCardIsNotDropped` reproduces it (run unskipped
   before committing: `nvidia-smi reported a card but the scan found none`) and
   is left `t.Skip("bug: ...")` because the fix needs a decision on what a
   MIG-enabled card's memory is (sum of MIG instances, or the driver's total),
   which is a01's corpus group.

## Checked and believed correct (no test)

- `nvidia-smi` units: `--format=csv,noheader,nounits` gives MiB and every
  value is shifted by 20; the same for `used_memory` of compute apps and for
  `QuickMemory`. No MB/MiB mixing found.
- `readAdapter` (`display_windows.go`): the 4-byte `HardwareInformation.
  MemorySize` fallback is a deliberate 4 GB clamp for old drivers. **Open
  question:** on drivers that store `HardwareInformation.qwMemorySize` as
  REG_BINARY (8 bytes) rather than REG_QWORD, `GetIntegerValue` fails and the
  code falls through to the 4-byte value, so a 24 GB card is read as 4 GB.
  Windows-only, cannot be executed here; a05 owns that value. Worth a fixture.
- `cudaVersionRe`: matches both header forms ("CUDA Version:" and "CUDA UMD
  Version:") and takes the first match; `[0-9.]+` would keep a trailing dot,
  but no driver prints one.
- `splitCSV`: naive comma split, but no shipping GPU name contains a comma
  (MIG device names use spaces).
- `appleBandwidth`: substring generation match would read a future "M10" as
  "M1". Speculative, not worth a fix tonight.
- `windowsGPUMem` / `displayScan` memory accounting: the per-adapter split of
  used memory is an approximation (documented in the code), not a parser bug.
- `topMem`, `FreeAt`, `roomiestDrive`, `installRoot`, `closableProc`: no
  out-of-range indexing; `FreeAt` terminates at the filesystem root.

## What changed

- `internal/scan/parser_bugs_test.go` (new).
- `internal/scan/gpu_discrete.go`: clamp + sort in `amdScan`; `pid > 0` in
  `parseGPUUsage`; mobile `m?` in `discreteRe`.
- `internal/scan/apps.go`: `upperFirst` helper, used by `friendlyName`.
- `internal/scan/scan.go`: `osName` uses `upperFirst`.

## Verification

```
gofmt -l .                                  # empty
go vet ./internal/scan/                     # ok
go build ./...                              # ok
GOOS=windows GOARCH=amd64 go build ./...    # ok
GOOS=linux go build ./...                   # ok
go test -p 2 ./internal/scan/               # ok (0.65s)
```

## Open questions

- Should a MIG-enabled card report the sum of its MIG instances or the driver's
  total? (a01)
- Is the REG_BINARY `qwMemorySize` case real on current AMD/Intel drivers? If
  so, `readAdapter` needs the 8-byte binary read before the 4-byte fallback.
  (a05)
- `appleBandwidth` and `otherBandwidth` are lookup tables matched by substring;
  a wrong entry is invisible to tests unless a fixture asserts a number. a14's
  inventory should list them as data, not logic.
