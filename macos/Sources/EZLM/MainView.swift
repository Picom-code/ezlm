import SwiftUI

// MARK: - Shell

struct MainView: View {
    @Environment(AppModel.self) private var m

    var body: some View {
        @Bindable var m = m
        NavigationSplitView {
            Sidebar()
                .navigationSplitViewColumnWidth(min: 200, ideal: 214, max: 260)
        } detail: {
            Pane(section: m.section)
                .background(Theme.black)
        }
        .sheet(isPresented: $m.installSheet) { InstallSheet().environment(m) }
    }
}

struct Sidebar: View {
    @Environment(AppModel.self) private var m

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            MiniLogo().padding(.horizontal, 18).padding(.top, 40).padding(.bottom, 16)
            SidebarStatus().padding(.horizontal, 12).padding(.bottom, 14)
            List(selection: Binding(get: { m.section }, set: { if let s = $0 { m.go(s) } })) {
                ForEach(Section.allCases) { s in
                    Label { Text(s.rawValue).font(Theme.mono(13)) } icon: { Image(systemName: s.icon) }
                        .tag(s)
                }
            }
            .listStyle(.sidebar)
            .scrollContentBackground(.hidden)

            VStack(alignment: .leading, spacing: 10) {
                Button("Report a problem") { m.openReport() }.buttonStyle(.small)
                if Engine.demo { Text("Demo mode").mono(10, .regular, Theme.faint) }
            }
            .padding(.horizontal, 16).padding(.bottom, 16).padding(.top, 8)
        }
        .background(Color(white: 0.055))
    }
}

/// Live / Loading model… / Stopped / Not set up, with the model's name.
private struct SidebarStatus: View {
    @Environment(AppModel.self) private var m
    var body: some View {
        let st = m.serverState
        let l = m.live
        let speed = l?.activity == "writing" ? "\(Int(l?.tokPerSec ?? 0)) tok/s" : l?.activity == "reading" ? "reading" : ""
        VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 8) {
                Pulse(color: st.color, active: st == .live || st == .loading)
                Text(st.title).mono(12, .bold)
                Spacer()
                Text(speed).mono(11, .regular, Theme.signal)
            }
            Text(m.status?.config?.modelName ?? "No model").mono(11, .regular, Theme.dim).lineLimit(1)
        }
        .padding(12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .overlay(Rectangle().stroke(Theme.rule, lineWidth: 1))
        .accessibilityElement(children: .combine)
    }
}

struct Pane: View {
    @Environment(AppModel.self) private var m
    let section: Section
    var body: some View {
        if section == .chat {
            ChatPane().id(section) // its own layout: the message box stays at the bottom
        } else {
            scrolling
        }
    }

    private var scrolling: some View {
        ScrollViewReader { proxy in
            ScrollView {
                VStack(alignment: .leading, spacing: 0) {
                    switch section {
                    case .chat: EmptyView()
                    case .overview: OverviewPane()
                    case .models: ModelsPane()
                    case .system: SystemPane()
                    case .connect: ConnectPane()
                    case .keys: KeysPane()
                    case .logs: LogsPane()
                    case .settings: SettingsPane()
                    }
                }
                .frame(maxWidth: 760, alignment: .leading)
                .padding(.horizontal, 34).padding(.top, 34).padding(.bottom, 30)
                .frame(maxWidth: .infinity, alignment: .leading)
            }
            .scrollIndicators(.automatic)
            // Keep the selected model in view as ↑/↓ move through the list.
            .onChange(of: m.chosenID) { _, id in
                guard section == .models, let id else { return }
                withAnimation(.easeOut(duration: 0.2)) { proxy.scrollTo(id) }
            }
        }
        .id(section)
        .transition(.opacity)
    }
}

struct PaneHeader: View {
    let title: String
    let subtitle: String
    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(title).mono(20, .bold)
            Text(subtitle).mono(12, .regular, Theme.dim)
        }
        .padding(.bottom, 24)
    }
}

// MARK: - Overview

struct OverviewPane: View {
    @Environment(AppModel.self) private var m

    var body: some View {
        if let s = m.status, s.deployed, let c = s.config {
            let st = m.serverState
            VStack(alignment: .leading, spacing: 0) {
                HStack(alignment: .firstTextBaseline) {
                    VStack(alignment: .leading, spacing: 4) {
                        Text(c.modelName).mono(22, .bold)
                        Text("\(c.quant), \(c.ctx / 1024)k context").mono(12, .regular, Theme.dim)
                    }
                    Spacer()
                    HStack(spacing: 8) {
                        Pulse(color: st.color, active: st == .live || st == .loading)
                        Text(st == .notSetUp ? ServerState.stopped.title : st.title).mono(13, .bold, st.color)
                    }
                }
                .padding(.bottom, 18)

                if AppPlace.temporary { MoveToApplicationsNote().padding(.bottom, 14) }
                if let latest = m.availableUpdate { UpdateBanner(latest: latest) }
                if s.unmanaged { UnmanagedCard() }
                if let why = c.cpuFallback, !why.isEmpty, !s.unmanaged { CPUFallbackCard(why: why) }
                if let tune = s.tune, st == .live, !m.installRunning, !s.unmanaged { TuneCard(tune: tune) }
                ActivityCard()
                LiveTiles().padding(.bottom, 26)

                SectionTitle("Connect with")
                EndpointBlock(url: s.localUrl ?? "", key: c.key)
                ConnectedApps(targets: s.targets).padding(.top, 10)

                SectionTitle("Use it")
                LaunchButtons(config: c)
                HStack(spacing: 12) {
                    Button("Connect another app") { m.go(.connect) }.buttonStyle(.small)
                    Button("Manage keys") { m.go(.keys) }.buttonStyle(.small)
                }
                .padding(.top, 10)

                ModelCheck()

                SectionTitle("Memory right now")
                MemoryNow()

                SectionTitle("Server")
                HStack(spacing: 12) {
                    if st == .live || st == .loading {
                        Button("Stop server") { Task { await m.stopServer() } }.buttonStyle(.term)
                    } else {
                        Button("Start server") { Task { await m.startServer() } }.buttonStyle(.primary)
                    }
                    Button("Change model") { m.go(.models) }.buttonStyle(.term)
                    Button("Free up memory") { m.startModelMode(.overview) }.buttonStyle(.term)
                }
                if !m.note.isEmpty { Text(m.note).mono(12, .regular, Theme.dim).padding(.top, 8).fixedSize(horizontal: false, vertical: true) }
            }
        } else if m.status != nil {
            VStack(alignment: .leading, spacing: 0) {
                PaneHeader(title: "Nothing is set up yet", subtitle: "Pick a model and EZLM installs everything for you.")
                Button { Task { await m.backToSetup() } } label: { HStack(spacing: 14) { Text("Choose a model"); Text("↩").foregroundStyle(Theme.dim) } }
                    .buttonStyle(.primary).keyboardShortcut(.defaultAction)
            }
        }
    }
}

/// "A faster setup is available": a better way to run the model that's running.
private struct TuneCard: View {
    @Environment(AppModel.self) private var m
    let tune: Tune
    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("A faster setup is available for this model").mono(13, .bold)
            Text(tune.why).mono(12, .regular, Theme.dim).fixedSize(horizontal: false, vertical: true)
            HStack(spacing: 10) {
                Button("Apply it") { Task { await m.applyTune() } }.buttonStyle(.primary)
                Text("Restarts your server, about a minute. Nothing is downloaded.").mono(11, .regular, Theme.dim)
            }
            .padding(.top, 2)
        }
        .padding(EdgeInsets(top: 12, leading: 14, bottom: 12, trailing: 14))
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Theme.signal.opacity(0.055))
        .overlay(Rectangle().stroke(Theme.signal.opacity(0.55), lineWidth: 1))
        .padding(.bottom, 14)
    }
}

/// The server answers but EZLM can't control its process.
/// The graphics card couldn't load the model, so it runs on the processor.
private struct CPUFallbackCard: View {
    @Environment(AppModel.self) private var m
    let why: String
    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Running on the processor").mono(13, .bold, Theme.gold)
            Text("Your graphics card couldn't load this model (\(why)), so it runs on the processor, slower than it could be. A newer graphics driver may fix this.")
                .mono(12, .regular, Theme.dim).fixedSize(horizontal: false, vertical: true)
            Button("Try the graphics card again") { Task { await m.retryGraphicsCard() } }.buttonStyle(.small).disabled(m.installRunning)
        }
        .padding(EdgeInsets(top: 12, leading: 14, bottom: 12, trailing: 14))
        .frame(maxWidth: .infinity, alignment: .leading)
        .overlay(Rectangle().stroke(Theme.gold.opacity(0.6), lineWidth: 1))
        .padding(.bottom, 14)
    }
}

private struct UnmanagedCard: View {
    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("EZLM can't control this server right now").mono(13, .bold, Theme.gold)
            Text("It was started with administrator rights, so EZLM can't stop it or switch models. It keeps working for your apps. Restart your Mac (or quit \"llama-server\" in Activity Monitor) and EZLM takes over again.")
                .mono(12, .regular, Theme.dim).fixedSize(horizontal: false, vertical: true)
        }
        .padding(EdgeInsets(top: 12, leading: 14, bottom: 12, trailing: 14))
        .frame(maxWidth: .infinity, alignment: .leading)
        .overlay(Rectangle().stroke(Theme.gold.opacity(0.6), lineWidth: 1))
        .padding(.bottom, 14)
    }
}

/// One card that always says what the model is doing right now.
private struct ActivityCard: View {
    @Environment(AppModel.self) private var m

    var body: some View {
        let st = m.serverState
        let l = m.live ?? Live()
        let phase = st == .loading ? "loading" : (st == .stopped || st == .notSetUp) ? "stopped" : l.activity
        let startError = phase == "stopped" ? (m.status?.startError ?? "") : ""
        VStack(alignment: .leading, spacing: 6) {
            HStack(alignment: .firstTextBaseline, spacing: 10) {
                Group {
                    switch phase {
                    case "reading", "loading": Spinner()
                    case "writing": Spinner(color: Theme.ok)
                    case "stopped": Text("■").mono(11, .regular, Theme.bad)
                    default: Text("●").mono(11, .regular, Theme.ok)
                    }
                }
                .frame(width: 16, alignment: .leading)
                Text(title(phase, failed: startError)).mono(13)
            }
            Text(detail(phase, l, failed: startError)).mono(12, .regular, Theme.dim).fixedSize(horizontal: false, vertical: true)
            if phase == "reading" {
                let pct = l.promptTotal > 0 ? Double(l.promptDone) / Double(l.promptTotal) : 0
                BlockBar(pct, height: 5).animation(.easeOut(duration: 0.5), value: pct).frame(maxWidth: 420).padding(.top, 2)
            } else if phase == "writing" {
                WorkingBar(height: 5, color: Theme.ok).frame(maxWidth: 420).padding(.top, 2)
            }
            if phase == "stopped" {
                HStack(spacing: 10) {
                    Button("Start server") { Task { await m.startServer() } }.buttonStyle(.primary)
                    if !startError.isEmpty {
                        Button("Report this problem") { m.openReport("The server couldn't start: \(startError)") }.buttonStyle(.term)
                    }
                }
                .padding(.top, 6)
            }
            if let f = m.ovFill {
                PourPanel(fill: f).id(ObjectIdentifier(f)).padding(.top, 8)
                    .transition(.opacity)
            }
        }
        .padding(EdgeInsets(top: 12, leading: 14, bottom: 12, trailing: 14))
        .frame(maxWidth: .infinity, alignment: .leading)
        .overlay(Rectangle().stroke(Theme.rule, lineWidth: 1))
        .padding(.bottom, 20)
        .animation(.easeOut(duration: 0.3), value: m.ovFill.map(ObjectIdentifier.init))
    }

    private func title(_ phase: String, failed: String) -> String {
        switch phase {
        case "reading": return "Reading a prompt"
        case "writing": return "Writing a reply"
        case "loading": return "Loading the model into memory"
        case "stopped": return failed.isEmpty ? "The server is stopped" : "The server couldn't start"
        default: return "Ready"
        }
    }

    private func detail(_ phase: String, _ l: Live, failed: String) -> String {
        if phase == "stopped" && !failed.isEmpty {
            return failed + ". Try Start server again; if it keeps happening, report the problem so it can be fixed."
        }
        let waiting = l.queued > 0 ? ", \(l.queued) more waiting" : ""
        switch phase {
        case "reading":
            let pct = l.promptTotal > 0 ? Double(l.promptDone) / Double(l.promptTotal) : 0
            return "\(l.promptDone.formatted()) of \(l.promptTotal.formatted()) tokens (\(Int((pct * 100).rounded()))%)\(waiting). Long prompts from coding agents take a moment before the reply starts."
        case "writing":
            return "\(l.written.formatted()) tokens so far at \(Int(l.tokPerSec)) tok/s\(waiting)"
        case "loading":
            return "This takes a little while after a restart. Requests work as soon as it's done."
        case "stopped":
            return "Apps can't reach your model until you start it again."
        default:
            return "Waiting for requests from your apps."
        }
    }
}

/// Writing speed, Generated, Waiting, Up for.
private struct LiveTiles: View {
    @Environment(AppModel.self) private var m
    var body: some View {
        let l = m.live
        HStack(spacing: 10) {
            StatTile(label: "Writing speed", value: m.lastSpeed > 0 ? "\(Int(m.lastSpeed))" : "–", unit: "tok/s")
            StatTile(label: "Generated", value: compact(l?.tokens ?? 0), unit: "tokens")
            StatTile(label: "Waiting", value: "\(l?.queued ?? 0)", unit: "requests")
            StatTile(label: "Up for", value: (l?.uptimeS ?? 0) > 0 ? uptimeText(l?.uptimeS ?? 0) : "–", unit: "")
        }
    }

    private func compact(_ n: UInt64) -> String {
        n >= 1_000_000 ? String(format: "%.1fM", Double(n) / 1e6) : n >= 1000 ? String(format: "%.1fk", Double(n) / 1e3) : "\(n)"
    }
}

private struct ModelCheck: View {
    @Environment(AppModel.self) private var m
    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack(alignment: .firstTextBaseline) {
                SectionTitle("Model check")
                Button(m.testing ? "Testing…" : m.testResults.isEmpty ? "Test model" : "Test again") { Task { await m.runTests() } }
                    .buttonStyle(.small).disabled(m.testing || m.serverState != .live)
            }
            if m.testResults.isEmpty && !m.testing {
                Text("Runs five real requests: speed, reasoning, tool calling, following instructions, and long-context recall.")
                    .mono(12, .regular, Theme.dim)
            }
            ForEach(m.testResults, id: \.name) { r in
                LogLine(state: r.pass ? .ok : .failed, label: r.name, sub: r.detail + String(format: "  (%.1fs)", r.secs))
                    .transition(.opacity.combined(with: .move(edge: .top)))
            }
            if m.testing {
                LogLine(state: .running, label: ["Speed", "Reasoning", "Tool calling", "Following instructions", "Long-context recall"][min(m.testResults.count, 4)])
            } else if !m.testResults.isEmpty {
                let passed = m.testResults.filter(\.pass).count
                Text("\(passed) of \(m.testResults.count) passed").mono(12, .regular, passed == m.testResults.count ? Theme.ok : Theme.gold)
                    .padding(.leading, 24).padding(.top, 4)
            }
        }
    }
}

/// Live memory bars (one shared bar on a Mac).
private struct MemoryNow: View {
    @Environment(AppModel.self) private var m
    var body: some View {
        if let l = m.live, l.ramTotal > 0 {
            if l.unified {
                LiveBar(label: "Memory, shared with graphics", total: l.ramTotal, free: l.ramAvail)
            } else {
                if l.gpuTotal > 0 { LiveBar(label: "Graphics memory", total: l.gpuTotal, free: l.gpuFree) }
                LiveBar(label: "System memory", total: l.ramTotal, free: l.ramAvail)
            }
        }
    }
}

struct StatTile: View {
    let label: String
    let value: String
    let unit: String
    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(label).mono(11, .regular, Theme.dim)
            HStack(alignment: .firstTextBaseline, spacing: 5) {
                Text(value).mono(22, .bold).monospacedDigit()
                Text(unit).mono(11, .regular, Theme.dim)
            }
            Spacer(minLength: 0)
        }
        .padding(12)
        .frame(maxWidth: .infinity, minHeight: 80, alignment: .topLeading)
        .overlay(Rectangle().stroke(Theme.rule, lineWidth: 1))
        .accessibilityElement(children: .combine)
    }
}

/// A live-updating memory bar (used vs free); gold when nearly full.
struct LiveBar: View {
    let label: String
    let total: UInt64
    let free: UInt64
    var body: some View {
        let used = total > free ? total - free : 0
        let frac = Double(used) / Double(max(total, 1))
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Text(label).mono(13, .regular, Theme.dim)
                Spacer()
                (Text(gb(used)).foregroundColor(Theme.ink) + Text(" used of \(gb(total))").foregroundColor(Theme.dim)).font(Theme.mono(12))
            }
            BlockBar(frac, color: frac > 0.95 ? Theme.gold : Theme.signal, height: 14)
                .animation(.easeOut(duration: 0.5), value: frac)
        }
        .padding(.bottom, 14)
    }
}

// MARK: - Models

struct ModelsPane: View {
    @Environment(AppModel.self) private var m

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            PaneHeader(title: "Models", subtitle: "Switch any time: your URL, keys and connected apps stay the same.")
            InstalledSection().padding(.bottom, 6)
            SectionTitle("Get another model")
            PrefPicker()
            if let r = m.scan {
                if !m.note.isEmpty { Text(m.note).mono(12, .regular, Theme.bad).padding(.top, 10).fixedSize(horizontal: false, vertical: true) }
                Text("Click a model to see its button, double-click or press ↩ to switch. Your current model keeps answering while a new one downloads.")
                    .mono(11, .regular, Theme.dim).padding(.top, 10).fixedSize(horizontal: false, vertical: true)
                PlanList(scan: r).padding(.top, 12)
                Button("Scan again") { Task { await m.rescan() } }.buttonStyle(.term).padding(.top, 16)
            } else {
                HStack(spacing: 10) { Spinner(); Text("Comparing models against your memory").mono() }.padding(.top, 14)
                if !m.note.isEmpty { Text(m.note).mono(12, .regular, Theme.bad).padding(.top, 8) }
            }
        }
        .task {
            if m.scan == nil { await m.refreshScan() }
            // Start on the running model (or the recommendation) if the selection isn't in the list.
            let ids = Set(m.visiblePlans.map(\.id))
            if m.chosenID == nil || !ids.contains(m.chosenID ?? "") {
                m.chosenID = m.currentModelID.flatMap { ids.contains($0) ? $0 : nil } ?? m.scan?.recommended?.id
            }
        }
    }
}

/// The model options. Cards are built once per scan: moving the selection only
/// restyles two of them and moves the action button.
private struct PlanList: View {
    @Environment(AppModel.self) private var m
    let scan: ScanResult

    var body: some View {
        let plans = m.visiblePlans
        let maxScore = plans.map(\.model.score).max() ?? 1
        let current = m.currentModelID
        VStack(alignment: .leading, spacing: 10) {
            ForEach(plans) { p in
                let selected = p.id == m.chosenID
                OptionCard(plan: p, hw: scan.hw, maxScore: maxScore, recommended: p.id == scan.recommended?.id,
                           running: p.id == current, selected: selected,
                           action: selected ? m.switchAction(p) : nil,
                           onSelect: { if m.chosenID != p.id { m.chosenID = p.id } },
                           onAction: { m.requestDeploy(p) })
                    .id(p.id)
            }
        }
        .arrowSelection(plans: plans)
    }
}

struct InstallSheet: View {
    @Environment(AppModel.self) private var m
    var body: some View {
        DeployView(inSheet: true)
            .padding(30)
            .frame(width: 600)
            .background(Theme.black)
            .preferredColorScheme(.dark)
            .interactiveDismissDisabled()
    }
}

// MARK: - System

struct SystemPane: View {
    @Environment(AppModel.self) private var m
    @State private var confirmRemove = false

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            PaneHeader(title: "System", subtitle: "What this machine has, and what's using it. Memory updates live.")
            ModelModeCard()
            if let r = m.scan {
                let hw = r.hw
                let engines = ["cuda": "NVIDIA CUDA", "metal": "Apple Metal", "vulkan": "Vulkan", "rocm": "AMD ROCm", "cpu": "Processor only"]
                Fact(k: "System", v: hw.osName ?? hw.os)
                Fact(k: "Processor", v: "\(hw.cpuName), \(hw.cores) threads")
                Fact(k: "Graphics", v: hw.unified ? "Built into the chip, shares system memory"
                     : (hw.gpus ?? []).isEmpty ? "None found"
                     : (hw.gpus ?? []).map { $0.name + ($0.driver.map { ", driver \($0)" } ?? "") }.joined(separator: "\n"))
                Fact(k: "Engine", v: engines[hw.backend] ?? hw.backend)
                Fact(k: "Storage", v: "\(gb(hw.diskFree)) free" + (hw.modelsDir.map { " in \($0)" } ?? "")).padding(.bottom, 20)
                MemoryNow()
                ProcTable(hw: hw)
            } else {
                HStack(spacing: 10) { Spinner(); Text("Reading your hardware").mono() }
            }

            SectionTitle("Remove EZLM's models")
            Text("Stops your model, removes it from startup, and deletes the models EZLM downloaded plus its settings and keys. To remove the app itself, drag EZLM from Applications to the Trash.")
                .mono(12, .regular, Theme.dim).fixedSize(horizontal: false, vertical: true)
            Button("Remove models and settings") { confirmRemove = true }.buttonStyle(.danger).padding(.top, 10)
            if !m.note.isEmpty { Text(m.note).mono(12, .regular, Theme.dim).padding(.top, 8) }
        }
        .task { if m.scan == nil { await m.refreshScan() } }
        .confirmationDialog("Remove EZLM's models and settings?", isPresented: $confirmRemove) {
            Button("Remove everything", role: .destructive) { Task { await m.uninstall() } }
        } message: { Text("Apps you connected will stop working until you set EZLM up again. This can't be undone.") }
    }
}

/// "Model mode": close apps you're not using so the model gets more memory.
private struct ModelModeCard: View {
    @Environment(AppModel.self) private var m
    var body: some View {
        // Graphics memory is what matters most on a PC-style graphics card; a Mac shares one pool.
        let shared = m.scan?.hw.unified ?? m.live?.unified ?? false
        VStack(alignment: .leading, spacing: 8) {
            Text("Model mode").mono(13, .bold)
            Text(shared ? "Close apps you're not using so your AI gets more memory. EZLM asks about each app; nothing closes without your OK."
                        : "Close apps you're not using so your AI gets more memory, especially graphics memory. EZLM asks about each app; nothing closes without your OK.")
                .mono(12, .regular, Theme.dim).fixedSize(horizontal: false, vertical: true)
            Button("Start Model mode") { m.startModelMode(.system) }.buttonStyle(.primary).disabled(m.installRunning)
                .padding(.top, 2)
        }
        .padding(EdgeInsets(top: 12, leading: 14, bottom: 12, trailing: 14))
        .frame(maxWidth: .infinity, alignment: .leading)
        .overlay(Rectangle().stroke(Theme.ok.opacity(0.43), lineWidth: 1))
        .padding(.bottom, 20)
    }
}

// MARK: - Connect

struct ConnectPane: View {
    @Environment(AppModel.self) private var m
    var body: some View {
        @Bindable var m = m
        VStack(alignment: .leading, spacing: 0) {
            PaneHeader(title: "Connect", subtitle: "Use your model inside these apps. EZLM installs them if needed, adds itself as a provider, and checks that it answers.")
            if let s = m.status {
                TileGrid { ForEach(s.targets) { t in AppTile(target: t) } }
                Toggle(isOn: $m.makeDefault) { Text("Make it the default model in those apps").mono() }
                    .toggleStyle(.checkbox).padding(.top, 14)
                let names = s.targets.filter { m.selectedApps.contains($0.id) }.map(\.name)
                Button { Task { await m.connectApps() } } label: {
                    HStack(spacing: 14) {
                        Text(names.isEmpty ? "Pick an app above" : "Connect \(names.joined(separator: " and "))")
                        if !names.isEmpty { Text("↩").foregroundStyle(Theme.dim) }
                    }
                }
                .buttonStyle(.primary).keyboardShortcut(.defaultAction)
                .disabled(names.isEmpty || m.appBusy || m.serverState != .live)
                .padding(.top, 14)
                if m.serverState != .live {
                    Text("Start your server first: connecting checks that the app gets an answer.").mono(12, .regular, Theme.gold).padding(.top, 8)
                }
                if m.appBusy {
                    Text("The first answer can take a minute: coding agents send long instructions with every request.").mono(12, .regular, Theme.dim).padding(.top, 8)
                }
                if !m.note.isEmpty { Text(m.note).mono(12, .regular, Theme.dim).padding(.top, 8).fixedSize(horizontal: false, vertical: true) }
            }
        }
    }
}

// MARK: - Keys

struct KeysPane: View {
    @Environment(AppModel.self) private var m
    @State private var newKeyName = ""
    @State private var confirmRotate: APIKey?
    @State private var confirmRevoke: APIKey?
    @State private var copied: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            PaneHeader(title: "Keys", subtitle: "Give each device or app its own key, so you can revoke one without breaking the others.")
            if let s = m.status, let c = s.config {
                let keys = c.keys ?? [APIKey(id: "k1", name: "Default", key: c.key)]
                VStack(spacing: 0) {
                    ForEach(Array(keys.enumerated()), id: \.element.id) { i, k in
                        HStack(alignment: .firstTextBaseline, spacing: 14) {
                            VStack(alignment: .leading, spacing: 2) {
                                Text(k.name).mono(13)
                                if i == 0 { Text("used by connected apps").mono(11, .regular, Theme.dim) }
                            }.frame(width: 160, alignment: .leading)
                            Text(String(k.key.prefix(9)) + String(repeating: "•", count: 12)).mono().frame(maxWidth: .infinity, alignment: .leading)
                            Button(copied == k.id ? "Copied" : "Copy") { m.copy(k.key); flash(k.id) }
                                .buttonStyle(.small).accessibilityLabel("Copy \(k.name)")
                            Button("Rotate") { confirmRotate = k }
                                .buttonStyle(.small).disabled(m.keyWork).accessibilityLabel("Rotate \(k.name)")
                            if keys.count > 1 {
                                Button("Revoke") { confirmRevoke = k }
                                    .buttonStyle(.small).disabled(m.keyWork).accessibilityLabel("Revoke \(k.name)")
                            }
                        }
                        .padding(.vertical, 10)
                        .background(m.flashKeyID == k.id ? Theme.signal.opacity(0.16) : .clear)
                        .animation(.easeOut(duration: 1.2), value: m.flashKeyID)
                        .overlay(alignment: .bottom) { Rectangle().fill(Theme.rule).frame(height: 1) }
                    }
                }
                .overlay(alignment: .top) { Rectangle().fill(Theme.rule).frame(height: 1) }
                .padding(.bottom, 14)
                HStack(spacing: 10) {
                    TextField("", text: $newKeyName, prompt: Text("Name it, e.g. Phone or Work laptop").foregroundColor(Theme.faint))
                        .textFieldStyle(.plain).font(Theme.mono(13)).foregroundStyle(Theme.ink)
                        .padding(.horizontal, 10).padding(.vertical, 6).overlay(Rectangle().stroke(Theme.faint, lineWidth: 1))
                        .onSubmit(addKey)
                        .disabled(m.keyWork)
                        .accessibilityLabel("New key name")
                    Button("New key", action: addKey).buttonStyle(.term).disabled(m.keyWork)
                }
                if m.keyWork { WorkingBar(height: 5).padding(.top, 10) }
                if !m.note.isEmpty { Text(m.note).mono(12, .regular, Theme.dim).padding(.top, 6) }

                SectionTitle("Network")
                Toggle(isOn: Binding(get: { c.lan }, set: { v in Task { await m.keyAction("lan", ["on": v]) } })) {
                    (Text("Let other devices on my network use it ") + Text("(phones, other computers)").foregroundColor(Theme.dim)).font(Theme.mono(13))
                }
                .toggleStyle(.checkbox).disabled(m.keyWork)
                if let lan = s.lanUrl, !lan.isEmpty { Fact(k: "LAN URL", v: lan).padding(.top, 8) }
            }
        }
        .confirmationDialog("Rotate \(confirmRotate?.name ?? "")?", isPresented: Binding(get: { confirmRotate != nil }, set: { if !$0 { confirmRotate = nil } })) {
            Button("Rotate") { if let k = confirmRotate { Task { await m.keyAction("keys/rotate", ["id": k.id], flash: k.id) } } }
        } message: { Text("Anything still using the old value stops working. Apps EZLM connected are updated for you.") }
        .confirmationDialog("Revoke \(confirmRevoke?.name ?? "")?", isPresented: Binding(get: { confirmRevoke != nil }, set: { if !$0 { confirmRevoke = nil } })) {
            Button("Revoke", role: .destructive) { if let k = confirmRevoke { Task { await m.keyAction("keys/revoke", ["id": k.id]) } } }
        } message: { Text(confirmRevoke.map { m.revokeMessage($0) } ?? "") }
    }

    private func addKey() {
        guard !m.keyWork else { return }
        let n = newKeyName.trimmingCharacters(in: .whitespaces)
        newKeyName = ""
        Task { await m.keyAction("keys/add", ["name": n]) }
    }

    private func flash(_ id: String) {
        copied = id
        DispatchQueue.main.asyncAfter(deadline: .now() + 1.4) { if copied == id { copied = nil } }
    }
}

// MARK: - Logs

struct LogsPane: View {
    @Environment(AppModel.self) private var m
    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            let header = PaneHeader(title: "Logs", subtitle: "What the model server is doing, updated live.")
            let actions = HStack(spacing: 8) {
                Button("Copy all") { m.copy(m.logs.joined(separator: "\n")) }.buttonStyle(.small)
                Button("Copy details for support") { Task { await m.copyDiagnostics() } }.buttonStyle(.small)
                Button("Report a problem") { m.openReport() }.buttonStyle(.small)
            }
            .fixedSize()
            // Buttons beside the title when there's room, else under it.
            ViewThatFits(in: .horizontal) {
                HStack(alignment: .firstTextBaseline) { header; Spacer(minLength: 16); actions }
                VStack(alignment: .leading, spacing: 0) { header; actions.padding(.top, -8).padding(.bottom, 22) }
            }
            if !m.note.isEmpty { Text(m.note).mono(12, .regular, Theme.dim).padding(.top, -12).padding(.bottom, 12) }
            LazyVStack(alignment: .leading, spacing: 2) {
                if m.logs.isEmpty { Text("No log output yet.").mono(12, .regular, Theme.dim) }
                ForEach(Array(m.logs.enumerated()), id: \.offset) { _, line in
                    Text(line).font(.system(size: 11, design: .monospaced))
                        .foregroundStyle(line.contains(" E ") || line.lowercased().contains("error") ? Theme.bad : line.contains("print_timing") ? Theme.ink : Theme.dim)
                        .textSelection(.enabled)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
            }
            .padding(14)
            .overlay(Rectangle().stroke(Theme.rule, lineWidth: 1))
        }
        .task { await m.refreshLogs() }
    }
}

/// Models already downloaded: switch instantly or remove to free disk.
struct InstalledSection: View {
    @Environment(AppModel.self) private var m
    @State private var confirmRemove: InstalledModel?

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            SectionTitle("On this computer")
            if m.installed.isEmpty {
                Text("Models you download stay here so you can switch back without downloading again.")
                    .mono(12, .regular, Theme.dim)
            }
            VStack(spacing: 0) {
                ForEach(m.installed) { im in
                    HStack(alignment: .center, spacing: 12) {
                        VendorMark(vendor: im.vendor, pixel: 2)
                        VStack(alignment: .leading, spacing: 3) {
                            Text(im.name).mono(13, .bold)
                            Text(detail(im)).mono(11, .regular, Theme.dim).lineLimit(1)
                        }
                        Spacer()
                        Origin(code: im.country)
                        if im.running {
                            Text("Running").mono(11, .bold, Theme.ok)
                                .padding(.horizontal, 8).padding(.vertical, 3)
                                .overlay(Rectangle().stroke(Theme.ok.opacity(0.5)))
                        } else {
                            Button("Switch") { Task { await m.use(im) } }
                                .buttonStyle(.small).disabled(im.plan == nil || m.installRunning)
                                .accessibilityLabel("Switch to \(im.name)")
                            Button("Remove") { confirmRemove = im }.buttonStyle(.small).accessibilityLabel("Remove \(im.name)")
                        }
                    }
                    .padding(.vertical, 10)
                    .overlay(alignment: .bottom) { Rectangle().fill(Theme.rule).frame(height: 1) }
                }
            }
            .overlay(alignment: .top) { Rectangle().fill(m.installed.isEmpty ? .clear : Theme.rule).frame(height: 1) }
        }
        .task { await m.refreshInstalled() }
        .confirmationDialog("Remove \(confirmRemove?.name ?? "")?", isPresented: Binding(get: { confirmRemove != nil }, set: { if !$0 { confirmRemove = nil } })) {
            Button("Remove", role: .destructive) { if let im = confirmRemove { Task { await m.remove(im) } } }
        } message: { Text("Frees \(gb(confirmRemove?.bytes ?? 0)). You can download it again any time.") }
    }

    private func detail(_ im: InstalledModel) -> String {
        var s = "\(im.quant), \(gb(im.bytes))"
        if let p = im.plan {
            s += ", ~\(p.words) words/s, \(p.ctx / 1024)k context"
        } else {
            s += ", doesn't fit in free memory right now"
        }
        return s
    }
}
