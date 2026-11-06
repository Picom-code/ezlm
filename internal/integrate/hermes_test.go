package integrate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

var hermesTestEP = Endpoint{BaseURL: "http://127.0.0.1:8081/v1", Key: "ezlm-testkey123", ModelID: "qwen3.5-4b", ModelName: "Qwen3.5 4B", Ctx: 65536, ToolCalls: true}

// hermesTestCopy copies a fixture into a temp file.
func hermesTestCopy(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "hermes", name))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// hermesTestDecode parses config.yaml into plain maps.
func hermesTestDecode(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := yaml.Unmarshal(b, &m); err != nil {
		t.Fatalf("%v\n%s", err, b)
	}
	return m
}

// hermesTestStrip removes everything EZLM owns, so the rest can be compared.
func hermesTestStrip(m map[string]any, ep Endpoint) map[string]any {
	isEZLM := func(v any) bool {
		e, _ := v.(map[string]any)
		n, _ := e["name"].(string)
		return hermesIsEZLM(n)
	}
	if p, ok := m["providers"].(map[string]any); ok {
		for k, v := range p {
			if hermesIsEZLM(k) || isEZLM(v) {
				delete(p, k)
			}
		}
		if len(p) == 0 {
			delete(m, "providers")
		}
	}
	if l, ok := m["custom_providers"].([]any); ok {
		var keep []any
		for _, v := range l {
			if !isEZLM(v) {
				keep = append(keep, v)
			}
		}
		if len(keep) == 0 {
			delete(m, "custom_providers")
		} else {
			m["custom_providers"] = keep
		}
	}
	if md, ok := m["model"].(map[string]any); ok {
		p, _ := md["provider"].(string)
		if hermesIsEZLM(p) {
			delete(m, "model")
		}
	}
	if a, ok := m["agent"].(map[string]any); ok {
		if ov, ok := a["reasoning_overrides"].(map[string]any); ok {
			delete(ov, ep.ModelID)
			if len(ov) == 0 {
				delete(a, "reasoning_overrides")
			}
		}
		if len(a) == 0 {
			delete(m, "agent")
		}
	}
	return m
}

func hermesTestBackups(t *testing.T, path string) int {
	m, _ := filepath.Glob(path + ".ezlm-backup-*")
	return len(m)
}

func TestHermesWriterFixtures(t *testing.T) {
	for _, tc := range []struct {
		file    string
		modern  bool
		comment string
	}{
		{"mac-v52.yaml", true, "# For custom OpenAI-compatible endpoints, add base_url and key_env."},
		{"desktop-v46-ezlm.yaml", true, ""},
		{"v11-legacy.yaml", false, "# the user's own box"},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := hermesTestCopy(t, tc.file)
			orig, _ := os.ReadFile(path)
			if err := WriteHermesConfig(path, hermesTestEP, false); err != nil {
				t.Fatal(err)
			}
			b, _ := os.ReadFile(path)
			if tc.comment != "" && !strings.Contains(string(b), tc.comment) {
				t.Errorf("comment lost:\n%s", b)
			}
			// Everything that isn't EZLM's is exactly as it was.
			before := hermesTestStrip(hermesTestDecode(t, orig), hermesTestEP)
			after := hermesTestStrip(hermesTestDecode(t, b), hermesTestEP)
			if !reflect.DeepEqual(before, after) {
				t.Errorf("other settings changed:\nbefore %v\nafter  %v", before, after)
			}
			m := hermesTestDecode(t, b)
			if tc.modern {
				e, _ := m["providers"].(map[string]any)["ezlm"].(map[string]any)
				want := map[string]any{"name": "ezlm", "api": hermesTestEP.BaseURL, "api_key": hermesTestEP.Key, "transport": "chat_completions",
					"default_model": "qwen3.5-4b", "context_length": 65536,
					"models": map[string]any{"qwen3.5-4b": map[string]any{"context_length": 65536, "name": "Qwen3.5 4B (EZLM)"}}}
				if !reflect.DeepEqual(e, want) {
					t.Errorf("providers.ezlm = %v", e)
				}
				for _, it := range asList(m["custom_providers"]) {
					if n, _ := it.(map[string]any)["name"].(string); n == "ezlm" {
						t.Errorf("EZLM's legacy entry is still there")
					}
				}
			} else {
				if _, ok := m["providers"]; ok {
					t.Errorf("a pre-v12 file got a providers map")
				}
				found := false
				for _, it := range asList(m["custom_providers"]) {
					e := it.(map[string]any)
					if e["name"] == "ezlm" {
						found = e["base_url"] == hermesTestEP.BaseURL && e["api_key"] == hermesTestEP.Key && e["default_model"] == "qwen3.5-4b"
					}
				}
				if !found {
					t.Errorf("legacy entry missing:\n%s", b)
				}
			}
			// A second write changes nothing and makes no backup.
			n := hermesTestBackups(t, path)
			if err := WriteHermesConfig(path, hermesTestEP, false); err != nil {
				t.Fatal(err)
			}
			b2, _ := os.ReadFile(path)
			if string(b2) != string(b) || hermesTestBackups(t, path) != n {
				t.Errorf("second write changed the file")
			}
			if st, _ := os.Stat(path); runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 {
				t.Errorf("mode %v", st.Mode())
			}
			st := hermesReadState(path)
			if !st.linked || st.stale(hermesTestEP) != "" {
				t.Errorf("state after write: linked=%v stale=%q", st.linked, st.stale(hermesTestEP))
			}
		})
	}
}

func asList(v any) []any { l, _ := v.([]any); return l }

func TestHermesMacKeepsSparkAndModel(t *testing.T) {
	path := hermesTestCopy(t, "mac-v52.yaml")
	if err := WriteHermesConfig(path, hermesTestEP, false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	m := hermesTestDecode(t, b)
	if p := m["model"].(map[string]any)["provider"]; p != "custom:spark" {
		t.Fatalf("model moved without makeDefault: %v", p)
	}
	if _, ok := m["providers"].(map[string]any)["custom:spark"]; !ok {
		t.Fatal("custom:spark lost")
	}
	if !strings.Contains(string(b), "custom:spark:\n    stale_timeout_seconds: 2400") {
		t.Fatalf("custom:spark changed:\n%s", b)
	}
	if err := WriteHermesConfig(path, hermesTestEP, true); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(path)
	md := hermesTestDecode(t, b)["model"].(map[string]any)
	if md["provider"] != "custom:ezlm" || md["default"] != "qwen3.5-4b" || md["base_url"] != hermesTestEP.BaseURL || md["api_mode"] != "chat_completions" {
		t.Fatalf("makeDefault: %v", md)
	}
}

func TestHermesNeeds64KLeavesFile(t *testing.T) {
	path := hermesTestCopy(t, "desktop-v46-ezlm.yaml")
	orig, _ := os.ReadFile(path)
	ep := hermesTestEP
	ep.Ctx = 32768
	if err := WriteHermesConfig(path, ep, true); !errors.Is(err, ErrNeeds64K) {
		t.Fatalf("err = %v", err)
	}
	if b, _ := os.ReadFile(path); string(b) != string(orig) || hermesTestBackups(t, path) != 0 {
		t.Fatal("file touched")
	}
	missing := filepath.Join(t.TempDir(), "none", "config.yaml")
	if err := WriteHermesConfig(missing, ep, false); !errors.Is(err, ErrNeeds64K) {
		t.Fatal(err)
	}
	if _, err := os.Stat(missing); err == nil {
		t.Fatal("created a file")
	}
	if _, err := (hermesTool{}).LaunchSpec(Found{Bin: "/x/hermes"}, ep, ModeTUI); err == nil || err.Error() != hermes64KText {
		t.Fatalf("launch: %v", err)
	}
}

func TestHermesModelOwnership(t *testing.T) {
	t.Setenv("EZLM_HOME", t.TempDir()) // its own port list
	NotePort(18080)                    // a port EZLM served on before
	for _, tc := range []struct {
		name, model string
		follow      bool
	}{
		{"custom:ezlm", "model:\n  default: old\n  provider: custom:ezlm\n  base_url: http://127.0.0.1:18080/v1\n", true},
		{"bare ezlm", "model:\n  default: old\n  provider: EZLM\n", true},
		{"picked inside Hermes", "model:\n  default: old\n  provider: custom\n  base_url: http://127.0.0.1:18080/v1\n  api_key: ezlm-oldkey999\n", true},
		{"custom on another port", "model:\n  default: x\n  provider: custom\n  base_url: http://127.0.0.1:9999/v1\n", false},
		{"custom on another host", "model:\n  default: x\n  provider: custom\n  base_url: http://10.0.0.2:18080/v1\n", false},
		{"someone else's", "model:\n  default: sonnet\n  provider: anthropic\n", false},
		{"empty", "model: {}\n", true},
		{"missing", "agent:\n  max_turns: 5\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			os.WriteFile(path, []byte(tc.model), 0o600)
			if err := WriteHermesConfig(path, hermesTestEP, false); err != nil {
				t.Fatal(err)
			}
			b, _ := os.ReadFile(path)
			md, _ := hermesTestDecode(t, b)["model"].(map[string]any)
			followed := md["provider"] == "custom:ezlm" && md["default"] == "qwen3.5-4b" && md["base_url"] == hermesTestEP.BaseURL
			if followed != tc.follow {
				t.Fatalf("followed=%v:\n%s", followed, b)
			}
			if k, ok := md["api_key"]; ok && k != hermesTestEP.Key {
				t.Fatalf("stale key kept in model block: %v", k)
			}
		})
	}
	// makeDefault over another provider drops that provider's credentials.
	path := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(path, []byte("model:\n  default: gpt\n  provider: custom\n  base_url: https://api.example.com/v1\n  api_key: sk-notezlmkey12345\n"), 0o600)
	WriteHermesConfig(path, hermesTestEP, true)
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), "sk-notezlmkey12345") || !strings.Contains(string(b), "provider: custom:ezlm") {
		t.Fatalf("takeover:\n%s", b)
	}
}

func TestHermesOnlyEZLMEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(path, []byte(`_config_version: 52
providers:
  ezlm-old:
    name: ezlm-old
    api: http://127.0.0.1:9000/v1
  custom:ezlm:
    name: ezlm
    api: http://127.0.0.1:8080/v1
    stale_timeout_seconds: 900
  ezlm:
    api: http://127.0.0.1:7000/v1
custom_providers:
  - name: ezlm-old
    base_url: http://127.0.0.1:9000/v1
  - name: EZLM
    base_url: http://127.0.0.1:8080/v1
`), 0o600)
	if err := WriteHermesConfig(path, hermesTestEP, false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	m := hermesTestDecode(t, b)
	p := m["providers"].(map[string]any)
	if len(p) != 2 || p["ezlm-old"] == nil || p["custom:ezlm"] == nil {
		t.Fatalf("providers = %v", p)
	}
	e := p["custom:ezlm"].(map[string]any)
	if e["api"] != hermesTestEP.BaseURL || e["stale_timeout_seconds"] != 900 {
		t.Fatalf("EZLM's entry wasn't updated in place: %v", e)
	}
	if l := asList(m["custom_providers"]); len(l) != 1 || l[0].(map[string]any)["name"] != "ezlm-old" {
		t.Fatalf("custom_providers = %v", l)
	}
}

func TestHermesReasoning(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(path, []byte("agent:\n  max_turns: 90\n  reasoning_overrides:\n    other-model: high\n"), 0o600)
	override := func() (string, string) {
		b, _ := os.ReadFile(path)
		a, _ := hermesTestDecode(t, b)["agent"].(map[string]any)
		ov, _ := a["reasoning_overrides"].(map[string]any)
		v, _ := ov[hermesTestEP.ModelID].(string)
		return v, string(b)
	}
	ep := hermesTestEP
	for _, tc := range []struct {
		levels []string
		global string
		want   string
	}{
		{nil, "", ""}, // no levels: write nothing
		{[]string{"low", "medium", "xhigh"}, "", ""}, // Hermes's default medium is taken
		{[]string{"low", "xhigh"}, "", "low"},        // medium isn't: the nearest weaker
		{[]string{"high", "max"}, "", "high"},        // nothing weaker: the weakest
		{[]string{"low", "medium", "xhigh"}, "high", "medium"},
		{[]string{"low", "medium", "xhigh"}, "xhigh", ""},
		{[]string{"low", "xhigh"}, "false", ""}, // thinking off is always taken
		{nil, "high", ""},                       // back to no levels: EZLM's override goes
	} {
		ep.Reasoning = Reasoning{Levels: tc.levels}
		// Set the user's global level by node edit, so EZLM's marks survive.
		doc, _ := yamlLoad(path)
		a := yamlChild(doc.Content[0], "agent")
		switch tc.global {
		case "":
			yamlDelete(a, "reasoning_effort")
		case "false":
			yamlSet(a, "reasoning_effort", yamlBool(false))
		default:
			yamlSet(a, "reasoning_effort", yamlStr(tc.global))
		}
		nb, _ := yamlEncode(doc)
		os.WriteFile(path, nb, 0o600)
		if err := WriteHermesConfig(path, ep, false); err != nil {
			t.Fatal(err)
		}
		got, body := override()
		if got != tc.want {
			t.Fatalf("levels %v global %q: override %q, want %q\n%s", tc.levels, tc.global, got, tc.want, body)
		}
		if tc.want != "" && !strings.Contains(body, hermesEZLMMark) {
			t.Fatalf("override not marked:\n%s", body)
		}
		if !strings.Contains(body, "other-model: high") {
			t.Fatalf("the user's override was touched:\n%s", body)
		}
		if st := hermesReadState(path); st.stale(ep) != "" {
			t.Fatalf("stale after write: %s", st.stale(ep))
		}
	}
	// An override EZLM wrote for a model it no longer serves goes away.
	ep.Reasoning = Reasoning{Levels: []string{"low", "xhigh"}}
	WriteHermesConfig(path, ep, false)
	next := ep
	next.ModelID = "qwen3.8-27b"
	WriteHermesConfig(path, next, false)
	if _, body := override(); strings.Contains(body, "qwen3.5-4b: low") || !strings.Contains(body, "qwen3.8-27b: low") {
		t.Fatalf("old override kept:\n%s", body)
	}
}

func TestHermesConfigPath(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HERMES_HOME", root)
	if got := hermesDefaultConfig(); got != filepath.Join(root, "config.yaml") {
		t.Fatalf("HERMES_HOME: %s", got)
	}
	os.MkdirAll(filepath.Join(root, "profiles", "work"), 0o755)
	os.WriteFile(filepath.Join(root, "active_profile"), []byte("work\n"), 0o644)
	if got := hermesDefaultConfig(); got != filepath.Join(root, "profiles", "work", "config.yaml") {
		t.Fatalf("active_profile: %s", got)
	}
	t.Setenv("HERMES_HOME", filepath.Join(root, "profiles", "work"))
	if got := hermesDefaultConfig(); got != filepath.Join(root, "profiles", "work", "config.yaml") {
		t.Fatalf("profile home: %s", got)
	}
	os.WriteFile(filepath.Join(root, "active_profile"), []byte("gone"), 0o644)
	t.Setenv("HERMES_HOME", root)
	if got := hermesDefaultConfig(); got != filepath.Join(root, "config.yaml") {
		t.Fatalf("missing profile: %s", got)
	}
	t.Setenv("HERMES_HOME", "")
	t.Setenv("HERMES_DATA_DIR_SUFFIX", "-dev")
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".hermes-dev", "config.yaml")
	if runtime.GOOS == "windows" {
		want = filepath.Join(os.Getenv("LOCALAPPDATA"), "hermes-dev", "config.yaml")
	}
	if got := hermesDefaultConfig(); got != want {
		t.Fatalf("default: %s, want %s", got, want)
	}
}

// hermesTestFake writes a fake hermes program that answers like 0.22.5: the
// version, `config path`, `chat --help` from the fixture, and a check run
// that reads ezlm-check.txt in its folder and prints stream-json events.
// mode "z" makes it a build without `chat -q`; "64k" makes the run fail
// like a model below Hermes's floor.
func hermesTestFake(t *testing.T, cfg, mode string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake hermes is a shell script")
	}
	dir := t.TempDir()
	help, _ := filepath.Abs(filepath.Join("testdata", "hermes", "chat-help-0.22.5.txt"))
	if mode == "z" {
		os.WriteFile(filepath.Join(dir, "help.txt"), []byte("usage: hermes [-h] [--version] [-z PROMPT] [-m MODEL] [--provider PROVIDER] [-t TOOLSETS] [--ignore-rules]\n"), 0o644)
		help = filepath.Join(dir, "help.txt")
	}
	script := `#!/bin/sh
echo "$@" >> "` + dir + `/calls"
case "$1" in
--version) echo "Hermes Agent v0.22.5+503.gb222bce (2026.9.24) · stable · local b222bced94bc"; exit 0 ;;
config) echo "preparing…"; echo "` + cfg + `"; exit 0 ;;
--help) cat "` + help + `"; exit 0 ;;
-z) [ "` + mode + `" = z ] || exit 2; cat ezlm-check.txt; exit 0 ;;
chat)
  if [ "$2" = "--help" ]; then [ "` + mode + `" = z ] && exit 2; cat "` + help + `"; exit 0; fi
  if [ "` + mode + `" = 64k ]; then echo "Model qwen has a context window of 32768 tokens, which is below the minimum 64,000 required by Hermes Agent" >&2; exit 2; fi
  n=$(cat ezlm-check.txt)
  printf '{"type":"system","subtype":"init","model":"m"}\n{"type":"tool_use","name":"read_file","input":{"path":"ezlm-check.txt"}}\n{"type":"tool_result","name":"read_file","output":"%s"}\n{"type":"text","text":"%s"}\n{"type":"result","exit_code":0,"text":"%s"}\n' "$n" "$n" "$n"
  echo "session_id: 1" >&2 ;;
esac
`
	bin := filepath.Join(dir, "hermes")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EZLM_TOOL_PATH", dir)
	return bin
}

func TestHermesResolveAndCheck(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "hermes-home", "config.yaml")
	bin := hermesTestFake(t, cfg, "")
	ctx := context.Background()
	f, err := hermesTool{}.Resolve(ctx, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if f.Bin != bin || f.Version != "0.22.5" || f.Source != SourcePath {
		t.Fatalf("found %+v", f)
	}
	if got := hermesConfigPath(bin); got != cfg {
		t.Fatalf("config from the probe: %s", got)
	}
	if err := (hermesTool{}).Configure(hermesTestEP, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cfg); err != nil {
		t.Fatalf("Configure didn't write where hermes reads: %v", err)
	}
	w, _ := NewCheckWorkspace()
	defer w.Close()
	toolCall, err := hermesTool{}.Check(ctx, f, hermesTestEP, w, nil)
	if err != nil || !toolCall {
		t.Fatalf("check: toolCall=%v err=%v", toolCall, err)
	}
	calls, _ := os.ReadFile(filepath.Join(filepath.Dir(bin), "calls"))
	want := "chat -q " + CheckPrompt + " -Q --provider custom:ezlm -m qwen3.5-4b -t file --ignore-rules --source tool --max-turns 6 --format stream-json"
	if !strings.Contains(string(calls), want) {
		t.Fatalf("check command:\n%s\nwant %s", calls, want)
	}
	if err := (hermesTool{}).Preflight(ctx, f, hermesTestEP, nil); err != nil {
		t.Fatalf("preflight: %v", err)
	}
	moved := hermesTestEP
	moved.BaseURL = "http://127.0.0.1:9191/v1"
	if err := (hermesTool{}).Preflight(ctx, f, moved, nil); err == nil || !strings.Contains(err.Error(), "old EZLM address") {
		t.Fatalf("stale preflight: %v", err)
	}

	d := hermesTool{}.Detect(&hermesTestEP)
	if d.Bin != bin || !d.Linked || d.State != "" || d.Version != "0.22.5" || d.Config != cfg {
		t.Fatalf("detect: %+v", d)
	}
	if d := (hermesTool{}).Detect(&moved); d.State != StateNeedsUpdate {
		t.Fatalf("moved port: %+v", d)
	}
	rotated := hermesTestEP
	rotated.Key = "ezlm-rotated999"
	if d := (hermesTool{}).Detect(&rotated); d.State != StateNeedsUpdate {
		t.Fatalf("rotated key: %+v", d)
	}
	noTools := hermesTestEP
	noTools.ToolCalls = false
	if d := (hermesTool{}).Detect(&noTools); d.State != StateNeedsTools {
		t.Fatalf("no tools: %+v", d)
	}
	for _, tg := range Targets(&hermesTestEP) {
		if tg.ID == "hermes" && (tg.State != StateConnected || !tg.Connected || tg.Source != SourcePath) {
			t.Fatalf("target: %+v", tg)
		}
	}
	small := hermesTestEP
	small.Ctx = 32768
	for _, tg := range Targets(&small) {
		if tg.ID == "hermes" && tg.State != StateNeeds64K {
			t.Fatalf("small context target: %+v", tg)
		}
	}
}

func TestHermesPreflightOtherConfig(t *testing.T) {
	reads := filepath.Join(t.TempDir(), "profiles", "work", "config.yaml")
	hermesTestFake(t, reads, "")
	ctx := context.Background()
	f, err := hermesTool{}.Resolve(ctx, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	// The env changed since the probe (a new active profile): Configure
	// writes by Hermes's rules, and the preflight notices the mismatch.
	p, _ := CachedProbe(f.Bin)
	p.Facts["env"] = "stale"
	SaveProbe(f.Bin, p)
	if err := (hermesTool{}).Configure(hermesTestEP, false); err != nil {
		t.Fatal(err)
	}
	err = hermesTool{}.Preflight(ctx, f, hermesTestEP, nil)
	if err == nil || !strings.Contains(err.Error(), "Hermes reads "+reads) {
		t.Fatalf("preflight: %v", err)
	}
	if got := hermesConfigPath(f.Bin); got != reads {
		t.Fatalf("the answer wasn't cached for the next Configure: %s", got)
	}
}

func TestHermesCheckFallbacks(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	ctx := context.Background()

	bin := hermesTestFake(t, cfg, "z")
	f, err := hermesTool{}.Resolve(ctx, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	w, _ := NewCheckWorkspace()
	defer w.Close()
	if _, err := (hermesTool{}).Check(ctx, f, hermesTestEP, w, nil); err != nil {
		t.Fatalf("-z check: %v", err)
	}
	calls, _ := os.ReadFile(filepath.Join(filepath.Dir(bin), "calls"))
	if !strings.Contains(string(calls), "-z "+CheckPrompt+" --provider custom:ezlm -m qwen3.5-4b -t file --ignore-rules") {
		t.Fatalf("-z command:\n%s", calls)
	}

	hermesTestFake(t, cfg, "64k")
	f, _ = hermesTool{}.Resolve(ctx, nil, false)
	if _, err := (hermesTool{}).Check(ctx, f, hermesTestEP, w, nil); err == nil || err.Error() != hermes64KText {
		t.Fatalf("64K: %v", err)
	}
}

func TestHermesInstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake installer is a shell script")
	}
	dir := t.TempDir()
	t.Setenv("EZLM_TOOL_PATH", dir)
	home, _ := os.UserHomeDir()
	agent := filepath.Join(home, "Library", "LaunchAgents", "ai.hermes.gateway.plist")
	if runtime.GOOS != "darwin" {
		agent = filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "systemd", "user", "hermes-gateway.service")
	}
	defer os.Remove(agent)
	old := hermesInstallUnix
	defer func() { hermesInstallUnix = old }()
	hermesInstallUnix = `[ "$HERMES_NONINTERACTIVE" = 1 ] || exit 3
echo "Installing Hermes Agent"
mkdir -p "` + filepath.Dir(agent) + `" && touch "` + agent + `"
printf '#!/bin/sh\necho "Hermes Agent v0.22.5"\n' > "` + dir + `/hermes" && chmod +x "` + dir + `/hermes"`
	var lines []string
	r := NewRun("hermes-test", func(m string) { lines = append(lines, m) })
	defer r.Close()
	f, err := hermesTool{}.Resolve(context.Background(), r, true)
	if err != nil {
		t.Fatal(err)
	}
	if f.Version != "0.22.5" {
		t.Fatalf("found %+v", f)
	}
	all := strings.Join(lines, "\n")
	if !strings.Contains(all, "Installing Hermes…") || !strings.Contains(all, "hermes gateway uninstall") {
		t.Fatalf("progress:\n%s", all)
	}

	os.Remove(filepath.Join(dir, "hermes"))
	hermesInstallUnix = `echo "error: could not clone the repository"; exit 1`
	_, err = hermesTool{}.Resolve(context.Background(), r, true)
	if err == nil || !strings.Contains(err.Error(), "could not clone the repository") {
		t.Fatalf("failed install: %v", err)
	}
	if _, err := (hermesTool{}).Resolve(context.Background(), r, false); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("no install: %v", err)
	}
}

func TestHermesSourceAndFind(t *testing.T) {
	dir := t.TempDir()
	shim := filepath.Join(dir, "hermes")
	os.WriteFile(shim, []byte("#!/bin/sh\nexec /Users/a/.hermes/hermes-agent/.hermes/bin/hermes \"$@\"\n"), 0o755)
	if s := hermesSource(shim); s != SourceScript {
		t.Fatalf("shim: %s", s)
	}
	if s := hermesSource(filepath.Join(dir, "missing")); s != SourcePath {
		t.Fatalf("other: %s", s)
	}
	t.Setenv("EZLM_TOOL_PATH", "")
	if bin, _ := hermesFind(); bin != "" {
		t.Fatalf("EZLM_TOOL_PATH must limit the search: %s", bin)
	}
	if runtime.GOOS != "windows" {
		return
	}
	t.Setenv("HERMES_HOME", "")
	la := os.Getenv("LOCALAPPDATA")
	os.Unsetenv("EZLM_TOOL_PATH") // the t.Setenv above restores it; SearchDirs reads the registry PATH
	dirs := SearchDirs(hermesDirs()...)
	for _, want := range []string{filepath.Join(la, "hermes", "bin"), filepath.Join(la, "Microsoft", "WindowsApps")} {
		if !slices.Contains(dirs, want) {
			t.Errorf("search lacks %s: %v", want, dirs)
		}
	}
	if s := hermesSource(filepath.Join(la, "hermes", "bin", "hermes.cmd")); s != SourceScript {
		t.Errorf("script install: %s", s)
	}
	if s := hermesSource(filepath.Join(la, "Microsoft", "WindowsApps", "hermes.exe")); s != SourceMSIX {
		t.Errorf("app alias: %s", s)
	}
	if got := hermesDefaultConfig(); got != filepath.Join(la, "hermes", "config.yaml") {
		t.Errorf("Windows default: %s", got)
	}
}

func TestHermesHelpers(t *testing.T) {
	help, _ := os.ReadFile(filepath.Join("testdata", "hermes", "chat-help-0.22.5.txt"))
	if got := hermesFlags(string(help)); got != strings.Join(hermesCheckFlags, ",") {
		t.Fatalf("flags of 0.22.5: %s", got)
	}
	if got := hermesFlags("usage: hermes chat [-q QUERY] [-m MODEL]"); got != "-q" {
		t.Fatalf("flags: %s", got)
	}
	if v := hermesVersion("Hermes Agent v0.22.5+503.gb222bce (2026.9.24) · stable\nPython: 3.14.9"); v != "0.22.5" {
		t.Fatalf("version %q", v)
	}
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	if p := hermesPathLine("hermes: preparing dependencies…\n  ✓ done\n" + cfg + "\n"); p != cfg {
		t.Fatalf("path %q", p)
	}
	s := hermesParseStream("noise\n{\"type\":\"tool_use\",\"name\":\"read_file\"}\n{\"type\":\"result\",\"exit_code\":2,\"text\":\"\",\"error\":\"HTTP 500\"}\nsession_id: x")
	if !s.toolCall || !s.sawResult || s.err != "HTTP 500" {
		t.Fatalf("stream %+v", s)
	}
	e, ok := Explain("hermes", "openai.InternalServerError: Jinja Exception: Unexpected reasoning effort high")
	if !ok || e.State != StateNeedsUpdate {
		t.Fatalf("explain: %+v", e)
	}
	if e, ok := Explain("hermes", "hermes: no dependency environment is committed for this install; run `hermes pm repair`"); !ok || !strings.Contains(e.Say, "hermes pm repair") {
		t.Fatalf("explain: %+v", e)
	}
}

// A check run recorded from Hermes 0.22.5 (`chat -q … --format stream-json`
// against the fake model): stdout events mixed with stderr lines.
func TestHermesRecordedStream(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "hermes", "stream-json-0.22.5.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	s := hermesParseStream(string(b))
	if !s.sawResult || !s.toolCall || s.err != "" {
		t.Fatalf("stream %+v", s)
	}
	w := &CheckWorkspace{Nonce: "EZLM-CHECK-VDZMME3T"}
	if !w.Passed(s.text) {
		t.Fatalf("the nonce isn't in the final answer: %q", s.text)
	}
}
