# T08/a11: threat model for the in-app updater

Scope: `internal/update/**` and the two install call sites that consume it
(`windows/EZLM/SettingsPane.cs`, `macos/Sources/EZLM/Updater.swift`). Written
against the baseline snapshot (commit `5213370`), running version
`0.1.11-beta` (`internal/version/version.go:5`).

The goal T08 states is a single sentence: *the updater can never brick an
install or install something it shouldn't.* This document says what that
sentence means concretely, where the current code already holds, and where it
does not.

---

## 1. What the updater is

Two halves, both driven by the Go engine:

- **Check** (`internal/update/update.go:56`): GET a GitHub releases list,
  pick the newest release that has an asset for this OS, compare its tag with
  the running version, report `Available`.
- **Download** (`internal/update/update.go:111`): fetch that asset into
  `os.TempDir()/EZLM-update/<basename>` via `engine.Download`, then verify
  SHA-256 if the release published one.

Then one half in each app, outside the Go code:

- **Windows** (`windows/EZLM/SettingsPane.cs:293`): `Process.Start` the
  downloaded `.exe` with `/SILENT /SUPPRESSMSGBOXES /NORESTART /SP-
  /CLOSEAPPLICATIONS /UPDATE=1`, then close the window ~1.2 s later.
- **macOS** (`macos/Sources/EZLM/Updater.swift:14`): attach the `.dmg`,
  `ditto` `EZLM.app` next to the running bundle, rename old aside, rename new
  in, delete old, relaunch.

Everything below follows from that shape: **the Go engine decides *what* to
install, the app decides *how* to install it, and nothing checks the installer
after it starts.**

---

## 2. What the updater trusts

| # | Input | Where it enters | Trusted for | What breaks if it lies |
|---|-------|-----------------|-------------|------------------------|
| T1 | GitHub API over TLS | `update.go:64-66` (`http.DefaultClient`, default TLS verify) | the whole release list: tags, URLs, sizes, digests | total: this is the root of trust |
| T2 | `EZLM_UPDATE_URL` env | `update.go:59-61` | replaces the API URL | any process that can set the app's environment redirects the check; the URL need not be GitHub |
| T3 | `tag_name` | `update.go:77`, trimmed at `:94` | the version to compare and install | a tag that parses low is silently "not newer"; one that parses high is offered |
| T4 | `draft` | `update.go:81`, filtered at `:95` | "this release is not for users" | a draft is offered as an update |
| T5 | `prerelease` | **not decoded at all** (`update.go:76-88`) | nothing | beta releases reach stable users; this is the current documented policy (`update.go:19-21`) but it is a comment, not a control |
| T6 | `assets[].name` | `update.go:99`, `assetFor` `:45-53` | which file is *this platform's* installer | prefix/suffix match only; no arch, no content check |
| T7 | `assets[].browser_download_url` | `update.go:100` → `:117` | where bytes come from | any host; redirects are followed (`dlClient`, `engine/download.go:25-32`); TLS to that host is the only check |
| T8 | `assets[].size` | `update.go:100` → `engine/download.go:44` | exact byte count | if 0, no size check at all (`download.go:45`, `:87`) |
| T9 | `assets[].digest` | `update.go:100` → `engine/engine.go:320` | content integrity | if absent or not `sha256:`, verification is **skipped and the download is accepted** (`engine.go:321-324`) |
| T10 | the downloaded file on disk | `update.go:116-124` | that this is the installer | a pre-existing file of the right size is reused without a fetch (`download.go:45-50`); only the digest catches it, and only if T9 exists |
| T11 | the temp path itself | `os.TempDir()/EZLM-update`, mode `0o755` (`update.go:112-113`) | nothing | predictable, world-readable, never cleaned; on a shared Mac another account can create it first and pre-seed a file |
| T12 | the install path handed to the app | `internal/app/settings.go:52-58` event `msg` | that this is the file `Download` produced | a local process that can post engine events can name any `.exe`; guarded only by the per-launch API token (`internal/web/server.go:247-254`) |
| T13 | the downloaded `.dmg`/`.exe` contents | Mac `Updater.swift:24-29`; Windows `SettingsPane.cs:302` | that it is EZLM | Mac checks only that `Contents/MacOS` exists and `CFBundleIdentifier` matches; Windows checks nothing |

The important asymmetry: **T1 through T9 all come from one HTTPS response.**
The digest is GitHub's own asset digest, which is better than a hand-uploaded
checksum file, but it travels in the same response as the URL it vouches for.
It detects corruption, CDN mixups and a swapped asset; it does **not** detect
a compromised repo account, a compromised API response, or a hostile
`EZLM_UPDATE_URL`. Authenticity rests entirely on TLS to `api.github.com`
plus GitHub account security.

There is **no code-signature or notarization check anywhere in the tree**
(`grep -rn "codesign\|notariz\|spctl\|quarantine"` finds nothing under
`internal/`, `macos/Sources/EZLM`, or `windows/EZLM`). Because the app
downloads with Go's HTTP client rather than a browser or `URLSession`, the
`com.apple.quarantine` xattr is never set, so **macOS Gatekeeper is not
consulted for updates at all**. On Windows, SmartScreen would only see the
`.exe` because `Process.Start` uses `UseShellExecute` (`SettingsPane.cs:302`).

---

## 3. Failure windows

A window is a point where a crash, power loss, or a lying server leaves state
that the user cannot easily recover from. Ordered by the pipeline.

### Check

| W | Window | What a failure leaves |
|---|--------|----------------------|
| W1 | 403/429 from the API (`update.go:72-75`) | `info.Error` set; `UpdateInfo` re-checks on **every** call while `Error != ""` (`internal/app/settings.go:36-38`), so a rate-limited API is hammered rather than backed off |
| W2 | API slower than the 10 s budget (`update.go:62`) | generic error; the budget covers the JSON decode too |
| W3 | 200 with a truncated or non-array body (`update.go:89-92`) | "couldn't read the release list"; no state change |
| W4 | release list with no asset for this OS | `Latest == nil`, `Available == false`, UI says "up to date" — a silent false negative, indistinguishable from success |

### Download

| W | Window | What a failure leaves |
|---|--------|----------------------|
| W5 | partial transfer | `*.part` in `/tmp/EZLM-update`; resumed next run (`download.go:51-55`, `:129-131`) |
| W6 | server answers `416` | treated as "already complete" (`download.go:148-149`); only the size check at `:87-96` catches a short `.part`, and only when `want > 0` |
| W7 | size mismatch | `.part` deleted, error surfaced (`download.go:92-95`) |
| W8 | digest mismatch | `dst` deleted (`update.go:121`), so the next attempt re-downloads fully |
| W9 | process dies after `rename(part, dst)` (`download.go:97`) | a complete file in temp; the next run reuses it after the size check, and re-verifies the digest |

### Install, Windows

| W | Window | What a failure leaves |
|---|--------|----------------------|
| W10 | `Process.Start` throws (`SettingsPane.cs:308-312`) | app stays open, error shown — the clean case |
| W11 | installer starts, then fails or is declined by UAC | `/SILENT /SUPPRESSMSGBOXES` (`:302`) hides it; the window closes 1.2 s later (`:305-306`) regardless, so EZLM disappears and the user gets no message. Whether the old install still runs depends entirely on the installer |
| W12 | installer races the old engine's shutdown | `StopEngineHelpers` (`:269-288`) kills only processes whose `MainModule` path equals this install's engine; `/CLOSEAPPLICATIONS` is the fallback. A locked `Engine\ezlm.exe` is the classic half-updated state |

### Install, macOS

| W | Window | What a failure leaves |
|---|--------|----------------------|
| W13 | `hdiutil attach` fails (`Updater.swift:20`) | nothing changed; error shown |
| W14 | dmg has no `EZLM.app`, or wrong `CFBundleIdentifier` (`:24-29`) | nothing changed |
| W15 | `ditto` copy fails (`:38-41`) | staged copy removed; nothing changed |
| W16 | **rename old aside succeeds, rename new in fails** (`:44-52`) | restore is `try?` (`:49`) — **best effort**. If the restore also fails, `/Applications/EZLM.app` does not exist |
| W17 | **crash between `:44` and `:48`** | no `EZLM.app` at the path; only `.EZLM-old-<pid>.app` and `.EZLM-update-<pid>.app` hidden in `/Applications`. Finder shows no EZLM. Recovery is a manual rename, and the `<pid>` is not known to the user |
| W18 | relaunch script fails (`:59-68`) | app has quit; user must reopen it by hand. The new bundle is in place, so this is cosmetic |

W16/W17 are the only windows in the whole pipeline that can genuinely
"brick" an install. Both are microseconds wide (two `rename(2)` calls on the
same volume), which is why they have not been seen, but they are real and
they are not journalled. Detail and the exact byte-level sequence belong to
a14 (`agents/T08/a14/replace.md`).

---

## 4. Invariants: "never worse than before"

The phrase needs to be made testable. These are the invariants the updater
should hold. Status is against the baseline.

**I1. Nothing is executed until a complete local file exists.**
The installer path is only produced by `Download` (`update.go:124`) and both
apps re-check that it exists before using it (Mac `Settings.swift:66-70`;
Windows starts it only on a `download ok` event, `SettingsPane.cs:295-302`).
*Status: enforced.*

**I2. A failed check or download changes nothing outside the temp directory.**
All writes are under `os.TempDir()/EZLM-update`; digest failure deletes the
file (`update.go:121`). The running app is untouched.
*Status: enforced.*

**I3. Only a strictly newer version is ever installed.**
`Check` gates on `Newer(Latest.Version, current)` (`update.go:105`).
*Status: partly.* The install path re-reads a cache that can be up to an
hour old (`internal/app/settings.go:36-38`, `:45-49`) and never re-validates
the version at install time. `Newer` (`update.go:128-145`) is also not a
total order on malformed input: `split` (`:147-158`) ignores `Atoi` errors
and drops components past the third, so `1.2.3.4` compares equal to `1.2.3`
and `v` compares equal to `0.0.0`. a05 and a07 own the proof.

**I4. The bytes installed are the bytes the release described.**
*Status: partly.* SHA-256 is checked when present, exact size when present
(`update.go:117-123`). Both are optional: a release with no digest and
`size: 0` is installed with **no integrity check at all**, and a digest of
any algorithm other than `sha256:` is silently skipped rather than refused
(`engine/engine.go:321-324`). a04 owns this.

**I5. A crash never leaves the app unreachable on macOS.**
*Status: partly.* W16/W17 above; the restore path is `try?`.

**I6. A crash never leaves the app unreachable on Windows.**
*Status: partly.* W11/W12; the app closes itself before it knows the
installer succeeded, and never reads the installer's exit code.

**I7. Repeated attempts converge; a failing server is not hammered.**
*Status: partly.* Download retries are bounded (five failures without
progress, `download.go:78-80`) and paced (`:84`). Check is not: an error
defeats the one-hour cache (`settings.go:37`), so every UI refresh re-hits
the API. a01 owns this.

**I8. Temp state is never executed and does not accumulate without bound.**
*Status: partly.* Temp files are never executed. But `/tmp/EZLM-update` is
never cleaned, and its contents are reused across runs on a size-only match
(`download.go:45-50`) — a stale or pre-seeded file is trusted as far as I4
lets it be.

---

## 5. Current controls (what actually exists today)

| Control | Where | Covers |
|---------|-------|--------|
| TLS with default verification, hostname checked | `update.go:66`, `engine/download.go:25-32` | T1, T7 (content in transit) |
| 10 s budget on the check | `update.go:62` | hung API |
| Drafts skipped | `update.go:95` | T4 |
| Newest-wins selection by version, not by list order | `update.go:93-104` | a reordered API response |
| Platform asset filter (name prefix/suffix) | `update.go:45-53` | T6, weakly |
| Exact size check when size is known | `engine/download.go:45`, `:87-96` | T8 |
| SHA-256 when published | `engine/engine.go:320-337` | T9 |
| Resume from `.part`, restart on `200` | `engine/download.go:51-55`, `:143-147` | long downloads, proxies |
| Stall watchdog (45 s of no bytes) | `engine/download.go:21`, `:100-123` | slow-drip servers |
| Bounded retries with backoff | `engine/download.go:57-86` | transient failures |
| Permanent-error classification (404/403/410) | `engine/download.go:197-199`, `:237-247` | pointless retries |
| Error text that names the next action | `engine/download.go:208-234` | user recovery |
| Download is a job with progress and cancel | `internal/app/settings.go:51-59` | UI responsiveness |
| Mac: bundle ID must match before the swap | `macos/Sources/EZLM/Updater.swift:27-29` | wrong-app dmg |
| Mac: stage, then two renames, then delete old | `Updater.swift:31-53` | partial copies |
| Windows: stop only this install's engine processes | `windows/EZLM/SettingsPane.cs:269-288` | file-in-use |
| Per-launch API token on the local control plane | `internal/web/server.go:247-254` | T12 |

What is **not** there: any signature or notarization check, any allow-list on
the asset host, any re-validation of the version at install time, any
observation of the installer's exit code, any journal for the Mac swap, and
any cleanup of the temp directory.

---

## 6. Gaps, mapped to the fault agents

| Gap | Trusted input | Invariant at risk | Owner |
|-----|---------------|-------------------|-------|
| 403/429 and secondary rate limits produce a generic error and defeat the cache, so the API is re-hit on every refresh | T1 | I7 | a01 |
| Redirects are followed to any host, for the API and for the asset; the asset target can change between check and download | T1, T7 | I4 | a02 |
| A truncated body is caught only by the size check, which is skipped when `size == 0`; `416` is trusted | T8 | I4 | a03 |
| Digest is optional, algorithm is not checked, a non-`sha256:` digest is silently ignored | T9 | I4 | a04 |
| No re-check at install time; `Newer` accepts malformed tags as `0.0.0`; `1.10` vs `1.9` is numeric so this one is fine, but `1.2.3.4` == `1.2.3` | T3 | I3 | a05 |
| `prerelease` is never decoded; only `draft` is filtered. Stable users get betas | T5 | I3 | a06 |
| `split` ignores parse errors and drops components; `Newer` is not a total order | T3 | I3 | a07 |
| No cap on the asset size, no cap on the API body, no host allow-list after a proxy | T1, T7 | I4 | a08 |
| Disk-full during write leaves a short `.part`; the old install is untouched but the error text says nothing about space | T10 | I2 | a09 |
| Asset selection is a name prefix/suffix only; the Mac checks bundle ID but nothing checks arch, executable magic, or archive layout | T6, T13 | I4 | a10 |
| The Mac two-rename window and the Windows fire-and-forget installer | T13 | I5, I6 | a14 |
| No signature/notarization check; Go's download sets no quarantine bit, so Gatekeeper is bypassed | T13 | I4 | *unowned tonight* |
| `/tmp/EZLM-update` is predictable, world-readable, never cleaned, and reused on a size-only match | T11 | I8 | *unowned tonight* |

---

## 7. Residual risk after tonight's fixes

Even with every fault above fixed, the updater's authenticity model stays:
**HTTPS to GitHub, plus a digest that GitHub computed, plus a repo account
nobody has phished.** That stops corruption and CDN mixups. It does not stop
a compromised `picom-code/ezlm` release, and it does not stop a local
attacker who can set the app's environment (`EZLM_UPDATE_URL`) or pre-create
`/tmp/EZLM-update`. Closing that class needs a signing key held off GitHub and
a signature check in both apps, which is out of scope for T08 but should be
on the list before the public beta.

The second residual risk is social: because W11 and W16/W17 are silent, a
failed update looks like EZLM simply quit. Both apps should report the
installer's exit code, and the Mac should leave a note (or a marker file) when
the swap did not complete.

---

## 8. Open questions

1. **Is "beta for everyone" the intended channel policy?** `update.go:19-21`
   says yes, and `0.1.11-beta` is what ships. If a stable channel is ever
   wanted, `prerelease` must be decoded and a policy chosen; today the field
   is ignored entirely.
2. **Should a missing digest be fatal?** GitHub has published asset digests
   for some time, so "missing" mostly means an old release. Refusing would
   make old releases uninstallable; accepting keeps the current hole. a04
   should decide and document.
3. **Who cleans `/tmp/EZLM-update`?** Nothing does. It is small (one
   installer per version) but unbounded across versions.
4. **Should `Download` re-check the version against the running binary?**
   The cache makes a check-then-install race possible; re-validating at
   install time is cheap.
5. **Does the Windows installer report failure?** Nothing in this tree reads
   its exit code. If Inno Setup is configured to log, the app could read the
   log; a14 should say whether that is practical.
