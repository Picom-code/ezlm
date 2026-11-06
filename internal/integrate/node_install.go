package integrate

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

// nodeInstall puts EZLM's pinned Node.js into ToolDir("node", <version>) and
// returns its node program. The archive is checked against its pinned
// SHA-256 before anything is unpacked, and the folder only appears once it
// is complete, so a failed install leaves nothing half-made.
func nodeInstall(ctx context.Context, r *Run) (string, error) {
	pin, ok := PinFor("node")
	if !ok {
		return "", errors.New("EZLM has no Node.js version to install")
	}
	if bin := nodeEZLM(); bin != "" {
		return bin, nil
	}
	key := AssetKey(runtime.GOOS, MachineArch(), false)
	a, ok := pin.Assets[key]
	if !ok {
		return "", fmt.Errorf("EZLM has no Node.js build for this computer (%s). Install Node.js %s or newer yourself, then press Connect again", key, dshNodeMin)
	}
	if err := CheckDisk(ToolsDir(), 300<<20, "Node.js"); err != nil {
		return "", err
	}
	r.Step(fmt.Sprintf("Installing Node.js %s for DeepSeek Harness…", pin.Version))
	parent := filepath.Join(ToolsDir(), "node")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return "", err
	}
	archive := filepath.Join(parent, ".download-"+path.Base(a.URL))
	defer os.Remove(archive)
	if err := FetchAsset(ctx, r, a, archive); err != nil {
		return "", err
	}
	r.Logf("checksum ok: %s", a.SHA256)
	tmp, err := os.MkdirTemp(parent, ".unpack-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	if err := nodeExtract(archive, tmp); err != nil {
		r.Logf("unpack %s: %v", archive, err)
		return "", fmt.Errorf("EZLM couldn't unpack Node.js: %v. Press Connect to try again", err)
	}
	dst := ToolDir("node", pin.Version)
	os.RemoveAll(dst)
	if err := os.Rename(tmp, dst); err != nil {
		return "", err
	}
	bin := nodeBinIn(dst)
	if _, err := os.Stat(bin); err != nil {
		os.RemoveAll(dst)
		return "", fmt.Errorf("the Node.js download had no %s. Press Connect to try again", filepath.Base(bin))
	}
	r.Logf("installed node %s at %s", pin.Version, bin)
	return bin, nil
}

// nodeExtract unpacks a Node.js .zip, .tar.gz or .tar.xz into dst, dropping
// the archive's top folder (node-v24.x.y-<platform>/). It refuses entries
// that would land outside dst.
func nodeExtract(archive, dst string) error {
	switch {
	case strings.HasSuffix(archive, ".zip"):
		return nodeUnzip(archive, dst)
	case strings.HasSuffix(archive, ".tar.gz"), strings.HasSuffix(archive, ".tgz"):
		f, err := os.Open(archive)
		if err != nil {
			return err
		}
		defer f.Close()
		gz, err := gzip.NewReader(f)
		if err != nil {
			return err
		}
		return nodeUntar(tar.NewReader(gz), dst)
	case strings.HasSuffix(archive, ".tar.xz"):
		// Go has no xz reader; every Mac and Linux has a tar that does.
		out, err := exec.Command("tar", "-xJf", archive, "-C", dst, "--strip-components", "1").CombinedOutput()
		if err != nil {
			return fmt.Errorf("tar: %v: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	return fmt.Errorf("unknown archive type %s", filepath.Base(archive))
}

// nodeTarget is where an archive entry goes: the name without its first
// folder, under dst. ok is false for the top folder itself and for names
// that would escape dst.
func nodeTarget(dst, name string) (string, bool) {
	name = strings.TrimPrefix(path.Clean("/"+strings.ReplaceAll(name, `\`, "/")), "/")
	i := strings.IndexByte(name, '/')
	if i < 0 {
		return "", false
	}
	rel := name[i+1:]
	if rel == "" || rel == "." {
		return "", false
	}
	out := filepath.Join(dst, filepath.FromSlash(rel))
	if r, err := filepath.Rel(dst, out); err != nil || strings.HasPrefix(r, "..") {
		return "", false
	}
	return out, true
}

func nodeUnzip(archive, dst string) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, f := range zr.File {
		out, ok := nodeTarget(dst, f.Name)
		if !ok {
			continue
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(out, 0o755); err != nil {
				return err
			}
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		err = nodeWriteFile(out, rc, f.Mode().Perm()|0o600)
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func nodeUntar(tr *tar.Reader, dst string) error {
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		out, ok := nodeTarget(dst, h.Name)
		if !ok {
			continue
		}
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(out, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := nodeWriteFile(out, tr, os.FileMode(h.Mode).Perm()|0o600); err != nil {
				return err
			}
		case tar.TypeSymlink:
			// bin/npm -> ../lib/node_modules/npm/bin/npm-cli.js and the like:
			// relative links that stay inside the folder.
			target := filepath.Join(filepath.Dir(out), filepath.FromSlash(h.Linkname))
			if filepath.IsAbs(h.Linkname) {
				continue
			}
			if r, err := filepath.Rel(dst, target); err != nil || strings.HasPrefix(r, "..") {
				continue
			}
			if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
				return err
			}
			os.Remove(out)
			if err := os.Symlink(h.Linkname, out); err != nil {
				return err
			}
		}
	}
}

func nodeWriteFile(out string, r io.Reader, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
