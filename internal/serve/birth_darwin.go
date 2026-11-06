package serve

import (
	"os"
	"syscall"
	"time"
)

func birthTime(fi os.FileInfo) (time.Time, bool) {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return time.Unix(st.Birthtimespec.Sec, st.Birthtimespec.Nsec), true
	}
	return time.Time{}, false
}
