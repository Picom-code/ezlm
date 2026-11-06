# T13 a06 report: connecting OpenCode, Hermes and DeepSeek Harness

**Slice:** `agents/T13/sections/a06.md` (1,967 words including the source list; 25 table rows).
**Status:** complete, docs only. No code changed.

## What the section covers

1. What the three apps are, in one line each.
2. The one-click path: start the server, open **Connect**, pick tiles, the default-model checkbox,
   press the named Connect button, read the tile ("✓ Connected and answering"), then press **Open**.
3. The URL and key to paste, with where to copy them (Overview > Connect with, Keys, the first-run
   wizard), the LAN URL, and the "key is a password" note.
4. A table of what EZLM writes into each app and where (config paths, how the key is stored), so a
   tester can find it and knows not to hand-edit EZLM's entry.
5. What happens after a model switch, key change or port move (EZLM rewrites connected apps).
6. What to do when EZLM's model is not the app's default.
7. A 14-row common-mistakes table, keyed by what the tester sees.
8. The CLI equivalent (`ezlm connect`, `ezlm doctor apps`).

## Claims and the code that proves them

| Claim in the section | Code |
|---|---|
| Connect installs, configures, sends a real answer with a tool call, runs a preflight and a hidden terminal test | `internal/integrate/integrate.go:100` (`Setup`), `:128` (progress), `:169` (check message) |
| Connect needs the model running; button disabled otherwise | `windows/EZLM/Panes.cs:564` (pane), `:628` (button enable), `:630` ("Start your server first"); `macos/Sources/EZLM/MainView.swift:568`, `:584` |
| Default-model checkbox, ticked by default | `windows/EZLM/Panes.cs:624`; `macos/Sources/EZLM/MainView.swift:571` |
| "✓ Connected and answering" | `windows/EZLM/Panes.cs:676` |
| Overview says **Ready** / **The server is stopped**, and Start server is on Overview | `windows/EZLM/Panes.cs:315`, `:323`; `macos/Sources/EZLM/MainView.swift:304` |
| URL is `http://127.0.0.1:<port>/v1`, port is not fixed | `internal/serve/config.go:100`; `internal/app/app.go:797` (`FreePort(max(cfg.Port, 8080))`) |
| LAN URL exists only while LAN mode is on | `internal/serve/config.go:103` |
| Key shape `ezlm-` + 40 base62 | `internal/keys/keys.go:11` |
| Key row shows first 9 characters plus dots, Copy copies the whole key | `macos/Sources/EZLM/Wizard.swift:391`; `windows/EZLM/Install.cs:346` |
| First key is "used by connected apps" | `windows/EZLM/Panes.cs:700`; `macos/Sources/EZLM/MainView.swift:614` |
| OpenCode: provider `ezlm`, name "EZLM (local)", `{file:}` key, config in `~/.config/opencode` | `internal/integrate/opencode.go:41`; `internal/integrate/opencode_config.go:87`, `:91`, `:25` |
| OpenCode workspace file picks EZLM's model (2.x has no `--model`) | `internal/integrate/opencode_config.go:138`; `internal/integrate/opencode.go:130` |
| Hermes: provider `custom:ezlm`, 64K floor, key in `api_key` | `internal/integrate/hermes.go:44`, `:45`, `:196`; `internal/integrate/hermes_config.go:230` |
| dsh: provider `ezlm`, `EZLM_API_KEY` in `.env` (0600), profiles headless/web/desktop | `internal/integrate/dsh_config.go:122`, `:25`, `:494`, `:38` |
| dsh browser on port 3080, one-time token, closing the window stops it | `internal/integrate/dsh_web.go:26`, `:20` |
| dsh needs Node 22.19.0+ | `internal/integrate/dsh.go:50` |
| Apps are rewritten on model switch, key change and port move | `internal/app/app.go:1578` (`rewriteApps`) |
| "Points at an old address or key: EZLM fixes it when you open it" | `windows/EZLM/Models.cs:186`; `macos/Sources/EZLM/Models.swift:209` |
| Open starts the server, connects first if needed, rewrites a stale config, then opens | `internal/app/launch.go:28` |
| CLI `ezlm connect` / `ezlm doctor apps` | `cmd/ezlm/main.go:617`, `:639` |

## Findings (docs-level; no code edited)

1. **Minor: the Connect completion note assumes the checkbox was unticked.** After a successful
   connect the note reads "Done. Open the app and pick the model named EZLM."
   (`windows/EZLM/Panes.cs:662`), but with **Make it the default model in those apps** ticked
   (the default) EZLM has already made its model the default. The note makes testers hunt for a
   step they do not need. Proposed: "Done. EZLM's model is ready in those apps." when
   `makeDefault`, else the current sentence.
2. **Minor: needs_update says "old address or key", but the check also covers model, limits and
   thinking levels.** `internal/integrate/opencode_config.go:211-234` returns reasons for an old
   address, key, model, limits and reasoning variants, all under one tile sentence
   (`windows/EZLM/Models.cs:186`). Testers who see it after switching model may think only the key
   changed. Proposed: "EZLM's settings in this app are out of date: EZLM fixes them when you open
   it." (feeds a14.)
3. **Minor: the visible Copy button is labelled "Copy"; "Copy URL" / "Copy Key" are the
   automation names.** `windows/EZLM/Install.cs:355` passes `$"Copy {k}"` as the automation
   name; `macos/Sources/EZLM/Wizard.swift:407` uses `Copy \(k)` as the accessibility label. The
   section says "press its Copy button", which matches what is on screen.
4. **Note: "Your AI is live" is the first-run wizard's line, not Overview's.** Overview says **Ready** / **Waiting for requests from your apps** (`windows/EZLM/Panes.cs:323`). A tester told to look for "Your AI is live" on Overview would not find it, so the section uses the Overview words and points at **Start server**.
5. **Note: Windows testers will look in `%APPDATA%` for OpenCode's config and not find it.** The
   config is `~/.config/opencode` on every OS (`internal/integrate/opencode_config.go:25`). The
   section says this explicitly; it is worth keeping in the final guide.

## What I could not verify

- **The real apps.** Per the rules I loaded no model and started none of the three apps, so the
  in-app wording for picking a model (OpenCode's model list, Hermes `/model`, the dsh settings
  screen) is from the apps' documented behaviour, not observed. The config shapes EZLM writes are
  verified in code.
- **OpenCode 2.x per-project model choice.** `writeOpenCodeWorkspace` writes
  `EZLM Workspace/opencode.json` (`opencode_config.go:138`) and the launch runs in that folder
  (`opencode.go:140`), so EZLM's window uses EZLM's model. Whether a session started elsewhere
  keeps the global default is OpenCode's behaviour, not EZLM's; the section says "make it once in
  `EZLM Workspace`" and does not claim more.
- **dsh browser sign-in reuse.** EZLM reuses a `dsh web` it started and opens
  `http://127.0.0.1:<port>/` (`dsh_web.go:107`, `:111`); the one-time token lives in the terminal
  window dsh printed it in. If the browser lost its cookie while that terminal is still open, the
  page asks for authentication and the user has to find the link in the terminal. I documented the
  symptom ("dsh web authentication required") and the fix (open it again / use the printed link),
  but I could not test the lost-cookie path.
- **Hermes profile switching.** `hermesConfigPath` follows `HERMES_HOME` and the active profile
  (`hermes_config.go:31-60`); I did not document switching profiles mid-connection beyond the
  "press Connect again" advice.

## Open questions for the Lead

- Should the assembled BETA.md keep the "What EZLM writes" table? It is the most technical part of
  the section; it is useful for troubleshooting and for a14, but it could move to
  TROUBLESHOOTING.md.
- The source list at the end of the section is marked "drop when publishing". If the assembly
  script keeps whole sections verbatim, the Lead needs to strip that block.
- a05 (keys and LAN) overlaps on the URL/key/LAN paragraphs. Keep a05's detail and leave this
  section's short version, or cross-reference instead of repeating.

## Checks run

Docs only. `gofmt -l .` printed nothing; `go build ./...` passed in the worktree (sanity only).
