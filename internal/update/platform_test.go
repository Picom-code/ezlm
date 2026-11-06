package update

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// --- test artifacts: just enough of each format for a content check ---

func peFile(machine uint16) []byte {
	b := make([]byte, 0x100)
	copy(b, "MZ")
	binary.LittleEndian.PutUint32(b[0x3c:], 0x40) // e_lfanew
	copy(b[0x40:], "PE\x00\x00")
	binary.LittleEndian.PutUint16(b[0x44:], machine) // COFF Machine
	return b
}

// dmgFile is a UDIF image: the last 512 bytes start with the "koly" trailer.
func dmgFile() []byte {
	b := make([]byte, 1024)
	copy(b[len(b)-512:], "koly")
	return b
}

func elfFile() []byte {
	return append([]byte{0x7f, 'E', 'L', 'F', 2, 1, 1, 0}, make([]byte, 56)...)
}

func machoFile() []byte {
	b := make([]byte, 32)
	binary.LittleEndian.PutUint32(b, 0xfeedfacf)
	return b
}

func zipFile() []byte {
	return append([]byte("PK\x03\x04"), make([]byte, 60)...)
}

// fakeGitHub_platform serves one release whose only asset has this platform's name but
// the bytes the caller supplies. The digest in the metadata matches the bytes,
// so this is a release published wrong, not a corrupted download.
func fakeGitHub_platform(t *testing.T, assetName string, body []byte) *httptest.Server {
	t.Helper()
	sum := sha256.Sum256(body)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases":
			json.NewEncoder(w).Encode([]map[string]any{{
				"tag_name": "v0.2.0",
				"name":     "EZLM 0.2.0",
				"html_url": "https://example.test/releases/v0.2.0",
				"assets": []map[string]any{{
					"name":                 assetName,
					"browser_download_url": srv.URL + "/asset",
					"size":                 len(body),
					"digest":               "sha256:" + hex.EncodeToString(sum[:]),
				}},
			}})
		case "/asset":
			w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// thisPlatformAsset is the name a release must use for the machine running
// the test, and a body that really is that platform's artifact.
func thisPlatformAsset(t *testing.T) (name string, good []byte) {
	t.Helper()
	switch runtime.GOOS {
	case "windows":
		// The real installer is one 32-bit PE for x64 and ARM64 alike.
		return "EZLM-Setup-0.2.0.exe", peFile(0x014c)
	case "darwin":
		return "EZLM-0.2.0.dmg", dmgFile()
	}
	t.Skip("the updater only picks assets for windows and macOS")
	return "", nil
}

func downloadRelease(t *testing.T, assetName string, body []byte) (string, error) {
	t.Helper()
	srv := fakeGitHub_platform(t, assetName, body)
	t.Setenv("EZLM_UPDATE_URL", srv.URL+"/releases")
	tmp := t.TempDir() // keep the download out of the real temp dir
	t.Setenv("TMPDIR", tmp)
	t.Setenv("TMP", tmp)
	t.Setenv("TEMP", tmp)
	t.Cleanup(func() { os.RemoveAll(filepath.Join(tmp, "EZLM-update")) })
	info := Check(context.Background(), "0.1.0")
	if !info.Available || info.Latest == nil {
		t.Fatalf("check did not offer the release: %+v", info)
	}
	if info.Latest.Asset != assetName {
		t.Fatalf("chose asset %q, want %q", info.Latest.Asset, assetName)
	}
	return Download(context.Background(), info.Latest, nil)
}

// A release can publish another platform's bytes under the right name, with a
// digest that matches them. Nothing in the name or the digest catches that;
// the updater must look at the contents before the app runs or mounts them.
func TestDownloadRejectsWrongPlatform(t *testing.T) {
	name, _ := thisPlatformAsset(t)
	var wrong []byte
	if runtime.GOOS == "windows" {
		wrong = elfFile()
	} else {
		wrong = peFile(0x8664)
	}
	path, err := downloadRelease(t, name, wrong)
	if err == nil {
		t.Fatalf("Download installed %q for the wrong platform (path %q)", name, path)
	}
	if path != "" {
		t.Errorf("Download returned path %q with an error", path)
	}
	if !strings.Contains(err.Error(), "platform") && !strings.Contains(err.Error(), "Windows") && !strings.Contains(err.Error(), "macOS") {
		t.Errorf("error does not say the file is for another platform: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(os.TempDir(), "EZLM-update", name)); statErr == nil {
		t.Errorf("wrong-platform file %q was left on disk", name)
	}
}

func TestDownloadAcceptsThisPlatform(t *testing.T) {
	name, good := thisPlatformAsset(t)
	path, err := downloadRelease(t, name, good)
	if err != nil {
		t.Fatalf("Download rejected a good %s: %v", name, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("Download returned %q but the file is missing: %v", path, err)
	}
}

// verifyPlatform is the check itself: every shape of "right name, wrong
// bytes" is rejected, and a real artifact for the platform passes.
func TestVerifyPlatform(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, b []byte) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	mzOnly := make([]byte, 0x80)
	copy(mzOnly, "MZ")
	binary.LittleEndian.PutUint32(mzOnly[0x3c:], 0x40) // no PE signature at 0x40

	for _, c := range []struct {
		name   string
		path   string
		goos   string
		goarch string
		want   string // substring of the error; empty means accepted
	}{
		{"windows amd64 installer", write("amd64.exe", peFile(0x8664)), "windows", "amd64", ""},
		{"windows arm64 installer", write("arm64.exe", peFile(0xaa64)), "windows", "arm64", ""},
		{"arm64 installer on amd64", write("arm-on-amd64.exe", peFile(0xaa64)), "windows", "amd64", "for arm64"},
		{"amd64 installer on arm64", write("amd64-on-arm64.exe", peFile(0x8664)), "windows", "arm64", ""},
		{"32-bit installer on arm64", write("setup-arm64.exe", peFile(0x014c)), "windows", "arm64", ""},
		{"32-bit installer on amd64", write("setup-amd64.exe", peFile(0x014c)), "windows", "amd64", ""},
		{"linux program named .exe", write("linux.exe", elfFile()), "windows", "amd64", "Linux program"},
		{"macOS program named .exe", write("macos.exe", machoFile()), "windows", "amd64", "macOS program"},
		{"disk image named .exe", write("image.exe", dmgFile()), "windows", "amd64", "macOS disk image"},
		{"zip named .exe", write("zip.exe", zipFile()), "windows", "amd64", "zip archive"},
		{"MZ without a PE header", write("dos.exe", mzOnly), "windows", "amd64", "DOS program"},
		{"windows program named .dmg", write("windows.dmg", peFile(0x8664)), "darwin", "arm64", "Windows program"},
		{"linux program named .dmg", write("linux.dmg", elfFile()), "darwin", "arm64", "Linux program"},
		{"zip named .dmg", write("zip.dmg", zipFile()), "darwin", "arm64", "zip archive"},
		{"disk image", write("good.dmg", dmgFile()), "darwin", "arm64", ""},
		{"empty file named .dmg", write("empty.dmg", nil), "darwin", "arm64", "unrecognized"},
		{"disk image with no trailer", write("trailerless.dmg", make([]byte, 511)), "darwin", "arm64", "unrecognized"},
		{"windows installer named .zip", write("installer.zip", peFile(0x8664)), "windows", "amd64", "not a Windows .exe"},
		{"linux host is not checked", write("anything", elfFile()), "linux", "amd64", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := verifyPlatform(c.path, c.goos, c.goarch)
			if c.want == "" {
				if err != nil {
					t.Fatalf("verifyPlatform(%s) = %v, want nil", filepath.Base(c.path), err)
				}
				return
			}
			if err == nil {
				t.Fatalf("verifyPlatform(%s) = nil, want an error containing %q", filepath.Base(c.path), c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q does not contain %q", err, c.want)
			}
		})
	}
}
