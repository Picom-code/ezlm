package main

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

// probeLoop is a tiny line-reading "TUI": it prompts "ready> ", answers each
// line with "you said: <line>" and ends with code 7 on /exit.
func probeLoop(t *testing.T) (string, []string) {
	if runtime.GOOS == "windows" {
		p := filepath.Join(t.TempDir(), "loop.cmd")
		os.WriteFile(p, []byte("@echo off\r\n:loop\r\nset \"l=\"\r\nset /p \"l=ready> \"\r\nif \"%l%\"==\"/exit\" exit /b 7\r\necho you said: %l%\r\ngoto loop\r\n"), 0o644)
		return p, nil
	}
	return "/bin/sh", []string{"-c", `printf 'ready> '; while read l; do [ "$l" = "/exit" ] && exit 7; echo "you said: $l"; printf 'ready> '; done`}
}

func TestProbeDrivesALoop(t *testing.T) {
	cmd, args := probeLoop(t)
	r := probe(options{Cmd: cmd, Args: args, Cols: 120, Rows: 40,
		Ready: "ready>", ReadyWait: 20 * time.Second,
		Type: "hello world", TypeDelay: 5 * time.Millisecond,
		Expect: regexp.MustCompile(`you said: h\w+o world`), Timeout: 20 * time.Second,
		Exit: "/exit{enter}", ExitWait: 20 * time.Second, WantCode: 7})
	if !r.OK || !r.Ready || !r.Matched || !r.Exited || r.Code != 7 || r.Match != "you said: hello world" {
		t.Fatalf("%+v", r)
	}
	if !strings.Contains(r.Screen, "you said: hello world") || r.Pid == 0 {
		t.Errorf("screen %q", r.Screen)
	}
}

func TestProbeReportsWhatFailed(t *testing.T) {
	cmd, args := probeLoop(t)
	r := probe(options{Cmd: cmd, Args: args, Cols: 120, Rows: 40,
		Ready: "ready>", ReadyWait: 20 * time.Second, Type: "hi",
		Expect: regexp.MustCompile(`EZLM-CHECK-[A-Z2-7]{8}`), Timeout: 2 * time.Second, WantCode: -1})
	if r.OK || !r.Ready || r.Matched || r.Exited || !strings.Contains(r.Fail, "did not appear") {
		t.Fatalf("%+v", r)
	}
	r = probe(options{Cmd: cmd, Args: args, Cols: 120, Rows: 40, Ready: "never", ReadyWait: time.Second, WantCode: -1})
	if r.OK || r.Ready || !strings.Contains(r.Fail, `"never" did not appear`) {
		t.Fatalf("%+v", r)
	}
	r = probe(options{Cmd: filepath.Join(t.TempDir(), "missing"), Cols: 80, Rows: 24, WantCode: -1})
	if r.OK || !strings.HasPrefix(r.Fail, "start:") {
		t.Fatalf("%+v", r)
	}
}

func TestProbeKeys(t *testing.T) {
	keys, waits, err := probeKeys("/exit{enter}{ctrl-c}{wait}{ctrl-C}{wait:2s}{up}x{lbrace}")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/exit", "\r", "\x03", "", "\x03", "", "\x1b[A", "x", "{"}
	if strings.Join(keys, "|") != strings.Join(want, "|") || waits[3] != 500*time.Millisecond || waits[5] != 2*time.Second {
		t.Fatalf("%q %v", keys, waits)
	}
	for _, bad := range []string{"{nope}", "{enter", "{wait:x}"} {
		if _, _, err := probeKeys(bad); err == nil {
			t.Errorf("%q must fail", bad)
		}
	}
}
