# Sources: example

The worked example of the fixture format, added with the corpus plumbing
(a13). It is not a parser group.

- `nvidia-smi-two-gpus.csv`: synthetic, built from the documented
  `nvidia-smi --query-gpu=name,memory.total,memory.free,driver_version
  --format=csv,noheader,nounits` output,
  https://docs.nvidia.com/deploy/nvidia-smi/index.html (accessed 2026-11-06).
  A desktop card, and a laptop card with little free memory.
