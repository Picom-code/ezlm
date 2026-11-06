// Command fakellm serves the scripted OpenAI-compatible fake model from
// internal/fakellm on a loopback port, for the e2e script. It prints its
// base URL (http://127.0.0.1:<port>) as the first line and runs until it is
// stopped by PID. Requests go to -log as JSON lines, without keys.
//
//	fakellm -key K -alias qwen3.5-4b [-addr 127.0.0.1:0] [-ctx 65536]
//	        [-levels low,medium,xhigh] [-thinking] [-no-tools] [-delay 2s]
//	        [-log requests.jsonl]
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/picom-code/ezlm/internal/fakellm"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "fakellm:", err)
		os.Exit(1)
	}
}

// run serves until ctx ends. The first line on stdout is the base URL.
func run(ctx context.Context, args []string, stdout io.Writer) error {
	fl := flag.NewFlagSet("fakellm", flag.ContinueOnError)
	addr := fl.String("addr", "127.0.0.1:0", "listen address")
	key := fl.String("key", "", "Bearer key the requests must carry")
	alias := fl.String("alias", "fake-model", "model id")
	nctx := fl.Int("ctx", 65536, "n_ctx reported by /props")
	levels := fl.String("levels", "", "comma-separated reasoning_effort levels the template accepts")
	thinking := fl.Bool("thinking", false, "stream reasoning before answers")
	noTools := fl.Bool("no-tools", false, "never call tools")
	delay := fl.Duration("delay", 0, "wait this long before each chat answer")
	logPath := fl.String("log", "", "append one JSON line per request to this file")
	if err := fl.Parse(args); err != nil {
		return err
	}
	if h, _, err := net.SplitHostPort(*addr); err != nil || (h != "127.0.0.1" && h != "localhost" && h != "::1") {
		return errors.New("-addr must be a loopback address such as 127.0.0.1:0")
	}
	o := fakellm.Options{Key: *key, Alias: *alias, Ctx: *nctx, Thinking: *thinking, NoTools: *noTools, Delay: *delay}
	if *levels != "" {
		o.Levels = strings.Split(*levels, ",")
	}
	if *logPath != "" {
		f, err := os.OpenFile(*logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		defer f.Close()
		o.Log = f
	}
	l, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, "http://"+l.Addr().String())
	srv := &http.Server{Handler: fakellm.New(o), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		srv.Close()
	}()
	if err := srv.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
