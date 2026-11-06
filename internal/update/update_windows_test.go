package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// within reports whether p is base or under it.
func within(base, p string) bool {
	rel, err := filepath.Rel(base, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// The installer used to wait in %TEMP%\EZLM-update. A program started from a
// temp folder is the shape Defender, SmartScreen and Smart App Control
// distrust, and Disk Cleanup or Storage Sense can delete it between the
// download and the run. It must wait in EZLM's own folder instead.
func TestDownloadDirIsNotTemp(t *testing.T) {
	// Two folders, so the home is not itself inside the temp folder the
	// test points os.TempDir at (t.TempDir alone is under it on Windows).
	home, tmp := filepath.Join(t.TempDir(), "home"), filepath.Join(t.TempDir(), "tmp")
	t.Setenv("EZLM_HOME", home)
	// os.TempDir reads TMPDIR on macOS, TMP then TEMP on Windows.
	t.Setenv("TMPDIR", tmp)
	t.Setenv("TMP", tmp)
	t.Setenv("TEMP", tmp)

	dir := DownloadDir()
	if !within(home, dir) {
		t.Fatalf("DownloadDir() = %q, want a folder under EZLM_HOME %q", dir, home)
	}
	if within(os.TempDir(), dir) {
		t.Fatalf("DownloadDir() = %q, still inside the temp folder %q", dir, os.TempDir())
	}
}

// Download puts the installer where DownloadDir says, so the path the window
// runs is the one that was checked.
func TestDownloadWritesIntoDownloadDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("EZLM_HOME", home)

	body := []byte("MZ fake installer")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		w.Write(body)
	}))
	defer srv.Close()

	sum := sha256.Sum256(body)
	r := Release{Asset: "EZLM-Setup-9.9.9.exe", URL: srv.URL + "/setup.exe", Size: uint64(len(body)), Digest: "sha256:" + hex.EncodeToString(sum[:])}
	path, err := Download(context.Background(), &r, nil)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	want := filepath.Join(DownloadDir(), r.Asset)
	if path != want {
		t.Fatalf("Download wrote %q, want %q", path, want)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read downloaded installer: %v", err)
	}
	if string(got) != string(body) {
		t.Fatalf("downloaded %q, want %q", got, body)
	}
}
