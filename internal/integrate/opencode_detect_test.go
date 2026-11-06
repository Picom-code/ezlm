package integrate

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestOpenCodeDecisionTable(t *testing.T) {
	for _, tc := range []struct {
		name, version, arch, data, machine, goos string
		pinned                                   bool
		note                                     string
	}{
		{"2.2.5 native", "2.2.5", "arm64", openCodeDataV1, "arm64", "darwin", false, ""},
		{"2.1.0 native, v2 data", "2.1.0", "amd64", openCodeDataV2, "amd64", "windows", false, ""},
		{"2.0.3 is too old", "2.0.3", "amd64", openCodeDataNone, "amd64", "windows", true, "older than EZLM supports"},
		{"2.x x64 on Arm", "2.2.5", "amd64", openCodeDataNone, "arm64", "windows", true, "x64 build"},
		{"1.19.5 with v1 data", "1.19.5", "amd64", openCodeDataV1, "amd64", "windows", false, ""},
		{"1.19.5 with no data", "1.19.5", "arm64", openCodeDataNone, "arm64", "darwin", false, ""},
		{"1.19.5 with converted data", "1.19.5", "arm64", openCodeDataBoth, "arm64", "darwin", false, ""},
		{"1.19.5 with v2 data (the laptop)", "1.19.5", "arm64", openCodeDataV2, "arm64", "windows", true, "OpenBot's"},
		{"1.19.3 on Windows Arm", "1.19.3", "arm64", openCodeDataV1, "arm64", "windows", true, "older than EZLM supports"},
		{"1.x x64 on Arm", "1.19.5", "amd64", openCodeDataV1, "arm64", "windows", true, "x64 build"},
		{"unknown arch counts as native", "1.19.5", "", openCodeDataV1, "arm64", "windows", false, ""},
		{"no version", "", "arm64", openCodeDataNone, "arm64", "darwin", true, "didn't start"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pinned, note := openCodeDecide(tc.version, tc.arch, tc.data, tc.machine, tc.goos)
			if pinned != tc.pinned || !strings.Contains(note, tc.note) {
				t.Fatalf("pinned=%v note=%q", pinned, note)
			}
			if pinned && (!strings.Contains(note, "2.2.5") || strings.ContainsAny(note, "—`")) {
				t.Fatalf("note %q must name the pinned version in plain words", note)
			}
		})
	}
}

func TestOpenCodeVersions(t *testing.T) {
	for out, want := range map[string]string{"1.19.5": "1.19.5", "opencode v2.2.5": "2.2.5", "opencode v2.2.5\n": "2.2.5", "garbage": ""} {
		if got := openCodeVersion(out); got != want {
			t.Errorf("%q: %q", out, got)
		}
	}
	if !openCodeAtLeast("1.19.10", "1.19.4") || openCodeAtLeast("1.9.40", "1.19.4") || !openCodeAtLeast("2.2.5", "2.2.5") || openCodeAtLeast("", "1.0.0") {
		t.Error("version comparison")
	}
}

// openCodeTestDB unpacks a gzipped database fixture into dir.
func openCodeTestDB(t *testing.T, fixture, dir, as string) {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "opencode", fixture))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(zr)
	os.MkdirAll(dir, 0o755)
	if err := os.WriteFile(filepath.Join(dir, as), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// The fixtures are databases OpenCode 1.19.5 and 2.2.5 created (rows
// removed): v1 by 1.19.5, v2 by 2.2.5, converted by 2.2.5 opening the v1
// one, and wal with session_v2 only in an uncheckpointed WAL.
func TestOpenCodeDBFormat(t *testing.T) {
	for _, tc := range []struct {
		files map[string]string
		want  string
	}{
		{nil, openCodeDataNone},
		{map[string]string{"opencode.db": "db-v1.sqlite.gz"}, openCodeDataV1},
		{map[string]string{"opencode.db": "db-v2.sqlite.gz"}, openCodeDataV2},
		{map[string]string{"opencode.db": "db-converted.sqlite.gz"}, openCodeDataBoth},
		{map[string]string{"opencode.db": "db-wal.sqlite.gz", "opencode.db-wal": "db-wal.sqlite-wal.gz"}, openCodeDataBoth},
	} {
		dir := t.TempDir()
		for as, fx := range tc.files {
			openCodeTestDB(t, fx, dir, as)
		}
		if got := openCodeDBFormat(dir); got != tc.want {
			t.Errorf("%v: %s, want %s", tc.files, got, tc.want)
		}
	}
}

func TestOpenCodeSchemaScan(t *testing.T) {
	noise := bytes.Repeat([]byte{0, 0xff, 'x'}, 3<<20)
	for _, tc := range []struct {
		text   string
		v1, v2 bool
	}{
		{"CREATE TABLE `session` (\n `id` text", true, false},
		{`CREATE TABLE "session_v2" (id text)`, false, true},
		{"CREATE TABLE session(id)", true, false},
		{"CREATE TABLE IF NOT EXISTS [session_v2] (id)", false, true},
		{"CREATE TABLE `session_message` (id)", false, false},
		{"CREATE TABLE `sessions` (id)", false, false},
	} {
		// Put the text across the scanner's 4 MiB chunk boundary.
		b := append(append(append([]byte{}, noise[:4<<20-10]...), tc.text...), noise[:100]...)
		p := filepath.Join(t.TempDir(), "x.db")
		os.WriteFile(p, b, 0o644)
		if v1, v2 := openCodeScanSchema(p); v1 != tc.v1 || v2 != tc.v2 {
			t.Errorf("%q: v1=%v v2=%v", tc.text, v1, v2)
		}
	}
}

func TestOpenCodeDBFormatCached(t *testing.T) {
	dir := t.TempDir()
	openCodeTestDB(t, "db-v2.sqlite.gz", dir, "opencode.db")
	if got := openCodeDBFormatCached(dir); got != openCodeDataV2 {
		t.Fatalf("first: %s", got)
	}
	// v2 stays v2 for the same file, without a rescan.
	if got := openCodeDBFormatCached(dir); got != openCodeDataV2 {
		t.Fatalf("cached: %s", got)
	}
	// A replaced database is scanned again.
	os.Remove(filepath.Join(dir, "opencode.db"))
	if got := openCodeDBFormatCached(dir); got != openCodeDataNone {
		t.Fatalf("removed: %s", got)
	}
}

func TestOpenCodeSource(t *testing.T) {
	home, _ := os.UserHomeDir()
	for p, want := range map[string]string{
		filepath.Join(home, ".opencode", "bin", "opencode"):                                         SourceScript,
		filepath.Join(home, ".local", "lib", "node_modules", "@opencode", "cli", "bin", "opencode"): SourceNPM,
		filepath.Join(home, "AppData", "Roaming", "npm", "opencode.cmd"):                            SourceNPM,
		filepath.Join(ToolDir("opencode", "2.2.5"), "opencode"):                                     SourceEZLM,
		"/usr/local/bin/opencode":                                                                   SourcePath,
	} {
		if got := openCodeSource(p); got != want {
			t.Errorf("%s: %s, want %s", p, got, want)
		}
	}
}

func TestOpenCodeShimTarget(t *testing.T) {
	dir := t.TempDir()
	shim := filepath.Join(dir, "opencode.cmd")
	os.WriteFile(shim, []byte("@echo off"), 0o644)
	if openCodeShimTarget(shim) != "" {
		t.Fatal("found a target that isn't there")
	}
	exe := filepath.Join(dir, "node_modules", "opencode-ai", "node_modules", "opencode-windows-x64", "bin", "opencode.exe")
	os.MkdirAll(filepath.Dir(exe), 0o755)
	os.WriteFile(exe, []byte("MZ"), 0o644)
	if got := openCodeShimTarget(shim); got != exe {
		t.Fatalf("target = %q", got)
	}
}

func openCodeTestUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake opencode is a shell script")
	}
}

// The laptop case: the user's opencode is 1.19.5 and the chat history was
// written by OpenCode 2.x, so EZLM picks its pinned copy and, without
// install, reports it isn't installed yet.
func TestOpenCodeResolvePicksPinnedForV2History(t *testing.T) {
	openCodeTestUnix(t)
	bin := openCodeTestFakeBin(t, "1.19.5", false)
	t.Setenv("EZLM_TOOL_PATH", filepath.Dir(bin))
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	openCodeTestDB(t, "db-v2.sqlite.gz", filepath.Join(data, "opencode"), "opencode.db")

	_, err := (openCodeTool{}).Resolve(context.Background(), nil, false)
	if !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("err = %v", err)
	}
	d := (openCodeTool{}).Detect(nil)
	if d.Bin != "" || !strings.Contains(d.Detail, "OpenBot's") {
		t.Fatalf("detect: %+v", d)
	}
	// With v1 history the user's 1.19.5 is used, with 1.x flags.
	os.Remove(filepath.Join(data, "opencode", "opencode.db"))
	f, err := (openCodeTool{}).Resolve(context.Background(), nil, false)
	if err != nil || f.Bin != bin || f.Version != "1.19.5" {
		t.Fatalf("v1: %+v %v", f, err)
	}
	if fl := openCodeFlagsOf(f); fl.standalone || !fl.tuiModel || !fl.variant {
		t.Fatalf("1.x flags = %+v", fl)
	}
}

// openCodeTestRelease serves a release archive holding a fake opencode and
// points the pin at it.
func openCodeTestRelease(t *testing.T, version string) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	h := &zip.FileHeader{Name: openCodeExe(), Method: zip.Deflate}
	h.SetMode(0o755)
	w, _ := zw.CreateHeader(h)
	w.Write([]byte("#!/bin/sh\ncase \"$1\" in --version) echo \"opencode v" + version + "\";; run) echo '  --standalone';; esac\n"))
	zw.Close()
	body := buf.Bytes()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	t.Cleanup(srv.Close)
	sum := sha256.Sum256(body)
	old, _ := PinFor("opencode")
	p := old
	p.Assets = map[string]Asset{openCodeAssetKey(): {URL: srv.URL + "/opencode.zip", SHA256: hex.EncodeToString(sum[:]), Size: uint64(len(body))}}
	RegisterPin(p)
	t.Cleanup(func() { RegisterPin(old) })
}

func TestOpenCodeInstallPinned(t *testing.T) {
	openCodeTestUnix(t)
	t.Setenv("EZLM_TOOL_PATH", t.TempDir()) // no opencode of the user's
	openCodeTestRelease(t, "2.2.5")
	t.Cleanup(func() { os.RemoveAll(ToolDir("opencode", "2.2.5")) })
	var steps []string
	r := NewRun("opencode-test", func(s string) { steps = append(steps, s) })
	defer r.Close()
	f, err := (openCodeTool{}).Resolve(context.Background(), r, true)
	if err != nil {
		t.Fatal(err)
	}
	if f.Bin != openCodePinnedBin() || f.Source != SourceEZLM || f.Version != "2.2.5" || !strings.Contains(strings.Join(f.Env, " "), "OPENCODE_DISABLE_AUTOUPDATE=1") {
		t.Fatalf("found %+v", f)
	}
	if !strings.Contains(strings.Join(steps, "\n"), "Downloading OpenCode 2.2.5") {
		t.Fatalf("steps = %q", steps)
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(ToolDir("opencode", "2.2.5")), ".*")); len(left) != 0 {
		t.Fatalf("staging left behind: %v", left)
	}
	// Installed: Detect sees it, a second Resolve reuses it.
	if d := (openCodeTool{}).Detect(nil); d.Bin != f.Bin || d.Source != SourceEZLM {
		t.Fatalf("detect: %+v", d)
	}
	if g, err := (openCodeTool{}).Resolve(context.Background(), nil, false); err != nil || g.Bin != f.Bin {
		t.Fatalf("again: %+v %v", g, err)
	}
	if fl := openCodeFlagsOf(f); !fl.standalone || fl.tuiModel {
		t.Fatalf("2.x flags = %+v", fl)
	}
}

func TestOpenCodeInstallRefusesWrongBuild(t *testing.T) {
	openCodeTestUnix(t)
	t.Setenv("EZLM_TOOL_PATH", t.TempDir())
	openCodeTestRelease(t, "2.2.4") // the archive's program isn't the pinned version
	t.Cleanup(func() { os.RemoveAll(ToolDir("opencode", "2.2.5")) })
	if _, err := (openCodeTool{}).Resolve(context.Background(), nil, true); err == nil || !strings.Contains(err.Error(), "didn't start") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(openCodePinnedBin()); err == nil {
		t.Fatal("a wrong build was left installed")
	}
}

func TestOpenCodeInstallChecksum(t *testing.T) {
	t.Setenv("EZLM_TOOL_PATH", t.TempDir())
	openCodeTestRelease(t, "2.2.5")
	p, _ := PinFor("opencode")
	a := p.Assets[openCodeAssetKey()]
	a.SHA256 = strings.Repeat("0", 64)
	p.Assets[openCodeAssetKey()] = a
	RegisterPin(p)
	_, err := (openCodeTool{}).Resolve(context.Background(), nil, true)
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(openCodePinnedBin()); err == nil {
		t.Fatal("installed despite a checksum mismatch")
	}
}

func TestOpenCodeAssetKeys(t *testing.T) {
	p, _ := PinFor("opencode")
	for _, k := range []string{"windows-arm64", "windows-x64", "windows-x64-baseline", "darwin-arm64", "darwin-x64", "darwin-x64-baseline", "linux-arm64", "linux-x64"} {
		a, ok := p.Assets[k]
		if !ok || len(a.SHA256) != 64 || !strings.Contains(a.URL, "/v2.2.5/") || strings.Contains(a.URL, "latest") || a.Size == 0 {
			t.Errorf("%s: %+v", k, a)
		}
	}
	if k := openCodeAssetKey(); !strings.HasPrefix(k, runtime.GOOS+"-") {
		t.Errorf("asset key %s", k)
	}
}
