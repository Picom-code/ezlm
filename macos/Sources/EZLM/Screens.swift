import SwiftUI

struct FailedView: View {
    @Environment(AppModel.self) private var m
    let message: String
    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            Text("Something went wrong").mono(18, .bold)
            Text(message).mono(13, .regular, Theme.bad).textSelection(.enabled).fixedSize(horizontal: false, vertical: true)
            HStack(spacing: 12) {
                Button("Try again") { Task { await m.retryAfterFailure() } }.buttonStyle(.primary).keyboardShortcut(.defaultAction)
                Button("Report this problem") { m.openReport(message) }.buttonStyle(.term)
                Button("Copy details for support") { Task { await m.copyDiagnostics() } }.buttonStyle(.small)
            }
        }
        .frame(maxWidth: 560, alignment: .leading)
    }
}

// MARK: - Log line

struct LogLine: View {
    let state: StepState
    let label: String
    var sub: String = ""
    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 10) {
            Group {
                switch state {
                case .running: Spinner()
                case .ok: Text("✓").mono(13, .regular, Theme.ok)
                case .failed: Text("✗").mono(13, .regular, Theme.bad)
                case .waiting: Text("·").mono(13, .regular, Theme.dim)
                }
            }.frame(width: 14, alignment: .leading)
            VStack(alignment: .leading, spacing: 2) {
                Text(label).mono(13, .regular, state == .failed ? Theme.bad : state == .waiting ? Theme.dim : Theme.ink)
                if !sub.isEmpty { Text(sub).mono(12, .regular, Theme.dim).textSelection(.enabled).fixedSize(horizontal: false, vertical: true) }
            }
        }
        .padding(.vertical, 3)
    }
}

// MARK: - Scan

struct ScanView: View {
    @Environment(AppModel.self) private var m
    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            ForEach(Array(m.scanLines.enumerated()), id: \.element.id) { i, l in
                LogLine(state: l.state, label: l.label, sub: l.value).reveal(Double(i) * 0.09)
            }
        }
    }
}

// MARK: - Memory

struct MemoryRow: View {
    let label: String
    let total: UInt64
    let used: UInt64
    var need: UInt64? = nil
    @State private var grown = false

    var body: some View {
        let free = total > used ? total - used : 0
        let n = min(need ?? 0, free)
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Text(label).mono(13, .regular, Theme.dim)
                Spacer()
                if let need, need > 0 {
                    (Text(gb(n)).foregroundColor(Theme.ink) + Text(" for the model, \(gb(free - n)) left").foregroundColor(Theme.dim)).font(Theme.mono(12))
                } else if need != nil {
                    (Text(gb(free) + " free").foregroundColor(Theme.ink) + Text(" of \(gb(total)), not used").foregroundColor(Theme.dim)).font(Theme.mono(12))
                } else {
                    (Text(gb(free) + " free").foregroundColor(Theme.ink) + Text(" of \(gb(total))").foregroundColor(Theme.dim)).font(Theme.mono(12))
                }
            }
            BlockBar(others: grown ? Double(used) / Double(max(total, 1)) : 0,
                     model: grown ? Double(n) / Double(max(total, 1)) : 0, height: 18)
                .animation(.easeOut(duration: 0.55), value: grown)
                .animation(.easeOut(duration: 0.45), value: need)
            HStack(spacing: 14) {
                LegendDot(color: Theme.dim, text: "Apps \(gb(used))")
                if n > 0 { LegendDot(color: Theme.signal, text: "Model \(gb(n))") }
                LegendDot(color: Theme.rule, text: "Free \((need ?? 0) > 0 ? gb(free - n) : gb(free))")
            }
        }
        .padding(.bottom, 16)
        .onAppear { DispatchQueue.main.asyncAfter(deadline: .now() + 0.1) { grown = true } }
    }
}

struct LegendDot: View {
    let color: Color
    let text: String
    var body: some View {
        HStack(spacing: 6) { Rectangle().fill(color).frame(width: 9, height: 9); Text(text).mono(11, .regular, Theme.dim) }
    }
}

struct Fact: View {
    let k: String
    let v: String
    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 16) {
            Text(k).mono(13, .regular, Theme.dim).frame(width: 96, alignment: .leading)
            Text(v).mono().textSelection(.enabled).frame(maxWidth: .infinity, alignment: .leading)
        }
        .padding(.vertical, 1.5)
    }
}

func speedWord(_ p: Plan) -> String { ["fast": "Fast", "good": "Comfortable", "slow": "Slow"][p.speed] ?? "" }

// MARK: - Model options

/// Running / On this computer, Recommended / Slow, Needs space.
struct PlanTags: View {
    let plan: Plan
    let recommended: Bool
    let running: Bool
    var size: CGFloat = 10

    var body: some View {
        HStack(spacing: 8) {
            if running { Text("Running").mono(size, .bold, Theme.ok) }
            else if plan.isDownloaded { Text("On this computer").mono(size, .regular, Theme.ok) }
            if recommended { Text("Recommended").mono(size, .regular, Theme.signal) }
            else if plan.speed == "slow" { Text("Slow").mono(size, .regular, Theme.bad) }
            if plan.needsSpace { Text("Needs space").mono(size, .regular, Theme.gold) }
        }
        .fixedSize()
    }
}

/// A model option in Models. The selected card carries its own action button
/// ("Switch to X", "Download and switch"), double-click does the same, and
/// nothing here replays when the selection moves: the bars grow once.
struct OptionCard: View {
    let plan: Plan
    let hw: Hardware
    let maxScore: Double
    let recommended: Bool
    let running: Bool
    let selected: Bool
    /// The button on the selected card (label, enabled), or nil.
    var action: (label: String, enabled: Bool)?
    var onSelect: () -> Void = {}
    var onAction: () -> Void = {}
    @State private var grown = false

    var body: some View {
        let p = plan
        VStack(alignment: .leading, spacing: 4) {
            HStack(spacing: 9) {
                VendorMark(vendor: p.model.vendor, pixel: 1.9)
                Text(p.model.name).mono(13, .bold).lineLimit(1)
                Origin(code: p.model.country)
                Spacer(minLength: 8)
                PlanTags(plan: p, recommended: recommended, running: running, size: 11)
            }
            Text(how(p)).mono(11, .regular, Theme.dim)
            Text(p.ctxLine).mono(11, .regular, Theme.dim)
            Text(p.quantLine).mono(10, .regular, Theme.faint).padding(.bottom, 8)
            Grid(alignment: .leading, horizontalSpacing: 12, verticalSpacing: 7) {
                row("Smarts", BlockBar(grown ? p.model.score / maxScore : 0, color: Theme.ink, height: 8),
                    p.model.score == maxScore ? "smartest here" : "")
                row("Speed", BlockBar(grown ? min(1, log(1 + p.tokPerSec) / log(151)) : 0, color: p.speed == "slow" ? Theme.bad : Theme.ok, height: 8),
                    "~\(p.words) words/s")
                if hw.unified {
                    row("Memory", BlockBar(others: grown ? frac(hw.ramTotal - hw.ramAvail, hw.ramTotal) : 0, model: grown ? frac(p.gpuNeed, hw.ramTotal) : 0, height: 8),
                        "\(gb(p.gpuNeed)) of \(gb(hw.ramTotal))")
                } else {
                    if !(hw.gpus ?? []).isEmpty {
                        row("Graphics", BlockBar(others: grown ? frac(hw.gpuTotal - hw.gpuFree, hw.gpuTotal) : 0, model: grown ? frac(p.gpuNeed, hw.gpuTotal) : 0, height: 8),
                            p.gpuNeed > 0 ? "\(gb(p.gpuNeed)) of \(gb(hw.gpuTotal))" : "not used")
                    }
                    row("System", BlockBar(others: grown ? frac(hw.ramTotal - hw.ramAvail, hw.ramTotal) : 0, model: grown ? frac(p.ramNeed, hw.ramTotal) : 0, height: 8),
                        p.ramNeed > 0 ? "\(gb(p.ramNeed)) of \(gb(hw.ramTotal))" : "not used")
                }
            }
            .animation(.easeOut(duration: 0.6), value: grown)
            if let action {
                Button(action: onAction) {
                    HStack(spacing: 14) {
                        Text(action.label)
                        if action.enabled { Text("↩").foregroundStyle(Theme.dim) }
                    }
                }
                .buttonStyle(.primary)
                .keyboardShortcut(.defaultAction)
                .disabled(!action.enabled)
                .padding(.top, 10)
            }
        }
        .padding(14)
        .background(selected ? Theme.signal.opacity(0.06) : .clear)
        .overlay(Rectangle().stroke(selected ? Theme.signal : Theme.faint, lineWidth: 1))
        .contentShape(Rectangle())
        // Select on the first click, act on a double-click.
        .gesture(TapGesture(count: 2).onEnded { if action?.enabled == true { onAction() } })
        .simultaneousGesture(TapGesture().onEnded { onSelect() })
        .accessibilityElement(children: .contain)
        .accessibilityLabel(p.model.name)
        .accessibilityAddTraits(selected ? [.isButton, .isSelected] : .isButton)
        .accessibilityAction { onSelect() }
        .onAppear { DispatchQueue.main.asyncAfter(deadline: .now() + 0.15) { grown = true } }
    }

    private func frac(_ a: UInt64, _ b: UInt64) -> Double { Double(a) / Double(max(b, 1)) }

    @ViewBuilder private func row(_ k: String, _ bar: BlockBar, _ v: String) -> some View {
        GridRow {
            Text(k).mono(11, .regular, Theme.dim).frame(width: 64, alignment: .leading)
            bar
            Text(v).mono(11, .regular, Theme.dim).frame(width: 118, alignment: .trailing)
        }
    }

    private func how(_ p: Plan) -> String {
        let m = p.model
        let kind = m.moe ? "Mixture of experts, \(Int(m.paramsB.rounded()))B total, \(Int((m.activeParamsB ?? 0).rounded()))B used per word"
                         : "Dense, all \(Int(m.paramsB.rounded()))B used per word"
        let whereText: String
        switch p.mode {
        case "gpu": whereText = hw.unified ? "fits in shared memory" : "all on the graphics card"
        case "hybrid": whereText = "experts in system memory (\(p.nCpuMoe) of \(m.layers) layers), the rest on the graphics card"
        case "split": whereText = "\(p.ngl) of \(m.layers) layers on the graphics card, the rest on the processor"
        default: whereText = "runs on the processor"
        }
        return "\(kind); \(whereText)"
    }
}

struct ProcTable: View {
    @Environment(AppModel.self) private var m
    let hw: Hardware
    @State private var confirm: Proc?

    var body: some View {
        let list = (hw.gpuProcs?.isEmpty == false ? hw.gpuProcs : hw.topMem) ?? []
        if !list.isEmpty {
            let maxMem = list.map(\.mem).max() ?? 1
            SectionTitle(hw.gpuProcs?.isEmpty == false ? "Already using your graphics card" : "Using the most memory right now")
            VStack(spacing: 0) {
                ForEach(list) { p in
                    HStack(alignment: .center, spacing: 14) {
                        VStack(alignment: .leading, spacing: 5) {
                            Text(p.name).mono(13)
                            BlockBar(Double(p.mem) / Double(maxMem), color: Theme.dim, height: 5).frame(maxWidth: 240)
                        }
                        Spacer()
                        Text(gb(p.mem)).mono(12, .regular, Theme.dim)
                        if p.killable {
                            Button("Quit") { confirm = p }.buttonStyle(.small)
                        }
                    }
                    .padding(.vertical, 7)
                    .overlay(alignment: .bottom) { Rectangle().fill(Theme.rule).frame(height: 1) }
                }
            }
            .confirmationDialog("Quit \(confirm?.name ?? "")?", isPresented: Binding(get: { confirm != nil }, set: { if !$0 { confirm = nil } })) {
                Button("Quit", role: .destructive) { if let p = confirm { Task { await m.kill(p) } } }
            } message: { Text("Unsaved work in it may be lost.") }
        }
    }
}

// MARK: - Install / switch progress

/// The steps of an install or switch, the pour panel once the model loads,
/// and how it ended. Used by the wizard (step 3) and the switch sheet.
struct DeployView: View {
    @Environment(AppModel.self) private var m
    let inSheet: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            DeployHeader().padding(.bottom, 26)
            ForEach(m.steps) { s in StepRow(step: s).padding(.bottom, 6) }
            if let f = m.fill {
                PourPanel(fill: f).id(ObjectIdentifier(f)).padding(.top, 14)
                    .transition(.opacity.combined(with: .move(edge: .top)))
            }
            DeployFooter(inSheet: inSheet)
        }
        .animation(.easeOut(duration: 0.3), value: m.fill.map(ObjectIdentifier.init))
    }
}

private struct DeployHeader: View {
    @Environment(AppModel.self) private var m
    var body: some View {
        let overall = m.overall
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text(m.installTitle).mono(13, .bold).lineLimit(1)
                Spacer()
                Text("\(Int(overall * 100))%").mono(13, .regular, m.deployError != nil ? Theme.bad : Theme.signal).monospacedDigit()
            }
            BlockBar(overall, color: m.deployError != nil ? Theme.bad : overall >= 1 ? Theme.ok : Theme.signal, height: 20,
                     sheen: m.deployError == nil && overall < 1)
                .animation(.easeOut(duration: 0.45), value: overall)
            HStack {
                Text(m.overallStatus).mono(11, .regular, Theme.dim)
                Spacer()
                Text(m.overallETA).mono(11, .regular, Theme.dim).monospacedDigit()
            }
        }
    }
}

private struct StepRow: View {
    let step: DeployStep
    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            LogLine(state: step.state, label: step.label, sub: step.sub)
            if step.showBar {
                Group {
                    if step.indeterminate { WorkingBar(height: 5) }
                    else { BlockBar(step.fraction, height: 5, sheen: true).animation(.easeOut(duration: 0.4), value: step.fraction) }
                }
                .frame(maxWidth: 420).padding(.leading, 24)
            }
            if !step.stat.isEmpty { Text(step.stat).mono(11, .regular, Theme.dim).monospacedDigit().padding(.leading, 24) }
        }
    }
}

private struct DeployFooter: View {
    @Environment(AppModel.self) private var m
    let inSheet: Bool

    var body: some View {
        if m.installFinished {
            VStack(alignment: .leading, spacing: 18) {
                (Text("✓ ").foregroundColor(Theme.ok) + Text(m.installDoneText).foregroundColor(Theme.ink)).font(Theme.mono(13))
                Button { m.closeInstallSheet() } label: { HStack(spacing: 14) { Text("Done"); Text("↩").foregroundStyle(Theme.dim) } }
                    .buttonStyle(.primary).keyboardShortcut(.defaultAction)
            }
            .padding(.top, 18)
        } else if let err = m.deployError {
            VStack(alignment: .leading, spacing: 10) {
                Text(err).mono(13, .regular, Theme.bad).textSelection(.enabled).fixedSize(horizontal: false, vertical: true)
                if !m.deployHint.isEmpty { Text(m.deployHint).mono(12, .regular, Theme.dim).fixedSize(horizontal: false, vertical: true) }
                HStack(spacing: 10) {
                    Button(inSheet ? "Close" : "Back to the start") {
                        if inSheet { m.closeInstallSheet() } else { Task { await m.runScan() } }
                    }
                    .buttonStyle(.primary).keyboardShortcut(.defaultAction)
                    Button("Report this problem") { m.openReport("\(m.installTitle): \(err)") }.buttonStyle(.term)
                    Button("Copy details for support") { Task { await m.copyDiagnostics() } }.buttonStyle(.small)
                }
                .padding(.top, 4)
            }
            .padding(.top, 14)
        } else if m.installRunning {
            HStack(spacing: 12) {
                Button(m.cancelling ? "Cancelling…" : "Cancel") { Task { await m.cancelInstall() } }
                    .buttonStyle(.term)
                    .keyboardShortcut(.cancelAction)
                    .disabled(m.cancelling)
                Text("esc").mono(11, .regular, Theme.faint)
            }
            .padding(.top, 18)
        }
    }
}

// MARK: - Connect

struct AppTile: View {
    @Environment(AppModel.self) private var m
    let target: Target

    var body: some View {
        let st = m.tileStatus(target)
        let state = target.current
        let trade = target.id == "hermes" ? m.hermesTradeoff() : nil
        let blocked = trade?.blocked == true || state == Target.State.needsTools
        let selectable = target.supported && !blocked
        let selected = m.selectedApps.contains(target.id)
        let settled = !st.working && m.appState[target.id] == nil
        let needs64 = settled && target.supported && state == Target.State.needs64K
        let broken = settled && target.supported && target.broken
        VStack(alignment: .leading, spacing: 8) {
            HStack(alignment: .center, spacing: 10) {
                AppLogo(target: target, size: 36, fallback: true)
                Text(target.name).mono(13, .bold).fixedSize(horizontal: false, vertical: true)
            }
            .padding(.bottom, target.versionLine == nil ? 4 : 0)
            // Its own line: "0.2.1-rc.5 · installed by EZLM" doesn't fit beside the logo.
            if let v = target.versionLine { Text(v).mono(11, .regular, Theme.dim).padding(.bottom, 4) }
            Text(target.desc ?? "").mono(11, .regular, Theme.dim).fixedSize(horizontal: false, vertical: true)
            Text(st.text).mono(11, .regular, st.color).fixedSize(horizontal: false, vertical: true)
            if settled, let checked = target.checkedLine() {
                Text(checked).mono(11, .regular, Theme.dim).fixedSize(horizontal: false, vertical: true)
            }
            // The 64K sentence already says what Connect does; the tradeoff adds the cost, if there is one.
            if let trade, !st.working, state != Target.State.needs64K || trade.warn {
                Text(trade.text).mono(11, .regular, trade.warn ? Theme.gold : Theme.dim).fixedSize(horizontal: false, vertical: true)
            }
            if needs64 || broken {
                FlowRow(spacing: 8) {
                    if needs64 {
                        // What the 64K sentence says: connect again, or a smaller model when this one can't have 64K.
                        if trade?.blocked == true {
                            Button("Pick a smaller model") { m.go(.models) }.buttonStyle(.small)
                        } else {
                            Button("Connect again") { Task { await m.reconnect(target.id) } }
                                .buttonStyle(.small).disabled(m.appBusy || m.serverState != .live)
                        }
                    }
                    if broken {
                        Button("Show details") { Task { await m.showAppLog(target) } }.buttonStyle(.small)
                        Button("Report a problem") { Task { await m.reportApp(target, reason: st.text) } }.buttonStyle(.small)
                    }
                }
                .padding(.top, 2)
            }
            Spacer(minLength: 4)
            if st.working { WorkingBar(height: 5) } else { Color.clear.frame(height: 5) }
        }
        .padding(16)
        .frame(maxWidth: .infinity, minHeight: 170, maxHeight: .infinity, alignment: .topLeading)
        .background(selected ? Theme.signal.opacity(0.06) : .clear)
        .overlay(Rectangle().stroke(selected ? Theme.signal : Theme.faint, lineWidth: 1))
        .contentShape(Rectangle())
        .onTapGesture { if selectable { m.toggleApp(target.id) } }
        .opacity(!target.supported ? 0.5 : blocked ? 0.6 : 1)
        .accessibilityElement(children: .combine)
        .accessibilityAddTraits(selected ? [.isButton, .isSelected] : .isButton)
        .accessibilityAction { if selectable { m.toggleApp(target.id) } }
        .accessibilityAction(named: "Show details") { if broken { Task { await m.showAppLog(target) } } }
    }
}

/// Tiles side by side, as many to a row as fit at `minimum` wide (three on a
/// normal window), every tile in a row as tall as the tallest.
struct TileGrid: Layout {
    var minimum: CGFloat = 220
    var spacing: CGFloat = 12

    private func columns(_ width: CGFloat, _ count: Int) -> (n: Int, width: CGFloat) {
        let n = max(1, min(count, Int((width + spacing) / (minimum + spacing))))
        return (n, max(0, (width - spacing * CGFloat(n - 1)) / CGFloat(n)))
    }

    private func rowHeights(_ col: (n: Int, width: CGFloat), _ subviews: Subviews) -> [CGFloat] {
        stride(from: 0, to: subviews.count, by: col.n).map { start in
            subviews[start..<min(start + col.n, subviews.count)]
                .map { $0.sizeThatFits(ProposedViewSize(width: col.width, height: nil)).height }.max() ?? 0
        }
    }

    func sizeThatFits(proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) -> CGSize {
        let width = proposal.width ?? (minimum * CGFloat(subviews.count) + spacing * CGFloat(max(subviews.count - 1, 0)))
        let heights = rowHeights(columns(width, subviews.count), subviews)
        return CGSize(width: width, height: heights.reduce(0, +) + spacing * CGFloat(max(heights.count - 1, 0)))
    }

    func placeSubviews(in bounds: CGRect, proposal: ProposedViewSize, subviews: Subviews, cache: inout ()) {
        let col = columns(bounds.width, subviews.count)
        let heights = rowHeights(col, subviews)
        var y = bounds.minY
        for (r, h) in heights.enumerated() {
            for c in 0..<col.n {
                let i = r * col.n + c
                guard i < subviews.count else { break }
                let x = bounds.minX + CGFloat(c) * (col.width + spacing)
                subviews[i].place(at: CGPoint(x: x, y: y), proposal: ProposedViewSize(width: col.width, height: h))
            }
            y += h + spacing
        }
    }
}

/// Overview's "Connect with" line: each connected app with its logo.
struct ConnectedApps: View {
    let targets: [Target]

    var body: some View {
        let on = targets.filter { $0.current == Target.State.connected }
        if on.isEmpty {
            Text("Not connected to any apps yet.").mono(12, .regular, Theme.dim)
        } else {
            HStack(spacing: 14) {
                Text("Connected to").mono(12, .regular, Theme.dim)
                ForEach(on) { t in
                    HStack(spacing: 6) {
                        AppLogo(target: t, size: 20)
                        Text(t.name).mono(12)
                    }
                }
            }
            .accessibilityElement(children: .ignore)
            .accessibilityLabel("Connected to \(on.map(\.name).joined(separator: " and ")).")
        }
    }
}
