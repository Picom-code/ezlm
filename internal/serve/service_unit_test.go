package serve

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestQuote(t *testing.T) {
	cases := map[string]string{
		"":                  `""`,
		"plain":             `"plain"`,
		"/a b/llama-server": `"/a b/llama-server"`,
		`a"b`:               `"a\"b"`,
		`a\b`:               `"a\\b"`,
		"100%":              `"100%%"`,
		"100%h":             `"100%%h"`,
	}
	for in, want := range cases {
		if got := quote(in); got != want {
			t.Errorf("quote(%q) = %s, want %s", in, got, want)
		}
	}
}

// The unit content is testable on any OS: only the file it is written to is
// Linux-specific.
func TestUnitFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("EZLM_HOME", filepath.Join(tmp, "100% real", ".ezlm"))
	c := &Config{
		Server:    filepath.Join(tmp, "home dir", ".ezlm", "bin", "llama-b11960", "llama-server"),
		ModelPath: filepath.Join(tmp, "models", "Qwen 2.5 7B 100% real.gguf"),
		ModelID:   "qwen2.5-7b",
		Port:      8080,
		Ctx:       4096,
		NGL:       99,
		Keys:      []APIKey{{ID: "k1", Name: "Default", Key: "REDACTED-KEY"}},
	}
	unit := unitFile(c)

	if !strings.HasSuffix(unit, "\n") {
		t.Fatal("unit does not end with a newline")
	}
	for _, want := range []string{
		"[Unit]\nDescription=EZLM local LLM server\n",
		"After=network-online.target\nWants=network-online.target\n",
		"[Service]\n",
		"Restart=on-failure\n",
		"RestartSec=10\n",
		"[Install]\nWantedBy=default.target\n",
	} {
		if !strings.Contains(unit, want) {
			t.Errorf("unit is missing %q:\n%s", want, unit)
		}
	}

	var execStart, envLine, stdoutLine, stderrLine string
	for _, line := range strings.Split(unit, "\n") {
		switch {
		case strings.HasPrefix(line, "ExecStart="):
			execStart = line
		case strings.HasPrefix(line, "Environment="):
			envLine = line
		case strings.HasPrefix(line, "StandardOutput="):
			stdoutLine = line
		case strings.HasPrefix(line, "StandardError="):
			stderrLine = line
		}
	}

	// Every argument is one quoted word, so a path with a space, a quote, a
	// backslash or a % (which systemd would expand as a specifier) survives.
	args := append([]string{c.Server}, c.Args()...)
	for _, a := range args {
		if !strings.Contains(execStart, quote(a)) {
			t.Errorf("ExecStart is missing the quoted argument %q:\n%s", a, execStart)
		}
	}
	if strings.Contains(execStart, "100% ") || !strings.Contains(execStart, "100%% real") {
		t.Errorf("ExecStart does not escape %%:\n%s", execStart)
	}

	// Environment= is a whitespace-separated list, so the assignment must be
	// quoted to keep a path with a space in it as one value.
	if !strings.HasPrefix(envLine, `Environment="LD_LIBRARY_PATH=`) || !strings.HasSuffix(envLine, `"`) {
		t.Errorf("Environment= value is not quoted:\n%s", envLine)
	}
	if !strings.Contains(envLine, "home dir") {
		t.Errorf("Environment= lost the space in the install path:\n%s", envLine)
	}

	// A literal % in the log path would be read as a specifier.
	logPath := unitEscape(LogPath())
	if logPath == LogPath() || !strings.Contains(logPath, "100%% real") {
		t.Fatalf("log path was not escaped: %q", logPath)
	}
	for _, line := range []string{stdoutLine, stderrLine} {
		if line != "StandardOutput=append:"+logPath && line != "StandardError=append:"+logPath {
			t.Errorf("log line = %q, want append:%q", line, logPath)
		}
	}
}

func TestUnitName(t *testing.T) {
	t.Setenv("EZLM_HOME", "")
	if got := unitName(); got != "ezlm.service" {
		t.Errorf("real install unit = %q, want ezlm.service", got)
	}

	t.Setenv("EZLM_HOME", t.TempDir())
	first := unitName()
	if !regexp.MustCompile(`^ezlm-test-[0-9a-f]{8}\.service$`).MatchString(first) {
		t.Fatalf("test instance unit = %q, want ezlm-test-<8 hex>.service", first)
	}
	if unitName() != first {
		t.Error("the same EZLM_HOME produced two unit names")
	}

	t.Setenv("EZLM_HOME", t.TempDir())
	if other := unitName(); other == first {
		t.Errorf("two EZLM_HOME values share the unit name %q", first)
	}
}

func TestUnitPathIn(t *testing.T) {
	t.Setenv("EZLM_HOME", "")
	got := unitPathIn(filepath.FromSlash("/home/ann/.config"))
	want := filepath.FromSlash("/home/ann/.config/systemd/user/ezlm.service")
	if got != want {
		t.Errorf("unitPathIn = %q, want %q", got, want)
	}
}

// libPath must stay inside the engine install. The old loop walked up past
// bin/ for a flat install (llama-server straight in bin), scanning the whole
// file system.
func TestLibPathStaysInTheEngineInstall(t *testing.T) {
	write := func(p string) {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("versioned install", func(t *testing.T) {
		tmp := t.TempDir()
		server := filepath.Join(tmp, "bin", "llama-b11960", "llama-server")
		write(server)
		write(filepath.Join(tmp, "bin", "llama-b11960", "libcuda.so.1"))
		write(filepath.Join(tmp, "bin", "llama-b11960", "runtime", "libggml.so"))
		want := filepath.Join(tmp, "bin", "llama-b11960") + ":" + filepath.Join(tmp, "bin", "llama-b11960", "runtime")
		if got := libPath(server); got != want {
			t.Errorf("libPath = %q, want %q", got, want)
		}
	})

	t.Run("flat install", func(t *testing.T) {
		tmp := t.TempDir()
		server := filepath.Join(tmp, "bin", "llama-server")
		write(server)
		write(filepath.Join(tmp, "bin", "libcuda.so.1"))
		got := libPath(server)
		if got != filepath.Join(tmp, "bin") {
			t.Errorf("libPath = %q, want just %q", got, filepath.Join(tmp, "bin"))
		}
	})

	t.Run("no bin directory", func(t *testing.T) {
		tmp := t.TempDir()
		server := filepath.Join(tmp, "engine", "llama-server")
		write(server)
		write(filepath.Join(tmp, "engine", "lib", "libggml.so"))
		got := libPath(server)
		want := filepath.Join(tmp, "engine") + ":" + filepath.Join(tmp, "engine", "lib")
		if got != want {
			t.Errorf("libPath = %q, want %q", got, want)
		}
	})
}
