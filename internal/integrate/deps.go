package integrate

// Modules the workstreams build on, imported here so go mod tidy keeps them
// in go.mod while their first users are being written: hujson edits OpenCode's
// JSONC config with comments intact, pty runs the TUI smoke test.
import (
	_ "github.com/creack/pty"
	_ "github.com/tailscale/hujson"
)
