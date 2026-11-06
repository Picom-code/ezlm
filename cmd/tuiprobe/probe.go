package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/picom-code/ezlm/internal/integrate"
)

// options is one probe run.
type options struct {
	Cmd       string
	Args      []string
	Dir       string
	Env       []string // KEY=VALUE on top of tuiprobe's own environment
	Cols      int
	Rows      int
	Ready     string        // text that means the app is up ("" = don't wait)
	ReadyWait time.Duration // how long to wait for it
	Type      string        // text typed once ready, then Enter ("" = type nothing)
	TypeDelay time.Duration // between typed characters (0 = all at once)
	Expect    *regexp.Regexp
	Timeout   time.Duration // how long to wait for Expect after typing
	Settle    time.Duration // after Expect matched, how long the app keeps drawing before the exit keys
	Exit      string        // keys sent at the end, e.g. "/exit{enter}" or "{ctrl-c}{wait}{ctrl-c}"
	ExitWait  time.Duration // how long to wait for the app to end after them
	WantCode  int           // the exit code the app must end with; -1 = any
}

// report is what the probe saw. OK means every wait asked for succeeded.
type report struct {
	OK      bool    `json:"ok"`
	Fail    string  `json:"fail,omitempty"`
	Ready   bool    `json:"ready"`
	Matched bool    `json:"matched"`
	Match   string  `json:"match,omitempty"`
	Exited  bool    `json:"exited"`
	Code    int     `json:"code"`
	Pid     int     `json:"pid"`
	Secs    float64 `json:"secs"`
	Screen  string  `json:"screen"` // everything drawn, ANSI stripped
}

// probeKeys turns key syntax into what to write: plain text, and {enter}
// {tab} {esc} {backspace} {up} {down} {left} {right} {ctrl-<letter>},
// {wait} (half a second) or {wait:2s}. An empty string in the result is a
// pause; the matching duration is in the second slice.
func probeKeys(s string) ([]string, []time.Duration, error) {
	named := map[string]string{
		"enter": "\r", "tab": "\t", "esc": "\x1b", "backspace": "\x7f", "space": " ",
		"up": "\x1b[A", "down": "\x1b[B", "right": "\x1b[C", "left": "\x1b[D",
		"lbrace": "{", "rbrace": "}",
	}
	var out []string
	var waits []time.Duration
	add := func(text string, d time.Duration) {
		out = append(out, text)
		waits = append(waits, d)
	}
	for s != "" {
		i := strings.IndexByte(s, '{')
		if i < 0 {
			add(s, 0)
			break
		}
		if i > 0 {
			add(s[:i], 0)
		}
		j := strings.IndexByte(s[i:], '}')
		if j < 0 {
			return nil, nil, fmt.Errorf("unclosed { in %q", s)
		}
		tok := strings.ToLower(s[i+1 : i+j])
		s = s[i+j+1:]
		switch {
		case named[tok] != "":
			add(named[tok], 0)
		case tok == "wait":
			add("", 500*time.Millisecond)
		case strings.HasPrefix(tok, "wait:"):
			d, err := time.ParseDuration(tok[5:])
			if err != nil {
				return nil, nil, fmt.Errorf("bad {%s}: %v", tok, err)
			}
			add("", d)
		case strings.HasPrefix(tok, "ctrl-") && len(tok) == 6 && tok[5] >= 'a' && tok[5] <= 'z':
			add(string(rune(tok[5]-'a'+1)), 0)
		default:
			return nil, nil, fmt.Errorf("unknown key {%s}", tok)
		}
	}
	return out, waits, nil
}

// probe runs o.Cmd in a hidden pseudo-terminal and drives it. It stops only
// the process tree it started.
func probe(o options) report {
	start := time.Now()
	var r report
	r.Code = -1
	var mu sync.Mutex
	var raw strings.Builder
	text := func(from int) string {
		mu.Lock()
		defer mu.Unlock()
		return integrate.StripANSI(raw.String()[from:])
	}
	finish := func() report {
		r.Secs = time.Since(start).Seconds()
		r.Screen = text(0)
		return r
	}
	exitKeys, exitWaits, err := probeKeys(o.Exit)
	if err != nil {
		r.Fail = err.Error()
		return finish()
	}
	cmd := exec.Command(o.Cmd, o.Args...)
	cmd.Dir = o.Dir
	cmd.Env = append(os.Environ(), o.Env...)
	p, err := integrate.StartPTY(cmd, o.Cols, o.Rows)
	if err != nil {
		r.Fail = "start: " + err.Error()
		return finish()
	}
	defer p.Close()
	r.Pid = p.Pid()
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		buf := make([]byte, 8192)
		for {
			n, err := p.Read(buf)
			mu.Lock()
			raw.Write(buf[:n])
			mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	since := 0 // Expect only looks at what was drawn after typing
	waitFor := func(ok func() bool, d time.Duration) bool {
		for end := time.Now().Add(d); ; time.Sleep(100 * time.Millisecond) {
			if ok() {
				return true
			}
			if p.Exited() {
				select {
				case <-drained:
				case <-time.After(2 * time.Second):
				}
				return ok()
			}
			if time.Now().After(end) {
				return false
			}
		}
	}
	if o.Ready != "" {
		r.Ready = waitFor(func() bool { return strings.Contains(text(0), o.Ready) }, o.ReadyWait)
		if !r.Ready {
			r.Fail = fmt.Sprintf("%q did not appear within %s", o.Ready, o.ReadyWait)
			return probeEnd(p, &r, finish)
		}
	}
	if o.Type != "" {
		mu.Lock()
		since = raw.Len()
		mu.Unlock()
		if o.TypeDelay > 0 {
			for _, c := range o.Type {
				p.Write([]byte(string(c)))
				time.Sleep(o.TypeDelay)
			}
		} else {
			p.Write([]byte(o.Type))
		}
		// Enter on its own, so a TUI doesn't take it as part of a paste.
		time.Sleep(150 * time.Millisecond)
		p.Write([]byte("\r"))
	}
	if o.Expect != nil {
		r.Matched = waitFor(func() bool {
			if m := o.Expect.FindString(text(since)); m != "" {
				r.Match = m
				return true
			}
			return false
		}, o.Timeout)
		if !r.Matched {
			r.Fail = fmt.Sprintf("%s did not appear within %s", o.Expect, o.Timeout)
			return probeEnd(p, &r, finish)
		}
		// The answer may still be streaming (the match can be a tool's
		// output): let it finish drawing.
		waitFor(func() bool { return false }, o.Settle)
	}
	if o.Exit != "" {
		for i, k := range exitKeys {
			if k == "" {
				time.Sleep(exitWaits[i])
				continue
			}
			p.Write([]byte(k))
		}
		if !waitFor(p.Exited, o.ExitWait) {
			r.Fail = fmt.Sprintf("the app was still running %s after the exit keys", o.ExitWait)
			return probeEnd(p, &r, finish)
		}
	}
	probeEnd(p, &r, finish)
	if o.WantCode >= 0 && (!r.Exited || r.Code != o.WantCode) {
		r.Fail = fmt.Sprintf("the app ended with code %d, want %d", r.Code, o.WantCode)
		if !r.Exited {
			r.Fail = "the app was still running, want exit code " + strconv.Itoa(o.WantCode)
		}
		return finish()
	}
	r.OK = r.Fail == ""
	return finish()
}

// probeEnd records whether the app has ended and with which code.
func probeEnd(p *integrate.PTY, r *report, finish func() report) report {
	if p.Exited() {
		r.Exited = true
		r.Code, _ = p.Wait()
	}
	return finish()
}
