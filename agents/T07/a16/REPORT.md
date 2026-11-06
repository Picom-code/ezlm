# T07 a16: severity table and report assembly

## What this agent does

Cross-cutting support for T07. Two deliverables:

- `agents/T07/a16/findings.md`: one row per threat in the T07 brief, with the
  status of the control in the current tree (open / fixed / mitigated), the
  evidence (file:line, plus the owning agent's test once it lands), a severity,
  the fix or residual risk, and the owner.
- `agents/T07/a16/REPORT.md`: this file. It records how the table was built and
  what the morning merge should read first.

No code changes. This agent owns `agents/T07/a16/` only, so the table cannot
collide with a fix in `internal/web` or `internal/serve`.

## Method

1. Read the T07 brief, `README.md` and the T07 section of `TASKS.md`.
2. Read the code the threats name, in this worktree: `internal/web/server.go`,
   `internal/keys/keys.go`, `internal/serve/{config,run,client}.go`,
   `internal/paths/`, `internal/integrate/log.go`, `cmd/ezlm/main.go`,
   `internal/web/static/app.js`, and both apps' engine clients
   (`windows/EZLM/Engine.cs`, `macos/Sources/EZLM/Engine.swift`).
3. Record each control as present or absent with the line that proves it. A
   control counts as `mitigated` only when the code path is the shipped one,
   not a test double.
4. Reconcile with each owner agent's report as it lands, and change the row to
   `fixed` (with the commit and the test name) or `open` (with the reason).

## What the table found

The snapshot already contains three of the nine threat controls:

- **T01 DNS rebinding**: `hostCheck` requires the exact `127.0.0.1:<port>`
  (`internal/web/server.go:232-237`). The owner should prove it and claim
  "verified", not "fixed".
- **T09 timing-safe compare**: `subtle.ConstantTimeCompare`
  (`internal/web/server.go:254`).
- **T08 redaction**: report body redacts configured keys and key-shaped strings
  (`internal/app/launch.go:230-237`, `internal/integrate/log.go:26-46`).

The open items that matter before beta:

- **T06 (major, open)**: the API key is on the llama-server command line
  (`--api-key`, `internal/serve/config.go:121`), so any local user can read it
  from `ps`. The 0600 mode on `config.json` does not help. Fix: a 0600 key file
  plus `--api-key-file`.
- **T04 (major, open)**: the engine token is in the page URL
  (`internal/web/server.go:82`), so it is in browser history. Cross-origin
  `Referer` is origin-only by default, so that leak is smaller than it looks.
- **T12 (major, open)**: an empty key list still passes `--api-key ""` to
  llama-server, which may mean no auth in LAN mode. Needs a test, then a
  startup guard.
- **T02 (major, mitigated)**: the token is the only CSRF control; content type
  is not checked and `?t=` is accepted on POST.

## Reconciliation so far

| Agent | Commit | Row | Status now |
|---|---|---|---|
| a03 | `7e14b66` | T03 CORS | fixed: CORS middleware, nosniff, preflight fails closed; five tests |
| a06 | `fcaa2d8` | T06 key on argv | fixed: 0600 key file + `--api-key-file`; fallback to `--api-key` if the file cannot be written |
| a07 | `0a6ae76` | T07 permissions | fixed on Unix (0700 home, 0600 log); Windows ACL still open (major on a shared PC) |
| a13 | `7526148` | T13 harness | fixed: `internal/web/authtest/` |
| a02 | `a91a2b6` | T02 CSRF | fixed: query token GET-only, JSON content type required on non-GET |
| a10 | `8d55ddb` | T10 fuzz | fixed: fuzzer found a 500 on a non-UTF-8 static path, now 404 |
| a12 | `1607820` | T12 keys | fixed F1-F4 (empty key, mirror, temp mode, loose mode); F5 open in frozen `internal/app` |
| a14 | `3012670` | T14 handlers | fixed: 1 MB body cap, 415/400/413, home paths redacted |
| a01 | `cc25fa7` | T01 rebinding | fixed: loopback Host allowlist; the baseline check was correct but rejected `localhost` |
| a04 | `95943c6` | T04 token in URL | fixed blast radius: query token only for `/api/events`, `Referrer-Policy: no-referrer`; history still carries it |
| a09 | `fed0be6` | T09 timing | fixed: `keys.Equal`, `guard` uses it; `==` timing measured at 1.5x |
| a11 | `36e0c79` | T11 threat model | doc landed |
| a15 | `24f300c` | T15 app clients | review landed; worst finding minor |
| a05 | `9384fa1` | T05 LAN mode | fixed: `hostCheck` also requires a loopback peer; LAN tests |

a07 also reports F5 (no Windows ACL at all) and F6 (`ui.url` keeps an older
file's mode, frozen `cmd/**`). F5 is folded into the T07 row; F6 is noted there.

Merge-order warning: seven branches edit `internal/web/server.go` (a01, a02,
a03, a04, a09, a10, a14) and three edit `internal/serve/config.go` (a06, a07,
a12). Apply in sequence and re-run the tests. a02 and a14 both add a JSON
content-type check; keep one implementation (a14's `decodeJSON`). a04 and a02
both narrow the query token; a04's rule (query only for `/api/events`) is the
stricter one. a01, a04 and a05 all edit `hostCheck` in `server.go`; merge the
three by hand and keep the loopback allowlist (a01), the loopback peer check
(a05) and `Referrer-Policy` (a04).

## What I changed

Nothing outside `agents/T07/a16/`.

## Could not finish / open questions

- The table is built from code reading until the owner agents commit. Rows are
  updated with their test names and commit shas when those land. If a row still
  says `[code]` at merge time, treat it as "control present, proof pending".
- Whether `llama-server --api-key ""` disables auth is a behaviour question
  about the upstream binary. It cannot be tested here (no model load allowed).
  a12 should confirm from llama.cpp's source or by reading its argument parser.
- Severity is relative to the beta threat model. A finding that only a
  same-user process can exploit (T06 needs a local login) is still `major`
  because a shared family PC or a second account is in scope.

## Next steps

1. Owner agents: replace `[code]` with `[test]` and the test name, and set the
   status to `fixed` with the commit.
2. The merge owner: read T06 and T04 first; they are the two changes with a
   visible user-facing effect (key file, URL).
3. After the merge, regenerate this table from the merged tree, not from the
   individual branches, so a control that was fixed on one branch and dropped
   on another shows up.
