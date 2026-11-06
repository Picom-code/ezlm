package integrate

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// dsh's cross-process writer lock (@deepseek-ai/dsh-atomic-write, 0.2.1-rc.5):
// a writer creates "<file>.lock" exclusively, holding "<pid>\n" at mode 0600,
// does its read-modify-write, and removes it. Contenders retry from 20 ms,
// doubling to 200 ms, until their deadline. A lock whose PID no longer runs
// is taken over under a claim file "<lock>.takeover-<first 16 hex of the
// record's SHA-256>", re-checking the record and PID before removing it.
// The credential store's writers wait 30 s; the profile editor locks the
// profile's package.json with the 2 s default.

var dshLockRecord = regexp.MustCompile(`^\d+\n$`)

// dshWithLock runs fn holding dsh's writer lock for target. The folder must
// exist.
func dshWithLock(target string, wait time.Duration, fn func() error) error {
	lock := target + ".lock"
	deadline := time.Now().Add(wait)
	delay := 20 * time.Millisecond
	retriedPerm := false
	for {
		f, err := os.OpenFile(lock, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			_, werr := f.WriteString(strconv.Itoa(os.Getpid()) + "\n")
			cerr := f.Close()
			if werr != nil || cerr != nil {
				os.Remove(lock)
				return errors.Join(werr, cerr)
			}
			break
		}
		if !dshLockContended(err, lock) {
			// Windows reports a racing exclusive create as a permission
			// error now and then; dsh retries one of those.
			if runtime.GOOS != "windows" || !errors.Is(err, os.ErrPermission) || retriedPerm {
				return err
			}
			retriedPerm = true
		} else if dshTakeOver(lock) {
			continue
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("DeepSeek Harness is saving its settings right now (%s is locked). Try again in a moment", lock)
		}
		time.Sleep(delay)
		if delay *= 2; delay > 200*time.Millisecond {
			delay = 200 * time.Millisecond
		}
	}
	defer os.Remove(lock)
	return fn()
}

func dshLockContended(err error, lock string) bool {
	if errors.Is(err, os.ErrExist) {
		return true
	}
	if !errors.Is(err, os.ErrPermission) {
		return false
	}
	_, lerr := os.Lstat(lock)
	return lerr == nil
}

// dshHolderGone reports whether a lock record names a process that is
// proven not to run any more.
func dshHolderGone(record string) bool {
	if !dshLockRecord.MatchString(record) {
		return false
	}
	pid, err := strconv.Atoi(record[:len(record)-1])
	if err != nil || pid == 0 || pid > 2147483647 || pid == os.Getpid() {
		return false
	}
	alive, err := process.PidExists(int32(pid))
	return err == nil && !alive
}

// dshTakeOver removes a lock whose holder has exited, the way dsh does it.
func dshTakeOver(lock string) bool {
	b, err := os.ReadFile(lock)
	if err != nil || !dshHolderGone(string(b)) {
		return false
	}
	sum := sha256.Sum256(b)
	claim := lock + ".takeover-" + hex.EncodeToString(sum[:])[:16]
	f, err := os.OpenFile(claim, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return false
	}
	f.WriteString(strconv.Itoa(os.Getpid()) + "\n")
	f.Close()
	defer os.Remove(claim)
	if again, err := os.ReadFile(lock); err != nil || string(again) != string(b) || !dshHolderGone(string(again)) {
		return false
	}
	return os.Remove(lock) == nil
}
