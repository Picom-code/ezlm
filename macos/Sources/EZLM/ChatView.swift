import SwiftUI
import AppKit
import UniformTypeIdentifiers

/// Chat, terminal style: the signal across the top, the conversation, and a
/// prompt at the bottom. Same page as the Windows app.
struct ChatPane: View {
    @Environment(AppModel.self) private var m
    @FocusState private var focused: Bool
    @State private var confirmDeleteAll = false
    @State private var readingBack = false
    @State private var endVisible = true
    @State private var scrollWatch: Any?

    /// An input method (Chinese, Japanese, Korean) is mid-word: its keys aren't ours.
    private var composing: Bool { (NSApp.keyWindow?.firstResponder as? NSTextView)?.hasMarkedText() ?? false }

    var body: some View {
        let c = m.chat
        VStack(alignment: .leading, spacing: 0) {
            SignalWaveView(wave: c.wave)
            statusLine(c).padding(.top, 6)
            Rectangle().fill(Theme.rule).frame(height: 1).padding(.top, 12)
            if c.messages.isEmpty { welcome(c) } else { transcript(c) }
            input(c)
        }
        .padding(.horizontal, 30).padding(.top, 18).padding(.bottom, 16)
        .background(Theme.black)
        .onAppear {
            c.attach(m)
            c.restoreOnce()
            c.wave.start()
            focused = c.live
        }
        .onDisappear { c.wave.stop() }
        // Drop text files anywhere on the page to ask about them.
        .onDrop(of: [.fileURL], isTargeted: nil) { providers in
            for p in providers {
                _ = p.loadObject(ofClass: URL.self) { url, _ in
                    if let url { Task { @MainActor in c.attach([url]) } }
                }
            }
            return true
        }
        .sheet(isPresented: Binding(get: { c.showMemory }, set: { c.showMemory = $0 })) { MemorySheet(chat: c).environment(m) }
        .alert("Delete all chats?", isPresented: $confirmDeleteAll) {
            Button("Delete all", role: .destructive) { c.deleteAll() }
            Button("Cancel", role: .cancel) {}
        } message: { Text("Every saved chat is removed from this Mac. This can't be undone.") }
    }

    // MARK: top

    private func statusLine(_ c: ChatModel) -> some View {
        // The full line when there's room; a shorter one in a narrow window (never wrapped).
        ViewThatFits(in: .horizontal) {
            statusRow(c, full: true)
            statusRow(c, full: false)
        }
    }

    private func statusRow(_ c: ChatModel, full: Bool) -> some View {
        let st = m.serverState
        let max = m.status?.config?.ctx ?? 0
        let f = max > 0 ? min(1, Double(c.ctxUsed) / Double(max)) : 0
        let speed = c.messages.last(where: { !$0.user && $0.rate > 0 })?.rate ?? 0
        // The live reading lags a moment behind: just after our own answer, it's still ours.
        let sinceAnswer = c.messages.last(where: { !$0.user })?.ended.map { Date().timeIntervalSince($0) } ?? 99 // restored chats carry made-up times
        let justAnswered = (0..<4).contains(sinceAnswer) || Date().timeIntervalSince(c.memEndedAt) < 4
        let state = st == .live ? (c.memWorking ? "noting what to remember" : (["writing", "reading"].contains(m.live?.activity ?? "") && !c.streaming && !justAnswered ? "busy with another app" : "live"))
            : st == .loading ? "loading" : m.status?.deployed == true ? "stopped" : "not set up"
        return HStack(spacing: 14) {
            HStack(spacing: 6) {
                Circle().fill(st.color).frame(width: 7, height: 7)
                Text(state).mono(11, .regular, Theme.dim).lineLimit(1).fixedSize()
            }
            HStack(spacing: 6) {
                if full { Text("chat size").mono(11, .regular, Theme.faint).fixedSize() }
                SizeBar(fraction: f, color: f > 0.9 ? Theme.bad : f > 0.75 ? Theme.gold : Theme.signal).frame(width: 84, height: 8)
                if full && max > 0 { Text("\(ChatModel.short(c.ctxUsed)) / \(ChatModel.short(max))").mono(11, .regular, Theme.faint).fixedSize() }
            }
            .help("How much of the model's memory this chat has used: \(ChatModel.short(c.ctxUsed)) of \(ChatModel.short(max)) tokens")
            if full && speed > 0 { Text("\(Int(speed)) tokens/s").mono(11, .regular, Theme.faint).fixedSize() }
            Spacer(minLength: 8)
            Button(c.thinkOn ? "∴ thinking on" : "∴ thinking off") { c.toggleThink() }
                .buttonStyle(.plain).mono(12, .regular, Theme.dim).fixedSize()
                .help("Let the model think before answering. Off: straight to the answer.")
            Button(!Memory.on ? "✦ memory off" : c.factCount == 0 ? "✦ memory" : "✦ remembers \(c.factCount)") { c.showMemory = true }
                .buttonStyle(.plain).mono(12, .regular, Theme.dim).fixedSize()
                .help("What the chat remembers about you")
            Menu {
                ForEach(Array(Dictionary(grouping: m.installed, by: \.modelId).values.compactMap(\.first)).sorted { $0.name < $1.name }) { im in
                    Button((im.modelId == m.status?.config?.modelId ? "● " : "   ") + im.name + (im.plan == nil ? "   (doesn't fit in free memory now)" : "")) { Task { await m.use(im) } }
                        .disabled(im.plan == nil || im.modelId == m.status?.config?.modelId || m.installRunning || c.streaming)
                }
                if m.installed.count <= 1 {
                    Divider()
                    Button("Get another model in Models…") { m.go(.models) }
                }
            } label: { Text("Model").font(Theme.mono(12)) }
            .menuStyle(.borderlessButton).fixedSize()
            .onAppear { Task { await m.refreshInstalled() } }
            Menu {
                if c.recent.isEmpty { Text("No saved chats yet") }
                ForEach(c.recent.prefix(15), id: \.id) { s in
                    Button("\(s.title)   ·   \(ChatStore.ago(s.updated))") { c.restore(s) }
                }
                Divider()
                Button("Copy this chat") { m.copy(c.transcript); m.showToast("Chat copied. Paste it anywhere.", Theme.ok) }.disabled(c.messages.isEmpty)
                Button("Delete this chat") { c.deleteCurrent() }.disabled(c.messages.isEmpty)
                Button("Delete all chats…") { confirmDeleteAll = true }.disabled(c.recent.isEmpty)
                Divider()
                Button("Open llama.cpp's own chat in the browser") { if let cfg = m.status?.config { m.openChat(cfg) } }
            } label: { Text("History").font(Theme.mono(12)) }
            .menuStyle(.borderlessButton).fixedSize()
            Button("New chat") { c.newChat() }
                .buttonStyle(.small).keyboardShortcut("n", modifiers: .command).disabled(c.messages.isEmpty).fixedSize()
        }
    }

    // MARK: middle

    private func welcome(_ c: ChatModel) -> some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                Text(BigType.lines(BigType.fit(c.modelName)).joined(separator: "\n"))
                    .font(.system(size: 15, design: .monospaced)).lineSpacing(-4).fixedSize()
                    .foregroundStyle(LinearGradient(colors: [Theme.signal, Color(red: 0.95, green: 0.93, blue: 1)], startPoint: .topLeading, endPoint: .bottomTrailing))
                Text("Ask \(c.modelName) anything. It runs on this computer, so what you type stays here.").mono(12, .regular, Theme.dim)
                Text("It remembers what you tell it about yourself (only here; see ✦ memory). Type / for commands.").mono(12, .regular, Theme.faint)
                Text("Try one of these, or write below:").mono(12, .regular, Theme.faint)
                VStack(alignment: .leading, spacing: 4) {
                    ForEach(ChatModel.ideas, id: \.self) { idea in
                        Button { c.send(idea) } label: {
                            (Text("❯ ").foregroundColor(Theme.signal) + Text(idea).foregroundColor(Theme.dim)).font(Theme.mono(13))
                        }
                        .buttonStyle(.plain).padding(.vertical, 3)
                        .disabled(!c.live)
                    }
                }
            }
            .padding(.top, 40).padding(.leading, 4)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    private func toEnd(_ proxy: ScrollViewProxy, _ c: ChatModel) { proxy.scrollTo("end", anchor: .bottom) }

    private func transcript(_ c: ChatModel) -> some View {
        ScrollViewReader { proxy in
            ScrollView {
                // Not lazy: a lazy list only measures rows it has built, and after a
                // very long answer it never built the newest turn, so following the
                // conversation stopped short and the new reply stayed out of sight.
                VStack(alignment: .leading, spacing: 26) {
                    ForEach(c.messages) { msg in MessageView(msg: msg, chat: c, last: msg === c.messages.last) }
                    // Back at the bottom: follow new words again.
                    Color.clear.frame(height: 1).id("end")
                        .onAppear { endVisible = true; readingBack = false }
                        .onDisappear { endVisible = false }
                }
                .padding(.top, 22).padding(.trailing, 22).padding(.bottom, 18)
                .frame(maxWidth: 900, alignment: .leading)
            }
            .onChange(of: c.tick) { _, _ in if !readingBack { toEnd(proxy, c) } }
            .onChange(of: c.messages.count) { _, _ in readingBack = false; toEnd(proxy, c) }
            .onAppear {
                proxy.scrollTo("end", anchor: .bottom)
                // Scrolling up to read stops the following, until you're back at the bottom.
                scrollWatch = NSEvent.addLocalMonitorForEvents(matching: .scrollWheel) { e in
                    if e.scrollingDeltaY > 0 { readingBack = true } else if e.scrollingDeltaY < 0, endVisible { readingBack = false }
                    return e
                }
            }
            .onDisappear { if let w = scrollWatch { NSEvent.removeMonitor(w) }; scrollWatch = nil }
        }
    }

    // MARK: bottom

    private func input(_ c: ChatModel) -> some View {
        let st = m.serverState
        let max = m.status?.config?.ctx ?? 0
        return VStack(alignment: .leading, spacing: 8) {
            Group {
                if st == .loading {
                    HStack(spacing: 10) { Spinner(); Text("\(c.modelName) is loading. You can chat in a moment.").mono(12, .regular, Theme.gold) }
                } else if st != .live {
                    if m.status?.deployed == true {
                        HStack(spacing: 12) {
                            Text("Your model is stopped.").mono(12, .regular, Theme.bad)
                            Button("Start server") { Task { await m.startServer() } }.buttonStyle(.small)
                        }
                    } else {
                        HStack(spacing: 12) {
                            Text("Set up a model first: pick one in Models.").mono(12, .regular, Theme.dim)
                            Button("Open Models") { m.go(.models) }.buttonStyle(.small)
                        }
                    }
                } else if max > 0, Double(c.ctxUsed) > Double(max) * 0.9 {
                    Text("This chat has nearly filled the model's memory, so its oldest messages are left out from here. New chat (⌘N) starts fresh.").mono(12, .regular, Theme.gold)
                }
            }
            let found = ChatModel.matches(c.draft)
            if !found.isEmpty {
                VStack(alignment: .leading, spacing: 2) {
                    ForEach(found, id: \.cmd) { cmd in
                        (Text(cmd.cmd).foregroundColor(Theme.signal) + Text(ChatModel.hint(cmd.cmd, cmd.arg)).foregroundColor(Theme.dim)
                         + Text("  " + cmd.what).foregroundColor(Theme.faint)).font(Theme.mono(12))
                    }
                    Text("tab completes").mono(11, .regular, Theme.faint)
                }
            }
            if !c.files.isEmpty {
                HStack(spacing: 8) { ForEach(c.files) { f in FileChip(file: f) { c.files.removeAll { $0.id == f.id } } } }
            }
            HStack(alignment: .bottom, spacing: 12) {
                Text("❯").mono(15, .bold, Theme.signal).padding(.bottom, 3)
                TextField("Message \(c.modelName)", text: Binding(get: { c.draft }, set: { c.draft = $0; c.typing() }), axis: .vertical)
                    .textFieldStyle(.plain).font(Theme.mono(13.5)).foregroundStyle(Theme.ink)
                    .lineLimit(1...8)
                    .environment(\.layoutDirection, layoutDirection(of: c.draft))
                    .focused($focused)
                    .onSubmit { c.send() }
                    .onKeyPress(.escape) { if c.streaming, !composing { c.stop(); return .handled }; return .ignored }
                    .onKeyPress(.upArrow) {
                        // Like a terminal: up brings back what you sent last (with its files), to fix and resend.
                        guard !composing, c.draft.isEmpty, let last = c.messages.last(where: { $0.user && $0.text != ChatModel.continueText }) else { return .ignored }
                        c.draft = last.text
                        if c.files.isEmpty { c.files = last.files }
                        return .handled
                    }
                    .onKeyPress(.tab) {
                        guard !composing, let first = ChatModel.matches(c.draft).first else { return .ignored }
                        c.draft = first.cmd + (first.arg.isEmpty ? "" : " ")
                        return .handled
                    }
                Button("+ file") { c.pickFiles() }.buttonStyle(.small).padding(.bottom, 2).help("Attach a text file")
                Button(c.streaming ? "Stop  esc" : "Send  ↩") { if c.streaming { c.stopFromButton() } else { c.send() } }
                    .buttonStyle(.primary).disabled(!c.streaming && st != .live)
            }
            .padding(.leading, 14).padding(.trailing, 8).padding(.vertical, 8)
            .background(Color(white: 0.055))
            .overlay(Rectangle().stroke(focused ? Color(red: 0.353, green: 0.31, blue: 0.549) : Theme.rule, lineWidth: 1))
            (["↩ send", "⌥↩ new line", "esc stop", "⌘N new chat", "↑ last message", "/remember save a fact"].map { h -> Text in
                let parts = h.split(separator: " ", maxSplits: 1)
                return Text(String(parts[0])).foregroundColor(Theme.dim) + Text(" " + String(parts.count > 1 ? parts[1] : "") + "     ").foregroundColor(Theme.faint)
            }.reduce(Text(""), +)).font(Theme.mono(11))
        }
        .padding(.top, 10)
    }
}

/// One turn: who said it, then the words. Answers get their thinking (live,
/// then folded away), stats, and Copy / Listen / Retry.
struct MessageView: View {
    @Environment(AppModel.self) private var m
    let msg: ChatMsg
    let chat: ChatModel
    let last: Bool

    var body: some View {
        HStack(alignment: .top, spacing: 16) {
            Rectangle().fill(msg.user ? Theme.faint : !msg.error.isEmpty ? Theme.bad : msg.done ? Color(red: 0.353, green: 0.31, blue: 0.549) : Theme.signal)
                .frame(width: 2)
            VStack(alignment: .leading, spacing: 8) {
                if msg.user { userBody } else { answerBody }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .fixedSize(horizontal: false, vertical: true)
    }

    private var userBody: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("you").mono(11, .regular, Theme.dim)
            if !msg.files.isEmpty { HStack(spacing: 8) { ForEach(msg.files) { FileChip(file: $0, remove: nil) } } }
            if !msg.text.isEmpty {
                Text(msg.text).mono(13).lineSpacing(5).textSelection(.enabled)
                    .environment(\.layoutDirection, layoutDirection(of: msg.text))
            }
        }
    }

    @ViewBuilder private var answerBody: some View {
        TimelineView(.periodic(from: .now, by: msg.done ? 3600 : 0.25)) { _ in
            Text("◆ \(msg.model.isEmpty ? chat.modelName : msg.model)  ·  \(state)").mono(11, .regular, Theme.signal)
        }
        if !msg.thinking.isEmpty { thinkingView }
        let shown = String(msg.text.prefix(msg.done ? msg.text.count : msg.shown))
        let cursor = !msg.done && (!msg.text.isEmpty || msg.thinking.isEmpty) // no cursor while it thinks
        if !shown.isEmpty || cursor { MarkdownView(text: shown, cursor: cursor) }
        if !msg.error.isEmpty { Text(msg.error).mono(13, .regular, Theme.bad).textSelection(.enabled) }
        if msg.done {
            HStack(spacing: 12) {
                if !msg.text.isEmpty {
                    Button("Copy") { m.copy(msg.text); m.showToast("Copied.", Theme.ok) }.buttonStyle(.small)
                    Button(chat.speakingID == msg.id ? "Stop" : "Listen") { chat.speak(msg) }.buttonStyle(.small).help("Read this answer aloud")
                }
                if last { Button("Retry") { chat.retry() }.buttonStyle(.small).help("Ask for a new answer").disabled(chat.streaming || !chat.live) }
                if last, msg.text.isEmpty, !msg.thinking.isEmpty, !msg.error.isEmpty { // thought until it ran out of room and never answered
                    Button("Answer without thinking") { chat.retry(think: false) }.buttonStyle(.small)
                        .help("Ask again for a straight answer").disabled(chat.streaming || !chat.live)
                }
                if last, msg.cut || msg.stopped, !msg.text.isEmpty { // ran into the model's limit, or stopped to read: carry on from there
                    Button("Continue") { chat.send(ChatModel.continueText, files: [ChatModel.answerEnd(msg.text)], think: false) }.buttonStyle(.small) // carrying on needs no thinking
                        .help("Ask the model to go on where the answer stopped").disabled(chat.streaming || !chat.live)
                }
                Text([msg.rate > 0 ? "\(Int(msg.rate)) tokens/s" : "", msg.tokens > 0 ? "\(msg.tokens) tokens" : "", msg.stopped ? "stopped by you" : "", msg.cut ? "stopped at the model's limit" : "", msg.left > 0 && msg.error.isEmpty ? "older messages left out" : ""]
                    .filter { !$0.isEmpty }.joined(separator: "   ")).mono(11, .regular, Theme.faint)
            }
        }
        ForEach(msg.remembered, id: \.self) { fact in
            HStack(spacing: 6) {
                (Text("✦ remembered  ").foregroundColor(Theme.signal) + Text(fact).foregroundColor(Theme.dim)).font(Theme.mono(11))
                Button("forget") { chat.forget(fact, from: msg) }.buttonStyle(.plain).mono(11, .regular, Theme.faint)
            }
        }
    }

    private var state: String {
        let secs = (msg.ended ?? Date()).timeIntervalSince(msg.started)
        if !msg.error.isEmpty { return "didn't finish" }
        if msg.stopped { return "stopped" }
        if msg.done { return String(format: "%.1fs", secs) + (msg.noThink ? "  ·  without thinking" : "") }
        let spin = ["⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"][Int(Date().timeIntervalSince1970 * 8) % 10]
        if !msg.text.isEmpty { return "writing \(spin)" }
        if !msg.thinking.isEmpty { return "thinking \(spin)" }
        if let l = m.live, l.promptTotal > 0 { return "reading \(spin)  \(ChatModel.short(l.promptDone)) / \(ChatModel.short(l.promptTotal)) tokens" }
        return "reading \(spin)"
    }

    @ViewBuilder private var thinkingView: some View {
        let liveThink = msg.text.isEmpty && !msg.done
        if liveThink {
            TimelineView(.periodic(from: .now, by: 0.5)) { _ in
                let secs = Date().timeIntervalSince(msg.thinkStart ?? Date())
                VStack(alignment: .leading, spacing: 6) {
                    Text("∴ thinking  \(Int(secs))s").mono(11, .regular, Theme.gold)
                    thinkText(String(msg.thinking.suffix(360)).trimmingCharacters(in: .whitespacesAndNewlines))
                    // Long thinking on a slow computer: offer the way straight to an answer.
                    if secs > 40 { Button("Answer without thinking") { chat.answerWithoutThinking() }.buttonStyle(.small) }
                }
            }
        } else {
            let secs = max(1, (msg.thinkEnd ?? msg.ended ?? Date()).timeIntervalSince(msg.thinkStart ?? msg.started))
            Button("∴ thought for \(Int(secs))s  \(msg.showThinking ? "▾" : "▸")") { msg.showThinking.toggle() }
                .buttonStyle(.plain).mono(11, .regular, Color(red: 0.61, green: 0.54, blue: 0.32))
            if msg.showThinking { thinkText(msg.thinking.trimmingCharacters(in: .whitespacesAndNewlines)) }
        }
    }

    private func thinkText(_ s: String) -> some View {
        HStack(alignment: .top, spacing: 12) {
            Rectangle().fill(Color(red: 0.30, green: 0.27, blue: 0.15)).frame(width: 1)
            Text(s).mono(12, .regular, Color(white: 0.54)).lineSpacing(4).textSelection(.enabled)
                .environment(\.layoutDirection, layoutDirection(of: s))
        }
        .fixedSize(horizontal: false, vertical: true)
    }
}

/// A file as a small tag: name and size; with a remove button while it's still a draft.
struct FileChip: View {
    let file: ChatFile
    let remove: (() -> Void)?
    init(file: ChatFile, remove: (() -> Void)?) { self.file = file; self.remove = remove }

    var body: some View {
        HStack(spacing: 8) {
            Text("▤").mono(12, .regular, Theme.signal)
            Text(file.name).mono(12)
            Text("\(ChatModel.short(ChatModel.tokens(file.text))) tokens").mono(11, .regular, Theme.faint)
            if let remove { Button("×", action: remove).buttonStyle(.plain).mono(12, .regular, Theme.dim).help("Remove \(file.name)") }
        }
        .padding(.horizontal, 10).padding(.vertical, 4)
        .background(Color(white: 0.055))
        .overlay(Rectangle().stroke(Theme.rule, lineWidth: 1))
    }
}

/// A thin bar of blocks, like the rest of EZLM's meters.
struct SizeBar: View {
    let fraction: Double
    let color: Color
    var body: some View {
        GeometryReader { g in
            ZStack(alignment: .leading) {
                Rectangle().fill(Theme.rule)
                Rectangle().fill(color).frame(width: max(fraction > 0 ? 2 : 0, g.size.width * fraction))
            }
        }
    }
}

/// Memory: everything the chat remembers, editable, with the switches and
/// the person's own instructions.
struct MemorySheet: View {
    @Environment(\.dismiss) private var dismiss
    let chat: ChatModel
    @State private var facts = Memory.facts()
    @State private var add = ""
    @State private var on = Memory.on
    @State private var auto = Memory.auto
    @State private var how = Memory.instructions
    @State private var note = ""

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            Text("Memory").mono(18, .bold)
            Text("What the chat remembers about you from earlier conversations. Your model reads it at the start of each new chat; it never leaves this computer.")
                .mono(12, .regular, Theme.dim).fixedSize(horizontal: false, vertical: true)
            ScrollView {
                VStack(alignment: .leading, spacing: 6) {
                    if facts.isEmpty { Text("Nothing yet. Tell it about yourself in a chat, or add something below.").mono(12, .regular, Theme.faint) }
                    ForEach(facts, id: \.self) { f in
                        HStack(alignment: .firstTextBaseline) {
                            Text("• " + f).mono(12).fixedSize(horizontal: false, vertical: true)
                            Spacer()
                            Button("×") { Memory.remove(f); refresh() }.buttonStyle(.plain).mono(12, .regular, Theme.dim).help("Forget this")
                        }
                    }
                }
            }
            .frame(maxHeight: 180)
            HStack(spacing: 8) {
                TextField("Add a fact, e.g. I'm vegetarian", text: $add).textFieldStyle(.roundedBorder).font(Theme.mono(12)).onSubmit(addIt)
                Button("Remember", action: addIt).buttonStyle(.term)
            }
            if !note.isEmpty { Text(note).mono(11, .regular, Theme.gold) }
            HStack(spacing: 8) {
                let used = Memory.used(facts)
                SizeBar(fraction: Double(used) / Double(Memory.budget), color: used > Int(Double(Memory.budget) * 0.85) ? Theme.gold : Theme.signal).frame(width: 140, height: 8)
                Text("\(used) / \(Memory.budget) characters").mono(11, .regular, Theme.faint)
            }
            Toggle(isOn: $on) { Text("Use what it remembers in chats").mono(12) }.toggleStyle(.checkbox).onChange(of: on) { _, v in Memory.on = v; chat.factCount = Memory.facts().count }
            Toggle(isOn: $auto) { Text("Notice new things by itself (after a chat goes quiet)").mono(12) }.toggleStyle(.checkbox).onChange(of: auto) { _, v in Memory.auto = v }
            Text("Your instructions").mono(12, .bold).padding(.top, 6)
            Text("Sent at the start of every new chat. Leave empty for none.").mono(11, .regular, Theme.faint)
            TextEditor(text: $how).font(Theme.mono(12)).frame(height: 70).scrollContentBackground(.hidden)
                .padding(6).background(Color(white: 0.07)).overlay(Rectangle().stroke(Theme.rule, lineWidth: 1))
                .overlay(alignment: .topLeading) {
                    if how.isEmpty { Text("How should it answer? e.g. Keep answers short. Explain things like I'm new to them.").mono(12, .regular, Theme.faint).padding(10).allowsHitTesting(false) }
                }
            HStack {
                Button("Forget everything") { Memory.clear(); refresh() }.buttonStyle(.danger)
                Spacer()
                Button("Done") { Memory.instructions = how; dismiss() }.buttonStyle(.primary).keyboardShortcut(.defaultAction)
            }
            .padding(.top, 6)
        }
        .padding(24)
        .frame(width: 560)
        .background(Color(white: 0.06))
        .preferredColorScheme(.dark)
        .onDisappear { Memory.instructions = how; chat.factCount = Memory.facts().count }
    }

    private func addIt() {
        let r = Memory.add(add)
        if r.ok { add = ""; note = "" } else { note = r.why }
        refresh()
    }

    private func refresh() { facts = Memory.facts(); chat.factCount = facts.count }
}
