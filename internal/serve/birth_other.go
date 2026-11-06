//go:build !darwin

package serve

import (
	"os"
	"time"
)

func birthTime(os.FileInfo) (time.Time, bool) { return time.Time{}, false }
