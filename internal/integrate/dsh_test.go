package integrate

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// dshTestInstall lays out a dsh package the way npm does on Windows (a
// shim next to node_modules), which dshScriptFor finds on every OS, and
// returns the folder to put on EZLM_TOOL_PATH.
func dshTestInstall(t *testing.T, version string) string {
	t.Helper()
	dir := t.TempDir()
	pkg := filepath.Join(dir, "node_modules", "@deepseek-ai", "dsh")
	os.MkdirAll(filepath.Join(pkg, "lib"), 0o755)
	os.WriteFile(filepath.Join(pkg, "package.json"), []byte(`{"name":"@deepseek-ai/dsh","version":"`+version+`"}`), 0o644)
	os.WriteFile(filepath.Join(pkg, "lib", "bin.js"), []byte("// fake\n"), 0o644)
	name := "dsh"
	if runtime.GOOS == "windows" {
		name += ".cmd"
	}
	os.WriteFile(filepath.Join(dir, name), []byte("fake shim\n"), 0o755)
	return dir
}

// dshTestNode writes a fake node into dir: it answers --version, and run on
// lib/bin.js it plays dsh's headless JSON events and config dumps (Unix
// only; it is a shell script).
func dshTestNode(t *testing.T, dir, version string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake node is a shell script")
	}
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then echo "v` + version + `"; exit 0; fi
shift # lib/bin.js
case " $* " in
*" --version "*) echo 0.2.1-rc.5; exit 0 ;;
*" --dump-config "*) [ -n "$FAKE_DUMP_WARN" ] && echo "$FAKE_DUMP_WARN" >&2; cat "$FAKE_DUMP"; exit 0 ;;
*" --dump-default-config "*) echo "[]"; exit 0 ;;
esac
grep -q "ezlm:" "$DSH_HOME/profiles/headless/cordis.patch.yml" || { echo "dsh: UNKNOWN_MODEL: no ezlm" >&2; exit 1; }
grep -q "^EZLM_API_KEY=" "$DSH_HOME/.env" || { echo "dsh: MISSING_CREDENTIAL: llm-pi-ai" >&2; exit 1; }
[ "$DSH_TELEMETRY_DISABLED" = 1 ] && [ "$DSH_TELEMETRY_MODE" = DISABLED ] || { echo "telemetry on" >&2; exit 1; }
n=$(cat ezlm-check.txt)
echo '{"type":"session","sessionId":"s1","cwd":"'"$PWD"'"}'
echo '{"type":"tool_call","callId":"c1","tool":"read","input":{"file_path":"ezlm-check.txt"}}'
echo '{"type":"final","text":"'"$n"'"}'
`
	p := filepath.Join(dir, "node")
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDshDetect(t *testing.T) {
	ep := dshTestEP()
	t.Setenv("DSH_HOME", filepath.Join(t.TempDir(), ".dsh"))
	d := dshTool{}.Detect(&ep)
	if d.Bin != "" || d.Linked || d.Config != dshPatchPath(dshHome(), "web") {
		t.Fatalf("nothing installed: %+v", d)
	}

	t.Setenv("EZLM_TOOL_PATH", dshTestInstall(t, "0.2.2-rc.1"))
	d = dshTool{}.Detect(&ep)
	if d.Bin != "" || !strings.Contains(d.Detail, "0.2.2-rc.1") {
		t.Fatalf("untested version: %+v", d)
	}

	dir := dshTestInstall(t, "0.2.1-rc.5")
	t.Setenv("EZLM_TOOL_PATH", dir)
	d = dshTool{}.Detect(&ep)
	if d.Bin == "" || d.Version != "0.2.1-rc.5" || d.Source != SourceNPM || d.State != "" {
		t.Fatalf("tested version: %+v", d)
	}

	if err := WriteDshConfig(dshHome(), ep, false); err != nil {
		t.Fatal(err)
	}
	if d = (dshTool{}).Detect(&ep); !d.Linked || d.State != "" {
		t.Fatalf("connected: %+v", d)
	}
	moved := ep
	moved.BaseURL = "http://127.0.0.1:8090/v1"
	if d = (dshTool{}).Detect(&moved); d.State != StateNeedsUpdate {
		t.Fatalf("port move: %+v", d)
	}
	os.WriteFile(dshCredentialsPath(dshHome()), dshTestFixture(t, "credentials-with-ref.yaml"), 0o600)
	if d = (dshTool{}).Detect(&ep); d.State != StateNeedsUpdate {
		t.Fatalf("a stored ref with an old key wins over .env: %+v", d)
	}
	noTools := ep
	noTools.ToolCalls = false
	if d = (dshTool{}).Detect(&noTools); d.State != StateNeedsTools || d.Detail == "" {
		t.Fatalf("no tools: %+v", d)
	}
	if d = (dshTool{}).Detect(nil); !d.Linked {
		t.Fatalf("no model: %+v", d)
	}
}

func TestDshEZLMCopy(t *testing.T) {
	p, _ := PinFor("dsh")
	prefix := dshPrefix(p.Version)
	defer os.RemoveAll(prefix)
	pkg := filepath.Join(prefix, "node_modules", "@deepseek-ai", "dsh")
	os.MkdirAll(filepath.Join(pkg, "lib"), 0o755)
	os.WriteFile(filepath.Join(pkg, "package.json"), []byte(`{"name":"@deepseek-ai/dsh","version":"`+p.Version+`"}`), 0o644)
	os.WriteFile(filepath.Join(pkg, "lib", "bin.js"), nil, 0o644)
	if _, ok := dshEZLMCopy(p.Version); ok {
		t.Fatal("an install without its marker must not count")
	}
	os.WriteFile(dshMarker(prefix), []byte(`{"version":"`+p.Version+`","node":"/x/node"}`), 0o644)
	c, ok := dshEZLMCopy(p.Version)
	if !ok || c.Source != SourceEZLM || c.Node != "/x/node" || !IsEZLMTool(c.Bin) {
		t.Fatalf("copy %+v %v", c, ok)
	}
	usable, _ := dshCopies()
	if len(usable) != 1 || usable[0].Source != SourceEZLM {
		t.Fatalf("usable %+v", usable)
	}
}

func TestDshCheckIntegrity(t *testing.T) {
	p, _ := PinFor("dsh")
	if !strings.HasPrefix(p.Integrity, "sha512-") {
		t.Fatalf("tools.json dsh pin has no integrity: %+v", p)
	}
	dir := t.TempDir()
	lock := func(version, integrity string) {
		os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte(`{"lockfileVersion":3,"packages":{"":{},
			"node_modules/@deepseek-ai/dsh":{"version":"`+version+`","integrity":"`+integrity+`"}}}`), 0o644)
	}
	if dshCheckIntegrity(dir, p) == nil {
		t.Fatal("no lock file must fail: nothing says what npm installed")
	}
	lock(p.Version, p.Integrity)
	if err := dshCheckIntegrity(dir, p); err != nil {
		t.Fatalf("matching lock: %v", err)
	}
	lock(p.Version, "sha512-other")
	if dshCheckIntegrity(dir, p) == nil {
		t.Fatal("a different integrity must fail")
	}
	lock("0.2.2-rc.1", p.Integrity)
	if dshCheckIntegrity(dir, p) == nil {
		t.Fatal("a different version must fail")
	}
	os.WriteFile(filepath.Join(dir, "package-lock.json"), []byte(`{"packages":{}}`), 0o644)
	if dshCheckIntegrity(dir, p) == nil {
		t.Fatal("a lock without the package must fail")
	}
}

func TestDshResolveCheckPreflight(t *testing.T) {
	dir := dshTestInstall(t, "0.2.1-rc.5")
	node := dshTestNode(t, dir, "22.22.3")
	t.Setenv("EZLM_TOOL_PATH", dir)
	home := dshTestHome(t, map[string]string{"web": "web-spark.cordis.patch.yml", "headless": "headless-empty.cordis.patch.yml"})
	t.Setenv("DSH_HOME", home)
	ep := dshTestEP()
	tool := dshTool{}

	f, err := tool.Resolve(context.Background(), nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if n, s, err := dshRuntime(f); err != nil || n != node || filepath.Base(s) != "bin.js" {
		t.Fatalf("runtime %q %q %v", n, s, err)
	}
	if err := tool.Configure(ep, false); err != nil {
		t.Fatal(err)
	}
	w, err := NewCheckWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	toolCall, err := tool.Check(context.Background(), f, ep, w, nil)
	if err != nil || !toolCall {
		t.Fatalf("check: %v %v", toolCall, err)
	}

	// A user environment holding an old key wins over .env in dsh.
	t.Setenv("EZLM_API_KEY", "ezlm-STALEKEY00000000")
	if _, err := tool.Check(context.Background(), f, ep, w, nil); err == nil || !strings.Contains(err.Error(), "EZLM_API_KEY") {
		t.Fatalf("stale env key: %v", err)
	}
	os.Unsetenv("EZLM_API_KEY")

	// Preflight: the composed web profile must show spark and ezlm.
	dump := filepath.Join(t.TempDir(), "dump.yml")
	web, _ := os.ReadFile(dshPatchPath(home, "web"))
	os.WriteFile(dump, append([]byte("# == @deepseek-ai/dsh-base\n- id: llm\n"), web...), 0o644)
	t.Setenv("FAKE_DUMP", dump)
	if err := tool.Preflight(context.Background(), f, ep, nil); err != nil {
		t.Fatalf("preflight: %v", err)
	}
	// The same dump with spark gone (a home-level patch replacing the row).
	os.WriteFile(dump, dshTestFixture(t, "home-override.cordis.patch.yml"), 0o644)
	os.WriteFile(filepath.Join(home, "cordis.patch.yml"), dshTestFixture(t, "home-override.cordis.patch.yml"), 0o644)
	err = tool.Preflight(context.Background(), f, ep, nil)
	if err == nil || !strings.Contains(err.Error(), "replaces DeepSeek Harness's provider list") {
		t.Fatalf("home override: %v", err)
	}
	os.Remove(filepath.Join(home, "cordis.patch.yml"))
	// A skipped EZLM row is reported.
	os.WriteFile(dump, append([]byte("# == x\n"), web...), 0o644)
	t.Setenv("FAKE_DUMP_WARN", `dsh: [`+dshPatchPath(home, "web")+`] patch: entry "agent-default-model" not found`)
	if err := tool.Preflight(context.Background(), f, ep, nil); err == nil || !strings.Contains(err.Error(), "skipped") {
		t.Fatalf("warning: %v", err)
	}
}

func TestDshResolveNeedsNewerNode(t *testing.T) {
	dir := dshTestInstall(t, "0.2.1-rc.5")
	dshTestNode(t, dir, "20.11.0")
	t.Setenv("EZLM_TOOL_PATH", dir)
	_, err := dshTool{}.Resolve(context.Background(), nil, false)
	if err == nil || !strings.Contains(err.Error(), "Node.js 22.19.0 or newer") {
		t.Fatalf("err = %v", err)
	}
	t.Setenv("EZLM_TOOL_PATH", t.TempDir())
	if _, err := (dshTool{}).Resolve(context.Background(), nil, false); err != ErrNotInstalled {
		t.Fatalf("nothing installed: %v", err)
	}
}

func TestDshParseEvents(t *testing.T) {
	ev := dshParseEvents(string(dshTestFixture(t, "headless-run.jsonl")) + "dsh: reasoning:\nThinking.\n")
	if !ev.toolCall || !strings.Contains(ev.final, "EZLM-CHECK-ABCDEFGH") || ev.errMsg != "" {
		t.Fatalf("run: %+v", ev)
	}
	ev = dshParseEvents(string(dshTestFixture(t, "headless-401.jsonl")))
	if ev.toolCall || ev.final != "" || !strings.Contains(ev.errMsg, "AUTH: 401") {
		t.Fatalf("401: %+v", ev)
	}
	if e, ok := Explain("dsh", string(dshTestFixture(t, "headless-401.jsonl"))); !ok || !strings.Contains(e.Say, "old EZLM key") {
		t.Fatalf("401 explained as %+v", e)
	}
}

func TestDshErrorTexts(t *testing.T) {
	for out, want := range map[string]string{
		`dsh: MISSING_CREDENTIAL: llm-pi-ai: no credential for provider route "ezlm"`:      "can't find EZLM's key",
		`dsh: UNKNOWN_MODEL: pi-ai provider "ezlm" has no configured model "other"`:        "isn't serving",
		"dsh: TRANSPORT: Connection error.":                                                "isn't running",
		"500: Jinja Exception: Unexpected reasoning effort high":                           "reasoning level",
		"credentials-local: /x/.credentials.yaml is readable beyond its owner (mode 644)":  "chmod 600",
		"atomic-write: timed out waiting for the writer lock at /x/.credentials.yaml.lock": "Try again in a moment",
	} {
		e, ok := Explain("dsh", out)
		if !ok || !strings.Contains(e.Say, want) {
			t.Errorf("%q -> %+v", out, e)
		}
		if strings.Contains(e.Say, "—") {
			t.Errorf("em dash in %q", e.Say)
		}
	}
}

func TestDshLaunchSpec(t *testing.T) {
	dir := dshTestInstall(t, "0.2.1-rc.5")
	node := dshTestNode(t, dir, "22.22.3")
	t.Setenv("EZLM_TOOL_PATH", dir)
	t.Setenv("DSH_HOME", filepath.Join(t.TempDir(), "elsewhere")) // not dsh's default home
	f, err := dshTool{}.Resolve(context.Background(), nil, false)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := dshTool{}.LaunchSpec(f, dshTestEP(), ModeWeb)
	if err != nil {
		t.Fatal(err)
	}
	port := spec.Args[len(spec.Args)-1]
	if spec.Bin != node || len(spec.Args) != 4 || spec.Args[1] != "web" || spec.Args[2] != "--port" ||
		spec.URL != "http://127.0.0.1:"+port+"/" || spec.Title != "DeepSeek Harness (EZLM)" {
		t.Fatalf("spec %+v", spec)
	}
	env := strings.Join(spec.Env, " ")
	if !strings.Contains(env, "DSH_TELEMETRY_MODE=DISABLED") || !strings.Contains(env, "DSH_TELEMETRY_DISABLED=1") ||
		!strings.Contains(env, "DSH_HOME="+dshHome()) || strings.Contains(env, "ezlm-") {
		t.Fatalf("env %v", spec.Env)
	}

	// A dsh web EZLM opened for this home is reused: only its URL opens.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, "dsh web authentication required; reopen the URL printed by dsh web.")
	}))
	defer srv.Close()
	p, _ := strconv.Atoi(srv.URL[strings.LastIndex(srv.URL, ":")+1:])
	dshSaveWeb(dshWebRecord{Port: p, Home: dshHome()})
	spec, err = dshTool{}.LaunchSpec(f, dshTestEP(), "")
	if err != nil || spec.Bin != "" || spec.URL != fmt.Sprintf("http://127.0.0.1:%d/", p) {
		t.Fatalf("reuse: %+v %v", spec, err)
	}
	// Another home's dsh web isn't reused.
	dshSaveWeb(dshWebRecord{Port: p, Home: "/somewhere/else"})
	if spec, _ = (dshTool{}).LaunchSpec(f, dshTestEP(), ""); spec.Bin == "" {
		t.Fatal("reused another home's dsh web")
	}
	// Something else on a port is not a dsh web.
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "hello") }))
	defer other.Close()
	op, _ := strconv.Atoi(other.URL[strings.LastIndex(other.URL, ":")+1:])
	if dshWebAnswers(op) || !dshWebAnswers(p) {
		t.Fatal("dsh web fingerprint")
	}
	if l, err := net.Listen("tcp", "127.0.0.1:0"); err == nil {
		lp := l.Addr().(*net.TCPAddr).Port
		if dshPortFree(lp) {
			t.Fatal("a listening port reads as free")
		}
		l.Close()
	}
	if _, err := (dshTool{}).LaunchSpec(f, dshTestEP(), ModeTUI); err == nil {
		t.Fatal("dsh has no terminal mode yet")
	}
}

func TestDshDefaultOverlay(t *testing.T) {
	got := string(dshDefaultOverlay("qwen3.8-27b"))
	if got != "- id: agent-default-model\n  config:\n    provider: ezlm\n    model: qwen3.8-27b\n" {
		t.Fatalf("overlay:\n%s", got)
	}
}
