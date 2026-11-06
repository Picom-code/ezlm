package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/picom-code/ezlm/internal/integrate"
	"github.com/picom-code/ezlm/internal/report"
	"github.com/picom-code/ezlm/internal/scan"
	"github.com/picom-code/ezlm/internal/serve"
	"github.com/picom-code/ezlm/internal/version"
)

// Launch opens an app (OpenCode, Hermes, DeepSeek Harness) for mode ("" =
// its main launch mode), using the user's model. One click either way: the
// server is started if it isn't running, an app that isn't set up yet is
// installed and connected first, and one whose settings point at an old
// address, key or model is updated first. Runs as the "launch" job, with
// steps "connect" and "open"; "open" ends only once the app has stayed up
// for launchWait, or fails with the reason it closed.
func (a *App) Launch(id, mode string) error {
	cfg, err := serve.Load()
	if err != nil {
		return errors.New("set up a model first")
	}
	tl, ok := integrate.Lookup(id)
	if !ok {
		return fmt.Errorf("unknown app %q", id)
	}
	inf := tl.Info()
	if mode == "" && len(inf.Launch) > 0 {
		mode = inf.Launch[0].ID
	}
	return a.start("launch", func(ctx context.Context, step func(string, string, string)) error {
		fail := func(s string, err error) error {
			err = sentence(err)
			step(s, "fail", err.Error())
			return err
		}
		if err := a.serverGate(ctx, cfg, func(msg string) { step("open", "info", msg) }); err != nil {
			return fail("open", err)
		}
		ep := endpointFor(cfg)
		var target *integrate.Target
		for _, t := range a.withAppErrs(integrate.Targets(&ep)) {
			if t.ID == id {
				t := t
				target = &t
			}
		}
		if target == nil {
			return fail("open", fmt.Errorf("unknown app %q", id))
		}
		if !target.Supported {
			return fail("open", errors.New(target.Why))
		}
		switch target.State {
		case integrate.StateNeeds64K, integrate.StateNeedsTools:
			return fail("open", errors.New(target.Detail))
		}
		switch {
		case !target.Linked || !target.Installed:
			step("connect", "run", fmt.Sprintf("Setting up %s (only the first time)", target.Name))
			if err := integrate.Setup(ctx, id, ep, false, func(msg string) { step("connect", "info", msg) }); err != nil {
				return fail("connect", err)
			}
			a.setAppErr(id, "")
			step("connect", "ok", target.Name+" is connected")
		case target.State == integrate.StateNeedsUpdate:
			// The config drifted (an old port, key or model): fix it and
			// make sure the app reads it before opening.
			step("open", "info", fmt.Sprintf("Updating %s to use %s", target.Name, cfg.ModelName))
			if err := integrate.Rewrite(id, ep); err != nil {
				a.setAppErr(id, err.Error())
				return fail("open", err)
			}
			a.setAppErr(id, "")
			if err := integrate.Preflight(ctx, id, ep, func(msg string) { step("open", "info", msg) }); err != nil {
				err = sentence(err)
				integrate.SaveCheck(id, integrate.CheckResult{At: time.Now(), Stage: integrate.StagePreflight, Msg: err.Error(), LogPath: integrate.LogPath(id)})
				return fail("open", err)
			}
		}
		step("open", "run", "Opening "+target.Name)
		l, err := openApp(ctx, id, mode, ep, func(msg string) { step("open", "info", msg) })
		if err == nil && l.ExitFile != "" && !l.DryRun {
			step("open", "info", target.Name+" is starting in a new window")
			err = a.watchExit(ctx, id, target.Name, l, ep)
		}
		if err != nil {
			err = sentence(err)
			integrate.SaveCheck(id, integrate.CheckResult{At: time.Now(), Stage: integrate.StageLaunch, Msg: err.Error(), LogPath: integrate.LogPath(id)})
			return fail("open", err)
		}
		integrate.ClearLaunchFailure(id)
		where := "in a new window"
		if mode == integrate.ModeWeb {
			where = "in your browser"
		}
		step("open", "ok", fmt.Sprintf("%s is open %s, using %s", target.Name, where, cfg.ModelName))
		return nil
	})
}

// launchWait is how long Open watches for an app that exits right away.
var launchWait = 20 * time.Second

// watchExit waits up to launchWait for the launcher's exit file. An app
// that exits with a non-zero code in that time failed to open, and the error
// says why as well as EZLM can tell: the launch preflight run again, else a
// known error text in the app's log. Launchers that don't write an exit
// file are not watched.
func (a *App) watchExit(ctx context.Context, id, name string, l integrate.Launched, ep integrate.Endpoint) error {
	if l.ExitFile == "" || l.DryRun {
		return nil
	}
	end := time.Now().Add(launchWait)
	for time.Now().Before(end) {
		if code, _, ok := integrate.ReadExit(l.ExitFile); ok {
			if code != 0 {
				return exitError(ctx, id, name, code, ep)
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
	return nil
}

// exitError is the message for an app that closed right after opening.
func exitError(ctx context.Context, id, name string, code int, ep integrate.Endpoint) error {
	pctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	reason := ""
	if err := integrate.Preflight(pctx, id, ep, nil); err != nil {
		reason = err.Error()
	} else if e, ok := integrate.Explain(id, strings.Join(integrate.LogTail(id, 80), "\n")); ok {
		reason = e.Say
	}
	if reason == "" {
		return fmt.Errorf("%s closed right after opening (exit code %d). Press Report a problem to send the details.", name, code)
	}
	return fmt.Errorf("%s closed right after opening (exit code %d): %s", name, code, reason)
}

// sentence is err as the apps show it on its own: a capital first letter
// and a closing period (Go errors are often neither).
func sentence(err error) error {
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		return err
	}
	r, n := utf8.DecodeRuneInString(msg)
	msg = string(unicode.ToUpper(r)) + msg[n:]
	if last, _ := utf8.DecodeLastRuneInString(msg); unicode.IsLetter(last) || unicode.IsDigit(last) || last == ')' {
		msg += "."
	}
	if msg == err.Error() {
		return err
	}
	return errors.New(msg)
}

// Report files a tester's problem report (see package report), with a
// summary of their setup and, if they agree, recent logs. API keys and the
// user's home folder are removed first.
func (a *App) Report(ctx context.Context, text string, details bool, appLog string) (report.Result, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return report.Result{}, errors.New("say what happened first")
	}
	first := strings.SplitN(text, "\n", 2)[0]
	if r := []rune(first); len(r) > 70 {
		first = string(r[:70]) + "…"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "### What happened\n\n%s\n\n### Setup\n\n", text)
	hw, _ := a.hardware(ctx)
	fmt.Fprintf(&b, "- EZLM %s on %s (%s/%s)\n", version.Version, hw.OSName, runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "- %s, %d threads, %.0f GB memory\n", hw.CPUName, hw.Cores, float64(hw.RAMTotal)/float64(scan.GB))
	for _, g := range hw.GPUs {
		fmt.Fprintf(&b, "- %s, %.0f GB\n", g.Name, float64(g.MemTotal)/float64(scan.GB))
	}
	fmt.Fprintf(&b, "- Engine: %s\n", hw.Backend)
	st := a.Status(ctx)
	if st.Config != nil {
		state := "stopped"
		switch {
		case st.Healthy:
			state = "running"
		case st.Starting:
			state = "loading"
		}
		fmt.Fprintf(&b, "- Model: %s (%s, %dK context), %s; %s\n", st.Config.ModelName, st.Config.Quant, st.Config.Ctx/1024, state, st.Config.Recipe)
	} else {
		b.WriteString("- No model set up yet\n")
	}
	for _, t := range st.Targets {
		ver := t.Version
		if ver == "" {
			ver = "version unknown"
		}
		fmt.Fprintf(&b, "- %s: %s (%s, %s)\n", t.Name, t.State, ver, firstNonEmpty(t.Source, "-"))
	}
	if details {
		b.WriteString("\n<details><summary>Logs</summary>\n\n```\n")
		lines := a.Logs(60)
		b.WriteString(strings.Join(lines, "\n"))
		b.WriteString("\n```\n")
		if appLog = strings.TrimSpace(appLog); appLog != "" {
			if len(appLog) > 8000 {
				appLog = appLog[len(appLog)-8000:]
			}
			b.WriteString("\nApp:\n```\n" + appLog + "\n```\n")
		}
		b.WriteString(appAttachments())
		b.WriteString("</details>\n")
	}
	var secrets []string
	if st.Config != nil {
		secrets = append(secrets, st.Config.Key)
		for _, k := range st.Config.Keys {
			secrets = append(secrets, k.Key)
		}
	}
	body := integrate.Redact(b.String(), secrets...)
	if home, err := os.UserHomeDir(); err == nil && len(home) > 3 {
		body = strings.ReplaceAll(body, home, "~")
	}
	if len(body) > 60000 {
		body = body[:60000] + "\n…(trimmed)"
	}
	return report.Send(ctx, "Beta report: "+first, body)
}

// appAttachments is each connectable app's integration log (the last 200
// lines) and its last launch exit file, for a problem report. The logs are
// written with keys removed; Report redacts the whole body once more.
func appAttachments() string {
	var b strings.Builder
	for _, t := range integrate.All() {
		inf := t.Info()
		lines := integrate.LogTail(inf.ID, 200)
		exit, _ := os.ReadFile(integrate.ExitFile(inf.ID))
		if len(lines) == 0 && len(exit) == 0 {
			continue
		}
		if len(lines) > 0 {
			text := strings.Join(lines, "\n")
			if len(text) > 12000 {
				text = "…" + text[len(text)-12000:]
			}
			fmt.Fprintf(&b, "\n%s (integrate-%s.log):\n```\n%s\n```\n", inf.Name, inf.ID, text)
		}
		if len(exit) > 0 {
			fmt.Fprintf(&b, "%s last exit (launch/%s.exit): %s\n", inf.Name, inf.ID, strings.TrimSpace(string(exit)))
		}
	}
	return b.String()
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}
