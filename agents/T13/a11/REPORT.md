# a11 report: TROUBLESHOOTING for internal/web, keys, proc, awake, prereq, version

## Deliverable

`agents/T13/sections/a11.md`, 26 entries. Every entry has `Message`, `Meaning` and `Action`, plus a
`Code:` line with the file and line that proves the text and the behaviour. 5 of the 26 are
messages that were too vague to document usefully and are collected at the end of the section for a14.

## Coverage by package

| Package | Entries | Notes |
|---|---|---|
| `internal/web` | 1 to 6 | 403 host check, 401 token, JSON `error` pass-through, `streaming unsupported`, bind failure, stuck scan |
| `internal/keys` | 7, 8 | the `crypto/rand` panic, and the key shape (`ezlm-` + 40 base62) |
| `internal/proc` | 9 to 13 | start error, exit status, killed by cancel/timeout, context errors, WaitDelay swallowing |
| `internal/awake` | 14 | silent no-op; documented by symptom because there is no message |
| `internal/prereq` | 15 to 25 | every `Detail` and `Action` string in `prereq.go`, plus the two wrappers in `app.go` |
| `internal/version` | 26 | no error path exists; documented the one real confusion (same string as release tag) |

## How the claims were checked

- Read every file in the six packages (`server.go`, `keys.go`, `proc*.go`, `awake*.go`, `prereq.go`,
  `hide_*.go`, `version.go`).
- Read the callers that decide what a tester actually sees: `internal/app/app.go:592` (awake),
  `internal/app/app.go:647-658` (prereq gate and the VCRedist fix), `internal/integrate/installer.go:41-70`
  and `log.go:149-161` (proc errors get wrapped), `cmd/ezlm/main.go:203-206` (`ezlm: <err>` on stderr)
  and `:232-235` (engine mode hands `Listen` errors to the native app).
- Verified the three `internal/web` HTTP paths with a throwaway test (deleted before commit, so the
  tree stays docs-only): a request with `Host: localhost:<port>` returned `403 forbidden`; a request
  with no token and one with a wrong token both returned `401 this page is out of date: reopen EZLM`.
  `internal/web` has no test file of its own, so nothing in the repo proves these today.

## Could not verify

- `internal/prereq` entries are written from the code and the app's rendering of `Check`, not from a
  run on each platform. Windows (`vcRedist`) and macOS (`macOS`) checks cannot execute on this
  darwin/arm64 box. The Linux `glibc` check and the NVIDIA branch were not run either, because the
  machine's backend is not CUDA.
- `internal/proc` behaviour on Windows (job objects, hidden consoles) is read-only: the job object
  paths only compile and run on Windows, so the entry describes the code, not an observed run.
- `internal/awake` on Windows (`PowerCreateRequest`) was not run.
- Entry 5's bind failure was not reproduced; `127.0.0.1:0` almost never fails, so the entry says so
  instead of guessing a cause.

## Open questions

1. `internal/web` has no test file. The 403/401 behaviour is security-relevant and only proven by my
   throwaway test. Suggest a permanent test in `internal/web` (owner of that path, not T13).
2. `streaming unsupported` is unreachable in practice because `net/http`'s writer is always a
   `http.Flusher`. It is still a real message, so it is documented, but it may be dead code.
3. `internal/awake` returns a no-op on every failure with no log line. A tester whose Mac sleeps
   mid-download gets no clue. Worth one `Logf` line, or a note in the FAQ.
4. `internal/version` has no error path. If the Lead wants only "real messages" in
   TROUBLESHOOTING, entry 26 can move to FAQ instead.
