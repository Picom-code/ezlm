package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

func DownloadSHA(ctx context.Context, url, dst string, want uint64, sha string, prog Progress) error {
	sum, err := parseSHA(sha)
	if err != nil {
		return err
	}
	if err := Download(ctx, url, dst, want, prog); err != nil {
		return err
	}
	if sum == "" {
		return nil
	}
	ok, err := fileMatchesSHA(dst, sum)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	// Download returns a file already on disk whose size matches, so a corrupt
	// one from an earlier run would survive; fetch a clean copy once.
	os.Remove(dst)
	os.Remove(dst + ".part")
	if prog != nil {
		prog(0, want, 0)
	}
	if err := Download(ctx, url, dst, want, prog); err != nil {
		return err
	}
	if ok, err = fileMatchesSHA(dst, sum); err != nil {
		return err
	}
	if ok {
		return nil
	}
	os.Remove(dst)
	return fmt.Errorf("the downloaded file is corrupt (its checksum doesn't match), so EZLM deleted it. Try again, and if it happens again, report this problem")
}

func parseSHA(sha string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(sha))
	s = strings.TrimPrefix(s, "sha256:")
	if s == "" {
		return "", nil
	}
	if len(s) != 2*sha256.Size {
		return "", fmt.Errorf("the download's checksum is %d characters long instead of %d, so EZLM can't check the file. Report this problem", len(s), 2*sha256.Size)
	}
	if _, err := hex.DecodeString(s); err != nil {
		return "", fmt.Errorf("the download's checksum isn't hexadecimal, so EZLM can't check the file. Report this problem")
	}
	return s, nil
}

func fileMatchesSHA(path, want string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	return hex.EncodeToString(h.Sum(nil)) == want, nil
}
