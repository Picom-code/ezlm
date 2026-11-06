# T13 a07 report: Chat

Slice: the built-in chat, what it can do, and its limits. Deliverable: `agents/T13/sections/a07.md` (about 1,150 words). Docs only, no code changed.

## What I covered

- Where Chat is, that it uses the local server with the key already filled in, and what the top line shows (state, chat size, speed, thinking switch, memory, model switch, history, new chat).
- Sending, stopping, the up-arrow history, the slash commands, and the example questions.
- Reading an answer: markdown, code copy, Listen, Retry, Continue, the stats line, thinking folded away.
- Attaching text files: what is accepted, the 2 MB cap, PDF and Word not read, the token budget check.
- Memory: the fact list, the automatic quiet check after an answer, `/remember` and `/forget`, the memory sheet, the 1,500-character budget, the instruction and secret filters.
- Saved chats: where they live on each platform, restoring the last chat from the past week, History, Copy this chat.
- Limits: one conversation, the context cap and older messages left out, answers cut at the model's limit, text only, no browsing, quitting mid-answer, sharing the server with other apps.
- The chat error messages with what to do, pointing to TROUBLESHOOTING for the full list.

Citations are inline in the section and repeated here.

## Findings

1. **Windows `/help` leaks a class name into the toast.** Windows shows `ChatLogic.Commands: /remember <fact> ...` while macOS shows `Commands: /remember <fact> ...`. Severity: minor. Evidence: `windows/EZLM/Chat.cs:773-776` against `macos/Sources/EZLM/ChatModel.swift:415`. A tester who types `/help` on Windows sees an internal name. Docs say "type / for commands" and do not quote the toast, so this does not block the guide; it is worth a one-word fix in T12's area.
2. **The chat size meter resets when an old chat is reopened.** `ctxUsed` / `chatCtxUsed` is set to 0 on restore, so a reopened long chat shows an empty bar and no "nearly filled" warning until the next answer. Severity: minor. Evidence: `macos/Sources/EZLM/ChatModel.swift:652`, `windows/EZLM/Chat.cs:1139`. Documented behaviour in the section is still true for a live chat; a tester may notice the empty bar on an old chat.
3. **The "couldn't read a file" wording differs by platform.** macOS says `Couldn't read NAME.`; Windows appends the system message: `Couldn't read NAME: <message>`. Severity: minor. Evidence: `macos/Sources/EZLM/ChatModel.swift:485` against `windows/EZLM/Chat.cs:1052`. The section does not quote either.
4. **macOS has no "no voice installed" message.** Windows catches a speech failure and says `This computer has no voice for reading aloud (add one in Windows Settings > Time & language > Speech).`; the macOS path has no equivalent message, so Listen can fail quietly. Severity: minor. Evidence: `windows/EZLM/Chat.cs:976`; `macos/Sources/EZLM/ChatModel.swift:599-617`. The section only promises the Windows message.
5. **A restored answer keeps its old model label but not the model.** Answers are labelled with the model that wrote them (`ChatMsg.model`), and restore copies it, so a chat restored after switching models still shows the old name. This is correct behaviour, but worth knowing when a tester reports "it says a different model". Severity: note, not a bug. Evidence: `macos/Sources/EZLM/ChatModel.swift:643`, `windows/EZLM/Chat.cs:1142`.

## What I could not verify

- No Windows or Mac build was run tonight (docs-only task, and the brief forbids heavy builds), so on-screen layout, the typewriter effect and the signal wave are described from code, not from a running app.
- No model was loaded, so streaming, token counts and the "cut at the model's limit" path are described from `ChatModel.stream` / `SendChat` and the demo reply, not from a live answer.
- llama.cpp's own web chat (opened from History) is not part of EZLM's code. I only document that the menu item copies the key and opens `http://127.0.0.1:<port>/`; its own screens are out of scope for this section.
- Whether a given model actually emits `reasoning_content` (and so shows a thinking block) depends on the model and template; the section says "thinking models", which matches `ChatModel.write`/`think` and the Windows equivalents.

## Open questions for the Lead

- Should the assembled guide keep the "Open llama.cpp's own chat in the browser" item? It is a developer-flavoured escape hatch and testers may confuse it with EZLM's Chat. I described it in one line under History.
- The demo build (`EZLM_DEMO` / `--demo`) answers Chat with a canned reply and saves no chats (`macos/Sources/EZLM/ChatData.swift:72`, `windows/EZLM/ChatStore.cs:50`). If the beta guide is also used for demo screenshots, say so; I left the demo out of the tester-facing text.
- No error in this slice is invented: every quoted string is copied from the code. a09 and a12 own the full error tables; a07 only lists the chat ones.

## Files

- `agents/T13/sections/a07.md` (new)
- `agents/T13/a07/REPORT.md` (new)
