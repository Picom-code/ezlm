import SwiftUI
import AppKit
import AVFoundation
import UniformTypeIdentifiers

struct ChatFile: Identifiable, Equatable {
    let id = UUID()
    let name: String
    let text: String
}

/// One turn in a chat.
@MainActor
@Observable
final class ChatMsg: Identifiable {
    let id = UUID()
    let user: Bool
    var text = ""
    var thinking = ""
    var files: [ChatFile] = []
    var error = ""
    var done = false
    var stopped = false
    var showThinking = false
    var noThink = false          // asked to answer without thinking
    var left = 0                 // older messages left out so the chat fit the model
    var cut = false              // the answer ran into the model's limit
    var model = ""               // who wrote the answer (models can be switched mid-chat)
    var started = Date()
    var ended: Date?
    var firstToken: Date?
    var thinkStart: Date?
    var thinkEnd: Date?
    var shown = 0                // characters revealed so far (the typewriter)
    var chunks = 0
    var tokens = 0
    var rate = 0.0
    var remembered: [String] = [] // facts noticed after this answer

    init(user: Bool) { self.user = user }
}

/// Chat: talk to your model right here. It streams from the local server
/// with the key filled in, keeps chats on this Mac, and remembers the person
/// between chats. Same behavior as the Windows app.
@MainActor
@Observable
final class ChatModel {
    @ObservationIgnored weak var app: AppModel?
    var messages: [ChatMsg] = []
    var draft = ""
    var files: [ChatFile] = []
    var ctxUsed = 0
    var streaming = false
    var memWorking = false
    @ObservationIgnored var memEndedAt = Date.distantPast // when the last quiet memory check finished
    var showMemory = false
    var recent: [ChatStore.Saved] = []
    var errorAt = Date.distantPast
    var tick = 0                  // advances with each typewriter frame (for scrolling)
    var thinkOn = Memory.think    // mirrors the saved switch so the button redraws
    var factCount = 0
    var speakingID: UUID?
    let wave = SignalWave()

    @ObservationIgnored private var task: Task<Void, Never>?
    @ObservationIgnored private var memTask: Task<Void, Never>?
    @ObservationIgnored private var chatID = ChatStore.newID()
    @ObservationIgnored private var briefing: String?
    @ObservationIgnored private var inlineNotes = false  // this chat's model has no place for notes: they go in the first question
    @ObservationIgnored private var share = 1.0          // how much of the room this chat's model took (below 1 after a squeeze)
    @ObservationIgnored private var memFrom = 0
    @ObservationIgnored private var restored = false
    @ObservationIgnored private var skipThink = false
    @ObservationIgnored private var speech: AVSpeechSynthesizer?
    @ObservationIgnored private lazy var session: URLSession = {
        let c = URLSessionConfiguration.default
        c.timeoutIntervalForRequest = 900 // a big prompt on a slow Mac can take minutes before the first word
        c.timeoutIntervalForResource = 24 * 3600
        return URLSession(configuration: c)
    }()

    static let ideas = [
        "Explain how vaccines work, in simple words.",
        "Write a friendly email asking my landlord to fix the heating.",
        "Plan a 3-day vegetarian menu with a shopping list.",
    ]

    nonisolated static let commands: [(cmd: String, arg: String, what: String)] = [
        ("/remember", " <fact>", "save something about you for future chats"),
        ("/forget", " <words>", "remove remembered things that mention these words"),
        ("/memory", "", "see and edit everything it remembers"),
        ("/new", "", "start a new chat"),
    ]

    var modelName: String { app?.status?.config?.modelName ?? "your model" }

    /// Developer check only: the demo can chat with a real test server given as
    /// --chat-server "url|key", to exercise the real streaming code.
    static let testServer: (url: String, key: String, ctx: Int?)? = {
        let a = CommandLine.arguments
        guard Engine.demo, let i = a.firstIndex(of: "--chat-server"), i + 1 < a.count else { return nil }
        let p = a[i + 1].split(separator: "|", maxSplits: 2).map(String.init) // url|key, or url|key|ctx
        return p.count >= 2 ? (p[0], p[1], p.count > 2 ? Int(p[2]) : nil) : nil
    }()

    /// Where chat requests go: the person's server, or the test server in a developer check.
    private func target() -> (ServerConfig, String)? {
        guard let c = app?.status?.config else { return nil }
        if let t = Self.testServer { var tc = c; tc.key = t.key; tc.ctx = t.ctx ?? c.ctx; return (tc, t.url) }
        guard let url = app?.status?.localUrl else { return nil }
        return (c, url)
    }
    var live: Bool { app?.serverState == .live }

    func attach(_ app: AppModel) {
        guard self.app == nil else { return }
        self.app = app
        wave.moodOf = { [weak self] in self?.mood() ?? .off }
        wave.rateOf = { [weak self] in self?.rate() ?? 0 }
        wave.onFrame = { [weak self] in self?.frame() }
        factCount = Memory.facts().count
    }

    /// The first time Chat opens, pick up the last conversation (from the past week).
    func restoreOnce() {
        guard !restored else { return }
        restored = true
        recent = ChatStore.recent()
        if messages.isEmpty, let last = recent.first, Date().timeIntervalSince(last.updated) < 7 * 86400 { restore(last) }
    }

    // MARK: the signal

    func mood() -> Mood {
        guard let app else { return .off }
        switch app.serverState {
        case .stopped, .notSetUp: return .off
        case .loading: return .loading
        case .live: break
        }
        if let m = messages.last, !m.user, !m.done { return !m.text.isEmpty ? .writing : !m.thinking.isEmpty ? .thinking : .reading }
        if Date().timeIntervalSince(errorAt) < 2.5 { return .error }
        // Another app (OpenCode, Hermes, a script) using the model shows here too.
        switch app.live?.activity { case "writing": return .writing; case "reading": return .reading; default: return .idle }
    }

    func rate() -> Double {
        if let m = messages.last, !m.user, !m.done, m.chunks > 4, let f = m.firstToken { return Double(m.chunks) / max(0.3, Date().timeIntervalSince(f)) }
        return app?.live?.tokPerSec ?? 0
    }

    /// 30 times a second while Chat is on screen: words appear smoothly, catching up with what arrived.
    @ObservationIgnored private var frames = 0

    private func frame() {
        frames &+= 1
        guard frames % 2 == 0, let m = messages.last, !m.user, !m.done else { return } // 15 updates a second is smooth and cheap
        let n = m.text.count
        if m.shown < n {
            m.shown = min(n, m.shown + max(3, (n - m.shown + 2) / 3))
            tick &+= 1
        }
    }

    // MARK: sending

    func send(_ raw: String? = nil, files given: [ChatFile]? = nil, think: Bool? = nil) {
        let fromBox = raw == nil
        let text = (raw ?? draft).trimmingCharacters(in: .whitespacesAndNewlines)
        if fromBox, command(text) { return }
        let attached = given ?? (fromBox ? files : [])
        guard app != nil, let (c, url) = target(), !streaming, live, !(text.isEmpty && attached.isEmpty) else { return }
        memTask?.cancel()
        let user = ChatMsg(user: true)
        user.text = text
        user.files = attached
        user.done = true
        if fromBox { files = []; draft = "" } // a suggestion or a retry leaves what you're typing alone
        let reply = ChatMsg(user: false)
        reply.model = c.modelName
        reply.noThink = !(think ?? Memory.think)
        messages.append(user)
        messages.append(reply)
        save() // the question is kept even if EZLM quits before the answer ends
        briefing = briefing ?? Memory.briefing() // set once, then fixed for the chat (keeps the model's cache warm)
        let (history, left) = self.history(upto: messages.count - 1, ctx: c.ctx, inline: inlineNotes, share: share) // as this chat's model needed last time
        reply.left = left
        streaming = true
        sentAt = Date()
        task = Task { [weak self] in await self?.stream(reply, history: history, config: c, url: url) }
    }

    /// The messages for the model (ChatModel.messages) from this chat, up to
    /// (not including) message `upto`.
    private func history(upto: Int, ctx: Int, inline: Bool = false, share: Double = 1) -> (msgs: [[String: String]], left: Int) {
        Self.messages(messages.map { ($0.user, $0.user ? content($0) : $0.text) }, upto: upto, notes: briefing, ctx: ctx, inline: inline, share: share)
    }

    /// The messages for the model: the notes about the person first (fixed for
    /// the chat), then the turns before `upto` that fit (fit). A question whose
    /// answer failed is left out, since many models need user and assistant
    /// turns to alternate. `inline` folds the notes into the first question,
    /// for chat formats with no place for them. `chat` is each message as the
    /// model reads it (a question with its files). Says how many were left out.
    nonisolated static func messages(_ chat: [(user: Bool, content: String)], upto: Int, notes: String?, ctx: Int, inline: Bool = false, share: Double = 1) -> (msgs: [[String: String]], left: Int) {
        var h: [[String: String]] = []
        for i in 0..<upto {
            let m = chat[i]
            if m.user, i == upto - 1 || (i + 1 < chat.count && !chat[i + 1].user && !chat[i + 1].content.isEmpty) {
                h.append(["role": "user", "content": m.content])
            } else if !m.user, !m.content.isEmpty {
                h.append(["role": "assistant", "content": m.content])
            }
        }
        let left = fit(&h, ctx: ctx, notes: notes.map { tokens($0) } ?? 0, share: share)
        if let b = notes {
            if !inline { h.insert(["role": "system", "content": b], at: 0) }
            else if let u = h.firstIndex(where: { $0["role"] == "user" }) { h[u]["content"] = b + "\n" + (h[u]["content"] ?? "") }
        }
        return (h, left)
    }

    enum Refusal { case inline, squeeze, fail }

    /// What to do when the model turns a request down: fold the notes into the
    /// question (its chat format has no place for them), leave out more of the
    /// oldest turns (still too long by its own count), or show the error.
    nonisolated static func afterRefusal(_ error: String, inline: Bool, hasNotes: Bool, squeezes: Int, questions: Int) -> Refusal {
        if !inline, hasNotes, error.localizedCaseInsensitiveContains("system") { return .inline }
        if tooLong(error), squeezes < 3, questions > 1 { return .squeeze }
        return .fail
    }

    /// What the model gets for a question: its files, then the words.
    private func content(_ m: ChatMsg) -> String {
        m.files.map { "File: \($0.name)\n```\n\($0.text)\n```\n\n" }.joined() + m.text
    }

    private func request(_ url: String, _ c: ServerConfig, _ msgs: [[String: String]], stream: Bool, extra: [String: Any] = [:]) throws -> URLRequest {
        var payload: [String: Any] = ["model": c.modelId, "stream": stream, "messages": msgs]
        for (k, v) in extra { payload[k] = v }
        var req = URLRequest(url: URL(string: url.trimmingCharacters(in: CharacterSet(charactersIn: "/")) + "/chat/completions")!)
        req.httpMethod = "POST"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.setValue("Bearer \(c.key)", forHTTPHeaderField: "Authorization")
        req.httpBody = try JSONSerialization.data(withJSONObject: payload)
        return req
    }

    private func stream(_ reply: ChatMsg, history: [[String: String]], config c: ServerConfig, url: String, tries: Int = 0) async {
        var stopped = false
        do {
            if Engine.demo && Self.testServer == nil { try await demoReply(reply) } else {
                // Without thinking: Qwen-style templates skip the think block; gpt-oss thinks briefly.
                let options: [String: Any] = reply.noThink ? ["chat_template_kwargs": ["enable_thinking": false], "reasoning_effort": "low"] : [:]
                var history = history, inline = inlineNotes, share = self.share, squeezes = 0
                var (bytes, resp) = try await session.bytes(for: request(url, c, history, stream: true, extra: options))
                while (resp as? HTTPURLResponse)?.statusCode != 200 {
                    var body = ""
                    for try await line in bytes.lines { body += line }
                    switch Self.afterRefusal(body, inline: inline, hasNotes: briefing != nil, squeezes: squeezes, questions: history.filter { $0["role"] == "user" }.count) {
                    case .inline: inline = true
                    case .squeeze:
                        squeezes += 1
                        share = Self.squeeze(body, sent: history, ctx: c.ctx, notes: briefing.map { Self.tokens($0) } ?? 0, share: share)
                    case .fail:
                        reply.error = Self.explain((resp as? HTTPURLResponse)?.statusCode ?? 0, body)
                        throw Done.early
                    }
                    guard let i = messages.firstIndex(where: { $0 === reply }) else { throw Done.early } // the chat changed meanwhile
                    let fitted = self.history(upto: i, ctx: c.ctx, inline: inline, share: share)
                    history = fitted.msgs
                    reply.left = max(reply.left, fitted.left)
                    (bytes, resp) = try await session.bytes(for: request(url, c, history, stream: true, extra: options))
                }
                (inlineNotes, self.share) = (inline, share) // the memory check asks the same way
                for try await line in bytes.lines {
                    guard line.hasPrefix("data: ") else { continue }
                    let data = line.dropFirst(6).trimmingCharacters(in: .whitespaces)
                    if data == "[DONE]" { break }
                    guard let obj = try? JSONSerialization.jsonObject(with: Data(data.utf8)) as? [String: Any] else { continue }
                    if obj["error"] != nil { reply.error = Self.explain(500, data); break }
                    if let choices = obj["choices"] as? [[String: Any]], let d = choices.first?["delta"] as? [String: Any] {
                        if let r = d["reasoning_content"] as? String, !r.isEmpty { think(reply, r) }
                        if let piece = d["content"] as? String, !piece.isEmpty { write(reply, piece) }
                        if choices.first?["finish_reason"] as? String == "length" { reply.cut = true }
                    }
                    if let tm = obj["timings"] as? [String: Any] {
                        if let pps = tm["predicted_per_second"] as? Double { reply.rate = pps }
                        let n = { (k: String) in (tm[k] as? Int) ?? Int((tm[k] as? Double) ?? 0) }
                        reply.tokens = n("predicted_n")
                        ctxUsed = n("prompt_n") + n("cache_n") + reply.tokens
                    }
                }
            }
            if reply.text.isEmpty && reply.error.isEmpty {
                reply.error = reply.thinking.isEmpty ? "It didn't answer. Try asking again." : "It thought about it but didn't write an answer. Try asking again."
            }
        } catch Done.early {
        } catch is CancellationError {
            stopped = true
        } catch let e as URLError where e.code == .cancelled {
            stopped = true
        } catch let e as URLError where Self.retryable(e, text: reply.text, thinking: reply.thinking, tries: tries) {
            // The connection died before the model wrote anything: the Mac may
            // have woken from sleep, or the server may be restarting. Ask again
            // instead of showing an error for an answer that never started.
            await waitForServer()
            if Task.isCancelled { finish(reply, stopped: true); return }
            return await stream(reply, history: history, config: c, url: url, tries: tries + 1)
        } catch let e as URLError where e.code == .cannotConnectToHost || e.code == .networkConnectionLost || e.code == .timedOut {
            reply.error = e.code == .networkConnectionLost ? "Your model stopped in the middle of answering. Check Overview, then ask again."
                : "Couldn't reach your model. It may have stopped: check Overview."
        } catch {
            if Task.isCancelled { stopped = true } else { reply.error = "Something went wrong: \(error.localizedDescription)" }
        }
        finish(reply, stopped: stopped)
    }

    private enum Done: Error { case early }

    /// A stream can die when the Mac wakes from sleep or the model server
    /// restarts. Asking again is safe only before the model has written
    /// anything: once words are on screen, a retry would replace them. Three
    /// tries covers a socket that died in sleep and a server that came back.
    nonisolated static func retryable(_ e: URLError, text: String, thinking: String, tries: Int) -> Bool {
        guard tries < 3, text.isEmpty, thinking.isEmpty else { return false }
        switch e.code {
        case .cannotConnectToHost, .networkConnectionLost, .timedOut, .notConnectedToInternet, .dnsLookupFailed, .cannotFindHost:
            return true
        default:
            return false
        }
    }

    /// After a drop, gives a restarting model server a moment to be live again,
    /// so the next try lands on a server that is really there. A server that
    /// is stopped (the person pressed Stop) won't come back on its own.
    private func waitForServer() async {
        let until = Date().addingTimeInterval(app?.serverState == .loading ? 15 : 1)
        while !live, Date() < until, !Task.isCancelled {
            try? await Task.sleep(nanoseconds: 250_000_000)
        }
    }

    private func think(_ m: ChatMsg, _ piece: String) {
        if m.thinking.isEmpty { m.thinkStart = Date() }
        m.thinking += piece
        wave.pulse(0.02)
    }

    private func write(_ m: ChatMsg, _ raw: String) {
        // A small model can leak its thinking tags into the answer; they mean nothing to a reader.
        let piece = raw.replacingOccurrences(of: "</think>", with: "").replacingOccurrences(of: "<think>", with: "")
        guard !piece.isEmpty else { return }
        if m.firstToken == nil { m.firstToken = Date() }
        if !m.thinking.isEmpty, m.thinkEnd == nil { m.thinkEnd = Date() }
        // Thinking models often open the answer with blank lines.
        m.text += m.text.isEmpty ? String(piece.drop(while: { $0.isWhitespace })) : piece
        m.chunks += 1
        wave.pulse(0.05)
    }

    private func finish(_ reply: ChatMsg, stopped: Bool) {
        reply.done = true
        reply.stopped = stopped
        reply.ended = Date()
        reply.shown = reply.text.count
        if !reply.thinking.isEmpty, reply.thinkEnd == nil { reply.thinkEnd = reply.ended }
        if reply.rate == 0, reply.chunks > 4, let f = reply.firstToken { reply.rate = Double(reply.chunks) / max(0.3, reply.ended!.timeIntervalSince(f)) }
        streaming = false
        task = nil
        if !reply.error.isEmpty { errorAt = Date() } else if !stopped { wave.rippleOnce() }
        // A long answer that finished while you were elsewhere: the Dock icon says so.
        if !stopped, reply.ended!.timeIntervalSince(reply.started) > 10 { Dock.finished(ok: reply.error.isEmpty) }
        if messages.contains(where: { $0 === reply }) { save() }
        if skipThink {
            skipThink = false
            if stopped, messages.last === reply { retry(think: false) }
            return
        }
        if reply.error.isEmpty, !stopped { noticeFacts(after: reply) }
    }

    /// Send turns into Stop: a double click mustn't stop the answer it just started.
    func stopFromButton() { if Date().timeIntervalSince(sentAt) > 0.6 { task?.cancel() } }
    @ObservationIgnored private var sentAt = Date.distantPast

    func stop() { task?.cancel() }

    /// Stops a long think and asks the same question again for a straight answer.
    func answerWithoutThinking() { skipThink = true; task?.cancel() }

    /// Asks again for the last question: the old answer goes, a new one streams in.
    func retry(think: Bool? = nil) {
        // Only when it can actually ask again: never drop the question for nothing.
        guard !streaming, live, target() != nil, messages.count >= 2, !messages[messages.count - 1].user, messages[messages.count - 2].user else { return }
        memTask?.cancel()
        let q = messages[messages.count - 2]
        messages.removeLast(2)
        send(q.text, files: q.files, think: think)
    }

    func newChat() {
        if streaming { save() } // keep the turn in progress
        task?.cancel()
        memTask?.cancel()
        messages = []
        draft = ""
        files = []
        ctxUsed = 0
        chatID = ChatStore.newID()
        briefing = nil
        inlineNotes = false
        share = 1
        memFrom = 0
    }

    func toggleThink() { Memory.think.toggle(); thinkOn = Memory.think }

    // MARK: commands

    /// The command list under the box: the argument padded so the
    /// descriptions line up. A command longer than the column keeps its
    /// argument whole, since Foundation pads a negative length to a huge one.
    nonisolated static func hint(_ cmd: String, _ arg: String) -> String {
        let room = 20 - cmd.count
        return room > 0 ? arg.padding(toLength: room, withPad: " ", startingAt: 0) : arg
    }

    nonisolated static func matches(_ draft: String) -> [(cmd: String, arg: String, what: String)] {
        guard draft.hasPrefix("/"), !draft.contains(" "), !draft.contains("\n") else { return [] }
        return commands.filter { $0.cmd.hasPrefix(draft.lowercased()) }
    }

    /// /remember, /forget, /memory, /new: handled here, never sent to the model.
    private func command(_ text: String) -> Bool {
        guard text.hasPrefix("/") else { return false }
        let parts = text.split(separator: " ", maxSplits: 1)
        let cmd = parts[0].lowercased(), arg = parts.count > 1 ? parts[1].trimmingCharacters(in: .whitespaces) : ""
        switch cmd {
        case "/remember":
            guard !arg.isEmpty else { app?.showToast("Write what to remember after /remember, like: /remember I'm vegetarian", Theme.gold); return true }
            let r = Memory.add(arg)
            app?.showToast(r.ok ? "Remembered: \(r.fact)" : r.why, r.ok ? Theme.ok : Theme.gold)
        case "/forget":
            if arg.isEmpty { showMemory = true } else {
                let n = Memory.forget(arg)
                app?.showToast(n > 0 ? "Forgot \(n) thing\(n == 1 ? "" : "s") about \"\(arg)\"." : "Nothing remembered mentions \"\(arg)\".", n > 0 ? Theme.ok : Theme.gold)
            }
        case "/memory": showMemory = true
        case "/new": newChat()
        case "/help", "/": app?.showToast("Commands: /remember <fact>   /forget <words>   /memory   /new"); return true
        default: return false // not a command: an ordinary message that starts with /
        }
        draft = ""
        factCount = Memory.facts().count
        return true
    }

    // MARK: remembering

    /// After an answer, once the chat has gone quiet for a few seconds, asks the
    /// model (briefly, without thinking) for lasting facts the person shared, and
    /// keeps the new ones. Typing or sending cancels it at once.
    private func noticeFacts(after reply: ChatMsg) {
        guard Memory.on, Memory.auto, !Engine.demo || Self.testServer != nil, let (c, url) = target(),
              let upto = messages.firstIndex(where: { $0 === reply }).map({ $0 + 1 }) else { return }
        guard messages[min(memFrom, upto)..<upto].contains(where: { $0.user && Memory.personal($0.text) }) else { return }
        memTask = Task { [weak self] in
            guard let self else { return }
            do {
                try await Task.sleep(nanoseconds: 8_000_000_000) // quiet first
                // The chat may have changed meanwhile (History, New chat, Retry): only look if this answer is still there.
                guard !streaming, live, draft.isEmpty, upto <= messages.count, messages[upto - 1] === reply else { return }
                memWorking = true
                defer { memWorking = false; memEndedAt = Date() }
                var msgs = history(upto: upto, ctx: c.ctx, inline: inlineNotes, share: share).msgs
                msgs.append(["role": "user", "content": Memory.extractPrompt])
                // Thinking off both ways (as in Chat): gpt-oss always reasons, and would spend the whole answer on it.
                let req = try request(url, c, msgs, stream: false, extra: ["max_tokens": 220, "temperature": 0.2, "chat_template_kwargs": ["enable_thinking": false], "reasoning_effort": "low"])
                let (data, resp) = try await session.data(for: req)
                guard (resp as? HTTPURLResponse)?.statusCode == 200,
                      let obj = try JSONSerialization.jsonObject(with: data) as? [String: Any],
                      let msg = (obj["choices"] as? [[String: Any]])?.first?["message"] as? [String: Any] else { return }
                memFrom = upto
                let text = msg["content"] as? String ?? ""
                for fact in Memory.parseFacts(text) {
                    let r = Memory.add(fact)
                    if r.ok { reply.remembered.append(r.fact) } // as stored, so "forget" finds it
                }
                factCount = Memory.facts().count
            } catch {}
        }
    }

    func forget(_ fact: String, from m: ChatMsg) {
        Memory.remove(fact)
        m.remembered.removeAll { $0 == fact }
        factCount = Memory.facts().count
    }

    func typing() { if !draft.isEmpty { memTask?.cancel() } }

    // MARK: files

    func pickFiles() {
        let p = NSOpenPanel()
        p.allowsMultipleSelection = true
        p.canChooseDirectories = false
        if p.runModal() == .OK { attach(p.urls) }
    }

    /// Reads text files to send with the next message; anything that isn't text,
    /// or wouldn't fit in the model's memory, is turned down with the reason.
    func attach(_ urls: [URL]) {
        let max = app?.status?.config?.ctx ?? 8192
        for u in urls {
            let name = u.lastPathComponent
            // Size first: a dropped video or model file must never be read in.
            let size = (try? u.resourceValues(forKeys: [.fileSizeKey]).fileSize) ?? 0
            if size > 2_000_000 { app?.showToast("\(name) is too big to attach (\(String(format: "%.1f", Double(size) / 1e6)) MB). Copy in the part you need instead.", Theme.gold); continue }
            guard let data = try? Data(contentsOf: u) else { app?.showToast("Couldn't read \(name).", Theme.bad); continue }
            guard let decoded = Self.decodeText(data) else {
                let doc = ["pdf", "docx", "pages", "doc"].contains(u.pathExtension.lowercased())
                app?.showToast(doc ? "EZLM can't read \(name) yet: PDF and Word files need their text copied in." : "\(name) isn't a text file, so the model can't read it.", Theme.gold)
                continue
            }
            let text = decoded.trimmingCharacters(in: CharacterSet(charactersIn: "\u{FEFF}"))
            // Older turns make way when needed (fit), but the files and question must fit on their own.
            let need = Self.tokens(text) + files.reduce(0) { $0 + Self.tokens($1.text) } + Self.tokens(draft)
            if need > Self.room(max) - 300 { app?.showToast("\(name) is about \(Self.short(Self.tokens(text))) tokens: more than \(modelName) can keep in mind (\(Self.short(max))).", Theme.gold); continue }
            files.append(ChatFile(name: name, text: text))
        }
    }

    /// Text in whatever form it was saved: UTF-8, UTF-16 (with or without a mark)
    /// or the old Windows code page. Nil for a file that isn't text.
    nonisolated static func decodeText(_ d: Data) -> String? {
        let b = [UInt8](d)
        if b.starts(with: [0xEF, 0xBB, 0xBF]) { return String(decoding: b.dropFirst(3), as: UTF8.self) }
        if b.starts(with: [0xFF, 0xFE]) { return String(data: Data(b.dropFirst(2)), encoding: .utf16LittleEndian) }
        if b.starts(with: [0xFE, 0xFF]) { return String(data: Data(b.dropFirst(2)), encoding: .utf16BigEndian) }
        let head = b.prefix(8000)
        if head.contains(0) {
            let oddZeros = head.enumerated().filter { $0.offset % 2 == 1 && $0.element == 0 }.count
            return head.count >= 4 && oddZeros * 10 >= head.count / 2 * 9 ? String(data: d, encoding: .utf16LittleEndian) : nil
        }
        return String(data: d, encoding: .utf8) ?? String(data: d, encoding: .windowsCP1252)
    }

    /// About how many tokens text is, on the safe side: English words run
    /// about 4 letters a token, but digits, punctuation and line breaks (most
    /// of a table of numbers or code) about one each; other alphabets 2 letters,
    /// and Chinese, Japanese or Korean about one a character.
    nonisolated static func tokens(_ s: String) -> Int {
        var letters = 0, marks = 0, wide = 0, other = 0
        for u in s.unicodeScalars {
            let v = u.value
            if (v >= 0x41 && v <= 0x5A) || (v >= 0x61 && v <= 0x7A) || v == 0x20 { letters += 1 }
            else if v < 0x80 { marks += 1 } else if v >= 0x2E80 { wide += 1 } else { other += 1 }
        }
        return Int(Double(letters) / 4 + Double(marks) + Double(wide) + Double(other) / 2)
    }

    /// What a model that reads `ctx` tokens at once has for the conversation,
    /// after keeping room for its answer.
    nonisolated static func room(_ ctx: Int) -> Int { ctx - min(max(ctx / 4, 512), 4096) }

    /// Keeps a conversation inside what the model can read at once: the newest
    /// turns stay and the oldest go, so a long chat keeps going instead of
    /// stopping. What's kept starts with a question, so turns still alternate,
    /// and an answer at the end keeps its question (the memory check). The
    /// starting point moves in steps of about a third of the room, not every
    /// turn, so the model reuses what it already read for several turns
    /// instead of reading the whole conversation again for each answer.
    /// `notes` is what goes along anyway (the memory notes); a `share` below 1
    /// squeezes harder after the model said it was still too long. Returns
    /// how many messages were left out.
    @discardableResult
    nonisolated static func fit(_ turns: inout [[String: String]], ctx: Int, notes: Int = 0, share: Double = 1) -> Int {
        guard ctx > 0, turns.count > 1 else { return 0 }
        let room = Double(Self.room(ctx) - notes) * share
        let cost = turns.map { Double(tokens($0["content"] ?? "") + 8) } // each turn's role markers cost a few tokens
        if cost.reduce(0, +) <= room { return 0 }
        var must = turns.count - 1 // the newest message always goes, with its question if it's an answer
        if must > 0, turns[must]["role"] != "user" { must -= 1 }
        var used = 0.0, least = must
        for i in stride(from: turns.count - 1, through: 0, by: -1) {
            used += cost[i]
            if used > room && i < must { break }
            least = i
        }
        // The first step boundary at or after `least`: steps are counted from the
        // start of the chat, so the boundary stays put while the chat grows.
        let step = max(1, room / 3)
        var before = 0.0, band = -1.0, start = least
        for i in 0...must {
            let b = (before / step).rounded(.down)
            if i >= least && b > band { start = i; break }
            band = b
            before += cost[i]
        }
        while start < must && turns[start]["role"] != "user" { start += 1 }
        turns.removeFirst(start)
        return start
    }

    /// What Continue sends when an answer ran into the model's limit: these
    /// words, with the end of the answer attached. If the answer filled the
    /// model's memory, the whole of it can't go back, but the model still
    /// needs to see where it stopped.
    static let continueText = "Continue exactly from where you stopped, without repeating anything."
    nonisolated static func answerEnd(_ answer: String) -> ChatFile {
        ChatFile(name: "end of the last answer", text: answer.count > 1200 ? "…" + answer.suffix(1200) : answer)
    }

    /// The model's own word that what it was sent is more than it can read at once.
    nonisolated static func tooLong(_ error: String) -> Bool { error.localizedCaseInsensitiveContains("context") }

    /// After the model said what was sent is too long: how much of the room to
    /// fill on the next try. llama-server says how many tokens it really was,
    /// so the guesses are scaled to match and the next try lands a little
    /// under the room; without that count, it halves.
    nonisolated static func squeeze(_ error: String, sent: [[String: String]], ctx: Int, notes: Int, share: Double) -> Double {
        let guess = Double(sent.reduce(0) { $0 + tokens($1["content"] ?? "") + 8 })
        let e = (try? JSONSerialization.jsonObject(with: Data(error.utf8)) as? [String: Any])?["error"] as? [String: Any]
        let real = Double((e?["n_prompt_tokens"] as? Int) ?? 0)
        if real <= guess || guess <= 0 { return share / 2 }
        let room = Double(Self.room(ctx)), aim = (0.9 * room * guess / real - Double(notes)) / max(1, room - Double(notes))
        return min(max(aim, 0.01), share * 0.8) // always tighter than last time
    }
    nonisolated static func short(_ n: Int) -> String { n >= 1000 ? String(format: "%.1fk", Double(n) / 1000).replacingOccurrences(of: ".0k", with: "k") : "\(n)" }

    // MARK: listening

    /// Reads an answer aloud with the Mac's own voice; pressing again stops it.
    func speak(_ m: ChatMsg) {
        if speakingID == m.id { speech?.stopSpeaking(at: .immediate); speakingID = nil; return }
        speech?.stopSpeaking(at: .immediate)
        let s = speech ?? AVSpeechSynthesizer()
        speech = s
        var text = m.text.replacingOccurrences(of: #"```[\s\S]*?```"#, with: " (code) ", options: .regularExpression)
        text = text.replacingOccurrences(of: #"[*_`#>|]+"#, with: " ", options: .regularExpression)
        s.speak(AVSpeechUtterance(string: String(text.prefix(6000))))
        speakingID = m.id
        let id = m.id
        // AVSpeechSynthesizer has no async end signal without a delegate: check now and then.
        Task { [weak self] in
            while let self, self.speakingID == id {
                try? await Task.sleep(nanoseconds: 500_000_000)
                if !(self.speech?.isSpeaking ?? false) { self.speakingID = nil }
            }
        }
    }

    // MARK: history

    func save() {
        guard !messages.isEmpty else { return }
        // The first question, on one line, is the chat's name in History.
        var title = (messages.first(where: \.user)?.text ?? "Chat").components(separatedBy: .whitespacesAndNewlines).filter { !$0.isEmpty }.joined(separator: " ")
        if title.count > 60 { title = String(title.prefix(57)) + "…" }
        let saved = ChatStore.Saved(id: chatID, title: title, model: modelName, updated: Date(), messages: messages.map { m in
            ChatStore.SavedMsg(user: m.user, model: m.model, text: m.text, thinking: m.thinking, error: m.error, stopped: m.stopped || (!m.user && !m.done), rate: m.rate, tokens: m.tokens, // saved mid-answer: it stopped there
                               left: m.left > 0 ? m.left : nil, cut: m.cut ? true : nil,
                               secs: m.ended.map { $0.timeIntervalSince(m.started) } ?? 0,
                               thinkSecs: m.thinkStart.flatMap { s in m.thinkEnd.map { $0.timeIntervalSince(s) } } ?? 0,
                               files: m.files.map { ChatStore.SavedFile(name: $0.name, text: $0.text) })
        })
        ChatStore.save(saved)
        // Only this chat changed, so put it at the top of the list rather than
        // read every saved chat back from disk: that read took longer than the
        // save and stalled a frame of the chat each time.
        recent.removeAll { $0.id == saved.id }
        recent.insert(saved, at: 0)
        if recent.count > 40 { recent.removeLast(recent.count - 40) }
    }

    func restore(_ s: ChatStore.Saved) {
        if streaming { save() } // keep the turn in progress
        task?.cancel()
        memTask?.cancel()
        memFrom = 0
        messages = s.messages.map { x in
            let m = ChatMsg(user: x.user)
            m.text = x.text; m.thinking = x.thinking; m.error = x.error; m.stopped = x.stopped; m.rate = x.rate; m.tokens = x.tokens; m.model = x.model
            m.left = x.left ?? 0; m.cut = x.cut ?? false
            m.files = x.files.map { ChatFile(name: $0.name, text: $0.text) }
            m.done = true; m.shown = x.text.count
            m.started = Date(); m.ended = m.started.addingTimeInterval(x.secs)
            if !x.thinking.isEmpty { m.thinkStart = Date(); m.thinkEnd = m.thinkStart!.addingTimeInterval(x.thinkSecs) }
            return m
        }
        chatID = s.id
        ctxUsed = 0
        briefing = nil
        inlineNotes = false
        share = 1
    }

    func deleteCurrent() { ChatStore.delete(chatID); newChat(); recent = ChatStore.recent() }
    func deleteAll() { ChatStore.deleteAll(); newChat(); recent = [] }

    /// The chat as plain text with markdown, to paste anywhere.
    var transcript: String {
        messages.filter { !$0.text.isEmpty || !$0.files.isEmpty }.map { m in
            (m.user ? "**You:** " : "**\(m.model.isEmpty ? modelName : m.model):** ") + m.files.map { "[\($0.name)] " }.joined() + m.text
        }.joined(separator: "\n\n")
    }

    // MARK: demo

    /// The demo has no model: a short, honest stand-in shows how answers look.
    private func demoReply(_ m: ChatMsg) async throws {
        try await Task.sleep(nanoseconds: 700_000_000)
        for w in "The person wants to see what an answer looks like. A short reply with a list, a little code and a table shows it all.".split(separator: " ") {
            think(m, w + " ")
            try await Task.sleep(nanoseconds: 45_000_000)
        }
        let answer = "This is **EZLM's demo**, so there's no real model here. In the real app, your model answers right on this computer.\n\n" +
            "Answers can have:\n\n- **lists** like this one\n- `inline code` and [links](https://github.com/picom-code/ezlm)\n- code with a copy button:\n\n" +
            "```python\n# count the words in a file\nwith open(\"notes.txt\") as f:\n    print(len(f.read().split()))\n```\n\n" +
            "| model | size | speed |\n|---|---|---|\n| Qwen 3.5 9B | 5.6 GB | 62 tok/s |\n| Gemma 4 12B | 7.1 GB | 48 tok/s |\n"
        var i = answer.startIndex
        while i < answer.endIndex {
            let j = answer.index(i, offsetBy: 3, limitedBy: answer.endIndex) ?? answer.endIndex
            write(m, String(answer[i..<j]))
            i = j
            try await Task.sleep(nanoseconds: 16_000_000)
        }
        m.rate = 62; m.tokens = 140
        ctxUsed += 420
    }

    /// llama-server's error, in words a person can act on.
    nonisolated static func explain(_ code: Int, _ body: String) -> String {
        var msg = body
        if let obj = try? JSONSerialization.jsonObject(with: Data(body.utf8)) as? [String: Any], let e = obj["error"] {
            msg = (e as? [String: Any])?["message"] as? String ?? "\(e)"
        }
        let low = msg.lowercased()
        if low.contains("context") { return "That's more than the model can read at once, even with older messages left out. Try a shorter message or file." }
        if code == 503 || low.contains("loading") { return "Your model is still loading. Try again in a moment." }
        if code == 401 { return "The server didn't accept EZLM's key. Restart EZLM; if it keeps happening, report a problem." }
        return "Your model answered with an error (\(code)): \(msg)"
    }
}
