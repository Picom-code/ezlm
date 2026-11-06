package app

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestInDirWholeNames(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix paths")
	}
	cases := []struct {
		path, dir  string
		in, inside bool
	}{
		{"/Volumes/T7/EZLM Models", "/Volumes/T7", true, true},
		{"/Volumes/T7 Backup/EZLM Models", "/Volumes/T7", false, false},
		{"/Volumes/T7", "/Volumes/T7", true, false},
		{"/Users/a/.ezlm/models", "/", true, true},
	}
	for _, c := range cases {
		if got := inDir(c.path, c.dir); got != c.in {
			t.Errorf("inDir(%q, %q) = %v, want %v", c.path, c.dir, got, c.in)
		}
		if got := within(filepath.Clean(c.path), c.dir); got != c.inside {
			t.Errorf("within(%q, %q) = %v, want %v", c.path, c.dir, got, c.inside)
		}
	}
}
