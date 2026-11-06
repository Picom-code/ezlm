package integrate

import (
	"crypto/rand"
	"encoding/base32"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"
)

// The shared check harness. Every model-and-tool check runs in a fresh
// workspace holding one file with a random nonce; the prompt never contains
// the nonce, so it can only reach the answer through a real tool call.

// CheckFile is the file every check asks the app to read.
const CheckFile = "ezlm-check.txt"

// CheckPrompt is the same for every tool.
const CheckPrompt = "Use your file-reading tool to open ezlm-check.txt in the current folder. Reply with exactly the text inside it and nothing else."

// CheckWorkspace is a throwaway folder for one check. Run the app with Dir
// as its working folder.
type CheckWorkspace struct {
	Dir   string // a fresh temp folder, ezlm-check-<random>
	File  string // Dir/ezlm-check.txt
	Nonce string // EZLM-CHECK-<8 base32 characters>, the file's whole content
}

// NewCheckWorkspace creates the folder and the nonce file.
func NewCheckWorkspace() (*CheckWorkspace, error) {
	dir, err := os.MkdirTemp("", "ezlm-check-")
	if err != nil {
		return nil, err
	}
	// The real path (macOS temp folders sit behind /var → /private/var), so
	// a path an app prints or resolves matches Dir and File.
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	w := &CheckWorkspace{Dir: dir, File: filepath.Join(dir, CheckFile), Nonce: NewNonce()}
	if err := os.WriteFile(w.File, []byte(w.Nonce+"\n"), 0o644); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	return w, nil
}

// NewNonce is EZLM-CHECK- plus 8 random base32 characters.
func NewNonce() string {
	b := make([]byte, 5)
	rand.Read(b)
	return "EZLM-CHECK-" + base32.StdEncoding.EncodeToString(b)
}

// Prompt is the check prompt.
func (w *CheckWorkspace) Prompt() string { return CheckPrompt }

// Passed reports whether the app's output contains the nonce.
func (w *CheckWorkspace) Passed(out string) bool { return strings.Contains(out, w.Nonce) }

// Close removes the workspace.
func (w *CheckWorkspace) Close() error {
	if w == nil {
		return nil
	}
	return os.RemoveAll(w.Dir)
}

// ErrorText maps a known failure text to the sentence the user sees.
type ErrorText struct {
	Match string // case-insensitive substring of the app's output
	Say   string // one sentence naming the next action; "{app}" becomes the tool's name
	State string // optional State* the target takes, e.g. StateNeeds64K
}

var (
	errTextMu sync.RWMutex
	errTexts  = map[string][]ErrorText{} // by tool id; "" = every tool
)

// RegisterErrorText adds failure texts for a tool ("" for every tool). Tools
// call it from init(); the first match wins, tool texts before common ones.
func RegisterErrorText(tool string, texts ...ErrorText) {
	errTextMu.Lock()
	defer errTextMu.Unlock()
	errTexts[tool] = append(errTexts[tool], texts...)
}

// Explain finds the sentence for a failed check's output.
func Explain(tool, out string) (ErrorText, bool) {
	low := strings.ToLower(out)
	name := tool
	if t, ok := Lookup(tool); ok {
		name = t.Info().Name
	}
	errTextMu.RLock()
	defer errTextMu.RUnlock()
	for _, list := range [][]ErrorText{errTexts[tool], errTexts[""]} {
		for _, e := range list {
			if strings.Contains(low, strings.ToLower(e.Match)) {
				e.Say = strings.ReplaceAll(e.Say, "{app}", name)
				return e, true
			}
		}
	}
	return ErrorText{}, false
}

func init() {
	RegisterErrorText("",
		ErrorText{Match: "invalid api key", Say: "{app} has an old EZLM key. Press Connect to update it.", State: StateNeedsUpdate},
		ErrorText{Match: "401 unauthorized", Say: "{app} has an old EZLM key. Press Connect to update it.", State: StateNeedsUpdate},
		ErrorText{Match: "status code 401", Say: "{app} has an old EZLM key. Press Connect to update it.", State: StateNeedsUpdate},
		ErrorText{Match: "http 401", Say: "{app} has an old EZLM key. Press Connect to update it.", State: StateNeedsUpdate},
		ErrorText{Match: "econnrefused", Say: "EZLM's model server isn't running. Start it on Overview, then try again."},
		ErrorText{Match: "connection refused", Say: "EZLM's model server isn't running. Start it on Overview, then try again."},
		ErrorText{Match: "model not found", Say: "{app} asks for a model EZLM isn't serving. Press Connect to update it.", State: StateNeedsUpdate},
		ErrorText{Match: "unexpected reasoning effort", Say: "{app} asked the model for a thinking level it doesn't support. Press Connect to update it.", State: StateNeedsUpdate},
		ErrorText{Match: "below the minimum 64,000", Say: "{app} needs a 64K context. Connect again to give the model the room (it may run slower), or pick a smaller model.", State: StateNeeds64K},
		ErrorText{Match: "exceed_context_size_error", Say: "The conversation got bigger than the model's context. Start a new session in {app}, or give the model more room in Models."},
		ErrorText{Match: "exceeds the available context size", Say: "The conversation got bigger than the model's context. Start a new session in {app}, or give the model more room in Models."},
		ErrorText{Match: "no space left on device", Say: "The disk is full. Free up some space, then try again."},
		ErrorText{Match: "there is not enough space on the disk", Say: "The disk is full. Free up some space, then try again."},
		ErrorText{Match: "eacces", Say: "{app} couldn't open one of its files because of folder permissions. Details are in the log; press Report a problem to send them."},
		ErrorText{Match: "permission denied", Say: "{app} couldn't open one of its files because of folder permissions. Details are in the log; press Report a problem to send them."},
	)
}

// ErrLine picks the most telling line of a failed command's output for an
// error message: the last line that reports an error, else the last line
// with real text. Progress bars, blank lines, terminal colours and "see the
// log" pointers are skipped, and a long line is shortened.
func ErrLine(s string) string {
	lines := strings.Split(strings.ReplaceAll(StripANSI(s), "\r", "\n"), "\n")
	pick := ""
	for i := len(lines) - 1; i >= 0 && pick == ""; i-- {
		if l := strings.TrimSpace(lines[i]); telling(l) && reportsError(l) {
			pick = l
		}
	}
	for i := len(lines) - 1; i >= 0 && pick == ""; i-- {
		if l := strings.TrimSpace(lines[i]); telling(l) {
			pick = l
		}
	}
	if pick == "" {
		return "no output"
	}
	if r := []rune(pick); len(r) > 200 {
		pick = string(r[:200]) + "…"
	}
	return pick
}

func reportsError(l string) bool {
	low := strings.ToLower(l)
	for _, w := range []string{"error", "failed", "fatal", "denied", "cannot", "can't", "not found", "exception", "refused", "unable"} {
		if strings.Contains(low, w) {
			return true
		}
	}
	return false
}

// telling: a line with words in it, not a progress bar or a log pointer.
func telling(l string) bool {
	if len(l) <= 3 {
		return false
	}
	letters := 0
	for _, c := range l {
		if unicode.IsLetter(c) {
			letters++
		}
	}
	if letters < 3 || letters*3 < len([]rune(l)) {
		return false // "#####  45%", "[====>   ] 12/40"
	}
	low := strings.ToLower(l)
	for _, w := range []string{"complete log of this run", "for more information", "run with --verbose", "see the log", "details are in"} {
		if strings.Contains(low, w) {
			return false
		}
	}
	return true
}
