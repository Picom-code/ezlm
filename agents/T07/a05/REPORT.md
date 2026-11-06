# T07 / a05: LAN mode exposure

**Threat:** another user on the same LAN (and, secondarily, a browser page on
that machine) reaches EZLM's servers. Question: what is on `0.0.0.0`, with
which key, and can a LAN peer reach the setup ("engine") API as well as
llama-server?

## What is exposed

| Port | Bound when | Bound to | Protection |
|---|---|---|---|
| Setup API (`internal/web`) | always | `127.0.0.1:<random>` (`internal/web/server.go:75`) | per-launch token in `X-EZLM-Token` or `?t=`, plus Host check (`server.go:234`) |
| llama-server (`/v1`, `/props`, `/slots`, `/metrics`, web UI) | always | `127.0.0.1:<port>`; `0.0.0.0:<port>` when LAN is on (`internal/serve/config.go:92-97`) | `--api-key` with **every** named key, comma-separated (`config.go:121`, `config.go:162-171`) |

`/health` and `/v1/health` are public in llama.cpp by design (upstream
`tools/server/server.cpp` marks them "public endpoint (no API key check)").
Everything else on the model server needs the key.

So a LAN peer gets: the OpenAI-compatible model server, using any of the
keys in `~/.ezlm/config.json` (the same keys local integrations use; there
is no LAN-only key). It does **not** get the setup API, which stays on
loopback.

## PoC and results (`internal/web/lan_test.go`)

Two clients, one loopback and one pretending to be a LAN peer:

1. `TestEngineAPIIsLoopbackOnly` starts the real `web.Listen` server. The
   local client with the token gets 200; without it 401; the same request
   with a LAN `Host` (`192.168.1.20:<port>`) gets 403 even with the correct
   token. The test also dials this machine's LAN address on the setup port
   and expects a refused connection (skipped when the machine has no LAN
   address).
2. `TestLANModeExposesModelServerOnly` binds the fake model server to
   `0.0.0.0` (what `--host 0.0.0.0` does) and shows a LAN peer gets 401
   without a key and 200 with one; the Host header plays no part, which is
   intended for a server shared on purpose.
3. `TestLANConfigExposure` pins the flags LAN mode passes: `--host
   0.0.0.0`, `--api-key <key1>,<key2>` (all named keys), `--metrics`; and
   `127.0.0.1` when LAN is off.

## Findings

**F1 (minor, fixed): the setup API's "local only" rested on the bind
address alone.** `hostCheck` validated only `r.Host`, which the caller
controls (`internal/web/server.go:234`). While the listener is
`127.0.0.1:0` (`server.go:75`) a LAN peer cannot connect, so this was not
exploitable. But the moment the listener is bound wider (a port forward, a
future change, a platform quirk) any LAN peer that knows the token is in,
and the Host check does not stop it because the peer can send `Host:
127.0.0.1:<port>`. Evidence: `TestEngineAPIRejectsNonLoopbackPeer` fails
before the fix with `got 200, want 403`.

Fix: `hostCheck` now also requires a loopback `RemoteAddr`
(`internal/web/server.go:252-261`). The test passes after.

**F2 (minor, open): LAN mode has no key scoping.** `--api-key` receives
every key in the config, including the main key that EZLM wired into
OpenCode and Hermes (`internal/serve/config.go:121`, `:162-171`). A guest
on the LAN can use any of them; revoking a guest's key is fine, but there
is no "LAN only" key and no way to see which key a LAN peer used (the
server log does not record it). Not a vulnerability; a policy gap.

**F3 (minor, open): the LAN URL can name the wrong interface.**
`lanIP()` takes the address of the default route (`config.go:183-190`).
With a VPN up, the default route is often the tunnel, so the UI shows a
`100.x`/`10.x` tunnel address that no phone on the Wi-Fi can reach, while
the server is correctly bound to `0.0.0.0`. Proposed fix: enumerate
interfaces, prefer a private IPv4 on an up, non-loopback, non-tunnel
interface. `internal/serve/config.go` is T07's to edit but the change
needs its own test file; left for the Lead to schedule.

**F4 (minor, open): no firewall hint.** Turning LAN mode on opens the port
on every interface; on Windows the first bind may raise a Defender prompt,
and if the user declines, the UI still says "Visible on your network".
Not verified on a Windows host (code-read only).

**F5 (informational): `/health` and the llama.cpp web UI are reachable
without a key on `0.0.0.0`.** This is llama.cpp's documented behaviour,
not EZLM code; it leaks only that a server is there and the model alias.

**F6 (informational, a06's): the key is on the llama-server command line**
(`config.go:121`), readable by other local users via `ps`. The LAN angle
is that the same key is the LAN credential, so a local user can become a
LAN client.

## Not reachable

A LAN peer cannot reach the setup API: the listener is loopback, the Host
check rejects its Host, and (after F1) the source check rejects its
address. There is no proxy from llama-server to the setup API.

## Changes

- `internal/web/server.go`: `hostCheck` also rejects non-loopback peers;
  new `loopbackPeer` helper.
- `internal/web/lan_test.go`: the three tests above.

Caveat for other agents: `httptest.NewRequest` sets `RemoteAddr` to
`192.0.2.1:1234`, which now counts as a peer. Tests that call
`hostCheck` directly with `httptest.NewRequest` must set
`req.RemoteAddr = "127.0.0.1:1234"`; tests that go through a real
listener are unaffected.

## Verification

```
gofmt -l internal/web          # empty
go test -p 2 ./internal/web/   # ok
go vet ./internal/web/
go build ./...
```

Before/after: `TestEngineAPIRejectsNonLoopbackPeer` failed (`got 200, want
403`) with the old `hostCheck`, passes with the new one.

## Open questions

- Should LAN mode get a dedicated key, or a per-key "LAN allowed" flag, so
  the main integration key is not the LAN credential?
- Should `SetLAN` warn when `lanIP()` falls back to a tunnel address?
- Is the llama.cpp web UI worth disabling with `--no-webui` when LAN is on?
