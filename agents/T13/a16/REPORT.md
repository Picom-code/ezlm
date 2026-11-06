# a16 report: read-through and cross-check

## Deliverable

`agents/T13/a16/review.md`. Docs only; no code changed.

## What I covered

- The sections that existed when I ran: a01 to a12 (about 15,000 words), read from each agent's
  worktree at its committed state. a13, a14 and a15 had not landed, so the assembled `BETA.md` was
  not available to review; the review says so and gives the Lead four checks to run afterwards.
- The root `BETA.md`, because a01, a02 and a08 cite it as their source and because the guide
  replaces it.
- Cross-checking method: every quoted message, path, label, minimum version and file name was
  compared with the code in this worktree, not with the other documents. Six claims (the ones I call
  "verified") were read at the exact line.

## What I found

Six contradictions worth fixing before assembly, all with file:line in the review:

1. `BETA.md` names the Windows download `EZLM-Setup.exe`; the shipped file is
   `EZLM-Setup-0.1.11.exe` (`windows/installer/EZLM.iss:15`).
2. `BETA.md` says uninstall offers to delete the models; the real prompt also deletes settings and
   keys, and Yes deletes them (`EZLM.iss:94`, `internal/app/app.go:1586-1605`).
3. `BETA.md` says the Mac app is Apple Silicon only; the app is universal and Intel works
   (`macos/scripts/build-app.sh:12,29,36`).
4. `BETA.md` says "Windows 10 or 11"; the installer requires build 19041, version 2004
   (`EZLM.iss:30`).
5. The Mac starts the model at login from a launchd agent (`internal/serve/service_darwin.go:44-70`),
   and no section tells a Mac tester that or how to stop it. `BETA.md` gives the Windows path only.
6. The Smart App Control paragraph has no source at all: a01 cites `BETA.md`, `BETA.md` cites
   nothing, and no code path can prove an operating-system behaviour.

Plus: two different macOS floors (14.0 app, 13.3 engine) that need framing, four wordings of the
Hermes 64K message where a12 documents one, the "Free up memory" button being called "Model mode" in
a04, reports keeping the first 9 characters of a key, a model size range of 0.55 to 76.9 GB against a
documented "2 to 60", and three citation problems in a11.

## What I could not verify

- Windows-only and Mac-only behaviour by running it: this box is darwin/arm64 and the rules forbid
  starting a model. Every Windows claim is read from code, not observed.
- The SmartScreen, Smart App Control and Gatekeeper wordings. They are operating-system text; the
  review asks for a source or a hedge rather than inventing one.
- a13, a14 and a15, which did not exist yet.
- a01, a03, a07, a09 and a10 got a targeted read (grep each quoted string back to code), not a line
  by line read. The review labels them "not contradicted" rather than "verified".

## Open questions

1. Does the beta doc or the guide want to keep saying "not code-signed yet" at all? If the signing
   lands before the beta, five paragraphs across four sections become wrong at once.
2. Should the Mac's login item get a UI control (a checkbox) instead of a documented `launchctl`
   recipe? That is an app change, not a docs change, but the guide is the only place a tester would
   learn about it.
3. Is "often 2 to 60 GB" a deliberate simplification for a non-technical reader? If so, it should
   still say "up to about 77 GB" somewhere, because a 64 GB laptop is a common tester machine.
