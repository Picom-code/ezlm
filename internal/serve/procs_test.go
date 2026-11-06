package serve

import (
	"path/filepath"
	"testing"
)

// EZLM may only ever stop its own server. These cases mirror real machines:
// OpenBot's server, a hand-tuned server, other apps' copies of llama-server.
func TestIsOwnExe(t *testing.T) {
	home := filepath.FromSlash("/Users/p/.ezlm")
	cases := map[string]bool{
		"/Users/p/.ezlm/bin/llama-b11960/llama-server":      true,
		"/Users/p/.ezlm/bin/ezlm":                           true,
		"/Users/p/llama.cpp/llama-server":                   false, // user's own build
		"/Users/p/.ezlm-old/bin/llama-server":               false, // look-alike folder
		"/Users/p/.ezlmx/llama-server":                      false,
		"/Applications/LM Studio.app/Contents/llama-server": false,
		"/Users/p/.ezlm":                                    false, // the folder itself is not a program inside it
	}
	for exe, want := range cases {
		if got := isOwnExe(filepath.FromSlash(exe), home); got != want {
			t.Errorf("isOwnExe(%q) = %v, want %v", exe, got, want)
		}
	}
}
