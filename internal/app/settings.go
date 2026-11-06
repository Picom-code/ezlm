package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/picom-code/ezlm/internal/paths"
	"github.com/picom-code/ezlm/internal/scan"
	"github.com/picom-code/ezlm/internal/serve"
	"github.com/picom-code/ezlm/internal/update"
	"github.com/picom-code/ezlm/internal/version"
	"github.com/shirou/gopsutil/v4/disk"
)

// MARK: updates

var (
	updMu    sync.Mutex
	updCache update.Info
)

// UpdateInfo says whether a newer EZLM is out (cached for an hour unless forced).
func (a *App) UpdateInfo(ctx context.Context, force bool) update.Info {
	updMu.Lock()
	defer updMu.Unlock()
	if force || updCache.Checked.IsZero() || time.Since(updCache.Checked) > time.Hour || updCache.Error != "" {
		updCache = update.Check(ctx, version.Version)
	}
	return updCache
}

// DownloadUpdate fetches the new installer (checked against GitHub's
// SHA-256) as the "update" job; its "download" ok event carries the file path.
func (a *App) DownloadUpdate() error {
	info := a.UpdateInfo(context.Background(), false)
	if !info.Available || info.Latest == nil {
		return errors.New("EZLM is up to date")
	}
	r := *info.Latest
	return a.start("update", func(ctx context.Context, step func(string, string, string)) error {
		step("download", "run", fmt.Sprintf("Downloading EZLM %s", r.Version))
		path, err := update.Download(ctx, &r, a.progress("update", "download"))
		if err != nil {
			step("download", "fail", err.Error())
			return err
		}
		step("download", "ok", path)
		return nil
	})
}

// MARK: model storage

// Drive is a place models can go.
type Drive struct {
	Path    string `json:"path"`
	Label   string `json:"label"`
	Free    uint64 `json:"free"`
	Total   uint64 `json:"total"`
	Current bool   `json:"current"`
}

type StorageInfo struct {
	Dir         string  `json:"dir"`          // where new downloads go
	Custom      bool    `json:"custom"`       // chosen in Settings (not EZLM's pick)
	Free        uint64  `json:"free"`         // free space there
	ModelsBytes uint64  `json:"models_bytes"` // EZLM models on this computer
	Elsewhere   uint64  `json:"elsewhere"`    // of which outside Dir (what "move" would move)
	Drives      []Drive `json:"drives"`
}

// Storage describes where models go and the drives they could go to.
func (a *App) Storage(ctx context.Context) StorageInfo {
	hw, _ := a.hardware(ctx)
	info := StorageInfo{Dir: hw.ModelsDir, Custom: paths.LoadSettings().ModelsDir != "", Free: hw.DiskFree}
	for _, f := range modelFiles(hw) {
		info.ModelsBytes += f.size
		if !within(f.path, info.Dir) {
			info.Elsewhere += f.size
		}
	}
	info.Drives = drives(ctx, info.Dir)
	return info
}

// StorageDir is the folder EZLM uses inside the one the user picked: always
// an "EZLM Models" folder, so removing EZLM can only ever delete its own files.
func StorageDir(picked string) string {
	picked = filepath.Clean(picked)
	if strings.Contains(strings.ToLower(filepath.Base(picked)), "ezlm") {
		return picked
	}
	return filepath.Join(picked, "EZLM Models")
}

// SetStorage makes new downloads go to the folder picked (and, with move,
// moves the models already downloaded there too) as the "storage" job.
func (a *App) SetStorage(picked string, move bool) error {
	if strings.TrimSpace(picked) == "" {
		return errors.New("pick a folder or drive")
	}
	dir := StorageDir(picked)
	if err := checkFolder(dir); err != nil {
		return err
	}
	return a.start("storage", func(ctx context.Context, step func(string, string, string)) error {
		hw := scan.Scan(ctx)
		st := paths.LoadSettings()
		st.ModelsDir = dir
		if err := paths.SaveSettings(st); err != nil {
			return err
		}
		paths.RememberModelsDir(dir)
		if !move {
			step("storage", "ok", "New downloads will go to "+dir)
			return nil
		}
		var todo []modelFile
		var total uint64
		for _, f := range modelFiles(hw) {
			if !within(f.path, dir) {
				todo = append(todo, f)
				total += f.size
			}
		}
		if len(todo) == 0 {
			step("storage", "ok", "Your models are already there")
			return nil
		}
		if free := scan.FreeAt(ctx, dir); !sameVolume(todo[0].path, dir) && free < total+(1<<30) {
			return fmt.Errorf("moving your models needs %.0f GB and %s has %.0f GB free", float64(total)/(1<<30)+1, filepath.VolumeName(dir)+string(filepath.Separator), float64(free)/(1<<30))
		}
		cfg, _ := serve.Load()
		running := cfg != nil && len(serve.OwnPIDs(ctx)) > 0
		usesMoved := func() bool {
			if cfg == nil {
				return false
			}
			for _, f := range todo {
				if f.path == filepath.Clean(cfg.ModelPath) || f.path == filepath.Clean(cfg.DraftPath) || sameSplitSet(f.path, cfg.ModelPath) {
					return true
				}
			}
			return false
		}()
		if running && usesMoved {
			step("storage", "run", "Pausing your model while its file moves")
			if err := serve.StopAndWait(cfg); err != nil {
				return err
			}
		}
		var done uint64
		moved := map[string]string{}
		prog := a.progress("storage", "move")
		var moveErr error
		for _, f := range todo {
			dst := filepath.Join(dir, filepath.Base(filepath.Dir(f.path)), filepath.Base(f.path))
			step("storage", "run", "Moving "+filepath.Base(f.path))
			if err := moveFile(ctx, f.path, dst, func(n uint64) { prog(done+n, total, 0) }); err != nil {
				moveErr = err
				break
			}
			moved[f.path] = dst
			done += f.size
			os.Remove(filepath.Dir(f.path)) // the old model folder, once empty
		}
		if cfg != nil {
			if p, ok := moved[filepath.Clean(cfg.ModelPath)]; ok {
				cfg.ModelPath = p
			}
			if p, ok := moved[filepath.Clean(cfg.DraftPath)]; ok {
				cfg.DraftPath = p
			}
			if len(moved) > 0 {
				cfg.ModelsDir = dir
			}
			cfg.Save()
		}
		if running && usesMoved {
			step("storage", "run", "Starting your model again from its new place")
			if err := a.StartSync(ctx, cfg, nil); err != nil {
				return err
			}
		}
		if moveErr != nil {
			return fmt.Errorf("moved %d of %d files; the rest stayed where they were: %v", len(moved), len(todo), moveErr)
		}
		step("storage", "ok", fmt.Sprintf("Moved %.1f GB to %s", float64(total)/(1<<30), dir))
		a.mu.Lock()
		a.hwAt = time.Time{}
		a.mu.Unlock()
		return nil
	})
}

type modelFile struct {
	path string
	size uint64
}

// modelFiles lists EZLM's model files in every folder it has used.
func modelFiles(hw *scan.Hardware) []modelFile {
	var out []modelFile
	seen := map[string]bool{}
	for _, root := range modelDirs(hw) {
		filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".gguf") {
				return nil
			}
			k := strings.ToLower(filepath.Clean(p))
			if seen[k] {
				return nil
			}
			seen[k] = true
			if info, err := d.Info(); err == nil {
				out = append(out, modelFile{filepath.Clean(p), uint64(info.Size())})
			}
			return nil
		})
	}
	return out
}

func within(path, dir string) bool {
	p, d := strings.ToLower(filepath.Clean(path)), strings.ToLower(filepath.Clean(dir))
	sep := string(filepath.Separator)
	if !strings.HasSuffix(d, sep) { // a drive root (D:\) already ends with one
		d += sep
	}
	return p != strings.TrimSuffix(d, sep) && strings.HasPrefix(p, d)
}

// inDir reports whether path is dir itself or inside it, by whole folder
// names: /Volumes/T7 doesn't contain /Volumes/T7 Backup.
func inDir(path, dir string) bool {
	return strings.EqualFold(filepath.Clean(path), filepath.Clean(dir)) || within(path, dir)
}

func sameVolume(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.VolumeName(a), filepath.VolumeName(b))
	}
	return false // can't tell cheaply: copy, which always works
}

// checkFolder makes sure EZLM can create and write the folder.
func checkFolder(dir string) error {
	if runtime.GOOS == "windows" {
		if root := strings.ToLower(os.Getenv("SystemRoot")); root != "" && strings.HasPrefix(strings.ToLower(dir), root) {
			return errors.New("pick a folder outside the Windows folder")
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("can't use %s: %v", dir, err)
	}
	probe := filepath.Join(dir, ".ezlm-write-test")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		return fmt.Errorf("EZLM can't save files in %s: %v", dir, err)
	}
	os.Remove(probe)
	return nil
}

// moveFile renames when it can, else copies (with progress) then deletes.
func moveFile(ctx context.Context, src, dst string, prog func(uint64)) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Rename(src, dst); err == nil {
		if st, err := os.Stat(dst); err == nil {
			prog(uint64(st.Size()))
		}
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	buf := make([]byte, 8<<20)
	var n uint64
	for {
		if ctx.Err() != nil {
			out.Close()
			os.Remove(tmp)
			return ctx.Err()
		}
		r, rerr := in.Read(buf)
		if r > 0 {
			if _, werr := out.Write(buf[:r]); werr != nil {
				out.Close()
				os.Remove(tmp)
				return werr
			}
			n += uint64(r)
			prog(n)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			out.Close()
			os.Remove(tmp)
			return rerr
		}
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return err
	}
	in.Close()
	return os.Remove(src)
}

// drives lists where models could go, roomiest first.
func drives(ctx context.Context, current string) []Drive {
	var out []Drive
	add := func(path, label string) {
		du, err := disk.UsageWithContext(ctx, path)
		if err != nil || du.Total < 8<<30 {
			return
		}
		out = append(out, Drive{Path: path, Label: label, Free: du.Free, Total: du.Total,
			Current: inDir(current, path)})
	}
	switch runtime.GOOS {
	case "windows":
		parts, _ := disk.PartitionsWithContext(ctx, false)
		for _, p := range parts {
			if strings.Contains(strings.ToLower(strings.Join(p.Opts, ",")), "cdrom") {
				continue
			}
			root := p.Mountpoint
			if !strings.HasSuffix(root, `\`) {
				root += `\`
			}
			add(root, volumeLabel(root))
		}
	case "darwin":
		home, _ := os.UserHomeDir()
		add(home, "This Mac")
		ents, _ := os.ReadDir("/Volumes")
		for _, e := range ents {
			p := filepath.Join("/Volumes", e.Name())
			if fi, err := os.Lstat(p); err == nil && fi.Mode()&os.ModeSymlink == 0 && e.Name() != "Recovery" {
				add(p, e.Name())
			}
		}
	default:
		home, _ := os.UserHomeDir()
		add(home, "Home")
		for _, base := range []string{"/mnt", "/media"} {
			ents, _ := os.ReadDir(base)
			for _, e := range ents {
				add(filepath.Join(base, e.Name()), e.Name())
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Free > out[j].Free })
	return out
}
