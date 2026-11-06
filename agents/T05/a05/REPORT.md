# T05 a05: Windows display-class registry values

## What I did

- Read the Windows display-class parser: `display_windows.go` (`displayAdapters`,
  `readAdapter`) and `gpu_discrete.go` (`displayAdapter`, `usableAdapters`).
- Added `internal/scan/display_registry_test.go` and eight fixtures under
  `internal/scan/testdata/` (`display_*.txt`). Sources: `agents/T05/a05/SOURCES.md`.
- Extracted the value decoding out of `readAdapter` into a registry-free
  `adapterFrom` / `adapterVRAM` pair (`internal/scan/display_registry.go`, no
  build tag) so the corpus can run on any OS. `readAdapter` opens the key and
  delegates; no other behavior changed.
- Fixed two decoding bugs found by the fixtures (below) and recorded one
  Windows limitation the registry cannot fix.

## Parser covered

`readAdapter` (`internal/scan/display_windows.go:75`) reads one display driver
entry: `DriverDesc` (card name), `ProviderName`, and the memory from
`HardwareInformation.qwMemorySize` (64-bit), then
`HardwareInformation.MemorySize` as a binary value or a number. The name feeds
`usableAdapters` (`gpu_discrete.go:203`), which decides the vendor and whether
models can run on the card.

## Bugs found

### 1. An 8-byte `MemorySize` is read as 32 bits (major, fixed)

`display_windows.go:86-87` (before the change) decoded any `REG_BINARY`
`HardwareInformation.MemorySize` with `binary.LittleEndian.Uint32(b)`. If a
driver writes the size as an 8-byte value, only the low 32 bits are used.

Evidence: `display_intel_arc_memorysize_qword.txt` holds 16106127360 bytes
(15 GiB); the low 32 bits are `0xC0000000`. Before the fix the test reports
`VRAM:3221225472` (3 GiB) instead of `VRAM:16106127360` and the planner would
size models against 3 GB. The same fixture with the fix passes.

Fix (`display_registry.go`): read `Uint64` when the value is 8 bytes or
longer, keep the 4-byte read for older drivers.

### 2. A zero `qwMemorySize` hides the real `MemorySize` (minor, fixed)

`display_windows.go:84` (before the change) accepted `qwMemorySize` whenever
the read succeeded, including 0, and never tried `MemorySize`. A driver entry
with `qwMemorySize = 0` reported a 0 GB card, which `usableAdapters` drops.

Evidence: `display_intel_uhd_770.txt` (zero qw, `MemorySize` = 1 GiB). Before
the fix the adapter is `VRAM:0`; after, `VRAM:1073741824`.

Fix: accept `qwMemorySize` only when it is non-zero, then fall back.

### 3. The 4-byte sentinel sits just below the discrete threshold (major, not fixable here)

Windows writes `0x7FFFF000` into a 4-byte `MemorySize` for cards over 2 GiB
(oshi #2907, #3086). That is 2147479552 bytes, 4096 bytes under the
`2 * GB` threshold in `usableAdapters` (`gpu_discrete.go:209`), so a 16 GB
Intel Arc A770 whose driver has no `qwMemorySize` is dropped completely: the
scan reports no card and the app says "None the AI can use". This matches the
symptom in odysseus #6612. `display_intel_arc_memorysize_4byte.txt` records
the value and asserts the current outcome (adapter 2147479552, not usable).

Not fixed: the registry holds no correct size in that case. Options are DXGI
(`IDXGIAdapter::GetDesc`), or treating `0x7FFFF000` as "at least 2 GiB" and
using `Win32_VideoController.AdapterRAM` (itself capped near 4 GiB) as a
lower bound. Both are larger changes; left as an open question.

## Not bugs (covered by fixtures)

- A missing class subkey or an entry with no values: `adapterFrom` returns
  `ok = false`, `displayAdapters` skips it (`display_missing_key.txt`).
- A stale entry for a removed card: named, 0 GB, and `usableAdapters` drops
  it (`display_ghost_entry.txt`).
- `DriverDesc` naming: AMD `AMD Radeon(TM) Graphics` / `AMD Radeon RX 9070`
  and Intel `Intel(R) Arc(TM) A770 Graphics` map to the right vendor and
  bandwidth through `vendorOf` / `otherBandwidth`.

## Checks run

`gofmt -l .` (empty), `go build ./...`, `go vet ./...`,
`go test -p 2 ./internal/scan/`, `GOOS=windows GOARCH=amd64 go build ./...`,
`GOOS=windows GOARCH=arm64 go build ./...`, `GOOS=linux go build ./...`.

## Open questions

- Does any real driver write an 8-byte `MemorySize`? The 8-byte case is
  synthetic; oshi describes the value as 32-bit. The fix is harmless either
  way, but the fixture should be replaced if a real dump appears.
- Should an entry with memory but an empty `DriverDesc` count? `readAdapter`
  returns false for it, so a card with no name is skipped. No fixture shows
  that combination.
- The `HardwareInformation.MemorySize` sentinel could be recognized as
  "at least 2 GiB" instead of being read literally. It would keep the Arc
  card in the scan, with a conservative size.
