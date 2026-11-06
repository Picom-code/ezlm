package integrate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"
)

func init() {
	Register(hermesTool{})
	RegisterErrorText("hermes",
		ErrorText{Match: "below the minimum 64,000", Say: hermes64KText, State: StateNeeds64K},
		ErrorText{Match: "Unexpected reasoning effort", Say: "Your model rejected the reasoning level Hermes asked for. Press Connect so EZLM sets one the model accepts.", State: StateNeedsUpdate},
		ErrorText{Match: "no dependency environment is committed", Say: "Hermes hasn't finished installing. Run hermes pm repair in a terminal, then press Connect again."},
		ErrorText{Match: "hermes pm repair", Say: "Hermes hasn't finished installing. Run hermes pm repair in a terminal, then press Connect again."},
		ErrorText{Match: "no longer exists. Switch back with", Say: "Hermes's saved profile is gone. Run hermes profile use default in a terminal, then press Connect again."},
		ErrorText{Match: "Connection error", Say: "EZLM's model server isn't running. Start it, then try again."},
		ErrorText{Match: "Unknown provider 'custom:ezlm'", Say: "Hermes doesn't see EZLM's provider. Press Connect to add it again.", State: StateNeedsUpdate},
		ErrorText{Match: "Hermes isn't configured yet", Say: "Hermes has no default model yet. Press Connect to make EZLM's model its default."},
	)
}

// Hermes Agent, from https://github.com/NousResearch/hermes-agent. Both
// installers skip their interactive stages (setup wizard, gateway service)
// with the non-interactive switch; Hermes adds its own bin folder to PATH.
var (
	hermesInstallUnix    = "curl -fsSL https://hermes-agent.nousresearch.com/install.sh | bash -s -- --non-interactive"
	hermesInstallWindows = "$ErrorActionPreference = 'Stop'; $global:LASTEXITCODE = 0; " +
		"& ([scriptblock]::Create((Invoke-RestMethod https://hermes-agent.nousresearch.com/install.ps1))) -NonInteractive; " +
		"exit $global:LASTEXITCODE"
	hermesInstallEnv = []string{"HERMES_NONINTERACTIVE=1", "HERMES_GATEWAY_INSTALL_START_ON_LOGIN=0"}
)

const (
	hermesProvider = "custom:ezlm" // what --provider and model.provider name
	hermesMinCtx   = 64000         // Hermes's MINIMUM_CONTEXT_LENGTH
	hermesProbeAge = 24 * time.Hour
	hermes64KText  = "Hermes needs a 64K context. Connect again to give the model the room (it may run slower), or pick a smaller model."
	hermesToolText = "This model can't use tools, so Hermes can't edit files with it. Pick a model marked Tools, or use Chat."
)

// hermesReady is what Hermes shows once it waits for the user: the chat
// prompt, or on a fresh home the one-time shared-metrics question that comes
// first. hermesFatal is what it shows when it won't start.
var (
	hermesReady = []string{"Help improve Hermes?", "Welcome to Hermes Agent", "Type your message", "Ask me anything", "/help for commands", "❯"}
	hermesFatal = []string{"below the minimum 64,000", "Traceback (most recent call last)", "hermes pm repair", "no longer exists. Switch back with", "Let's set up Hermes", "Unknown provider"}
)

type hermesTool struct{}

func (hermesTool) Info() Info {
	return Info{ID: "hermes", Name: "Hermes", Desc: "Personal agent with tools and memory", Icon: "hermes", Order: 20, MinCtx: hermesMinCtx,
		Launch: []LaunchMode{{ID: ModeTUI, Label: "Open Hermes"}}}
}

func (hermesTool) Detect(ep *Endpoint) Detection {
	var d Detection
	d.Bin, d.Source = hermesFind()
	d.Config = hermesConfigPath(d.Bin)
	if d.Bin != "" {
		if p, ok := CachedProbe(d.Bin); ok {
			d.Version, d.Arch = p.Version, p.Arch
		}
	}
	st := hermesReadState(d.Config)
	d.Linked = st.linked
	if !d.Linked || ep == nil {
		return d
	}
	if !ep.ToolCalls {
		d.State, d.Detail = StateNeedsTools, hermesToolText
	} else if why := st.stale(*ep); why != "" {
		d.State, d.Detail = StateNeedsUpdate, why
	}
	return d
}

func (hermesTool) Resolve(ctx context.Context, r *Run, install bool) (Found, error) {
	bin, src := hermesFind()
	if bin == "" {
		if !install {
			return Found{}, ErrNotInstalled
		}
		if err := hermesInstall(ctx, r); err != nil {
			return Found{}, err
		}
		if bin, src = hermesFind(); bin == "" {
			return Found{}, hermesArmErr(errors.New("Hermes's installer finished, but EZLM can't find the hermes program. Open a new terminal, run hermes --version, then press Connect again"), r)
		}
	}
	p, err := hermesProbe(ctx, r, bin, false)
	if err != nil {
		return Found{}, err
	}
	return Found{Bin: bin, Version: p.Version, Source: src, Arch: p.Arch, Env: hermesEnv()}, nil
}

// hermesEnv is what a hermes process EZLM starts needs on top of EZLM's own
// environment: a HERMES_HOME that only the user's shell files set, so every
// command EZLM runs reads the config EZLM wrote.
func hermesEnv() []string {
	if os.Getenv("HERMES_HOME") != "" {
		return nil
	}
	if h := LoginEnv("HERMES_HOME"); h != "" {
		return []string{"HERMES_HOME=" + h}
	}
	return nil
}

func (hermesTool) Configure(ep Endpoint, makeDefault bool) error {
	bin, _ := hermesFind()
	return WriteHermesConfig(hermesConfigPath(bin), ep, makeDefault)
}

// Check asks Hermes, in a throwaway folder, to read the check file with its
// file tool. It uses the user's real config (what the TUI uses), but skips
// AGENTS.md, SOUL.md and memory, offers only the file tools and tags the
// session as a tool's so it stays out of the user's session lists.
func (hermesTool) Check(ctx context.Context, f Found, ep Endpoint, w *CheckWorkspace, r *Run) (bool, error) {
	p, _ := CachedProbe(f.Bin)
	cmd := exec.Command(f.Bin, hermesCheckArgs(p.Facts, w.Prompt(), ep.ModelID)...)
	cmd.Dir = w.Dir
	cmd.Env = append(append(os.Environ(), f.Env...), "HERMES_NONINTERACTIVE=1")
	out, err := r.Exec(ctx, 10*time.Minute, cmd)
	res := hermesParseStream(out)
	answer := out
	if res.sawResult {
		answer = res.text
	}
	if w.Passed(answer) {
		return res.toolCall, nil // the nonce is the proof, whatever the exit code
	}
	if e, ok := Explain("hermes", out); ok {
		return res.toolCall, errors.New(e.Say)
	}
	if err == nil && strings.TrimSpace(answer) != "" {
		return res.toolCall, errors.New("Hermes answered without the check file's text, so the model didn't use its file tool. Press Connect to try again, or pick a model marked Tools")
	}
	line := res.err
	if line == "" {
		line = ErrLine(out)
	}
	return res.toolCall, fmt.Errorf("Hermes couldn't finish the check (%s). Press Connect to try again", strings.TrimSuffix(line, "."))
}

// Preflight runs the program Open runs, asks it which config it reads, and
// checks that config holds EZLM's current entry. It starts no chat.
func (hermesTool) Preflight(ctx context.Context, f Found, ep Endpoint, r *Run) error {
	if ep.Ctx < hermesMinCtx {
		return errors.New(hermes64KText)
	}
	cmd := exec.Command(f.Bin, "--version")
	cmd.Env = append(os.Environ(), f.Env...)
	if out, err := r.Exec(ctx, 2*time.Minute, cmd); err != nil {
		if e, ok := Explain("hermes", out); ok {
			return errors.New(e.Say)
		}
		return fmt.Errorf("Hermes doesn't start (%s). Press Connect to repair it", strings.TrimSuffix(ErrLine(out), "."))
	}
	wrote := hermesConfigPath(f.Bin)
	reads, err := hermesAskConfig(ctx, r, f)
	if err != nil {
		return err
	}
	if reads != "" && !hermesSamePath(reads, wrote) {
		hermesSaveConfigFact(f.Bin, reads)
		return fmt.Errorf("Hermes reads %s, but EZLM's entry is in %s. Press Connect to move it", reads, wrote)
	}
	st := hermesReadState(wrote)
	if !st.linked {
		return errors.New("Hermes's settings don't have EZLM's provider yet. Press Connect to add it")
	}
	if why := st.stale(ep); why != "" {
		return errors.New(why)
	}
	return nil
}

// LaunchSpec opens Hermes's chat on EZLM's model. It honours the user's
// display.interface (the classic REPL or the modern TUI).
func (hermesTool) LaunchSpec(f Found, ep Endpoint, mode string) (LaunchSpec, error) {
	if ep.Ctx < hermesMinCtx {
		return LaunchSpec{}, errors.New(hermes64KText)
	}
	return LaunchSpec{Bin: f.Bin, Args: []string{"chat", "--provider", hermesProvider, "-m", ep.ModelID}, Env: f.Env,
		Ready: hermesReady, Fatal: hermesFatal}, nil
}

// hermesFind is the hermes program the user's shell would run and where it
// came from: FindTool's search (PATH, the login shell's or registry PATH,
// which an installer changes without telling running programs), Hermes's
// own bin folders, then the Hermes Agent app's alias in WindowsApps, which
// FindTool searches last.
func hermesFind() (bin, source string) {
	bin = FindTool("hermes", hermesDirs()...)
	if bin == "" {
		return "", ""
	}
	return bin, hermesSource(bin)
}

// hermesDirs are the folders Hermes's installers put the hermes command in.
func hermesDirs() []string {
	home, _ := os.UserHomeDir()
	var dirs []string
	if runtime.GOOS == "windows" {
		dirs = append(dirs, filepath.Join(hermesNativeHome(), "bin"))
		if h := hermesHomeEnv(); h != "" {
			dirs = append(dirs, filepath.Join(h, "bin"))
		}
	}
	for _, root := range []string{hermesHomeEnv(), filepath.Join(home, ".hermes")} {
		if root != "" {
			dirs = append(dirs, filepath.Join(root, "hermes-agent", ".hermes", "bin"))
		}
	}
	return dirs
}

// hermesSource tells the Hermes Agent app (MSIX) from a script install.
func hermesSource(bin string) string {
	if IsAppAlias(bin) {
		return SourceMSIX
	}
	if strings.Contains(strings.ToLower(filepath.ToSlash(bin)), "/hermes-agent/.hermes/bin/") {
		return SourceScript
	}
	if runtime.GOOS == "windows" {
		dir := filepath.Dir(bin)
		if hermesSamePath(dir, filepath.Join(hermesNativeHome(), "bin")) || (hermesHomeEnv() != "" && hermesSamePath(dir, filepath.Join(hermesHomeEnv(), "bin"))) {
			return SourceScript
		}
	}
	// The script install's ~/.local/bin/hermes is a two-line shim into
	// ~/.hermes/hermes-agent.
	if st, err := os.Stat(bin); err == nil && st.Size() < 4096 {
		if b, err := os.ReadFile(bin); err == nil && strings.Contains(string(b), "hermes-agent") {
			return SourceScript
		}
	}
	return SourcePath
}

// hermesInstall runs Hermes's own installer without its interactive stages.
// It never sets up the gateway; if the installer did anyway, the user is
// told and it is left as it is.
func hermesInstall(ctx context.Context, r *Run) error {
	home, _ := os.UserHomeDir()
	if err := CheckDisk(home, 2<<30, "Hermes"); err != nil {
		return err
	}
	r.Step("Installing Hermes…")
	before := hermesLoginItems(ctx, r)
	script := hermesInstallUnix
	if runtime.GOOS == "windows" {
		script = hermesInstallWindows
	}
	cmd := ShellCommand(script)
	cmd.Env = append(cmd.Env, hermesInstallEnv...)
	err := r.Install(ctx, cmd, 30*time.Minute)
	for _, it := range hermesLoginItems(ctx, r) {
		if !slices.Contains(before, it) {
			r.Logf("the installer added a gateway login item: %s", it)
			r.Step("Hermes's installer set its gateway to start at login. EZLM left it as it is; run hermes gateway uninstall to remove it.")
			break
		}
	}
	if err == nil {
		return nil
	}
	if bin, _ := hermesFind(); bin != "" {
		// The probe that follows decides: a published command that runs is
		// an install that worked, whatever the installer's last exit code.
		r.Logf("the installer reported %v, but %s exists; probing it", err, bin)
		return nil
	}
	return hermesArmErr(err, r)
}

// hermesArmErr explains an install that failed on Windows on Arm, where
// Hermes's script installer has open bugs and its app is the reliable way.
func hermesArmErr(err error, r *Run) error {
	if runtime.GOOS != "windows" || MachineArch() != "arm64" {
		return err
	}
	return fmt.Errorf("Hermes's installer didn't finish on Windows on Arm (%s). Install the Hermes Agent app from hermes-agent.nousresearch.com, then press Connect again. Details are in %s", strings.TrimSuffix(err.Error(), "."), r.LogPath())
}

// hermesLoginItems lists the Hermes gateway services that start at login,
// so an install can be checked for one it added.
func hermesLoginItems(ctx context.Context, r *Run) []string {
	home, _ := os.UserHomeDir()
	var globs []string
	switch runtime.GOOS {
	case "darwin":
		globs = []string{filepath.Join(home, "Library", "LaunchAgents", "ai.hermes.gateway*.plist")}
	case "windows":
		startup := filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs", "Startup")
		globs = []string{filepath.Join(startup, "Hermes_Gateway*.vbs"), filepath.Join(startup, "Hermes_Gateway*.cmd")}
	default:
		cfg := os.Getenv("XDG_CONFIG_HOME")
		if cfg == "" {
			cfg = filepath.Join(home, ".config")
		}
		globs = []string{filepath.Join(cfg, "systemd", "user", "hermes-gateway*.service")}
	}
	var items []string
	for _, g := range globs {
		m, _ := filepath.Glob(g)
		items = append(items, m...)
	}
	return append(items, hermesScheduledTasks(ctx, r)...)
}

// hermesProbe runs the program once for what Detect may not: the config
// file it reads (`hermes config path`, which also finishes a fresh data
// folder's setup), its version and the check flags its chat command takes.
// The result is cached by file and environment, and redone daily because a
// `hermes update` doesn't change the launcher file.
func hermesProbe(ctx context.Context, r *Run, bin string, fresh bool) (Probe, error) {
	env := hermesEnvKey()
	if p, ok := CachedProbe(bin); ok && !fresh && p.Version != "" && p.Facts["env"] == env && time.Since(p.At) < hermesProbeAge {
		return p, nil
	}
	run := func(timeout time.Duration, args ...string) (string, error) {
		cmd := exec.Command(bin, args...)
		cmd.Env = append(os.Environ(), hermesEnv()...)
		return r.Exec(ctx, timeout, cmd)
	}
	p := Probe{Arch: BinaryArch(bin), Facts: map[string]string{"env": env}}
	if out, err := run(10*time.Minute, "config", "path"); err == nil {
		p.Facts["config"] = hermesPathLine(out)
	}
	out, err := run(2*time.Minute, "--version")
	if err != nil {
		if e, ok := Explain("hermes", out); ok {
			return Probe{}, errors.New(e.Say)
		}
		return Probe{}, fmt.Errorf("Hermes is installed but doesn't start (%s). Press Connect to repair it", strings.TrimSuffix(ErrLine(out), "."))
	}
	p.Version = hermesVersion(out)
	if out, err := run(time.Minute, "chat", "--help"); err == nil {
		p.Facts["chat"] = hermesFlags(out)
	} else if out, err := run(time.Minute, "--help"); err == nil {
		p.Facts["top"] = hermesFlags(out)
	}
	if err := SaveProbe(bin, p); err != nil {
		r.Logf("probe not cached: %v", err)
	}
	return p, nil
}

// hermesAskConfig asks the launch program for its config file and caches
// the answer; "" when it can't say.
func hermesAskConfig(ctx context.Context, r *Run, f Found) (string, error) {
	cmd := exec.Command(f.Bin, "config", "path")
	cmd.Env = append(os.Environ(), f.Env...)
	out, err := r.Exec(ctx, 2*time.Minute, cmd)
	if err != nil {
		if e, ok := Explain("hermes", out); ok {
			return "", errors.New(e.Say)
		}
		r.Logf("hermes config path failed; using Hermes's default rules")
		return "", nil
	}
	return hermesPathLine(out), nil
}

// hermesSaveConfigFact updates the cached config path of bin.
func hermesSaveConfigFact(bin, path string) {
	if p, ok := CachedProbe(bin); ok && p.Facts != nil {
		p.Facts["config"], p.Facts["env"] = path, hermesEnvKey()
		SaveProbe(bin, p)
	}
}

var hermesVersionRe = regexp.MustCompile(`v?(\d+\.\d+\.\d+)`)

// hermesVersion is "0.22.5" from "Hermes Agent v0.22.5+503.gb222bce (…)".
func hermesVersion(out string) string {
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(strings.ToLower(l), "hermes") {
			if m := hermesVersionRe.FindStringSubmatch(l); m != nil {
				return m[1]
			}
		}
	}
	if m := hermesVersionRe.FindStringSubmatch(out); m != nil {
		return m[1]
	}
	return ""
}

// hermesPathLine is the last output line naming a config.yaml (the command
// may print setup progress first).
func hermesPathLine(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if strings.HasSuffix(strings.ToLower(l), "config.yaml") && filepath.IsAbs(l) {
			return l
		}
	}
	return ""
}

// The check flags EZLM uses when a Hermes build has them.
var hermesCheckFlags = []string{"-q", "-Q", "--oneshot", "-t", "--ignore-rules", "--source", "--max-turns", "--format"}

// hermesFlags is the comma list of check flags a --help output mentions.
func hermesFlags(help string) string {
	var have []string
	for _, f := range hermesCheckFlags {
		if regexp.MustCompile(`(^|[\s\[,|])` + regexp.QuoteMeta(f) + `([\s\],=|]|$)`).MatchString(help) {
			have = append(have, f)
		}
	}
	return strings.Join(have, ",")
}

// hermesCheckArgs is the one-shot check command: `hermes chat -q` with the
// isolation flags this build has, or the older `hermes -z`.
func hermesCheckArgs(facts map[string]string, prompt, model string) []string {
	has := func(key, flag string) bool { return slices.Contains(strings.Split(facts[key], ","), flag) }
	if has("chat", "-q") {
		args := []string{"chat", "-q", prompt}
		if has("chat", "-Q") {
			args = append(args, "-Q")
		} else if has("chat", "--oneshot") {
			args = append(args, "--oneshot")
		}
		args = append(args, "--provider", hermesProvider, "-m", model)
		for _, a := range [][]string{{"-t", "file"}, {"--ignore-rules"}, {"--source", "tool"}, {"--max-turns", "6"}, {"--format", "stream-json"}} {
			if has("chat", a[0]) {
				args = append(args, a...)
			}
		}
		return args
	}
	args := []string{"-z", prompt, "--provider", hermesProvider, "-m", model}
	if has("top", "-t") {
		args = append(args, "-t", "file")
	}
	if has("top", "--ignore-rules") {
		args = append(args, "--ignore-rules")
	}
	return args
}

// hermesStream is what a `--format stream-json` run said.
type hermesStream struct {
	sawResult bool
	text      string // the final answer
	err       string // the result's error, if any
	toolCall  bool   // a tool_use event appeared
}

// hermesParseStream reads Hermes's JSONL events (system/init, text,
// tool_use, tool_result, result); other lines (stderr) are skipped.
func hermesParseStream(out string) hermesStream {
	var s hermesStream
	var deltas strings.Builder
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		if !strings.HasPrefix(l, "{") {
			continue
		}
		var ev struct {
			Type  string `json:"type"`
			Text  string `json:"text"`
			Error string `json:"error"`
		}
		if json.Unmarshal([]byte(l), &ev) != nil {
			continue
		}
		switch ev.Type {
		case "text":
			deltas.WriteString(ev.Text)
		case "tool_use":
			s.toolCall = true
		case "result":
			s.sawResult, s.text, s.err = true, ev.Text, ev.Error
		}
	}
	if s.sawResult && s.text == "" {
		s.text = deltas.String()
	}
	return s
}

// hermesSamePath compares two paths the way the file system does.
func hermesSamePath(a, b string) bool {
	clean := func(p string) string {
		p = filepath.Clean(p)
		if r, err := filepath.EvalSymlinks(p); err == nil {
			p = r
		} else if d, err := filepath.EvalSymlinks(filepath.Dir(p)); err == nil {
			p = filepath.Join(d, filepath.Base(p))
		}
		if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
			p = strings.ToLower(p)
		}
		return p
	}
	return clean(a) == clean(b)
}
