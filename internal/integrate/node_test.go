package integrate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNodeVersions(t *testing.T) {
	for _, c := range []struct {
		have, min string
		ok        bool
	}{
		{"22.22.3", "22.19.0", true},
		{"v22.19.0", "22.19.0", true},
		{"22.18.9", "22.19.0", false},
		{"24.9.0", "22.19.0", true},
		{"20.20.0", "22.19.0", false},
		{"23.0.0-nightly", "22.19.0", true},
		{"garbage", "22.19.0", false},
	} {
		if got := nodeAtLeast(c.have, c.min); got != c.ok {
			t.Errorf("nodeAtLeast(%q, %q) = %v", c.have, c.min, got)
		}
	}
	p, ok := PinFor("node")
	if !ok || !strings.HasPrefix(p.Version, "24.") || !nodeAtLeast(p.Version, dshNodeMin) {
		t.Fatalf("node pin %+v", p)
	}
	for k, a := range p.Assets {
		if len(a.SHA256) != 64 || !strings.Contains(a.URL, "node-v"+p.Version+"-") {
			t.Errorf("asset %s: %+v", k, a)
		}
		if strings.HasPrefix(k, "windows") != strings.HasSuffix(a.URL, ".zip") {
			t.Errorf("asset %s: Windows gets a zip, others a tarball", k)
		}
	}
	for _, k := range []string{"windows-x64", "windows-arm64", "darwin-arm64", "darwin-x64", "linux-x64", "linux-arm64"} {
		if _, ok := p.Assets[k]; !ok {
			t.Errorf("no node asset for %s", k)
		}
	}
}

// nodeTestArchive builds a Node-shaped archive (top folder node-v24.0.0-x)
// plus one entry that tries to escape.
func nodeTestArchive(t *testing.T, kind string) []byte {
	t.Helper()
	files := map[string]string{
		"node-v24.0.0-x/bin/node":                            "#!/bin/sh\necho v24.0.0\n",
		"node-v24.0.0-x/node.exe":                            "MZ",
		"node-v24.0.0-x/lib/node_modules/npm/bin/npm-cli.js": "// npm\n",
		"node-v24.0.0-x/node_modules/npm/bin/npm-cli.js":     "// npm\n",
		"../escape.txt":                                      "no",
	}
	var buf bytes.Buffer
	switch kind {
	case "zip":
		zw := zip.NewWriter(&buf)
		for name, body := range files {
			h := &zip.FileHeader{Name: name, Method: zip.Deflate}
			h.SetMode(0o755)
			w, _ := zw.CreateHeader(h)
			w.Write([]byte(body))
		}
		zw.Close()
	case "tar.gz":
		gz := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gz)
		tw.WriteHeader(&tar.Header{Name: "node-v24.0.0-x/", Typeflag: tar.TypeDir, Mode: 0o755})
		for name, body := range files {
			tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o755, Size: int64(len(body))})
			tw.Write([]byte(body))
		}
		tw.WriteHeader(&tar.Header{Name: "node-v24.0.0-x/bin/npm", Typeflag: tar.TypeSymlink, Linkname: "../lib/node_modules/npm/bin/npm-cli.js"})
		tw.WriteHeader(&tar.Header{Name: "node-v24.0.0-x/bin/evil", Typeflag: tar.TypeSymlink, Linkname: "../../../../etc/passwd"})
		tw.Close()
		gz.Close()
	}
	return buf.Bytes()
}

func TestNodeExtract(t *testing.T) {
	for _, kind := range []string{"zip", "tar.gz"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			archive := filepath.Join(dir, "node."+kind)
			os.WriteFile(archive, nodeTestArchive(t, kind), 0o644)
			dst := filepath.Join(dir, "out")
			os.MkdirAll(dst, 0o755)
			if err := nodeExtract(archive, dst); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(dst, "bin", "node")); err != nil {
				t.Fatal("top folder not stripped")
			}
			if _, err := os.Stat(filepath.Join(dir, "escape.txt")); !os.IsNotExist(err) {
				t.Fatal("an entry escaped the folder")
			}
			if runtime.GOOS != "windows" {
				if st, _ := os.Stat(filepath.Join(dst, "bin", "node")); st.Mode().Perm()&0o100 == 0 {
					t.Fatal("lost the executable bit")
				}
			}
			if kind == "tar.gz" && runtime.GOOS != "windows" {
				if l, err := os.Readlink(filepath.Join(dst, "bin", "npm")); err != nil || l != "../lib/node_modules/npm/bin/npm-cli.js" {
					t.Fatalf("npm link %q %v", l, err)
				}
				if _, err := os.Lstat(filepath.Join(dst, "bin", "evil")); !os.IsNotExist(err) {
					t.Fatal("kept a link out of the folder")
				}
			}
		})
	}
}

func TestNodeNpmCLI(t *testing.T) {
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	unix := filepath.Join(dir, "unix")
	os.MkdirAll(filepath.Join(unix, "bin"), 0o755)
	os.MkdirAll(filepath.Join(unix, "lib", "node_modules", "npm", "bin"), 0o755)
	os.WriteFile(filepath.Join(unix, "bin", "node"), nil, 0o755)
	os.WriteFile(filepath.Join(unix, "lib", "node_modules", "npm", "bin", "npm-cli.js"), nil, 0o644)
	if got := nodeNpmCLI(filepath.Join(unix, "bin", "node")); got != filepath.Join(unix, "lib", "node_modules", "npm", "bin", "npm-cli.js") {
		t.Fatalf("unix layout: %q", got)
	}
	win := filepath.Join(dir, "win")
	os.MkdirAll(filepath.Join(win, "node_modules", "npm", "bin"), 0o755)
	os.WriteFile(filepath.Join(win, "node.exe"), nil, 0o755)
	os.WriteFile(filepath.Join(win, "node_modules", "npm", "bin", "npm-cli.js"), nil, 0o644)
	if got := nodeNpmCLI(filepath.Join(win, "node.exe")); got != filepath.Join(win, "node_modules", "npm", "bin", "npm-cli.js") {
		t.Fatalf("windows layout: %q", got)
	}
	if got := nodeNpmCLI(filepath.Join(dir, "nothing", "node")); got != "" {
		t.Fatalf("no npm: %q", got)
	}
}

func TestNodeInstallChecksPin(t *testing.T) {
	kind := "tar.gz"
	if runtime.GOOS == "windows" {
		kind = "zip"
	}
	body := nodeTestArchive(t, kind)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	defer srv.Close()
	old, _ := PinFor("node")
	defer RegisterPin(old)
	sum := sha256.Sum256(body)
	key := AssetKey(runtime.GOOS, MachineArch(), false)
	pin := Pin{Tool: "node", Version: "24.0.0", Assets: map[string]Asset{key: {URL: srv.URL + "/node-v24.0.0-x." + kind, SHA256: strings.Repeat("0", 64)}}}
	RegisterPin(pin)
	defer os.RemoveAll(filepath.Join(ToolsDir(), "node"))

	if _, err := nodeInstall(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("a wrong digest must fail: %v", err)
	}
	if _, err := os.Stat(ToolDir("node", "24.0.0")); !os.IsNotExist(err) {
		t.Fatal("a failed install left a folder")
	}
	pin.Assets[key] = Asset{URL: pin.Assets[key].URL, SHA256: hex.EncodeToString(sum[:])}
	RegisterPin(pin)
	bin, err := nodeInstall(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if bin != nodeBinIn(ToolDir("node", "24.0.0")) || nodeEZLM() != bin || !IsEZLMTool(bin) {
		t.Fatalf("bin %q", bin)
	}
	if left, _ := filepath.Glob(filepath.Join(ToolsDir(), "node", ".*")); len(left) != 0 {
		t.Fatalf("leftovers %v", left)
	}
}
