package integrate

import (
	"context"
	"slices"
	"time"
)

// The types in this file are the contract every workstream codes against
// (see CONTRACT.md). Change them only through the core owner.

// Endpoint is what the writers, checks and launchers know about the deployed
// model.
type Endpoint struct {
	BaseURL   string // http://127.0.0.1:8080/v1
	Key       string // the server's main key; never logged or put in a launcher
	ModelID   string // the --alias id, e.g. qwen3.8-27b
	ModelName string // display name, e.g. Qwen3.8 27B
	Ctx       int    // context window per slot
	ToolCalls bool   // the catalog says the model can call tools
	Vision    bool   // an image projector is loaded (never, today)
	Reasoning Reasoning
}

// Reasoning is what the model's chat template accepts for thinking, read
// from llama-server's /props after each deploy. Writers never make a tool's
// default request carry a level that isn't in Levels; with Levels empty they
// leave reasoning settings out, so the default request sends none.
type Reasoning struct {
	Levels   []string // reasoning_effort values the template renders, lowest first
	Thinking bool     // the template reads chat_template_kwargs.enable_thinking
}

// Accepts reports whether the template takes this reasoning_effort value.
// "none" (thinking off) is accepted whenever the template knows any level.
func (r Reasoning) Accepts(level string) bool {
	if level == "none" {
		return len(r.Levels) > 0
	}
	return slices.Contains(r.Levels, level)
}

// Target states, in the order they take precedence.
const (
	StateNotInstalled = "not_installed"
	StateNeeds64K     = "needs_64k"    // the context is below the tool's MinCtx
	StateNeedsTools   = "needs_tools"  // the model can't call tools and the tool needs them
	StateNeedsUpdate  = "needs_update" // EZLM's entry points at an old URL, key, model or context
	StateBroken       = "broken"       // the last check or launch failed; Detail says why
	StateConnected    = "connected"
	StateInstalled    = "installed" // installed, no EZLM entry yet
)

// Where the program EZLM runs came from. The tiles say "installed by EZLM"
// for SourceEZLM and "yours" for every other value.
const (
	SourceEZLM   = "ezlm"   // EZLM's pinned copy under ~/.ezlm/tools
	SourceMSIX   = "msix"   // a Windows Store / MSIX app alias (Hermes Agent desktop)
	SourceScript = "script" // the app's own installer script
	SourceNPM    = "npm"    // a global npm install
	SourcePath   = "path"   // found on PATH or in a usual folder; origin unknown
)

// Launch mode ids.
const (
	ModeTUI = "tui" // the app's terminal UI in a new terminal window
	ModeWeb = "web" // the app's browser UI
)

// Target is one app EZLM can connect, as the UIs receive it in Status. Every
// field after Connected is optional on the wire, so the apps still decode an
// older engine's JSON.
type Target struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Desc      string       `json:"desc"`
	Supported bool         `json:"supported"` // false where the app can't run
	Why       string       `json:"why"`       // reason when unsupported
	Installed bool         `json:"installed"`
	Connected bool         `json:"connected"`            // kept for older UIs: State == connected
	Icon      string       `json:"icon,omitempty"`       // logo key: the apps show app-<icon>.png
	Version   string       `json:"version,omitempty"`    // "2.2.5"; "" until probed
	Bin       string       `json:"bin,omitempty"`        // absolute path EZLM launches
	Source    string       `json:"source,omitempty"`     // a Source* value
	State     string       `json:"state,omitempty"`      // a State* value
	Detail    string       `json:"detail,omitempty"`     // one sentence for the tile
	Config    string       `json:"config,omitempty"`     // the config file EZLM edits
	MinCtx    int          `json:"min_ctx,omitempty"`    // the tool refuses smaller contexts
	Launch    []LaunchMode `json:"launch,omitempty"`     // the first is the main button
	LastCheck *CheckResult `json:"last_check,omitempty"` // the newest check or launch result
	Linked    bool         `json:"-"`                    // the config has EZLM's entry: rewrite it when the endpoint changes
}

// LaunchMode is one way to open a target, shown as one button.
type LaunchMode struct {
	ID    string `json:"id"`    // ModeTUI | ModeWeb
	Label string `json:"label"` // the button text, e.g. "Open OpenCode"
}

// Check stages, in the order Setup runs them.
const (
	StageModel     = "model"     // the model-and-tool check
	StagePreflight = "preflight" // the launch preflight
	StageTUI       = "tui"       // the hidden TUI smoke test
	StageLaunch    = "launch"    // Open: the app exited right after opening
)

// CheckResult is the outcome of the newest check or launch of a target,
// cached in ~/.ezlm/integrations.json.
type CheckResult struct {
	At       time.Time `json:"at"`
	OK       bool      `json:"ok"`
	ToolCall bool      `json:"tool_call"`          // the app made a real tool call
	Secs     float64   `json:"secs"`               // how long the model check took
	Stage    string    `json:"stage,omitempty"`    // a Stage* value; on failure, the one that failed
	Msg      string    `json:"msg,omitempty"`      // one sentence; on failure, the reason
	LogPath  string    `json:"log_path,omitempty"` // ~/.ezlm/logs/integrate-<id>.log
}

// Info is what a tool says about itself; it doesn't change at run time.
type Info struct {
	ID     string // "opencode" | "hermes" | "dsh"
	Name   string // "OpenCode"
	Desc   string // one sentence for the tile
	Icon   string // logo key, normally the id
	Order  int    // tile order, lowest first
	MinCtx int    // the tool refuses to run below this context (0: no floor)
	Launch []LaunchMode
}

// Found is the program EZLM runs for a tool.
type Found struct {
	Bin     string   // absolute path; "" when not installed
	Version string   // "" until probed
	Source  string   // a Source* value
	Arch    string   // "arm64" | "amd64" | "" (unknown)
	Env     []string // KEY=VALUE the program needs on top of the inherited env (never keys)
}

// Detection is what a tool's cheap Detect sees.
type Detection struct {
	Found
	State       string // "" lets core derive not_installed / installed / connected from Bin and Linked
	Detail      string // one sentence for the tile, when State needs one
	Linked      bool   // the config has EZLM's entry, whatever URL, key or model it holds
	Config      string // the config file EZLM edits
	Unsupported string // non-empty: this OS or architecture can't run it, in plain words
}

// LaunchSpec is exactly what Open runs. The preflight and the TUI smoke use
// the same spec, so the check runs what the user gets.
type LaunchSpec struct {
	ID    string   // tool id; names the launcher and the exit file
	Mode  string   // ModeTUI | ModeWeb
	Title string   // window title, e.g. "OpenCode (EZLM)"
	Bin   string   // absolute path; a .cmd or .bat gets `call` on Windows
	Args  []string // passed as separate arguments, quoted by the launcher
	Env   []string // KEY=VALUE on top of the inherited environment (never keys or tokens)
	Dir   string   // working folder, normally Workspace()
	URL   string   // ModeWeb: the page the app serves
	Ready []string // TUI smoke: screen text that means the app is up; empty skips the smoke
	Fatal []string // TUI smoke: tool-specific fatal texts, on top of CommonFatal
}

// Launched is what Launch did.
type Launched struct {
	Launcher string // the script written (.command, .cmd or .sh); "" if none
	ExitFile string // where the launcher writes "<code> <unix-seconds>"; "" when it doesn't
	DryRun   bool   // EZLM_LAUNCH_DRYRUN: written, not opened
}

// SmokeResult is what the hidden TUI smoke test saw.
type SmokeResult struct {
	OK     bool
	Alive  bool   // still running when the wait ended
	Ready  bool   // a ready marker appeared
	Fatal  string // the fatal text that appeared, if any
	Code   int    // exit code, when it exited
	Screen string // the screen text, ANSI stripped (goes to the log)
}

// Tool is one app EZLM connects. Each tool lives in its own files and calls
// Register from its init(), so adding a tool touches no shared file.
type Tool interface {
	Info() Info
	// Detect runs on every Status poll: it may stat files, parse configs and
	// read the probe cache, but must never run a program. ep is nil when no
	// model is deployed.
	Detect(ep *Endpoint) Detection
	// Resolve picks the program EZLM will run, probing version, architecture
	// and data format (cached with SaveProbe). With install, it installs
	// EZLM's pinned copy when nothing usable is found; without, it returns
	// ErrNotInstalled.
	Resolve(ctx context.Context, r *Run, install bool) (Found, error)
	// Configure writes EZLM's entry into the tool's config. Rewrite calls it
	// with makeDefault false. It returns ErrNeeds64K when ep.Ctx is below
	// the tool's floor, leaving the file untouched.
	Configure(ep Endpoint, makeDefault bool) error
	// Check is the model-and-tool check: the app reads w's file with a real
	// tool call. A nil error means the nonce came back. The error is one
	// plain sentence (see Explain).
	Check(ctx context.Context, f Found, ep Endpoint, w *CheckWorkspace, r *Run) (toolCall bool, err error)
	// Preflight is the fast launch check: the exact launch program,
	// environment and data folders, no new session.
	Preflight(ctx context.Context, f Found, ep Endpoint, r *Run) error
	// LaunchSpec is what Open runs for mode (a LaunchMode id).
	LaunchSpec(f Found, ep Endpoint, mode string) (LaunchSpec, error)
}
