package app

import (
	"os"
	"strings"
	"time"

	"github.com/picom-code/ezlm/internal/integrate"
)

// The demo's connectable apps. By default they start in a spread of states
// for walkthroughs and UI work: OpenCode needs an update, Hermes needs 64K,
// DeepSeek Harness isn't installed (and its first launch fails, see Launch).
// EZLM_DEMO_APPS sets other starting states for snapshots, e.g.
// "opencode=connected,hermes=broken,dsh=needs_tools"; "+fail" after a state
// ("dsh=connected+fail") makes that app's next launch fail.

type demoApp struct {
	id, state, detail    string
	version, source, bin string
	config, log          string
	check                *integrate.CheckResult
	failNext             bool // the next launch exits right away
}

func demoName(id string) string {
	if t, ok := integrate.Lookup(id); ok {
		return t.Info().Name
	}
	return id
}

const demoUser = `C:\Users\Sam`

func demoApps() map[string]*demoApp {
	ago := func(d time.Duration) time.Time { return time.Now().Add(-d) }
	logOf := func(id string) string { return demoUser + `\.ezlm\logs\integrate-` + id + ".log" }
	apps := map[string]*demoApp{
		"opencode": {id: "opencode", state: integrate.StateNeedsUpdate,
			detail:  "Points at an old address or key: EZLM fixes it when you open it",
			version: "2.2.5", source: integrate.SourceEZLM, bin: demoUser + `\.ezlm\tools\opencode\2.2.5\opencode.exe`,
			config: demoUser + `\.config\opencode\opencode.json`, log: logOf("opencode"),
			check: &integrate.CheckResult{At: ago(26 * time.Hour), OK: true, ToolCall: true, Secs: 14, Stage: integrate.StageModel, Msg: "answered and used a tool (14 s)", LogPath: logOf("opencode")}},
		"hermes": {id: "hermes", state: integrate.StateNeeds64K,
			detail:  "Hermes needs a 64K context. Connect again to give the model the room (it may run slower), or pick a smaller model.",
			version: "0.22.5", source: integrate.SourceMSIX, bin: demoUser + `\AppData\Local\Microsoft\WindowsApps\hermes.exe`,
			config: demoUser + `\AppData\Local\hermes\config.yaml`, log: logOf("hermes"),
			check: &integrate.CheckResult{At: ago(3 * time.Hour), OK: true, ToolCall: true, Secs: 22, Stage: integrate.StageModel, Msg: "answered and used a tool (22 s)", LogPath: logOf("hermes")}},
		"dsh": {id: "dsh", state: integrate.StateNotInstalled, failNext: true,
			config: demoUser + `\.dsh\profiles\web\cordis.patch.yml`, log: logOf("dsh")},
	}
	for _, kv := range strings.Split(os.Getenv("EZLM_DEMO_APPS"), ",") {
		id, state, ok := strings.Cut(strings.TrimSpace(kv), "=")
		if a := apps[id]; ok && a != nil {
			state, fail := strings.CutSuffix(state, "+fail")
			a.setState(state)
			a.failNext = fail
		}
	}
	return apps
}

// setState puts an app into state with the details that state shows.
func (a *demoApp) setState(state string) {
	switch state {
	case integrate.StateNotInstalled:
		*a = demoApp{id: a.id, state: state, config: a.config, log: a.log}
		return
	case integrate.StateConnected:
		a.connect()
		return
	case integrate.StateNeedsUpdate:
		a.detail = "Points at an old address or key: EZLM fixes it when you open it"
	case integrate.StateNeeds64K:
		a.detail = demoName(a.id) + " needs a 64K context. Connect again to give the model the room (it may run slower), or pick a smaller model."
	case integrate.StateNeedsTools:
		a.detail = "This model can't use tools, so " + demoName(a.id) + " can't edit files with it. Pick a model marked Tools, or use Chat."
	case integrate.StateBroken:
		a.detail = demoName(a.id) + " closed right after opening (exit code 1). Press Report a problem to send the details."
		a.check = &integrate.CheckResult{At: time.Now().Add(-5 * time.Minute), Stage: integrate.StageLaunch, Msg: a.detail, LogPath: a.log}
		a.failNext = false
	case integrate.StateInstalled:
		a.detail = ""
	default:
		return
	}
	a.installed()
	a.state = state
}

// installed fills in the version and paths of an installed app.
func (a *demoApp) installed() {
	if a.bin != "" {
		return
	}
	switch a.id {
	case "opencode":
		a.version, a.source, a.bin = "2.2.5", integrate.SourceEZLM, demoUser+`\.ezlm\tools\opencode\2.2.5\opencode.exe`
	case "hermes":
		a.version, a.source, a.bin = "0.22.5", integrate.SourceMSIX, demoUser+`\AppData\Local\Microsoft\WindowsApps\hermes.exe`
	case "dsh":
		a.version, a.source, a.bin = "0.2.1-rc.5", integrate.SourceEZLM, demoUser+`\.ezlm\tools\dsh\0.2.1-rc.5\node_modules\.bin\dsh.cmd`
	}
}

// connect is a successful Connect: installed, connected, a fresh check.
func (a *demoApp) connect() {
	a.installed()
	a.state, a.detail = integrate.StateConnected, ""
	a.check = &integrate.CheckResult{At: time.Now(), OK: true, ToolCall: true, Secs: 14, Stage: integrate.StageModel, Msg: "answered and used a tool (14 s)", LogPath: a.log}
}

// targets is the demo's Status.Targets, shaped exactly like the real ones.
func (d *Demo) targets() []integrate.Target {
	var out []integrate.Target
	for _, id := range []string{"opencode", "hermes", "dsh"} {
		a := d.apps[id]
		tl, ok := integrate.Lookup(id)
		if a == nil || !ok {
			continue
		}
		inf := tl.Info()
		t := integrate.Target{ID: id, Name: inf.Name, Desc: inf.Desc, Supported: true, Icon: inf.Icon,
			Installed: a.state != integrate.StateNotInstalled, Connected: a.state == integrate.StateConnected,
			Version: a.version, Bin: a.bin, Source: a.source, State: a.state, Detail: a.detail, Config: a.config,
			MinCtx: inf.MinCtx, Launch: inf.Launch, Linked: a.state != integrate.StateNotInstalled && a.state != integrate.StateInstalled}
		if a.check != nil {
			c := *a.check
			t.LastCheck = &c
		}
		out = append(out, t)
	}
	return out
}

// demoInstallLines are what the demo's installers "print".
var demoInstallLines = map[string][]string{
	"opencode": {"Downloading opencode-windows-x64.zip (2.2.5)", "Checksum matches", "Unpacked to .ezlm\\tools\\opencode\\2.2.5"},
	"hermes":   {"Installing uv", "Installing Python 3.11", "Installing Hermes Agent", "hermes 0.22.5 is ready"},
	"dsh":      {"Downloading Node.js 24 (win-x64)", "Checksum matches", "npm install @deepseek-ai/dsh@0.2.1-rc.5", "added 412 packages"},
}
