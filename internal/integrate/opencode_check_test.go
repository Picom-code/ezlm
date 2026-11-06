package integrate

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func openCodeTestRecording(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "opencode", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The recordings are `opencode run --format json` from the real 1.19.5 and
// 2.2.5 against the fake model (TestOpenCodeReal with EZLM_OPENCODE_RECORD).
func TestOpenCodeParseRecordings(t *testing.T) {
	for _, v := range []string{"1.19.5", "2.2.5"} {
		res := parseOpenCodeRun(openCodeTestRecording(t, "run-"+v+"-ok.jsonl"))
		if !slices.Equal(res.Tools, []string{"read"}) || len(res.Errors) != 0 || len(res.Texts) != 1 {
			t.Fatalf("%s ok: %+v", v, res)
		}
		w := &CheckWorkspace{Nonce: "EZLM-CHECK-NONCE0AB"}
		if !w.Passed(res.Texts[0]) {
			t.Fatalf("%s: nonce not in the answer %q", v, res.Texts[0])
		}
	}
	for _, tc := range []struct{ file, say string }{
		{"run-2.2.5-401.jsonl", "old EZLM key"},
		{"run-1.19.5-401.jsonl", "old EZLM key"},
		{"run-2.2.5-model-missing.jsonl", "model EZLM isn't serving"},
		{"run-1.19.5-model-missing.jsonl", "model EZLM isn't serving"},
		{"run-2.2.5-variant-missing.jsonl", "thinking level"},
		{"run-2.2.5-refused.jsonl", "isn't running"},
		{"run-1.19.5-refused.jsonl", "isn't running"},
	} {
		out := openCodeTestRecording(t, tc.file)
		res := parseOpenCodeRun(out)
		if len(res.Errors) == 0 {
			t.Errorf("%s: no error event", tc.file)
		}
		err := openCodeCheckErr(out, res, nil)
		if err == nil || !strings.Contains(err.Error(), tc.say) {
			t.Errorf("%s: %v", tc.file, err)
		}
	}
}

func TestOpenCodeExplain(t *testing.T) {
	for out, want := range map[string]string{
		"Error: Unexpected error\n\nDatabase is not empty and has no session table":      "OpenCode 2.2.5",
		"Failed to initialize OpenTUI render library: bun:ffi dlopen() is not available": "can't draw its window",
		"ERROR\n  Unrecognized flag: --model in command opencode":                        "doesn't take the options",
		`{"error":{"message":"Jinja Exception: Unexpected reasoning effort high"}}`:      "thinking level",
		`{"type":"error","error":{"type":"provider.auth","message":"Invalid API Key"}}`:  "old EZLM key",
		"ProviderModelNotFoundError: Model not found: ezlm/old.":                         "Press Connect to update it",
		"ECONNREFUSED: Unable to connect. Is the computer able to access the url?":       "isn't running",
	} {
		e, ok := Explain("opencode", out)
		if !ok || !strings.Contains(e.Say, want) || strings.ContainsAny(e.Say, "—") {
			t.Errorf("%q: %+v", out, e)
		}
	}
}

func TestOpenCodeCheckWithFake(t *testing.T) {
	openCodeTestUnix(t)
	bin := openCodeTestFakeBin(t, "opencode v2.2.5", true)
	argv := filepath.Join(t.TempDir(), "argv")
	t.Setenv("OPENCODE_FAKE_ARGV", argv)
	f := Found{Bin: bin, Version: "2.2.5"}
	w, err := NewCheckWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	tool, err := openCodeCheck(context.Background(), f, openCodeTestEP, w, nil)
	if err != nil || !tool {
		t.Fatalf("check: tool=%v err=%v", tool, err)
	}
	b, _ := os.ReadFile(argv)
	if !strings.HasPrefix(string(b), "run --standalone --model ezlm/qwen3.5-4b --format json Use your file-reading tool") {
		t.Fatalf("argv = %q", b)
	}

	// A failure maps to its sentence.
	rec, _ := filepath.Abs(filepath.Join("testdata", "opencode", "run-2.2.5-401.jsonl"))
	t.Setenv("OPENCODE_FAKE_OUT", rec)
	if _, err := openCodeCheck(context.Background(), f, openCodeTestEP, w, nil); err == nil || !strings.Contains(err.Error(), "old EZLM key") {
		t.Fatalf("401: %v", err)
	}

	// 1.x: no --standalone, its errors come from the log.
	bin1 := openCodeTestFakeBin(t, "1.19.5", false)
	os.Remove(argv)
	t.Setenv("OPENCODE_FAKE_OUT", "")
	if _, err := openCodeCheck(context.Background(), Found{Bin: bin1, Version: "1.19.5"}, openCodeTestEP, w, nil); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(argv)
	if !strings.HasPrefix(string(b), "run --print-logs --log-level ERROR --model ezlm/qwen3.5-4b --format json ") {
		t.Fatalf("1.x argv = %q", b)
	}
}

func TestOpenCodeCheckNeedsToolUse(t *testing.T) {
	res := parseOpenCodeRun(`{"type":"text","part":{"type":"text","text":"I can't read files."}}`)
	if err := openCodeCheckErr("", res, nil); err == nil || !strings.Contains(err.Error(), "without using a tool") {
		t.Fatalf("err = %v", err)
	}
}

func TestOpenCodePreflightWithFake(t *testing.T) {
	openCodeTestUnix(t)
	t.Setenv("OPENCODE_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Cleanup(func() { os.Remove(filepath.Join(Workspace(), "opencode.json")) })
	bin := openCodeTestFakeBin(t, "opencode v2.2.5", true)
	argv := filepath.Join(t.TempDir(), "argv")
	t.Setenv("OPENCODE_FAKE_ARGV", argv)
	f := Found{Bin: bin, Version: "2.2.5"}
	if err := openCodePreflight(context.Background(), f, openCodeTestEP, nil); err == nil || !strings.Contains(err.Error(), "old EZLM key") {
		t.Fatalf("before configure: %v", err)
	}
	if err := (openCodeTool{}).Configure(openCodeTestEP, false); err != nil {
		t.Fatal(err)
	}
	if err := openCodePreflight(context.Background(), f, openCodeTestEP, nil); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(argv); strings.TrimSpace(string(b)) != "models --standalone" {
		t.Fatalf("argv = %q", b)
	}
	ep := openCodeTestEP
	ep.ModelID = "qwen3.8-27b"
	os.WriteFile(filepath.Join(Workspace(), "opencode.json"), []byte(`{"model":"ezlm/qwen3.8-27b"}`), 0o644)
	if err := openCodePreflight(context.Background(), f, ep, nil); err == nil || !strings.Contains(err.Error(), "doesn't list EZLM's model") {
		t.Fatalf("missing model: %v", err)
	}
	// 1.x with history only 2.x can open.
	writeOpenCodeWorkspace(openCodeTestEP)
	openCodeTestDB(t, "db-v2.sqlite.gz", openCodeDataDir(), "opencode.db")
	if err := openCodePreflight(context.Background(), Found{Bin: bin, Version: "1.19.5"}, openCodeTestEP, nil); err == nil || !strings.Contains(err.Error(), "2.2.5") {
		t.Fatalf("1.x on v2: %v", err)
	}
}

func TestOpenCodeLaunchSpec(t *testing.T) {
	t.Cleanup(func() { os.Remove(filepath.Join(Workspace(), "opencode.json")) })
	pinned := Found{Bin: openCodePinnedBin(), Version: "2.2.5", Source: SourceEZLM, Env: []string{"OPENCODE_DISABLE_AUTOUPDATE=1"}}
	s, err := (openCodeTool{}).LaunchSpec(pinned, openCodeTestEP, ModeTUI)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(s.Args, []string{"--standalone"}) || s.Dir != Workspace() || !slices.Contains(s.Ready, "EZLM (local)") {
		t.Fatalf("2.x spec = %+v", s)
	}
	if !slices.Contains(s.Env, "OPENCODE_DISABLE_AUTOUPDATE=1") || !slices.Contains(s.Env, "PWD="+Workspace()) {
		t.Fatalf("2.x env = %v", s.Env)
	}
	for _, e := range s.Env {
		if strings.Contains(e, openCodeTestEP.Key) {
			t.Fatalf("key in the launch env: %v", s.Env)
		}
	}
	if w, _ := readOpenCodeConfig(filepath.Join(Workspace(), "opencode.json")); w.Model != "ezlm/qwen3.5-4b" {
		t.Fatalf("workspace model %q", w.Model)
	}
	s, _ = (openCodeTool{}).LaunchSpec(Found{Bin: "/x/opencode", Version: "1.19.5"}, openCodeTestEP, ModeTUI)
	if !slices.Equal(s.Args, []string{"--model", "ezlm/qwen3.5-4b"}) {
		t.Fatalf("1.x args = %v", s.Args)
	}
	if _, err := (openCodeTool{}).LaunchSpec(pinned, openCodeTestEP, ModeWeb); err == nil {
		t.Fatal("OpenCode has no web mode")
	}
}

// TestOpenCodeCause: the reason OpenCode gives when it exits at once, as
// 1.19.5 prints it and as 2.2.5 logs it (both seen with a history whose
// migration rows were removed).
func TestOpenCodeCause(t *testing.T) {
	out1 := "\x1b[91m\x1b[1mError: \x1b[0mUnexpected error\n\ntable `project` already exists\n"
	if got := openCodeCause(out1, ""); got != "table `project` already exists" {
		t.Errorf("1.x cause = %q", got)
	}
	data := t.TempDir()
	os.MkdirAll(filepath.Join(data, "log"), 0o755)
	logText := `timestamp=2026-11-06T06:10:18.545Z level=ERROR run=11379a29 message="cli process failed" cause="Cause([Die(SQLiteError: table ` + "`project`" + ` already exists)])" args="[\"serve\",\"--stdio\",\"--port\",\"0\"]" role=cli
timestamp=2026-11-06T06:10:18.551Z level=ERROR run=a93d5d52 message="cli process failed" cause="Cause([Fail(Error: Standalone server exited before reporting readiness)])" args="[\"models\",\"--standalone\"]" role=cli
`
	os.WriteFile(filepath.Join(data, "log", "opencode.log"), []byte(logText), 0o644)
	out2 := "Error: Standalone server exited before reporting readiness\n    at cli.models (/$bunfs/root/chunk-3pyaktb2.js:7:21343)\n"
	if got := openCodeCause(out2, data); got != "SQLiteError: table `project` already exists" {
		t.Errorf("2.x cause = %q", got)
	}
	if !openCodeDBError.MatchString(openCodeCause(out2, data)) {
		t.Error("an SQLite error isn't taken for a history problem")
	}
	if got := openCodeCause("Error: something else\n", data); got != "Error: something else" {
		t.Errorf("plain error = %q", got)
	}
}
