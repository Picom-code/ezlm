import SwiftUI

// MARK: - Segmented block bar

/// The terminal progress bar, drawn as blocks: [0, a) in `first`, [a, b) in
/// `second`, the rest as empty blocks. Animatable, so bars grow smoothly.
struct BlockBar: View, Animatable {
    var a: Double
    var b: Double
    var first: Color = Theme.dim
    var second: Color = Theme.signal
    var height: CGFloat = 12
    var sheen = false

    var animatableData: AnimatablePair<Double, Double> {
        get { AnimatablePair(a, b) }
        set { a = newValue.first; b = newValue.second }
    }

    /// A single fill from 0 to `value`.
    init(_ value: Double, color: Color = Theme.signal, height: CGFloat = 12, sheen: Bool = false) {
        a = 0; b = value; second = color; self.height = height; self.sheen = sheen
    }

    /// Two segments: other usage (dim) then this model (signal).
    init(others: Double, model: Double, height: CGFloat = 12) {
        a = others; b = others + model; self.height = height
    }

    var body: some View {
        Canvas { ctx, size in
            let seg: CGFloat = 6, gap: CGFloat = 2
            let n = Int((size.width + gap) / (seg + gap))
            for i in 0..<max(n, 0) {
                let x = CGFloat(i) * (seg + gap)
                let mid = Double((x + seg / 2) / size.width)
                let color: Color = mid < a ? first : mid < b ? second : Theme.rule
                ctx.fill(Path(CGRect(x: x, y: 0, width: seg, height: size.height)), with: .color(color))
            }
        }
        .frame(height: height)
        .overlay(sheen ? AnyView(Sheen(until: b)) : AnyView(EmptyView()))
    }
}

/// A light sweep across the filled part of a bar so it never looks frozen.
private struct Sheen: View {
    let until: Double
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    var body: some View {
        TimelineView(.animation(paused: reduceMotion)) { tl in
            GeometryReader { geo in
                let w = geo.size.width * until
                let t = tl.date.timeIntervalSinceReferenceDate.truncatingRemainder(dividingBy: 1.6) / 1.6
                LinearGradient(colors: [.clear, .white.opacity(reduceMotion ? 0 : 0.35), .clear], startPoint: .leading, endPoint: .trailing)
                    .frame(width: 90)
                    .offset(x: -90 + (w + 90) * t)
                    .frame(width: max(w, 0), alignment: .leading)
                    .clipped()
            }
        }
        .allowsHitTesting(false)
    }
}

/// Indeterminate bar: a block of light sliding back and forth.
struct WorkingBar: View {
    var height: CGFloat = 6
    var color: Color = Theme.signal
    var body: some View {
        TimelineView(.animation) { tl in
            let t = tl.date.timeIntervalSinceReferenceDate.truncatingRemainder(dividingBy: 2.6) / 1.3
            let p = t < 1 ? t : 2 - t
            let eased = p * p * (3 - 2 * p)
            BlockBar(a: eased * 0.78, b: eased * 0.78 + 0.22, first: Theme.rule, second: color, height: height)
        }
    }
}

extension BlockBar {
    init(a: Double, b: Double, first: Color, second: Color, height: CGFloat) {
        self.a = a; self.b = b; self.first = first; self.second = second; self.height = height
    }
}

// MARK: - Spinner

struct Spinner: View {
    var color: Color = Theme.signal
    private let frames = ["⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"]
    var body: some View {
        TimelineView(.periodic(from: .now, by: 0.08)) { tl in
            let i = Int(tl.date.timeIntervalSinceReferenceDate / 0.08) % frames.count
            Text(frames[i]).mono(13, .regular, color)
        }
    }
}

// MARK: - Logo

enum LogoState { case idle, busy, done }

struct Logo: View {
    var state: LogoState
    private static let E = ["███████╗", "██╔════╝", "█████╗  ", "██╔══╝  ", "███████╗", "╚══════╝"]
    private static let Z = ["███████╗", "╚══███╔╝", "  ███╔╝ ", " ███╔╝  ", "███████╗", "╚══════╝"]
    private static let L = ["██╗     ", "██║     ", "██║     ", "██║     ", "███████╗", "╚══════╝"]
    private static let M = ["███╗   ███╗", "████╗ ████║", "██╔████╔██║", "██║╚██╔╝██║", "██║ ╚═╝ ██║", "╚═╝     ╚═╝"]
    @State private var drawn = 0
    @State private var flash = false

    var body: some View {
        TimelineView(.animation(paused: state != .busy)) { tl in
            let pulse = state == .busy ? (sin(tl.date.timeIntervalSinceReferenceDate * 3.4) + 1) / 2 : 0
            let lm: Color = flash ? Theme.ok : state == .busy ? mix(Theme.ink, Theme.signal, pulse) : Theme.ink
            VStack(alignment: .leading, spacing: 0) {
                ForEach(0..<6, id: \.self) { i in
                    (Text(Self.E[i] + Self.Z[i]).foregroundColor(Theme.dim) + Text(Self.L[i] + Self.M[i]).foregroundColor(lm))
                        .font(.system(size: 12.5, weight: .bold, design: .monospaced))
                        .lineSpacing(0)
                        .opacity(i < drawn ? 1 : 0)
                        .mask(alignment: .leading) { Rectangle().scaleEffect(x: i < drawn ? 1 : 0, anchor: .leading) }
                }
            }
            .shadow(color: (flash ? Theme.ok : Theme.signal).opacity(state == .busy ? pulse * 0.45 : flash ? 0.5 : 0), radius: 14)
        }
        .fixedSize()
        .onAppear {
            for i in 1...6 {
                withAnimation(.easeOut(duration: 0.45).delay(Double(i) * 0.06)) { drawn = i }
            }
        }
        .onChange(of: state) { _, s in
            guard s == .done else { return }
            withAnimation(.easeOut(duration: 0.25)) { flash = true }
            withAnimation(.easeIn(duration: 0.9).delay(0.4)) { flash = false }
        }
    }

    private func mix(_ a: Color, _ b: Color, _ t: Double) -> Color {
        let ca = NSColor(a).usingColorSpace(.sRGB)!, cb = NSColor(b).usingColorSpace(.sRGB)!
        return Color(red: ca.redComponent + (cb.redComponent - ca.redComponent) * t,
                     green: ca.greenComponent + (cb.greenComponent - ca.greenComponent) * t,
                     blue: ca.blueComponent + (cb.blueComponent - ca.blueComponent) * t)
    }
}

// MARK: - App marks (pixel font, half blocks)

enum PixelFont {
    static let glyphs: [Character: [String]] = [
        "o": [".##.", "#..#", "#..#", "#..#", ".##."],
        "p": ["###.", "#..#", "###.", "#...", "#..."],
        "e": ["####", "#...", "###.", "#...", "####"],
        "n": ["#..#", "##.#", "#.##", "#..#", "#..#"],
        "c": [".###", "#...", "#...", "#...", ".###"],
        "d": ["###.", "#..#", "#..#", "#..#", "###."],
        "h": ["#..#", "#..#", "####", "#..#", "#..#"],
        "r": ["###.", "#..#", "###.", "#.#.", "#..#"],
        "m": ["#...#", "##.##", "#.#.#", "#...#", "#...#"],
        "s": [".###", "#...", ".##.", "...#", "###."],
    ]

    /// Renders a word as 3 lines of half-block characters (5 pixel rows + 1 blank).
    static func lines(_ word: String) -> [String] {
        let rows: [String] = (0..<6).map { r in
            word.map { ch -> String in
                let g = glyphs[ch] ?? ["."]
                return (r < 5 ? g[r] : String(repeating: ".", count: g[0].count)) + "."
            }.joined()
        }
        return stride(from: 0, to: 6, by: 2).map { r in
            let top = Array(rows[r]), bot = Array(rows[r + 1])
            return String(top.indices.map { i -> Character in
                switch (top[i] == "#", bot[i] == "#") {
                case (true, true): return "█"
                case (true, false): return "▀"
                case (false, true): return "▄"
                default: return " "
                }
            })
        }
    }
}

/// The pixel wordmark an app tile shows when its logo file is missing.
/// OpenCode: grey "open" + white "code". Hermes: gold. DeepSeek Harness: white "dsh".
struct AppMark: View {
    let id: String
    var body: some View {
        let font = Font.system(size: 7.5, weight: .bold, design: .monospaced)
        VStack(alignment: .leading, spacing: 0) {
            switch id {
            case "opencode":
                let a = PixelFont.lines("open"), b = PixelFont.lines("code")
                ForEach(0..<3, id: \.self) { i in
                    (Text(a[i]).foregroundColor(Theme.dim) + Text(b[i]).foregroundColor(Theme.ink)).font(font)
                }
            case "hermes":
                ForEach(PixelFont.lines("hermes"), id: \.self) { l in Text(l).font(font).foregroundColor(Theme.gold) }
            case "dsh":
                ForEach(PixelFont.lines("dsh"), id: \.self) { l in Text(l).font(font).foregroundColor(Theme.ink) }
            default:
                Text(id).mono(11, .bold, Theme.dim)
            }
        }
        .fixedSize()
        .accessibilityHidden(true)
    }
}

/// An app's logo in a square box (36 on tiles, 20 in lists, 16 on buttons).
/// The DeepSeek Harness whale is a bare glyph, so it's drawn at 80 % to match
/// the weight of the two that bring their own tile. Without the file: the
/// pixel wordmark where there's room (`fallback`), else nothing.
struct AppLogo: View {
    let target: Target
    var size: CGFloat = 36
    var fallback = false

    var body: some View {
        Group {
            if let img = Logos.app(target.logoKey) {
                let glyph = target.logoKey == "dsh" ? size * 0.8 : size
                Image(nsImage: img)
                    .resizable()
                    .interpolation(.high)
                    .aspectRatio(contentMode: .fit)
                    .frame(width: glyph, height: glyph)
                    .frame(width: size, height: size)
            } else if fallback {
                AppMark(id: target.id)
            }
        }
        .accessibilityHidden(true)
    }
}

// MARK: - Typewriter

/// What each keyed typewriter last typed, so rebuilding a screen (or coming
/// back to it) shows the text at once instead of typing it again.
@MainActor enum TypedMemory { static var last: [String: String] = [:] }

/// Types `text` out once. It types again only when the text itself changes;
/// with a `key`, never twice in a row for the same text, even across rebuilds.
struct Typewriter: View {
    let text: String
    var speed: Double = 0.03
    var key: String?
    @State private var shown: Int

    init(text: String, speed: Double = 0.03, key: String? = nil) {
        self.text = text
        self.speed = speed
        self.key = key
        let seen = key.map { TypedMemory.last[$0] == text } ?? false
        _shown = State(initialValue: seen ? text.count : 0)
    }

    var body: some View {
        HStack(spacing: 2) {
            Text(String(text.prefix(shown)))
            if shown < text.count { Text("▍").foregroundStyle(Theme.signal) }
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(text)
        .task(id: text) {
            if let key {
                if TypedMemory.last[key] == text { shown = text.count; return }
                TypedMemory.last[key] = text
            }
            shown = 0
            for i in 1...max(text.count, 1) {
                try? await Task.sleep(nanoseconds: UInt64(speed * 1e9))
                if Task.isCancelled { return }
                shown = i
            }
        }
    }
}

// MARK: - Misc

/// Fades and lifts content in, staggered by `delay`.
struct Reveal: ViewModifier {
    var delay: Double = 0
    @State private var on = false
    func body(content: Content) -> some View {
        content.opacity(on ? 1 : 0).offset(y: on ? 0 : 5)
            .onAppear { withAnimation(.easeOut(duration: 0.35).delay(delay)) { on = true } }
    }
}

extension View {
    func reveal(_ delay: Double = 0) -> some View { modifier(Reveal(delay: delay)) }
}

/// A status dot; while `active`, a ring keeps expanding out of it. Driven by
/// the clock, so it follows `active` and `color` as they change.
struct Pulse: View {
    var color: Color = Theme.ok
    var active = true
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    var body: some View {
        let moving = active && !reduceMotion
        TimelineView(.animation(minimumInterval: 1.0 / 30, paused: !moving)) { tl in
            let t = moving ? tl.date.timeIntervalSinceReferenceDate.truncatingRemainder(dividingBy: 1.8) / 1.8 : 1
            let e = 1 - (1 - t) * (1 - t)
            Circle().fill(color).frame(width: 9, height: 9)
                .background(Circle().stroke(color.opacity(moving ? 0.6 * (1 - e) : 0), lineWidth: 10 * e).scaleEffect(1 + 0.6 * e))
        }
        .frame(width: 9, height: 9)
        .accessibilityHidden(true)
    }
}

/// A short confirmation at the bottom of the window that fades by itself.
struct ToastView: View {
    let toast: Toast
    var body: some View {
        Text(toast.text)
            .mono(13, .regular, toast.color)
            .multilineTextAlignment(.center)
            .padding(.horizontal, 16).padding(.vertical, 10)
            .background(Color(white: 0.078))
            .overlay(Rectangle().stroke(toast.color == Theme.ink ? Theme.faint : toast.color.opacity(0.55), lineWidth: 1))
            .frame(maxWidth: 720)
            .fixedSize(horizontal: false, vertical: true)
            .allowsHitTesting(false)
    }
}
