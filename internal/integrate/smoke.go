package integrate

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// CommonFatal are screen texts that mean an app failed to start, whatever
// the app. LaunchSpec.Fatal adds a tool's own.
var CommonFatal = []string{
	"Failed to initialize OpenTUI",
	"Database is not empty",
	"Traceback",
	"Error:",
	"Model not found",
	"below the minimum 64,000",
	"MISSING_CREDENTIAL",
}

// StripANSI turns terminal output into plain text: escape sequences go,
// cursor moves that a TUI uses for layout become a space (moving right) or a
// line break (moving to another row), so words drawn apart stay apart, and
// carriage returns become line breaks.
func StripANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == 0x1b:
			i = smokeEscape(s, i, &b)
		case c == '\r':
			if i+1 < len(s) && s[i+1] == '\n' {
				continue
			}
			b.WriteByte('\n')
		case c == '\n' || c == '\t':
			b.WriteByte(c)
		case c < 0x20 || c == 0x7f:
			// BEL, backspace and the other control characters.
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// smokeEscape handles the escape sequence starting at s[i] and returns the
// index of its last byte.
func smokeEscape(s string, i int, b *strings.Builder) int {
	if i+1 >= len(s) {
		return i
	}
	// until returns the index of the last byte of the string terminator
	// (BEL or ESC \), or the end.
	until := func(j int, bel bool) int {
		for ; j < len(s); j++ {
			if bel && s[j] == 0x07 {
				return j
			}
			if s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\' {
				return j + 1
			}
		}
		return len(s) - 1
	}
	switch s[i+1] {
	case '[': // CSI: parameters, intermediates, one final byte
		j := i + 2
		for j < len(s) && s[j] >= 0x30 && s[j] <= 0x3f {
			j++
		}
		params := s[i+2 : j]
		for j < len(s) && s[j] >= 0x20 && s[j] <= 0x2f {
			j++
		}
		if j >= len(s) {
			return len(s) - 1
		}
		switch s[j] {
		case 'C': // cursor forward n
			n, err := strconv.Atoi(strings.SplitN(params, ";", 2)[0])
			if err != nil || n < 1 {
				n = 1
			}
			b.WriteString(strings.Repeat(" ", min(n, 300)))
		case 'G': // to a column on this row
			b.WriteByte(' ')
		case 'H', 'f', 'd', 'E', 'F': // to another row
			b.WriteByte('\n')
		}
		return j
	case ']': // OSC, ended by BEL or ESC \
		return until(i+2, true)
	case 'P', 'X', '^', '_': // DCS, SOS, PM, APC
		return until(i+2, false)
	case '(', ')', '*', '+', '-', '.', '/', '#', '%', ' ': // charset and similar: one more byte
		return min(i+2, len(s)-1)
	default:
		return i + 1
	}
}

// smokeMaxOutput caps how much output a smoke run keeps (a TUI that redraws
// for 20 s stays far below it); smokeMaxScreen is what goes into the result.
const (
	smokeMaxOutput = 8 << 20
	smokeMaxScreen = 16 << 10
)

// Smoke starts spec in a hidden 120x40 pseudo-terminal and watches it for up
// to wait: it passes when the app is still running, a ready marker appeared
// and no fatal text did. No prompt is typed, so the app's history gets no
// session. The app's process tree is stopped afterwards.
func Smoke(ctx context.Context, spec LaunchSpec, wait time.Duration) (SmokeResult, error) {
	if spec.Bin == "" {
		return SmokeResult{}, errors.New("there is no program to test")
	}
	if err := launchCheckSecrets(spec); err != nil {
		return SmokeResult{}, err
	}
	cmd := exec.Command(spec.Bin, spec.Args...)
	cmd.Dir = spec.Dir
	cmd.Env = append(os.Environ(), launchExpandEnv(spec.Env, os.Getenv)...)
	p, err := StartPTY(cmd, 120, 40)
	if err != nil {
		return SmokeResult{}, err
	}
	defer p.Close()
	var mu sync.Mutex
	var screen strings.Builder
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		buf := make([]byte, 8192)
		for {
			n, err := p.Read(buf)
			mu.Lock()
			if screen.Len() < smokeMaxOutput {
				screen.Write(buf[:n])
			}
			mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	fatal := append(append([]string{}, CommonFatal...), spec.Fatal...)
	var res SmokeResult
	var text string
	var readyAt time.Time
	deadline := time.Now().Add(wait)
	for {
		exited := p.Exited()
		if exited {
			// The terminal passes on the last output after the program ends.
			select {
			case <-drained:
			case <-time.After(2 * time.Second):
			}
		}
		mu.Lock()
		text = StripANSI(screen.String())
		mu.Unlock()
		for _, f := range fatal {
			if res.Fatal == "" && strings.Contains(text, f) {
				res.Fatal = f
			}
		}
		for _, r := range spec.Ready {
			if !res.Ready && strings.Contains(text, r) {
				res.Ready, readyAt = true, time.Now()
			}
		}
		res.Alive = !exited
		// Once ready, watch one more second for a late fatal text.
		settled := res.Ready && time.Since(readyAt) > time.Second
		if res.Fatal != "" || !res.Alive || settled || time.Now().After(deadline) || ctx.Err() != nil {
			break
		}
		select {
		case <-ctx.Done():
		case <-time.After(200 * time.Millisecond):
		}
	}
	res.Screen = smokeTail(text, smokeMaxScreen)
	if !res.Alive {
		res.Code, _ = p.Wait()
	}
	res.OK = res.Alive && res.Fatal == "" && (res.Ready || len(spec.Ready) == 0)
	return res, ctx.Err()
}

// smokeTail is the last n bytes of s, cut at a line start when it can be.
func smokeTail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[len(s)-n:]
	if i := strings.IndexByte(s, '\n'); i >= 0 && i < 200 {
		return s[i+1:]
	}
	for len(s) > 0 && !utf8.RuneStart(s[0]) {
		s = s[1:]
	}
	return s
}
