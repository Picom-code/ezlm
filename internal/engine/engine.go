// Package engine installs llama.cpp's llama-server for this machine and
// downloads model weights.
package engine

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/picom-code/ezlm/internal/catalog"
	"github.com/picom-code/ezlm/internal/paths"
	"github.com/picom-code/ezlm/internal/prereq"
	"github.com/picom-code/ezlm/internal/scan"
)

// ReleasesAPI is llama.cpp's GitHub releases endpoint.
var ReleasesAPI = "https://api.github.com/repos/ggml-org/llama.cpp/releases"

type Asset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Size   uint64 `json:"size"`
	Digest string `json:"digest"` // "sha256:<hex>" when GitHub provides it
}

type Release struct {
	Tag    string  `json:"tag_name"`
	Assets []Asset `json:"assets"`
}

// Installed describes an unpacked llama-server.
type Installed struct {
	Server  string       `json:"server"`
	Tag     string       `json:"tag"`
	Backend scan.Backend `json:"backend"`
}

func getJSON(ctx context.Context, url string, v any) error {
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ezlm")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("could not reach GitHub for llama.cpp: %s", resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

// LatestRelease returns the build that llama.cpp's latest stable release
// pins. Stable releases (v0.x) carry no binaries, only nightly-tag.txt naming
// the bNNNNN build that does.
func LatestRelease(ctx context.Context) (*Release, error) {
	var stable Release
	if err := getJSON(ctx, ReleasesAPI+"/latest", &stable); err != nil {
		return nil, err
	}
	if hasBinaries(&stable) {
		return &stable, nil
	}
	for _, a := range stable.Assets {
		if a.Name != "nightly-tag.txt" {
			continue
		}
		req, _ := http.NewRequestWithContext(ctx, "GET", a.URL, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
		resp.Body.Close()
		tag := strings.TrimSpace(string(b))
		var r Release
		if err := getJSON(ctx, ReleasesAPI+"/tags/"+tag, &r); err != nil {
			return nil, err
		}
		return &r, nil
	}
	return nil, fmt.Errorf("llama.cpp release %s has no downloadable builds", stable.Tag)
}

func hasBinaries(r *Release) bool {
	for _, a := range r.Assets {
		// The CUDA runtime archives (cudart-llama-...) contain "-bin-" too,
		// but only a llama-...-bin-... archive holds llama-server.
		if strings.HasPrefix(a.Name, "llama-") && strings.Contains(a.Name, "-bin-") {
			return true
		}
	}
	return false
}

// assetPatterns lists, per platform and backend, substrings that identify the
// right archive, in order of preference. All parts of a pattern must match.
// CUDA 12 builds are preferred over 13: they run on older drivers too.
func assetPatterns(goos, arch string, b scan.Backend, cuda13 bool) [][]string {
	a := map[string]string{"amd64": "x64", "arm64": "arm64", "s390x": "s390x"}[arch]
	if a == "" {
		return nil // an architecture no release builds for: match nothing
	}
	// Blackwell GPU code only ships in the CUDA 13 builds. Only Linux and
	// Windows publish them; elsewhere (macOS, unknown systems) the prefix is
	// empty and would match any archive for the architecture, so fall through
	// to the per-OS rules below.
	if pre := map[string]string{"linux": "-bin-ubuntu-cuda-13", "windows": "-bin-win-cuda-13"}[goos]; b == scan.CUDA && cuda13 && pre != "" {
		vk := map[string]string{"linux": "-bin-ubuntu-vulkan-", "windows": "-bin-win-vulkan-"}[goos]
		return [][]string{{pre, a}, {vk + a}}
	}
	switch goos {
	case "darwin":
		return [][]string{{"-bin-macos-" + a}}
	case "linux":
		switch b {
		case scan.CUDA:
			return [][]string{{"-bin-ubuntu-cuda-12", a}, {"-bin-ubuntu-cuda-", a}, {"-bin-ubuntu-vulkan-" + a}, {"-bin-ubuntu-" + a}}
		case scan.ROCm, scan.Vulkan:
			// The ROCm archive expects a ROCm 10.0 runtime on the machine, and
			// there is no ROCm runtime archive to install next to it, so Vulkan
			// stays the safer pick.
			return [][]string{{"-bin-ubuntu-vulkan-" + a}, {"-bin-ubuntu-" + a}}
		default:
			return [][]string{{"-bin-ubuntu-" + a}}
		}
	case "windows":
		switch b {
		case scan.CUDA:
			// Only CUDA 12 on x64: a CUDA 13 build needs a 13.0 driver, which
			// a machine that got here does not have. Vulkan still runs. arm64
			// has no CUDA 12 build at all, only 13.4.
			if arch == "arm64" {
				return [][]string{{"-bin-win-cuda-13", a}, {"-bin-win-vulkan-" + a}, {"-bin-win-cpu-" + a}}
			}
			return [][]string{{"-bin-win-cuda-12", a}, {"-bin-win-vulkan-" + a}, {"-bin-win-cpu-" + a}}
		case scan.ROCm, scan.Vulkan:
			return [][]string{{"-bin-win-vulkan-" + a}, {"-bin-win-cpu-" + a}}
		default:
			return [][]string{{"-bin-win-cpu-" + a}}
		}
	}
	return nil
}

func matches(name string, parts []string) bool {
	if !(strings.HasSuffix(name, ".zip") || strings.HasSuffix(name, ".tar.gz")) {
		return false
	}
	for _, p := range parts {
		if !strings.Contains(name, p) {
			return false
		}
	}
	return true
}

// cudaFlavor extracts e.g. "-cuda-12.4-x64" from an asset name.
func cudaFlavor(name string) string {
	i := strings.Index(name, "-cuda-")
	if i < 0 {
		return ""
	}
	rest := strings.TrimSuffix(strings.TrimSuffix(name[i:], ".zip"), ".tar.gz")
	return rest
}

// PickAssets chooses the archive(s) for this machine. CUDA builds also need
// the matching CUDA runtime archive (cudart-...), published alongside.
func PickAssets(r *Release, goos, arch string, b scan.Backend, cuda13 bool) ([]Asset, error) {
	for _, pat := range assetPatterns(goos, arch, b, cuda13) {
		for _, a := range r.Assets {
			if !strings.HasPrefix(a.Name, "llama-") || !matches(a.Name, pat) {
				continue
			}
			out := []Asset{a}
			if flavor := cudaFlavor(a.Name); flavor != "" {
				osPart := map[string]string{"windows": "-bin-win-", "linux": "-bin-ubuntu-"}[goos]
				for _, c := range r.Assets {
					// Same CUDA version, arch AND operating system: Windows and
					// Linux runtimes share the "-cuda-13.4-x64" suffix.
					if strings.HasPrefix(c.Name, "cudart-") && strings.Contains(c.Name, osPart) &&
						strings.HasSuffix(strings.TrimSuffix(strings.TrimSuffix(c.Name, ".zip"), ".tar.gz"), flavor) {
						out = append(out, c)
						break
					}
				}
			}
			return out, nil
		}
	}
	return nil, fmt.Errorf("no llama.cpp build for %s/%s (%s) in release %s", goos, arch, b, r.Tag)
}

// Install downloads and unpacks llama-server, reusing an existing copy of the
// same release.
func Install(ctx context.Context, hw *scan.Hardware, prog Progress) (*Installed, error) {
	// Beta builds use the llama.cpp release they were tested with, so an
	// upstream change can't break anyone mid-beta (EZLM_LLAMA=latest opts in
	// to the newest build). This also never touches GitHub's rate-limited API.
	pr := pinnedRelease
	r := &pr
	if os.Getenv("EZLM_LLAMA") == "latest" {
		if lr, err := LatestRelease(ctx); err == nil {
			r = lr
		} else if inst := existing(); inst != nil {
			return inst, nil // offline: keep using what we have
		}
	}
	if runtime.GOOS == "darwin" {
		// Installs from before engine folders were named per build flavor
		// (macOS has only the Metal flavor, so the old folder is the same build).
		if srv := findServer(filepath.Join(paths.Bin(), "llama-"+r.Tag)); srv != "" {
			return &Installed{Server: srv, Tag: r.Tag, Backend: hw.Backend}, nil
		}
	}
	backend := hw.Backend
	if backend == scan.CUDA && !cudaOK(hw.CUDAMax, "12.4") {
		backend = scan.Vulkan // driver too old for any CUDA build
	}
	cuda13 := prereq.NeedsCUDA13(hw) || runtime.GOARCH == "arm64" // arm64 only has CUDA 13 builds
	if cuda13 && !cudaOK(hw.CUDAMax, "13.0") {
		backend = scan.Vulkan
	}
	assets, err := PickAssets(r, runtime.GOOS, runtime.GOARCH, backend, cuda13)
	if err != nil {
		return nil, err
	}
	// One folder per build flavor, so switching CUDA 12 / 13 / Vulkan never
	// reuses the wrong binaries.
	dir := filepath.Join(paths.Bin(), "llama-"+r.Tag+"-"+flavor(assets[0].Name, r.Tag))
	if srv := findServer(dir); srv != "" {
		return &Installed{Server: srv, Tag: r.Tag, Backend: hw.Backend}, nil
	}

	var total uint64
	for _, a := range assets {
		total += a.Size
	}
	var base uint64
	tmp := dir + ".tmp"
	os.RemoveAll(tmp)
	for _, a := range assets {
		arc := filepath.Join(paths.Bin(), a.Name)
		err := Download(ctx, a.URL, arc, a.Size, func(d, _ uint64, bps float64) {
			if prog != nil {
				prog(base+d, total, bps)
			}
		})
		if err != nil {
			return nil, err
		}
		if err := verifyDigest(arc, a.Digest); err != nil {
			os.Remove(arc)
			return nil, err
		}
		if err := extract(arc, tmp); err != nil {
			return nil, fmt.Errorf("unpacking %s: %w", a.Name, err)
		}
		os.Remove(arc)
		base += a.Size
	}
	if findServer(tmp) == "" {
		return nil, fmt.Errorf("llama-server not found in %s", assets[0].Name)
	}
	os.RemoveAll(dir)
	if err := os.Rename(tmp, dir); err != nil {
		return nil, err
	}
	return &Installed{Server: findServer(dir), Tag: r.Tag, Backend: hw.Backend}, nil
}

// flavor turns "llama-b11960-bin-win-cuda-13.4-x64.zip" into "win-cuda-13.4-x64".
func flavor(asset, tag string) string {
	f := strings.TrimPrefix(asset, "llama-"+tag+"-bin-")
	return strings.TrimSuffix(strings.TrimSuffix(f, ".zip"), ".tar.gz")
}

func cudaOK(have, need string) bool {
	if have == "" {
		return false
	}
	hv, _ := strconv.ParseFloat(have, 64)
	nv, _ := strconv.ParseFloat(need, 64)
	return hv >= nv
}

func existing() *Installed {
	entries, _ := os.ReadDir(paths.Bin())
	for i := len(entries) - 1; i >= 0; i-- {
		e := entries[i]
		if e.IsDir() && strings.HasPrefix(e.Name(), "llama-") && !strings.HasSuffix(e.Name(), ".tmp") {
			if srv := findServer(filepath.Join(paths.Bin(), e.Name())); srv != "" {
				return &Installed{Server: srv, Tag: strings.TrimPrefix(e.Name(), "llama-")}
			}
		}
	}
	return nil
}

func serverName() string {
	if runtime.GOOS == "windows" {
		return "llama-server.exe"
	}
	return "llama-server"
}

func findServer(dir string) string {
	var found string
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == serverName() {
			found = p
			return fs.SkipAll
		}
		return nil
	})
	return found
}

// VerifyDigest checks a file against a GitHub "sha256:<hex>" digest.
func VerifyDigest(path, digest string) error { return verifyDigest(path, digest) }

func verifyDigest(path, digest string) error {
	want, ok := strings.CutPrefix(digest, "sha256:")
	if !ok {
		return nil // GitHub did not publish one; size was already checked
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("checksum mismatch for %s", filepath.Base(path))
	}
	return nil
}

// safeJoin prevents archive entries from escaping dst.
func safeJoin(dst, name string) (string, error) {
	p := filepath.Join(dst, name)
	if p != filepath.Clean(dst) && !strings.HasPrefix(p, filepath.Clean(dst)+string(os.PathSeparator)) {
		return "", fmt.Errorf("unsafe path in archive: %s", name)
	}
	return p, nil
}

// Extract unpacks a .zip or .tar.gz archive into dst.
func Extract(arc, dst string) error { return extract(arc, dst) }

func extract(arc, dst string) error {
	if strings.HasSuffix(arc, ".zip") {
		return unzip(arc, dst)
	}
	return untar(arc, dst)
}

func unzip(arc, dst string) error {
	zr, err := zip.OpenReader(arc)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		p, err := safeJoin(dst, f.Name)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(p, 0o755)
			continue
		}
		os.MkdirAll(filepath.Dir(p), 0o755)
		rc, err := f.Open()
		if err != nil {
			return err
		}
		if f.Mode()&fs.ModeSymlink != 0 {
			target, _ := io.ReadAll(rc)
			rc.Close()
			os.Remove(p)
			if err := os.Symlink(string(target), p); err != nil {
				return err
			}
			continue
		}
		mode := f.Mode().Perm() | 0o600
		if strings.HasPrefix(filepath.Base(p), "llama-") || strings.HasSuffix(p, ".so") || strings.Contains(p, ".dylib") {
			mode |= 0o755
		}
		err = writeFile(p, rc, mode)
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func untar(arc, dst string) error {
	f, err := os.Open(arc)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		p, err := safeJoin(dst, h.Name)
		if err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(p, 0o755)
		case tar.TypeSymlink:
			os.MkdirAll(filepath.Dir(p), 0o755)
			os.Remove(p)
			if err := os.Symlink(h.Linkname, p); err != nil {
				return err
			}
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(p), 0o755)
			if err := writeFile(p, tr, fs.FileMode(h.Mode).Perm()|0o600); err != nil {
				return err
			}
		}
	}
}

func writeFile(p string, r io.Reader, mode fs.FileMode) error {
	out, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, r); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// ModelPath is where a quant's first file lives once downloaded.
func ModelPath(m catalog.Model, q catalog.Quant) string {
	return filepath.Join(paths.Models(), m.ID, filepath.Base(q.Files[0]))
}

// DownloadModel fetches every file of a quant (split GGUFs have several).
// llama.cpp loads the remaining parts automatically from the first.
func DownloadModel(ctx context.Context, m catalog.Model, q catalog.Quant, prog Progress) (string, error) {
	dir := filepath.Join(paths.Models(), m.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	// Per-file sizes aren't in the catalog, so only the single-file case is size-checked.
	var base uint64
	for _, file := range q.Files {
		var want uint64
		if len(q.Files) == 1 {
			want = q.Bytes
		}
		dst := filepath.Join(dir, filepath.Base(file))
		var last uint64
		err := Download(ctx, m.FileURL(q, file), dst, want, func(d, _ uint64, bps float64) {
			last = d
			if prog != nil {
				prog(base+d, q.Bytes, bps)
			}
		})
		if err != nil {
			return "", err
		}
		base += last
	}
	return ModelPath(m, q), nil
}

// DownloadDraft fetches a model's separate speculative-decoding draft file.
func DownloadDraft(ctx context.Context, m catalog.Model, prog Progress) (string, error) {
	d := m.Draft
	dst := filepath.Join(paths.Models(), m.ID, filepath.Base(d.File))
	return dst, Download(ctx, d.URL(), dst, d.Bytes, prog)
}

// Prewarm reads files sequentially so their pages are in the OS cache before
// the server memory-maps them. Experts kept in RAM are read at random on a
// cold start, which made the first prompts crawl.
func Prewarm(ctx context.Context, files []string, prog Progress) {
	var total, done uint64
	for _, f := range files {
		if st, err := os.Stat(f); err == nil {
			total += uint64(st.Size())
		}
	}
	buf := make([]byte, 64<<20)
	for _, f := range files {
		fh, err := os.Open(f)
		if err != nil {
			continue
		}
		for ctx.Err() == nil {
			n, err := fh.Read(buf)
			done += uint64(n)
			if prog != nil {
				prog(done, total, 0)
			}
			if err != nil {
				break
			}
		}
		fh.Close()
	}
}

// SplitFiles lists every part of a (possibly split) GGUF given its first part.
func SplitFiles(first string) []string {
	out := []string{first}
	if i := strings.LastIndex(first, "-00001-of-"); i > 0 {
		var n int
		fmt.Sscanf(first[i+len("-00001-of-"):], "%05d", &n)
		for k := 2; k <= n; k++ {
			out = append(out, fmt.Sprintf("%s-%05d-of-%05d.gguf", first[:i], k, n))
		}
	}
	return out
}
