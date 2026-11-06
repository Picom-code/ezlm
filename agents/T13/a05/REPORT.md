# a05 report: keys and LAN mode

## What I covered

`agents/T13/sections/a05.md`, about 1,000 words, for a non-technical tester:

- Where the URL and key are (Overview > Connect with, and the end-of-setup "Your AI is live" screen),
  why the key is masked, and that **Copy** is the only way to get the full value.
- Where the key is stored (`~/.ezlm/config.json`, mode 0600), in plain words.
- How to copy it, plus the terminal fallback (`ezlm status`, `ezlm key`).
- The **Keys** page: new key, the first key being the one connected apps use, rotate, revoke, the
  "at least one key" rule, and the fact that every key change restarts the model server.
- What LAN mode exposes (the model API, not EZLM's own page), that the key still applies, the single
  request slot, and that the setting survives restarts and model switches.
- The security trade-off in plain words: off by default, the key is the only lock, plain HTTP on the
  LAN so the key and prompts are readable by anyone watching the network, use it on trusted networks.
- A short "if another device cannot connect" list, including the `127.0.0.1` mistake.

Every behavioural claim has a numbered citation in "Where this comes from" at the end of the section.

## Findings

1. **LAN mode has no firewall story, and EZLM adds no firewall rule.** (major for beta support)
   `SetLAN` only saves the config and restarts the server (`internal/app/app.go:990-1009`), and there
   is no `netsh`, `firewall` or rule-adding code anywhere in `internal/` or `windows/`
   (`grep -rni firewall --include=*.go --include=*.cs --include=*.ps1` returns nothing). On Windows a
   tester who ticks the box and connects from a phone may get a silent timeout with nothing in EZLM to
   explain it. The section tells them to allow the Windows prompt if it appears, and the report flags
   this as unverified (no Windows machine available here).

2. **The LAN address is chosen from the routing table, not from a list of interfaces.** (minor)
   `lanIP` dials a UDP address that sends no packets and takes the local address of the result
   (`internal/serve/config.go:183-190`). On a laptop with a VPN, Docker, Hyper-V or a tethered phone
   this can pick the wrong interface, and the printed **LAN URL** then does not work from the phone
   even though the checkbox is on. Not reproduced here; needs a multi-homed Windows machine.

3. **Plain HTTP carries the key.** (minor, accepted design)
   Both addresses are `http://` (`internal/serve/config.go:100`, `103-110`) and the key travels in
   the `Authorization: Bearer` header (`internal/serve/client.go:44`). Over LAN mode the key is
   readable by anyone who can watch the network. Documented in plain words; worth a line in the FAQ
   (a13) as well.

4. **The GUI never shows a full key.** (minor)
   Every key is rendered as first nine characters plus twelve dots (`windows/EZLM/Models.cs:324`,
   `windows/EZLM/Panes.cs:702`, `macos/Sources/EZLM/MainView.swift:617`). There is no "show" control,
   so a tester who wants to read the key to someone must press Copy. That is fine, but the section has
   to say it, or testers will think the dots are the key.

5. **The rotate/revoke dialogs do not warn that the model reloads.** (minor)
   The confirmation text only talks about the old value (`windows/EZLM/Panes.cs:705`,
   `macos/Sources/EZLM/MainView.swift:661`); the restart is mentioned in the note under the buttons
   after the action starts (`windows/EZLM/Panes.cs:735`). A tester mid-chat may be surprised by a
   failed request. The section warns them.

## What I could not verify

- The Windows Firewall prompt on first LAN connection (no Windows machine; no code path adds a rule).
- That a second device can actually reach the model with the LAN URL and key (would need two machines
  and a live server; the brief forbids starting llama-server).
- Which interface `lanIP` picks on a multi-homed machine (finding 2).
- Whether the root of the LAN port serves llama-server's own web page without a key. The section only
  claims the API is exposed and that EZLM's own page is not, both of which the code supports
  (`internal/web/server.go:75`).

## Open questions for the Lead

- Should the guide tell testers to add a Windows Firewall rule by hand, and if so for which executable
  (`llama-server.exe` in `%USERPROFILE%\.ezlm\bin`)? I left it as "allow the prompt if it appears"
  because I could not confirm the prompt or the binary path at runtime.
- Should LAN mode be described as "beta" in the guide? The code has no LAN tests
  (`grep -rn lan internal/app/*_test.go internal/serve/*_test.go` is empty), so it is the least
  tested path in this slice.
- The terminal fallback (`ezlm status`, `ezlm key`) is in the section for completeness. Cut it if the
  Lead wants a strictly GUI-only guide.

## Notes for other agents

- a06 (connecting apps): the address to paste is the **URL** row on Overview, not the **LAN URL**,
  when the app runs on the same computer. The key is the same either way.
- a09/a11 (messages): the key and LAN paths add three real strings to the troubleshooting set:
  `the server needs at least one key` (`internal/app/app.go:1249`), `that key no longer exists`
  (`internal/app/app.go:1241`, `1257`), and `nothing is deployed yet` from `SetLAN`
  (`internal/app/app.go:992`). The LAN toasts are `Other devices on your network can use it now.` and
  `Only this computer can use it now.` (`windows/EZLM/Panes.cs:748`,
  `macos/Sources/EZLM/AppModel.swift:995-996`).
