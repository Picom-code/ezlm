package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/picom-code/ezlm/internal/engine"
	"github.com/picom-code/ezlm/internal/scan"
	"github.com/picom-code/ezlm/internal/serve"
)

// A missing llama-server is downloaded again and the service re-registered
// with its new path; if that fails, the message says what to do.
func TestRestoreMissingServer(t *testing.T) {
	oldEngine, oldService := installEngine, installService
	defer func() { installEngine, installService = oldEngine, oldService }()

	dir := t.TempDir()
	fresh := filepath.Join(dir, "llama-b1-new", "llama-server")
	os.MkdirAll(filepath.Dir(fresh), 0o755)
	os.WriteFile(fresh, []byte("x"), 0o755)
	var registered string
	installEngine = func(context.Context, *scan.Hardware, engine.Progress) (*engine.Installed, error) {
		return &engine.Installed{Server: fresh, Tag: "b1"}, nil
	}
	installService = func(c *serve.Config) error { registered = c.Server; return nil }

	a := New()
	cfg := &serve.Config{ModelID: "m", Server: filepath.Join(dir, "gone", "llama-server"), Port: 18999}
	if err := a.restoreServer(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Server != fresh || cfg.EngineTag != "b1" || registered != fresh {
		t.Fatalf("server %q tag %q registered %q", cfg.Server, cfg.EngineTag, registered)
	}
	if saved, _ := serve.Load(); saved == nil || saved.Server != fresh {
		t.Fatalf("the new path wasn't saved: %+v", saved)
	}

	installEngine = func(context.Context, *scan.Hardware, engine.Progress) (*engine.Installed, error) {
		return nil, errors.New("offline")
	}
	cfg.Server = filepath.Join(dir, "gone", "llama-server")
	err := a.restoreServer(context.Background(), cfg)
	if err == nil || !strings.Contains(err.Error(), "antivirus") || !strings.Contains(err.Error(), "offline") {
		t.Fatalf("got %v", err)
	}
}
