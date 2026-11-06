import SwiftUI

// One-click "Open OpenCode / Open Hermes / Open DeepSeek Harness", and the "Report a problem" form.

// MARK: - Use it

extension AppModel {
    /// Opens llama.cpp's built-in chat. It asks for the key, so it's copied first.
    func openChat(_ c: ServerConfig) {
        copy(c.key)
        if !launching {
            launchFailed = false
            launchNote = "Your key is copied: if the chat asks for an API key, paste it into the chat's settings."
        }
        if let url = URL(string: "http://127.0.0.1:\(c.port)/") { NSWorkspace.shared.open(url) }
    }

    /// Opens the app (in a terminal, or a browser for `mode` "web") with the user's
    /// model, setting it up first if it isn't yet. An empty mode is the app's main one.
    func launch(_ t: Target, mode: String = "") async {
        guard !launching else { return }
        launching = true // set first: the job's first events can arrive before the reply
        launchFailed = false
        launchID = t.id
        launchNote = "Opening \(t.name)…"
        var body: [String: Any] = ["id": t.id]
        if !mode.isEmpty { body["mode"] = mode }
        do { try await engine.post("launch", body) } catch {
            launching = false
            launchFailed = true
            launchNote = error.localizedDescription
        }
    }

    func handleLaunch(_ e: EngineEvent) {
        guard launching else { return } // not one this window started
        if e.step == "done" {
            launching = false
            if e.state == "ok" {
                showToast(launchNote.isEmpty ? "Opened." : launchNote, Theme.ok)
                launchNote = ""
            } else {
                launchFailed = e.state != "cancelled"
                let why = e.msg ?? ""
                // The engine's reasons can start lowercase (they're Go errors): this one stands alone.
                launchNote = e.state == "cancelled" ? "Cancelled." : why.isEmpty ? "Couldn't open it." : why.prefix(1).uppercased() + why.dropFirst()
                if launchFailed { showToast(launchNote, Theme.bad); debug("launch: \(launchNote)") }
            }
            Task { await refreshStatus() } // the app may be installed and connected now
            return
        }
        if let msg = e.msg, !msg.isEmpty { launchNote = msg }
    }

    /// An app's connection log (keys already taken out by the engine), newest last.
    func integrationLog(_ id: String) async -> [String] {
        struct Lines: Decodable { var lines: [String]? }
        let q = id.addingPercentEncoding(withAllowedCharacters: .alphanumerics) ?? id
        return ((try? await engine.get("integrate/log?id=\(q)", as: Lines.self))?.lines) ?? []
    }

    /// "Show details": the app's connection log in a sheet.
    func showAppLog(_ t: Target) async {
        let lines = await integrationLog(t.id)
        appLog = AppLogView(id: t.id, name: t.name, lines: lines)
    }

    /// "Report a problem" for an app: the reason first, then the end of its log.
    func reportApp(_ t: Target, reason: String) async {
        let tail = await integrationLog(t.id).suffix(40)
        var text = "\(t.name): \(reason)"
        if !tail.isEmpty { text += "\n\n\(t.name) log (last \(tail.count) lines):\n" + tail.joined(separator: "\n") }
        openReport(Self.hideKeys(text))
    }
}

/// "Chat with it", then one button per way to open each app that can run here,
/// with what the launch is doing underneath and, when it fails, why and what to do.
struct LaunchButtons: View {
    @Environment(AppModel.self) private var m
    let config: ServerConfig

    var body: some View {
        let live = m.serverState == .live
        let targets = (m.status?.targets ?? []).filter(\.supported)
        VStack(alignment: .leading, spacing: 8) {
            FlowRow(spacing: 10) {
                Button("Chat with it") { if !m.setupDone { m.finishSetup() }; m.go(.chat) }.buttonStyle(.term)
                ForEach(targets) { t in
                    ForEach(Array(t.modes.enumerated()), id: \.element.id) { i, mode in
                        Button { Task { await m.launch(t, mode: i == 0 ? "" : mode.id) } } label: {
                            HStack(spacing: 7) {
                                AppLogo(target: t, size: 16)
                                Text(mode.label)
                            }
                        }
                        .buttonStyle(i == 0 ? .term : .small)
                        .disabled(m.launching || !live)
                    }
                }
            }
            if !m.launchNote.isEmpty {
                HStack(alignment: .firstTextBaseline, spacing: 8) {
                    if m.launching { Spinner() }
                    Text(m.launchNote).mono(12, .regular, m.launchFailed ? Theme.bad : Theme.dim)
                        .textSelection(.enabled).fixedSize(horizontal: false, vertical: true)
                }
            }
            if m.launchFailed, !m.launching, let t = targets.first(where: { $0.id == m.launchID }) {
                HStack(spacing: 10) {
                    if t.current == Target.State.needs64K {
                        if t.id == "hermes", m.hermesTradeoff()?.blocked == true {
                            Button("Pick a smaller model") { m.go(.models) }.buttonStyle(.term)
                        } else {
                            Button("Connect again") { m.go(.connect); Task { await m.reconnect(t.id) } }
                                .buttonStyle(.term).disabled(m.appBusy)
                        }
                    }
                    Button("Show details") { Task { await m.showAppLog(t) } }.buttonStyle(.small)
                    Button("Report a problem") { Task { await m.reportApp(t, reason: m.launchNote) } }.buttonStyle(.small)
                }
            }
        }
    }
}

/// Lays its children out left to right and wraps them onto new lines.
struct FlowRow: Layout {
    var spacing: CGFloat = 10

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        let rows = arrange(proposal.width ?? .infinity, subviews)
        return CGSize(width: rows.map(\.width).max() ?? 0, height: rows.map(\.height).reduce(0, +) + spacing * CGFloat(max(rows.count - 1, 0)))
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        var y = bounds.minY
        for row in arrange(bounds.width, subviews) {
            var x = bounds.minX
            for i in row.items {
                let s = subviews[i].sizeThatFits(.unspecified)
                subviews[i].place(at: CGPoint(x: x, y: y + (row.height - s.height) / 2), proposal: .unspecified)
                x += s.width + spacing
            }
            y += row.height + spacing
        }
    }

    private func arrange(_ width: CGFloat, _ subviews: Subviews) -> [(items: [Int], width: CGFloat, height: CGFloat)] {
        var rows: [(items: [Int], width: CGFloat, height: CGFloat)] = []
        var cur: (items: [Int], width: CGFloat, height: CGFloat) = ([], 0, 0)
        for (i, v) in subviews.enumerated() {
            let s = v.sizeThatFits(.unspecified)
            let w = cur.items.isEmpty ? s.width : cur.width + spacing + s.width
            if !cur.items.isEmpty, w > width {
                rows.append(cur)
                cur = ([i], s.width, s.height)
            } else {
                cur = (cur.items + [i], w, max(cur.height, s.height))
            }
        }
        if !cur.items.isEmpty { rows.append(cur) }
        return rows
    }
}

/// "Show details": an app's connection log, as EZLM wrote it (no keys).
struct AppLogSheet: View {
    @Environment(AppModel.self) private var m
    let log: AppLogView

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            VStack(alignment: .leading, spacing: 4) {
                Text("\(log.name): connection log").mono(18, .bold)
                Text("What EZLM did and what \(log.name) said, newest last. Keys are never written here.")
                    .mono(12, .regular, Theme.dim).fixedSize(horizontal: false, vertical: true)
            }
            ScrollView {
                Text(log.lines.isEmpty ? "Nothing logged yet." : log.lines.joined(separator: "\n"))
                    .mono(11, .regular, log.lines.isEmpty ? Theme.dim : Theme.ink)
                    .textSelection(.enabled)
                    .padding(10)
                    .frame(maxWidth: .infinity, minHeight: 300, alignment: .topLeading)
            }
            .defaultScrollAnchor(.bottom)
            .frame(height: 300)
            .overlay(Rectangle().stroke(Theme.faint, lineWidth: 1))
            HStack(spacing: 10) {
                Button("Close") { m.appLog = nil }.buttonStyle(.primary).keyboardShortcut(.defaultAction)
                Button("Copy") { m.copy(log.lines.joined(separator: "\n")) }.buttonStyle(.term)
                Button("Report a problem") {
                    let t = m.status?.targets.first { $0.id == log.id }
                    let reason = t.map { $0.stateLine.text } ?? "a problem with \(log.name)"
                    if let t { Task { await m.reportApp(t, reason: reason) } }
                }
                .buttonStyle(.term)
            }
        }
        .padding(30)
        .frame(width: 680, alignment: .leading)
        .background(Theme.black)
        .preferredColorScheme(.dark)
    }
}

// MARK: - Report a problem

extension AppModel {
    /// Opens the report form, its box filled with `text` (an error, say).
    func openReport(_ text: String = "") {
        let draft = ReportDraft(text: text.isEmpty ? unsentReport : text)
        if appLog != nil {
            appLog = nil
            Task {
                try? await Task.sleep(nanoseconds: 350_000_000)
                reportDraft = draft
            }
        } else if installSheet {
            // One sheet at a time: let the switch sheet go first.
            closeInstallSheet()
            Task {
                try? await Task.sleep(nanoseconds: 350_000_000)
                reportDraft = draft
            }
        } else {
            reportDraft = draft
        }
    }

    func sendReport(_ text: String, details: Bool) async {
        showToast("Sending your report…")
        unsentReport = ""
        do {
            // The crash note goes last: the engine keeps the end of a long log.
            let r: ReportResult = try await engine.post("report", ["text": text, "details": details, "app_log": AppLog.shared.tail(6000) + (CrashNote.latest().map { "\n\n" + $0 } ?? "")])
            if r.sent {
                showToast("Sent. Thank you! (report #\(r.number))", Theme.ok)
                return
            }
            guard let url = URL(string: r.url) else { throw EngineError.message("the report link didn't work") }
            NSWorkspace.shared.open(url)
            showToast("Your report is filled in on GitHub: press Submit there to send it.", Theme.gold)
        } catch {
            debug("report: \(error.localizedDescription)")
            // EZLM's engine couldn't take it (it may be what stopped): fill in GitHub's page from here.
            if let url = Self.reportPage(text, details: details) {
                NSWorkspace.shared.open(url)
                showToast("Your report is filled in on GitHub: press Submit there to send it.", Theme.gold)
            } else {
                unsentReport = text
                showToast("Couldn't send the report: \(error.localizedDescription)", Theme.bad)
            }
        }
    }

    /// GitHub's new-issue page with the report filled in, made without the engine
    /// (the same shape internal/report makes). Keys and the home folder are hidden.
    static func reportPage(_ text: String, details: Bool) -> URL? {
        let version = Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "dev"
        #if arch(arm64)
        let arch = "arm64"
        #else
        let arch = "x86_64"
        #endif
        let first = text.split(separator: "\n", omittingEmptySubsequences: false).first.map(String.init) ?? text
        let title = "Beta report: " + (first.count > 70 ? String(first.prefix(70)) + "…" : first)
        var body = "### What happened\n\n\(text)\n\n### Setup\n\n- EZLM \(version) (Mac app) on macOS \(ProcessInfo.processInfo.operatingSystemVersionString) (\(arch))\n- Sent without EZLM's engine, which couldn't be reached\n"
        if details { body += "\n<details><summary>Logs</summary>\n\nApp:\n```\n\(AppLog.shared.tail(3000))\n```\n</details>\n" }
        body = hideKeys(body).replacingOccurrences(of: NSHomeDirectory(), with: "~")
        if body.count > 5500 { body = String(body.prefix(5500)) + "\n\n…(trimmed)" }
        guard var c = URLComponents(string: "https://github.com/picom-code/ezlm/issues/new") else { return nil }
        c.queryItems = [URLQueryItem(name: "labels", value: "beta-report"), URLQueryItem(name: "title", value: title), URLQueryItem(name: "body", value: body)]
        // A "+" left as is would arrive as a space.
        let query = c.percentEncodedQuery?.replacingOccurrences(of: "+", with: "%2B")
        c.percentEncodedQuery = query
        return c.url
    }
}

/// Presents the report form from anywhere: the wizard, the main app, a failure.
struct ReportHost: ViewModifier {
    @Environment(AppModel.self) private var m
    func body(content: Content) -> some View {
        content.sheet(item: Binding(get: { m.reportDraft }, set: { if $0 == nil { m.reportDraft = nil } })) { d in
            ReportSheet(draft: d).environment(m)
        }
        .sheet(item: Binding(get: { m.appLog }, set: { if $0 == nil { m.appLog = nil } })) { l in
            AppLogSheet(log: l).environment(m)
        }
    }
}

struct ReportSheet: View {
    @Environment(AppModel.self) private var m
    @State private var text: String
    @State private var details = true
    @State private var note = ""
    @FocusState private var focused: Bool

    init(draft: ReportDraft) {
        _text = State(initialValue: draft.text)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            VStack(alignment: .leading, spacing: 4) {
                Text("Report a problem").mono(18, .bold)
                Text("This posts on EZLM's public GitHub page, where the people testing EZLM read it. Thanks for helping.")
                    .mono(12, .regular, Theme.dim).fixedSize(horizontal: false, vertical: true)
            }
            ZStack(alignment: .topLeading) {
                TextEditor(text: $text)
                    .font(Theme.mono(13))
                    .foregroundStyle(Theme.ink)
                    .scrollContentBackground(.hidden)
                    .focused($focused)
                    .accessibilityLabel("What happened")
                if text.isEmpty {
                    Text("What happened? What did you expect instead?").mono(13, .regular, Theme.faint)
                        .padding(.leading, 5)
                        .allowsHitTesting(false)
                        .accessibilityHidden(true)
                }
            }
            .padding(8)
            .frame(height: 150)
            .overlay(Rectangle().stroke(focused ? Theme.dim : Theme.faint, lineWidth: 1))
            .onChange(of: text) { if !note.isEmpty { note = "" } }
            Toggle(isOn: $details) {
                Text("Include technical details: your hardware, versions and recent logs. Keys and your files are never included.")
                    .mono(12).fixedSize(horizontal: false, vertical: true)
            }
            .toggleStyle(.checkbox)
            if !note.isEmpty { Text(note).mono(12, .regular, Theme.bad) }
            HStack(spacing: 10) {
                // ⌘↩, since ↩ in the box starts a new line.
                Button(action: send) { HStack(spacing: 14) { Text("Send report"); Text("⌘↩").foregroundStyle(Theme.dim) } }
                    .buttonStyle(.primary).keyboardShortcut(.return, modifiers: .command)
                Button("Cancel") { m.reportDraft = nil }.buttonStyle(.term).keyboardShortcut(.cancelAction)
            }
            .padding(.top, 2)
        }
        .padding(30)
        .frame(width: 600, alignment: .leading)
        .background(Theme.black)
        .preferredColorScheme(.dark)
        .onAppear { DispatchQueue.main.async { focused = true } }
    }

    private func send() {
        let t = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !t.isEmpty else {
            note = "Say what happened first, even in a few words."
            return
        }
        m.reportDraft = nil
        Task { await m.sendReport(t, details: details) }
    }
}

/// EZLM's newest crash in the last week, from the report macOS keeps
/// (~/Library/Logs/DiagnosticReports): when, which version, what went wrong
/// and where. The app's own log is lost in a crash, so a problem report
/// includes this instead. No paths, names or anything personal.
enum CrashNote {
    static func latest(maxAge: TimeInterval = 7 * 86400) -> String? {
        // Reports macOS has already sent on (crash dialog, Mac Analytics) move to Retired.
        let reports = FileManager.default.homeDirectoryForCurrentUser.appendingPathComponent("Library/Logs/DiagnosticReports")
        let files = [reports, reports.appendingPathComponent("Retired")].flatMap {
            (try? FileManager.default.contentsOfDirectory(at: $0, includingPropertiesForKeys: [.contentModificationDateKey])) ?? []
        }
        let recent = files.filter { $0.lastPathComponent.hasPrefix("EZLM") && $0.pathExtension == "ips" }
            .compactMap { u -> (URL, Date)? in
                guard let d = try? u.resourceValues(forKeys: [.contentModificationDateKey]).contentModificationDate,
                      Date().timeIntervalSince(d) < maxAge else { return nil }
                return (u, d)
            }
            .sorted { $0.1 > $1.1 }
        for (url, _) in recent.prefix(5) { // newest first; one that won't parse doesn't hide an older crash
            if let text = try? String(contentsOf: url, encoding: .utf8), let note = summary(text) { return note }
        }
        return nil
    }

    /// A crash report (.ips) is a one-line JSON header, then the report as JSON.
    static func summary(_ ips: String) -> String? {
        guard let nl = ips.firstIndex(of: "\n"),
              let head = try? JSONSerialization.jsonObject(with: Data(ips[..<nl].utf8)) as? [String: Any],
              let body = try? JSONSerialization.jsonObject(with: Data(ips[ips.index(after: nl)...].utf8)) as? [String: Any]
        else { return nil }
        var lines = ["EZLM crashed \(head["timestamp"] as? String ?? "?"), version \(head["app_version"] as? String ?? "?"), \(head["os_version"] as? String ?? "")"]
        if let e = body["exception"] as? [String: Any] {
            lines.append("exception: " + [e["type"], e["signal"], e["subtype"]].compactMap { $0 as? String }.joined(separator: " "))
        }
        if let t = body["termination"] as? [String: Any], let why = t["indicator"] as? String { lines.append("termination: \(why)") }
        // Swift's own words for it ("Index out of range"), with any home folder taken out.
        if let asi = body["asi"] as? [String: Any] {
            let words = asi.values.compactMap { $0 as? [String] }.joined().joined(separator: " ")
            let clean = words.replacingOccurrences(of: #"/Users/[^/\s]+"#, with: "~", options: .regularExpression)
            if !clean.isEmpty { lines.append("message: " + String(clean.prefix(300))) }
        }
        let images = body["usedImages"] as? [[String: Any]] ?? []
        if let threads = body["threads"] as? [[String: Any]], !threads.isEmpty {
            let i = body["faultingThread"] as? Int ?? 0
            for f in (threads[min(max(i, 0), threads.count - 1)]["frames"] as? [[String: Any]] ?? []).prefix(12) {
                let image = (f["imageIndex"] as? Int).flatMap { $0 >= 0 && $0 < images.count ? images[$0]["name"] as? String : nil } ?? "?"
                lines.append("  \(image)  \(f["symbol"] as? String ?? "?")")
            }
        }
        return lines.joined(separator: "\n")
    }
}
