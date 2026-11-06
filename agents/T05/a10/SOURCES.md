# a10 fixture sources: edge machines

All fixtures live in `internal/scan/testdata/edge/`. Each one is a small,
hand-built reconstruction of a real machine's output, not a capture from a
machine this agent had access to; the format of every fixture is the format the
parser under test reads, and the provenance of that format is cited below. All
are labelled **synthetic** where the numbers are representative rather than
observed.

## Formats

1. **Windows display-class driver entries** (`display-*.txt`).
   `internal/scan/display_windows.go:75` `readAdapter` builds one
   `displayAdapter` per registry key
   `SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}\NNNN`,
   reading `DriverDesc`, `ProviderName` and `HardwareInformation.qwMemorySize`
   (64-bit, bytes) or `HardwareInformation.MemorySize`.
   Source for the class key path, the 32-bit `AdapterRAM` clamp and the
   `HardwareInformation.MemorySize` mapping:
   Microsoft, "Win32_VideoController class" (uint32 AdapterRAM;
   `MappingStrings` `Win32Registry|HardwareInformation.MemorySize`; the class
   key `{4D36E968-E325-11CE-BFC1-08002BE10318}\0000` appears in
   `InfFilename`), https://learn.microsoft.com/en-us/windows/win32/cimwin32prov/win32-videocontroller,
   fetched 2026-11-06. The same page states that hardware not compatible with
   WDDM "returns inaccurate property values", which is why the driver entries,
   not `AdapterRAM`, are the source of a card's size.
   My fixture writes the three values as `DriverDesc | ProviderName | bytes`,
   which is exactly the tuple `readAdapter` returns; the test's `edgeAdapters`
   helper does the split.

2. **`nvidia-smi --query-gpu=name,memory.total,memory.free,driver_version
   --format=csv,noheader,nounits`** (`nvidia-smi-*.csv`), and
   `--query-compute-apps=pid,process_name,used_memory`
   (`nvidia-smi-*-procs.csv`). Read by `internal/scan/gpu_discrete.go:15`
   `nvidiaScan`. Units are MiB; the parser shifts left by 20.
   Source for the query and the CSV shape: NVIDIA, `nvidia-smi` documentation
   (the `--query-gpu`/`--format=csv` options and the MiB unit of
   `memory.total`), https://docs.nvidia.com/deploy/nvidia-smi/index.html,
   referenced 2026-11-06. Row values are synthetic.

3. **`rocm-smi --showmeminfo vram --showproductname --json`**
   (`rocm-smi-*.json`). Read by `internal/scan/gpu_discrete.go:147`
   `amdScan`, which takes the string keys `VRAM Total Memory (B)`,
   `VRAM Total Used Memory (B)` and `Card series` (or `Card Series`).
   Source for the JSON shape and those key names: ROCm, `rocm_smi_lib`
   (the repo's `--json` output), https://github.com/ROCm/rocm_smi_lib,
   fetched 2026-11-06; documentation index
   https://rocm.docs.amd.com/projects/rocm_smi_lib/en/latest/.
   Row values are synthetic.

## Fixtures

| Fixture | Machine | Status | Why it is here |
|---|---|---|---|
| `display-woa-snapdragon-x-elite.txt` | Windows on Arm, Snapdragon X Elite, 16 GB shared LPDDR5X; Adreno X1-85 reporting an 8 GB shared aperture, plus `Microsoft Basic Display Adapter` | synthetic | The GPU has no VRAM of its own. `vendorOf` knew only NVIDIA, AMD and Intel, so the adapter was dropped and the laptop scanned as CPU-only. |
| `display-woa-snapdragon-x-elite-small-aperture.txt` | same laptop with the driver's default 2 GB aperture | synthetic | Below the 8 GB built-in threshold the processor should keep the model, so this pins the other side of the decision. |
| `display-vm-gpu-passthrough.txt` | QEMU/KVM Windows guest, RTX 4090 passed through, plus `Microsoft Hyper-V Video` | synthetic | NVIDIA cards must be left to `nvidia-smi`; the emulated adapter must not become a Vulkan card. |
| `display-vm-amd-passthrough.txt` | Windows guest with a Radeon RX 7900 XTX passed through and no `rocm-smi` in the guest | synthetic | The display entry is the only source of the card's size in that guest; 24 GB must survive as uint64 and keep the 960 GB/s estimate. |
| `display-1tb-workstation.txt` | 1 TB RAM Threadripper PRO workstation: two 96 GB RTX PRO 6000 and a 48 GB Radeon PRO W7900 | synthetic | 64-bit `qwMemorySize` values above 2^32 must not be clamped or dropped. |
| `nvidia-smi-1tb-dgx.csv` | DGX H200, eight 141 GB cards (1.1 TB aggregate), one `NVIDIA NPU` row reporting `[N/A]` | synthetic | 1 TB+ aggregate memory, and the `[N/A]` row must not count as a card. |
| `nvidia-smi-1tib-single.csv` | one accelerator with 1 TiB (1048576 MiB) | synthetic | The MiB-to-bytes shift must stay in uint64. |
| `nvidia-smi-zero-free.csv` + `nvidia-smi-zero-free-procs.csv` | RTX 4090 held entirely by a game: 0 bytes free, one app using 24000 MiB | synthetic | A card with nothing free must still be reported, with 0 usable. |
| `nvidia-smi-laptop-free-over-total.csv` | RTX 3050 Laptop under WDDM, 4096 MiB total, 6144 MiB free | synthetic | The driver counts shared system memory in "free"; free must be clamped to the card. |
| `rocm-smi-apu-used-over-total.json` | Radeon 780M APU, 512 MiB VRAM total, 1.5 GiB "used" | synthetic | Used includes shared memory and runs past total; the old `total - used` wrapped. |
| `rocm-smi-zero-free.json` | Radeon RX 7900 XTX with all VRAM in use | synthetic | 0 bytes free, card kept. |

## What is not a capture, and what to check next

- The Snapdragon aperture (8 GB of 16 GB, or 2 GB) is representative. On a
  real Snapdragon X laptop the Adreno driver reports its own aperture, which
  may be a fixed 2 GB or a share of RAM; the test covers both sides of the
  8 GB threshold, so the decision holds either way. See REPORT.md open
  questions.
- The DGX H200 numbers use 143771 MiB, which is the 141 GB (decimal) HBM3e
  figure in MiB. The `[N/A]` NPU row is modelled on the `NVIDIA NPU` row that
  `gpu_discrete.go:27` already comments about.
- No machine output was captured from a real VM. The VM fixtures assert what
  the parsers do with the shapes a guest produces, not that a specific guest
  produces them.
