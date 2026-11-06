# T08 a15: shared fake GitHub

## What this is

`internal/update/fakegithub_test.go` is one `httptest` fixture that stands in for both
GitHub's release list (`/repos/picom-code/ezlm/releases`) and the release assets
(`/download/<name>`), so the fault agents (a01 to a10, a13) do not each rebuild one.
It is in package `update`, so every `*_test.go` in `internal/update` can use it.

It starts with one good release, `v0.1.2`, carrying an `EZLM-Setup-0.1.2.exe` and an
`EZLM-0.1.2.dmg`, both with a correct sha256 digest. A test then changes one knob and
asserts what the updater does.

## Use

```go
fg := newFakeGitHub(t)   // v0.1.2, both installers, correct digest
fg.Use()                 // sets EZLM_UPDATE_URL and a private TMPDIR (Download's temp dir)
info := Check(t.Context(), "0.1.0")

fg.Installer()           // the asset Check picks on this computer
fg.Asset("EZLM-0.1.2.dmg")
fg.Release("v0.1.3", body) // add a release with both installers
fg.Add(&fakeRelease{...})  // add one exactly as given
fg.OnAPI(func(w http.ResponseWriter, r *http.Request) { ... })     // rate limits, redirects, junk JSON
fg.OnAsset(name, func(w http.ResponseWriter, r *http.Request) { ... }) // truncation, drip, redirects
fg.APIHits(), fg.AssetHits(name), fg.Hits(path)                   // "must not spin" assertions
fg.Reset()
```

Knobs on `fakeAsset`: `Body`, `Size` (release-list size), `Digest` (release-list digest),
`URL` (release-list URL), `OmitDigest` (sends `"digest": null`, as GitHub does when it
has none), `Status`, and `Serve` for any bytes-level fault. `fakeRelease` has `Tag`,
`Name`, `Body`, `Page`, `Draft`, `Prerelease`, `Assets`. The default asset serving
honours `Range`, so resume tests work.

The returned release/asset pointers are live; change them between requests. `Use`
calls `t.Setenv`, so a test that uses it must not be parallel.

## Evidence

`TestFakeGitHub` (in the same file) proves the fixture itself: a good release checks
and downloads, the bytes match, a wrong digest in the release list makes `Download`
refuse, a 403 from `OnAPI` surfaces as `Info.Error`, and the hit counters count one
request per call. `TestFakeGitHubAssetHooks` proves the asset side: `Range` gets a 206
with the tail, and `OnAsset` replaces the bytes and headers. Both pass:

```
go test -p 2 -run 'TestFakeGitHub' -v ./internal/update/
```

## Notes for the fault agents

- The updater reads `digest` as a string; a missing digest arrives as `null` and leaves
  it empty, which is what `OmitDigest` sends.
- `Check` picks the installer by `runtime.GOOS`; `fg.Installer()` returns the same
  asset, so tests stay platform-neutral.
- `Download` writes under `os.TempDir()/EZLM-update`; `Use()` points `TMPDIR` at
  `t.TempDir()`, so tests do not touch the real temp directory. Set `TMPDIR` yourself
  after `Use()` if a test needs a different one (a09).
- `Download` returns an existing file without refetching when it is the expected size.
  A test that changes the digest after a successful download must remove the file or
  use a fresh `TMPDIR`; the self-test does the refused download first for this reason.
- `Newer` and `Check` ignore `prerelease` today; the fake sends it, so a06 can show it.

## Residual risk

- If a fault agent writes its own fake before this lands, the merge keeps both files;
  duplicate helpers with the same names in package `update` would not compile. Names
  here are `fakeGitHub`, `fakeRelease`, `fakeAsset`, `newFakeGitHub`, `apiPath`.
- The fixture is test-only code in package `update`; it adds no production behaviour.
