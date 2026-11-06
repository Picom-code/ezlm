package integrate

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Which OpenCode EZLM runs (PLAN 4.2, DECISIONS 2 and 3).
//
// OpenCode 2.x keeps its sessions in a table named session_v2. A database
// 2.x created has only that table, and 1.x refuses it ("Database is not
// empty and has no session table"); a 1.x database that 2.x opened keeps the
// old session table next to the new one, and 1.19.5 still opens it. EZLM
// reads the format from the schema text in opencode.db and opencode.db-wal,
// so it needs no SQLite library and never opens the database itself.

// Data formats of ~/.local/share/opencode/opencode.db.
const (
	openCodeDataNone = "none" // no database yet
	openCodeDataV1   = "v1"   // session only: 1.x wrote it, 2.x would convert it
	openCodeDataV2   = "v2"   // session_v2 only: 1.x can't open it
	openCodeDataBoth = "both" // 2.x converted a 1.x database; both can open it
)

// openCodeChoice is the program EZLM runs and why.
type openCodeChoice struct {
	found  Found  // Bin "" when the pinned copy is the choice and isn't installed
	pinned bool   // EZLM's own copy rather than the user's
	note   string // why the user's opencode isn't used; "" when there is none
	user   string // the user's opencode, if any
	data   string // the chat history's format
}

// openCodeChoose applies the decision table. With a nil ctx it only reads
// cached probes (Detect); otherwise it probes what isn't cached (Resolve).
// data reads the chat history's format; it is called only for a 1.x, since
// the scan reads the whole database.
func openCodeChoose(ctx context.Context, r *Run, data func(string) string) openCodeChoice {
	c := openCodeChoice{user: openCodeUserBin()}
	if c.user != "" {
		var p Probe
		var ok bool
		if ctx == nil {
			p, ok = CachedProbe(c.user)
		} else {
			var err error
			p, err = openCodeProbe(ctx, r, c.user)
			ok = true // a failed probe leaves Version "", which the table rejects
			if err != nil {
				r.Logf("opencode probe of %s failed: %v", c.user, err)
			}
		}
		if !ok {
			// Not probed yet: show the user's program until Resolve decides.
			c.found = Found{Bin: c.user, Source: openCodeSource(c.user), Arch: openCodeArch(c.user)}
			return c
		}
		if p.Version != "" && !openCodeAtLeast(p.Version, "2.0.0") {
			c.data = data(openCodeDataDir())
		}
		c.pinned, c.note = openCodeDecide(p.Version, p.Arch, c.data, MachineArch(), runtime.GOOS)
		if !c.pinned {
			c.found = Found{Bin: c.user, Version: p.Version, Source: openCodeSource(c.user), Arch: p.Arch}
			return c
		}
	} else {
		c.pinned = true
	}
	c.found = openCodePinnedFound(ctx, r)
	return c
}

// openCodeDecide is the PLAN 4.2 table for a user's opencode of version (""
// when it didn't report one) and arch, given the data format and the
// machine. It returns whether EZLM's pinned copy is used instead, and the
// sentence that says why.
func openCodeDecide(version, arch, data, machine, goos string) (pinned bool, note string) {
	pin, _ := PinFor("opencode")
	if version == "" {
		return true, fmt.Sprintf("Your opencode command didn't start, so EZLM uses its own OpenCode %s.", pin.Version)
	}
	emulated := machine == "arm64" && arch == "amd64"
	if emulated {
		return true, fmt.Sprintf("Your opencode command is the x64 build, so EZLM uses OpenCode %s for Arm.", pin.Version)
	}
	switch {
	case openCodeAtLeast(version, "2.1.0"):
		return false, ""
	case openCodeAtLeast(version, "2.0.0"):
		return true, fmt.Sprintf("Your OpenCode %s is older than EZLM supports, so EZLM uses OpenCode %s.", version, pin.Version)
	case data == openCodeDataV2:
		return true, fmt.Sprintf("Your OpenCode 1.x can't open the chat history a newer OpenCode (for example OpenBot's) wrote, so EZLM uses OpenCode %s.", pin.Version)
	case openCodeAtLeast(version, "1.19.4"):
		return false, ""
	}
	return true, fmt.Sprintf("Your OpenCode %s is older than EZLM supports, so EZLM uses OpenCode %s.", version, pin.Version)
}

// openCodeUserBin is the user's opencode command: the first one on the
// search path that isn't EZLM's own copy.
func openCodeUserBin() string {
	for _, b := range FindAllTools("opencode") {
		if !IsEZLMTool(b) {
			return b
		}
	}
	return ""
}

// openCodePinnedBin is where EZLM's pinned OpenCode lives.
func openCodePinnedBin() string {
	pin, _ := PinFor("opencode")
	return filepath.Join(ToolDir("opencode", pin.Version), openCodeExe())
}

// openCodePinnedFound is EZLM's pinned copy if it is installed (and, with a
// ctx, still runs), else a Found with no Bin.
func openCodePinnedFound(ctx context.Context, r *Run) Found {
	pin, _ := PinFor("opencode")
	bin := openCodePinnedBin()
	if st, err := os.Stat(bin); err != nil || st.IsDir() {
		return Found{Source: SourceEZLM}
	}
	if ctx != nil {
		if p, err := openCodeProbe(ctx, r, bin); err != nil || p.Version != pin.Version {
			r.Logf("EZLM's OpenCode at %s doesn't run as %s (%q, %v); it will be installed again", bin, pin.Version, p.Version, err)
			return Found{Source: SourceEZLM}
		}
	}
	return Found{Bin: bin, Version: pin.Version, Source: SourceEZLM, Arch: BinaryArch(bin), Env: []string{"OPENCODE_DISABLE_AUTOUPDATE=1"}}
}

// openCodeFlags are the options a build takes, read from its --help.
type openCodeFlags struct {
	standalone bool // 2.x: run with a private server instead of the background service
	tuiModel   bool // 1.x: the TUI takes --model (2.x's doesn't)
	variant    bool // 1.x: run takes --variant (2.x uses model#variant)
}

func openCodeFlagsOf(f Found) openCodeFlags {
	if p, ok := CachedProbe(f.Bin); ok && p.Facts != nil {
		return openCodeFlags{standalone: p.Facts["standalone"] == "1", tuiModel: p.Facts["tui_model"] == "1", variant: p.Facts["variant"] == "1"}
	}
	v2 := openCodeAtLeast(f.Version, "2.0.0")
	return openCodeFlags{standalone: v2, tuiModel: !v2 && f.Version != "", variant: !v2 && f.Version != ""}
}

// openCodeProbe runs a build's --version and --help (cached while the file
// is unchanged). These read neither config nor chat history.
func openCodeProbe(ctx context.Context, r *Run, bin string) (Probe, error) {
	if p, ok := CachedProbe(bin); ok {
		return p, nil
	}
	f := Found{Bin: bin}
	out, err := r.Exec(ctx, openCodeTimeout, openCodeCmd(f, "", nil, "--version"))
	v := openCodeVersion(out)
	if v == "" {
		if err == nil {
			err = fmt.Errorf("no version in %q", ErrLine(out))
		}
		return Probe{}, err
	}
	help, _ := r.Exec(ctx, openCodeTimeout, openCodeCmd(f, "", nil, "--help"))
	runHelp, _ := r.Exec(ctx, openCodeTimeout, openCodeCmd(f, "", nil, "run", "--help"))
	flag := func(b bool) string {
		if b {
			return "1"
		}
		return ""
	}
	p := Probe{Version: v, Arch: openCodeArch(bin), Facts: map[string]string{
		"standalone": flag(strings.Contains(runHelp, "--standalone")),
		"tui_model":  flag(openCodeModelFlag.MatchString(help)),
		"variant":    flag(strings.Contains(runHelp, "--variant")),
	}}
	if err := SaveProbe(bin, p); err != nil {
		r.Logf("saving the probe of %s: %v", bin, err)
	}
	return p, nil
}

var (
	openCodeVersionRe = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)
	openCodeModelFlag = regexp.MustCompile(`--model\b`)
)

// openCodeVersion finds the version in `opencode --version` output: "1.19.5"
// from 1.x, "opencode v2.2.5" from 2.x.
func openCodeVersion(out string) string {
	return openCodeVersionRe.FindString(out)
}

// openCodeAtLeast compares dotted versions numerically; an unparsable
// version is never at least anything.
func openCodeAtLeast(v, min string) bool {
	a, b := openCodeVersionRe.FindStringSubmatch(v), openCodeVersionRe.FindStringSubmatch(min)
	if a == nil || b == nil {
		return false
	}
	for i := 1; i <= 3; i++ {
		x, _ := strconv.Atoi(a[i])
		y, _ := strconv.Atoi(b[i])
		if x != y {
			return x > y
		}
	}
	return true
}

// openCodeSource says where the user's opencode came from.
func openCodeSource(bin string) string {
	if IsEZLMTool(bin) {
		return SourceEZLM
	}
	real := bin
	if p, err := filepath.EvalSymlinks(bin); err == nil {
		real = p
	}
	slash := filepath.ToSlash(strings.ToLower(real))
	home, _ := os.UserHomeDir()
	switch {
	case strings.Contains(slash, "/node_modules/"), strings.Contains(slash, "/appdata/roaming/npm/"):
		return SourceNPM
	case strings.HasPrefix(slash, strings.ToLower(filepath.ToSlash(filepath.Join(home, ".opencode")))+"/"):
		return SourceScript
	}
	return SourcePath
}

// openCodeArch reads the program's architecture. An npm .cmd shim on
// Windows is followed to the opencode.exe it starts.
func openCodeArch(bin string) string {
	if ext := strings.ToLower(filepath.Ext(bin)); ext == ".cmd" || ext == ".bat" {
		if exe := openCodeShimTarget(bin); exe != "" {
			return BinaryArch(exe)
		}
		return ""
	}
	return BinaryArch(bin)
}

// openCodeShimTarget finds the native binary next to an npm shim: the
// @opencode/cli (2.x) or opencode-ai (1.x) package, whose platform package
// npm picked for Node's architecture.
func openCodeShimTarget(shim string) string {
	nm := filepath.Join(filepath.Dir(shim), "node_modules")
	cands := []string{filepath.Join(nm, "@opencode", "cli", "bin", "opencode.exe")}
	for _, pkg := range []string{"opencode-windows-arm64", "opencode-windows-x64", "opencode-windows-x64-baseline"} {
		cands = append(cands,
			filepath.Join(nm, "opencode-ai", "node_modules", pkg, "bin", "opencode.exe"),
			filepath.Join(nm, pkg, "bin", "opencode.exe"))
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return ""
}

// openCodeDataDir is OpenCode's data folder: $XDG_DATA_HOME/opencode, else
// ~/.local/share/opencode (Windows too).
func openCodeDataDir() string {
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "opencode")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "opencode")
}

// openCodeDBFormat scans opencode.db and its WAL for the session tables'
// schema text. session_v2 anywhere means 2.x has used the database (1.x
// never writes that name); stale text in free pages can't turn a v2
// database back into v1.
func openCodeDBFormat(dataDir string) string {
	db := filepath.Join(dataDir, "opencode.db")
	st, err := os.Stat(db)
	if err != nil || st.Size() == 0 {
		return openCodeDataNone
	}
	var v1, v2 bool
	for _, p := range []string{db + "-wal", db} {
		a, b := openCodeScanSchema(p)
		v1, v2 = v1 || a, v2 || b
	}
	switch {
	case v1 && v2:
		return openCodeDataBoth
	case v2:
		return openCodeDataV2
	case v1:
		return openCodeDataV1
	}
	// A database with neither table is one 1.x would refuse too.
	return openCodeDataV2
}

// openCodeSchemaMark starts every table definition SQLite stores.
var openCodeSchemaMark = []byte("CREATE TABLE ")

// openCodeScanSchema reads path in chunks and reports whether it holds a
// CREATE TABLE for `session` and for `session_v2`. A read error (for example
// the byte range SQLite locks on Windows at 1 GiB) skips ahead instead of
// giving up.
func openCodeScanSchema(path string) (v1, v2 bool) {
	f, err := os.Open(path)
	if err != nil {
		return false, false
	}
	defer f.Close()
	const chunk, overlap = 4 << 20, 96
	buf := make([]byte, chunk+overlap)
	var off int64
	keep := 0
	for {
		n, err := f.ReadAt(buf[keep:], off)
		data := buf[:keep+n]
		a, b := openCodeFindSchema(data)
		v1, v2 = v1 || a, v2 || b
		if v2 && v1 {
			return
		}
		if err != nil && err != io.EOF {
			// The lock-byte page: skip 64 KiB past it and go on.
			const lock = 1 << 30
			if off < lock+65536 && off+int64(chunk) > lock {
				off, keep = lock+65536, 0
				continue
			}
			return
		}
		if err == io.EOF || n == 0 {
			return
		}
		off += int64(n)
		keep = copy(buf, data[max(0, len(data)-overlap):])
	}
}

// openCodeFindSchema looks for `CREATE TABLE [IF NOT EXISTS] <q>session<q>`
// and the same for session_v2, with any of SQLite's identifier quotes.
func openCodeFindSchema(b []byte) (v1, v2 bool) {
	for {
		i := bytes.Index(b, openCodeSchemaMark)
		if i < 0 {
			return
		}
		b = b[i+len(openCodeSchemaMark):]
		rest := bytes.TrimPrefix(b, []byte("IF NOT EXISTS "))
		if len(rest) > 0 && strings.IndexByte("`\"[", rest[0]) >= 0 {
			rest = rest[1:]
		}
		for _, t := range []struct {
			name string
			hit  *bool
		}{{"session_v2", &v2}, {"session", &v1}} {
			if bytes.HasPrefix(rest, []byte(t.name)) && len(rest) > len(t.name) && strings.IndexByte("`\"] (\n\t", rest[len(t.name)]) >= 0 {
				*t.hit = true
				break
			}
		}
	}
}

// The data format for Detect, which runs on every status poll: rescanned
// when the files change, but at most every 30 s while they keep changing (a
// running OpenCode writes all the time), and never again once a database is
// v2 (it can't go back) unless it is replaced.
var openCodeDBCache struct {
	sync.Mutex
	m map[string]openCodeDBSeen
}

type openCodeDBSeen struct {
	db, wal os.FileInfo
	format  string
	at      time.Time
}

func openCodeDBFormatCached(dataDir string) string {
	db := filepath.Join(dataDir, "opencode.db")
	st, _ := os.Stat(db)
	wal, _ := os.Stat(db + "-wal")
	openCodeDBCache.Lock()
	seen, ok := openCodeDBCache.m[dataDir]
	openCodeDBCache.Unlock()
	if ok && openCodeSameFile(seen.db, st) {
		if seen.format == openCodeDataV2 || seen.format == openCodeDataBoth {
			return seen.format
		}
		if openCodeSameStat(seen.db, st) && openCodeSameStat(seen.wal, wal) || time.Since(seen.at) < 30*time.Second {
			return seen.format
		}
	}
	format := openCodeDBFormat(dataDir)
	openCodeDBCache.Lock()
	if openCodeDBCache.m == nil {
		openCodeDBCache.m = map[string]openCodeDBSeen{}
	}
	openCodeDBCache.m[dataDir] = openCodeDBSeen{db: st, wal: wal, format: format, at: time.Now()}
	openCodeDBCache.Unlock()
	return format
}

func openCodeSameFile(a, b os.FileInfo) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return os.SameFile(a, b)
}

func openCodeSameStat(a, b os.FileInfo) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}
