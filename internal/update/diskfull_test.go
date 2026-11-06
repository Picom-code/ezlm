package update

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// mountTinyDisk mounts a disk image with less free space than the update, so
// the write really fails with ENOSPC partway through. It returns the mount
// point and points TMPDIR at it, because Download stages files under
// os.TempDir().
func mountTinyDisk(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("needs hdiutil to make a small disk")
	}
	if _, err := exec.LookPath("hdiutil"); err != nil {
		t.Skip("hdiutil is not available")
	}
	base := t.TempDir() // on the real disk, before TMPDIR moves
	img := filepath.Join(base, "tiny.dmg")
	mnt := filepath.Join(base, "mnt")
	if err := os.Mkdir(mnt, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("hdiutil", "create", "-size", "1m", "-fs", "HFS+", "-volname", "EZLMTEST", "-quiet", img).CombinedOutput(); err != nil {
		t.Skipf("hdiutil create: %v: %s", err, out)
	}
	if out, err := exec.Command("hdiutil", "attach", "-nobrowse", "-noautoopen", "-quiet", "-mountpoint", mnt, img).CombinedOutput(); err != nil {
		t.Skipf("hdiutil attach: %v: %s", err, out)
	}
	t.Cleanup(func() {
		if out, err := exec.Command("hdiutil", "detach", "-force", "-quiet", mnt).CombinedOutput(); err != nil {
			t.Logf("hdiutil detach: %v: %s", err, out)
		}
	})
	t.Setenv("TMPDIR", mnt)
	// Since T15 a08 the updater downloads into EZLM_HOME/update, so point the
	// home at the full image too.
	t.Setenv("EZLM_HOME", mnt)
	return mnt
}

// TestDownloadDiskFull fills the staging disk in the middle of the download.
// A full disk cannot be fixed by retrying, so Download must fail at once with
// a plain message, leave the previous installer untouched and remove the
// half-written file.
func TestDownloadDiskFull(t *testing.T) {
	mnt := mountTinyDisk(t)

	body := bytes.Repeat([]byte("EZLM"), 512*1024) // 2 MiB, more than the disk holds
	// Since T08 a10 Download verifies the platform: give the .dmg its koly
	// trailer so the test reaches the disk-full path.
	body = append(body, make([]byte, 512)...)
	copy(body[len(body)-512:], "koly")
	sum := sha256.Sum256(body)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "EZLM.dmg", time.Time{}, bytes.NewReader(body))
	}))
	defer srv.Close()

	// The installer a previous update left behind. A failed download must not
	// replace it with a truncated file.
	dir := filepath.Join(mnt, "update")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(dir, "EZLM.dmg")
	const oldText = "OLD-INSTALL"
	if err := os.WriteFile(old, []byte(oldText), 0o644); err != nil {
		t.Fatal(err)
	}

	r := &Release{
		Version: "9.9.9",
		Asset:   "EZLM.dmg",
		URL:     srv.URL + "/EZLM.dmg",
		Size:    uint64(len(body)),
		Digest:  "sha256:" + hex.EncodeToString(sum[:]),
	}
	start := time.Now()
	got, err := Download(context.Background(), r, nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("Download succeeded on a full disk: %q", got)
	}
	if got != "" {
		t.Errorf("Download returned %q with an error", got)
	}
	// A full disk is not a network problem: retrying with the same free space
	// cannot help, so it must fail at once and say what to do.
	if elapsed > 5*time.Second {
		t.Errorf("Download spent %v retrying a full disk", elapsed)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "free up some space") && !strings.Contains(strings.ToLower(err.Error()), "free some space") {
		t.Errorf("error does not name the next action: %v", err)
	}
	// The previous installer is still there, byte for byte.
	b, rerr := os.ReadFile(old)
	if rerr != nil {
		t.Fatalf("the old installer is gone: %v", rerr)
	}
	if string(b) != oldText {
		t.Errorf("the old installer changed to %q", b)
	}
	// No half-written file is left for the next attempt to resume from.
	if _, err := os.Stat(old + ".part"); err == nil {
		t.Errorf("a partial download was left behind at %s.part", old)
	} else if !os.IsNotExist(err) {
		t.Errorf("stat partial file: %v", err)
	}
}
