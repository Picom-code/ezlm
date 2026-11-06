package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/picom-code/ezlm/internal/catalog"
	"github.com/picom-code/ezlm/internal/integrate"
	"github.com/picom-code/ezlm/internal/serve"
)

// appTool is a fake connectable app: it records what the app layer asked of
// it and keeps an in-memory "config" so Detect can tell linked from stale.
type appTool struct {
	id, name string
	minCtx   int

	mu           sync.Mutex
	calls        []string
	linked       bool
	cfgKey       string
	cfgCtx       int
	configureErr error
	preflightErr error
}

func (f *appTool) Info() integrate.Info {
	return integrate.Info{ID: f.id, Name: f.name, Icon: f.id, Order: map[string]int{"opencode": 10, "hermes": 20, "dsh": 30}[f.id],
		MinCtx: f.minCtx, Launch: []integrate.LaunchMode{{ID: integrate.ModeTUI, Label: "Open " + f.name}}}
}

func (f *appTool) record(c string) {
	f.mu.Lock()
	f.calls = append(f.calls, c)
	f.mu.Unlock()
}

func (f *appTool) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

func (f *appTool) Detect(ep *integrate.Endpoint) integrate.Detection {
	f.mu.Lock()
	defer f.mu.Unlock()
	d := integrate.Detection{Found: integrate.Found{Bin: "/fake/" + f.id, Version: "1.0", Source: integrate.SourcePath}, Linked: f.linked}
	if f.linked && ep != nil && (f.cfgKey != ep.Key || f.cfgCtx != ep.Ctx) {
		d.State, d.Detail = integrate.StateNeedsUpdate, "Points at an old address or key."
	}
	return d
}

func (f *appTool) Resolve(context.Context, *integrate.Run, bool) (integrate.Found, error) {
	f.record("resolve")
	return integrate.Found{Bin: "/fake/" + f.id}, nil
}

func (f *appTool) Configure(ep integrate.Endpoint, makeDefault bool) error {
	f.record(fmt.Sprintf("configure ctx=%d default=%v", ep.Ctx, makeDefault))
	if f.minCtx > 0 && ep.Ctx < f.minCtx {
		return integrate.ErrNeeds64K
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.configureErr != nil {
		return f.configureErr
	}
	f.linked, f.cfgKey, f.cfgCtx = true, ep.Key, ep.Ctx
	return nil
}

func (f *appTool) Check(_ context.Context, _ integrate.Found, _ integrate.Endpoint, w *integrate.CheckWorkspace, _ *integrate.Run) (bool, error) {
	f.record("check")
	return true, nil
}

func (f *appTool) Preflight(context.Context, integrate.Found, integrate.Endpoint, *integrate.Run) error {
	f.record("preflight")
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.preflightErr
}

func (f *appTool) LaunchSpec(integrate.Found, integrate.Endpoint, string) (integrate.LaunchSpec, error) {
	f.record("spec")
	return integrate.LaunchSpec{Bin: "/fake/" + f.id}, nil
}

// appWorld swaps in fake apps, a saved config and a server that answers
// (or not), and turns integrations on for one test.
type appWorld struct {
	oc, hm, ds *appTool
	cfg        *serve.Config
	healthy    bool
	started    int
	opened     []string
	openErr    error
	exitCode   int // >= 0: the fake launcher writes this exit code right away
}

func newAppWorld(t *testing.T) *appWorld {
	t.Helper()
	t.Setenv("EZLM_NO_INTEGRATIONS", "")
	w := &appWorld{
		oc:       &appTool{id: "opencode", name: "OpenCode"},
		hm:       &appTool{id: "hermes", name: "Hermes", minCtx: 64000},
		ds:       &appTool{id: "dsh", name: "DeepSeek Harness"},
		healthy:  true,
		exitCode: -1,
	}
	for _, f := range []*appTool{w.oc, w.hm, w.ds} {
		t.Cleanup(integrate.Replace(f))
	}
	w.cfg = &serve.Config{ModelID: "m1", ModelName: "Model One", Port: 18555, Ctx: 32768, ToolCalls: true,
		Keys: []serve.APIKey{{ID: "k1", Name: "Default", Key: "ezlm-AppTestKeyOne12345"}}, Key: "ezlm-AppTestKeyOne12345"}
	if err := w.cfg.Save(); err != nil {
		t.Fatal(err)
	}
	os.Remove(serve.StoppedFlag())
	oldH, oldW, oldS, oldP, oldO, oldC, oldI, oldWait := serverHealthy, serverWait, serverStart, fetchProps, openApp, loadCatalog, installService, launchWait
	t.Cleanup(func() {
		serverHealthy, serverWait, serverStart, fetchProps, openApp, loadCatalog, installService, launchWait = oldH, oldW, oldS, oldP, oldO, oldC, oldI, oldWait
	})
	serverHealthy = func(context.Context, *serve.Config) bool { return w.healthy }
	serverWait = func(context.Context, *serve.Config, time.Duration, func(time.Duration)) error {
		if !w.healthy {
			return errors.New("the model server stopped while loading")
		}
		return nil
	}
	serverStart = func(_ *App, _ context.Context, _ *serve.Config) error {
		w.started++
		w.healthy = true
		return nil
	}
	fetchProps = func(context.Context, *serve.Config) (*serve.Props, error) {
		return &serve.Props{Ctx: 32768, Levels: []string{"low", "medium"}, Thinking: true}, nil
	}
	loadCatalog = func(context.Context) (*catalog.Catalog, string) { return nil, "" }
	installService = func(*serve.Config) error { return nil }
	launchWait = 2 * time.Second
	openApp = func(_ context.Context, id, mode string, ep integrate.Endpoint, _ integrate.Log) (integrate.Launched, error) {
		w.opened = append(w.opened, id+":"+mode)
		if w.openErr != nil {
			return integrate.Launched{}, w.openErr
		}
		if w.exitCode < 0 {
			return integrate.Launched{DryRun: true}, nil
		}
		exit := integrate.ExitFile(id)
		os.MkdirAll(filepath.Dir(exit), 0o755)
		os.WriteFile(exit, []byte(fmt.Sprintf("%d %d\n", w.exitCode, time.Now().Unix())), 0o600)
		return integrate.Launched{Launcher: exit + ".sh", ExitFile: exit}, nil
	}
	return w
}

// runJob starts a job and collects its events until it's done.
func runJob(t *testing.T, a *App, start func() error) ([]Event, Event) {
	t.Helper()
	ch, cancel := a.Subscribe()
	defer cancel()
	if err := start(); err != nil {
		t.Fatal(err)
	}
	var evs []Event
	for {
		select {
		case e := <-ch:
			if e.Step == "done" {
				return evs, e
			}
			evs = append(evs, e)
		case <-time.After(30 * time.Second):
			t.Fatalf("the job didn't finish; events so far: %+v", evs)
		}
	}
}

func msgs(evs []Event) string {
	var b strings.Builder
	for _, e := range evs {
		fmt.Fprintf(&b, "%s/%s: %s\n", e.Step, e.State, e.Msg)
	}
	return b.String()
}

func targetOf(ts []integrate.Target, id string) integrate.Target {
	for _, t := range ts {
		if t.ID == id {
			return t
		}
	}
	return integrate.Target{}
}

// A model switch to less than 64K rewrites OpenCode and DeepSeek Harness but
// never Hermes, which shows needs_64k instead; a failed rewrite stays on its
// tile.
func TestRewriteLinkedKeepsHermesBelow64K(t *testing.T) {
	w := newAppWorld(t)
	for _, f := range []*appTool{w.oc, w.hm, w.ds} {
		f.linked, f.cfgKey, f.cfgCtx = true, "ezlm-OldKeyOld12345", 65536
	}
	w.ds.configureErr = errors.New("DeepSeek Harness's settings file is not valid YAML. Fix or remove it, then press Connect.")
	a := New()
	var said []string
	names := a.rewriteLinked(w.cfg, func(m string) { said = append(said, m) })
	if !slices.Equal(names, []string{"OpenCode"}) {
		t.Fatalf("updated %v; said %v", names, said)
	}
	if got := w.hm.Calls(); len(got) != 0 {
		t.Fatalf("Hermes must not be rewritten below 64K: %v", got)
	}
	if !slices.ContainsFunc(said, func(s string) bool { return strings.HasPrefix(s, "Hermes needs a 64K context.") }) {
		t.Fatalf("said %v", said)
	}
	st := a.Status(context.Background())
	if h := targetOf(st.Targets, "hermes"); h.State != integrate.StateNeeds64K || !strings.Contains(h.Detail, "64K") {
		t.Fatalf("hermes = %+v", h)
	}
	if o := targetOf(st.Targets, "opencode"); o.State != integrate.StateConnected {
		t.Fatalf("opencode = %+v", o)
	}
	if d := targetOf(st.Targets, "dsh"); d.State != integrate.StateNeedsUpdate || d.Detail != w.ds.configureErr.Error() {
		t.Fatalf("dsh must show why it wasn't updated: %+v", d)
	}
	w.ds.configureErr = nil
	a.rewriteLinked(w.cfg, func(string) {})
	if d := targetOf(a.Status(context.Background()).Targets, "dsh"); d.State != integrate.StateConnected || d.Detail != "" {
		t.Fatalf("a later rewrite clears the error: %+v", d)
	}
}

// Rotating the key rewrites every connected app and asks for open windows
// to be reopened.
func TestRotateKeyRewritesAllApps(t *testing.T) {
	w := newAppWorld(t)
	w.cfg.Ctx = 65536
	w.cfg.Save()
	for _, f := range []*appTool{w.oc, w.hm, w.ds} {
		f.linked, f.cfgKey, f.cfgCtx = true, w.cfg.Key, 65536
	}
	a := New()
	evs, done := runJob(t, a, func() error { return a.RotateKey("k1") })
	if done.State != "ok" {
		t.Fatalf("rotate: %+v\n%s", done, msgs(evs))
	}
	cfg, _ := serve.Load()
	for _, f := range []*appTool{w.oc, w.hm, w.ds} {
		if f.cfgKey != cfg.Key || cfg.Key == "ezlm-AppTestKeyOne12345" {
			t.Fatalf("%s still has the old key", f.name)
		}
	}
	if !strings.Contains(msgs(evs), "If OpenCode, Hermes or DeepSeek Harness is already open, close it and open it again") {
		t.Fatalf("no reopen note:\n%s", msgs(evs))
	}
	if strings.Contains(msgs(evs), cfg.Key) {
		t.Fatal("a key reached the job events")
	}
}

// Connect with the server stopped starts it first, saves what /props says,
// and then sets the app up; a server that can't start fails at once.
func TestIntegrateStartsTheServerFirst(t *testing.T) {
	w := newAppWorld(t)
	w.cfg.Ctx = 65536
	w.cfg.Save()
	w.healthy = false
	os.WriteFile(serve.StoppedFlag(), []byte("stopped by the user\n"), 0o600)
	a := New()
	evs, done := runJob(t, a, func() error { return a.Integrate([]string{"opencode", "--default"}[:1], true) })
	if done.State != "ok" || w.started != 1 {
		t.Fatalf("integrate: %+v started=%d\n%s", done, w.started, msgs(evs))
	}
	if !strings.Contains(msgs(evs), "integrate:opencode/run: Starting your model server") {
		t.Fatalf("no server step:\n%s", msgs(evs))
	}
	if _, err := os.Stat(serve.StoppedFlag()); err == nil {
		t.Fatal("connecting an app must clear the user's stop")
	}
	if got := strings.Join(w.oc.Calls(), ","); got != "resolve,configure ctx=65536 default=true,check,preflight,spec" {
		t.Fatalf("calls = %s", got)
	}
	if cfg, _ := serve.Load(); cfg.Props == nil || !slices.Equal(cfg.Props.Levels, []string{"low", "medium"}) {
		t.Fatalf("props not saved: %+v", cfg.Props)
	}
	if !slices.Contains(integrate.KnownPorts(), w.cfg.Port) {
		t.Fatalf("port not noted: %v", integrate.KnownPorts())
	}

	w.healthy = false
	serverStart = func(*App, context.Context, *serve.Config) error {
		return errors.New("the model server couldn't start: there isn't enough free memory for this model right now")
	}
	evs, done = runJob(t, a, func() error { return a.Integrate([]string{"dsh"}, false) })
	if done.State != "fail" || !strings.Contains(done.Msg, "free memory") || len(w.ds.Calls()) != 0 {
		t.Fatalf("a dead server must fail before any check: %+v %v\n%s", done, w.ds.Calls(), msgs(evs))
	}
}

// Opening with the server stopped starts it; a config that points at an
// old key is rewritten and preflighted before the app opens.
func TestLaunchRewritesStaleConfigFirst(t *testing.T) {
	w := newAppWorld(t)
	w.cfg.Ctx = 65536
	w.cfg.Save()
	w.oc.linked, w.oc.cfgKey, w.oc.cfgCtx = true, "ezlm-OldKeyOld12345", 65536
	w.healthy = false
	a := New()
	evs, done := runJob(t, a, func() error { return a.Launch("opencode", "") })
	if done.State != "ok" || w.started != 1 {
		t.Fatalf("launch: %+v\n%s", done, msgs(evs))
	}
	if got := strings.Join(w.oc.Calls(), ","); got != "configure ctx=65536 default=false,resolve,preflight" {
		t.Fatalf("calls = %s", got)
	}
	if !slices.Equal(w.opened, []string{"opencode:tui"}) {
		t.Fatalf("opened %v", w.opened)
	}
	if !strings.Contains(msgs(evs), "open/info: Updating OpenCode to use Model One") {
		t.Fatalf("events:\n%s", msgs(evs))
	}
}

// Hermes below 64K refuses to open with the 64K sentence; an app that isn't
// connected yet is set up first.
func TestLaunchRefusesBelow64KAndConnectsFirst(t *testing.T) {
	w := newAppWorld(t)
	w.hm.linked, w.hm.cfgKey, w.hm.cfgCtx = true, w.cfg.Key, 65536
	a := New()
	_, done := runJob(t, a, func() error { return a.Launch("hermes", "") })
	if done.State != "fail" || done.Msg != integrate.NeedsCtxDetail("Hermes", 64000) || len(w.opened) != 0 {
		t.Fatalf("hermes: %+v opened %v", done, w.opened)
	}
	evs, done := runJob(t, a, func() error { return a.Launch("dsh", "") })
	if done.State != "ok" || !strings.Contains(msgs(evs), "connect/ok: DeepSeek Harness is connected") {
		t.Fatalf("dsh: %+v\n%s", done, msgs(evs))
	}
}

// An app that exits right after opening fails the launch with the reason
// (the preflight run again), the tile turns broken, and the next good launch
// brings the last check back.
func TestLaunchEarlyExit(t *testing.T) {
	w := newAppWorld(t)
	w.cfg.Ctx = 65536
	w.cfg.Save()
	w.oc.linked, w.oc.cfgKey, w.oc.cfgCtx = true, w.cfg.Key, 65536
	good := integrate.CheckResult{At: time.Now(), OK: true, ToolCall: true, Secs: 9, Stage: integrate.StageModel, Msg: "answered and used a tool (9 s)"}
	integrate.SaveCheck("opencode", good)
	w.oc.preflightErr = errors.New("OpenCode 1.x can't open the chat history a newer OpenCode wrote; EZLM will use OpenCode 2.2.5.")
	w.exitCode = 1
	a := New()
	_, done := runJob(t, a, func() error { return a.Launch("opencode", "") })
	want := "OpenCode closed right after opening (exit code 1): OpenCode 1.x can't open the chat history"
	if done.State != "fail" || !strings.HasPrefix(done.Msg, want) {
		t.Fatalf("early exit: %+v", done)
	}
	tg := targetOf(a.Status(context.Background()).Targets, "opencode")
	if tg.State != integrate.StateBroken || tg.LastCheck == nil || tg.LastCheck.Stage != integrate.StageLaunch {
		t.Fatalf("after a failed launch: %+v", tg)
	}

	w.oc.preflightErr = nil
	_, done = runJob(t, a, func() error { return a.Launch("opencode", "") })
	if done.State != "fail" || !strings.Contains(done.Msg, "(exit code 1). Press Report a problem") {
		t.Fatalf("no reason known: %+v", done)
	}

	w.exitCode = 0
	_, done = runJob(t, a, func() error { return a.Launch("opencode", "") })
	if done.State != "ok" {
		t.Fatalf("clean exit: %+v", done)
	}
	tg = targetOf(a.Status(context.Background()).Targets, "opencode")
	if tg.State != integrate.StateConnected || tg.LastCheck == nil || tg.LastCheck.Msg != good.Msg {
		t.Fatalf("a good launch must bring the last check back: %+v %+v", tg, tg.LastCheck)
	}
}

// A problem report carries each app's integration log and last exit, with
// keys removed.
func TestReportAttachesAppLogs(t *testing.T) {
	w := newAppWorld(t)
	r := integrate.NewRun("opencode", nil)
	r.Logf("open opencode: key %s", w.cfg.Key)
	r.Close()
	os.MkdirAll(integrate.LaunchDir(), 0o755)
	os.WriteFile(integrate.ExitFile("opencode"), []byte("1 1762380000\n"), 0o600)
	got := integrate.Redact(appAttachments(), w.cfg.Key)
	if !strings.Contains(got, "OpenCode (integrate-opencode.log)") || !strings.Contains(got, "OpenCode last exit (launch/opencode.exit): 1 1762380000") {
		t.Fatalf("attachments:\n%s", got)
	}
	if strings.Contains(got, w.cfg.Key) {
		t.Fatal("key leaked into the report")
	}
}

func TestRestartNoteAndCtxFor(t *testing.T) {
	if restartNote(nil) != "" || restartNote([]string{"Hermes"}) != "If Hermes is already open, close it and open it again so it uses the new settings." {
		t.Fatal(restartNote([]string{"Hermes"}))
	}
	if ctxFor(64000) != 65536 || ctxFor(65536) != 65536 || ctxFor(32000) != 32768 {
		t.Fatal("ctxFor")
	}
}
