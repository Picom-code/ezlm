package serve

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A server that quits while loading (a graphics driver crash, a bad flag)
// writes no known error line; the wait must say so in seconds, with the log,
// rather than wait out its whole timeout.
func TestWaitHealthyNoticesAStoppedServer(t *testing.T) {
	home := t.TempDir()
	t.Setenv("EZLM_HOME", home)
	os.MkdirAll(filepath.Join(home, "logs"), 0o755)
	os.WriteFile(LogPath(), []byte("load_backend: loaded Vulkan backend\nerror while handling argument \"--device\": invalid device: no-such-gpu\n"), 0o644)
	l, err := net.Listen("tcp", "127.0.0.1:0") // a port nothing answers on
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()

	oldGrace, oldChecks := stoppedGrace, stoppedChecks
	stoppedGrace, stoppedChecks = 0, 2
	defer func() { stoppedGrace, stoppedChecks = oldGrace, oldChecks }()

	start := time.Now()
	err = WaitHealthy(context.Background(), &Config{Port: port}, time.Minute, nil)
	if err == nil || !strings.HasPrefix(err.Error(), ErrStopped) || !strings.Contains(err.Error(), "invalid device") {
		t.Fatalf("got %v", err)
	}
	if took := time.Since(start); took > 15*time.Second {
		t.Fatalf("took %s to notice", took)
	}
}

// A server that's still loading answers on its port (503) even when OwnPIDs
// can't see it (a profile reached through a link): that's not "stopped".
func TestWaitHealthyKeepsWaitingForALoadingServer(t *testing.T) {
	home := t.TempDir()
	t.Setenv("EZLM_HOME", home)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"Loading model"}}`, http.StatusServiceUnavailable)
	})}
	go srv.Serve(l)
	defer srv.Close()

	oldGrace, oldChecks := stoppedGrace, stoppedChecks
	stoppedGrace, stoppedChecks = 0, 2
	defer func() { stoppedGrace, stoppedChecks = oldGrace, oldChecks }()

	err = WaitHealthy(context.Background(), &Config{Port: l.Addr().(*net.TCPAddr).Port}, 4*time.Second, nil)
	if err == nil || strings.HasPrefix(err.Error(), ErrStopped) {
		t.Fatalf("a loading server must be waited for, got %v", err)
	}
}
