package integrate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"time"
)

// OpenCode (https://opencode.ai). EZLM reuses an OpenCode already on the
// machine when the probe in opencode_detect.go says it can run EZLM's model
// against the user's chat history, and otherwise installs its pinned 2.2.5
// into ~/.ezlm/tools/opencode/2.2.5 and launches that by absolute path. The
// config writer is in opencode_config.go, the pinned install in
// opencode_install.go and the check and preflight in opencode_check.go.

func init() {
	Register(openCodeTool{})
	// The pinned OpenCode 2.2.5 (all 12 release assets, digests and sizes)
	// is in tools.json; PinFor("opencode") reads it.

	// Texts seen from OpenCode 1.19.5 and 2.2.5 (testdata/opencode/*.jsonl);
	// the common ones (401, connection refused, model not found) are in
	// check.go.
	RegisterErrorText("opencode",
		ErrorText{Match: "Database is not empty and has no session table", Say: "OpenCode 1.x can't open the chat history a newer OpenCode wrote; press Connect and EZLM will use OpenCode 2.2.5."},
		ErrorText{Match: "Failed to initialize OpenTUI", Say: "This OpenCode build can't draw its window on this computer; press Connect and EZLM will use OpenCode 2.2.5."},
		ErrorText{Match: "Unrecognized flag", Say: "This OpenCode version doesn't take the options EZLM uses; press Connect and EZLM will use OpenCode 2.2.5."},
		ErrorText{Match: "Variant unavailable", Say: "OpenCode asks for a thinking level this model doesn't offer. Press Connect to update it.", State: StateNeedsUpdate},
		ErrorText{Match: "Model unavailable", Say: "OpenCode asks for a model EZLM isn't serving. Press Connect to update it.", State: StateNeedsUpdate},
		ErrorText{Match: "Unexpected reasoning effort", Say: "The model rejected OpenCode's thinking level. Press Connect so EZLM sets the levels this model accepts.", State: StateNeedsUpdate},
		ErrorText{Match: "Unable to connect. Is the computer able to access the url", Say: "EZLM's model server isn't running. Start it, then try again."},
	)
}

// openCodeProvider is EZLM's provider id in OpenCode's config; models are
// "ezlm/<id>".
const openCodeProvider = "ezlm"

// openCodeReady is what both OpenCode TUIs (1.19.5 and 2.2.5) draw in their
// footer once EZLM's model is selected: the provider's name.
const openCodeReady = "EZLM (local)"

type openCodeTool struct{}

func (openCodeTool) Info() Info {
	return Info{ID: "opencode", Name: "OpenCode", Desc: "Coding agent for your terminal", Icon: "opencode", Order: 10,
		Launch: []LaunchMode{{ID: ModeTUI, Label: "Open OpenCode"}}}
}

// Detect picks the program from cached probes only (Resolve fills them) and
// compares EZLM's config entry with ep.
func (openCodeTool) Detect(ep *Endpoint) Detection {
	dir := openCodeConfigDir()
	d := Detection{Config: openCodeConfigFile(dir)}
	c := openCodeChoose(nil, nil, openCodeDBFormatCached)
	d.Found = c.found
	if c.pinned && d.Bin == "" {
		d.Detail = c.note
	}
	cfg, err := readOpenCodeConfig(d.Config)
	if err != nil {
		d.State, d.Detail = StateNeedsUpdate, err.Error()
		return d
	}
	d.Linked = cfg.linked()
	switch {
	case ep != nil && !ep.ToolCalls:
		d.State, d.Detail = StateNeedsTools, openCodeNoToolsMsg(*ep)
	case ep != nil && d.Linked:
		if why := cfg.stale(*ep, dir); why != "" {
			d.State, d.Detail = StateNeedsUpdate, why
		}
	}
	return d
}

// Resolve probes the user's opencode (version, architecture, flags) and the
// chat history's format, and decides between it and EZLM's pinned copy
// (PLAN 4.2). It installs the pinned copy when that is the choice and
// install is set.
func (openCodeTool) Resolve(ctx context.Context, r *Run, install bool) (Found, error) {
	c := openCodeChoose(ctx, r, openCodeDBFormat)
	r.Logf("opencode: data %q, user %q, choice %s %q", c.data, c.user, map[bool]string{true: "pinned", false: "user"}[c.pinned], c.found.Bin)
	if !c.pinned {
		return c.found, nil
	}
	if c.note != "" {
		r.Step(c.note)
	}
	if c.found.Bin != "" {
		return c.found, nil
	}
	if !install {
		return Found{}, ErrNotInstalled
	}
	return openCodeInstall(ctx, r)
}

func (openCodeTool) Configure(ep Endpoint, makeDefault bool) error {
	if !ep.ToolCalls {
		return errors.New(openCodeNoToolsMsg(ep))
	}
	if err := WriteOpenCodeConfig(openCodeConfigDir(), ep, makeDefault); err != nil {
		return err
	}
	return writeOpenCodeWorkspace(ep)
}

func (openCodeTool) Check(ctx context.Context, f Found, ep Endpoint, w *CheckWorkspace, r *Run) (bool, error) {
	return openCodeCheck(ctx, f, ep, w, r)
}

func (openCodeTool) Preflight(ctx context.Context, f Found, ep Endpoint, r *Run) error {
	return openCodePreflight(ctx, f, ep, r)
}

// LaunchSpec opens the TUI in the workspace. OpenCode 2.x runs with
// --standalone, so it reads the config EZLM just wrote rather than a
// background service that loaded an older one; its TUI has no --model, so
// the workspace's opencode.json picks EZLM's model. 1.x takes --model.
func (openCodeTool) LaunchSpec(f Found, ep Endpoint, mode string) (LaunchSpec, error) {
	if mode != ModeTUI {
		return LaunchSpec{}, fmt.Errorf("OpenCode has no %q mode.", mode)
	}
	if err := writeOpenCodeWorkspace(ep); err != nil {
		return LaunchSpec{}, err
	}
	fl := openCodeFlagsOf(f)
	var args []string
	if fl.standalone {
		args = append(args, "--standalone")
	}
	if fl.tuiModel {
		args = append(args, "--model", openCodeProvider+"/"+ep.ModelID)
	}
	return LaunchSpec{Bin: f.Bin, Args: args, Env: openCodeLaunchEnv(f), Dir: Workspace(),
		Ready: []string{openCodeReady},
		Fatal: []string{"Database is not empty", "Unrecognized flag", "Model unavailable", "Variant unavailable", "ProviderModelNotFoundError"},
	}, nil
}

func openCodeNoToolsMsg(ep Endpoint) string {
	name := ep.ModelName
	if name == "" {
		name = "This model"
	}
	return name + " can't use tools, so OpenCode can't edit files with it. Pick a model marked Tools, or use Chat."
}

// openCodeLaunchEnv is what the TUI needs on top of the terminal's own
// environment. A Mac Terminal window doesn't inherit EZLM's environment, so
// folders EZLM was pointed at explicitly are passed on, and EZLM's pinned
// copy never updates itself (that would replace the checked binary).
func openCodeLaunchEnv(f Found) []string {
	// OpenCode takes its working folder from $PWD (see openCodeCmd).
	env := append(append([]string{}, f.Env...), "PWD="+Workspace())
	if os.Getenv("OPENCODE_CONFIG_DIR") != "" || os.Getenv("XDG_CONFIG_HOME") != "" {
		env = append(env, "OPENCODE_CONFIG_DIR="+openCodeConfigDir())
	}
	for _, k := range []string{"XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME"} {
		if v := os.Getenv(k); v != "" {
			env = append(env, k+"="+v)
		}
	}
	return env
}

// openCodeCmd builds a command for f with the inherited environment, f.Env
// and extra on top (later values win), and no stdin: `opencode run` reads a
// piped stdin as part of the message and waits for it to close. OpenCode
// takes its working folder from $PWD, not the process's own, so PWD is set
// to dir too.
func openCodeCmd(f Found, dir string, extra []string, args ...string) *exec.Cmd {
	cmd := exec.Command(f.Bin, args...)
	cmd.Dir = dir
	cmd.Env = append(append(append(os.Environ(), f.Env...), "OPENCODE_DISABLE_AUTOUPDATE=1"), extra...)
	if dir != "" {
		cmd.Env = append(cmd.Env, "PWD="+dir)
	}
	return cmd
}

// openCodeTimeout bounds every probe that should answer at once.
const openCodeTimeout = 20 * time.Second

// openCodeExe is the program's file name in a release archive.
func openCodeExe() string {
	if runtime.GOOS == "windows" {
		return "opencode.exe"
	}
	return "opencode"
}
