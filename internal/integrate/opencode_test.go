package integrate

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tailscale/hujson"
)

var openCodeTestEP = Endpoint{BaseURL: "http://127.0.0.1:8081/v1", Key: "ezlm-test", ModelID: "qwen3.5-4b", ModelName: "Qwen3.5 4B", Ctx: 65536, ToolCalls: true}

// openCodeTestJSON parses a JSONC file into plain values.
func openCodeTestJSON(t *testing.T, b []byte) map[string]any {
	t.Helper()
	std, err := hujson.Standardize(append([]byte{}, b...))
	if err != nil {
		t.Fatalf("not JSONC: %v\n%s", err, b)
	}
	var m map[string]any
	if err := json.Unmarshal(std, &m); err != nil {
		t.Fatalf("not an object: %v\n%s", err, b)
	}
	return m
}

// openCodeTestWithout drops the paths EZLM owns, so the rest can be compared.
func openCodeTestWithout(m map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		out[k] = v
	}
	for _, k := range []string{"model", "small_model"} {
		if s, _ := out[k].(string); strings.HasPrefix(s, "ezlm/") {
			delete(out, k)
		}
	}
	if p, ok := out["provider"].(map[string]any); ok {
		q := map[string]any{}
		for k, v := range p {
			if k != "ezlm" {
				q[k] = v
			}
		}
		out["provider"] = q
	}
	return out
}

func openCodeTestFixture(t *testing.T, name, dir, as string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "opencode", name))
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(dir, 0o755)
	if err := os.WriteFile(filepath.Join(dir, as), b, 0o644); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestOpenCodeWriterKeepsEverythingElse(t *testing.T) {
	for _, tc := range []struct{ fixture, as string }{
		{"mac-v2-user.json", "opencode.json"},
		{"desktop-ezlm-v1.json", "opencode.json"},
		{"commented.jsonc", "opencode.jsonc"},
	} {
		t.Run(tc.fixture, func(t *testing.T) {
			dir := t.TempDir()
			orig := openCodeTestFixture(t, tc.fixture, dir, tc.as)
			path := filepath.Join(dir, tc.as)
			if err := WriteOpenCodeConfig(dir, openCodeTestEP, false); err != nil {
				t.Fatal(err)
			}
			first, _ := os.ReadFile(path)
			before, after := openCodeTestJSON(t, orig), openCodeTestJSON(t, first)
			if !reflect.DeepEqual(openCodeTestWithout(before), openCodeTestWithout(after)) {
				t.Fatalf("settings outside EZLM's entry changed:\n%s", first)
			}
			if _, ok := after["provider"].(map[string]any)["ezlm"]; !ok {
				t.Fatalf("no ezlm provider:\n%s", first)
			}
			// Idempotent: a second write changes nothing, and makes no backup.
			backups, _ := filepath.Glob(path + ".ezlm-backup-*")
			if err := WriteOpenCodeConfig(dir, openCodeTestEP, false); err != nil {
				t.Fatal(err)
			}
			second, _ := os.ReadFile(path)
			if !bytes.Equal(first, second) {
				t.Fatalf("second write differs:\n%s\n---\n%s", first, second)
			}
			if again, _ := filepath.Glob(path + ".ezlm-backup-*"); len(again) != len(backups) || len(backups) != 1 {
				t.Fatalf("backups = %v then %v", backups, again)
			}
			if tc.as == "opencode.jsonc" {
				for _, c := range []string{"// OpenCode settings, hand-written.", "// Cloud model by default; EZLM must not change this.", "// the default", "/* the Spark ring */", "// big", "// trailing comma follows"} {
					if !strings.Contains(string(first), c) {
						t.Errorf("comment %q lost:\n%s", c, first)
					}
				}
				if _, err := os.Stat(filepath.Join(dir, "opencode.json")); err == nil {
					t.Error("wrote opencode.json next to opencode.jsonc")
				}
			}
		})
	}
}

func TestOpenCodeWriterLayout(t *testing.T) {
	dir := t.TempDir()
	openCodeTestFixture(t, "mac-v2-user.json", dir, "opencode.json")
	if err := WriteOpenCodeConfig(dir, openCodeTestEP, false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "opencode.json"))
	// The user's part is byte-for-byte what it was, and EZLM's entry is
	// indented like the file.
	want := `    "spark": {
      "npm": "@ai-sdk/openai-compatible",
      "name": "DeepSeek (Sparks)",`
	if !strings.Contains(string(b), want) || !strings.Contains(string(b), `"limit": { "context": 1000000, "output": 32768 }`) {
		t.Fatalf("user's provider reformatted:\n%s", b)
	}
	if !strings.Contains(string(b), "\n    \"ezlm\": {\n      \"npm\": \"@ai-sdk/openai-compatible\",\n      \"name\": \"EZLM (local)\",") {
		t.Fatalf("EZLM's entry isn't indented like the file:\n%s", b)
	}
	if !strings.HasSuffix(string(b), "}\n") || strings.Count(string(b), "\n}\n") != 1 {
		t.Fatalf("file end:\n%s", b)
	}
}

func TestOpenCodeEntry(t *testing.T) {
	dir := t.TempDir()
	if err := WriteOpenCodeConfig(dir, openCodeTestEP, false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "opencode.json"))
	cfg := openCodeTestJSON(t, b)
	if cfg["$schema"] != "https://opencode.ai/config.json" || cfg["model"] != "ezlm/qwen3.5-4b" {
		t.Fatalf("new file: %s", b)
	}
	e := cfg["provider"].(map[string]any)["ezlm"].(map[string]any)
	opts := e["options"].(map[string]any)
	key := filepath.ToSlash(filepath.Join(dir, "ezlm.key"))
	if opts["baseURL"] != openCodeTestEP.BaseURL || opts["apiKey"] != "{file:"+key+"}" {
		t.Fatalf("options = %v", opts)
	}
	for _, k := range []string{"timeout", "headerTimeout", "chunkTimeout"} {
		if opts[k] != float64(1800000) {
			t.Errorf("%s = %v", k, opts[k])
		}
	}
	m := e["models"].(map[string]any)["qwen3.5-4b"].(map[string]any)
	if m["tool_call"] != true || m["name"] != "Qwen3.5 4B (EZLM)" {
		t.Fatalf("model = %v", m)
	}
	if got, _ := json.Marshal(m["modalities"]); string(got) != `{"input":["text"],"output":["text"]}` {
		t.Errorf("modalities = %s", got)
	}
	if got, _ := json.Marshal(m["limit"]); string(got) != `{"context":65536,"output":16384}` {
		t.Errorf("limit = %s", got)
	}
	for _, k := range []string{"reasoning", "release_date", "attachment", "variants"} {
		if _, ok := m[k]; ok {
			t.Errorf("wrote %q", k)
		}
	}
	if k, _ := os.ReadFile(filepath.Join(dir, "ezlm.key")); string(k) != "ezlm-test" {
		t.Fatalf("key file = %q", k)
	}
	if st, _ := os.Stat(filepath.Join(dir, "ezlm.key")); st.Mode().Perm() != 0o600 && os.PathSeparator == '/' {
		t.Errorf("key file mode %v", st.Mode())
	}
}

func TestOpenCodeOutputClamp(t *testing.T) {
	for ctx, want := range map[int]int{4096: 2048, 8192: 4096, 16384: 4096, 65536: 16384, 131072: 32768, 1048576: 32768} {
		if got := openCodeOutput(ctx); got != want {
			t.Errorf("ctx %d: output %d, want %d", ctx, got, want)
		}
	}
}

func TestOpenCodeVariantsOnlyAcceptedLevels(t *testing.T) {
	dir := t.TempDir()
	ep := openCodeTestEP
	ep.Reasoning = Reasoning{Levels: []string{"low", "medium"}, Thinking: true}
	if err := WriteOpenCodeConfig(dir, ep, false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "opencode.json"))
	m := openCodeTestJSON(t, b)["provider"].(map[string]any)["ezlm"].(map[string]any)["models"].(map[string]any)["qwen3.5-4b"].(map[string]any)
	got, _ := json.Marshal(m["variants"])
	if string(got) != `{"low":{"reasoningEffort":"low"},"medium":{"reasoningEffort":"medium"}}` {
		t.Fatalf("variants = %s", got)
	}
	if strings.Index(string(b), `"low"`) > strings.Index(string(b), `"medium"`) {
		t.Errorf("levels out of order:\n%s", b)
	}
	// Levels gone: no variants, so no request can carry a level.
	if err := WriteOpenCodeConfig(dir, openCodeTestEP, false); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(filepath.Join(dir, "opencode.json"))
	if strings.Contains(string(b), "variants") || strings.Contains(string(b), "reasoning") {
		t.Fatalf("reasoning settings left:\n%s", b)
	}
}

func TestOpenCodeDefaultModelRules(t *testing.T) {
	write := func(t *testing.T, cfg string, makeDefault bool) map[string]any {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "opencode.json"), []byte(cfg), 0o644)
		if err := WriteOpenCodeConfig(dir, openCodeTestEP, makeDefault); err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(filepath.Join(dir, "opencode.json"))
		return openCodeTestJSON(t, b)
	}
	for _, tc := range []struct {
		name, cfg   string
		makeDefault bool
		model       any
		small       any
	}{
		{"someone else's default stays", `{"model":"spark/ds","small_model":"spark/mini"}`, false, "spark/ds", "spark/mini"},
		{"makeDefault takes model, not small_model", `{"model":"spark/ds","small_model":"spark/mini"}`, true, "ezlm/qwen3.5-4b", "spark/mini"},
		{"empty default becomes EZLM's", `{"theme":"system"}`, false, "ezlm/qwen3.5-4b", nil},
		{"EZLM's default follows a model switch", `{"model":"ezlm/old","small_model":"ezlm/old"}`, false, "ezlm/qwen3.5-4b", "ezlm/qwen3.5-4b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := write(t, tc.cfg, tc.makeDefault)
			if cfg["model"] != tc.model || cfg["small_model"] != tc.small {
				t.Fatalf("model=%v small_model=%v", cfg["model"], cfg["small_model"])
			}
		})
	}
}

func TestOpenCodeKeyWrittenWhenConfigIsBroken(t *testing.T) {
	dir := t.TempDir()
	bad := []byte("{ \"model\": ")
	os.WriteFile(filepath.Join(dir, "opencode.json"), bad, 0o644)
	ep := openCodeTestEP
	ep.Key = "ezlm-rotated"
	if err := WriteOpenCodeConfig(dir, ep, false); err == nil {
		t.Fatal("expected an error for a broken file")
	}
	if k, _ := os.ReadFile(filepath.Join(dir, "ezlm.key")); string(k) != "ezlm-rotated" {
		t.Fatalf("key file = %q", k)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "opencode.json")); !bytes.Equal(b, bad) {
		t.Fatalf("broken file was changed: %q", b)
	}
}

func TestOpenCodeConfigDir(t *testing.T) {
	home, _ := os.UserHomeDir()
	t.Setenv("OPENCODE_CONFIG_DIR", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	if got := openCodeConfigDir(); got != filepath.Join(home, ".config", "opencode") {
		t.Errorf("default: %s", got)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
	if got := openCodeConfigDir(); got != filepath.Join(home, "xdg", "opencode") {
		t.Errorf("XDG_CONFIG_HOME: %s", got)
	}
	t.Setenv("OPENCODE_CONFIG_DIR", filepath.Join(home, "oc"))
	if got := openCodeConfigDir(); got != filepath.Join(home, "oc") {
		t.Errorf("OPENCODE_CONFIG_DIR: %s", got)
	}
	dir := t.TempDir()
	if got := openCodeConfigFile(dir); got != filepath.Join(dir, "opencode.json") {
		t.Errorf("no file: %s", got)
	}
	os.WriteFile(filepath.Join(dir, "opencode.json"), []byte("{}"), 0o644)
	os.WriteFile(filepath.Join(dir, "opencode.jsonc"), []byte("{}"), 0o644)
	if got := openCodeConfigFile(dir); got != filepath.Join(dir, "opencode.jsonc") {
		t.Errorf("jsonc should win: %s", got)
	}
}

func TestOpenCodeWorkspaceFile(t *testing.T) {
	t.Setenv("OPENCODE_CONFIG_DIR", t.TempDir())
	path := filepath.Join(Workspace(), "opencode.json")
	os.MkdirAll(Workspace(), 0o755)
	os.WriteFile(path, []byte("{\n  \"model\": \"ezlm/old\",\n  \"theme\": \"tokyonight\"\n}\n"), 0o644)
	t.Cleanup(func() { os.Remove(path) })
	if err := (openCodeTool{}).Configure(openCodeTestEP, false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	cfg := openCodeTestJSON(t, b)
	if cfg["model"] != "ezlm/qwen3.5-4b" || cfg["theme"] != "tokyonight" || cfg["$schema"] == nil {
		t.Fatalf("workspace file: %s", b)
	}
}

func TestOpenCodeConfigureNeedsTools(t *testing.T) {
	ep := openCodeTestEP
	ep.ToolCalls = false
	err := (openCodeTool{}).Configure(ep, false)
	if err == nil || !strings.Contains(err.Error(), "can't use tools") {
		t.Fatalf("err = %v", err)
	}
}

func TestOpenCodeDetectState(t *testing.T) {
	bin := openCodeTestFakeBin(t, "opencode v2.2.5", true)
	t.Setenv("EZLM_TOOL_PATH", filepath.Dir(bin))
	SaveProbe(bin, Probe{Version: "2.2.5", Arch: MachineArch(), Facts: map[string]string{"standalone": "1"}})
	t.Setenv("OPENCODE_CONFIG_DIR", t.TempDir())
	t.Cleanup(func() { os.Remove(filepath.Join(Workspace(), "opencode.json")) })

	ep := openCodeTestEP
	d := (openCodeTool{}).Detect(&ep)
	if d.Bin != bin || d.Version != "2.2.5" || d.Linked || d.State != "" {
		t.Fatalf("before connect: %+v", d)
	}
	if err := (openCodeTool{}).Configure(ep, false); err != nil {
		t.Fatal(err)
	}
	if d := (openCodeTool{}).Detect(&ep); !d.Linked || d.State != "" {
		t.Fatalf("connected: %+v", d)
	}
	for name, change := range map[string]func(*Endpoint){
		"port":   func(e *Endpoint) { e.BaseURL = "http://127.0.0.1:8090/v1" },
		"key":    func(e *Endpoint) { e.Key = "ezlm-new" },
		"model":  func(e *Endpoint) { e.ModelID = "qwen3.8-27b" },
		"ctx":    func(e *Endpoint) { e.Ctx = 32768 },
		"levels": func(e *Endpoint) { e.Reasoning.Levels = []string{"low"} },
	} {
		e := ep
		change(&e)
		if d := (openCodeTool{}).Detect(&e); d.State != StateNeedsUpdate || !d.Linked {
			t.Errorf("%s change: %+v", name, d)
		}
	}
	e := ep
	e.ToolCalls = false
	if d := (openCodeTool{}).Detect(&e); d.State != StateNeedsTools || !strings.Contains(d.Detail, "Pick a model marked Tools") {
		t.Errorf("no tools: %+v", d)
	}
	// Someone else's workspace edit to another EZLM model is stale too.
	os.WriteFile(filepath.Join(Workspace(), "opencode.json"), []byte(`{"model":"ezlm/old"}`), 0o644)
	if d := (openCodeTool{}).Detect(&ep); d.State != StateNeedsUpdate {
		t.Errorf("stale workspace: %+v", d)
	}
}

// openCodeTestFakeBin writes a fake opencode into a fresh folder. On Unix it
// is a shell script that answers --version with version, --help and
// run --help like 2.x (v2) or 1.x, `models` with EZLM's model, and `run`
// with a passing check, or with the file $OPENCODE_FAKE_OUT; it appends its
// arguments to $OPENCODE_FAKE_ARGV. On Windows it is a placeholder that is
// found but never run.
func openCodeTestFakeBin(t *testing.T, version string, v2 bool) string {
	t.Helper()
	dir := t.TempDir()
	if os.PathSeparator == '\\' {
		p := filepath.Join(dir, "opencode.exe")
		os.WriteFile(p, []byte("MZ"), 0o755)
		return p
	}
	help, runHelp := "  -m, --model   model to use", "  --variant   model variant"
	if v2 {
		help, runHelp = "  --standalone   Run with a private server", "  --model, -m string\n  --standalone"
	}
	script := `#!/bin/sh
[ -n "$OPENCODE_FAKE_ARGV" ] && echo "$@" >> "$OPENCODE_FAKE_ARGV"
case "$1" in
--version) echo "` + version + `"; exit 0;;
--help) echo "` + help + `"; exit 0;;
models) echo "ezlm/qwen3.5-4b"; exit 0;;
run)
  if [ "$2" = "--help" ]; then echo "` + runHelp + `"; exit 0; fi
  if [ -n "$OPENCODE_FAKE_OUT" ]; then cat "$OPENCODE_FAKE_OUT"; exit 1; fi
  n=$(cat ezlm-check.txt)
  echo '{"type":"step_start","part":{"type":"step-start"}}'
  echo '{"type":"tool_use","part":{"type":"tool","tool":"read"}}'
  echo "{\"type\":\"text\",\"part\":{\"type\":\"text\",\"text\":\"$n\"}}"
  exit 0;;
esac
exit 0
`
	p := filepath.Join(dir, "opencode")
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}
