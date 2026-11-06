# T05 a01: nvidia-smi CSV fixture sources

Command the parser reads (`internal/scan/gpu_discrete.go`):

```
nvidia-smi --query-gpu=name,memory.total,memory.free,driver_version --format=csv,noheader,nounits
nvidia-smi --query-compute-apps=pid,process_name,used_memory --format=csv,noheader,nounits
```

With `noheader,nounits` each row is `name, total MiB, free MiB, driver` and an
unavailable value is printed as `[N/A]` (NVIDIA prints the brackets).

| Fixture | Provenance |
|---|---|
| `query_gpu_a100_mig.csv` | Synthetic row in the documented format. Parent memory 81920 MiB and the fact that `--query-gpu` lists the physical GPU, not the slice, measured on an A100-SXM4-80GB with a 1g.10gb MIG slice: aicell-lab/bioengine PR #226, 2026-10-01, https://github.com/aicell-lab/bioengine/pull/226 . MIG-enabled output shape also in NVIDIA, *MIG User Guide: Getting Started with MIG*, updated 2026-11-04, https://docs.nvidia.com/datacenter/tesla/mig-user-guide/getting-started-with-mig.html (H100 shows the parent card and 81559 MiB while two MIG devices exist). |
| `query_gpu_gb10_na.csv` | `[N/A]` memory on GB10 (Grace-Blackwell, DGX Spark), which shares system RAM: mostlygeek/llama-swap PR #965, 2026-07-30, https://github.com/mostlygeek/llama-swap/pull/965 ("nvidia-smi reports memory.used/memory.total as N/A on Grace-Blackwell/Grace-Hopper superchips and Jetson/Tegra boards (e.g. NVIDIA GB10), since they have no dedicated framebuffer"). The `NVIDIA NPU` row is synthetic, matching the existing parser comment. |
| `query_gpu_wddm_na.csv` | Real Windows WDDM output for a Quadro RTX 5000: `--query-gpu=memory.total,memory.used --format=csv,nounits,noheader` returned `[N/A], [N/A]`, one line ending in a NUL byte. Megvii-BaseDetection/YOLOX issue #1429, 2022-07-11, https://github.com/Megvii-BaseDetection/YOLOX/issues/1429 . Reduced to the four columns the parser asks for. |
| `query_gpu_dual_4090.csv` | Synthetic, documented format: two RTX 4090 rows, one with less free memory (a GPU in use). |
| `query_gpu_laptop_4060.csv` | Synthetic, documented format: one RTX 4060 Laptop GPU row with Windows CRLF line endings. |
| `query_gpu_free_na.csv` | Synthetic, documented format: total known, free `[N/A]`. |
| `compute_apps_busy.csv` | Synthetic, documented format: one process with 6 GiB, one graphics process with `[N/A]` memory, one invalid PID. |

All fixtures are a few lines of plain text and contain no host data.
