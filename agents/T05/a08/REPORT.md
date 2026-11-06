# T05 a08: macOS `system_profiler` corpus

Slice: `system_profiler SPDisplaysDataType` and `SPHardwareDataType` on
Apple Silicon and Intel, including an external GPU and a headless Mac.

## What I did

- Added `internal/scan/system_profiler.go`: a text parser for both data
  types (`parseProfilerDisplays`, `parseProfilerHardware`, `profilerBytes`,
  `profilerVendor`, `profilerCoreCounts`) and `metalGPUs`, which picks the
  discrete or external card an Intel Mac can run the Metal build on.
- Wired it into `internal/scan/scan_darwin.go`:
  - Intel Macs (`scan_darwin.go:72` `intelScan`) now read
    `system_profiler SPDisplaysDataType`, and use a Metal-capable PCIe card
    with 2 GB or more of its own VRAM as the GPU. Built-in Intel graphics
    still leave the machine on the CPU build, as before.
  - Apple Silicon uses `SPHardwareDataType` only as a fallback for the chip
    name and RAM size when `sysctl`/the memory API returned nothing
    (`scan_darwin.go:16-26`).
- Added `internal/scan/system_profiler_test.go` and seven fixtures under
  `internal/scan/testdata/` (sources: `SOURCES.md`).

## Parsers covered

| Parser | Input | Test |
|---|---|---|
| `parseProfilerDisplays` | `system_profiler SPDisplaysDataType` text | `TestProfilerDisplaysAppleSilicon`, `...IntelEGPUHeadless`, `...Headless`, `...IntelBuiltInOnly`, `...LegacyMetal`, `...UnusableCards`, `...Empty` |
| `parseProfilerHardware` | `system_profiler SPHardwareDataType` text | `TestProfilerHardware` |
| `profilerCoreCounts` | "11 (5 Performance and 6 Efficiency)" | `TestProfilerCoreCounts` |
| `profilerBytes` | "1536 MB", "8 GB", "1.5 GB" | `TestProfilerBytes` |
| live output | this Mac's `SPDisplaysDataType` and `SPHardwareDataType` | `TestReadsThisMacsProfiler` (read-only, macOS only) |
| `metalGPUs` | parsed cards | the display tests above |

## Findings

**F1 (major, fixed): Intel Macs were always CPU-only, even with a discrete
or external GPU.** Baseline `scan_darwin.go:10-14` returned as soon as
`Arch != "arm64"`, set `Backend = CPU` and never looked at the graphics
hardware, so a MacBook Pro with a Radeon Pro 5500M or an eGPU in a Razer
Core X was planned as a processor-only machine. The scanner had no
`system_profiler` parser at all.

Evidence: the baseline early return above; the new test
`TestProfilerDisplaysIntelEGPUHeadless` parses a three-card Intel/eGPU dump,
asserts the built-in UHD 630 is skipped and the RX 580 (8 GB) and Radeon Pro
5500M (4 GB) are returned, biggest first. `TestProfilerDisplaysIntelBuiltInOnly`
keeps the old behaviour for Macs with only Iris/UHD graphics.

Caveat on "failing before": the parser did not exist, so the test could not
compile against the baseline. This is a missing-detection gap proved by the
baseline code, not a misparse of an existing parser.

**F2 (minor, open): the amd64 slice of the Mac app on Apple Silicon is
detected as an Intel Mac.** `scan_darwin.go:10` decides by `h.Arch`
(`runtime.GOARCH`), and `macos/scripts/build-app.sh:29-30` builds a
universal binary. Under Rosetta the amd64 engine takes the Intel path;
`SPDisplaysDataType` reports an Apple GPU, which `metalGPUs` excludes, so the
scan reports `Backend: CPU` with the note "Intel Mac detected". Native arm64
launches are unaffected. Not fixed here: it needs a decision on whether an
x86_64 llama.cpp build on Apple Silicon should use Metal, and the fix belongs
with backend selection, not this parser.

**F3 (minor, fixed by coverage): the `SPHardwareDataType` core wording
changed case in macOS 26.** Captured output says "11 (5 Performance and 6
Efficiency)"; older macOS says "performance"/"efficiency" in lower case.
`profilerCoreCounts` matches case-insensitively; `TestProfilerCoreCounts`
covers both spellings.

**F4 (observation): `system_profiler` prints diagnostics on stderr**
(`hw.cpufamily: 0x...`, plus a timestamp) on macOS 26. `run` uses
`cmd.Output()`, which keeps only stdout, so the parser is unaffected.

**F5 (observation): an Intel Mac with only built-in Intel graphics still
reports CPU.** Deliberate: on those Macs the iGPU is about as fast as the
processor. The note text is unchanged.

No bugs were found in the shared parsers (`gpu_discrete.go`, `scan.go`) while
building this corpus.

## Behaviour details

- Apple Silicon: the parser returns one Apple GPU with no VRAM of its own
  (`MemTotal == 0`); `metalGPUs` ignores it because the existing
  unified-memory path already synthesises the GPU from the chip and RAM.
- Intel with a discrete/eGPU card: `MemTotal` and `MemFree` are the card's
  own VRAM. There is no per-process GPU memory source for AMD on macOS, so
  free VRAM is not reduced by other apps; the planner's 4% margin covers
  the desktop.
- Headless: no `Displays:` section is required; cards are found from the
  card blocks alone.
- Legacy macOS 10.14 to 11 output uses `Metal: Supported, feature set
  macOS GPUFamily2 v1` and MB-sized VRAM; both parse.
- Cards without Metal support, or with under 2 GB of VRAM, are not used.
  That keeps NVIDIA Pascal eGPUs (no Metal on macOS) on the CPU build.

## What I could not finish

- No Intel Mac or eGPU was available on this host, so the Intel and legacy
  fixtures are synthetic from the documented format (see `SOURCES.md`). The
  Apple Silicon fixtures are real captures from macOS 26.3.1.
- `intelScan` itself was not exercised against a real Intel Mac; the test
  covers the parse and selection functions it calls. `TestReadsThisMacsProfiler`
  does run both parsers against this Mac's live output (Apple M3 Pro, 18 GB).
- The `-json` form of `system_profiler` was not parsed. It would be more
  stable than the text output, but it is a different shape (and not
  available on older macOS), so the text parser stays.
- Rosetta detection (F2) is left open.

## Next steps

1. Capture real `SPDisplaysDataType` output on an Intel Mac with a dGPU and
   on an eGPU setup, and replace the synthetic fixtures.
2. If users report Rosetta launches, detect Apple Silicon from
   `SPHardwareDataType`/`SPDisplaysDataType` instead of `runtime.GOARCH`.
3. Consider reading `SPDisplaysDataType -json` when available, falling back
   to the text parser.

## Verification

```
gofmt -l internal/scan/            # empty
go test -p 2 ./internal/scan/      # ok
go build ./...                      # ok
go vet ./...                        # ok
```

Commit: `feec832` (parser, tests, fixtures) plus the report commit.
