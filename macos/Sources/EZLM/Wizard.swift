import SwiftUI

// MARK: - Root

struct RootView: View {
    @Environment(AppModel.self) private var m
    var body: some View {
        Group {
            if m.setupDone { MainView().transition(.opacity) }
            else { SetupWizard().transition(.opacity) }
        }
        .background(Theme.black)
        .overlay(alignment: .bottom) { ToastHost() }
        .modifier(MakeRoomAlert())
        .modifier(ModelModeHost())
        .modifier(ReportHost())
        .background(CloseGuard(blocked: m.installRunning || m.storageBusy || m.updateBusy))
    }
}

/// Greys out the window's close button (and so Cmd-W) while EZLM is in the
/// middle of something closing would stop, the way an installer does. Closing
/// the last window quits, and quitting asks first (see AppDelegate).
private struct CloseGuard: NSViewRepresentable {
    let blocked: Bool
    func makeNSView(context: Context) -> NSView { NSView() }
    func updateNSView(_ v: NSView, context: Context) {
        let blocked = blocked
        DispatchQueue.main.async { v.window?.standardWindowButton(.closeButton)?.isEnabled = !blocked }
    }
}

/// Model mode opens from the wizard and from the main app, so it lives here.
private struct ModelModeHost: ViewModifier {
    @Environment(AppModel.self) private var m
    func body(content: Content) -> some View {
        content.sheet(item: Binding(get: { m.modelMode }, set: { if $0 == nil { m.endModelMode() } })) { mm in
            ModelModeSheet(mm: mm).environment(m)
        }
    }
}

/// The toast lives above everything at the bottom of the window.
private struct ToastHost: View {
    @Environment(AppModel.self) private var m
    var body: some View {
        ZStack {
            if let t = m.toast { ToastView(toast: t).id(t.id).transition(.opacity) }
        }
        .padding(.horizontal, 24)
        .padding(.bottom, m.setupDone ? 18 : 40)
    }
}

/// "Make room for this model?": shown before a download that needs disk space
/// EZLM would have to free by removing models. Nothing is deleted without it.
private struct MakeRoomAlert: ViewModifier {
    @Environment(AppModel.self) private var m
    func body(content: Content) -> some View {
        content.alert("Make room for this model?",
                      isPresented: Binding(get: { m.makeRoom != nil }, set: { if !$0 { m.makeRoom = nil } }),
                      presenting: m.makeRoom) { mr in
            Button("Make room and continue", role: .destructive) { Task { await m.deploy(mr.plan) } }
            Button("Cancel", role: .cancel) {}.keyboardShortcut(.defaultAction) // as on Windows: Return keeps things as they are
        } message: { mr in
            Text(mr.message)
        }
    }
}

// MARK: - Setup wizard (first run): one screen per step, no scrolling

struct SetupWizard: View {
    @Environment(AppModel.self) private var m

    private var step: Int {
        switch m.screen {
        case .booting, .scanning, .failed: return 0
        case .pick: return 1
        case .deploying: return 2
        case .done: return 3
        }
    }

    var body: some View {
        VStack(spacing: 0) {
            StepHeader(current: step).padding(.top, 38).padding(.horizontal, 36)
            Rectangle().fill(Theme.rule).frame(height: 1).padding(.top, 18)
            Group {
                switch m.screen {
                case .booting, .scanning: ScanStep()
                case .pick: ChooseStep()
                case .deploying: InstallStep()
                case .done: ReadyStep()
                case .failed(let msg): FailedView(message: msg).frame(maxWidth: .infinity, maxHeight: .infinity)
                }
            }
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .padding(.horizontal, 36)
            if AppPlace.temporary { MoveToApplicationsNote().padding(.horizontal, 36).padding(.bottom, 10) }
            Text(Engine.demo ? "Demo: nothing is installed. Data is from an RTX 4070 SUPER desktop."
                             : "Everything runs on this computer. Nothing you type is sent anywhere else.")
                .mono(11, .regular, Theme.faint).padding(.bottom, 14)
        }
    }
}

/// "1 Scan  2 Choose  3 Install  4 Ready": a real sequence, so numbered.
struct StepHeader: View {
    let current: Int
    private let names = ["Scan", "Choose", "Install", "Ready"]
    var body: some View {
        HStack(spacing: 0) {
            MiniLogo()
            Spacer()
            HStack(spacing: 22) {
                ForEach(names.indices, id: \.self) { i in
                    HStack(spacing: 8) {
                        Text(i < current ? "✓" : "\(i + 1)")
                            .mono(11, .bold, i < current ? Theme.ok : i == current ? Theme.black : Theme.dim)
                            .frame(width: 20, height: 20)
                            .background(i == current ? Theme.ink : .clear)
                            .overlay(Rectangle().stroke(i == current ? Theme.ink : i < current ? Theme.ok.opacity(0.5) : Theme.faint, lineWidth: 1))
                        Text(names[i]).mono(12, i == current ? .bold : .regular, i == current ? Theme.ink : Theme.dim)
                    }
                    if i < names.count - 1 {
                        Rectangle().fill(i < current ? Theme.ok.opacity(0.5) : Theme.rule).frame(width: 26, height: 1)
                    }
                }
            }
            .animation(.easeOut(duration: 0.3), value: current)
        }
    }
}

/// Compact two-tone wordmark for headers and the sidebar.
struct MiniLogo: View {
    var body: some View {
        (Text("EZ").foregroundColor(Theme.dim) + Text("LM").foregroundColor(Theme.ink))
            .font(.system(size: 18, weight: .heavy, design: .monospaced))
            .tracking(-0.5)
    }
}

struct ScanStep: View {
    @Environment(AppModel.self) private var m
    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Spacer()
            Logo(state: m.logo)
            Text("Your own AI model, running on this computer.").mono(13, .regular, Theme.dim).padding(.top, 16).padding(.bottom, 28)
            ScanView().frame(height: 210, alignment: .top)
            Spacer()
        }
        .frame(maxWidth: 560)
    }
}

struct ChooseStep: View {
    @Environment(AppModel.self) private var m

    var body: some View {
        if let r = m.scan {
            let plans = m.visiblePlans
            HStack(alignment: .top, spacing: 32) {
                // Left: the pick and where it lands in memory.
                VStack(alignment: .leading, spacing: 0) {
                    if let p = m.chosen {
                        Text("Your \(r.hw.machineWord) can run").mono(13, .regular, Theme.dim)
                        HStack(spacing: 12) {
                            VendorMark(vendor: p.model.vendor, pixel: 3)
                            // Types only when the pick changes, never on a rebuild.
                            Typewriter(text: p.model.name, speed: 0.03, key: "choose").mono(22, .bold).lineLimit(2)
                        }
                        .padding(.top, 6)
                        Origin(code: p.model.country).padding(.top, 6).padding(.bottom, 10)
                        Fact(k: "File", v: p.quantLine)
                        Fact(k: "Download", v: p.downloadText)
                        Fact(k: "Speed", v: "\(speedWord(p)), about \(p.words) words/s")
                        Fact(k: "Context", v: "\(p.ctx / 1024)k tokens")
                        Fact(k: "Tools", v: p.model.toolCalling ? "Works with coding agents" : "Chat only")
                        if p.needsSpace {
                            (Text("Needs about \(gb(p.downloadBytes + 2 << 30)) of disk, \(gb(r.hw.diskFree)) is free. ").foregroundColor(Theme.gold) +
                             Text("You can still set it up; make room before the download finishes.").foregroundColor(Theme.dim))
                                .font(Theme.mono(11)).padding(.top, 8)
                        }
                        Spacer().frame(height: 14)
                        MemoryPair(hw: r.hw, plan: p)
                    } else {
                        Text("No model fits in the memory that's free right now.").mono(15, .bold, Theme.bad)
                        Text(MemoryUsers.noFitHint(r.hw)).mono(12, .regular, Theme.dim).padding(.top, 6)
                    }
                    if let u = r.unlock {
                        (Text(r.unlockGpu == true ? "Your graphics card is busy. " : "") + Text("Free about \(Int((r.unlockFreeGb ?? 0).rounded(.up))) GB to run ") +
                         Text(u.model.name).foregroundColor(Theme.ink) + Text(" at ~\(u.words) words/s."))
                            .font(Theme.mono(12)).foregroundStyle(Theme.dim).padding(.top, 4)
                    }
                    Spacer()
                    // One row when it fits; otherwise the other two buttons go below.
                    ViewThatFits(in: .horizontal) {
                        HStack(spacing: 12) { setUpButton; scanAgainButton; freeUpButton }
                        VStack(alignment: .leading, spacing: 10) { setUpButton; HStack(spacing: 12) { scanAgainButton; freeUpButton } }
                    }
                    if !m.note.isEmpty { Text(m.note).mono(12, .regular, Theme.bad).padding(.top, 8).fixedSize(horizontal: false, vertical: true) }
                }
                .frame(width: 380, alignment: .leading)
                .padding(.vertical, 26)

                Rectangle().fill(Theme.rule).frame(width: 1)

                // Right: the other ways to run a model, plus what's in the way.
                VStack(alignment: .leading, spacing: 10) {
                    PrefPicker().padding(.bottom, 6)
                    Text("Other ways to run a model here  ↑↓ to choose, ↩ to set up").mono(13, .regular, Theme.dim)
                    let maxScore = plans.map(\.model.score).max() ?? 1
                    let running = m.currentModelID
                    ScrollViewReader { proxy in
                        ScrollView {
                            VStack(spacing: 8) {
                                ForEach(plans) { p in
                                    OptionRow(plan: p, maxScore: maxScore, recommended: p.id == r.recommended?.id,
                                              running: p.id == running, selected: p.id == m.chosenID)
                                        .id(p.id)
                                        .onTapGesture { m.chosenID = p.id }
                                }
                            }
                            .padding(1)
                        }
                        .onChange(of: m.chosenID) { _, id in withAnimation(.easeOut(duration: 0.2)) { proxy.scrollTo(id, anchor: .center) } }
                    }
                    if let procs = MemoryUsers.rows(r.hw) {
                        Text(MemoryUsers.title(r.hw)).mono(13, .regular, Theme.dim).padding(.top, 8)
                        ForEach(procs.prefix(2)) { p in ProcRow(proc: p) }
                    }
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.vertical, 26)
            }
            .frame(maxHeight: .infinity)
            .arrowSelection(plans: plans)
        }
    }
}

extension ChooseStep {
    var setUpButton: some View {
        Button { m.requestDeploy() } label: { HStack(spacing: 14) { Text(m.setUpLabel); Text("↩").foregroundStyle(Theme.dim) } }
            .buttonStyle(.primary).keyboardShortcut(.defaultAction).disabled(m.chosen == nil || m.installRunning)
            .fixedSize()
    }
    var scanAgainButton: some View {
        Button("Scan again") { Task { await m.runScan() } }.buttonStyle(.term).fixedSize()
    }
    var freeUpButton: some View {
        Button("Free up memory") { m.startModelMode(.wizard) }.buttonStyle(.term).fixedSize()
    }
}

/// Graphics + system memory (or one shared bar on a Mac) with the plan's footprint.
struct MemoryPair: View {
    let hw: Hardware
    let plan: Plan?
    var body: some View {
        if hw.unified {
            MemoryRow(label: "Memory, shared with graphics", total: hw.ramTotal, used: hw.ramTotal - hw.ramAvail, need: plan?.gpuNeed)
        } else {
            if !(hw.gpus ?? []).isEmpty {
                MemoryRow(label: "Graphics memory", total: hw.gpuTotal, used: hw.gpuTotal - hw.gpuFree, need: plan.map { $0.mode == "cpu" ? 0 : $0.gpuNeed })
            }
            MemoryRow(label: "System memory", total: hw.ramTotal, used: hw.ramTotal - hw.ramAvail, need: plan?.ramNeed)
        }
    }
}

/// Compact comparison row for the wizard's right column.
struct OptionRow: View {
    let plan: Plan
    let maxScore: Double
    let recommended: Bool
    var running = false
    let selected: Bool
    var body: some View {
        let p = plan
        VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 9) {
                VendorMark(vendor: p.model.vendor, pixel: 1.9)
                Text(p.model.name).mono(12, .bold).lineLimit(1)
                Origin(code: p.model.country)
                Spacer(minLength: 8)
                PlanTags(plan: p, recommended: recommended, running: running)
            }
            Text(where_(p)).mono(10, .regular, Theme.dim).lineLimit(1)
            Text(p.ctxLine).mono(10, .regular, Theme.dim).lineLimit(1)
            Text(p.quantLine).mono(10, .regular, Theme.faint).lineLimit(1)
            HStack(spacing: 10) {
                Text("Smarts").mono(10, .regular, Theme.dim).frame(width: 44, alignment: .leading)
                BlockBar(p.model.score / maxScore, color: Theme.ink, height: 6)
                Text("Speed").mono(10, .regular, Theme.dim).frame(width: 40, alignment: .leading)
                BlockBar(min(1, log(1 + p.tokPerSec) / log(151)), color: p.speed == "slow" ? Theme.bad : Theme.ok, height: 6)
            }
        }
        .padding(10)
        .background(selected ? Theme.signal.opacity(0.07) : .clear)
        .overlay(Rectangle().stroke(selected ? Theme.signal : Theme.faint, lineWidth: 1))
        .contentShape(Rectangle())
        .accessibilityElement(children: .combine)
        .accessibilityAddTraits(selected ? [.isButton, .isSelected] : .isButton)
    }

    private func where_(_ p: Plan) -> String {
        let kind = p.model.moe ? "MoE \(Int(p.model.paramsB.rounded()))B" : "Dense \(Int(p.model.paramsB.rounded()))B"
        switch p.mode {
        case "gpu": return "\(kind), all on the graphics chip, ~\(p.words) words/s"
        case "hybrid": return "\(kind), experts in system memory, ~\(p.words) words/s"
        case "split": return "\(kind), split GPU + processor, ~\(p.words) words/s"
        default: return "\(kind), on the processor, ~\(p.words) words/s"
        }
    }
}

/// The processes the wizard lists beside the pick, and the words above them.
/// A Mac has one memory pool, so the engine sends the biggest memory users
/// (top_mem) and no graphics-card list; other machines send the processes
/// holding graphics memory.
enum MemoryUsers {
    /// Rows to show, or nil when there is nothing to list.
    static func rows(_ hw: Hardware) -> [Proc]? {
        if let gpu = hw.gpuProcs, !gpu.isEmpty { return gpu }
        if let top = hw.topMem, !top.isEmpty { return top }
        return nil
    }

    static func title(_ hw: Hardware) -> String {
        if let gpu = hw.gpuProcs, !gpu.isEmpty { return "Using your graphics card" }
        return "Using the most memory right now"
    }

    /// The line under "no model fits": only point at the list when one is shown.
    static func noFitHint(_ hw: Hardware) -> String {
        rows(hw) == nil ? "Close a few apps, then scan again." : "Quit an app on the right, then scan again."
    }
}

struct ProcRow: View {
    @Environment(AppModel.self) private var m
    let proc: Proc
    @State private var confirm = false
    var body: some View {
        HStack(spacing: 10) {
            Text(proc.name).mono(12)
            Spacer()
            Text(gb(proc.mem)).mono(11, .regular, Theme.dim)
            if proc.killable { Button("Quit") { confirm = true }.buttonStyle(.small) }
        }
        .confirmationDialog("Quit \(proc.name)?", isPresented: $confirm) {
            Button("Quit", role: .destructive) { Task { await m.kill(proc) } }
        } message: { Text("Only this app is closed. Unsaved work in it may be lost.") }
    }
}

struct InstallStep: View {
    var body: some View {
        VStack { Spacer(); DeployView(inSheet: false).frame(maxWidth: 560); Spacer() }
    }
}

struct ReadyStep: View {
    @Environment(AppModel.self) private var m
    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Spacer()
            HStack(spacing: 12) {
                Pulse()
                Text("Your AI is live").mono(24, .bold)
            }
            .padding(.bottom, 8)
            Text("Use this URL and key in any app that supports the OpenAI API.").mono(12, .regular, Theme.dim).padding(.bottom, 20)
            if let s = m.status, let c = s.config {
                EndpointBlock(url: s.localUrl ?? "", key: c.key, animate: true)
            }
            if !m.reply.isEmpty {
                Text("It says:").mono(12, .regular, Theme.dim).padding(.top, 20).padding(.bottom, 6)
                Typewriter(text: m.reply, speed: 0.012, key: "reply").mono(13).padding(.leading, 14)
                    .overlay(alignment: .leading) { Rectangle().fill(Theme.faint).frame(width: 2) }
            }
            if let c = m.status?.config {
                Text("Start using it").mono(13, .bold).padding(.top, 26).padding(.bottom, 4)
                Text("Each button opens your model in that app, setting the app up first if it isn't yet.")
                    .mono(12, .regular, Theme.dim).fixedSize(horizontal: false, vertical: true).padding(.bottom, 10)
                LaunchButtons(config: c)
            }
            HStack(spacing: 12) {
                Button { m.finishSetup() } label: { HStack(spacing: 14) { Text("Open EZLM"); Text("↩").foregroundStyle(Theme.dim) } }
                    .buttonStyle(.primary).keyboardShortcut(.defaultAction)
            }
            .padding(.top, 20)
            Spacer()
        }
        .frame(maxWidth: 560, alignment: .leading)
    }
}

/// URL + main key with copy buttons.
struct EndpointBlock: View {
    @Environment(AppModel.self) private var m
    let url: String
    let key: String
    var animate = false
    @State private var copied: String?

    var body: some View {
        VStack(spacing: 0) {
            row("URL", url, url)
            row("Key", String(key.prefix(9)) + String(repeating: "•", count: 12), key)
        }
        .overlay(alignment: .top) { Rectangle().fill(Theme.rule).frame(height: 1) }
    }

    @ViewBuilder private func row(_ k: String, _ shown: String, _ value: String) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: 16) {
            Text(k).mono(13, .regular, Theme.dim).frame(width: 36, alignment: .leading)
            Group { if animate { Typewriter(text: shown, speed: 0.016, key: "endpoint-" + k) } else { Text(shown) } }
                .mono().textSelection(.enabled).frame(maxWidth: .infinity, alignment: .leading)
            Button(copied == k ? "Copied" : "Copy") {
                m.copy(value)
                copied = k
                DispatchQueue.main.asyncAfter(deadline: .now() + 1.4) { if copied == k { copied = nil } }
            }
            .buttonStyle(.small)
            .accessibilityLabel(copied == k ? "Copied" : "Copy \(k)")
        }
        .padding(.vertical, 10)
        .overlay(alignment: .bottom) { Rectangle().fill(Theme.rule).frame(height: 1) }
    }
}

// MARK: - Keyboard

/// ↑/↓ move the selection through plans; ↩ is handled by the primary button's
/// default-action shortcut (the wizard's "Set it up", the selected card's button).
struct ArrowSelection: ViewModifier {
    @Environment(AppModel.self) private var m
    let plans: [Plan]
    @FocusState private var focused: Bool

    func body(content: Content) -> some View {
        content
            .focusable()
            .focused($focused)
            .focusEffectDisabled()
            .onAppear { focused = true }
            // Clicking anywhere in the list takes the arrow keys back (from the sidebar, say).
            .simultaneousGesture(TapGesture().onEnded { focused = true })
            .onKeyPress(.downArrow) { move(1); return .handled }
            .onKeyPress(.upArrow) { move(-1); return .handled }
    }

    private func move(_ d: Int) {
        guard !plans.isEmpty else { return }
        let i = plans.firstIndex { $0.id == m.chosenID } ?? -d
        let next = plans[max(0, min(plans.count - 1, i + d))]
        guard next.id != m.chosenID else { return }
        withAnimation(.easeOut(duration: 0.15)) { m.chosenID = next.id }
    }
}

extension View {
    func arrowSelection(plans: [Plan]) -> some View { modifier(ArrowSelection(plans: plans)) }
}

/// Balanced / Smartest / Fastest: what the user values when models trade off.
struct PrefPicker: View {
    @Environment(AppModel.self) private var m
    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 0) {
                segment(.balanced)
                segment(.smartest)
                segment(.fastest)
            }
            .overlay(Rectangle().stroke(Theme.faint, lineWidth: 1))
            Text(m.prefBusy ? "Comparing…" : m.pref.blurb).mono(11, .regular, Theme.dim)
        }
    }

    private func segment(_ p: Pref) -> some View {
        let on = p == m.pref
        return Text(p.title)
            .mono(12, on ? .bold : .regular, on ? Theme.black : Theme.dim)
            .padding(.horizontal, 14).padding(.vertical, 5)
            .background(on ? Theme.ink : Color.clear)
            .contentShape(Rectangle())
            .onTapGesture { Task { await m.setPref(p) } }
            .accessibilityAddTraits(on ? [.isButton, .isSelected] : .isButton)
            .accessibilityAction { Task { await m.setPref(p) } }
    }
}
