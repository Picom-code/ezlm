package integrate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"golang.org/x/sys/cpu"

	"github.com/picom-code/ezlm/internal/engine"
)

// EZLM's own OpenCode (DECISIONS 2): the pinned release archive, checked
// against its SHA-256, unpacked into ~/.ezlm/tools/opencode/<version> and
// launched by absolute path. No npm, no "latest", no PATH or shell changes,
// and a user's own install is never touched.

// openCodeDiskNeed covers the archive and the unpacked program side by side.
const openCodeDiskNeed = 450 << 20

// openCodeAssetKey picks the build for this machine: its own architecture
// (an x64 EZLM on Arm still gets the Arm build) and, on x64 CPUs without
// AVX2, the baseline build.
func openCodeAssetKey() string {
	arch := MachineArch()
	return AssetKey(runtime.GOOS, arch, arch == "amd64" && !cpu.X86.HasAVX2)
}

// openCodeInstall installs the pinned OpenCode and returns it. A failed or
// cancelled install leaves no half-unpacked copy behind.
func openCodeInstall(ctx context.Context, r *Run) (Found, error) {
	pin, ok := PinFor("opencode")
	if !ok {
		return Found{}, fmt.Errorf("EZLM has no OpenCode version to install.")
	}
	key := openCodeAssetKey()
	a, ok := pin.Assets[key]
	if !ok {
		return Found{}, fmt.Errorf("EZLM has no OpenCode %s for this computer (%s). Install OpenCode yourself, then press Connect again.", pin.Version, key)
	}
	dir := ToolDir("opencode", pin.Version)
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return Found{}, err
	}
	if err := CheckDisk(parent, openCodeDiskNeed, "OpenCode"); err != nil {
		return Found{}, err
	}
	stage, err := os.MkdirTemp(parent, "."+pin.Version+"-")
	if err != nil {
		return Found{}, err
	}
	defer os.RemoveAll(stage)

	r.Step(fmt.Sprintf("Downloading OpenCode %s…", pin.Version))
	dctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	arc := filepath.Join(stage, filepath.Base(a.URL))
	if err := FetchAsset(dctx, r, a, arc); err != nil {
		if dctx.Err() == context.DeadlineExceeded {
			return Found{}, fmt.Errorf("Downloading OpenCode didn't finish within 15 minutes. Check the connection, then press Connect again.")
		}
		return Found{}, err
	}
	r.Logf("checksum ok for %s", filepath.Base(a.URL))
	r.Step("Unpacking OpenCode…")
	out := filepath.Join(stage, "out")
	if err := engine.Extract(arc, out); err != nil {
		return Found{}, fmt.Errorf("Unpacking OpenCode failed (%v). Press Connect to try again.", err)
	}
	os.Remove(arc)
	exe := openCodeFindExe(out)
	if exe == "" {
		return Found{}, fmt.Errorf("The OpenCode download has no %s in it. Press Connect to try again.", openCodeExe())
	}
	if err := os.Chmod(exe, 0o755); err != nil {
		return Found{}, err
	}
	if rel, _ := filepath.Rel(out, exe); rel != openCodeExe() {
		// Keep only the program's own folder, so it lands at dir/opencode.
		out = filepath.Dir(exe)
	}
	os.RemoveAll(dir) // only ever EZLM's own copy of this version
	if err := os.Rename(out, dir); err != nil {
		return Found{}, err
	}
	bin := filepath.Join(dir, openCodeExe())
	p, err := openCodeProbe(ctx, r, bin)
	if err != nil || p.Version != pin.Version {
		os.RemoveAll(dir)
		r.Logf("installed OpenCode reports %q (%v), want %s", p.Version, err, pin.Version)
		return Found{}, fmt.Errorf("The OpenCode EZLM installed didn't start. Details are in %s.", r.LogPath())
	}
	r.Logf("installed OpenCode %s at %s", pin.Version, bin)
	return Found{Bin: bin, Version: pin.Version, Source: SourceEZLM, Arch: p.Arch, Env: []string{"OPENCODE_DISABLE_AUTOUPDATE=1"}}, nil
}

// openCodeFindExe finds the program in an unpacked archive: at the top or
// one folder down.
func openCodeFindExe(dir string) string {
	for _, pat := range []string{openCodeExe(), filepath.Join("*", openCodeExe())} {
		m, _ := filepath.Glob(filepath.Join(dir, pat))
		for _, p := range m {
			if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
				return p
			}
		}
	}
	return ""
}
