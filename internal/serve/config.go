// Package serve runs llama-server as a background service and talks to it.
package serve

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/picom-code/ezlm/internal/paths"
)

// APIKey is one named credential for the server.
type APIKey struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Key     string    `json:"key"`
	Created time.Time `json:"created"`
}

// Config is the deployed setup, persisted at ~/.ezlm/config.json.
type Config struct {
	// Key mirrors Keys[0], the key EZLM wires into OpenCode and Hermes.
	Key       string   `json:"key"`
	Keys      []APIKey `json:"keys"`
	Port      int      `json:"port"`
	LAN       bool     `json:"lan"`
	ModelID   string   `json:"model_id"`
	ModelName string   `json:"model_name"`
	Quant     string   `json:"quant"`
	ModelPath string   `json:"model_path"`
	ModelsDir string   `json:"models_dir,omitempty"`
	Ctx       int      `json:"ctx"`
	NGL       int      `json:"ngl"`
	NCPUMoE   int      `json:"n_cpu_moe"`
	Server    string   `json:"server"`
	EngineTag string   `json:"engine_tag"`
	ToolCalls bool     `json:"tool_calls"`
	DraftPath string   `json:"draft_path,omitempty"`
	ExtraArgs []string `json:"extra_args,omitempty"` // recipe flags chosen by the fit engine
	// CPUFallback says why the model runs on the processor: the graphics
	// card's driver couldn't load it (see app.cpuInstead). Empty normally.
	CPUFallback string `json:"cpu_fallback,omitempty"`
	Recipe      string `json:"recipe,omitempty"`   // human-readable summary of the tuning
	GPUNeed     uint64 `json:"gpu_need,omitempty"` // planned graphics memory use
	RAMNeed     uint64 `json:"ram_need,omitempty"` // planned system memory use
	// Props: what /props said after the last deploy (reasoning levels for
	// the app writers). Nil until read.
	Props *Props `json:"props,omitempty"`

	DeployedAt time.Time `json:"deployed_at"`
}

func Load() (*Config, error) {
	b, err := os.ReadFile(paths.Config())
	if err != nil {
		return nil, err
	}
	tightenConfigMode()
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf")) // a BOM from editing in Notepad or PowerShell
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("EZLM's settings file (%s) is damaged: %v", paths.Config(), err)
	}
	if len(c.Keys) == 0 && c.Key != "" { // configs from before named keys
		c.Keys = []APIKey{{ID: "k1", Name: "Default", Key: c.Key, Created: c.DeployedAt}}
	}
	c.repairKeys()
	return &c, nil
}

// repairKeys makes the key list usable. An empty value is replaced with a
// fresh key, because llama-server accepts "Bearer " for an empty --api-key:
// that is no protection on a LAN. Key mirrors the first entry, the one EZLM
// hands to connected apps; a file with the two out of step would give an
// app a key the server rejects.
func (c *Config) repairKeys() {
	for i := range c.Keys {
		if strings.TrimSpace(c.Keys[i].Key) == "" {
			c.Keys[i].Key = newKey()
		}
	}
	if len(c.Keys) > 0 {
		c.Key = c.Keys[0].Key
	}
}

// tightenConfigMode makes config.json private again when it was copied from
// elsewhere or written by an older build. Best effort: reading still works
// when the mode can't be changed.
func tightenConfigMode() {
	if runtime.GOOS == "windows" {
		return // Windows protects the file with ACLs, not mode bits
	}
	if fi, err := os.Stat(paths.Config()); err == nil && fi.Mode().Perm()&0o077 != 0 {
		os.Chmod(paths.Config(), 0o600)
	}
}

func (c *Config) Save() error {
	c.repairKeys()
	if err := paths.Ensure(); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	tmp := paths.Config() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	// os.WriteFile keeps the mode of a file that already exists, so a
	// config.json.tmp left behind with loose bits would be renamed into
	// place readable by other users.
	if err := os.Chmod(tmp, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, paths.Config())
}

func (c *Config) Host() string {
	if c.LAN {
		return "0.0.0.0"
	}
	return "127.0.0.1"
}

// LocalURL is the OpenAI-compatible base URL on this machine.
func (c *Config) LocalURL() string { return fmt.Sprintf("http://127.0.0.1:%d/v1", c.Port) }

// LANURL is the base URL other devices on the network use ("" if LAN is off).
func (c *Config) LANURL() string {
	if !c.LAN {
		return ""
	}
	if ip := lanIP(); ip != "" {
		return fmt.Sprintf("http://%s:%d/v1", ip, c.Port)
	}
	return ""
}

// KeyFile is the file llama-server reads its accepted keys from, one per
// line. A file keeps the keys out of the process arguments, which every local
// user can read with ps on a shared machine.
func KeyFile() string { return filepath.Join(paths.Home(), "api-keys.txt") }

// writeKeyFile writes the accepted keys for --api-key-file. llama-server
// treats each line as one whole key, so a key is never comma-joined here.
func (c *Config) writeKeyFile() error {
	if err := os.MkdirAll(paths.Home(), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	for _, k := range c.keyValues() {
		b.WriteString(k)
		b.WriteByte('\n')
	}
	tmp := KeyFile() + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o600); err != nil {
		return err
	}
	// WriteFile keeps the mode of a file that already exists; make sure a
	// key file made by an older EZLM (or a wider umask) is private.
	if err := os.Chmod(tmp, 0o600); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, KeyFile())
}

// Args are the llama-server flags. Every memory-relevant flag is explicit:
// nothing is left to defaults that could overcommit memory.
func (c *Config) Args() []string {
	a := []string{
		"-m", c.ModelPath,
		"--alias", c.ModelID,
		"--host", c.Host(),
		"--port", strconv.Itoa(c.Port),
		"-c", strconv.Itoa(c.Ctx),
		"-ngl", strconv.Itoa(c.NGL),
		"--jinja",
		"--metrics", // live tokens/sec for the dashboard (protected by the API key)
		"-np", "1",  // one personal user: all memory goes to one long context
	}
	if err := c.writeKeyFile(); err == nil {
		a = append(a, "--api-key-file", KeyFile())
	} else {
		// A locked or read-only home leaves no file to read. The server still
		// starts, but the key is back on the command line for ps to show.
		a = append(a, "--api-key", c.keyList())
	}
	if c.NCPUMoE > 0 {
		a = append(a, "--n-cpu-moe", strconv.Itoa(c.NCPUMoE))
	}
	if c.DraftPath != "" {
		a = append(a, "-md", c.DraftPath)
	}
	if c.NGL > 0 && os.Getenv("EZLM_TEST_GPU_FAIL") != "" {
		a = append(a, "--device", "no-such-gpu") // developer check: the graphics card can't load the model
	}
	return append(a, c.ExtraArgs...)
}

// PortFree reports whether nothing listens on port and it can be bound on
// both loopback and all interfaces. Binding alone proves nothing on Windows:
// llama-server sets SO_REUSEADDR there, so a new listener succeeds on a port
// it's already serving (and a second llama-server then shares it). Connecting
// is the reliable test. Checking only 0.0.0.0 is not enough either: macOS lets
// a wildcard bind succeed while another process holds 127.0.0.1.
func PortFree(port int) bool {
	if c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 500*time.Millisecond); err == nil {
		c.Close()
		return false
	}
	for _, host := range []string{"127.0.0.1", "0.0.0.0"} {
		l, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
		if err != nil {
			return false
		}
		l.Close()
	}
	return true
}

// keyValues is every accepted key, in the order llama-server should read them.
func (c *Config) keyValues() []string {
	if len(c.Keys) == 0 {
		if c.Key == "" {
			return nil
		}
		return []string{c.Key}
	}
	ks := make([]string, 0, len(c.Keys))
	for _, k := range c.Keys {
		ks = append(ks, k.Key)
	}
	return ks
}

// keyList is every accepted key, comma-separated as llama-server expects.
func (c *Config) keyList() string {
	return strings.Join(c.keyValues(), ",")
}

// FreePort returns the first free TCP port at or after start. It looks past
// a busy run of 100 ports, because returning one of those would save a port
// known to be taken and fail the very start it was meant to fix. When nothing
// is free it returns start and the bind then reports why.
func FreePort(start int) int {
	if p := freePortFrom(start); p != 0 {
		return p
	}
	return start
}

func lanIP() string {
	conn, err := net.Dial("udp", "192.0.2.1:9") // no packets are sent
	if err != nil {
		return ""
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}

// StoppedFlag exists while the user has deliberately stopped the server.
func StoppedFlag() string { return filepath.Join(paths.Home(), "stopped") }

// ErrStillRunning: the server kept its port after being told to stop.
var ErrStillRunning = errors.New("the running model server didn't stop. It was probably started with administrator rights, which EZLM can't override: restart your computer, or end \"llama-server\" in Task Manager, then try again")

// StopAndWait stops EZLM's server and waits until it's really gone. Another
// program answering on the same port doesn't count (callers move to a free
// port); EZLM's own server still answering means it couldn't be stopped.
func StopAndWait(c *Config) error {
	Stop()
	if c == nil || c.Port == 0 {
		return nil
	}
	ctx := context.Background()
	for i := 0; i < 40; i++ {
		if len(OwnPIDs(ctx)) == 0 && (PortFree(c.Port) || !Ours(ctx, c)) {
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return ErrStillRunning
}

// newKey returns a fresh 32-byte hex API key. It lives here rather than in
// internal/keys so that keys' own tests can import this package without an
// import cycle.
func newKey() string {
	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	b := make([]byte, 40)
	max := big.NewInt(int64(len(alphabet)))
	for i := range b {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			panic("serve: cannot generate an API key: " + err.Error())
		}
		b[i] = alphabet[n.Int64()]
	}
	return "ezlm-" + string(b)
}
