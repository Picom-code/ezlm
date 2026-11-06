# T11 a05: `internal/report` tests

## What I did

Added `internal/report/report_a05_test.go` (new file only; `report.go` and the
baseline `report_test.go` are untouched). Tests are hermetic: no network, no
files, no real home. `Send` is exercised by swapping `http.DefaultClient` for a
fake `RoundTripper` and restoring it with `t.Cleanup`; the token is set through
`t.Setenv("EZLM_REPORT_TOKEN", …)` and the package `Token` global, both restored.

Cases:

- `TestSendWithoutTokenReturnsPrefilledURL`: no token means no request and the
  pre-filled browser URL, `Sent=false`.
- `TestSendFilesIssue`: a token POSTs to
  `https://api.github.com/repos/picom-code/ezlm/issues` with `Bearer`,
  `Accept: application/vnd.github+json`, `Content-Type: application/json`, the
  title/body/labels payload, and maps a 201 + `html_url`/`number` to `Result`.
- `TestSendTokenPrecedence`: `EZLM_REPORT_TOKEN` wins over the build-time
  `Token`; the build-time value is used when the env var is empty.
- `TestSendFallsBackWhenGitHubIsUnavailable`: transport error, 500, 401, and a
  201 with an unreadable body all return the pre-filled URL with `Sent=false`
  and no error, and the fallback URL does not contain the token.
- `TestNewIssueURLRoundTrip`: empty, plain, query-reserved characters
  (`& = ? # % + < > "`), unicode, and already-redacted text round-trip through
  `url.Parse` with `labels=beta-report`.
- `TestNewIssueURLTrimsLongBodies`: ASCII, symbol-heavy, three-byte runes,
  emoji, a multi-byte rune exactly at the cut, and a 1 MB body. Asserts the
  encoded body stays `<= maxBodyURL`, the result is valid UTF-8, the start and
  the `…(trimmed)` marker survive, and the kept text is a prefix of the input.
- `TestNewIssueURLKeepsBodiesUnderTheCap`: bodies at and just under the cap are
  returned unchanged.

## Coverage

`go test -cover ./internal/report/`:

| | before | after |
|---|---|---|
| total | 27.3% | 100.0% |
| `token` | 0.0% | 100.0% |
| `Send` | 0.0% | 100.0% |
| `NewIssueURL` | 100.0% | 100.0% |

## Bugs found

None. No `t.Skip("bug: …")` tests were added.

## Open questions

- `Send` hard-codes `https://api.github.com` and uses `http.DefaultClient`, so
  the only way to test it is to swap the global client. A package-level
  `apiBase`/`client` variable would make this cleaner; left alone because this
  slice is test-only.
- If GitHub creates the issue (201) but the response body is not JSON, `Send`
  reports the browser fallback, so a tester could file the same report twice.
  Current behavior is covered as-is; worth a look if duplicates show up.
- Trimming cuts 10% of the body per step, so a body barely over the cap loses
  far more than needed. Cosmetic only.
- The trimmer assumes valid UTF-8 input; invalid bytes are escaped but the
  trimmed body can remain invalid UTF-8. `app.Report` always passes strings, so
  this is not reachable today.
- Redaction itself lives in `internal/integrate` (`Redact`) and is applied by
  `internal/app.Report` before `report.Send`; `internal/report` has no redaction
  of its own. The tests only pin that a token never appears in the fallback URL.
