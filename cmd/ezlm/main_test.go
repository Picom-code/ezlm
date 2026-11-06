package main

import (
	"os"
	"slices"
	"testing"
)

// `ezlm connect opencode --default` connects OpenCode and makes it the
// default: flags are never taken as app ids.
func TestConnectArgs(t *testing.T) {
	old := os.Args
	defer func() { os.Args = old }()
	os.Args = []string{"ezlm", "connect", "opencode", "--default", "dsh"}
	if got := ids(os.Args[2:]); !slices.Equal(got, []string{"opencode", "dsh"}) {
		t.Fatalf("ids = %v", got)
	}
	if !hasFlag("--default") || hasFlag("--json") {
		t.Fatal("hasFlag")
	}
	if got := ids([]string{"--json"}); len(got) != 0 {
		t.Fatalf("ids = %v", got)
	}
}
