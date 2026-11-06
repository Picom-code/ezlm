// Package update finds newer EZLM releases on GitHub and downloads them.
package update

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/picom-code/ezlm/internal/engine"
	"github.com/picom-code/ezlm/internal/paths"
)

// ReleasesURL lists EZLM's public releases, beta builds included
// (GitHub's /releases/latest skips pre-releases). EZLM_UPDATE_URL overrides
// it for testing.
var ReleasesURL = "https://api.github.com/repos/picom-code/ezlm/releases?per_page=10"

// Rate limits: GitHub answers 403 with X-RateLimit-Remaining: 0 for its
// primary limit, and 403 or 429 with Retry-After for its secondary one. The
// app asks again whenever the last check failed, so Check remembers when
// GitHub said to come back and skips the request until then.
var (
	rateMu    sync.Mutex
	rateUntil time.Time
)

const (
	defaultRateLimitWait = time.Minute
	maxRateLimitWait     = time.Hour
	// rateLimitRetryBudget is the longest Retry-After worth waiting for
	// inside one check; the check itself gives up after 10 seconds.
	rateLimitRetryBudget = 3 * time.Second
)

// Release is a newer version and the file to download for this computer.
type Release struct {
	Version    string `json:"version"`
	Name       string `json:"name"`
	Notes      string `json:"notes"`
	Page       string `json:"page"`
	Asset      string `json:"asset"`
	URL        string `json:"url"`
	Size       uint64 `json:"size"`
	Digest     string `json:"digest"`
	Prerelease bool   `json:"prerelease"` // GitHub marks beta builds with this
}

type Info struct {
	Current   string    `json:"current"`
	Available bool      `json:"available"`
	Latest    *Release  `json:"latest,omitempty"`
	Checked   time.Time `json:"checked"`
	Error     string    `json:"error,omitempty"`
}

// httpClient reaches GitHub for the update check and works out where the
// installer really comes from. GitHub redirects both, so one policy covers
// every hop: a short chain that never leaves https.
var httpClient = &http.Client{CheckRedirect: redirectPolicy}

var (
	errInsecureRedirect = errors.New("the update server redirected to an insecure address")
	errTooManyRedirects = errors.New("the update server redirected too many times")
)

// redirectPolicy bounds the redirect chain and keeps it on https. GitHub hands
// the release list and the installer to its CDN through one or two hops; a hop
// that drops to plain http can be rewritten by anything on the network.
func redirectPolicy(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errTooManyRedirects
	}
	if req.URL.Host == "" {
		return fmt.Errorf("the update server redirected to an address with no host")
	}
	if via[0].URL.Scheme == "https" && req.URL.Scheme != "https" {
		return fmt.Errorf("%w: %s://%s", errInsecureRedirect, req.URL.Scheme, req.URL.Host)
	}
	return nil
}

// assetProbeTimeout bounds the redirect probe so a server that never answers
// can't stall the download before it starts.
var assetProbeTimeout = 30 * time.Second

// resolveAsset follows the installer's redirect chain and returns the address
// the bytes actually come from, so a chain that loops or leaves https is
// refused before anything is written. A probe that fails for some other reason
// leaves the download to report the problem in its own words.
func resolveAsset(ctx context.Context, url string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, assetProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return url, nil
	}
	req.Header.Set("User-Agent", "ezlm")
	req.Header.Set("Range", "bytes=0-0")
	resp, err := httpClient.Do(req)
	if err != nil {
		if errors.Is(err, errInsecureRedirect) || errors.Is(err, errTooManyRedirects) {
			return "", err
		}
		return url, nil
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<10))
	if resp.Request == nil || resp.Request.URL == nil {
		return url, nil
	}
	return resp.Request.URL.String(), nil
}

// assetFor picks this platform's installer from a release's files.
func assetFor(goos, name string) bool {
	switch goos {
	case "windows":
		return strings.HasPrefix(name, "EZLM-Setup") && strings.HasSuffix(name, ".exe")
	case "darwin":
		return strings.HasPrefix(name, "EZLM") && strings.HasSuffix(name, ".dmg")
	}
	return false
}

// Check finds the newest release with a file for this platform.
func Check(ctx context.Context, current string) Info {
	info := Info{Current: current, Checked: time.Now()}
	url := ReleasesURL
	if u := os.Getenv("EZLM_UPDATE_URL"); u != "" {
		url = u
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := httpClient.Do(req)
	if err != nil {
		switch {
		case errors.Is(err, errInsecureRedirect):
			info.Error = "the update server answered over an insecure connection, so EZLM ignored it"
		case errors.Is(err, errTooManyRedirects):
			info.Error = "the update server sent EZLM in circles, so check for updates again later"
		default:
			info.Error = "couldn't reach GitHub to check for updates"
		}
		return info
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		info.Error = fmt.Sprintf("checking for updates failed (%s)", resp.Status)
		return info
	}
	var rels []struct {
		Tag        string `json:"tag_name"`
		Name       string `json:"name"`
		Body       string `json:"body"`
		Page       string `json:"html_url"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
		Assets     []struct {
			Name   string `json:"name"`
			URL    string `json:"browser_download_url"`
			Size   uint64 `json:"size"`
			Digest string `json:"digest"`
		} `json:"assets"`
	}
	for attempt := 0; ; attempt++ {
		if wait, ok := rateLimited(); ok {
			info.Error = rateLimitMessage(wait)
			return info
		}
		req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
		req.Header.Set("Accept", "application/vnd.github+json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			info.Error = "couldn't reach GitHub to check for updates"
			return info
		}
		if wait, ok := retryDelay(resp, time.Now()); ok {
			resp.Body.Close()
			if attempt == 0 && wait <= rateLimitRetryBudget {
				select {
				case <-ctx.Done():
					info.Error = "couldn't reach GitHub to check for updates"
					return info
				case <-time.After(wait):
				}
				continue
			}
			noteRateLimit(wait)
			info.Error = rateLimitMessage(wait)
			return info
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			info.Error = fmt.Sprintf("checking for updates failed (%s)", resp.Status)
			return info
		}
		err = json.NewDecoder(resp.Body).Decode(&rels)
		resp.Body.Close()
		if err != nil {
			info.Error = "couldn't read the release list"
			return info
		}
		break
	}
	// A release whose installer URL or digest is byte-identical to an OLDER
	// release's is a replayed old build: the version says "new" but the bytes
	// are the ones the user may already have.
	assetVersions := map[string][]string{} // url or digest -> versions
	for _, r := range rels {
		v := strings.TrimPrefix(r.Tag, "v")
		for _, a := range r.Assets {
			if !assetFor(runtime.GOOS, a.Name) {
				continue
			}
			for _, k := range []string{a.URL, a.Digest} {
				if k == "" {
					continue
				}
				assetVersions[k] = append(assetVersions[k], v)
			}
		}
	}
	replayed := func(v string) bool {
		for _, k := range []string{} {
			_ = k
		}
		for _, vs := range assetVersions {
			for _, w := range vs {
				if w != v && Newer(v, w) {
					// v is newer but reuses w's bytes
					matched := false
					for _, u := range vs {
						if u == v {
							matched = true
						}
					}
					if matched {
						return true
					}
				}
			}
		}
		return false
	}

	// Channel policy: beta builds are candidates (see ReleasesURL), drafts are
	// not, and Newer makes a final release beat its own pre-release.
	for _, r := range rels {
		v := strings.TrimPrefix(r.Tag, "v")
		if replayed(v) {
			if Newer(v, current) && (info.Latest == nil || Newer(v, info.Latest.Version)) {
				info.Error = "the update server offered an old build under a new version, so check again later"
			}
			continue
		}
		if r.Draft || !Newer(v, current) || (info.Latest != nil && !Newer(v, info.Latest.Version)) {
			continue
		}
		if info.Latest != nil {
			latest, _ := parseVersion(info.Latest.Version)
			cur, _ := parseVersion(v)
			if !cur.newerThan(latest) {
				continue
			}
		}
		for _, a := range r.Assets {
			if assetFor(runtime.GOOS, a.Name) {
				info.Latest = &Release{Version: v, Name: r.Name, Notes: r.Body, Page: r.Page, Asset: a.Name, URL: a.URL, Size: a.Size, Digest: a.Digest, Prerelease: r.Prerelease}
				break
			}
		}
	}
	info.Available = info.Latest != nil
	return info
}

// digestSHA256 returns the hex SHA-256 in a GitHub digest such as
// "sha256:9f86d0...". A missing digest, another algorithm or the wrong
// length is an error, so Download never installs what it cannot verify.
func digestSHA256(digest string) (string, error) {
	algo, hexPart, ok := strings.Cut(strings.TrimSpace(digest), ":")
	if !ok || !strings.EqualFold(algo, "sha256") {
		return "", fmt.Errorf("this release has no SHA-256 digest, so EZLM did not install it; try again later")
	}
	if len(hexPart) != sha256.Size*2 {
		return "", fmt.Errorf("this release's SHA-256 digest is the wrong length, so EZLM did not install it; try again later")
	}
	if _, err := hex.DecodeString(hexPart); err != nil {
		return "", fmt.Errorf("this release's SHA-256 digest is not valid hex, so EZLM did not install it; try again later")
	}
	return strings.ToLower(hexPart), nil
}

// DownloadDir is where the installer waits to be run: EZLM's own folder, not
// %TEMP%. Windows treats a program started from a temp folder as untrusted,
// and Disk Cleanup or Storage Sense can delete it between the download and
// the moment the installer runs.
func DownloadDir() string { return filepath.Join(paths.Home(), "update") }

// Download fetches the release's installer (resuming) and checks it against
// GitHub's published SHA-256 before anything runs it. The asset's redirects are
// resolved first, so the bytes come from an address the policy approved.
func Download(ctx context.Context, r *Release, prog engine.Progress) (string, error) {
	// Validate the published digest before fetching a byte: a release with no
	// digest, another algorithm or the wrong length cannot be verified.
	if _, err := digestSHA256(r.Digest); err != nil {
		return "", err
	}
	if r.Size > maxAssetSize {
		return "", fmt.Errorf("the update file is %d MB, far bigger than an EZLM installer should be, so EZLM didn't download it. Get it by hand from the releases page and try again", r.Size>>20)
	}
	dir := DownloadDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, filepath.Base(r.Asset))
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var slow atomic.Bool
	var last, windowBytes uint64
	var windowStart time.Time
	// engine's stall watchdog only notices silence, so a server that trickles
	// a byte at a time can keep a download alive for hours. Cut it when a
	// whole window passes under the speed floor.
	watch := func(done, total uint64, rate float64) {
		now := time.Now()
		if windowStart.IsZero() {
			windowStart, last = now, done
		}
		if done > last {
			windowBytes += done - last
			last = done
		}
		if elapsed := now.Sub(windowStart); elapsed >= slowWindow {
			if float64(windowBytes) < float64(slowFloor)*elapsed.Seconds() {
				slow.Store(true)
				cancel()
			}
			windowStart, windowBytes = now, 0
		}
		if prog != nil {
			prog(done, total, rate)
		}
	}
	if err := engine.Download(ctx, r.URL, dst, r.Size, watch); err != nil {
		if slow.Load() {
			return "", fmt.Errorf("the update download was too slow, under %d KB/s for %s, so EZLM stopped it and kept the part it had. Try again, or download the installer by hand from the releases page", slowFloor>>10, slowWindow)
		}
		return "", err
	}
	// A body can stop on a clean HTTP boundary (a proxy or CDN that cuts on a
	// chunk boundary), which the transfer can't tell from the end of the file.
	// Without a size or a digest there is nothing left to check the body
	// against, so refuse it rather than run half an installer.
	if r.Size == 0 && !strings.HasPrefix(r.Digest, "sha256:") {
		os.Remove(dst)
		return "", fmt.Errorf("EZLM couldn't check that the update downloaded completely, because the release lists no size or checksum. Try again, and if it happens again, report this problem")
	}
	if r.Size > 0 {
		st, err := os.Stat(dst)
		if err != nil {
			return "", err
		}
		if uint64(st.Size()) != r.Size {
			os.Remove(dst)
			return "", fmt.Errorf("the update downloaded the wrong size (%d bytes instead of %d), so EZLM deleted it. Try again, and if it happens again, report this problem", st.Size(), r.Size)
		}
	}
	want, err := digestSHA256(r.Digest)
	if err != nil {
		os.Remove(dst)
		return "", err
	}
	got, err := fileSHA256(dst)
	if err != nil {
		os.Remove(dst)
		return "", err
	}
	if got != want {
		os.Remove(dst)
		return "", fmt.Errorf("the downloaded file does not match its published checksum, so EZLM deleted it; download the update again")
	}
	if err := verifyPlatform(dst, runtime.GOOS, runtime.GOARCH); err != nil {
		os.Remove(dst)
		return "", err
	}
	return dst, nil
}

// verifyPlatform checks that the downloaded file really is this platform's
// artifact. A release can publish another platform's bytes under the right
// name, and a digest only proves the bytes are the ones GitHub listed, so the
// contents are checked before the app runs or mounts the file.
func verifyPlatform(path, goos, goarch string) error {
	switch goos {
	case "windows":
		if !strings.EqualFold(filepath.Ext(path), ".exe") {
			return fmt.Errorf("the update file %s is not a Windows .exe, so EZLM deleted it. Try again, and if it happens again, report this problem", filepath.Base(path))
		}
		machine, ok := peMachine(path)
		if !ok {
			return wrongPlatform("a Windows installer", kindOf(path))
		}
		if !peRunsOn(machine, goarch) {
			return fmt.Errorf("the update file is for %s, but this computer is %s, so EZLM deleted it. Try again, and if it happens again, report this problem", peArchName(machine), goarch)
		}
	case "darwin":
		if !strings.EqualFold(filepath.Ext(path), ".dmg") {
			return fmt.Errorf("the update file %s is not a macOS .dmg, so EZLM deleted it. Try again, and if it happens again, report this problem", filepath.Base(path))
		}
		if !isDMG(path) {
			return wrongPlatform("a macOS disk image", kindOf(path))
		}
	}
	return nil
}

func wrongPlatform(want, got string) error {
	return fmt.Errorf("the update file is not %s: its contents look like %s, so EZLM deleted it. Try again, and if it happens again, report this problem", want, got)
}

// kindOf names a file's type from its first bytes, for the rejection message.
func kindOf(path string) string {
	b := make([]byte, 8)
	f, err := os.Open(path)
	if err != nil {
		return "an unreadable file"
	}
	defer f.Close()
	n, _ := io.ReadFull(f, b)
	b = b[:n]
	switch {
	case len(b) >= 4 && string(b[:4]) == "\x7fELF":
		return "a Linux program"
	case len(b) >= 2 && string(b[:2]) == "MZ":
		if _, ok := peMachine(path); ok {
			return "a Windows program"
		}
		return "a DOS program"
	case len(b) >= 4 && string(b[:4]) == "PK\x03\x04":
		return "a zip archive"
	case len(b) >= 2 && b[0] == 0x1f && b[1] == 0x8b:
		return "a gzip archive"
	case len(b) >= 4 && isMachOMagic(binary.LittleEndian.Uint32(b)):
		return "a macOS program"
	case isDMG(path):
		return "a macOS disk image"
	}
	return "an unrecognized file"
}

func isMachOMagic(m uint32) bool {
	switch m {
	case 0xfeedface, 0xfeedfacf, 0xcefaedfe, 0xcffaedfe, 0xcafebabe, 0xbebafeca:
		return true
	}
	return false
}

// peMachine reads the COFF machine field of a PE executable.
func peMachine(path string) (uint16, bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, false
	}
	defer f.Close()
	var head [0x40]byte
	if _, err := io.ReadFull(f, head[:]); err != nil || string(head[:2]) != "MZ" {
		return 0, false
	}
	off := binary.LittleEndian.Uint32(head[0x3c:])
	if off < 0x40 || off > 1<<20 {
		return 0, false
	}
	var sig [6]byte
	if _, err := f.ReadAt(sig[:], int64(off)); err != nil || string(sig[:4]) != "PE\x00\x00" {
		return 0, false
	}
	return binary.LittleEndian.Uint16(sig[4:]), true
}

// peRunsOn reports whether a PE built for machine can run on goarch. EZLM's
// Windows installer is one 32-bit PE that picks x64 or ARM64 files at install
// time (windows/installer/EZLM.iss), so a 32-bit setup is always accepted and
// an x64 program is accepted on ARM64 Windows, which emulates it.
func peRunsOn(machine uint16, goarch string) bool {
	if machine == 0x014c { // 32-bit; runs on x64 and ARM64 Windows
		return true
	}
	switch goarch {
	case "amd64":
		return machine == 0x8664
	case "arm64":
		return machine == 0xaa64 || machine == 0x8664
	case "arm":
		return machine == 0x01c0 || machine == 0x01c4
	}
	return true
}

func peArchName(machine uint16) string {
	switch machine {
	case 0x8664:
		return "amd64"
	case 0xaa64:
		return "arm64"
	case 0x014c:
		return "386"
	case 0x01c0, 0x01c4:
		return "arm"
	}
	return fmt.Sprintf("an unknown processor (0x%04x)", machine)
}

// isDMG reports whether path is a UDIF disk image: its 512-byte trailer
// starts with "koly", which hdiutil writes and attaches.
func isDMG(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.Size() < 512 {
		return false
	}
	var magic [4]byte
	if _, err := f.ReadAt(magic[:], st.Size()-512); err != nil {
		return false
	}
	return string(magic[:]) == "koly"
}

// compareIdentifier orders prerelease identifiers: numeric ones by value and
// below alphanumeric ones, as SemVer does.
// rateLimited reports how long GitHub asked the updater to wait, if it is
// still inside a rate-limit window.
func rateLimited() (time.Duration, bool) {
	rateMu.Lock()
	defer rateMu.Unlock()
	if wait := time.Until(rateUntil); wait > 0 {
		return wait, true
	}
	return 0, false
}

// noteRateLimit starts a window in which Check sends nothing to GitHub.
func noteRateLimit(wait time.Duration) {
	if wait <= 0 {
		wait = defaultRateLimitWait
	}
	if wait > maxRateLimitWait {
		wait = maxRateLimitWait
	}
	rateMu.Lock()
	rateUntil = time.Now().Add(wait)
	rateMu.Unlock()
}

// clearRateLimit forgets the window; tests use it to stay independent.
func clearRateLimit() {
	rateMu.Lock()
	rateUntil = time.Time{}
	rateMu.Unlock()
}

// retryDelay reports how long GitHub asked the updater to wait, if the
// response is a rate limit at all. A 403 counts only when GitHub says the
// limit is used up or sends Retry-After; any other 403 is a real refusal.
func retryDelay(resp *http.Response, now time.Time) (time.Duration, bool) {
	limited := resp.StatusCode == http.StatusTooManyRequests
	if resp.StatusCode == http.StatusForbidden {
		limited = resp.Header.Get("X-RateLimit-Remaining") == "0" || resp.Header.Get("Retry-After") != ""
	}
	if !limited {
		return 0, false
	}
	wait := time.Duration(-1)
	if after := strings.TrimSpace(resp.Header.Get("Retry-After")); after != "" {
		if secs, err := strconv.Atoi(after); err == nil {
			wait = time.Duration(secs) * time.Second
		} else if at, err := http.ParseTime(after); err == nil {
			wait = at.Sub(now)
		}
	}
	if wait < 0 {
		if reset, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
			wait = time.Unix(reset, 0).Sub(now)
		}
	}
	if wait < 0 {
		wait = defaultRateLimitWait
	}
	if wait > maxRateLimitWait {
		wait = maxRateLimitWait
	}
	return wait, true
}

// rateLimitMessage says, in plain words, when the updater will ask again.
func rateLimitMessage(wait time.Duration) string {
	if wait < time.Minute {
		return "GitHub is limiting update checks, so EZLM will try again in a few seconds"
	}
	minutes := int((wait + time.Minute - 1) / time.Minute)
	if minutes == 1 {
		return "GitHub is limiting update checks, so EZLM will try again in a minute"
	}
	return fmt.Sprintf("GitHub is limiting update checks, so EZLM will try again in %d minutes", minutes)
}

var (
	// maxAssetSize is the biggest asset the updater will start downloading.
	// GitHub reports the size before the transfer starts, so a release that
	// claims to be many gigabytes is refused instead of filling the disk.
	maxAssetSize uint64 = 1 << 30

	// slowFloor and slowWindow stop a download whose speed stays below
	// slowFloor bytes per second for a whole window. A server that sends a
	// few bytes at a time never trips engine's stall watchdog, which only
	// notices silence.
	slowFloor  uint64 = 16 << 10
	slowWindow        = 30 * time.Second
)

// fileSHA256 returns the lower-case hex SHA-256 of a file.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
