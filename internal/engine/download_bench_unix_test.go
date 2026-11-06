//go:build unix

package engine

import (
	"runtime"
	"syscall"
	"time"
)

// peakRSS reports the largest resident set size this process has reached, in
// bytes. It only grows, so a test compares it before and after a transfer.
func peakRSS() int64 {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0
	}
	if runtime.GOOS == "darwin" { // macOS reports bytes, Linux kilobytes
		return int64(ru.Maxrss)
	}
	return int64(ru.Maxrss) * 1024
}

// processCPU reports the user and system CPU time this process has used. The
// fake server runs in this process, so the number covers both ends of the
// transfer.
func processCPU() time.Duration {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return 0
	}
	return time.Duration(ru.Utime.Sec+ru.Stime.Sec)*time.Second +
		time.Duration(ru.Utime.Usec+ru.Stime.Usec)*time.Microsecond
}
