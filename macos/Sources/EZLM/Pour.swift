import SwiftUI
import Observation

// MARK: - The numbers

/// "Pouring the model into memory": what the panel shows while a model loads.
///
/// It's driven by real numbers: memory used (graphics memory from the driver,
/// or the Mac's shared memory) as the server loads the model, plus how much of
/// the file the engine has read in ahead of time (the pre-read). Each bar's
/// outline is the planned size. The lowest usage seen since loading began
/// counts as "other apps", which also handles a switch where the previous
/// model is unloaded first.
@MainActor
@Observable
final class MemoryFill {
    struct Row: Identifiable, Equatable {
        let id: String
        let label: String
        var total: UInt64 = 0
        var need: UInt64 = 0
        /// The model's share so far; it only grows.
        var model: UInt64 = 0
        /// Where the model ends up: the planned size (or what was measured).
        var target: UInt64 = 0
        /// Lowest "used" seen since loading began: everything else on the machine.
        var base: UInt64?

        var appsFraction: Double { fraction(base ?? 0) }
        var modelFraction: Double { fraction(model) }
        var targetFraction: Double { fraction(target) }
        private func fraction(_ v: UInt64) -> Double { Double(v) / Double(max(total, 1)) }

        /// e.g. ("4.1 GB", " of 9.5 GB") or ("9.5 GB", " loaded")
        func numbers(done: Bool) -> (String, String) {
            if done { return (gb(target), " loaded") }
            if need > 0 { return (gb(model), " of \(gb(need))") }
            return (gb(model), "")
        }
    }

    let name: String
    private(set) var rows: [Row] = []
    private(set) var done = false
    private(set) var percent: Int?
    /// Fraction of the model file read into memory so far (the pre-read).
    @ObservationIgnored private var warmth = 0.0

    init(name: String) { self.name = name }

    var title: String { done ? "Loaded \(name)" : "Pouring \(name) into memory" }

    /// The pre-read: how much of the model file is in memory now.
    func warm(done d: UInt64, total t: UInt64) {
        if t > 0 { warmth = max(warmth, min(1, Double(d) / Double(t))) }
    }

    /// One /api/live sample. Planned sizes come from the chosen setup, or from
    /// the running setup's plan the engine reports.
    func feed(_ l: Live, gpuNeed: UInt64 = 0, ramNeed: UInt64 = 0) {
        if l.loadTotal > 0 { warm(done: l.loadDone, total: l.loadTotal) }
        let g = gpuNeed > 0 ? gpuNeed : l.gpuPlan
        let r = ramNeed > 0 ? ramNeed : l.ramPlan
        if l.unified {
            sample("mem", "Memory, shared with graphics", total: l.ramTotal, need: g + r,
                   used: l.ramTotal - min(l.ramAvail, l.ramTotal), warm: warmth)
        } else {
            if l.gpuTotal > 0 {
                sample("gpu", "Graphics memory", total: l.gpuTotal, need: g,
                       used: l.gpuTotal - min(l.gpuFree, l.gpuTotal), warm: 0)
            }
            // System memory matters when part of the model lives there (or there's no graphics card).
            if r >= 1 << 30 || l.gpuTotal == 0 || rows.contains(where: { $0.id == "ram" }) {
                sample("ram", "System memory", total: l.ramTotal, need: r,
                       used: l.ramTotal - min(l.ramAvail, l.ramTotal), warm: warmth)
            }
        }
        if done {
            percent = 100
        } else {
            let loaded = rows.reduce(UInt64(0)) { $0 + $1.model }, need = rows.reduce(UInt64(0)) { $0 + $1.need }
            percent = need > 0 ? min(100, Int(100 * Double(loaded) / Double(need))) : nil
        }
    }

    /// Loading finished: fill to the planned size (the bars flash and settle).
    func complete(_ last: Live?) {
        guard !done else { return }
        done = true
        if let last {
            feed(last)
        } else {
            for i in rows.indices { rows[i].model = rows[i].target }
        }
        percent = 100
    }

    private func sample(_ id: String, _ label: String, total: UInt64, need: UInt64, used: UInt64, warm: Double) {
        var row = rows.first { $0.id == id } ?? Row(id: id, label: label)
        row.total = total
        if need > 0 || row.need == 0 { row.need = need } // a later sample without a plan keeps the plan
        let base = min(row.base ?? used, used)
        row.base = base
        let measured = used > base ? used - base : 0
        let target = row.need > 0 ? row.need : max(row.model, measured)
        row.model = max(row.model, min(max(measured, UInt64(warm * Double(row.need))), target))
        if done { row.model = target }
        row.target = target
        if let i = rows.firstIndex(where: { $0.id == id }) {
            if rows[i] != row { rows[i] = row }
        } else {
            rows.append(row)
        }
    }
}

// MARK: - The panel

/// "Pouring NAME into memory", one bar per memory, turning into "Loaded NAME".
struct PourPanel: View {
    let fill: MemoryFill

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            // No implicit animations here: the bar's own (clock-driven) flash marks the
            // finish, and the panel may be moving for other reasons at that moment.
            HStack(alignment: .firstTextBaseline) {
                Text(fill.title).mono(13, .bold, fill.done ? Theme.ok : Theme.ink).lineLimit(1)
                Spacer(minLength: 12)
                Text(fill.percent.map { "\($0)%" } ?? "").mono(12, .regular, fill.done ? Theme.ok : Theme.dim).monospacedDigit()
            }
            ForEach(fill.rows) { r in PourRow(row: r, done: fill.done) }
        }
        .padding(EdgeInsets(top: 14, leading: 16, bottom: 16, trailing: 16))
        .frame(maxWidth: .infinity, alignment: .leading)
        .overlay(Rectangle().stroke(fill.done ? Theme.ok.opacity(0.35) : Theme.rule, lineWidth: 1))
    }
}

private struct PourRow: View {
    let row: MemoryFill.Row
    let done: Bool

    var body: some View {
        let n = row.numbers(done: done)
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Text(row.label).mono(12, .regular, Theme.dim)
                Spacer()
                (Text(n.0).foregroundColor(done ? Theme.ok : Theme.ink) + Text(n.1).foregroundColor(Theme.dim))
                    .font(Theme.mono(12)).monospacedDigit()
            }
            PourBar(apps: row.appsFraction, model: row.modelFraction, target: row.targetFraction, complete: done)
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("\(row.label): \(n.0)\(n.1)")
    }
}

// MARK: - The bar

/// Memory as blocks, filling like something being poured in: other apps in dim
/// blocks, the model's share flowing toward a faint pulsing outline of where it
/// will end up, with a bright pulsing leading edge and a shimmer running
/// through what's already loaded. Values glide toward each new sample, so
/// samples a third of a second apart read as one continuous pour. When loading
/// finishes the model's blocks flash green and settle back to lilac.
///
/// All motion lives in a TimelineView + Canvas fed by plain numbers, so it costs
/// one cheap redraw per frame and never restarts when unrelated state changes.
/// With Reduce Motion it's a plain fill.
struct PourBar: View {
    var apps: Double
    var model: Double
    var target: Double
    var complete: Bool
    var height: CGFloat = 16

    /// Room above and below the blocks for the glow.
    static let glow: CGFloat = 7

    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var anim = PourAnimation()
    @State private var settled = false

    var body: some View {
        let moving = !reduceMotion && !settled
        TimelineView(.animation(minimumInterval: 1.0 / 60, paused: !moving)) { tl in
            Canvas { ctx, size in
                anim.advance(to: tl.date, apps: apps, model: model, target: target, complete: complete, smooth: moving)
                anim.draw(in: &ctx, size: size, pad: Self.glow, now: tl.date, motion: !reduceMotion)
            }
        }
        .frame(height: height + 2 * Self.glow)
        .padding(.vertical, -Self.glow)
        .allowsHitTesting(false)
        // After the green flash has settled, stop redrawing every frame.
        .task(id: complete) {
            guard complete else { settled = false; return }
            try? await Task.sleep(nanoseconds: 1_900_000_000)
            settled = true
        }
    }
}

/// The bar's motion state: lives across redraws, never observed by SwiftUI.
final class PourAnimation {
    private var shown = 0.0, shownApps = 0.0
    private var apps = 0.0, model = 0.0, target = 0.0
    private var complete = false
    private var last: Date?
    private var completedAt: Date?
    private let started = Date()

    func advance(to now: Date, apps a: Double, model m: Double, target t: Double, complete c: Bool, smooth: Bool) {
        func clamp(_ v: Double) -> Double { v.isFinite ? min(1, max(0, v)) : 0 }
        apps = clamp(a)
        target = clamp(t)
        model = clamp(m)
        if c && !complete { completedAt = now }
        complete = c
        if c { model = max(model, target) }
        let dt = min(0.1, max(0, now.timeIntervalSince(last ?? now)))
        last = now
        guard smooth else { shown = model; shownApps = apps; return }
        // Glide toward the measured value, with a minimum pace so it visibly flows.
        let gap = model - shown
        if abs(gap) < 0.0005 {
            shown = model
        } else {
            shown += (gap > 0 ? 1 : -1) * max(abs(gap) * min(1, dt * 5), min(abs(gap), dt * 0.08))
        }
        shownApps += (apps - shownApps) * min(1, dt * 6)
    }

    func draw(in ctx: inout GraphicsContext, size: CGSize, pad: CGFloat, now: Date, motion: Bool) {
        let seg: CGFloat = 6, gap: CGFloat = 2
        let h = size.height - 2 * pad
        let n = Int((size.width + gap) / (seg + gap))
        guard n > 0, h > 0 else { return }
        func idx(_ f: Double) -> Int { max(0, min(n, Int((f * Double(n)).rounded()))) }
        func rect(_ k: Int) -> CGRect { CGRect(x: CGFloat(k) * (seg + gap), y: pad, width: seg, height: h) }

        let ia = idx(shownApps)                                   // other apps end here
        let im = max(ia, idx(shownApps + shown))                  // the model so far
        let ig = max(im, idx(shownApps + max(target, shown)))     // the model when fully loaded
        let t = now.timeIntervalSince(started)
        let span = im - ia
        var sh = -1 // shimmer block: runs left to right, with a pause between passes
        if motion && !complete && span > 3 {
            sh = ia + Int((t * 26).truncatingRemainder(dividingBy: Double(span + 8)))
            if sh >= im { sh = -1 }
        }
        let edge = (complete || !motion) ? im : max(ia, im - 2)  // the 2 bright leading blocks

        let pulse = motion ? 0.5 + 0.5 * sin(t * 9) : 0.5
        var modelC = RGB.signal
        var edgeC = RGB.signal.mix(.white, 0.35 + 0.65 * pulse)
        var flash = 0.0
        if complete, motion, let at = completedAt {
            let f = min(1, max(0, now.timeIntervalSince(at) / 1.4))
            modelC = RGB.ok.mix(.signal, f * f)
            edgeC = modelC
            flash = 1 - f
        } else if complete {
            edgeC = modelC
        }
        let ghostAlpha = motion ? (38 + 34 * (0.5 + 0.5 * sin(t * 2.2))) / 255 : 0.2

        // Glow: under the leading edge while pouring, over the whole pour as it flashes green.
        if motion && im > ia && (!complete || flash > 0) {
            ctx.drawLayer { layer in
                layer.addFilter(.blur(radius: 5))
                var p = Path()
                for k in (complete ? ia : edge)..<im { p.addRect(rect(k)) }
                layer.fill(p, with: .color(complete ? RGB.ok.color.opacity(0.55 * flash) : edgeC.color.opacity(0.3 + 0.45 * pulse)))
            }
        }

        var empty = Path(), others = Path(), ghost = Path(), ghostLine = Path(), poured = Path(), lead = Path()
        for k in 0..<n {
            let r = rect(k)
            if k < ia { others.addRect(r) }
            else if k < edge { if k != sh { poured.addRect(r) } }
            else if k < im { lead.addRect(r) }
            else if k < ig { ghost.addRect(r); ghostLine.addRect(r.insetBy(dx: 0.5, dy: 0.5)) }
            else { empty.addRect(r) }
        }
        ctx.fill(empty, with: .color(Theme.rule))
        ctx.fill(others, with: .color(Theme.dim))
        ctx.fill(ghost, with: .color(RGB.signal.color.opacity(ghostAlpha)))
        ctx.stroke(ghostLine, with: .color(RGB.signal.color.opacity(min(1, ghostAlpha * 2.2))), lineWidth: 1)
        ctx.fill(poured, with: .color(modelC.color))
        if sh >= 0 && sh < edge { ctx.fill(Path(rect(sh)), with: .color(RGB.signal.mix(.white, 0.45).color)) }
        ctx.fill(lead, with: .color(edgeC.color))
    }
}

/// Theme colors as components, for mixing per frame.
private struct RGB {
    var r, g, b: Double
    static let signal = RGB(r: 0.725, g: 0.659, b: 1.0)   // #b9a8ff
    static let ok = RGB(r: 0.561, g: 0.890, b: 0.710)     // #8fe3b5
    static let white = RGB(r: 1, g: 1, b: 1)
    func mix(_ o: RGB, _ f: Double) -> RGB { RGB(r: r + (o.r - r) * f, g: g + (o.g - g) * f, b: b + (o.b - b) * f) }
    var color: Color { Color(red: r, green: g, blue: b) }
}
