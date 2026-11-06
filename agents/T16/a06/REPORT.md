# T16 a06: `iogpu.wired_limit_mb` and memory-pressure reading

## What I reviewed

- `internal/scan/scan_darwin.go` (the only place that reads `memory_pressure` and
  `iogpu.wired_limit_mb`), `internal/scan/quick.go` (the 2-second `/api/live` path),
  `internal/scan/scan.go`, `internal/fit/fit.go` (how `RAMAvail`/`GPUUsable` become a
  model plan), and the Mac app's consumers of the same numbers
  (`macos/Sources/EZLM/Models.swift:385`, `Wizard.swift:265-270`,
  `Screens.swift:175-183`, `AppModel.swift:308`).
- Apple's own definition of the number `memory_pressure` prints, from the xnu source:
  `memorystatus_level = available_memory * 100 / total_pages`
  (`osfmk/vm/vm_pageout.c:4738`), where `available_memory` is
  `AVAILABLE_NON_COMPRESSED_MEMORY = active + inactive + free + speculative`
  (`osfmk/vm/vm_page.h:1528`).

Measured on this Mac (M-series, 18 GiB, macOS 26.3.1):

```
sysctl -n iogpu.wired_limit_mb        -> 0            (unset: default rule applies)
memory_pressure | tail -1              -> System-wide memory free percentage: 50%
sysctl kern.memorystatus_vm_pressure_level -> 2       (warn)
gopsutil mem.VirtualMemory().Available -> 4841734144  (4.84 GB, 25%)
```

So at `warn` pressure the kernel calls 50% of RAM "free" (9.0 GB) while only
4.84 GB is inactive or free. The difference is the active working set of the
open apps: the kernel counts it as available because it can compress or swap it.

## Findings

### 1. Major: the pressure reading overstates available RAM by up to the active set

`internal/scan/scan_darwin.go:18-27` (before the fix) set

```go
h.RAMAvail = max(h.RAMAvail, uint64(float64(h.RAMTotal)*pct/100))
```

`pct` is the kernel's *not wired and not compressed* share, which includes the
active pages of every open app (xnu above). `gopsutil`'s available figure is
`inactive + free` only, which is always a subset of that, so `max()` always
picked the inflated number. Evidence: the numbers above, from this machine.
The estimate is not "free RAM"; it is "RAM the kernel could take back by
paging out the user's apps".

Consequences on an 8 GB Mac: at the same 50% warn level the scan reports
4.0 GB available, while `inactive + free` may be ~1.5 to 2 GB. The planner
(`fit.ComputeBudgets`, 10% margin, 1 GB OS floor on 8 GB) then picks a model
that must swap out whatever the user has open to load, which is the swap-thrash
case on the smallest machines.

Fix: `pressureAvailable()` subtracts the `Pages active` count that
`memory_pressure` also prints (using the page size from its own header) before
raising `RAMAvail`, so the value is the memory that can be reused without
paging. `max` with gopsutil is kept, because `inactive + free` is a subset.
Tests: `TestPressureAvailable`, `TestPressureAvailableWithoutPageCounts`.

### 2. Major (not fixed here, outside my owned paths): `/api/live` ignores the wired limit

`internal/scan/quick.go:29-41` reads `memory_pressure` (same overstatement as
finding 1) and sets `m.GPUTotal, m.GPUFree = m.RAMTotal, m.RAMAvail` on Apple
Silicon, without reading `iogpu.wired_limit_mb` and without the 67%/75% default.
The full scan (`scan_darwin.go`) reports the real wired limit as
`GPUUsable`. The Mac app polls `/api/live` every ~2 seconds
(`AppModel.swift:681,741`) and drives the memory-fill animation from it, so on
an 8 GB Mac the app shows 8 GB of GPU memory where only about 5.3 GB can be
wired, and on a Mac where the user lowered `iogpu.wired_limit_mb` it overstates
by much more. The percentage parse is also duplicated between the two files.

Not changed because `quick.go` is not a `*_darwin.go` file and T16's owned
paths are `macos/**` (non-frozen) and `*_darwin.go`; a patch there may be
dropped at merge. Next step: move `pressureAvailable`, `parseWiredLimitMB` and
`defaultGPUUsable` into a shared helper and use them from `QuickMemory`.

### 3. Minor: a user-set wired limit could exceed physical RAM

`sysctl -w iogpu.wired_limit_mb=65536` on an 8 GB Mac (a common "give the GPU
everything" tweak; the driver clamps it internally) made the scan report a GPU
with 64 GB of memory and `GPUUsable` above `RAMTotal`. Fixed by clamping to
`RAMTotal` in `platformScan`; `fit` already caps a unified budget at
`availRAM`, so the plan was safe, but the app's GPU numbers were not.
Test: `TestGPUUsableEightGB`.

### 4. Correct, and worth keeping

- The 67% / 75%-above-36-GB fallback matches Apple's default wired limits
  (8 GB -> 5.3 GB, 16 GB -> 10.6 GB, 48 GB -> 36 GB). This is what 8 GB Macs
  get, because `iogpu.wired_limit_mb` is `0` (unset) on a stock machine.
  Covered by `TestDefaultGPUUsable`.
- `iogpu.wired_limit_mb` is in MB and was converted correctly (`mb << 20`);
  `0` is correctly treated as "unset". The parsing is now explicit and tested
  (`TestParseWiredLimitMB`).
- Intel Macs return before the pressure and wired-limit code, which is right:
  they run on the CPU and `gopsutil` available is enough.

## What I could not finish

- No Swift change was needed for this slice, but I did not add an app-side
  "memory pressure is warning" hint: the scan reports `RAMAvail` but not
  `kern.memorystatus_vm_pressure_level`, and the app (frozen `AppModel.swift`)
  shows no pressure warning while planning a load. That belongs with the 8 GB
  work (a07) or the app model owner.
- `QuickMemory` still overstates `RAMAvail` and `GPUTotal` (finding 2).
- Everything was measured on this 18 GiB machine; I did not have an 8 GB Mac to
  run the full scan on.

## Open questions

- Should a unified budget count compressed pages as available? They can be
  swapped out, but only with disk I/O. This change is deliberately
  conservative: active pages are excluded, compressed pages are not counted as
  free (they were never counted by `gopsutil` either).
- Apple does not document the 36 GB boundary. It matches the tables for
  current chips; if a future chip changes it, `defaultGPUUsable` is the one
  place to update.

## Verified on this Mac

`gofmt -l .` empty; `go build ./...`, `go vet ./...`, `go test -p 2 ./...`,
`GOOS=windows GOARCH=amd64|arm64 go build ./...`, `GOOS=linux go build ./...`
all pass. `cd macos && swift build && swift test` passes (22 tests, 0 failures).
