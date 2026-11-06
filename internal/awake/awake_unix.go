//go:build darwin || linux

package awake

import "sync"

// hold starts the platform helper built by holdCmd (see awake_darwin.go and
// awake_linux.go) and returns a release that kills it.
func hold(why string) func() {
	cmd := holdCmd(why)
	if cmd == nil {
		return func() {}
	}
	if cmd.Start() != nil {
		return func() {}
	}
	done := make(chan struct{})
	go func() { cmd.Wait(); close(done) }()
	var once sync.Once
	return func() { once.Do(func() { cmd.Process.Kill(); <-done }) }
}
