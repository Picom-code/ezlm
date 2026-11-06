package serve

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// foreignServer starts a real listener on a real port, like the other program
// (a dev tool, a proxy, a second llama-server) that can hold EZLM's port.
func foreignServer(t *testing.T, h http.HandlerFunc) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: h}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	return l.Addr().(*net.TCPAddr).Port
}

// Another program on EZLM's port is never EZLM's server, whatever it answers:
// EZLM must move to a free port, not adopt it.
func TestForeignListenerIsNotOurs(t *testing.T) {
	key := "ezlm-foreign-test-key"
	model := "qwen3.6-35b-a3b"
	cases := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"empty model list", func(w http.ResponseWriter, r *http.Request) {
			// Ignores the key and lists nothing: a dev tool's JSON API.
			fmt.Fprint(w, `{"data":[]}`)
		}},
		{"json but no models", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"ok":true}`)
		}},
		{"not json", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `<!doctype html><h1>it works</h1>`)
		}},
		{"another model", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"data":[{"id":"granite-4.2-3b"}]}`)
		}},
		{"rejects the key", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			port := foreignServer(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/health" {
					w.WriteHeader(http.StatusOK) // many programs answer this
					return
				}
				tc.handler(w, r)
			})
			c := &Config{Port: port, Key: key, ModelID: model}
			ctx := context.Background()
			if Ours(ctx, c) {
				t.Errorf("Ours() adopted the program on port %d", port)
			}
			if Healthy(ctx, c) {
				t.Errorf("Healthy() adopted the program on port %d", port)
			}
		})
	}
}

// Our own server is still recognised: the same listener with our key and
// model alias must count as ours.
func TestOwnServerOnListenerIsOurs(t *testing.T) {
	port := foreignServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK) // /health is unauthenticated, like llama-server's
			return
		}
		if r.Header.Get("Authorization") != "Bearer ezlm-own-test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/v1/models" {
			fmt.Fprint(w, `{"data":[{"id":"qwen3.6-35b-a3b"}]}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	c := &Config{Port: port, Key: "ezlm-own-test-key", ModelID: "qwen3.6-35b-a3b"}
	if !Ours(context.Background(), c) {
		t.Fatal("our own server on the port was not recognised")
	}
	if !Healthy(context.Background(), c) {
		t.Fatal("our own healthy server was not recognised")
	}
}

// Stopping the server while another program holds the port must succeed, and
// must leave that program running: EZLM only stops its own processes.
func TestStopAndWaitLeavesForeignServerAlone(t *testing.T) {
	t.Setenv("EZLM_HOME", t.TempDir()) // own service name, never the real install's
	port := foreignServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		fmt.Fprint(w, `{"data":[]}`) // answers 200 to anything, even with no key
	})
	c := &Config{Port: port, Key: "ezlm-stop-test-key", ModelID: "qwen3.6-35b-a3b"}

	start := time.Now()
	if err := StopAndWait(c); err != nil {
		t.Fatalf("StopAndWait reported %q: another program on port %d was taken for our server", err, port)
	}
	if took := time.Since(start); took > 3*time.Second {
		t.Fatalf("StopAndWait took %s: it waited out the timeout for a foreign server", took)
	}
	// The other program is untouched and still listening.
	if PortFree(port) {
		t.Fatalf("port %d was freed: the other program's listener was closed", port)
	}
	resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
	if err != nil {
		t.Fatalf("the other program stopped answering: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("the other program answered %s after StopAndWait", resp.Status)
	}
}

// Another app's llama-server is never one of EZLM's own: OwnPIDs is the only
// gate before a kill, so a copy of the same program outside EZLM's folder must
// not appear in it. The same program inside EZLM's folder must.
func TestOwnPIDsIgnoresForeignLlamaServer(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	blob, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	home, err := filepath.EvalSymlinks(t.TempDir()) // /var is a link to /private/var on macOS
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("EZLM_HOME", home)

	foreign := copyExe(t, blob, t.TempDir(), "llama-server")
	pid := startFakeLlama(t, foreign)
	if PortFree(pid.port) {
		t.Fatalf("fake llama-server on port %d is not listening", pid.port)
	}
	if got := OwnPIDs(context.Background()); contains(got, int32(pid.cmd.Process.Pid)) {
		t.Fatalf("another app's llama-server (pid %d, outside %s) counts as EZLM's", pid.cmd.Process.Pid, home)
	}

	own := copyExe(t, blob, home, "llama-server")
	ownPID := startFakeLlama(t, own)
	if got := OwnPIDs(context.Background()); !contains(got, int32(ownPID.cmd.Process.Pid)) {
		t.Fatalf("EZLM's own llama-server (pid %d, inside %s) was not found; got %v", ownPID.cmd.Process.Pid, home, got)
	}
}

// TestHelperLlamaListen is the fake llama-server: it listens on a port and
// prints it, then waits to be stopped. Only runs when started by a test.
func TestHelperLlamaListen(t *testing.T) {
	if os.Getenv("EZLM_TEST_FAKE_LLAMA") != "1" {
		t.Skip("helper process")
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Println("ERR", err)
		os.Exit(1)
	}
	fmt.Printf("PORT %d\n", l.Addr().(*net.TCPAddr).Port)
	select {} // killed by the parent test
}

type fakeLlama struct {
	cmd  *exec.Cmd
	port int
}

// startFakeLlama runs exe as the fake llama-server and reads its port.
func startFakeLlama(t *testing.T, exe string) fakeLlama {
	t.Helper()
	cmd := exec.Command(exe, "-test.run=TestHelperLlamaListen")
	cmd.Env = append(os.Environ(), "EZLM_TEST_FAKE_LLAMA=1")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})
	sc := bufio.NewScanner(out)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if p, ok := strings.CutPrefix(line, "PORT "); ok {
			port, err := strconv.Atoi(p)
			if err != nil {
				t.Fatalf("helper printed %q", line)
			}
			return fakeLlama{cmd: cmd, port: port}
		}
	}
	t.Fatalf("fake llama-server exited before printing a port: %v", sc.Err())
	return fakeLlama{}
}

func copyExe(t *testing.T, blob []byte, dir, name string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	dst := dir + string(os.PathSeparator) + name
	if err := os.WriteFile(dst, blob, 0o755); err != nil {
		t.Fatal(err)
	}
	return dst
}

func contains(pids []int32, pid int32) bool {
	for _, p := range pids {
		if p == pid {
			return true
		}
	}
	return false
}
