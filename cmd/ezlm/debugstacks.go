package main

import (
	"os"
	"runtime"
	"time"
)

// EZLM_DEBUG_STACKS=<file>: write every goroutine's stack to the file every
// 30 seconds (diagnosing hangs on Windows, where there's no SIGQUIT).
func init() {
	f := os.Getenv("EZLM_DEBUG_STACKS")
	if f == "" {
		return
	}
	go func() {
		for range time.Tick(30 * time.Second) {
			buf := make([]byte, 4<<20)
			os.WriteFile(f, buf[:runtime.Stack(buf, true)], 0o644)
		}
	}()
}
