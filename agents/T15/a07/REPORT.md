# T15 a07: ARM64 vs x64 on Windows

Slice: process architecture, emulation, `runtime.GOARCH` assumptions and child
process architecture mismatches. Owner of `*_windows.go` outside the frozen
areas; `windows/**` is read-only, so the C# and installer items below are
findings only.

Environment: no Windows machine, so everything is code-read plus tests that
build under `GOOS=windows`. The one new test file is
`internal/engine/arch_windows_test.go` (`//go:build windows`); its assertions
also run on the host (they take the architecture as a parameter), which is how
they were checked here.

## How EZLM is shipped on Windows (the premise for everything below)

- `windows/EZLM/EZLM.csproj:10-11`: `RuntimeIdentifiers=win-x64;win-arm64`,
  `Platforms=x64;ARM64`, and `Engine\**` is copied from the publish folder.
  So `Engine\ezlm.exe` has the same architecture as `EZLM.exe` **as long as
  the publish step puts the matching Go build there**.
- `windows/installer/EZLM.iss:17-18,39-40`: one universal setup,
  `ArchitecturesAllowed=x64compatible or arm64`, and `[Files]` copies
  `SrcArm64` when `IsArm64` (Inno's built-in machine check, true even though
  setup itself runs as x86), otherwise `SrcX64`.
- Consequence: on a native install `runtime.GOARCH` (the build's arch) and the
  machine arch agree, so the existing `GOARCH` decisions are self-consistent
  and EZLM works on Windows on Arm. The gaps below are about the paths where
  they can disagree, and about one latent asset-selection trap.

Nothing in this repo builds the Windows engine (`windows/` holds only the C#
project and the .iss), so "the arm64 publish contains an arm64 engine" is a
release-process invariant with no test behind it. See F1.

## Findings

### F1. An x64 engine in the arm64 app quietly downloads an x64 llama.cpp (major)

`internal/engine/engine.go:218` and `:222`:

```go
cuda13 := prereq.NeedsCUDA13(hw) || runtime.GOARCH == "arm64"
...
assets, err := PickAssets(r, runtime.GOOS, runtime.GOARCH, backend, cuda13)
```

Both the CUDA-13 rule ("arm64 only has CUDA 13 builds") and the archive choice
use the **build's** architecture. If `Engine\ezlm.exe` in the win-arm64
publish is an amd64 build (wrong publish step, or an older install whose
engine was not replaced), the whole machine is served by the x64 llama.cpp
under emulation: it runs, so nobody sees an error, it is just roughly half the
CPU speed and without NEON. The reverse (arm64 engine on x64) cannot happen
through the installer, but a copied folder can produce it, and then
`llama-server.exe` does not start at all.

Evidence: `internal/engine/engine_test.go:36-56` covers the asset table for
both architectures, and the new `TestPickAssetsWindowsArch` pins "the archive
must be this architecture" for CPU/Vulkan/ROCm/CUDA, but neither can catch a
wrong engine binary in the publish folder.

Proposed fix: use the machine's architecture where the OS may run a native
child, keeping `runtime.GOARCH` only where the file must match this process.
`internal/integrate/arch_windows.go:11-21` already does exactly that with
`IsWow64Process2`; the same check belongs in a small `scan.MachineArch()`
(both `engine` and `prereq` import `scan`, so there is no cycle), then
`Install` becomes `PickAssets(r, runtime.GOOS, scan.MachineArch(), ...)` and
the CUDA-13 rule keys off it too. Also add the engine architecture to the
publish check, or at least a self-test line: `Hardware.Arch` is already in the
report (`internal/scan/scan.go:91`).

### F2. `vcArch()` picks the VC redist for the build, not for the llama-server that runs (major, same cause as F1)

`internal/prereq/prereq.go:81-86`, used at `:89` (download URL), `:93`
(installed-version registry query) and `:184` (installer file name):

```go
func vcArch() string {
	if runtime.GOARCH == "arm64" {
		return "arm64"
	}
	return "x64"
}
```

The MSVC runtime is needed by the **process that loads it** — `llama-server.exe`
— not by EZLM. Today both are the same architecture, so this is consistent;
with F1 fixed (or with an x64 engine in an arm64 app) it is not: the arm64
machine would get an arm64 VC redist while running the x64 `llama-server`, and
the check at `:93` would report "ok" for a runtime the server cannot use.
Proposed fix: derive `vcArch` from the same machine-arch helper, or better,
from the architecture of the `llama-server.exe` EZLM is about to run (which is
known after `PickAssets`). This is a prereq.go change, outside my owned files.

### F3. An x64 EZLM on Arm loses the unified-memory detection on macOS (minor, same class, not Windows)

`internal/scan/quick.go:29` (`runtime.GOOS == "darwin" && runtime.GOARCH == "arm64"`),
`internal/scan/scan.go:91` (`Arch: runtime.GOARCH`) and
`internal/scan/scan_darwin.go:10` (`h.Arch != "arm64"`). The Mac app builds a
universal engine (`macos/scripts/build-app.sh:29-36`, `lipo` of arm64+amd64), so
a user who launches the app under Rosetta gets `GOARCH=amd64` on Apple Silicon:
`Unified` stays false, GPU memory falls back to `nvidia-smi`/`otherGPUMemory`,
the model fit is computed for a discrete card, and the engine downloads
`llama-bXXXX-bin-macos-x64` (`internal/engine/engine.go:120`). Proposed fix: the
same `MachineArch()` helper in `scan` (Rosetta check is already written in
`internal/integrate/arch_darwin.go:11-16`), used by `quick.go`, `scan.go` and
`scan_darwin.go`.

### F4. `cuda13=false` still selects a CUDA 13 build on arm64 (minor, latent)

`internal/engine/engine.go:130-135`: with `cuda13 == false` the patterns are
`{"-bin-win-cuda-12", a}`, `{"-bin-win-cuda-", a}`, ... The second one matches
`llama-b11960-bin-win-cuda-13.4-arm64.zip`, because no CUDA 12 arm64 build
exists. `Install` gates the driver at 12.4 (`:215`) before that call, so
today the build only arrives through the arm64 forcing at `:218`; if that
forcing is ever moved or a caller passes `cuda13=false` for arm64, EZLM installs
a server that needs driver 13.0 on a 12.x driver and `llama-server` fails to
start. Same for `linux/arm64` (pinned list has only `ubuntu-cuda-13.4-arm64`).
Proposed fix: make the "arm64 has no CUDA 12 build" rule part of
`assetPatterns` (drop the `cuda-12` pattern for `a == "arm64"`) so the
knowledge is in one place. My `TestPickAssetsWindowsArch` covers both
`cuda13` values but only asserts the architecture, not the driver level.

### F5. The report header names the build architecture (minor)

`internal/app/launch.go:190` prints `runtime.GOOS`/`runtime.GOARCH` in the
diagnostic report, and `windows/EZLM/Shell.cs:204` prints
`RuntimeInformation.OSArchitecture` (correct: OS, not process, so Arm shows
Arm even for an x64 process). The two lines disagree on an x64 EZLM on Arm.
Proposed fix: print `hw.Arch` (machine) and, when it differs from
`runtime.GOARCH`, both ("arm64 machine, x64 build under emulation"), which
would also have made F1 visible in a user's report. `launch.go` is frozen
(`internal/app/**`), so this is a note for whoever owns it.

### F6. Nothing else in the Windows-specific code is architecture-dependent (informational)

Checked every `*_windows.go` outside the frozen areas for x86/x64 assumptions:
`internal/proc/proc_windows.go` (job object structs use `uintptr`, identical
on amd64 and arm64), `internal/awake/awake_windows.go` (same),
`internal/serve/service_windows.go` (Run key, `schtasks`, `os.Executable`),
`internal/scan/display_windows.go` (registry + PowerShell, no arch),
`internal/scan/apps_windows.go` (registry), `internal/integrate/pty_windows.go`
and `terminal_windows.go` (`%SystemRoot%\System32\cmd.exe`, correct for a
64-bit process on both architectures), `internal/app/volume_windows.go`,
`internal/scan/exec_windows.go`, the two `hide_windows.go` files. No 32-bit
assumption anywhere (EZLM is 64-bit only on both), no `PROCESSOR_ARCHITECTURE`
env reads, no `Program Files (x86)` paths.

The integrate package is already architecture-correct and is the model to copy:
`internal/integrate/opencode_install.go:25-29` and `hermes.go:294` use
`MachineArch()` (an x64 EZLM on Arm gets the Arm OpenCode/Node build), and
`opencode_detect.go:92` replaces an x64 `opencode` under emulation. Its pins
cover `windows-arm64` (`internal/integrate/tools.json:8,27`).

### F7. Cross-slice note: the VC redist version check never matches (a05's area)

`internal/prereq/prereq.go:94`: `regexp.MustCompile("v(\\d+\\.\\d+\\.\\d+)")`
against `reg query ... /v Version` output, which is
`    Version    REG_SZ    14.44.35211.0` — there is no `v` before the number,
so `m == nil` and `vcRedist` always reports "Not installed" and reinstalls the
redistributable on every check. Arch-independent; flagging it because it is in
the same function as F2. Needs a Windows run to confirm the exact output
format.

## What I changed

- `internal/engine/arch_windows_test.go` (new, `//go:build windows`):
  - `TestPickAssetsWindowsArch`: for `windows/amd64` and `windows/arm64`,
    across CPU, Vulkan, ROCm and CUDA (both `cuda13` values), the chosen
    archive and its CUDA runtime must both be the machine's architecture and
    must never contain the other one. This is the regression guard for the
    whole slice: it fails if anyone loosens `assetPatterns`.
  - `TestFlavorSeparatesWindowsArch`: the install folder flavor differs
    between x64 and arm64 for the same release (`win-cpu-x64` vs
    `win-cpu-arm64`), so an architecture switch never reuses the other
    architecture's `llama-server`.
  No production code changed: every real fix above lands in `engine.go`,
  `prereq.go` or `scan/*.go`, which are outside this task's owned
  `*_windows.go` paths, and one (`integrate/**`) is frozen.

## What I could not verify without a Windows machine

- That `Engine\ezlm.exe` in the win-arm64 publish is really an arm64 Go build
  (F1) — it depends on the release script, which is not in this repo.
- That Inno's `IsArm64` behaves as expected when setup runs as x86 on Arm
  (`EZLM.iss:39-40`); code-read only.
- The exact `reg query` output for the VC redist key (F7), and whether the
  arm64 redist writes `...\VC\Runtimes\arm64` (F2).
- That `IsWow64Process2` (`internal/integrate/arch_windows.go:13`) is present
  and correct on the minimum supported build (19041) — it is documented for
  Windows 10 1809+, so this should be fine.
- Emulated x64 `llama-server` speed on Arm (the "about half" figure in F1 is
  an estimate, not a measurement).

## Open questions

- Should EZLM ever download the *native* build on an emulated process (F1/F2
  fix), or is matching the engine's architecture deliberate? Native is faster,
  but it means two different architectures running in one install, and a
  fallback is needed if the native build fails to start.
- Is the arm64 Windows app a supported target for the public beta, or
  best-effort? If supported, the manual checklist should cover it (a16).

## Next steps

1. Add `scan.MachineArch()` (`IsWow64Process2` on Windows, `proc_translated`
   on macOS, `runtime.GOARCH` elsewhere) and move the three `GOARCH` decisions
   (`engine.Install`, `prereq.vcArch`, `scan.QuickMemory`/`Scan`) onto it.
2. Print both machine and build architecture in the report (F5) and in the
   Windows app's self-test.
3. Fold the "arm64 has only CUDA 13" rule into `assetPatterns` (F4) and add
   the driver-level assertion to the test above.
4. On a Windows ARM64 VM: confirm the installed `Engine\ezlm.exe` header, that
   `PickAssets` lands on `win-cpu-arm64`/`win-vulkan-arm64`, and that the
   arm64 VC redist key is the one queried.
