# T05/a09 report: CPU-only machines

Slice: Linux `/proc/cpuinfo`, `lscpu`, macOS CPU-only, and a container where
`/proc` is partial. Branch `agent/T05-a09`.

## What the scan does on a CPU-only machine

There is no CPU-text parser in the baseline. `internal/scan/scan.go:91-98`
takes `Cores` from `runtime.NumCPU()`, `PhysCores` from
`gopsutil/cpu.Counts`, and `CPUName` from `gopsutil/cpu.Info`. On Linux with
no NVIDIA card, `platformScan` (`internal/scan/scan_other.go:7`) tries
`nvidia-smi`, `rocm-smi` and the Windows display path, then sets
`Backend = CPU` and adds the note "No supported GPU found". On an Intel Mac,
`platformScan` (`internal/scan/scan_darwin.go:10-14`) sets `Backend = CPU` and
the note "Intel Mac detected" and returns before the `sysctl` block.

## Findings

1. **`internal/scan/scan.go:96-98` (baseline): CPU name and physical cores
   come only from gopsutil.** gopsutil reads `/proc/cpuinfo` and `/sys`; in a
   container with a cut-down `/proc` (no `physical id`, `core id` or
   `cpu cores`) or without `/sys` mounted, `cpu.Counts` can fail and
   `cpu.Info` can return nothing. `CPUName` then stays empty and `PhysCores`
   stays 0. Consequences: `cmd/ezlm/main.go:365` prints "CPU  (N cores)",
   `internal/app/launch.go:191` prints an empty name, and
   `internal/fit/fit.go:467` leaves `Threads` unset in CPU mode.
   Severity: minor (the plan still runs, llama.cpp picks its own thread count,
   but the user sees a nameless CPU). Fixed: new `internal/scan/cpuinfo.go`
   parses `/proc/cpuinfo` and falls back to `lscpu`; `Scan` calls it only on
   Linux and only when a field is still empty, so a gopsutil reading always
   wins. `lscpu` reads `/sys`, so it still answers when `/proc/cpuinfo` is
   missing.

2. **`internal/scan/scan_darwin.go:10-14` (baseline): the Intel Mac branch
   returns before any `sysctl` call.** The Apple Silicon branch reads
   `machdep.cpu.brand_string` itself (`scan_darwin.go:29`), the Intel branch
   relies on gopsutil having done it. If gopsutil's `cpu.Info` fails there,
   the brand string is lost and the CLI and launch note show an empty CPU.
   Severity: minor. Fixed: when `CPUName` is empty, the Intel branch asks
   `sysctl -n machdep.cpu.brand_string`; an existing name is kept.

3. **`internal/scan/scan.go:91`: `Cores` is `runtime.NumCPU()`, which ignores
   a container's CPU quota.** In Docker with `--cpus=2` on a 64-core host the
   scan reports 64 logical and (via gopsutil) 32 physical cores, so
   `internal/fit` sizes CPU-mode plans for the host, not the container.
   Severity: major for container users. **Not fixed** (out of this slice):
   needs a cgroup v2 `cpu.max` read, and a decision on whether to lower
   `PhysCores` as well. Open question below.

## Tests and fixtures

`internal/scan/cpuonly_test.go`:

- `TestParseProcCPUInfo`: N100 desktop (4 cores, no SMT), cut-down container
  `/proc` (2 vCPUs, no topology keys), Graviton2 (arm64, named from
  `CPU part: 0xd0c`), a two-socket VM with `cpu cores` but no `core id`, an
  old arm64 board named in `Hardware`, and empty/partial input.
- `TestParseLscpu`: x86 and arm64 fixtures, a two-socket line set, empty
  input.
- `TestFillCPUInfo`: fills empty fields, keeps a gopsutil reading, caps
  physical cores at logical, does nothing with no input.
- `TestLinuxCPUInfo`: `/proc/cpuinfo` only, `lscpu` only when the file is
  missing, neither (no panic, no invented values). Commands are faked with
  scripts in a `t.TempDir()` and a temp `PATH`.
- `TestCPUOnlyLinuxPlatformScan` (Linux only): no `nvidia-smi`, no
  `rocm-smi` gives `Backend = CPU`, no GPUs, and the CPU-only note.
- `TestIntelMacCPUOnly` (macOS only): `Arch = amd64` gives `Backend = CPU`,
  no GPU, `Unified = false`, the Intel Mac note, and the brand string from
  `sysctl` without overwriting an existing one.

Fixtures and provenance: `agents/T05/a09/SOURCES.md`. The fixtures are
synthetic, built from the documented formats and the vendors' published
topologies; no host was probed.

Evidence note: the baseline had no `/proc/cpuinfo`/`lscpu` parser at all, so
the gap is evidenced by the code paths above rather than by a test that fails
on the baseline. One real parser bug was found while writing the tests: the
two-socket case recorded `physical id` only after the `core id` check, so it
counted 8 cores instead of 16. `TestParseProcCPUInfo` failed with
`got 8, want 16` and passes after the one-line move in `cpuinfo.go`.

## Checks run

```
gofmt -l internal/scan/        # empty
go build ./...
go vet ./internal/scan/
go test -p 2 ./internal/scan/   # ok
```

The Linux-only test skips on this macOS machine; it compiles on both
platforms.

## Open questions

- Should the scan lower `Cores`/`PhysCores` to a container's cgroup quota?
  That changes plan sizing, so it needs a decision beyond this slice.
- `armCPUParts` maps only the four Neoverse parts cloud machines use. Other
  arm64 CPUs (Ampere Altra 0xd0c is covered, Apple under Linux is not) keep
  an empty name unless their `/proc` has a `model name`.
- Is running `lscpu` acceptable when gopsutil fails? It is Linux-only and
  only on the failure path, so it costs one exec on an unusual machine.
