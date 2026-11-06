// Command ezlm deploys the smartest local LLM your machine can run and hands
// you an OpenAI-compatible URL and API key.
//
// Run with no arguments (or double-click) to open the setup page in your
// browser. Subcommands cover headless use.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/picom-code/ezlm/internal/app"
	"github.com/picom-code/ezlm/internal/bench"
	"github.com/picom-code/ezlm/internal/integrate"
	"github.com/picom-code/ezlm/internal/paths"
	"github.com/picom-code/ezlm/internal/scan"
	"github.com/picom-code/ezlm/internal/serve"
	"github.com/picom-code/ezlm/internal/version"
	"github.com/picom-code/ezlm/internal/web"
)

const banner = `
 ███████╗███████╗██╗     ███╗   ███╗
 ██╔════╝╚══███╔╝██║     ████╗ ████║
 █████╗    ███╔╝ ██║     ██╔████╔██║
 ██╔══╝   ███╔╝  ██║     ██║╚██╔╝██║
 ███████╗███████╗███████╗██║ ╚═╝ ██║
 ╚══════╝╚══════╝╚══════╝╚═╝     ╚═╝
`

func main() {
	cmd := ""
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	var err error
	switch cmd {
	case "", "ui", "open", "--demo":
		err = ui(hasFlag("--demo"))
	case "engine":
		err = engine(hasFlag("--demo"))
	case "scan":
		err = scanCmd()
	case "deploy":
		model := ""
		if len(os.Args) > 2 && !strings.HasPrefix(os.Args[2], "--") {
			model = os.Args[2]
		}
		err = deployCmd(model)
	case "status":
		err = statusCmd()
	case "test":
		err = testCmd()
	case "models":
		err = modelsCmd()
	case "use", "swap":
		if len(os.Args) < 3 {
			err = fmt.Errorf("usage: ezlm use <model-id> [file]   (see ezlm models / ezlm scan)")
			break
		}
		q := ""
		if len(os.Args) > 3 && !strings.HasPrefix(os.Args[3], "--") {
			q = os.Args[3]
		}
		a := app.New()
		err = waitDeploy(a, func() error { return a.Use(context.Background(), os.Args[2], q, pref()) })
	case "remove", "rm":
		if len(os.Args) < 3 {
			err = fmt.Errorf("usage: ezlm remove <model-id> [file]")
			break
		}
		q := ""
		if len(os.Args) > 3 {
			q = os.Args[3]
		}
		if err = app.New().Remove(context.Background(), os.Args[2], q); err == nil {
			fmt.Println("Removed.")
		}
	case "connect":
		err = connectCmd(os.Args[2:])
	case "doctor":
		// ezlm doctor apps [ids] [--json]: each app's state and its launch
		// preflight; opens no windows and starts no chat.
		if len(os.Args) < 3 || os.Args[2] != "apps" {
			err = fmt.Errorf("usage: ezlm doctor apps [opencode hermes dsh] [--json]")
			break
		}
		err = doctorApps(ids(os.Args[3:]), hasFlag("--json"))
	case "key", "keys":
		err = keyCmd(os.Args[2:])
	case "url":
		err = withConfig(func(c *serve.Config) { fmt.Println(c.LocalURL()) })
	case "port":
		// ezlm port 8090: move the server; connected apps follow.
		var port int
		if len(os.Args) > 2 {
			fmt.Sscan(os.Args[2], &port)
		}
		err = app.New().SetPort(context.Background(), port)
		if err == nil {
			fmt.Println("Server is on port", port)
			if note := app.OpenSessionsNote(); note != "" {
				fmt.Println(note)
			}
		}
	case "storage":
		// ezlm storage                show where models go
		// ezlm storage <folder> [--move]   send new downloads there (and move existing ones)
		a := app.New()
		if len(os.Args) < 3 {
			st := a.Storage(context.Background())
			fmt.Printf("Models go to %s (%.0f GB free); EZLM models take %.1f GB\n", st.Dir, float64(st.Free)/float64(scan.GB), float64(st.ModelsBytes)/float64(scan.GB))
			for _, d := range st.Drives {
				fmt.Printf("  %-6s %-14s %6.0f GB free of %.0f GB\n", d.Path, d.Label, float64(d.Free)/float64(scan.GB), float64(d.Total)/float64(scan.GB))
			}
			break
		}
		move := len(os.Args) > 3 && os.Args[3] == "--move"
		err = waitFor(a, "storage", func() error { return a.SetStorage(os.Args[2], move) })
	case "update":
		a := app.New()
		info := a.UpdateInfo(context.Background(), true)
		switch {
		case info.Error != "":
			err = errors.New(info.Error)
		case !info.Available:
			fmt.Println("EZLM", info.Current, "is the latest version.")
		default:
			fmt.Printf("EZLM %s is available (you have %s).\n", info.Latest.Version, info.Current)
			if len(os.Args) > 2 && os.Args[2] == "--download" {
				err = waitFor(a, "update", func() error { return a.DownloadUpdate() })
			}
		}
	case "apps":
		// What Model mode would offer to close; "apps close ID [--force]" closes one.
		args := os.Args[2:]
		if len(args) >= 2 && args[0] == "close" {
			r, cerr := scan.CloseApp(context.Background(), args[1], len(args) > 2 && args[2] == "--force")
			fmt.Printf("closed=%v remaining=%d freed=%.2f GB need_force=%v\n", r.Closed, r.Remaining, float64(r.Freed)/float64(scan.GB), r.NeedForce)
			err = cerr
			break
		}
		minMB := uint64(250)
		if len(args) >= 2 && args[0] == "--min" {
			fmt.Sscan(args[1], &minMB)
		}
		for _, a := range scan.MemoryApps(context.Background(), minMB<<20) {
			fmt.Printf("%-28s %8.2f GB RAM %6.2f GB GPU  %-10s win=%-5v %2d procs  %s\n", a.Name, float64(a.RAM)/float64(scan.GB), float64(a.GPU)/float64(scan.GB), a.Kind, a.Windows, len(a.PIDs), a.ID)
		}
	case "start":
		// --login: the automatic start at sign-in, skipped if the user had stopped it.
		if len(os.Args) > 2 && os.Args[2] == "--login" {
			if _, serr := os.Stat(serve.StoppedFlag()); serr == nil {
				return
			}
		}
		// Also used at login (Windows Run key, via the app's --autostart):
		// pre-reads split models, moves off a port another program took, and
		// waits until the server is really running.
		if len(os.Args) <= 2 || os.Args[2] != "--login" {
			os.Remove(serve.StoppedFlag())
		}
		if c, lerr := serve.Load(); lerr != nil {
			err = lerr
		} else {
			err = app.New().StartSync(context.Background(), c, nil)
		}
	case "stop":
		if len(os.Args) > 2 && os.Args[2] == "--for-uninstall" {
			// The app is being removed: stop the server, but don't record it as
			// the user's choice (a reinstall should start it again).
			err = serve.Stop()
			break
		}
		err = app.New().Stop()
	case "run-server":
		err = serve.RunForeground()
	case "uninstall":
		err = app.New().Uninstall()
		if err == nil {
			fmt.Println("EZLM removed: server stopped, models and settings deleted.")
		}
	case "version", "--version", "-v":
		fmt.Println("ezlm", version.Version)
	case "help", "--help", "-h":
		usage()
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "ezlm:", err)
		os.Exit(1)
	}
}

func hasFlag(f string) bool {
	for _, a := range os.Args[1:] {
		if a == f {
			return true
		}
	}
	return false
}

func backend(demo bool) web.Backend {
	if demo {
		return app.NewDemo()
	}
	return app.New()
}

// engine is how the native apps run EZLM: it serves the API on a random
// loopback port, prints {"url","token"} as one JSON line, and exits when the
// app closes our stdin (so it never outlives the app).
func engine(demo bool) error {
	if !demo {
		go serve.EnsureAutostart()
	}
	srv, l, _, err := web.Listen(backend(demo), nil)
	if err != nil {
		return err
	}
	json.NewEncoder(os.Stdout).Encode(map[string]string{"url": "http://" + srv.Addr(), "token": srv.Token()})
	go func() {
		io.Copy(io.Discard, os.Stdin)
		os.Exit(0)
	}()
	return srv.Serve(l)
}

func usage() {
	fmt.Print(banner + `
  ezlm             open the setup page in your browser
  ezlm scan        show what this machine can run (--smartest / --fastest)
  ezlm deploy [id] deploy the recommended model (or a specific one)
  ezlm status      show the URL, key and server state
  ezlm test        run a quick check of the deployed model
  ezlm models      models already on this computer (switch instantly)
  ezlm use <id>    switch to a model (no download if it's already here)
  ezlm remove <id> delete a downloaded model
  ezlm connect opencode hermes dsh [--default]   add your model to these apps (installs them if needed)
  ezlm doctor apps [ids] [--json]   show each app's state and check how it would open
  ezlm key         print the main API key (ezlm url prints the base URL)
  ezlm key list    list all keys
  ezlm key new [name]   create another key
  ezlm key rotate [id]  replace a key's value (default: the main key)
  ezlm start|stop  start or stop the server
  ezlm uninstall   remove the server, models and settings
`)
}

func withConfig(fn func(*serve.Config)) error {
	c, err := serve.Load()
	if err != nil {
		return fmt.Errorf("nothing is deployed yet: run ezlm")
	}
	fn(c)
	return nil
}

// ui serves the setup page and opens it. A second launch just reopens the
// page of the instance already running.
func ui(demo bool) error {
	urlFile := filepath.Join(paths.Home(), "ui.url")
	if demo {
		urlFile += ".demo"
	}
	if b, err := os.ReadFile(urlFile); err == nil {
		u := strings.TrimSpace(string(b))
		if reachable(u) {
			fmt.Println("EZLM is already open:", u)
			return openBrowser(u)
		}
	}

	var last atomic.Int64
	touch := func() { last.Store(time.Now().Unix()) }
	touch()
	a := backend(demo)
	srv, l, url, err := web.Listen(a, touch)
	if err != nil {
		return err
	}
	paths.Ensure()
	os.WriteFile(urlFile, []byte(url), 0o600)
	defer os.Remove(urlFile)

	fmt.Print(banner)
	fmt.Println("\n  Setup page:", url)
	fmt.Println("  Leave this running while you use the page. It closes itself when you're done.")
	if os.Getenv("EZLM_NO_BROWSER") == "" {
		openBrowser(url)
	}

	go func() {
		// The open page pings every 15s; quit a few minutes after it's closed,
		// but never in the middle of a job.
		for range time.Tick(30 * time.Second) {
			idle := time.Since(time.Unix(last.Load(), 0))
			if idle > 3*time.Minute && a.Status(context.Background()).Busy == "" {
				os.Remove(urlFile)
				os.Exit(0)
			}
		}
	}()
	return srv.Serve(l)
}

func reachable(u string) bool {
	c := http.Client{Timeout: time.Second}
	resp, err := c.Get(u)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

func gb(b uint64) string { return fmt.Sprintf("%.1f GB", float64(b)/float64(scan.GB)) }

// pref reads --smartest / --fastest from the command line.
func pref() string {
	switch {
	case hasFlag("--smartest"):
		return "smartest"
	case hasFlag("--fastest"):
		return "fastest"
	}
	return "balanced"
}

func scanCmd() error {
	if hasFlag("--json") {
		return json.NewEncoder(os.Stdout).Encode(app.New().Scan(context.Background(), pref()))
	}
	r := app.New().Scan(context.Background(), pref())
	fmt.Printf("Preference: %s\n", r.Pref)
	h := r.HW
	fmt.Printf("CPU      %s (%d cores)\n", h.CPUName, h.Cores)
	fmt.Printf("Memory   %s total, %s free\n", gb(h.RAMTotal), gb(h.RAMAvail))
	for _, g := range h.GPUs {
		fmt.Printf("GPU      %s, %s total, %s free\n", g.Name, gb(g.MemTotal), gb(g.MemFree))
	}
	fmt.Printf("Disk     %s free (models go in %s)\n", gb(h.DiskFree), h.ModelsDir)
	for _, p := range h.GPUProcs {
		fmt.Printf("On GPU   %s (pid %d) %s\n", p.Name, p.PID, gb(p.Mem))
	}
	fmt.Printf("Engine   %s\n", h.Backend)
	for _, n := range h.Notes {
		fmt.Println("Note    ", n)
	}
	for _, c := range r.Prereqs {
		mark := map[string]string{"ok": "✓", "fix": "↻", "warn": "!", "fail": "✗"}[string(c.Status)]
		fmt.Printf("Needs    %s %s: %s %s\n", mark, c.Name, c.Detail, c.Action)
	}
	fmt.Println()
	if r.Recommended == nil {
		fmt.Println("No model fits right now. Close some apps or free disk space and scan again.")
	}
	for i, p := range r.Plans {
		mark := "  "
		if r.Recommended != nil && p.Model.ID == r.Recommended.Model.ID {
			mark = "→ "
		}
		disk := ""
		if !p.DiskOK {
			disk = "  (not enough disk)"
		}
		fmt.Printf("%s%-26s %-20s %3dk %-5s %-6s ~%3.0f tok/s  %7s  q=%.1f  %s%s\n", mark, p.Model.ID, p.Quant.Name, p.Ctx/1024, p.KV, p.Mode, p.TokPerSec, gb(p.Download), p.Effective, p.Spec, disk)
		if i > 8 {
			break
		}
	}
	if r.Unlock != nil {
		what := "memory"
		if r.UnlockGPU {
			what = "graphics memory"
		}
		fmt.Printf("\nFree up %.0f GB of %s (close apps) to run %s (%s, ~%.0f tok/s) instead.\n", r.UnlockFreeGB, what, r.Unlock.Model.Name, r.Unlock.Mode, r.Unlock.TokPerSec)
	}
	return nil
}

// deployCmd runs a deploy in the terminal, printing progress lines.
func deployCmd(model string) error {
	a := app.New()
	r := a.Scan(context.Background(), pref())
	if r.Recommended == nil && model == "" {
		return fmt.Errorf("no model fits this machine right now")
	}
	return waitDeploy(a, func() error { return a.Deploy(model) })
}

// waitDeploy starts a deploy job and prints its progress until it finishes.
// waitFor runs a background job and prints its steps until it finishes.
func waitFor(a *app.App, job string, start func() error) error {
	ch, cancel := a.Subscribe()
	defer cancel()
	if err := start(); err != nil {
		return err
	}
	lastPct := -1
	for e := range ch {
		if e.Job != job {
			continue
		}
		switch {
		case e.State == "progress" && e.Total > 0:
			if pct := int(e.Done * 100 / e.Total); pct/10 != lastPct/10 {
				fmt.Printf("    %3d%%  %s / %s\n", pct, gb(e.Done), gb(e.Total))
				lastPct = pct
			}
		case e.Step == "done" && e.State == "ok":
			return nil
		case e.Step == "done":
			return errors.New(e.Msg)
		case e.State == "run" || e.State == "info" || e.State == "ok":
			if e.Msg != "" {
				fmt.Println("  …", e.Msg)
			}
		case e.State == "fail":
			fmt.Println("  ✗", e.Msg)
		}
	}
	return nil
}

func waitDeploy(a *app.App, start func() error) error {
	ch, cancel := a.Subscribe()
	defer cancel()
	if err := start(); err != nil {
		return err
	}
	lastPct := -1
	for e := range ch {
		switch e.State {
		case "progress":
			if e.Total > 0 {
				pct := int(e.Done * 100 / e.Total)
				if pct/10 != lastPct/10 {
					fmt.Printf("    %3d%%  %s / %s  %.1f MB/s\n", pct, gb(e.Done), gb(e.Total), e.BPS/1e6)
					lastPct = pct
				}
			}
		case "run":
			lastPct = -1
			fmt.Println("  …", e.Msg)
		case "ok":
			if e.Step == "done" {
				c, _ := serve.Load()
				fmt.Printf("\n  URL  %s\n  KEY  %s\n", c.LocalURL(), c.Key)
				return nil
			}
			fmt.Println("  ✓", e.Msg)
		case "info":
			if e.Msg != "" {
				fmt.Println("  ·", e.Msg)
			}
		case "fail":
			if e.Step == "done" {
				return fmt.Errorf("%s", e.Msg)
			}
			fmt.Println("  ✗", e.Msg)
		}
	}
	return nil
}

func statusCmd() error {
	a := app.New()
	s := a.Status(context.Background())
	if !s.Deployed {
		fmt.Println("Nothing deployed yet. Run: ezlm")
		return nil
	}
	state := "stopped"
	switch {
	case s.Healthy:
		state = "running"
	case s.Starting:
		state = "loading the model (it answers when this finishes)"
	}
	fmt.Printf("Model   %s (%s, %dk context)\nServer  %s\nURL     %s\n", s.Config.ModelName, s.Config.Quant, s.Config.Ctx/1024, state, s.LocalURL)
	if s.LANURL != "" {
		fmt.Printf("LAN     %s\n", s.LANURL)
	}
	fmt.Printf("Key     %s\n", s.Config.Key)
	return nil
}

// keyCmd manages API keys from the terminal.
func keyCmd(args []string) error {
	c, err := serve.Load()
	if err != nil {
		return fmt.Errorf("nothing is deployed yet: run ezlm")
	}
	if len(args) == 0 {
		fmt.Println(c.Key)
		return nil
	}
	a := app.New()
	switch args[0] {
	case "list", "ls":
		for _, k := range c.Keys {
			fmt.Printf("%-6s %-20s %s\n", k.ID, k.Name, k.Key)
		}
		return nil
	case "new", "add":
		name := strings.Join(args[1:], " ")
		return keysAfter(waitJob(a, func() error { return a.AddKey(name) }))
	case "rotate":
		id := c.Keys[0].ID
		if len(args) > 1 {
			id = args[1]
		}
		return keysAfter(waitJob(a, func() error { return a.RotateKey(id) }))
	case "revoke", "rm":
		if len(args) < 2 {
			return fmt.Errorf("usage: ezlm key revoke <id> (see ezlm key list)")
		}
		return keysAfter(waitJob(a, func() error { return a.RevokeKey(args[1]) }))
	}
	return fmt.Errorf("unknown key command %q", args[0])
}

// keysAfter prints the keys once a key command's job succeeded (the user
// asked for a new one). Other jobs never print keys.
func keysAfter(err error) error {
	if err != nil {
		return err
	}
	if c, lerr := serve.Load(); lerr == nil {
		for _, k := range c.Keys {
			fmt.Printf("  %-6s %-20s %s\n", k.ID, k.Name, k.Key)
		}
	}
	return nil
}

// waitJob starts a background job and prints its progress until it ends.
func waitJob(a *app.App, start func() error) error {
	ch, cancel := a.Subscribe()
	defer cancel()
	if err := start(); err != nil {
		return err
	}
	for e := range ch {
		if e.Step == "done" {
			if e.State == "fail" {
				return fmt.Errorf("%s", e.Msg)
			}
			return nil
		}
		if e.Msg != "" && e.State != "progress" {
			fmt.Println("  …", e.Msg)
		}
	}
	return nil
}

// testCmd runs the model check suite against the deployed server.
func testCmd() error {
	c, err := serve.Load()
	if err != nil {
		return fmt.Errorf("nothing is deployed yet: run ezlm")
	}
	// A model that's still loading answers 503: wait for it rather than fail.
	if !serve.Healthy(context.Background(), c) {
		if len(serve.OwnPIDs(context.Background())) == 0 {
			return fmt.Errorf("the server isn't running: start it with ezlm start")
		}
		fmt.Println("Waiting for the model to finish loading…")
		if err := serve.WaitHealthy(context.Background(), c, 10*time.Minute, nil); err != nil {
			return err
		}
	}
	fmt.Printf("Testing %s (%s)\n\n", c.ModelName, c.Quant)
	passed := 0
	res := bench.Run(context.Background(), bench.Target{BaseURL: c.LocalURL(), Key: c.Key, Model: c.ModelID}, func(r bench.Result) {
		mark := "✗"
		if r.Pass {
			mark = "✓"
			passed++
		}
		fmt.Printf("  %s %-24s %s  (%.1fs)\n", mark, r.Name, r.Detail, r.Secs)
	})
	fmt.Printf("\n%d of %d passed\n", passed, len(res))
	return nil
}

// connectCmd wires the deployed model into agent apps, printing progress.
// Flags (--default) are not app ids.
func connectCmd(args []string) error {
	list := ids(args)
	if len(list) == 0 {
		return fmt.Errorf("usage: ezlm connect opencode hermes dsh [--default]")
	}
	a := app.New()
	return waitJob(a, func() error { return a.Integrate(list, hasFlag("--default")) })
}

// ids drops flags from a list of app ids.
func ids(args []string) []string {
	var out []string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			out = append(out, a)
		}
	}
	return out
}

// doctorApps prints each connectable app and runs its launch preflight.
func doctorApps(only []string, asJSON bool) error {
	a := app.New()
	targets := a.Status(context.Background()).Targets
	if len(only) > 0 {
		var keep []integrate.Target
		for _, t := range targets {
			if slices.Contains(only, t.ID) {
				keep = append(keep, t)
			}
		}
		targets = keep
	}
	type row struct {
		integrate.Target
		Preflight string `json:"preflight"`
	}
	var rows []row
	for _, t := range targets {
		r := row{Target: t, Preflight: "skipped"}
		if t.Installed && t.Supported {
			r.Preflight = "ok"
			if err := a.Preflight(context.Background(), t.ID); err != nil {
				r.Preflight = err.Error()
			}
		}
		rows = append(rows, r)
	}
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(rows)
	}
	for _, r := range rows {
		ver := r.Version
		if ver == "" {
			ver = "-"
		}
		fmt.Printf("%-10s %-13s %-12s %s\n", r.ID, r.State, ver, r.Bin)
		if r.Detail != "" {
			fmt.Printf("           %s\n", r.Detail)
		}
		if !r.Supported {
			fmt.Printf("           %s\n", r.Why)
		}
		if r.Config != "" {
			fmt.Printf("           config: %s\n", r.Config)
		}
		if c := r.LastCheck; c != nil {
			fmt.Printf("           last check %s: ok=%v %s\n", c.At.Local().Format("2006-01-02 15:04"), c.OK, c.Msg)
		}
		fmt.Printf("           preflight: %s\n", r.Preflight)
	}
	return nil
}

func modelsCmd() error {
	list := app.New().Installed(context.Background(), pref())
	if len(list) == 0 {
		fmt.Println("No models downloaded yet. Run: ezlm")
		return nil
	}
	for _, m := range list {
		mark, how := "  ", "doesn't fit in free memory right now"
		if m.Running {
			mark = "▶ "
		}
		if m.Plan != nil {
			how = fmt.Sprintf("%s, ~%.0f tok/s, %dk context", m.Plan.Mode, m.Plan.TokPerSec, m.Plan.Ctx/1024)
		}
		fmt.Printf("%s%-28s %-22s %7s  %s\n", mark, m.ModelID, m.Quant, gb(m.Bytes), how)
	}
	fmt.Println("\nSwitch with: ezlm use <model-id>")
	return nil
}
