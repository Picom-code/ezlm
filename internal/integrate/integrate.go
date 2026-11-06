// Package integrate registers the local endpoint as a provider in agent apps
// (OpenCode, Hermes, DeepSeek Harness), installing them first if needed.
//
// Each app is a Tool in its own files (opencode*.go, hermes*.go, dsh*.go)
// that registers itself from init(). This file runs the shared pipelines:
// Targets for the UI, Setup (install, configure, check), Rewrite, Preflight
// and Open. CONTRACT.md says who owns which file.
package integrate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Log receives human-readable progress lines.
type Log func(msg string)

var (
	// ErrNotInstalled: Resolve without install found no usable program.
	ErrNotInstalled = errors.New("not installed")
	// ErrNeeds64K: Configure refused a context below the tool's MinCtx.
	ErrNeeds64K = errors.New("needs a 64K context")
)

// Off: EZLM_NO_INTEGRATIONS keeps a test instance from touching the real
// OpenCode/Hermes/DeepSeek Harness settings on the machine.
func Off() bool { return os.Getenv("EZLM_NO_INTEGRATIONS") != "" }

// Workspace is the folder the apps open in when EZLM launches them: a place
// of their own, so a coding agent works on files there rather than across
// the user's home folder.
func Workspace() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "EZLM Workspace")
}

// Targets lists the integrations and their state. It runs on every Status
// poll, so it only parses configs and reads caches. ep is the deployed
// model (nil if none), which decides connected vs needs_update.
func Targets(ep *Endpoint) []Target {
	var out []Target
	for _, t := range All() {
		inf := t.Info()
		d := t.Detect(ep)
		tg := Target{
			ID: inf.ID, Name: inf.Name, Desc: inf.Desc, Icon: inf.Icon,
			Supported: d.Unsupported == "", Why: d.Unsupported,
			Installed: d.Bin != "", Version: d.Version, Bin: d.Bin, Source: d.Source,
			State: d.State, Detail: d.Detail, Config: d.Config, MinCtx: inf.MinCtx,
			Launch: inf.Launch, LastCheck: LastCheck(inf.ID), Linked: d.Linked,
		}
		tg.State, tg.Detail = deriveState(tg, ep)
		tg.Connected = tg.State == StateConnected
		if Off() {
			tg.Connected, tg.Linked, tg.Supported = false, false, false
			tg.Why = "Turned off for this test install"
		}
		out = append(out, tg)
	}
	return out
}

// deriveState fills in what the tool left to core: not_installed / installed
// / connected from Bin and Linked, needs_64k from MinCtx, and broken when the
// last check or launch failed.
func deriveState(t Target, ep *Endpoint) (string, string) {
	state, detail := t.State, t.Detail
	if !t.Installed {
		return StateNotInstalled, detail
	}
	if state == "" {
		state = StateInstalled
		if t.Linked {
			state = StateConnected
		}
	}
	if t.Linked && ep != nil && t.MinCtx > 0 && ep.Ctx < t.MinCtx {
		return StateNeeds64K, NeedsCtxDetail(t.Name, t.MinCtx)
	}
	if (state == StateConnected || state == StateInstalled) && t.LastCheck != nil && !t.LastCheck.OK {
		return StateBroken, t.LastCheck.Msg
	}
	return state, detail
}

// NeedsCtxDetail is the needs_64k sentence for an app with a context floor.
func NeedsCtxDetail(name string, minCtx int) string {
	return fmt.Sprintf("%s needs a %dK context. Connect again to give the model the room (it may run slower), or pick a smaller model.", name, minCtx/1000)
}

// Setup installs (if missing), configures and verifies one integration:
// the model-and-tool check, the launch preflight and, when the tool gives
// ready markers, the hidden TUI smoke test. The result is saved as the
// target's LastCheck.
func Setup(ctx context.Context, id string, ep Endpoint, makeDefault bool, log Log) error {
	if Off() {
		return fmt.Errorf("connecting apps is turned off for this test install")
	}
	t, ok := Lookup(id)
	if !ok {
		return fmt.Errorf("unknown integration %q", id)
	}
	inf := t.Info()
	r := NewRun(id, log, ep.Key)
	defer r.Close()
	r.Logf("setup %s: model %s, context %d, tools %v, reasoning levels %v (thinking %v), %s", id, ep.ModelID, ep.Ctx, ep.ToolCalls, ep.Reasoning.Levels, ep.Reasoning.Thinking, ep.BaseURL)
	res := CheckResult{At: time.Now(), LogPath: r.LogPath()}
	fail := func(stage string, err error) error {
		if errors.Is(err, ErrNeeds64K) {
			err = errors.New(NeedsCtxDetail(inf.Name, max(inf.MinCtx, 64000)))
		}
		res.Stage, res.Msg = stage, err.Error()
		r.Logf("%s failed: %s", stage, err)
		SaveCheck(id, res)
		return err
	}

	f, err := t.Resolve(ctx, r, true)
	if err != nil {
		return fail(StageModel, err)
	}
	r.Logf("using %s (version %q, source %s, arch %s)", f.Bin, f.Version, f.Source, f.Arch)
	r.Step(fmt.Sprintf("Adding EZLM as %s %s provider…", article(inf.Name), inf.Name))
	if err := t.Configure(ep, makeDefault); err != nil {
		return fail(StageModel, err)
	}
	r.Step("Testing " + inf.Name + " with your model…")
	w, err := NewCheckWorkspace()
	if err != nil {
		return fail(StageModel, err)
	}
	defer w.Close()
	start := time.Now()
	toolCall, err := t.Check(ctx, f, ep, w, r)
	res.Secs, res.ToolCall = time.Since(start).Seconds(), toolCall
	if err != nil {
		return fail(StageModel, err)
	}
	if err := t.Preflight(ctx, f, ep, r); err != nil {
		return fail(StagePreflight, err)
	}
	if len(inf.Launch) > 0 {
		if spec, err := t.LaunchSpec(f, ep, inf.Launch[0].ID); err == nil && len(spec.Ready) > 0 {
			fillSpec(&spec, inf, inf.Launch[0].ID)
			// The smoke runs where the launch will: the workspace, which
			// Open creates but a first Connect may not have yet.
			if err := os.MkdirAll(spec.Dir, 0o755); err != nil {
				return fail(StageTUI, err)
			}
			if holdsKey(spec, ep.Key) {
				return fail(StageTUI, keyInLauncher(inf.Name))
			}
			sm, err := Smoke(ctx, spec, 20*time.Second)
			r.Logf("TUI smoke: ok=%v alive=%v ready=%v fatal=%q code=%d\n%s", sm.OK, sm.Alive, sm.Ready, sm.Fatal, sm.Code, r.Redact(sm.Screen))
			if err == nil && !sm.OK {
				err = fmt.Errorf("%s didn't start in a terminal: %s", inf.Name, firstNonEmpty(sm.Fatal, "it showed no prompt within 20 s"))
			}
			if err != nil {
				return fail(StageTUI, err)
			}
		}
	}
	// An OK message continues "Checked 2 min ago: ", so it starts lowercase.
	res.OK, res.Stage = true, StageModel
	res.Msg = fmt.Sprintf("answered (%.0f s)", res.Secs)
	if toolCall {
		res.Msg = fmt.Sprintf("answered and used a tool (%.0f s)", res.Secs)
	}
	r.Logf("setup ok: %s", res.Msg)
	SaveCheck(id, res)
	return nil
}

// Rewrite updates an already-connected app's EZLM entry (e.g. after a key
// rotation) without installing or testing anything. The default model is
// left as the user set it.
func Rewrite(id string, ep Endpoint) error {
	if Off() {
		return nil
	}
	t, ok := Lookup(id)
	if !ok {
		return fmt.Errorf("unknown integration %q", id)
	}
	return t.Configure(ep, false)
}

// Preflight runs a tool's fast launch check (ezlm doctor apps, and Open when
// the config was just rewritten).
func Preflight(ctx context.Context, id string, ep Endpoint, log Log) error {
	if Off() {
		return errors.New("checking apps is turned off for this test install")
	}
	t, ok := Lookup(id)
	if !ok {
		return fmt.Errorf("unknown integration %q", id)
	}
	r := NewRun(id, log, ep.Key)
	defer r.Close()
	f, err := t.Resolve(ctx, r, false)
	if errors.Is(err, ErrNotInstalled) {
		return fmt.Errorf("%s isn't installed yet", t.Info().Name)
	}
	if err != nil {
		return err
	}
	return t.Preflight(ctx, f, ep, r)
}

// Open starts the app for mode ("" = its main launch mode), set to use
// EZLM's model. Watching for an early exit is the caller's job (see
// Launched.ExitFile).
func Open(ctx context.Context, id, mode string, ep Endpoint, log Log) (Launched, error) {
	if Off() {
		return Launched{}, errors.New("opening apps is turned off for this test install")
	}
	t, ok := Lookup(id)
	if !ok {
		return Launched{}, errors.New("unknown app " + id)
	}
	inf := t.Info()
	if mode == "" && len(inf.Launch) > 0 {
		mode = inf.Launch[0].ID
	}
	if err := os.MkdirAll(Workspace(), 0o755); err != nil {
		return Launched{}, err
	}
	r := NewRun(id, log, ep.Key)
	defer r.Close()
	f, err := t.Resolve(ctx, r, false)
	if errors.Is(err, ErrNotInstalled) {
		return Launched{}, fmt.Errorf("%s isn't installed yet", inf.Name)
	}
	if err != nil {
		return Launched{}, err
	}
	spec, err := t.LaunchSpec(f, ep, mode)
	if err != nil {
		return Launched{}, err
	}
	fillSpec(&spec, inf, mode)
	if holdsKey(spec, ep.Key) {
		return Launched{}, keyInLauncher(inf.Name)
	}
	r.Logf("open %s (%s): %s %s in %s", id, mode, spec.Bin, r.Redact(strings.Join(spec.Args, " ")), spec.Dir)
	l, err := Launch(spec)
	if err != nil {
		r.Logf("open failed: %s", err)
	}
	return l, err
}

// holdsKey reports whether a launch spec carries the server's key anywhere.
// Launch and Smoke refuse anything shaped like a key; this also catches a
// key of any other shape.
func holdsKey(s LaunchSpec, key string) bool {
	if key == "" {
		return false
	}
	for _, v := range append(append([]string{s.Bin, s.Dir, s.URL}, s.Args...), s.Env...) {
		if strings.Contains(v, key) {
			return true
		}
	}
	return false
}

func keyInLauncher(name string) error {
	return fmt.Errorf("EZLM won't put its key into the %s launcher; the app reads it from its own settings", name)
}

// fillSpec sets the fields every launch shares when the tool left them empty.
func fillSpec(s *LaunchSpec, inf Info, mode string) {
	s.ID, s.Mode = inf.ID, mode
	if s.Title == "" {
		s.Title = inf.Name + " (EZLM)"
	}
	if s.Dir == "" {
		s.Dir = Workspace()
	}
}

func article(name string) string {
	if name != "" && strings.ContainsRune("AEIOU", rune(name[0])) {
		return "an"
	}
	return "a"
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}
