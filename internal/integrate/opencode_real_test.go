package integrate

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/picom-code/ezlm/internal/fakellm"
)

// TestOpenCodeReal runs a real OpenCode build end to end against the fake
// model, inside this package's throwaway home (main_test.go):
//
//	EZLM_TEST_OPENCODE=~/.local/bin/opencode go test -run OpenCodeReal ./internal/integrate/
//
// Setup does it all: probe, config, the model-and-tool check, the preflight
// and the TUI smoke in a pseudo-terminal. 2.x runs only with --standalone,
// so no background service is started or reached. With
// EZLM_OPENCODE_RECORD=<dir> it also records `opencode run --format json`
// for the failure cases (paths and ports normalised) as the fixtures in
// testdata/opencode.
func TestOpenCodeReal(t *testing.T) {
	bin := os.Getenv("EZLM_TEST_OPENCODE")
	if bin == "" {
		t.Skip("set EZLM_TEST_OPENCODE to an opencode binary")
	}
	tools := t.TempDir()
	if err := os.Symlink(bin, filepath.Join(tools, "opencode")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EZLM_TOOL_PATH", tools)
	const key = "ezlm-RealTestKey0123456789"
	fake := fakellm.New(fakellm.Options{Key: key, Alias: "qwen3.5-4b", Levels: []string{"low", "medium"}})
	srv := httptest.NewServer(fake)
	defer srv.Close()
	ep := Endpoint{BaseURL: srv.URL + "/v1", Key: key, ModelID: "qwen3.5-4b", ModelName: "Qwen3.5 4B", Ctx: 65536, ToolCalls: true,
		Reasoning: Reasoning{Levels: []string{"low", "medium"}}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	var steps []string
	err := Setup(ctx, "opencode", ep, true, func(s string) { steps = append(steps, s) })
	for _, l := range LogTail("opencode", 400) {
		if strings.Contains(l, key) {
			t.Fatalf("key in the log: %s", l)
		}
	}
	if err != nil {
		t.Fatalf("setup: %v\nsteps: %q\nlog:\n%s", err, steps, strings.Join(LogTail("opencode", 120), "\n"))
	}
	var smoked bool
	for _, l := range LogTail("opencode", 400) {
		if strings.Contains(l, "TUI smoke: ok=true") {
			smoked = true
		}
	}
	if !smoked {
		t.Fatalf("no passing TUI smoke in the log:\n%s", strings.Join(LogTail("opencode", 60), "\n"))
	}
	lc := LastCheck("opencode")
	if lc == nil || !lc.OK || !lc.ToolCall {
		t.Fatalf("last check = %+v", lc)
	}
	var sawNonce bool
	for _, r := range fake.Requests() {
		if strings.Contains(r.ToolResult, "EZLM-CHECK-") {
			sawNonce = true
		}
		// The default request carries no level; a variant only an accepted one.
		if r.ReasoningEffort != "" && r.ReasoningEffort != "low" && r.ReasoningEffort != "medium" {
			t.Errorf("request sent reasoning_effort %q", r.ReasoningEffort)
		}
		if r.Status != 200 && r.Path != "/health" {
			t.Errorf("request failed: %+v", r)
		}
	}
	if !sawNonce {
		t.Fatalf("no tool result with the nonce reached the model: %+v", fake.Requests())
	}
	f, err := (openCodeTool{}).Resolve(ctx, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("OpenCode %s (%s, %s) checked in %.1f s; steps %q", f.Version, f.Source, f.Arch, lc.Secs, steps)
	if d := (openCodeTool{}).Detect(&ep); d.State != "" || !d.Linked {
		t.Fatalf("detect after setup: %+v", d)
	}

	// The preflight and the smoke opened the real (test) data folder, so the
	// database there is in this build's own format.
	data := openCodeDBFormat(openCodeDataDir())
	v2 := openCodeAtLeast(f.Version, "2.0.0")
	if (v2 && data != openCodeDataV2) || (!v2 && data != openCodeDataV1) {
		t.Fatalf("OpenCode %s wrote a %s database", f.Version, data)
	}
	if !v2 {
		// The laptop case: a newer OpenCode's history appears. 1.x must not
		// be used for it, and its own error text maps to the sentence.
		db := filepath.Join(openCodeDataDir(), "opencode.db")
		for _, s := range []string{"", "-wal", "-shm"} {
			os.Remove(db + s)
		}
		openCodeTestDB(t, "db-v2.sqlite.gz", openCodeDataDir(), "opencode.db")
		if _, err := (openCodeTool{}).Resolve(ctx, nil, false); !errors.Is(err, ErrNotInstalled) {
			t.Fatalf("1.x with v2 history: %v", err)
		}
		out, _ := exec.Command(bin, "models", "ezlm").CombinedOutput()
		if e, ok := Explain("opencode", string(out)); !ok || !strings.Contains(e.Say, "2.2.5") {
			t.Fatalf("1.x on v2 history said %q", out)
		}
	}

	if dir := os.Getenv("EZLM_OPENCODE_RECORD"); dir != "" {
		openCodeRecord(t, ctx, f, ep, srv.URL, dir)
	}
}

// openCodeRecord runs the check's exact command for the cases the error map
// covers and saves what OpenCode printed.
func openCodeRecord(t *testing.T, ctx context.Context, f Found, ep Endpoint, url, dir string) {
	ver := f.Version
	run := func(name string, ep Endpoint, model string) {
		w, err := NewCheckWorkspace()
		if err != nil {
			t.Fatal(err)
		}
		defer w.Close()
		fl := openCodeFlagsOf(f)
		args := []string{"run"}
		if fl.standalone {
			args = append(args, "--standalone")
		} else {
			args = append(args, "--print-logs", "--log-level", "ERROR")
		}
		args = append(args, "--model", model, "--format", "json", w.Prompt())
		if err := WriteOpenCodeConfig(openCodeConfigDir(), ep, false); err != nil {
			t.Fatal(err)
		}
		tmp := t.TempDir()
		cmd := openCodeCmd(f, w.Dir, []string{"XDG_DATA_HOME=" + tmp + "/d", "XDG_STATE_HOME=" + tmp + "/s", "XDG_CACHE_HOME=" + tmp + "/c", "OPENCODE_DISABLE_MODELS_FETCH=1"}, args...)
		out, _ := NewRun("opencode-record", nil).Exec(ctx, 4*time.Minute, cmd)
		out = strings.ReplaceAll(out, w.Nonce, "EZLM-CHECK-NONCE0AB")
		if real, err := filepath.EvalSymlinks(w.Dir); err == nil {
			out = strings.ReplaceAll(out, real, "/tmp/ezlm-check-0000")
		}
		out = strings.ReplaceAll(out, w.Dir, "/tmp/ezlm-check-0000")
		out = strings.ReplaceAll(out, strings.TrimSuffix(url, "/"), "http://127.0.0.1:8081")
		out = openCodeRecordDates.ReplaceAllString(out, "Thu, 05 Nov 2026 12:00:00 GMT")
		out = Redact(out, ep.Key)
		path := filepath.Join(dir, "run-"+ver+"-"+name+".jsonl")
		if err := os.WriteFile(path, []byte(out+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("recorded %s (%d bytes)", path, len(out))
	}
	model := openCodeProvider + "/" + ep.ModelID
	run("ok", ep, model)
	if openCodeAtLeast(ver, "2.0.0") {
		run("variant", ep, model+"#medium")
		run("variant-missing", ep, model+"#high")
	}
	run("model-missing", ep, openCodeProvider+"/nope")
	bad := ep
	bad.Key = "ezlm-WrongKeyWrongKey99"
	run("401", bad, model)
	down := ep
	down.BaseURL = "http://127.0.0.1:9/v1"
	run("refused", down, model)
	WriteOpenCodeConfig(openCodeConfigDir(), ep, false)
}

var openCodeRecordDates = regexp.MustCompile(`[A-Z][a-z]{2}, \d{2} [A-Z][a-z]{2} \d{4} \d{2}:\d{2}:\d{2} GMT`)

// TestOpenCodeRealInstall installs the pinned OpenCode from a downloaded
// copy of its real release archive, served locally under the pinned digest:
//
//	EZLM_TEST_OPENCODE_ZIP=opencode-darwin-arm64.zip go test -run OpenCodeRealInstall ./internal/integrate/
func TestOpenCodeRealInstall(t *testing.T) {
	arc := os.Getenv("EZLM_TEST_OPENCODE_ZIP")
	if arc == "" {
		t.Skip("set EZLM_TEST_OPENCODE_ZIP to a pinned OpenCode release archive")
	}
	t.Setenv("EZLM_TOOL_PATH", t.TempDir()) // the user has no opencode
	old, _ := PinFor("opencode")
	key := openCodeAssetKey()
	a, ok := old.Assets[key]
	if !ok {
		t.Skipf("no pin for %s", key)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, arc) }))
	defer srv.Close()
	p := old
	p.Assets = map[string]Asset{key: {URL: srv.URL + "/" + filepath.Base(a.URL), SHA256: a.SHA256, Size: a.Size}}
	RegisterPin(p)
	defer RegisterPin(old)
	defer os.RemoveAll(ToolDir("opencode", old.Version))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	f, err := (openCodeTool{}).Resolve(ctx, NewRun("opencode", nil), true)
	if err != nil {
		t.Fatalf("install: %v\n%s", err, strings.Join(LogTail("opencode", 40), "\n"))
	}
	if f.Bin != openCodePinnedBin() || f.Version != old.Version || f.Source != SourceEZLM || f.Arch != MachineArch() {
		t.Fatalf("found %+v", f)
	}
	if fl := openCodeFlagsOf(f); !fl.standalone || fl.tuiModel {
		t.Fatalf("flags %+v", fl)
	}
	t.Logf("installed %s (%s)", f.Bin, f.Arch)
}
