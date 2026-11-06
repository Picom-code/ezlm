# T05 a03 fixture sources

Every fixture is `rocm-smi --showmeminfo vram --showproductname --json` output,
the exact command `amdScan` runs. Values are bytes, as rocm-smi prints them.

## Real captures

| Fixture | Source | Notes |
|---|---|---|
| `rocm-6.1-rx7900xt.json` | marimo-team/marimo issue #9237, 2026-04-16: https://github.com/marimo-team/marimo/issues/9237 | Pasted `rocm-smi --json` output for a Radeon RX 7900 XT. Used verbatim; the issue's CSV line carries the same numbers. |
| `rocm-7.1-strix-halo-apu.json` | Same issue, `--showmeminfo all` output for a Strix Halo APU (gfx1151) | The VRAM keys and product keys are verbatim. The GTT keys from that capture were dropped because `amdScan` asks only for `vram`; see REPORT finding 6. |

## Built from the documented format

| Fixture | Source | Notes |
|---|---|---|
| `rocm-5.7-rx6800xt.json` | ROCm 5.7.1 `rocm_smi.py` (`showProductName`, `showMemInfo`): https://github.com/ROCm/rocm_smi_lib/blob/rocm-5.7.1/python_smi_tools/rocm_smi.py | ROCm 5.x prints `Card series` / `Card model` / `Card vendor` (lower-case second word) and `VRAM Total Memory (B)` / `VRAM Total Used Memory (B)`. Numbers are plausible RX 6800 XT values (16 GiB class). Synthetic capture in the documented shape. |
| `rocm-6.4-multi-gpu.json` | Same ROCm 6 key spelling as the marimo capture, two `cardN` objects. | Synthetic two-card machine (RX 6800 XT + RX 6600), built to pin card ordering. |
| `rocm-6.2-igpu-780m.json` | ROCm 6 spelling; Phoenix iGPU (Radeon 780M, device 0x15bf). | Synthetic capture. iGPU VRAM is the BIOS carveout (512 MiB here), which is what `--showmeminfo vram` reports. |
| `rocm-6.0-no-gpu.json` | `{}` / `system`-only is what rocm-smi prints when ROCm is installed but no AMD DRM card is present. | Synthetic. |
| `rocm-6.4-used-over-total.json` | Synthetic edge case in the documented format; used > total is a stale or shared counter. | Synthetic. Proves the free-memory underflow guard. |
| `rocm-6.0-low-power-banner.json` | guaardvark/guaardvark PR #362, 2026-10-31: https://github.com/guaardvark/guaardvark/pull/362 ("rocm-smi prints a low-power warning before the JSON on some machines"). Warning text is from ROCm `rocm_smi.py` (`"AMD GPU device(s) is/are in a low-power state..."`), https://github.com/ROCm/rocm-systems/blob/develop/projects/rocm-smi-lib/python_smi_tools/rocm_smi.py | The card object is the RX 7900 XTX from the same ROCm 6 format. Synthetic combination of a real warning line and a documented card object. |

## Format notes

- ROCm 6.x `formatJson` writes `card<N>` objects and a `system` object, all
  values are JSON strings, and spacers are suppressed under `--json`.
- ROCm 5.x `formatJson` does the same but its product keys are `Card series`,
  `Card model`, `Card vendor`.
- A machine without ROCm has no `rocm-smi` binary; `amdScan` must return
  false without touching the hardware.
