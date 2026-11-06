# a02 sources: nvidia-smi across driver versions

All fixtures live in `internal/scan/testdata/nvidia-smi/`. None were captured
on this machine (it has no NVIDIA GPU, and T05 forbids probing the host), so
each is a **synthetic case built from the documented format** for the driver
release named in its file name. No fixture contains a real hostname, user name
or serial number.

## Primary source

- NVIDIA, `nvidia-smi` documentation, <https://docs.nvidia.com/deploy/nvidia-smi/index.html>,
  read 2026-11-06. Statements used:
  - "The output of NVSMI is not guaranteed to be backwards compatible." This is
    the reason the fixtures are split by driver release rather than by card.
  - "Some devices and/or environments don't support all possible information.
    Any unsupported data is indicated by a 'N/A' in the output." This covers
    `memory.total` and `memory.free` printing `[N/A]` on MIG-enabled and
    virtualised GPUs.
  - `--format=` takes `csv`, `noheader` and `nounits`; `--query-gpu=` takes a
    comma-separated property list, and `--query-compute-apps=` likewise.
  - `-x, --xml-format` produces XML instead of the human-readable/CSV output.
    EZLM never passes it (see REPORT.md).

## Fixture provenance

| Fixture | Driver / card | Built from | Notes |
|---|---|---|---|
| `csv_r470_rtx3060_linux.txt` | R470, RTX 3060 12 GB | documented CSV format, MiB, no units | four fields: name, total, free, driver |
| `csv_r340_gtx780_legacy.txt` | R340, GTX 780 | documented CSV format | old drivers print the board name without the `NVIDIA` prefix |
| `csv_r535_two_gpus_in_use.txt` | R535, 2x RTX 3090 | documented CSV format | second card has less free memory |
| `csv_r550_windows_crlf.txt` | R550, 2x RTX 4090 on Windows | documented CSV format | CRLF line endings |
| `csv_r580_no_memory_mig.txt` | R580, A100 with MIG on | documented `N/A` behaviour | `memory.total` and `memory.free` are `[N/A]` |
| `csv_r450_free_unknown.txt` | R450, A100 80 GB | documented `N/A` behaviour | total known, free `[N/A]` |
| `csv_r580_gb10_with_npu_row.txt` | R580, GB10 | documented `N/A` behaviour | an `NVIDIA NPU` row follows the GPU row (matches the comment in `gpu_discrete.go`) |
| `header_r470.txt` | R470 | documented summary header | `CUDA Version: 11.4` |
| `header_r580.txt` | R580 | documented summary header | `CUDA UMD Version: 13.0`; the same rename is asserted in `internal/scan/cuda_test.go` |
| `header_r340_legacy.txt` | R340 | documented summary header | older layout with no CUDA field at all |
| `xml_r550_rtx4090.xml` | R550, RTX 4090 | `-x/--xml-format` device DTD shape | not read by the parser; kept to prove that |

## How the fixtures are used

`internal/scan/nvidia_versions_test.go` parses each CSV fixture with
`parseNvidiaGPUs` and compares the resulting `scan.GPU` values field by field.
The header fixtures are fed to `cudaVersionRe`, and the XML fixture proves the
parser only understands the CSV query.
