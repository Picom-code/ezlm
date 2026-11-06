# T11 a12: `internal/bench` tests

## What I did

Added one new test file, `internal/bench/parse_test.go` (no logic changes), with table tests
for the parsing and formatting paths in `internal/bench/bench.go`:

- `TestClip` / `TestClipKeepsValidUTF8`: whitespace collapsing, truncation at and past the
  limit, empty input, zero width, multibyte output.
- `TestChatParsesReply`: request path, bearer key, model in body, and decoding of
  `choices[].message` (content, `reasoning_content`, `tool_calls`) and `timings`.
- `TestChatErrors`: non-200, malformed JSON, empty `choices`; each returns a nil reply and a
  useful error.
- `TestFailuresAreReported`: all five checks turn a server error into a named `Result` with
  the reason in `Detail`, not a zero value.
- `TestToolCallResult`: correct call, wrong city, wrong tool, no call, unreadable arguments.
- `TestJSONFormatResult`: plain JSON, ` ```json ` fence, bare fence, wrong year, wrong person,
  prose.
- `TestJSONFormatUppercaseFence` (skipped bug, below).
- `TestJSONFormatNotesThinkingOff`: a second try with thinking off is noted in the detail.
- `TestAskKeepsFirstAnswerWhenRetryFails`: the first answer survives a failed retry.
- `TestRunReportsEveryResult` / `TestRunWithoutReporter`: `Run` executes all five checks in
  order, reports each as it finishes, formats the speed and recall details, and works with a
  nil reporter.

All tests use `httptest` only. No files, no network, no model.

## Coverage

`go test -cover ./internal/bench/`: **48.2% before, 99.1% after**.

Per function after: `chat` 95.0%, everything else 100%. The `chat` remainder is the
`json.Marshal`/`http.NewRequestWithContext` error branches, which cannot fail for the maps and
URLs this package builds.

## Bugs found (left as skipped tests, per the brief)

1. **minor** `bench.go:257` `clip` truncates at a byte offset, so a multibyte character can be
   split and the detail text becomes invalid UTF-8. Model answers are free text, so this is
   reachable. Test: `TestClipKeepsValidUTF8`
   (`clip("café ☕ time", 4)` returns a broken rune). Fix: cut on a rune boundary, e.g. build
   the prefix with `strings.Builder` over `[]rune` or back off to the last valid rune.
2. **minor** `bench.go:219` `jsonFormat` strips only a lowercase ` ```json ` prefix. A model
   that answers with ` ```JSON ` (or ` ``` json `) fails a correct answer with "strict JSON:
   ..." even though the JSON is valid. Test: `TestJSONFormatUppercaseFence`. Fix: compare the
   fence prefix case-insensitively, or use a small regexp/`strings.TrimSpace` loop.

Both are ownerless (T11 is a test task), so they are left skipped for the morning merge; they
are not in the T07 web/keys area.

## Not covered / open questions

- `Run` does not check `ctx` between checks, so a cancelled context still starts the next
  request. There is no test for it because the observable behaviour is a slow run, not a wrong
  result; it is a design question for the bench owner.
- `chat` mutates the caller's request map (`body["model"] = t.Model`). `ask` copies before its
  second call, so nothing breaks today; a test would only pin the side effect.

## Checks run

`gofmt -l .` (empty), `go vet ./...`, `go build ./...`, `go test -p 2 ./internal/bench/`.
