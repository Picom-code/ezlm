package integrate

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func dshTestEP() Endpoint {
	return Endpoint{BaseURL: "http://127.0.0.1:8081/v1", Key: "ezlm-NEWKEY1234567890", ModelID: "qwen3.8-27b",
		ModelName: "Qwen3.8 27B", Ctx: 65536, ToolCalls: true}
}

func dshTestFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "dsh", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// dshTestHome is a fresh DSH_HOME with fixtures placed as profile patches
// (profile name -> fixture name).
func dshTestHome(t *testing.T, profiles map[string]string) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), ".dsh")
	for p, fx := range profiles {
		path := dshPatchPath(home, p)
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, dshTestFixture(t, fx), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	os.MkdirAll(home, 0o700)
	return home
}

// dshTestWithout is after with EZLM's provider pair (and nothing else) cut
// out, which must give back the file as it was.
func dshTestWithout(t *testing.T, after []byte) []byte {
	t.Helper()
	d, err := dshParse(after)
	if err != nil {
		t.Fatal(err)
	}
	prov := dshProviders(d.root())
	i := dshPairIndex(prov, dshProvider)
	if i < 0 {
		t.Fatalf("no ezlm provider in:\n%s", after)
	}
	k := prov.Content[i]
	return append(append([]byte{}, after[:d.offset(k.Line, 1)]...), after[d.offset(d.pairEnd(prov, i), 1):]...)
}

func TestDshMergeKeepsSparkByteForByte(t *testing.T) {
	src := dshTestFixture(t, "web-spark.cordis.patch.yml")
	out, err := dshUpsertProvider(src, dshProviderNode(dshTestEP()))
	if err != nil {
		t.Fatal(err)
	}
	if got := dshTestWithout(t, out); !bytes.Equal(got, src) {
		t.Fatalf("the rest of the file changed:\n--- got\n%s\n--- want\n%s", got, src)
	}
	d, _ := dshParse(out)
	prov := dshProviders(d.root())
	if names := dshProviderNames(prov); len(names) != 1 || names[0] != "spark" {
		t.Fatalf("providers = %v", names)
	}
	// The default model stays the user's without makeDefault.
	out2, err := dshSetDefault(out, dshTestEP(), false)
	if err != nil || !bytes.Equal(out2, out) {
		t.Fatalf("default row changed without makeDefault (err %v)", err)
	}
	// The ezlm entry sits with the other providers, at their indent.
	if !strings.Contains(string(out), "\n      ezlm:\n        displayName: EZLM (local)\n") {
		t.Fatalf("ezlm entry not where expected:\n%s", out)
	}
}

func TestDshWriteProfileIdempotentAndBackups(t *testing.T) {
	home := dshTestHome(t, map[string]string{"web": "web-spark.cordis.patch.yml"})
	path := dshPatchPath(home, "web")
	before, _ := os.ReadFile(path)
	others, err := WriteDshProfile(path, dshTestEP(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(others) != 1 || others[0] != "spark" {
		t.Fatalf("others = %v", others)
	}
	first, _ := os.ReadFile(path)
	if _, err := WriteDshProfile(path, dshTestEP(), false); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(path)
	if !bytes.Equal(first, second) {
		t.Fatal("a second write changed the file")
	}
	backups, _ := filepath.Glob(path + ".ezlm-backup-*")
	if len(backups) != 1 {
		t.Fatalf("backups = %v, want exactly the first one", backups)
	}
	if b, _ := os.ReadFile(backups[0]); !bytes.Equal(b, before) {
		t.Fatal("the backup isn't the original file")
	}
	if st, _ := os.Stat(path); runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want the file's own 0600", st.Mode().Perm())
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "package.json.lock")); !os.IsNotExist(err) {
		t.Fatal("the profile lock was left behind")
	}
}

func TestDshCreatesRowInEmptyTemplate(t *testing.T) {
	src := dshTestFixture(t, "headless-empty.cordis.patch.yml")
	out, err := dshUpsertProvider(src, dshProviderNode(dshTestEP()))
	if err != nil {
		t.Fatal(err)
	}
	header := string(src[:bytes.LastIndex(src, []byte("[]"))])
	if !strings.HasPrefix(string(out), header+"- id: llm-pi-ai\n  config:\n    providers:\n      ezlm:\n") {
		t.Fatalf("got:\n%s", out)
	}
	d, err := dshParse(out)
	if err != nil || len(d.root().Content) != 1 {
		t.Fatalf("rows: %v %v", err, d.root())
	}
	// A missing file starts from dsh's own template.
	home := dshTestHome(t, nil)
	path := dshPatchPath(home, "headless")
	if _, err := WriteDshProfile(path, dshTestEP(), false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(b), "# Your patch layer for this dsh profile") || !strings.Contains(string(b), "ezlm:") {
		t.Fatalf("new file:\n%s", b)
	}
}

func TestDshDefaultModelOnlyWithMakeDefault(t *testing.T) {
	ep := dshTestEP()
	src := dshTestFixture(t, "web-spark.cordis.patch.yml")
	out, err := dshSetDefault(src, ep, true)
	if err != nil {
		t.Fatal(err)
	}
	d, _ := dshParse(out)
	i := dshRowIndex(d.root(), dshModelRow, "")
	cfg := yamlChild(d.root().Content[i], "config")
	if yamlChild(cfg, "provider").Value != "ezlm" || yamlChild(cfg, "model").Value != ep.ModelID || yamlChild(cfg, "reasoningEffort") != nil {
		t.Fatalf("default row:\n%s", out)
	}
	// Only that row's config changed.
	want := strings.Replace(string(src), "    provider: spark\n    model: deepseek-v4.1-flash\n    reasoningEffort: high\n- id: ui-settings-general",
		"    provider: ezlm\n    model: qwen3.8-27b\n- id: ui-settings-general", 1)
	if string(out) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}
	// No row and no makeDefault: nothing to do; with makeDefault a row is added.
	empty := dshTestFixture(t, "headless-empty.cordis.patch.yml")
	if out, _ := dshSetDefault(empty, ep, false); !bytes.Equal(out, empty) {
		t.Fatal("added a default row without makeDefault")
	}
	out, _ = dshSetDefault(empty, ep, true)
	if !strings.Contains(string(out), "- id: agent-default-model\n  config:\n    provider: ezlm\n    model: qwen3.8-27b\n") {
		t.Fatalf("got:\n%s", out)
	}
}

func TestDshReplacesOldEntryAndFollowsModelSwitch(t *testing.T) {
	src := dshTestFixture(t, "existing-ezlm.cordis.patch.yml")
	ep := dshTestEP()
	ep.Reasoning = Reasoning{Levels: []string{"low", "medium", "xhigh"}}
	home := dshTestHome(t, map[string]string{"web": "existing-ezlm.cordis.patch.yml"})
	path := dshPatchPath(home, "web")
	others, err := WriteDshProfile(path, ep, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(others) != 1 || others[0] != "spark" {
		t.Fatalf("others = %v", others)
	}
	out, _ := os.ReadFile(path)
	b, _ := dshParse(src)
	a, _ := dshParse(out)
	bp, ap := dshProviders(b.root()), dshProviders(a.root())
	if !bytes.Equal(b.pairText(bp, dshPairIndex(bp, "spark")), a.pairText(ap, dshPairIndex(ap, "spark"))) {
		t.Fatalf("spark changed:\n%s", out)
	}
	if !dshSame(ap.Content[dshPairIndex(ap, "ezlm")+1], dshProviderNode(ep)) {
		t.Fatalf("ezlm entry not replaced:\n%s", out)
	}
	if !strings.Contains(string(out), "baseURL: !!js process.env.SPARK_URL ?? 'https://spark.tailnet.example/v1'") {
		t.Fatal("the !!js tag was lost")
	}
	// The default row used EZLM: it follows the new model, and the "high"
	// level this template rejects is dropped.
	i := dshRowIndex(a.root(), dshModelRow, "")
	cfg := yamlChild(a.root().Content[i], "config")
	if yamlChild(cfg, "model").Value != ep.ModelID || yamlChild(cfg, "reasoningEffort") != nil {
		t.Fatalf("default row:\n%s", out)
	}
	if !strings.HasPrefix(string(out), "# Written by an earlier EZLM, on another port.\n") {
		t.Fatal("header comment lost")
	}
}

func TestDshTrickyLayouts(t *testing.T) {
	src := dshTestFixture(t, "tricky.cordis.patch.yml")
	out, err := dshUpsertProvider(src, dshProviderNode(dshTestEP()))
	if err != nil {
		t.Fatal(err)
	}
	if got := dshTestWithout(t, out); !bytes.Equal(got, src) {
		t.Fatalf("the rest of the file changed:\n--- got\n%s\n--- want\n%s", got, src)
	}
	d, _ := dshParse(out)
	if names := dshProviderNames(dshProviders(d.root())); strings.Join(names, ",") != "local-vllm,openrouter" {
		t.Fatalf("providers = %v\n%s", names, out)
	}
	// CRLF files keep CRLF.
	crlf := bytes.ReplaceAll(dshTestFixture(t, "web-spark.cordis.patch.yml"), []byte("\n"), []byte("\r\n"))
	out, err = dshUpsertProvider(crlf, dshProviderNode(dshTestEP()))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(bytes.ReplaceAll(out, []byte("\r\n"), nil), []byte("\n")) {
		t.Fatal("a bare LF crept into a CRLF file")
	}
	if got := dshTestWithout(t, out); !bytes.Equal(got, crlf) {
		t.Fatal("the rest of the CRLF file changed")
	}
}

func TestDshRowShapes(t *testing.T) {
	ep := dshTestEP()
	for name, src := range map[string]string{
		"row without config":    "- id: llm-pi-ai\n  name: '@deepseek-ai/dsh-llm-pi-ai'\n- id: ui-theme\n",
		"config without routes": "- id: llm-pi-ai\n  config:\n    retryPolicy:\n      mode: normal\n",
		"flow providers":        "- id: llm-pi-ai\n  config:\n    providers: {spark: {api: openai-completions}}\n",
		"null config":           "- id: llm-pi-ai\n  config:\n",
		"flow config":           "- id: llm-pi-ai\n  config: {providers: {spark: {api: x}}}\n",
		"flow config no routes": "- id: llm-pi-ai\n  config: {retryPolicy: {mode: normal}}\n",
		"config first":          "- config:\n    providers:\n      spark: {api: x}\n  id: llm-pi-ai\n",
		"other plugin same id":  "- id: llm-pi-ai\n  name: '@acme/other'\n  config: {x: 1}\n",
		"insert row":            "- insert: [{id: llm-pi-ai}]\n",
		"comments only":         "# nothing here yet\n",
		"empty":                 "",
		"no trailing newline":   "- id: ui-theme\n  config:\n    preference: dark",
	} {
		t.Run(name, func(t *testing.T) {
			out, err := dshUpsertProvider([]byte(src), dshProviderNode(ep))
			if err != nil {
				t.Fatal(err)
			}
			d, err := dshParse(out)
			if err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
			prov := dshProviders(d.root())
			if i := dshPairIndex(prov, dshProvider); i < 0 || !dshSame(prov.Content[i+1], dshProviderNode(ep)) {
				t.Fatalf("no ezlm entry:\n%s", out)
			}
			again, _ := dshUpsertProvider(out, dshProviderNode(ep))
			if !bytes.Equal(again, out) {
				t.Fatalf("second write differs:\n%s\n---\n%s", out, again)
			}
		})
	}
	if _, err := dshUpsertProvider([]byte("providers: {}\n"), dshProviderNode(ep)); err == nil {
		t.Fatal("a mapping at the top should be refused")
	}
}

func TestDshReasoningLevels(t *testing.T) {
	ep := dshTestEP()
	n := dshProviderNode(ep)
	compat := yamlChild(n, "compat")
	model := yamlChild(n, "models").Content[0]
	if yamlChild(compat, "supportsReasoningEffort").Value != "false" || yamlChild(compat, "thinkingFormat") != nil ||
		yamlChild(model, "reasoningEfforts").Value != "false" {
		t.Fatal("no levels must mean no reasoning settings")
	}
	if yamlChild(model, "contextWindow").Value != "65536" || yamlChild(model, "maxTokens").Value != "16384" {
		t.Fatal("contextWindow and maxTokens must be explicit")
	}
	ep.Reasoning = Reasoning{Levels: []string{"low", "medium", "xhigh"}, Thinking: true}
	n = dshProviderNode(ep)
	model = yamlChild(n, "models").Content[0]
	b, _ := yamlEncode(yamlChild(model, "reasoningEfforts"))
	if string(b) != "off:\nlow: low\nmedium: medium\nxhigh: xhigh\n" {
		t.Fatalf("reasoningEfforts:\n%s", b)
	}
	if yamlChild(yamlChild(n, "compat"), "supportsReasoningEffort").Value != "true" {
		t.Fatal("levels need supportsReasoningEffort")
	}
	if dshMaxTokens(262144) != 32768 || dshMaxTokens(2048) != 1024 {
		t.Fatal("maxTokens clamp")
	}
}

func TestDshEnvMerge(t *testing.T) {
	src := dshTestFixture(t, "env-with-others.env")
	out := string(dshMergeEnv(src, "ezlm-NEWKEY1234567890"))
	want := "# DeepSeek Harness home .env\nSPARK_API_KEY=REDACTED-KEY\nEZLM_API_KEY=ezlm-NEWKEY1234567890\nHTTPS_PROXY=http://proxy.example:3128 # keep me\n"
	if out != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}
	if got := dshEnvKey([]byte(out)); got != "ezlm-NEWKEY1234567890" {
		t.Fatalf("read back %q", got)
	}
	if got := string(dshMergeEnv([]byte("A=1"), "k")); got != "A=1\nEZLM_API_KEY=k\n" {
		t.Fatalf("append: %q", got)
	}
	if got := dshEnvKey(dshMergeEnv(nil, `we"ird #key`)); got != `we"ird #key` {
		t.Fatalf("quoted round trip: %q", got)
	}
	home := dshTestHome(t, nil)
	os.WriteFile(dshEnvPath(home), src, 0o644)
	if err := WriteDshEnv(home, "ezlm-NEWKEY1234567890"); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(dshEnvPath(home)); runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 {
		t.Fatalf(".env mode %v", st.Mode().Perm())
	}
}

func TestDshStoredRef(t *testing.T) {
	home := dshTestHome(t, nil)
	path := dshCredentialsPath(home)
	src := dshTestFixture(t, "credentials-with-ref.yaml")
	os.WriteFile(path, src, 0o600)
	if err := UpdateDshStoredRef(home, "ezlm-NEWKEY1234567890"); err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(path)
	want := strings.Replace(string(src), "  EZLM_API_KEY: ezlm-OLDKEY0000000000 # written by the web Models page\n", "  EZLM_API_KEY: ezlm-NEWKEY1234567890\n", 1)
	if string(out) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}
	if v, ok := dshStoredRef(home); !ok || v != "ezlm-NEWKEY1234567890" {
		t.Fatal("ref not updated")
	}
	if st, _ := os.Stat(path); runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", st.Mode().Perm())
	}
	if _, err := os.Stat(path + ".lock"); !os.IsNotExist(err) {
		t.Fatal("lock left behind")
	}
	if b, _ := filepath.Glob(path + ".ezlm-backup-*"); len(b) != 0 {
		t.Fatal("a credential store must not be copied")
	}
	// No ref: EZLM adds none and leaves the file alone.
	none := dshTestFixture(t, "credentials-no-ref.yaml")
	os.WriteFile(path, none, 0o600)
	if err := UpdateDshStoredRef(home, "ezlm-NEWKEY1234567890"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); !bytes.Equal(b, none) {
		t.Fatal("added a ref")
	}
}

func TestDshLockTakeOverAndWait(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "f")
	// A lock left by a process that is gone is taken over.
	os.WriteFile(target+".lock", []byte("2147480000\n"), 0o600)
	ran := false
	if err := dshWithLock(target, 2*time.Second, func() error { ran = true; return nil }); err != nil || !ran {
		t.Fatalf("stale lock: %v", err)
	}
	// A live holder (this process) is waited for, then reported.
	os.WriteFile(target+".lock", []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600)
	err := dshWithLock(target, 100*time.Millisecond, func() error { t.Fatal("ran under someone else's lock"); return nil })
	if err == nil || !strings.Contains(err.Error(), "saving its settings") {
		t.Fatalf("err = %v", err)
	}
	os.Remove(target + ".lock")
}

func TestDshWriteConfigAllProfiles(t *testing.T) {
	home := dshTestHome(t, map[string]string{
		"web":      "web-spark.cordis.patch.yml",
		"headless": "headless-empty.cordis.patch.yml",
		"desktop":  "desktop-spark.cordis.patch.yml",
	})
	os.WriteFile(dshCredentialsPath(home), dshTestFixture(t, "credentials-with-ref.yaml"), 0o600)
	ep := dshTestEP()
	if err := WriteDshConfig(home, ep, false); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"web", "headless", "desktop"} {
		b, _ := os.ReadFile(dshPatchPath(home, p))
		d, _ := dshParse(b)
		if prov := dshProviders(d.root()); dshPairIndex(prov, dshProvider) < 0 {
			t.Fatalf("%s has no ezlm provider", p)
		}
	}
	if _, err := os.Stat(filepath.Join(home, "cordis.patch.yml")); !os.IsNotExist(err) {
		t.Fatal("wrote the home-level patch file")
	}
	st := dshReadState(home, &ep)
	if !st.linked || !st.current {
		t.Fatalf("state %+v", st)
	}
	ep2 := ep
	ep2.Key = "ezlm-ROTATED000000000"
	if st := dshReadState(home, &ep2); st.current {
		t.Fatal("a new key must read as needs_update")
	}
	if err := WriteDshConfig(home, ep2, false); err != nil {
		t.Fatal(err)
	}
	if v, _ := dshStoredRef(home); v != ep2.Key {
		t.Fatal("rotation didn't reach the stored ref")
	}
	if st := dshReadState(home, &ep2); !st.current {
		t.Fatal("rotation didn't make it current")
	}
	// The web profile without desktop: desktop is only touched when it exists.
	home2 := dshTestHome(t, map[string]string{"web": "web-spark.cordis.patch.yml"})
	WriteDshConfig(home2, ep, false)
	if _, err := os.Stat(dshProfileDir(home2, "desktop")); !os.IsNotExist(err) {
		t.Fatal("created a desktop profile")
	}
}

func TestDshSameIgnoresStyle(t *testing.T) {
	var a, b yaml.Node
	yaml.Unmarshal([]byte("x: {a: 1, 'b': \"two\", c: ~}\n"), &a)
	yaml.Unmarshal([]byte("x:\n  a: 1 # one\n  b: two\n  c:\n"), &b)
	if !dshSame(&a, &b) {
		t.Fatal("same meaning, different style")
	}
	yaml.Unmarshal([]byte("x: {a: '1', b: two, c: ~}\n"), &b)
	if dshSame(&a, &b) {
		t.Fatal("a string 1 isn't the int 1")
	}
}
