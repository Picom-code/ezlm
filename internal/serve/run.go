package serve

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// RunForeground runs llama-server from the saved config, logging to the
// server log, until it exits or this process is signalled. `ezlm run-server`
// exposes it; the Windows login entry starts the server detached instead.
func RunForeground() error {
	c, err := Load()
	if err != nil {
		return err
	}
	log, err := os.OpenFile(LogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer log.Close()
	cmd := exec.Command(c.Server, c.Args()...)
	cmd.Stdout, cmd.Stderr = log, log
	hideChild(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		cmd.Process.Kill()
	}()
	return cmd.Wait()
}
