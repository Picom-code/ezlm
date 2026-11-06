package integrate

import (
	"bytes"
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/picom-code/ezlm/internal/fakellm"
)

// The real DeepSeek Harness against a scripted model, end to end through
// core's Setup and Open. It runs only when EZLM_DSH_REAL names the folder
// holding a dsh 0.2.1-rc.5 and a node it can run on (for example
// ~/.local/bin); everything else stays in this package's temp home.
// EZLM_DSH_SEED may name a DSH_HOME whose profile patch files are copied in
// (read only) instead of the fixtures.
//
//	EZLM_DSH_REAL=$HOME/.local/bin go test -run TestDshReal -v ./internal/integrate/

func TestDshReal(t *testing.T) {
	bin := os.Getenv("EZLM_DSH_REAL")
	if bin == "" {
		t.Skip("set EZLM_DSH_REAL to the folder of a real dsh 0.2.1-rc.5 and its node")
	}
	t.Setenv("EZLM_TOOL_PATH", bin)
	home := filepath.Join(t.TempDir(), ".dsh")
	t.Setenv("DSH_HOME", home)
	seed := map[string][]byte{
		"web":      dshTestFixture(t, "web-spark.cordis.patch.yml"),
		"headless": dshTestFixture(t, "headless-empty.cordis.patch.yml"),
		"desktop":  dshTestFixture(t, "desktop-spark.cordis.patch.yml"),
	}
	if s := os.Getenv("EZLM_DSH_SEED"); s != "" {
		for p := range seed {
			if b, err := os.ReadFile(dshPatchPath(s, p)); err == nil {
				seed[p] = b
			}
		}
	}
	for p, b := range seed {
		os.MkdirAll(dshProfileDir(home, p), 0o755)
		os.WriteFile(dshPatchPath(home, p), b, 0o600)
	}
	before := map[string]*dshDoc{}
	for p, b := range seed {
		before[p], _ = dshParse(b)
	}

	// The shared fake model calls dsh's read tool on the check file, echoes
	// the tool result, and turns down a reasoning_effort outside levels the
	// way a llama-server chat template does.
	key, levels := "ezlm-REALTEST0000000000", []string{"low", "medium", "xhigh"}
	llm := fakellm.New(fakellm.Options{Key: key, Alias: "fake-model", Ctx: 65536, Levels: levels, Thinking: true})
	srv := httptest.NewServer(llm)
	defer srv.Close()
	ep := Endpoint{BaseURL: srv.URL + "/v1", Key: key, ModelID: "fake-model", ModelName: "Fake", Ctx: 65536, ToolCalls: true,
		Reasoning: Reasoning{Levels: levels, Thinking: true}}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var lines []string
	if err := Setup(ctx, "dsh", ep, false, func(s string) { lines = append(lines, s) }); err != nil {
		t.Fatalf("setup: %v\n%s", err, strings.Join(LogTail("dsh", 80), "\n"))
	}
	c := LastCheck("dsh")
	if c == nil || !c.OK || !c.ToolCall {
		t.Fatalf("check %+v", c)
	}
	t.Logf("progress: %q; check: %s", lines, c.Msg)
	var results, efforts []string
	for _, q := range llm.Requests() {
		if q.Path != "/v1/chat/completions" {
			continue
		}
		efforts = append(efforts, q.ReasoningEffort)
		if q.ToolResult != "" {
			results = append(results, q.ToolResult)
		}
		if e := q.ReasoningEffort; e != "" && e != "none" && !slices.Contains(levels, e) {
			t.Fatalf("a request carried reasoning_effort %q", e)
		}
	}
	if len(results) == 0 || !strings.Contains(results[0], "EZLM-CHECK-") {
		t.Fatalf("the model never got the check file back: %q", results)
	}
	t.Logf("reasoning_effort per request: %q", efforts)

	// Every provider the user had is still in their profile, byte for byte,
	// and the composed web profile has both theirs and EZLM's.
	for p, d := range before {
		b, _ := os.ReadFile(dshPatchPath(home, p))
		a, _ := dshParse(b)
		bp, ap := dshProviders(d.root()), dshProviders(a.root())
		for _, n := range dshProviderNames(bp) {
			if !bytes.Equal(d.pairText(bp, dshPairIndex(bp, n)), a.pairText(ap, dshPairIndex(ap, n))) {
				t.Fatalf("%s: provider %s changed", p, n)
			}
		}
	}
	f, err := dshTool{}.Resolve(ctx, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	node, script, _ := dshRuntime(f)
	out, err := dshCommand(node, script, home, "--profile", "web", "--dump-config").Output()
	if err != nil {
		t.Fatal(err)
	}
	var doc dshDoc
	if d, err := dshParse(out); err == nil {
		doc = *d
	}
	names := dshProviderNames(dshProviders(doc.root()))
	if dshPairIndex(dshProviders(doc.root()), dshProvider) < 0 || (len(dshProviderNames(dshProviders(before["web"].root()))) > 0 && len(names) == 0) {
		t.Fatalf("composed web providers: ezlm missing or %v", names)
	}
	t.Logf("composed web profile providers: %v + ezlm", names)

	// Open writes the launcher (dry run) for dsh web.
	l, err := Open(ctx, "dsh", "", ep, nil)
	if err != nil || !l.DryRun {
		t.Fatalf("open %+v %v", l, err)
	}
	if _, err := os.Stat(filepath.Join(home, "cordis.patch.yml")); !os.IsNotExist(err) {
		t.Fatal("wrote the home-level patch")
	}
}
