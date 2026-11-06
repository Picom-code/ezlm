# a07 sources: macOS `sysctl` on M1 to M5 and Intel

All fixtures are small text captures. `sysctl` dumps use the `sysctl -a` line
format (`key: value`); a leading `#` line records the machine the row stands
for. Values that are not real captures are marked synthetic.

## Fixtures

| Fixture | Command / format | Machine | Provenance |
|---|---|---|---|
| `sysctl_m1_16gb.txt` | `sysctl -a` excerpt | Apple M1, 8-core (4P+4E), 16 GB | Synthetic from the documented format. Core split and RAM from Apple's M1 specs (MacBook Air, 2020). |
| `sysctl_m1_max_64gb.txt` | `sysctl -a` excerpt | Apple M1 Max, 10-core (8P+2E), 64 GB | Synthetic. Core split from Apple's M1 Max specs (MacBook Pro 16-inch, 2021). |
| `sysctl_m2_16gb.txt` | `sysctl -a` excerpt | Apple M2, 8-core (4P+4E), 16 GB | Synthetic. Core split from Apple's M2 specs (MacBook Air, 2022). |
| `sysctl_m2_pro_32gb.txt` | `sysctl -a` excerpt | Apple M2 Pro, 12-core (8P+4E), 32 GB | Synthetic. Core split from Apple's M2 Pro specs (Mac mini, 2023). |
| `sysctl_m3_pro_18gb.txt` | `sysctl -a` excerpt | Apple M3 Pro, 11-core (5P+6E), 18 GB, macOS 26.3.1 | **Captured** on the author's Mac, 2026-11-06. |
| `sysctl_m4_pro_24gb.txt` | `sysctl -a` excerpt | Apple M4 Pro, 12-core (8P+4E), 24 GB | Synthetic. Core split from Apple's M4 Pro specs (MacBook Pro 14-inch, 2024). |
| `sysctl_m5_16gb.txt` | `sysctl -a` excerpt | Apple M5, 10-core (4P+6E), 16 GB | Synthetic. Core split from Apple's M5 specs (MacBook Pro 14-inch, 2025). |
| `sysctl_m5_max_128gb.txt` | `sysctl -a` excerpt | Apple M5 Max, 16-core (12P+4E), 128 GB | Synthetic. The top tier the parser's bandwidth table anticipates. |
| `sysctl_intel_macbookpro16_16gb.txt` | `sysctl -a` excerpt | Intel Core i9-9880H, 8 cores / 16 threads, 16 GB | Synthetic from Apple's 2019 MacBook Pro 16-inch specs. |
| `sysctl_proc_translated_rosetta.txt` | `arch -x86_64 sysctl -n sysctl.proc_translated` | Apple M3 Pro under Rosetta | **Captured** on the author's Mac, 2026-11-06: output `1`. The same command natively prints `0`, and on an Intel Mac the key does not exist. |
| `memory_pressure_m3_pro.txt` | `memory_pressure` | Apple M3 Pro, 18 GB, macOS 26.3.1 | **Captured** 2026-11-06. Page counters are rounded; the parser only reads the last line (`System-wide memory free percentage: 66%`). |
| `memory_pressure_m1_16gb.txt` | `memory_pressure` | Apple M1, 16 GB | Synthetic from the documented layout, with `free percentage: 62%`. |

## Formats and documented behaviour

- `sysctl(8)` man page (macOS 26.3.1), <https://keith.github.io/xcode-man-pages/sysctl.8.html>, accessed 2026-11-06.
  `-n` prints the value only; a missing key writes `sysctl: unknown oid ...` to
  stderr and exits non-zero, so `run()` returns empty output.
- `memory_pressure(1)` man page, <https://keith.github.io/xcode-man-pages/memory_pressure.1.html>, accessed 2026-11-06.
- Apple, "Recommended maximum working set size", <https://developer.apple.com/documentation/metal/device/recommendedmaxworkingsetsize>, accessed 2026-11-06:
  the GPU may wire 67% of RAM below 36 GB and 75% at 36 GB and above, unless
  the user raises `iogpu.wired_limit_mb`. This is the boundary the parser got
  wrong for exactly-36 GB Macs (see REPORT).
- Rosetta detection with `sysctl.proc_translated`: Apple, "About the Rosetta
  translation environment", <https://developer.apple.com/documentation/apple-silicon/about-the-rosetta-translation-environment>, accessed 2026-11-06;
  the same check already exists in this repo at `internal/integrate/arch_darwin.go:12`.
- `hw.memsize` is the same value gopsutil's darwin `VirtualMemory` reads; the
  fixture exists to pin the number and to cover the new fallback when gopsutil
  fails.

Apple's published unified-memory bandwidth figures (68/100/100/120/153 GB/s for
M1 to M5 base, and the Pro/Max/Ultra tiers) are the numbers the parser's table
already encodes; the fixtures assert them rather than re-derive them.
