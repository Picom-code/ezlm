//go:build !unix

package engine

import "time"

// peakRSS and processCPU are only used by the 20 GB test, which runs on the
// platforms EZLM ships. Elsewhere they report nothing so the test compiles.
func peakRSS() int64            { return 0 }
func processCPU() time.Duration { return 0 }
