package engine_test

// This file studies the llama.cpp flags EZLM's serve package builds
// (internal/serve/config.go). It lives here because T06 owns internal/engine
// and reads internal/serve; the builder itself is not ours to change.
//
// Checked against llama.cpp b11960 and b12110 (the build v0.9.0 pins) and
// b12144: --jinja, -np, --cache-ram and --kv-unified kept the same names,
// defaults and meanings across all three. See agents/T06/a09/flags.md.

import (
	"slices"
	"strings"
	"testing"

	"github.com/picom-code/ezlm/internal/serve"
)

// baseConfig is a deployed setup like the one app.go writes.
func baseConfig() *serve.Config {
	return &serve.Config{
		Key:       "REDACTED-KEY",
		Port:      8080,
		ModelPath: "/models/test.gguf",
		ModelID:   "test-model",
		Ctx:       32768,
		NGL:       99,
	}
}

// countFlags returns every index at which any of names appears in args.
func countFlags(args []string, names ...string) []int {
	var at []int
	for i, a := range args {
		if slices.Contains(names, a) {
			at = append(at, i)
		}
	}
	return at
}

// TestArgsCoreFlags pins the four flags studied for the b11960 -> v0.9.0
// upgrade. None of them changed, so EZLM must keep asking for the same
// command line: exactly one slot that owns the whole context, and jinja
// templates on.
func TestArgsCoreFlags(t *testing.T) {
	cfg := baseConfig()
	args := cfg.Args()

	// --jinja: llama.cpp already defaults it on (common/common.h use_jinja),
	// so this is belt and braces. EZLM relies on jinja rendering for tool
	// calls and for the reasoning levels it reads from /props, so it keeps
	// passing the flag rather than trusting the default to stay on.
	if got := countFlags(args, "--jinja"); len(got) != 1 {
		t.Fatalf("--jinja appears %d times in %v, want exactly once", len(got), args)
	}
	if got := countFlags(args, "--no-jinja"); len(got) != 0 {
		t.Fatalf("--no-jinja also passed: %v", args)
	}

	// -np 1: the server default is -1, which means auto and turns into
	// four slots with a unified KV cache (tools/server/server.cpp). One slot
	// is what gives the single user the whole -c and keeps kv_unified off.
	np := countFlags(args, "-np", "--parallel")
	if len(np) != 1 {
		t.Fatalf("-np/--parallel appears %d times in %v, want exactly once", len(np), args)
	}
	if i := np[0]; i+1 >= len(args) || args[i+1] != "1" {
		t.Fatalf("-np is not followed by 1: %v", args)
	}

	// -c is the full per-slot context. llama.cpp divides n_ctx over slots
	// only when they are not unified; with one slot the two agree, but the
	// value EZLM passes must stay the configured context.
	ci := countFlags(args, "-c")
	if len(ci) != 1 || ci[0]+1 >= len(args) || args[ci[0]+1] != "32768" {
		t.Fatalf("-c is not the configured context: %v", args)
	}

	// The prompt-cache flags are deliberately left to llama.cpp's defaults
	// (--cache-ram 8192 MiB, --kv-unified off with an explicit -np). If they
	// start appearing here, the value must still be a MiB count that
	// llama.cpp accepts: 0 disables, -1 means no limit.
	for _, name := range []string{"--cache-ram", "-cram"} {
		at := countFlags(args, name)
		if len(at) > 1 {
			t.Fatalf("%s passed more than once: %v", name, args)
		}
	}
	for _, name := range []string{"--kv-unified", "-kvu", "--no-kv-unified"} {
		if len(countFlags(args, name)) > 1 {
			t.Fatalf("%s passed more than once: %v", name, args)
		}
	}
}

// TestArgsRecipeFlagsLast checks that the fit engine's recipe flags come
// after the fixed ones. llama.cpp lets the last value win, so appending them
// is what lets a recipe override a default without EZLM rewriting the base
// command line.
func TestArgsRecipeFlagsLast(t *testing.T) {
	cfg := baseConfig()
	recipe := []string{"-ctk", "q8_0", "-ctv", "q8_0"}
	cfg.ExtraArgs = slices.Clone(recipe)

	args := cfg.Args()
	if len(args) < len(recipe) || !slices.Equal(args[len(args)-len(recipe):], recipe) {
		t.Fatalf("recipe flags are not last: %v", args)
	}
	if got := countFlags(args, "--jinja"); len(got) != 1 {
		t.Fatalf("--jinja appears %d times in %v", len(got), args)
	}
	if got := countFlags(args, "-np", "--parallel"); len(got) != 1 {
		t.Fatalf("-np/--parallel appears %d times in %v", len(got), args)
	}
	// No duplicate flag names among the four under study.
	seen := map[string]bool{}
	for _, a := range args {
		switch a {
		case "--jinja", "-np", "--cache-ram", "--kv-unified":
			if seen[a] {
				t.Fatalf("%s is passed twice: %v", a, args)
			}
			seen[a] = true
		}
	}
	if strings.Contains(strings.Join(args, " "), "--parallel") {
		t.Fatalf("the --parallel alias is passed: %v", args)
	}
}
