package engine

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

// TestLiveDownload downloads EZLM_LIVE_URL with the real downloader for
// EZLM_LIVE_SECONDS (default 180) and dumps every goroutine's stack if no
// progress arrives for 90 s, to find where a stalled download is stuck.
func TestLiveDownload(t *testing.T) {
	url := os.Getenv("EZLM_LIVE_URL")
	if url == "" {
		t.Skip("set EZLM_LIVE_URL")
	}
	secs := 180
	if s := os.Getenv("EZLM_LIVE_SECONDS"); s != "" {
		if n, err := time.ParseDuration(s + "s"); err == nil {
			secs = int(n.Seconds())
		}
	}
	dst := filepath.Join(t.TempDir(), "probe.gguf")
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(secs)*time.Second)
	defer cancel()
	var got, lastAt atomic.Int64
	lastAt.Store(time.Now().UnixNano())
	go func() {
		dumped := false
		for range time.Tick(5 * time.Second) {
			idle := time.Since(time.Unix(0, lastAt.Load()))
			t.Logf("%s: %.1f MB, idle %s", time.Now().Format("15:04:05"), float64(got.Load())/1e6, idle.Round(time.Second))
			if idle > 90*time.Second && !dumped {
				buf := make([]byte, 1<<20)
				t.Logf("STALLED, goroutines:\n%s", buf[:runtime.Stack(buf, true)])
				dumped = true
			}
		}
	}()
	err := Download(ctx, url, dst, 0, func(done, _ uint64, _ float64) {
		got.Store(int64(done))
		lastAt.Store(time.Now().UnixNano())
	})
	t.Logf("Download returned after the test window: %v (%.1f MB)", err, float64(got.Load())/1e6)
}
