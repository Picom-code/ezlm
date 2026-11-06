package integrate

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"
)

// WriteFileAtomic writes data to path through a temp file in the same
// folder, synced and then renamed over the original, so a crash or a full
// disk never leaves another app's config half-written. It creates the folder.
// A path that is a symlink (dotfiles kept in a repo) is written through: the
// link stays and its target gets the new content.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	path = realPath(path)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, filepath.Base(path)+".ezlm-tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // gone after a successful rename; cleans up otherwise
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, perm); err != nil {
		return err
	}
	return renameFile(tmp, path)
}

// renameFile replaces dst with src. On Windows a rename fails while another
// program has dst open for a moment (an editor, a virus scanner, the app
// reading its config), so it retries for about two seconds. Tests swap it
// to inject a failure.
var renameFile = func(src, dst string) error {
	var err error
	for i := 0; i < 20; i++ {
		if err = os.Rename(src, dst); err == nil || runtime.GOOS != "windows" {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
	return err
}

// realPath follows a symlink at path to the file it points at (relative
// links resolve against the link's folder). Anything else is returned as is.
func realPath(path string) string {
	for i := 0; i < 8; i++ {
		st, err := os.Lstat(path)
		if err != nil || st.Mode()&os.ModeSymlink == 0 {
			return path
		}
		to, err := os.Readlink(path)
		if err != nil {
			return path
		}
		if !filepath.IsAbs(to) {
			to = filepath.Join(filepath.Dir(path), to)
		}
		path = to
	}
	return path
}

// Backup copies path to path.ezlm-backup-<unix nanos> before an edit. It
// keeps the oldest backup (the file as it was before EZLM touched it) and
// the newest three, so repeated model switches don't litter the folder. A
// missing file needs no backup and gives "".
func Backup(path string) (string, error) {
	path = realPath(path)
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	dst := fmt.Sprintf("%s.ezlm-backup-%d", path, time.Now().UnixNano())
	if err := os.WriteFile(dst, b, 0o600); err != nil {
		return "", err
	}
	if old, _ := filepath.Glob(path + ".ezlm-backup-*"); len(old) > 4 {
		sort.Strings(old)
		for _, f := range old[1 : len(old)-3] {
			os.Remove(f)
		}
	}
	return dst, nil
}

// WriteConfig backs path up, then writes data atomically. Re-read and merge
// the file right before calling it; never write from a stale copy.
func WriteConfig(path string, data []byte, perm os.FileMode) error {
	if _, err := Backup(path); err != nil {
		return err
	}
	return WriteFileAtomic(path, data, perm)
}
