package engine

import (
	"io"
	"os"
)

// partFile is the part of *os.File that fetch needs. Tests swap openPart to
// fake a full disk partway through a write.
type partFile interface {
	io.Writer
	io.Closer
}

var openPart = func(name string, flags int, perm os.FileMode) (partFile, error) {
	return os.OpenFile(name, flags, perm)
}
