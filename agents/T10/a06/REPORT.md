# T10 a06: port conflicts

Slice: the chosen port is taken, the port is taken between the free check and
the bind, and the retry/fallback path. Branch `agent/T10-a06`.

## What I reviewed

- `internal/serve/config.go`: `PortFree`, `FreePort`, `StopAndWait` (port
  part), and the comment that says the connect test exists because a bind
  alone proves nothing on Windows.
- `internal/serve/service_*.go`: how a start reaches the port (`Install`,
  `Start`, `startLogged`). All of them are asynchronous: launchd
  `bootstrap`, systemd `restart` and Windows `cmd.Start` return before
  llama-server binds, so a bind failure is only visible in the server log.
- `internal/serve/client.go`: `fatalInLog` already recognises "address
  already in use" / "couldn't bind".
- `internal/app/app.go` (read only, it is off limits and being rewritten):
  the deploy path and `StartSync`, the only places that decide a port and
  start the service.

## Tests added

`internal/proc/port_test.go` (external test package, imports `internal/serve`;
the port code lives in `internal/serve/config.go` and the assigned test path
is in the owned `internal/proc` tree, so no other agent's test file is
touched).

- `TestPortTakenBetweenCheckAndBind`: finds a free port, confirms
  `PortFree` says free, then another program binds the port (the race). A
  fake llama-server (this test binary re-executed) tries to bind, fails, and
  writes llama-server's own line
  `srv  llama_server: couldn't bind HTTP server socket, port N`.
  `BindFailed` recognises it, `MoveToFreePort` moves the config to a port
  that `PortFree` confirms is free, the saved config has the new port, the
  fake server starts and answers on it, and the program that took the first
  port still answers (not adopted, not killed). Only the fake process is
  killed, by PID. The fake is a skipped-by-default test function re-executed
  with `-test.run`, not a `TestMain`, so a12's shared fake-process helper can
  define its own entry point without a collision.
- `TestFreePortLooksPastABusyRun`: holds 100 consecutive ports and checks
  that `FreePort` does not return one of them. Failed before the fix
  (`FreePort(53570) returned the busy port`), passes after.

## Findings

### F1 (major, open): the bind race is detected but never retried

`internal/app/app.go:1431` checks `serve.PortFree(cfg.Port)`, then
`internal/app/app.go:1456` pre-reads a split model (can take minutes), then
`internal/app/app.go:1458` calls `serve.Start()`. If another program takes
the port in that gap, llama-server exits with "couldn't bind" and
`StartSync`'s `fail` (`internal/app/app.go:1466`) calls `serve.Stop()` and
returns `"the model server couldn't start: another program is using its
port..."` (`internal/app/app.go:1559`). The user has to press Start again;
the retry/fallback path does not exist. The same shape is in the deploy path
(`internal/app/app.go:796` check, `:805` prewarm, `:807` `serve.Install`).
The comment at `internal/app/app.go:1424` ("moving off a port another
program took") covers only the pre-check case, not the race.

Why it is realistic, not theoretical: the gap contains the pre-read of a
split model (minutes on a big model), and on macOS/Linux the service manager
returns before the bind, so the only signal is the log line the app already
matches.

Fix (needs `internal/app`, which is off limits tonight): in the `fail` path,
when `serve.BindFailed(serve.TailLog(60))` is true, move once and restart
instead of giving up:

```go
if serve.BindFailed(serve.TailLog(60)) {
    old, port, err := serve.MoveToFreePort(cfg) // saves the config
    if err == nil {
        integrate.NotePort(old)
        integrate.NotePort(port)
        a.rewriteApps(cfg) // connected apps follow
        serve.Start()
        // continue the wait loop once more
    }
}
```

The serve-side pieces are in this branch and tested. Wiring them is a small,
mechanical change to the rewritten `StartSync`.

### F2 (minor, fixed): `FreePort` returned a port known to be busy

`internal/serve/config.go:177` scanned only `start..start+99` and then
returned `start`, which is the first port it had just found busy (or an
invalid port above 65535). The caller saves it (`internal/app/app.go:797`,
`:1439`) and the start fails on a port EZLM already knew was taken. Fixed:
the scan now runs to 65535 through `freePortFrom`, and only a fully busy
range falls back to `start` (reported by the bind). Covered by
`TestFreePortLooksPastABusyRun`.

### F3 (minor, fixed): `MoveToFreePort` never saves a taken port

New `internal/serve/port.go`: `MoveToFreePort` scans, errors when nothing is
free, and restores `c.Port` if the save fails, so a caller cannot persist a
port it did not verify.

### F4 (minor, code-read): `SetPort` has the same check-then-bind window

`internal/app/app.go:1496` stops the server, waits 500ms, checks
`PortFree`, saves, then `StartSync` starts. A program that takes the chosen
port in that window hits F1. No separate fix; the retry covers it.

### F5 (minor, code-read, Windows): `PortFree` can miss a listener bound to a
single non-loopback address

`PortFree` connects to 127.0.0.1 and binds 127.0.0.1 and 0.0.0.0. Windows
sets `SO_REUSEADDR` for llama-server, so a program listening only on one LAN
address (not 0.0.0.0) is not seen by the dial and the wildcard bind can
succeed. EZLM's own loopback URL still works and the LAN URL falls through
to the other program for that one address, so this is a sharing edge, not a
failed start. Not changed.

## What I could not finish

- The retry is not wired into `internal/app/app.go` (off limits, being
  rewritten). F1 has the exact call site and sketch.

## Open questions

- Should the retry move ports at all, or wait a few seconds for the thief to
  go? Moving is what the existing pre-check does and what connected apps are
  rewritten for, so moving keeps one behaviour.
- How many times may a start move? One move per start is enough for a race;
  a program that keeps taking ports should be reported, not chased.

## Checks run

`gofmt -l .` (empty), `go build ./...`, `go vet ./...`,
`go test -p 2 ./internal/proc/ ./internal/serve/`,
`GOOS=windows GOARCH=amd64 go build ./...`, `GOOS=windows GOARCH=arm64 go build ./...`,
`GOOS=linux go build ./...`.
