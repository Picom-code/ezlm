# T07/a01: DNS rebinding against the engine API

**Threat.** A malicious page (`attacker.example`) is opened in the user's browser. Its DNS
answer flips to `127.0.0.1` on a short TTL, so the page's own origin now points at the EZLM
engine API. The browser still sends the page's hostname in the `Host` header
(`Host: attacker.example:PORT`). Without a Host check the engine answers, and the page can read
`/api/status`, `/api/logs`, `/api/live` and the rest of the API. The per-launch token is not a
barrier here: it sits in the URL the user opened (`/?t=...`), and the guard falls back to the
query string.

## PoC

`internal/web/host_rebinding_test.go` starts the real server (`Listen` + `Serve`) on a loopback
port with a stub backend, then sends the same request with different `Host` headers, using a
valid token. Nothing else differs, so a status difference is the Host check.

- `TestHostRebindingReadsAPI`: `Host: attacker.example:PORT` must get `403 forbidden`; the same
  request with the real `Host` gets `200` and the app status JSON. This is the rebinding read.
- `TestHostRebindingHosts`: allowlist table (`127.0.0.1:PORT`, `localhost:PORT`,
  `LOCALHOST:PORT` pass; a name, a missing port, another port, a trailing space, a dotted suffix
  and a doubled host are rejected).
- `TestHostCheckRunsBeforeToken`: a foreign `Host` gets `403` even with a wrong token and even
  with the real token, so the Host gate is outside the token gate.

### Before / after

The baseline snapshot already had a Host check (`internal/web/server.go:232` before this
change), but it compared `r.Host` to `s.addr` for exact string equality. "Before" below is that
check removed entirely, which is the state the brief describes:

```
--- FAIL: TestHostRebindingReadsAPI
    host_rebinding_test.go:82: rebinding Host got 200, want 403
    (body "{\"deployed\":true,\"healthy\":true,\"local_url\":\"http://127.0.0.1:8080/v1\"...}")
--- FAIL: TestHostRebindingHosts
    host_rebinding_test.go:124: Host "attacker.example:61543" got 200, want 403
```

With the baseline exact-match check the rebinding cases pass, but the check is too strict and
fails the allowlist table:

```
--- FAIL: TestHostRebindingHosts
    host_rebinding_test.go:124: Host "localhost:61252" got 403, want 200
    host_rebinding_test.go:124: Host "LOCALHOST:61252" got 403, want 200
```

After the fix (below): `ok github.com/picom-code/ezlm/internal/web`.

## Fix

`internal/web/server.go`:

- `hostCheck` now calls `loopbackHost(r.Host)` (line 236).
- `loopbackHost` (line 254) parses the header with `net.SplitHostPort`, requires the port to
  equal the port we listen on, and accepts only `localhost` (case-insensitive) or a loopback IP
  literal (`net.ParseIP(...).IsLoopback()`). A hostname that merely resolves to `127.0.0.1` is
  not accepted, which is exactly the rebinding case.
- New `strings` import.

No other file changed. The `403` body stays the generic `forbidden`.

## Severity

**Blocker** if the check is absent: a web page reads the user's engine state and logs, and can
drive the POST endpoints (deploy, stop, uninstall, key add/rotate/revoke). With the check in
place the threat is closed.

## Residual risk

- The Host check does not stop a page whose origin is already `http://127.0.0.1:PORT` (or
  `localhost:PORT`): a simple `text/plain` POST needs no preflight and reaches the API even
  though the page cannot read the reply. That is a02's and a03's slice.
- Accepting `localhost:PORT` and any `127/8` literal with the right port is safe (the listener is
  on `127.0.0.1`), but it does mean another local process could point a browser page at
  `http://127.0.0.2:PORT` and pass the Host gate. It still needs the token, so this is minor.
- The check trusts `s.addr`. It is set from `net.Listen("tcp", "127.0.0.1:0")` in `Listen`, so
  it is loopback today; if a future caller serves a different listener, the check follows that
  address rather than asserting loopback.

## Open questions

- Should a `Host` with no port (`127.0.0.1`) be accepted when the server is on a non-default
  port? Browsers always send the port there; the strict answer is no, and that is what the code
  does.
- Windows treats all of `127/8` as loopback; the check allows it there too. No client uses it.

## Files

- `internal/web/host_rebinding_test.go` (new)
- `internal/web/server.go` (Host check)
- `agents/T07/a01/REPORT.md`
