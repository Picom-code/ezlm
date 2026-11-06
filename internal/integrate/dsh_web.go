package integrate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Opening DeepSeek Harness: the official browser UI, `dsh web` (DECISIONS 4).
// A dsh web that already serves this DSH_HOME is reused: EZLM's provider
// edits reach it live. Otherwise EZLM opens a terminal running dsh web,
// which opens the browser itself with a one-time sign-in token; closing the
// window stops the server.

// dshWebPort is dsh web's default port. EZLM uses it whenever it is free:
// the browser's sign-in cookie is tied to the address, so the same port
// keeps the user signed in from one run to the next.
const dshWebPort = 3080

// dshWebRecord remembers the last dsh web EZLM opened, so a later Open can
// find it again.
type dshWebRecord struct {
	Port int    `json:"port"`
	Home string `json:"home"`
}

func dshWebRecordPath() string { return filepath.Join(ToolsDir(), "dsh", "web.json") }

func dshLoadWeb() dshWebRecord {
	var w dshWebRecord
	if b, err := os.ReadFile(dshWebRecordPath()); err == nil {
		json.Unmarshal(b, &w)
	}
	return w
}

func dshSaveWeb(w dshWebRecord) error {
	b, _ := json.Marshal(w)
	return WriteFileAtomic(dshWebRecordPath(), append(b, '\n'), 0o644)
}

// dshWebAnswers reports whether a dsh web answers on 127.0.0.1:port. Without
// its cookie dsh web turns every page down with its own sentence, which is
// what identifies it.
func dshWebAnswers(port int) bool {
	c := &http.Client{Timeout: 800 * time.Millisecond}
	resp, err := c.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return strings.Contains(string(b), "dsh web")
}

// dshPortFree reports whether nothing listens on 127.0.0.1:port.
func dshPortFree(port int) bool {
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return false
	}
	l.Close()
	return true
}

// dshFreePort is a port the system has free right now.
func dshFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// dshPickWeb finds a running dsh web for this DSH_HOME (reuse) or the port a
// new one should use. A running one counts as this home's when EZLM opened
// it for this home, or when it sits on dsh's default port and this is dsh's
// default home.
func dshPickWeb() (port int, reuse bool, err error) {
	home := filepath.Clean(dshHome())
	if w := dshLoadWeb(); w.Port > 0 && filepath.Clean(w.Home) == home && dshWebAnswers(w.Port) {
		return w.Port, true, nil
	}
	if dshDefaultHome() && dshWebAnswers(dshWebPort) {
		return dshWebPort, true, nil
	}
	if dshPortFree(dshWebPort) {
		return dshWebPort, false, nil
	}
	port, err = dshFreePort()
	return port, false, err
}

func (dshTool) LaunchSpec(f Found, ep Endpoint, mode string) (LaunchSpec, error) {
	if mode != "" && mode != ModeWeb {
		return LaunchSpec{}, errors.New("DeepSeek Harness opens in your browser; use Open DeepSeek Harness (browser)")
	}
	port, reuse, err := dshPickWeb()
	if err != nil {
		return LaunchSpec{}, fmt.Errorf("EZLM couldn't find a free port for DeepSeek Harness: %v", err)
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/", port)
	if reuse {
		return LaunchSpec{Mode: ModeWeb, URL: url}, nil
	}
	node, script, err := dshRuntime(f)
	if err != nil {
		return LaunchSpec{}, err
	}
	env := append([]string{}, dshTelemetryEnv...)
	if os.Getenv("DSH_HOME") != "" {
		env = append(env, "DSH_HOME="+dshHome())
	}
	dshSaveWeb(dshWebRecord{Port: port, Home: dshHome()})
	return LaunchSpec{
		Mode:  ModeWeb,
		Title: "DeepSeek Harness (EZLM)",
		Bin:   node,
		Args:  []string{script, "web", "--port", strconv.Itoa(port)},
		Env:   env,
		Dir:   Workspace(),
		URL:   url,
	}, nil
}
