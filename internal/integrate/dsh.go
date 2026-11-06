package integrate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// DeepSeek Harness (dsh), DeepSeek's agent harness (PLAN section 6,
// DECISIONS 4 and 6). EZLM routes it through dsh's llm-pi-ai adapter with a
// provider called "ezlm", checks it with the headless profile and opens the
// official browser UI (`dsh web`). dsh is a Node.js app: EZLM runs its
// lib/bin.js with an absolute node, so neither PATH nor npm's shims decide
// which Node it gets.

func init() {
	Register(dshTool{})
	RegisterErrorText("dsh",
		ErrorText{Match: "MISSING_CREDENTIAL", Say: "DeepSeek Harness can't find EZLM's key. Press Connect to write it again."},
		ErrorText{Match: "INVALID_CREDENTIAL", Say: "DeepSeek Harness has an old EZLM key. Press Connect to update it."},
		ErrorText{Match: "UNKNOWN_MODEL", Say: "DeepSeek Harness asks for a model EZLM isn't serving. Press Connect to update it.", State: StateNeedsUpdate},
		ErrorText{Match: "Unexpected reasoning effort", Say: "The model refused a reasoning level DeepSeek Harness asked for. Press Connect to update the levels it offers.", State: StateNeedsUpdate},
		ErrorText{Match: "UNSUPPORTED_REASONING_EFFORT", Say: "DeepSeek Harness asked for a reasoning level this model doesn't offer. Press Connect to update the levels it offers.", State: StateNeedsUpdate},
		ErrorText{Match: "exceed_context_size", Say: "The model's context is too small for DeepSeek Harness. Pick a larger context or a smaller model, then connect again."},
		ErrorText{Match: "CONTEXT_WINDOW_EXCEEDED", Say: "The model's context is too small for DeepSeek Harness. Pick a larger context or a smaller model, then connect again."},
		ErrorText{Match: "LLM_STREAM_IDLE_TIMEOUT", Say: "The model took too long to answer DeepSeek Harness. Try again, or pick a faster model."},
		ErrorText{Match: "TRANSPORT: Connection error", Say: "EZLM's model server isn't running. Start it, then try again."},
		ErrorText{Match: "is readable beyond its owner", Say: "DeepSeek Harness won't read its credentials file while other users can. Run chmod 600 on it, then try again."},
		ErrorText{Match: "timed out waiting for the writer lock", Say: "DeepSeek Harness is saving its settings right now. Try again in a moment."},
		ErrorText{Match: "EADDRINUSE", Say: "Another program is using DeepSeek Harness's port. Close it, then open DeepSeek Harness again."},
		ErrorText{Match: "ERR_MODULE_NOT_FOUND", Say: "DeepSeek Harness's install is incomplete. Press Connect to install it again."},
		ErrorText{Match: "Cannot find module", Say: "DeepSeek Harness's install is incomplete. Press Connect to install it again."},
		ErrorText{Match: "EBADENGINE", Say: "This Node.js is too old for DeepSeek Harness. Press Connect to let EZLM install its own."},
	)
}

// dshNodeMin is the oldest Node.js dsh 0.2.1-rc.5 runs on. The package
// declares no engines of its own; its strictest dependencies (pi-ai 1.5.0,
// undici, pi-schema) need >=22.19.0, and lib/bin.js uses import.meta.main.
const dshNodeMin = "22.19.0"

// dshTelemetryEnv goes on every dsh process EZLM starts. 0.2.1-rc.5 reads
// both: DSH_TELEMETRY_MODE is the session-telemetry-otel row's mode
// (FEEDBACK_ONLY by default), and any non-empty DSH_TELEMETRY_DISABLED makes
// the launcher disable that row outright.
var dshTelemetryEnv = []string{"DSH_TELEMETRY_MODE=DISABLED", "DSH_TELEMETRY_DISABLED=1"}

type dshTool struct{}

func (dshTool) Info() Info {
	return Info{ID: "dsh", Name: "DeepSeek Harness", Desc: "DeepSeek's agent harness, in your browser", Icon: "dsh", Order: 30,
		Launch: []LaunchMode{{ID: ModeWeb, Label: "Open DeepSeek Harness (browser)"}}}
}

// dshHome is DeepSeek Harness's data folder: $DSH_HOME, else ~/.dsh
// (%USERPROFILE%\.dsh on Windows).
func dshHome() string {
	if h := os.Getenv("DSH_HOME"); h != "" {
		return h
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".dsh")
}

// dshDefaultHome reports whether dshHome is dsh's own default, the home a
// `dsh web` the user started without DSH_HOME serves.
func dshDefaultHome() bool {
	home, _ := os.UserHomeDir()
	return filepath.Clean(dshHome()) == filepath.Join(home, ".dsh")
}

// dshCopy is one dsh installation.
type dshCopy struct {
	Bin     string // what the tile shows: the user's dsh or EZLM's node_modules/.bin/dsh
	Script  string // its lib/bin.js
	Version string // from its package.json
	Source  string
	Node    string // EZLM's copy: the node it was installed with
}

// dshPrefix is EZLM's own dsh: ~/.ezlm/tools/dsh/<version>.
func dshPrefix(version string) string { return ToolDir("dsh", version) }

// dshMarker is written last by EZLM's install, so a half-done npm install
// never counts as installed. It records the node the copy was built with:
// native modules are built for one Node.
func dshMarker(prefix string) string { return filepath.Join(prefix, ".ezlm-install.json") }

type dshMarkerFile struct {
	Version     string `json:"version"`
	Node        string `json:"node"`
	NodeVersion string `json:"node_version"`
}

// dshScriptFor finds the lib/bin.js behind a dsh command: an npm symlink
// (Mac, Linux), a Windows shim next to node_modules, or EZLM's .bin entry.
func dshScriptFor(bin string) string {
	if real, err := filepath.EvalSymlinks(bin); err == nil && filepath.Base(real) == "bin.js" {
		if dshPackageVersion(real) != "" {
			return real
		}
	}
	dir := filepath.Dir(bin)
	for _, c := range []string{
		filepath.Join(dir, "node_modules", "@deepseek-ai", "dsh", "lib", "bin.js"),              // %APPDATA%\npm\dsh.cmd
		filepath.Join(dir, "..", "@deepseek-ai", "dsh", "lib", "bin.js"),                        // node_modules/.bin/dsh
		filepath.Join(dir, "..", "lib", "node_modules", "@deepseek-ai", "dsh", "lib", "bin.js"), // <prefix>/bin/dsh
	} {
		if dshPackageVersion(c) != "" {
			return filepath.Clean(c)
		}
	}
	return ""
}

// dshPackageVersion is the version in the package.json of the dsh package a
// lib/bin.js belongs to ("" when it isn't dsh's).
func dshPackageVersion(script string) string {
	b, err := os.ReadFile(filepath.Join(filepath.Dir(filepath.Dir(script)), "package.json"))
	if err != nil {
		return ""
	}
	var p struct{ Name, Version string }
	if json.Unmarshal(b, &p) != nil || p.Name != "@deepseek-ai/dsh" {
		return ""
	}
	return p.Version
}

// dshTested reports whether EZLM runs this dsh version as it is.
func dshTested(v string) bool {
	p, ok := PinFor("dsh")
	return ok && v != "" && (v == p.Version || slices.Contains(p.Tested, v))
}

// dshCopies lists the usable installations without running anything: the
// user's own dsh when its version is tested (DECISIONS 4), then EZLM's copy.
// other is the user's dsh when it is a version EZLM hasn't tested.
func dshCopies() (usable []dshCopy, other *dshCopy) {
	for _, bin := range FindAllTools("dsh") {
		if IsEZLMTool(bin) {
			continue
		}
		s := dshScriptFor(bin)
		if s == "" {
			continue
		}
		c := dshCopy{Bin: bin, Script: s, Version: dshPackageVersion(s), Source: SourcePath}
		if strings.Contains(filepath.ToSlash(s), "/node_modules/@deepseek-ai/dsh/") {
			c.Source = SourceNPM
		}
		if dshTested(c.Version) {
			usable = append(usable, c)
		} else if other == nil {
			other = &c
		}
	}
	if p, ok := PinFor("dsh"); ok {
		if c, ok := dshEZLMCopy(p.Version); ok {
			usable = append(usable, c)
		}
	}
	return usable, other
}

// dshEZLMCopy is EZLM's own dsh of that version, if its install finished.
func dshEZLMCopy(version string) (dshCopy, bool) {
	prefix := dshPrefix(version)
	b, err := os.ReadFile(dshMarker(prefix))
	if err != nil {
		return dshCopy{}, false
	}
	var m dshMarkerFile
	json.Unmarshal(b, &m)
	script := filepath.Join(prefix, "node_modules", "@deepseek-ai", "dsh", "lib", "bin.js")
	if dshPackageVersion(script) != version {
		return dshCopy{}, false
	}
	bin := filepath.Join(prefix, "node_modules", ".bin", "dsh")
	if runtime.GOOS == "windows" {
		bin += ".cmd"
	}
	return dshCopy{Bin: bin, Script: script, Version: version, Source: SourceEZLM, Node: m.Node}, true
}

func (dshTool) Detect(ep *Endpoint) Detection {
	home := dshHome()
	d := Detection{Config: dshPatchPath(home, "web")}
	usable, other := dshCopies()
	if len(usable) > 0 {
		c := usable[0]
		d.Bin, d.Version, d.Source = c.Bin, c.Version, c.Source
		if p, ok := CachedProbe(c.Bin); ok {
			d.Arch = p.Arch
		}
	} else if other != nil {
		p, _ := PinFor("dsh")
		d.Detail = fmt.Sprintf("Your DeepSeek Harness %s isn't a version EZLM has tested, so Connect installs %s for EZLM to use.", other.Version, p.Version)
	}
	st := dshReadState(home, ep)
	d.Linked = st.linked
	switch {
	case ep != nil && !ep.ToolCalls:
		d.State = StateNeedsTools
		d.Detail = "This model can't use tools, so DeepSeek Harness can't edit files with it. Pick a model marked Tools, or use Chat."
	case d.Linked && ep != nil && !st.current:
		d.State = StateNeedsUpdate
	}
	return d
}

// dshState is what the files say about EZLM's connection.
type dshState struct {
	linked  bool // some managed profile has EZLM's provider
	current bool // every managed profile has it for ep, and the key in force is ep's
}

func dshReadState(home string, ep *Endpoint) dshState {
	var st dshState
	all := true
	var want *yaml.Node
	if ep != nil {
		want = dshProviderNode(*ep)
	}
	for _, p := range dshManagedProfiles(home) {
		b, err := os.ReadFile(dshPatchPath(home, p))
		if err != nil {
			all = false
			continue
		}
		d, err := dshParse(b)
		if err != nil {
			all = false
			continue
		}
		prov := dshProviders(d.root())
		i := dshPairIndex(prov, dshProvider)
		if i < 0 {
			all = false
			continue
		}
		st.linked = true
		if ep != nil && !dshSame(prov.Content[i+1], want) {
			all = false
		}
	}
	if ep == nil {
		st.current = st.linked
		return st
	}
	env, _ := os.ReadFile(dshEnvPath(home))
	if dshEnvKey(env) != dshKeyValue(ep.Key) {
		all = false
	}
	if ref, ok := dshStoredRef(home); ok && ref != dshKeyValue(ep.Key) {
		all = false
	}
	st.current = st.linked && all
	return st
}

// dshKeyValue is the credential EZLM writes: the server key, or a
// placeholder for a server without one (llm-pi-ai refuses a route whose
// credential resolves to nothing).
func dshKeyValue(key string) string {
	if key == "" {
		return "no-key"
	}
	return key
}

func (dshTool) Resolve(ctx context.Context, r *Run, install bool) (Found, error) {
	pin, _ := PinFor("dsh")
	usable, other := dshCopies()
	var c dshCopy
	var node string
	for _, u := range usable {
		n, err := dshNodeFor(ctx, r, u, false)
		if err == nil {
			c, node = u, n
			break
		}
		r.Logf("dsh %s (%s): %v", u.Bin, u.Version, err)
	}
	if c.Bin == "" && len(usable) > 0 && install {
		// A tested dsh whose Node is too old: give it EZLM's Node.
		if n, err := dshNodeFor(ctx, r, usable[0], true); err == nil {
			c, node = usable[0], n
		}
	}
	if c.Bin == "" {
		if !install {
			if len(usable) > 0 {
				return Found{}, fmt.Errorf("DeepSeek Harness needs Node.js %s or newer. Press Connect to let EZLM install it", dshNodeMin)
			}
			return Found{}, ErrNotInstalled
		}
		if other != nil {
			r.Logf("found dsh %s at %s; EZLM tests %s, so it installs its own copy", other.Version, other.Bin, pin.Version)
		}
		var err error
		if c, node, err = dshInstall(ctx, r, pin); err != nil {
			return Found{}, err
		}
	}
	if install {
		dshInitProfiles(ctx, r, node, c.Script)
	}
	f := Found{Bin: c.Bin, Version: c.Version, Source: c.Source, Arch: BinaryArch(node)}
	SaveProbe(c.Bin, Probe{Version: c.Version, Arch: f.Arch, Facts: map[string]string{"node": node, "script": c.Script}})
	r.Logf("dsh %s at %s runs on %s", c.Version, c.Script, node)
	return f, nil
}

// dshNodeFor picks the node a dsh runs on: the one EZLM's copy was installed
// with; for the user's own dsh, the node their PATH gives it, then EZLM's.
// With install it installs EZLM's pinned Node when none of those is new
// enough.
func dshNodeFor(ctx context.Context, r *Run, c dshCopy, install bool) (string, error) {
	cands := []string{nodeFind()}
	if c.Node != "" {
		cands = []string{c.Node, nodeFind()}
	}
	if n, _ := nodePick(ctx, r, dshNodeMin, cands...); n != "" {
		return n, nil
	}
	if !install {
		return "", fmt.Errorf("no Node.js %s or newer", dshNodeMin)
	}
	return nodeInstall(ctx, r)
}

// dshInstall installs the pinned dsh into ~/.ezlm/tools/dsh/<version> with
// npm (the user's when their Node is new enough, else EZLM's pinned Node).
func dshInstall(ctx context.Context, r *Run, pin Pin) (dshCopy, string, error) {
	node, _ := nodePick(ctx, r, dshNodeMin, nodeFind())
	npm := ""
	if node != "" {
		npm = nodeNpmCLI(node)
	}
	if npm == "" {
		var err error
		if node, err = nodeInstall(ctx, r); err != nil {
			return dshCopy{}, "", err
		}
		if npm = nodeNpmCLI(node); npm == "" {
			return dshCopy{}, "", errors.New("EZLM's Node.js has no npm. Press Connect to install it again")
		}
	}
	nodeVer, _ := nodeVersionOf(ctx, r, node)
	if err := CheckDisk(ToolsDir(), 1<<30, "DeepSeek Harness"); err != nil {
		return dshCopy{}, "", err
	}
	prefix := dshPrefix(pin.Version)
	os.RemoveAll(prefix) // an earlier attempt that didn't finish
	if err := os.MkdirAll(prefix, 0o755); err != nil {
		return dshCopy{}, "", err
	}
	r.Step(fmt.Sprintf("Installing DeepSeek Harness %s…", pin.Version))
	// --package-lock overrides a user's package-lock=false: the lock is how
	// EZLM checks what npm installed (dshCheckIntegrity).
	cmd := exec.Command(node, npm, "install", "--prefix", prefix, "--no-audit", "--no-fund", "--omit=dev",
		"--package-lock=true", "--foreground-scripts", pin.Package+"@"+pin.Version)
	cmd.Dir = prefix
	cmd.Env = append(dshEnv(dshHome(), node), "CI=1", "NONINTERACTIVE=1", "npm_config_update_notifier=false")
	if err := r.Install(ctx, cmd, 20*time.Minute); err != nil {
		if e, ok := Explain("dsh", err.Error()); ok {
			return dshCopy{}, "", errors.New(e.Say)
		}
		return dshCopy{}, "", err
	}
	if err := dshCheckIntegrity(prefix, pin); err != nil {
		r.Logf("%v", err)
		os.RemoveAll(prefix)
		return dshCopy{}, "", fmt.Errorf("The DeepSeek Harness download didn't match the version EZLM tested; press Connect to try again (details are in %s)", r.LogPath())
	}
	script := filepath.Join(prefix, "node_modules", "@deepseek-ai", "dsh", "lib", "bin.js")
	out, err := r.Exec(ctx, time.Minute, dshCommand(node, script, dshHome(), "--version"))
	if err != nil || nodeLastLine(out) != pin.Version {
		return dshCopy{}, "", fmt.Errorf("DeepSeek Harness installed but didn't start (%s). Details are in %s", ErrLine(out), r.LogPath())
	}
	m, _ := json.MarshalIndent(dshMarkerFile{Version: pin.Version, Node: node, NodeVersion: nodeVer}, "", "  ")
	if err := WriteFileAtomic(dshMarker(prefix), append(m, '\n'), 0o644); err != nil {
		return dshCopy{}, "", err
	}
	c, ok := dshEZLMCopy(pin.Version)
	if !ok {
		return dshCopy{}, "", fmt.Errorf("DeepSeek Harness installed but EZLM can't find its program. Details are in %s", r.LogPath())
	}
	r.Logf("installed dsh %s at %s", pin.Version, prefix)
	return c, node, nil
}

// dshCheckIntegrity compares the integrity npm recorded for the dsh package
// in the prefix's package-lock.json with the pin's (tools.json). npm checks
// the download against the registry's own record; this also catches a
// registry that serves a different tarball for the pinned version. A pin
// without an integrity passes; a missing lock file fails (the install asks
// npm for one, so a missing one means EZLM can't tell what it got).
func dshCheckIntegrity(prefix string, pin Pin) error {
	if pin.Integrity == "" {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(prefix, "package-lock.json"))
	if err != nil {
		return fmt.Errorf("dsh install: npm wrote no package-lock.json, so the download can't be checked: %v", err)
	}
	var lock struct {
		Packages map[string]struct {
			Version   string `json:"version"`
			Integrity string `json:"integrity"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(b, &lock); err != nil {
		return fmt.Errorf("dsh install: package-lock.json is unreadable: %v", err)
	}
	e, ok := lock.Packages["node_modules/"+pin.Package]
	if !ok {
		return fmt.Errorf("dsh install: package-lock.json has no entry for %s", pin.Package)
	}
	if e.Version != pin.Version || e.Integrity != pin.Integrity {
		return fmt.Errorf("dsh install: npm installed %s %s with integrity %s; the pin is %s %s", pin.Package, e.Version, e.Integrity, pin.Version, pin.Integrity)
	}
	return nil
}

// dshInitProfiles lets dsh create the headless and web profiles from its own
// templates when they don't exist yet (a boot-free config dump does that),
// so EZLM only ever adds to dsh's files.
func dshInitProfiles(ctx context.Context, r *Run, node, script string) {
	home := dshHome()
	for _, p := range []string{"headless", "web"} {
		if _, err := os.Stat(filepath.Join(dshProfileDir(home, p), "package.json")); err == nil {
			continue
		}
		if _, err := r.Exec(ctx, time.Minute, dshCommand(node, script, home, "--profile", p, "--dump-default-config")); err != nil {
			r.Logf("dsh couldn't set up its %s profile; EZLM writes the patch file and dsh sets up the rest when it starts: %v", p, err)
		}
	}
}

// dshEnv is the environment of a dsh process EZLM runs: telemetry off,
// DSH_HOME set, and node's folder first on PATH for anything dsh starts by
// name.
func dshEnv(home, node string) []string {
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		switch strings.ToUpper(k) {
		case "DSH_HOME", "DSH_TELEMETRY_MODE", "DSH_TELEMETRY_DISABLED":
			continue
		case "PATH":
			kv = k + "=" + filepath.Dir(node) + string(os.PathListSeparator) + kv[len(k)+1:]
		}
		env = append(env, kv)
	}
	return append(append(env, "DSH_HOME="+home), dshTelemetryEnv...)
}

// dshCommand is `node lib/bin.js args...` for the given DSH_HOME.
func dshCommand(node, script, home string, args ...string) *exec.Cmd {
	cmd := exec.Command(node, append([]string{script}, args...)...)
	cmd.Env = dshEnv(home, node)
	return cmd
}

// dshRuntime is the node and lib/bin.js Resolve chose for f.
func dshRuntime(f Found) (node, script string, err error) {
	if p, ok := CachedProbe(f.Bin); ok {
		node, script = p.Facts["node"], p.Facts["script"]
	}
	if script == "" {
		script = dshScriptFor(f.Bin)
	}
	if node == "" {
		if c, ok := dshEZLMCopy(f.Version); ok && c.Bin == f.Bin {
			node = c.Node
		} else {
			node = nodeFind()
		}
	}
	if script == "" {
		return "", "", errors.New("EZLM can't find DeepSeek Harness's program files. Press Connect to install it again")
	}
	if node == "" {
		return "", "", fmt.Errorf("DeepSeek Harness needs Node.js %s or newer. Press Connect to let EZLM install it", dshNodeMin)
	}
	return node, script, nil
}

func (dshTool) Configure(ep Endpoint, makeDefault bool) error {
	return WriteDshConfig(dshHome(), ep, makeDefault)
}

// WriteDshConfig writes EZLM's connection into a DeepSeek Harness home: the
// key first (so a key rotation lands even if a patch file can't be edited),
// then the stored ref if dsh has one, then every managed profile.
func WriteDshConfig(home string, ep Endpoint, makeDefault bool) error {
	key := dshKeyValue(ep.Key)
	errs := []error{WriteDshEnv(home, key), UpdateDshStoredRef(home, key)}
	for _, p := range dshManagedProfiles(home) {
		_, err := WriteDshProfile(dshPatchPath(home, p), ep, makeDefault)
		errs = append(errs, err)
	}
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
