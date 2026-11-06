package integrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRegistryOrder(t *testing.T) {
	var ids []string
	for _, tl := range All() {
		ids = append(ids, tl.Info().ID)
	}
	if strings.Join(ids, ",") != "opencode,hermes,dsh" {
		t.Fatalf("tools = %v", ids)
	}
}

func TestTargetsWireFormat(t *testing.T) {
	ts := Targets(nil)
	if len(ts) != 3 {
		t.Fatalf("targets = %+v", ts)
	}
	for _, tg := range ts {
		if tg.State != StateNotInstalled || tg.Installed || tg.Connected {
			t.Fatalf("%s: nothing is installed in the test home, got %+v", tg.ID, tg)
		}
		if tg.Icon == "" || len(tg.Launch) == 0 {
			t.Fatalf("%s: icon and launch modes are required: %+v", tg.ID, tg)
		}
	}
	b, _ := json.Marshal(ts[0])
	var m map[string]any
	json.Unmarshal(b, &m)
	for _, k := range []string{"id", "name", "desc", "supported", "why", "installed", "connected", "icon", "state", "launch"} {
		if _, ok := m[k]; !ok {
			t.Errorf("JSON lacks %q: %s", k, b)
		}
	}
	if _, ok := m["Linked"]; ok {
		t.Errorf("Linked must stay off the wire: %s", b)
	}
}

func TestDeriveState(t *testing.T) {
	ep := &Endpoint{Ctx: 32768}
	base := Target{Name: "Hermes", Installed: true, Linked: true, MinCtx: 64000}
	if s, _ := deriveState(base, ep); s != StateNeeds64K {
		t.Fatalf("small context: %s", s)
	}
	base.MinCtx = 0
	if s, _ := deriveState(base, ep); s != StateConnected {
		t.Fatalf("linked: %s", s)
	}
	base.LastCheck = &CheckResult{OK: false, Msg: "Hermes closed right after opening."}
	if s, d := deriveState(base, ep); s != StateBroken || d != base.LastCheck.Msg {
		t.Fatalf("failed check: %s %q", s, d)
	}
	base.Installed = false
	if s, _ := deriveState(base, ep); s != StateNotInstalled {
		t.Fatalf("not installed: %s", s)
	}
}

func TestRedact(t *testing.T) {
	key := "ezlm-AbCdEfGhIjKlMnOpQrStUvWxYz0123456789ab"
	s := Redact("key="+key+" other=ezlm-ZZZZZZZZZZZZ auth: Bearer abcdefghijklmnop sk-proj-1234567890abc", key)
	for _, leak := range []string{key, "ZZZZZZZZZZZZ", "abcdefghijklmnop", "1234567890abc"} {
		if strings.Contains(s, leak) {
			t.Fatalf("leaked %q: %s", leak, s)
		}
	}
}

func TestRunLogsRedacted(t *testing.T) {
	key := "ezlm-SecretSecretSecret42"
	os.Remove(LogPath("redact-test")) // -count=N reruns share the package's home
	t.Cleanup(func() { os.Remove(LogPath("redact-test")) })
	var shown []string
	r := NewRun("redact-test", func(m string) { shown = append(shown, m) }, key)
	r.Step("using key " + key)
	r.Logf("again %s", key)
	r.Close()
	b, _ := os.ReadFile(LogPath("redact-test"))
	if strings.Contains(string(b), key) || strings.Contains(strings.Join(shown, ""), key) {
		t.Fatalf("key reached the log or UI: %s / %v", b, shown)
	}
	if len(LogTail("redact-test", 10)) != 2 {
		t.Fatalf("tail = %v", LogTail("redact-test", 10))
	}
}

func TestWriteConfigBackupsAndAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "c.json")
	for i := 0; i < 7; i++ {
		if err := WriteConfig(path, []byte{byte('0' + i)}, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if b, _ := os.ReadFile(path); string(b) != "6" {
		t.Fatalf("content = %q", b)
	}
	if m, _ := filepath.Glob(path + ".ezlm-backup-*"); len(m) != 4 {
		t.Fatalf("backups = %v", m)
	}
	if m, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "*.ezlm-tmp-*")); len(m) != 0 {
		t.Fatalf("temp files left: %v", m)
	}
}

func TestCheckWorkspace(t *testing.T) {
	w, err := NewCheckWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	if !strings.HasPrefix(w.Nonce, "EZLM-CHECK-") || len(w.Nonce) != len("EZLM-CHECK-")+8 {
		t.Fatalf("nonce = %q", w.Nonce)
	}
	if strings.Contains(w.Prompt(), w.Nonce) {
		t.Fatal("the prompt must never contain the nonce")
	}
	if b, _ := os.ReadFile(filepath.Join(w.Dir, CheckFile)); !w.Passed(string(b)) || w.Passed("EZLM-CHECK-AAAAAAAA") {
		t.Fatal("Passed must match only the nonce")
	}
}

func TestExplain(t *testing.T) {
	e, ok := Explain("opencode", "Error: Database is not empty and has no session table")
	if !ok || !strings.Contains(e.Say, "2.2.5") {
		t.Fatalf("opencode text: %+v", e)
	}
	e, ok = Explain("hermes", "HTTP 401 Unauthorized: invalid api key")
	if !ok || e.Say != "Hermes has an old EZLM key. Press Connect to update it." {
		t.Fatalf("common text: %+v", e)
	}
}

func TestFindToolUsesToolPath(t *testing.T) {
	dir := t.TempDir()
	name := "faketool" + exts()[0]
	os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o755)
	if FindTool("faketool") != "" {
		t.Fatal("EZLM_TOOL_PATH is empty: nothing may be found")
	}
	t.Setenv("EZLM_TOOL_PATH", dir)
	if got := FindTool("faketool"); got != filepath.Join(dir, name) {
		t.Fatalf("FindTool = %q", got)
	}
}

func TestCacheProbeAndChecks(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "prog")
	os.WriteFile(bin, []byte("v1"), 0o755)
	SaveProbe(bin, Probe{Version: "1.0"})
	if p, ok := CachedProbe(bin); !ok || p.Version != "1.0" {
		t.Fatalf("probe = %+v %v", p, ok)
	}
	os.WriteFile(bin, []byte("v2 is longer"), 0o755)
	if _, ok := CachedProbe(bin); ok {
		t.Fatal("a changed file must invalidate its probe")
	}
	NotePort(8080)
	NotePort(8081)
	NotePort(8080)
	if p := KnownPorts(); len(p) != 2 {
		t.Fatalf("ports = %v", p)
	}
}

// fakeTool records what the Setup pipeline called.
type fakeTool struct {
	calls    []string
	checkErr error
}

func (f *fakeTool) Info() Info {
	return Info{ID: "fake", Name: "Fake", Icon: "fake", Order: 99, Launch: []LaunchMode{{ID: ModeTUI, Label: "Open Fake"}}}
}
func (f *fakeTool) Detect(*Endpoint) Detection {
	return Detection{Found: Found{Bin: "/fake"}, Linked: true}
}
func (f *fakeTool) Resolve(context.Context, *Run, bool) (Found, error) {
	f.calls = append(f.calls, "resolve")
	return Found{Bin: "/fake"}, nil
}
func (f *fakeTool) Configure(Endpoint, bool) error {
	f.calls = append(f.calls, "configure")
	return nil
}
func (f *fakeTool) Check(_ context.Context, _ Found, _ Endpoint, w *CheckWorkspace, _ *Run) (bool, error) {
	f.calls = append(f.calls, "check")
	if _, err := os.Stat(w.File); err != nil {
		return false, err
	}
	return true, f.checkErr
}
func (f *fakeTool) Preflight(context.Context, Found, Endpoint, *Run) error {
	f.calls = append(f.calls, "preflight")
	return nil
}
func (f *fakeTool) LaunchSpec(Found, Endpoint, string) (LaunchSpec, error) {
	f.calls = append(f.calls, "spec")
	return LaunchSpec{Bin: "/fake"}, nil
}

func TestSetupPipeline(t *testing.T) {
	ft := &fakeTool{}
	defer Replace(ft)()
	ep := Endpoint{BaseURL: "http://127.0.0.1:1/v1", Key: "ezlm-pipelinekey", ModelID: "m", Ctx: 65536}
	if err := Setup(context.Background(), "fake", ep, false, nil); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(ft.calls, ","); got != "resolve,configure,check,preflight,spec" {
		t.Fatalf("calls = %s", got)
	}
	lc := LastCheck("fake")
	if lc == nil || !lc.OK || !lc.ToolCall || lc.LogPath == "" {
		t.Fatalf("last check = %+v", lc)
	}

	ft.calls, ft.checkErr = nil, errors.New("Fake could not reach the model.")
	if err := Setup(context.Background(), "fake", ep, false, nil); err == nil {
		t.Fatal("a failed check must fail Setup")
	}
	for _, tg := range Targets(&ep) {
		if tg.ID == "fake" && (tg.State != StateBroken || tg.Detail != "Fake could not reach the model.") {
			t.Fatalf("after a failed check: %+v", tg)
		}
	}

	l, err := Open(context.Background(), "fake", "", ep, nil)
	if err != nil || !l.DryRun {
		t.Fatalf("open under EZLM_LAUNCH_DRYRUN: %+v %v", l, err)
	}
}

func TestPinsEmbeddedAndCatalogOverride(t *testing.T) {
	for tool, keys := range map[string][]string{
		"opencode": {"windows-arm64", "windows-x64", "windows-x64-baseline", "darwin-arm64", "darwin-x64", "darwin-x64-baseline", "linux-x64", "linux-arm64"},
		"node":     {"windows-x64", "windows-arm64", "darwin-arm64", "darwin-x64", "linux-x64", "linux-arm64"},
	} {
		p, ok := embedded[tool] // tools.json; a tool's RegisterPin may replace it
		if !ok {
			t.Fatalf("no %s pin", tool)
		}
		for _, k := range keys {
			a, ok := p.Assets[k]
			if !ok || len(a.SHA256) != 64 || !strings.HasPrefix(a.URL, "https://") || !strings.Contains(a.URL, p.Version) || a.Size == 0 {
				t.Errorf("%s %s: %+v", tool, k, a)
			}
		}
	}
	if p := embedded["node"]; !strings.HasPrefix(p.Version, "24.") {
		t.Fatalf("node pin %s: dsh runs on Node 24 LTS", p.Version)
	}
	for tool := range embedded {
		p, _ := PinFor(tool)
		e := embedded[tool]
		if p.Version != e.Version {
			t.Errorf("%s: registered %s, tools.json %s", tool, p.Version, e.Version)
		}
		for k, a := range p.Assets {
			if e.Assets[k].SHA256 != a.SHA256 {
				t.Errorf("%s %s: registered digest differs from tools.json", tool, k)
			}
		}
	}
	if p, _ := PinFor("dsh"); p.Version != "0.2.1-rc.5" || p.Package != "@deepseek-ai/dsh" || !strings.HasPrefix(p.Integrity, "sha512-") {
		t.Fatalf("dsh pin %+v", p)
	}

	// The catalog wins, but only with checksums; a bad section changes nothing.
	defer UseCatalogPins(nil)
	err := UseCatalogPins([]byte(`[{"tool":"opencode","version":"2.2.6","assets":{
		"darwin-arm64":{"url":"https://example.invalid/oc-2.2.6.zip","sha256":"` + strings.Repeat("ab", 32) + `"},
		"windows-x64":{"url":"https://example.invalid/oc.zip","sha256":"nope"},
		"linux-x64":{"url":"http://example.invalid/oc.zip","sha256":"` + strings.Repeat("cd", 32) + `"}}},
		{"version":"1"}]`))
	if err != nil {
		t.Fatal(err)
	}
	p, _ := PinFor("opencode")
	if p.Version != "2.2.6" || len(p.Assets) != 1 || p.Assets["darwin-arm64"].SHA256 == "" {
		t.Fatalf("catalog pin = %+v", p)
	}
	if UseCatalogPins([]byte(`{"not":"a list"}`)) == nil {
		t.Fatal("an invalid tools section must be reported")
	}
	UseCatalogPins(nil)
	if p, _ := PinFor("opencode"); p.Version != "2.2.5" {
		t.Fatalf("cleared override: %+v", p)
	}
	if AssetKey("windows", "amd64", true) != "windows-x64-baseline" || AssetKey("darwin", "arm64", false) != "darwin-arm64" {
		t.Fatal("AssetKey")
	}
}

func TestFetchAssetChecksum(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "a.zip")
	if err := FetchAsset(context.Background(), nil, Asset{URL: "https://example.invalid/a.zip"}, dst); err == nil || !strings.Contains(err.Error(), "no checksum") {
		t.Fatalf("an asset without a digest must be refused: %v", err)
	}
	body := []byte("pinned bytes")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	defer srv.Close()
	sum := sha256.Sum256(body)
	good := Asset{URL: srv.URL + "/a.zip", SHA256: hex.EncodeToString(sum[:]), Size: uint64(len(body))}
	if err := FetchAsset(context.Background(), nil, good, dst); err != nil {
		t.Fatal(err)
	}
	bad := good
	bad.SHA256 = strings.Repeat("0", 64)
	if err := FetchAsset(context.Background(), nil, bad, dst); err == nil || !strings.Contains(err.Error(), "didn't match its checksum") {
		t.Fatalf("bad digest: %v", err)
	}
	if _, err := os.Stat(dst); err == nil {
		t.Fatal("a download that fails its checksum must be deleted")
	}
	if !IsEZLMTool(filepath.Join(ToolDir("opencode", "2.2.5"), "opencode")) || IsEZLMTool(filepath.Join(t.TempDir(), "opencode")) {
		t.Fatal("IsEZLMTool")
	}
}

func TestErrLine(t *testing.T) {
	for in, want := range map[string]string{
		"Downloading...\n#####          45%\n\x1b[31mError: EACCES: permission denied, mkdir '/usr/lib/x'\x1b[0m\nnpm error A complete log of this run can be found in: /x.log\n": "Error: EACCES: permission denied, mkdir '/usr/lib/x'",
		"Traceback (most recent call last):\n  File \"x.py\", line 1\nValueError: below the minimum 64,000":                                                                       "ValueError: below the minimum 64,000",
		"step one\nstep two done\n[=====>     ] 12/40\n": "step two done",
		"": "no output",
	} {
		if got := ErrLine(in); got != want {
			t.Errorf("ErrLine(%q) = %q, want %q", in, got, want)
		}
	}
	if got := ErrLine(strings.Repeat("error ", 100)); len([]rune(got)) > 201 {
		t.Fatalf("long line kept: %d", len(got))
	}
}

func TestExplainCommonTexts(t *testing.T) {
	for out, state := range map[string]string{
		"HTTP 500: Jinja Exception: Unexpected reasoning effort high":                            StateNeedsUpdate,
		"Model has a context window of 32768 tokens, which is below the minimum 64,000 required": StateNeeds64K,
		"connect ECONNREFUSED 127.0.0.1:8080":                                                    "",
	} {
		e, ok := Explain("nosuchtool", out)
		if !ok || e.State != state || strings.Contains(e.Say, "{app}") {
			t.Errorf("Explain(%q) = %+v %v", out, e, ok)
		}
	}
	if _, ok := Explain("opencode", "all good"); ok {
		t.Fatal("no match expected")
	}
}

func TestWriteFileAtomicFailureAndSymlink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.json")
	os.WriteFile(path, []byte("original"), 0o600)
	old := renameFile
	renameFile = func(string, string) error { return errors.New("injected") }
	err := WriteConfig(path, []byte("new"), 0o600)
	renameFile = old
	if err == nil {
		t.Fatal("the injected failure must be returned")
	}
	if b, _ := os.ReadFile(path); string(b) != "original" {
		t.Fatalf("original changed: %q", b)
	}
	if m, _ := filepath.Glob(filepath.Join(dir, "*.ezlm-tmp-*")); len(m) != 0 {
		t.Fatalf("temp left: %v", m)
	}

	real := filepath.Join(dir, "dotfiles", "real.json")
	os.MkdirAll(filepath.Dir(real), 0o755)
	os.WriteFile(real, []byte("a"), 0o600)
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink(filepath.Join("dotfiles", "real.json"), link); err != nil {
		t.Skip("no symlinks here:", err)
	}
	if err := WriteConfig(link, []byte("b"), 0o600); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Lstat(link); st.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the symlink was replaced by a file")
	}
	if b, _ := os.ReadFile(real); string(b) != "b" {
		t.Fatalf("target = %q", b)
	}
	if m, _ := filepath.Glob(real + ".ezlm-backup-*"); len(m) != 1 {
		t.Fatalf("the backup goes next to the real file: %v", m)
	}
}

func TestClearLaunchFailure(t *testing.T) {
	good := CheckResult{At: time.Now(), OK: true, ToolCall: true, Stage: StageModel, Msg: "answered and used a tool (3 s)"}
	SaveCheck("clr", good)
	SaveCheck("clr", CheckResult{Stage: StageLaunch, Msg: "Clr closed right after opening (exit code 1)."})
	SaveCheck("clr", CheckResult{Stage: StageLaunch, Msg: "again"})
	if lc := LastCheck("clr"); lc.OK || lc.Msg != "again" {
		t.Fatalf("last = %+v", lc)
	}
	ClearLaunchFailure("clr")
	if lc := LastCheck("clr"); lc == nil || !lc.OK || lc.Msg != good.Msg {
		t.Fatalf("restored = %+v", lc)
	}
	ClearLaunchFailure("clr") // nothing to clear: stays
	if lc := LastCheck("clr"); lc == nil || !lc.OK {
		t.Fatalf("after a second clear = %+v", lc)
	}
	SaveCheck("clr2", CheckResult{Stage: StageLaunch, Msg: "x"})
	ClearLaunchFailure("clr2")
	if LastCheck("clr2") != nil {
		t.Fatal("a launch failure with nothing before it is dropped")
	}
}

func TestCheckDisk(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "not", "yet")
	if err := CheckDisk(dir, 1, "Thing"); err != nil {
		t.Fatal(err)
	}
	err := CheckDisk(dir, 1<<62, "Thing")
	if err == nil || !strings.Contains(err.Error(), "installing Thing needs about") {
		t.Fatalf("err = %v", err)
	}
}

func TestSetupNeeds64KSentence(t *testing.T) {
	ft := &fakeTool{}
	defer Replace(&ctxTool{fakeTool: ft})()
	err := Setup(context.Background(), "fake", Endpoint{Ctx: 32768, Key: "ezlm-x123456"}, false, nil)
	if err == nil || err.Error() != NeedsCtxDetail("Fake", 64000) {
		t.Fatalf("err = %v", err)
	}
}

// ctxTool is fakeTool with Hermes's 64K floor.
type ctxTool struct{ *fakeTool }

func (c *ctxTool) Info() Info {
	i := c.fakeTool.Info()
	i.MinCtx = 64000
	return i
}
func (c *ctxTool) Configure(ep Endpoint, _ bool) error {
	if ep.Ctx < 64000 {
		return ErrNeeds64K
	}
	return nil
}

func TestHoldsKey(t *testing.T) {
	key := "custom-server-secret-value"
	if holdsKey(LaunchSpec{Bin: "/x/app", Args: []string{"run"}, Env: []string{"PWD=/x"}}, key) {
		t.Fatal("a clean spec holds no key")
	}
	for _, s := range []LaunchSpec{
		{Bin: "/x/app", Args: []string{"--auth", key}},
		{Bin: "/x/app", Env: []string{"OTHER=" + key}},
		{Bin: "/x/app", URL: "http://127.0.0.1:3080/?k=" + key},
	} {
		if !holdsKey(s, key) {
			t.Errorf("%+v holds the key", s)
		}
	}
	if holdsKey(LaunchSpec{Bin: "/x/app"}, "") {
		t.Fatal("a server without a key: nothing to find")
	}
}
