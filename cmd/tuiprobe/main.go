// Command tuiprobe runs a program in a hidden pseudo-terminal and drives it
// like a person would (PLAN 9.3 Tier C): wait for a ready marker, type a
// prompt and Enter, wait for a regular expression in what the program draws,
// send exit keys and report the screen text and the exit code. It uses the
// same pseudo-terminal as EZLM's TUI smoke test (creack/pty on Unix, ConPTY
// on Windows) and stops only the process tree it started.
//
//	tuiprobe -cmd ~/.ezlm/launch/opencode.command [-cols 120 -rows 40]
//	         [-ready "text"] [-wait 20s]
//	         [-type "prompt"] [-type-delay 10ms]
//	         [-expect 'EZLM-CHECK-[A-Z2-7]{8}'] [-timeout 180s] [-settle 0s]
//	         [-exit '/exit{enter}' | -exit '{ctrl-c}{wait}{ctrl-c}'] [-exit-wait 10s] [-code 0]
//	         [-dir folder] [-env K=V ...] [-json] [-out screen.txt] [-- args]
//
// Exit status 0 means every wait asked for succeeded; 1 that one failed
// (the reason is on stderr); 2 a usage error. -json prints the whole report
// on stdout; otherwise the screen text goes to stdout (or -out).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

type multi []string

func (m *multi) String() string     { return strings.Join(*m, ",") }
func (m *multi) Set(v string) error { *m = append(*m, v); return nil }

func main() {
	var o options
	var env multi
	flag.StringVar(&o.Cmd, "cmd", "", "program or launcher to run")
	flag.IntVar(&o.Cols, "cols", 120, "terminal width")
	flag.IntVar(&o.Rows, "rows", 40, "terminal height")
	flag.StringVar(&o.Ready, "ready", "", "text that means the app is up")
	flag.DurationVar(&o.ReadyWait, "wait", 20*time.Second, "how long to wait for the ready text")
	flag.StringVar(&o.Type, "type", "", "text to type (then Enter) once ready")
	flag.DurationVar(&o.TypeDelay, "type-delay", 0, "pause between typed characters")
	expect := flag.String("expect", "", "regular expression to wait for after typing")
	flag.DurationVar(&o.Timeout, "timeout", 180*time.Second, "how long to wait for -expect")
	flag.DurationVar(&o.Settle, "settle", 0, "after -expect matched, how long to let the app keep drawing before -exit")
	flag.StringVar(&o.Exit, "exit", "", "keys that end the app, e.g. '/exit{enter}' or '{ctrl-c}{wait}{ctrl-c}'")
	flag.DurationVar(&o.ExitWait, "exit-wait", 10*time.Second, "how long to wait for the app to end after -exit")
	flag.IntVar(&o.WantCode, "code", -1, "exit code the app must end with (-1: any)")
	flag.StringVar(&o.Dir, "dir", "", "working folder")
	flag.Var(&env, "env", "KEY=VALUE added to the environment (repeatable)")
	asJSON := flag.Bool("json", false, "print the report as JSON")
	out := flag.String("out", "", "write the screen text here")
	flag.Parse()
	if o.Cmd == "" {
		fmt.Fprintln(os.Stderr, "usage: tuiprobe -cmd <program> [flags] [-- args]")
		os.Exit(2)
	}
	if *expect != "" {
		re, err := regexp.Compile(*expect)
		if err != nil {
			fmt.Fprintln(os.Stderr, "tuiprobe: -expect:", err)
			os.Exit(2)
		}
		o.Expect = re
	}
	o.Args, o.Env = flag.Args(), env
	r := probe(o)
	if *out != "" {
		if err := os.WriteFile(*out, []byte(r.Screen), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "tuiprobe:", err)
		}
	}
	if *asJSON {
		b, _ := json.MarshalIndent(r, "", "  ")
		fmt.Println(string(b))
	} else if *out == "" {
		fmt.Print(r.Screen)
	}
	status := fmt.Sprintf("ready=%v matched=%v exited=%v code=%d secs=%.1f", r.Ready, r.Matched, r.Exited, r.Code, r.Secs)
	if !r.OK {
		fmt.Fprintf(os.Stderr, "tuiprobe: %s (%s)\n", r.Fail, status)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "tuiprobe: ok (%s)\n", status)
}
