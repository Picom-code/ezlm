package integrate

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/picom-code/ezlm/internal/engine"
	"github.com/picom-code/ezlm/internal/paths"
)

// Pinned versions of what EZLM installs. tools.json (embedded) holds the
// pins this build was tested with; a tool's init() may RegisterPin its own;
// and a "tools" section in the remote catalog (UseCatalogPins) overrides
// both by tool name, so a tested version bump needs no app release.

// Pin is one tool version EZLM installs, with its downloads.
type Pin struct {
	Tool      string           `json:"tool"`                // "opencode" | "node" | "dsh"
	Version   string           `json:"version"`             // "2.2.5"
	Package   string           `json:"package,omitempty"`   // npm package, for npm-installed tools ("@deepseek-ai/dsh")
	Integrity string           `json:"integrity,omitempty"` // npm dist.integrity of Package@Version ("sha512-…")
	Tested    []string         `json:"tested,omitempty"`    // other versions EZLM reuses when it finds them
	Assets    map[string]Asset `json:"assets,omitempty"`    // by AssetKey
}

// Asset is one download, checked against its SHA-256 before use.
type Asset struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"` // hex
	Size   uint64 `json:"size,omitempty"`
}

//go:embed tools.json
var toolsJSON []byte

var (
	pinMu       sync.RWMutex
	pins        = map[string]Pin{} // RegisterPin
	embedded    = mustPins(toolsJSON)
	catalogPins = map[string]Pin{} // UseCatalogPins
)

// RegisterPin sets a tool's pin, replacing the embedded one.
func RegisterPin(p Pin) {
	pinMu.Lock()
	defer pinMu.Unlock()
	pins[p.Tool] = p
}

// PinFor is the version EZLM installs for tool: the remote catalog's pin,
// else the registered one, else the one built in.
func PinFor(tool string) (Pin, bool) {
	pinMu.RLock()
	defer pinMu.RUnlock()
	for _, m := range []map[string]Pin{catalogPins, pins, embedded} {
		if p, ok := m[tool]; ok {
			return p, true
		}
	}
	return Pin{}, false
}

// UseCatalogPins applies the remote catalog's "tools" section (a JSON list
// of pins; empty or null clears the overrides). A pin without a tool or
// version is ignored, and so is any asset without a SHA-256, so a bad
// catalog can never make EZLM install something unchecked.
func UseCatalogPins(raw []byte) error {
	next := map[string]Pin{}
	if len(bytes.TrimSpace(raw)) > 0 && string(bytes.TrimSpace(raw)) != "null" {
		var list []Pin
		if err := json.Unmarshal(raw, &list); err != nil {
			return fmt.Errorf("the catalog's tools section is invalid: %w", err)
		}
		for _, p := range validPins(list) {
			next[p.Tool] = p
		}
	}
	pinMu.Lock()
	catalogPins = next
	pinMu.Unlock()
	return nil
}

func mustPins(b []byte) map[string]Pin {
	var f struct {
		Tools []Pin `json:"tools"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		panic("integrate: tools.json is invalid: " + err.Error())
	}
	m := map[string]Pin{}
	for _, p := range validPins(f.Tools) {
		m[p.Tool] = p
	}
	return m
}

func validPins(list []Pin) []Pin {
	var out []Pin
	for _, p := range list {
		if p.Tool == "" || p.Version == "" {
			continue
		}
		assets := map[string]Asset{}
		for k, a := range p.Assets {
			if _, err := hex.DecodeString(a.SHA256); err == nil && len(a.SHA256) == 64 && strings.HasPrefix(a.URL, "https://") {
				assets[k] = a
			}
		}
		if len(assets) == 0 {
			assets = nil
		}
		p.Assets = assets
		out = append(out, p)
	}
	return out
}

// AssetKey names a download for an OS and architecture the way the pins
// do: "<goos>-<x64|arm64>", plus "-baseline" for CPUs without AVX2.
func AssetKey(goos, goarch string, baseline bool) string {
	arch := map[string]string{"amd64": "x64"}[goarch]
	if arch == "" {
		arch = goarch
	}
	k := goos + "-" + arch
	if baseline {
		k += "-baseline"
	}
	return k
}

// ToolsDir is where EZLM keeps the programs it installs: ~/.ezlm/tools.
func ToolsDir() string { return filepath.Join(paths.Home(), "tools") }

// ToolDir is one installed version: ~/.ezlm/tools/<tool>/<version>.
func ToolDir(tool, version string) string { return filepath.Join(ToolsDir(), tool, version) }

// IsEZLMTool reports whether bin lives under ToolsDir (Source "ezlm").
func IsEZLMTool(bin string) bool {
	rel, err := filepath.Rel(ToolsDir(), bin)
	return err == nil && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)
}

// FetchAsset downloads a to dst and checks its SHA-256; a mismatch deletes
// the file. An asset without a digest is refused.
func FetchAsset(ctx context.Context, r *Run, a Asset, dst string) error {
	name := filepath.Base(a.URL)
	if len(a.SHA256) != 64 {
		return fmt.Errorf("EZLM has no checksum for %s, so it won't install it", name)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	r.Logf("download %s -> %s", a.URL, dst)
	last := -1
	prog := func(done, total uint64, _ float64) {
		if total == 0 {
			return
		}
		if pct := int(done * 100 / total); pct/10 > last/10 {
			last = pct
			r.Stream(fmt.Sprintf("Downloading %s: %d%%", name, pct))
		}
	}
	if err := engine.Download(ctx, a.URL, dst, a.Size, prog); err != nil {
		return fmt.Errorf("downloading %s: %w", name, err)
	}
	f, err := os.Open(dst)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, err = io.Copy(h, f)
	f.Close()
	if err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, a.SHA256) {
		os.Remove(dst)
		r.Logf("checksum mismatch for %s: got %s, want %s", name, got, a.SHA256)
		return fmt.Errorf("the download of %s didn't match its checksum, so EZLM deleted it. Try again later", name)
	}
	return nil
}
