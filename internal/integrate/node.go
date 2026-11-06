package integrate

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// The Node.js runtime DeepSeek Harness runs on (dsh workstream). EZLM reuses
// the user's node when it satisfies dsh's engines range, else installs its
// pinned Node 24 LTS into ToolDir("node", <version>) with the SHA-256 from
// SHASUMS256.txt pinned in tools.json (DECISIONS 4).

// nodeMajor is the Node line EZLM installs.
const nodeMajor = 24

// nodeFind is the user's node, if any.
func nodeFind() string { return FindTool("node") }

// nodeBinIn is the node program inside a Node.js folder.
func nodeBinIn(dir string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(dir, "node.exe")
	}
	return filepath.Join(dir, "bin", "node")
}

// nodeEZLM is EZLM's own pinned node, or "" when it isn't installed.
func nodeEZLM() string {
	p, ok := PinFor("node")
	if !ok {
		return ""
	}
	bin := nodeBinIn(ToolDir("node", p.Version))
	if st, err := os.Stat(bin); err != nil || st.IsDir() {
		return ""
	}
	return bin
}

// nodeNpmCLI is npm's script next to a node program (npm ships with every
// Node.js build), or "" when that node has no npm. Running it with the node
// itself avoids npm's shell shims and whatever node comes first on PATH.
func nodeNpmCLI(node string) string {
	real, err := filepath.EvalSymlinks(node)
	if err != nil {
		real = node
	}
	dir := filepath.Dir(real)
	for _, c := range []string{
		filepath.Join(dir, "node_modules", "npm", "bin", "npm-cli.js"),              // Windows layout
		filepath.Join(dir, "..", "lib", "node_modules", "npm", "bin", "npm-cli.js"), // Unix layout
	} {
		if _, err := os.Stat(c); err == nil {
			return filepath.Clean(c)
		}
	}
	return ""
}

// nodeVersionOf runs `node --version` (cached while the file is unchanged)
// and returns it without the "v", e.g. "22.22.3".
func nodeVersionOf(ctx context.Context, r *Run, node string) (string, error) {
	if p, ok := CachedProbe(node); ok && p.Version != "" {
		return p.Version, nil
	}
	out, err := r.Exec(ctx, 30*time.Second, exec.Command(node, "--version"))
	if err != nil {
		return "", fmt.Errorf("%s --version failed: %s", node, ErrLine(out))
	}
	v := strings.TrimPrefix(strings.TrimSpace(nodeLastLine(out)), "v")
	if _, ok := nodeSemver(v); !ok {
		return "", fmt.Errorf("%s --version printed %q", node, out)
	}
	SaveProbe(node, Probe{Version: v, Arch: BinaryArch(node)})
	return v, nil
}

func nodeLastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return s[i+1:]
	}
	return s
}

// nodeSemver parses "22.19.0" (a "v" prefix and a pre-release suffix are
// ignored).
func nodeSemver(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// nodeAtLeast reports whether version have is min or newer.
func nodeAtLeast(have, min string) bool {
	h, ok1 := nodeSemver(have)
	m, ok2 := nodeSemver(min)
	if !ok1 || !ok2 {
		return false
	}
	for i := range h {
		if h[i] != m[i] {
			return h[i] > m[i]
		}
	}
	return true
}

// nodePick is the first node that is min or newer: the given candidates in
// order, then EZLM's own. It returns "" when none is.
func nodePick(ctx context.Context, r *Run, min string, candidates ...string) (bin, version string) {
	seen := map[string]bool{}
	for _, c := range append(candidates, nodeEZLM()) {
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		v, err := nodeVersionOf(ctx, r, c)
		if err != nil {
			r.Logf("node %s: %v", c, err)
			continue
		}
		if nodeAtLeast(v, min) {
			return c, v
		}
		r.Logf("node %s is %s, older than the %s DeepSeek Harness needs", c, v, min)
	}
	return "", ""
}
