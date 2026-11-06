package integrate

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/picom-code/ezlm/internal/fakellm"
)

// TestHermesRealSetup runs Setup (find, configure, model-and-tool check,
// preflight, TUI smoke) against a real Hermes and a fake model server whose
// template takes only "low" and "xhigh", so Hermes's default "medium" would
// fail without EZLM's override. Opt in with:
//
//	EZLM_REAL_HERMES=<folder holding hermes> EZLM_REAL_HERMES_ROOT=<scratch folder> go test -run HermesRealSetup ./internal/integrate/
//
// EZLM_REAL_HERMES_SLOW=1 adds the run without EZLM's reasoning override.
//
// Every Hermes and EZLM folder points into the scratch folder, never the
// real home. A data folder Hermes hasn't seen first sets itself up (a few
// hundred MB), so reuse the same scratch folder between runs.
func TestHermesRealSetup(t *testing.T) {
	binDir, root := os.Getenv("EZLM_REAL_HERMES"), os.Getenv("EZLM_REAL_HERMES_ROOT")
	if binDir == "" || root == "" {
		t.Skip("set EZLM_REAL_HERMES and EZLM_REAL_HERMES_ROOT to run against a real Hermes")
	}
	home := filepath.Join(root, "home")
	for k, v := range map[string]string{
		"HOME": home, "USERPROFILE": home,
		"XDG_CONFIG_HOME": filepath.Join(home, ".config"), "XDG_DATA_HOME": filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME": filepath.Join(home, ".local", "state"), "XDG_CACHE_HOME": filepath.Join(home, ".cache"),
		"HERMES_HOME": filepath.Join(home, ".hermes"), "LOCALAPPDATA": filepath.Join(home, "AppData", "Local"),
		"DSH_HOME": filepath.Join(home, ".dsh"), "OPENCODE_CONFIG_DIR": filepath.Join(home, ".config", "opencode"),
		"EZLM_HOME": filepath.Join(root, "ezlm"), "EZLM_TOOL_PATH": binDir, "PYTHONDONTWRITEBYTECODE": "1", "GITHUB_TOKEN": "",
	} {
		t.Setenv(k, v)
	}
	t.Cleanup(func() { hermesTestStopGateway(t, filepath.Join(home, ".hermes")) })
	os.MkdirAll(filepath.Join(home, ".hermes"), 0o700)
	os.MkdirAll(Workspace(), 0o755) // the TUI smoke runs there (Setup creates it too)

	var reqLog strings.Builder
	fake := fakellm.New(fakellm.Options{Key: "ezlm-realtest123", Alias: "fake-model", Ctx: 65536, Levels: []string{"low", "xhigh"}, Log: &reqLog})
	srv := httptest.NewServer(fake)
	defer srv.Close()
	ep := Endpoint{BaseURL: srv.URL + "/v1", Key: "ezlm-realtest123", ModelID: "fake-model", ModelName: "Fake Model", Ctx: 65536, ToolCalls: true,
		Reasoning: Reasoning{Levels: []string{"low", "xhigh"}}}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	var steps []string
	err := Setup(ctx, "hermes", ep, true, func(m string) { steps = append(steps, m) })
	logb, _ := os.ReadFile(LogPath("hermes"))
	t.Logf("steps: %q", steps)
	t.Logf("integrate-hermes.log:\n%s", logb)
	for _, rq := range fake.Requests() {
		b, _ := json.Marshal(rq)
		t.Logf("request: %s", b)
	}
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	c := LastCheck("hermes")
	if c == nil || !c.OK || !c.ToolCall {
		t.Fatalf("last check: %+v", c)
	}
	for _, rq := range fake.Requests() {
		if rq.ReasoningEffort != "" && rq.ReasoningEffort != "none" && rq.ReasoningEffort != "low" {
			t.Errorf("Hermes sent reasoning_effort %q", rq.ReasoningEffort)
		}
	}
	for _, tg := range Targets(&ep) {
		if tg.ID == "hermes" && tg.State != StateConnected {
			t.Errorf("target after setup: %+v", tg)
		}
	}
	cfgPath := filepath.Join(home, ".hermes", "config.yaml")
	cfg, _ := os.ReadFile(cfgPath)
	t.Logf("config.yaml EZLM wrote (key redacted):\n%s", Redact(string(cfg), ep.Key))

	// A fresh Hermes home asks about shared metrics before its first chat;
	// with that answered (as a user would), the chat itself must be ready.
	doc, _ := yamlLoad(cfgPath)
	yamlSet(doc.Content[0], "telemetry", yamlMap("shared_metrics", yamlMap("enabled", yamlBool(false), "send", yamlBool(false), "offer_version", yamlInt(2))))
	b, _ := yamlEncode(doc)
	os.WriteFile(cfgPath, b, 0o600)
	f, err := hermesTool{}.Resolve(ctx, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	spec, _ := hermesTool{}.LaunchSpec(f, ep, ModeTUI)
	fillSpec(&spec, hermesTool{}.Info(), ModeTUI)
	sm, err := Smoke(ctx, spec, 25*time.Second)
	t.Logf("chat smoke: ok=%v alive=%v ready=%v fatal=%q err=%v\n%s", sm.OK, sm.Alive, sm.Ready, sm.Fatal, err, sm.Screen)
	if err != nil || !sm.OK {
		t.Fatalf("the chat didn't come up")
	}

	// The same launch with the modern TUI as the user's interface.
	doc, _ = yamlLoad(cfgPath)
	yamlSet(doc.Content[0], "display", yamlMap("interface", yamlStr("tui")))
	b, _ = yamlEncode(doc)
	os.WriteFile(cfgPath, b, 0o600)
	sm, err = Smoke(ctx, spec, 60*time.Second)
	t.Logf("modern TUI smoke: ok=%v alive=%v ready=%v fatal=%q err=%v\n%s", sm.OK, sm.Alive, sm.Ready, sm.Fatal, err, sm.Screen)
	if err != nil || !sm.OK {
		t.Fatalf("the modern TUI didn't come up")
	}

	// Without EZLM's override Hermes asks for its default "medium", which
	// this template rejects; the check says so in plain words. Hermes
	// retries for about five minutes first, so this part is opt-in too.
	if os.Getenv("EZLM_REAL_HERMES_SLOW") == "" {
		return
	}
	doc, _ = yamlLoad(cfgPath)
	yamlDelete(doc.Content[0], "agent")
	b, _ = yamlEncode(doc)
	os.WriteFile(cfgPath, b, 0o600)
	w, _ := NewCheckWorkspace()
	defer w.Close()
	_, err = hermesTool{}.Check(ctx, f, ep, w, nil)
	sent := map[string]int{}
	for _, rq := range fake.Requests() {
		sent[rq.ReasoningEffort]++
	}
	t.Logf("without the override: %v; reasoning_effort sent: %v", err, sent)
	if e, _ := Explain("hermes", "Unexpected reasoning effort"); err == nil || err.Error() != e.Say || sent["medium"] == 0 {
		t.Fatalf("expected the reasoning sentence and a medium request")
	}
}

// hermesTestStopGateway stops the idle gateway a `hermes chat` starts for
// the scratch home, by the PID it recorded, only when that record names
// this home. (`hermes gateway stop` is avoided: it can reach the account's
// own service.)
func hermesTestStopGateway(t *testing.T, hermesHome string) {
	b, err := os.ReadFile(filepath.Join(hermesHome, "gateway.pid"))
	if err != nil {
		return
	}
	var rec struct {
		PID  int    `json:"pid"`
		Home string `json:"hermes_home"`
	}
	if json.Unmarshal(b, &rec) != nil || rec.PID <= 1 || !hermesSamePath(rec.Home, hermesHome) {
		return
	}
	if p, err := os.FindProcess(rec.PID); err == nil && p.Signal(syscall.Signal(0)) == nil {
		t.Logf("stopping the scratch home's gateway, pid %d", rec.PID)
		p.Signal(syscall.SIGTERM)
	}
}
