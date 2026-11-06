# T05/a09 fixture sources: CPU-only machines

All fixtures were read on 2026-11-06. None contains a real user's data or a
secret. The `/proc/cpuinfo` flag and `lscpu` vulnerability lists are trimmed
for size; every field the parser reads is kept as it appears on the machine.

## Fixtures

| File | Machine | Kind | Source |
|---|---|---|---|
| `internal/scan/testdata/proc_cpuinfo_n100.txt` | Intel N100 mini PC, 4 cores, no SMT, no discrete GPU | synthetic | Built from the documented `/proc/cpuinfo` format (Linux, `Documentation/filesystems/proc.rst`, https://docs.kernel.org/filesystems/proc.html) with the N100's published topology (4 cores, 4 threads, 6 MB L3): https://www.intel.com/content/www/us/en/products/sku/231803/intel-processor-n100-6m-cache-up-to-3-40-ghz/specifications.html |
| `internal/scan/testdata/lscpu_n100.txt` | Same machine, `lscpu` | synthetic | Built from the `lscpu(1)` output format (https://man7.org/linux/man-pages/man1/lscpu.1.html) and the N100 topology above. |
| `internal/scan/testdata/proc_cpuinfo_container_partial.txt` | Container with two vCPUs on a Xeon Platinum 8375C host, `/proc` copy without `physical id`, `core id` or `cpu cores` | synthetic | Built from the documented format, with the `hypervisor` flag and the field subset some container runtimes (gVisor, some Kata and Firecracker guests) expose. The CPU is the one AWS uses for c6i instances: https://github.com/aws/aws-graviton-getting-started |
| `internal/scan/testdata/proc_cpuinfo_graviton2.txt` | AWS Graviton2 (arm64), 4 cores, no model name, `CPU part: 0xd0c` | synthetic | Built from the documented format and the kernel's arm64 part numbers (`arch/arm64/include/asm/cputype.h`, https://git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux.git/tree/arch/arm64/include/asm/cputype.h). 0xd0c is Neoverse-N1: https://github.com/aws/aws-graviton-getting-started |
| `internal/scan/testdata/lscpu_graviton2.txt` | Same machine, `lscpu` inside a 4-CPU container | synthetic | Same as above; lscpu names the core from the kernel's part table, which is where the fixture's `Model name: Neoverse-N1` comes from. |

## Inline cases

The tests also build small strings in code for a two-socket VM that keeps
`cpu cores` but drops `core id`, an old arm64 board that names itself in
`Hardware`, and an empty or partial `/proc` with no processor records. These
are synthetic and documented in the test comments.

## Not from a fixture

The Intel Mac CPU-only test fakes `sysctl -n machdep.cpu.brand_string` with a
common 2019 MacBook Pro chip (`Intel(R) Core(TM) i7-9750H CPU @ 2.60GHz`).
macOS has no `/proc`, so there is no file fixture for it.
