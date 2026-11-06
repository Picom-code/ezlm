package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/picom-code/ezlm/internal/paths"
	"github.com/picom-code/ezlm/internal/serve"
)

// Switching back to a model that's already downloaded must never delete it to
// "make room" for itself.
func TestPruneSparesTarget(t *testing.T) {
	dir := t.TempDir()
	if home := paths.Home(); !strings.Contains(home, "ezlm-test-home-") {
		t.Fatalf("refusing to run against a real EZLM home: %s", home)
	}
	paths.SetModels(dir)
	defer paths.SetModels("")
	write := func(rel string) string {
		p := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, make([]byte, 10), 0o644)
		return p
	}
	cur := write("small/small-Q4.gguf")
	curDraft := write("small/mtp-small.gguf")
	big1 := write("big/big-IQ2-00001-of-00002.gguf")
	big2 := write("big/big-IQ2-00002-of-00002.gguf")
	bigDraft := write("big/mtp-big.gguf")
	stale := write("old/old-Q4.gguf")

	otherCopy := write("elsewhere/big/big-IQ2-00001-of-00002.gguf") // the same model in another folder
	paths.RememberModelsDir(filepath.Join(dir, "elsewhere"))
	freed := pruneUnused(&serve.Config{ModelPath: cur, DraftPath: curDraft}, 1<<40, "big", big1, bigDraft)
	if freed != 10 {
		t.Fatalf("freed %d bytes, want only the stale model (10)", freed)
	}
	for _, p := range []string{cur, curDraft, big1, big2, bigDraft, otherCopy} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s was deleted", filepath.Base(p))
		}
	}
	if _, err := os.Stat(stale); err == nil {
		t.Errorf("stale model survived")
	}
}

func TestIsEZLMDir(t *testing.T) {
	for dir, want := range map[string]bool{
		"":                  false,
		"/":                 false,
		`D:\EZLM\models`:    true,
		"/home/a/.ezlm":     true,
		"/home/a/Documents": false,
		`D:\`:               false,
	} {
		if got := isEZLMDir(dir); got != want {
			t.Errorf("isEZLMDir(%q) = %v, want %v", dir, got, want)
		}
	}
}

// Reconnecting clients get the running job's events, never a finished one.
func TestFinishedJobsArentReplayed(t *testing.T) {
	var b bus
	b.emit(Event{Job: "deploy", Step: "model", State: "run"})
	ch, stop := b.Subscribe()
	if len(ch) != 1 {
		t.Fatalf("running job: replayed %d events, want 1", len(ch))
	}
	stop()
	b.emit(Event{Job: "deploy", Step: "done", State: "ok"})
	ch, stop = b.Subscribe()
	defer stop()
	if len(ch) != 0 {
		t.Fatalf("finished job: replayed %d events, want 0", len(ch))
	}
}
