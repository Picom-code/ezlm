import SwiftUI
import AppKit

/// What the model is doing, as the signal shows it.
enum Mood { case off, loading, idle, reading, thinking, writing, error }

/// The signal: a braille waveform across the top of Chat that shows what the
/// model is doing, whichever app is using it. It breathes while idle, scans
/// while reading a prompt, beats while thinking, pulses with the words it
/// writes, and lies flat (with a heartbeat blip) when the server is off.
/// The motion lives here, outside the view, so it carries on smoothly when
/// the page redraws.
@MainActor
@Observable
final class SignalWave {
    private(set) var front = ""
    private(set) var back = ""
    private(set) var color: (r: Double, g: Double, b: Double) = (0.23, 0.23, 0.23)

    @ObservationIgnored var moodOf: () -> Mood = { .idle }
    @ObservationIgnored var rateOf: () -> Double = { 0 }
    @ObservationIgnored var onFrame: () -> Void = {}
    @ObservationIgnored var cols = 90

    static let rows = 4 // braille rows, 4 dots each: 16 dots tall
    static let size: CGFloat = 14
    @ObservationIgnored private var phase = 0.0
    @ObservationIgnored private var energy = 0.0
    @ObservationIgnored private var kick = 0.0
    @ObservationIgnored private var ripple = -1.0
    @ObservationIgnored private var t = 0.0
    @ObservationIgnored private var drawn = 0.0
    @ObservationIgnored private(set) var timer: Timer?

    /// The width of one braille cell in the wave's font.
    static let cellWidth: CGFloat = {
        let font = NSFont.monospacedSystemFont(ofSize: size, weight: .regular)
        let w = (String(repeating: "\u{28FF}", count: 40) as NSString).size(withAttributes: [.font: font]).width / 40
        return w > 0 ? w : 8.4
    }()

    /// Whether the wave is drawing (a timer is scheduled).
    var isRunning: Bool { timer != nil }

    func start() {
        guard timer == nil else { return }
        timer = Timer.scheduledTimer(withTimeInterval: 1.0 / 30, repeats: true) { [weak self] t in
            MainActor.assumeIsolated {
                // The run loop owns the timer: if the wave is gone without stop()
                // (a pane torn down some other way), the timer has to end itself.
                guard let self else { t.invalidate(); return }
                self.step()
            }
        }
    }

    func stop() { timer?.invalidate(); timer = nil }

    deinit { timer?.invalidate() } // a wave that goes away without stop() leaves no timer behind

    /// A burst of energy: every chunk of words written kicks the wave a little.
    func pulse(_ amount: Double) { kick = min(1.6, kick + amount) }
    /// One ripple from the middle: an answer just finished.
    func rippleOnce() { ripple = 0 }

    private static let cSignal = rgb(Theme.signal), cFaint = rgb(Theme.faint), cDim = rgb(Theme.dim), cGold = rgb(Theme.gold), cInk = rgb(Theme.ink), cBad = rgb(Theme.bad)

    private static func rgb(_ c: Color) -> (Double, Double, Double) {
        let n = NSColor(c).usingColorSpace(.sRGB) ?? .gray
        return (Double(n.redComponent), Double(n.greenComponent), Double(n.blueComponent))
    }

    private static func mix(_ a: (Double, Double, Double), _ b: (Double, Double, Double), _ t: Double) -> (Double, Double, Double) {
        (a.0 + (b.0 - a.0) * t, a.1 + (b.1 - a.1) * t, a.2 + (b.2 - a.2) * t)
    }

    @ObservationIgnored private var ticks = 0

    private func step() {
        onFrame() // the chat's typewriter keeps its pace even when the wave rests
        ticks &+= 1
        let mood = moodOf()
        // Save the battery: no drawing while EZLM is hidden or covered; a calm 10 fps while idle.
        let visible = NSApp.isActive || NSApp.windows.contains { $0.isVisible && $0.occlusionState.contains(.visible) }
        let calm = mood == .idle || mood == .off
        if !visible || (calm && ticks % 3 != 0) { return }
        let dt = calm ? 0.1 : 1.0 / 30
        let rate = max(0, rateOf())
        let signal = Self.cSignal, faint = Self.cFaint
        let (target, speed, tint): (Double, Double, (Double, Double, Double)) = {
            switch mood {
            case .off: return (0.05, 0.5, Self.cDim)
            case .loading: return (0.32, 1.0, Self.cGold)
            case .reading: return (0.62, 3.2, signal)
            case .thinking: return (0.72, 2.3, Self.cGold)
            case .writing: return (0.5 + min(0.3, rate / 160), 2.0 + min(3, rate / 35), Self.mix(signal, Self.cInk, 0.25))
            case .error: return (0.04, 0.3, Self.cBad)
            case .idle: return (0.42 + 0.14 * sin(t * 0.9), 0.8, Self.mix(signal, faint, 0.2)) // a slow breath
            }
        }()
        energy += (target + kick * 0.35 - energy) * 0.07
        kick *= 0.9
        phase += speed * dt
        t += dt
        if ripple >= 0 { ripple += dt; if ripple > 1.6 { ripple = -1 } }
        drawn = min(1, drawn + dt / 0.9)
        color = Self.mix(color, tint, 0.08)
        let w = cols * 2, h = Self.rows * 4
        front = render(w, h, thick: 2) { self.wave(mood, $0, 0) }
        back = render(w, h, thick: 1) { -self.wave(mood, $0, 1) * 0.8 } // the echo mirrors the main strand: they braid
    }

    private static let tau = Double.pi * 2

    /// The wave's height at u (0..1 across), from -1 to 1. Layer 1 is the echo.
    private func wave(_ mood: Mood, _ u: Double, _ layer: Int) -> Double {
        let p = phase * (layer == 0 ? 1 : 0.7) + Double(layer) * 1.9, k = layer == 0 ? 1.0 : 0.8
        let env = pow(sin(Double.pi * u), 0.8) // meets the center line at both ends
        var f: Double
        switch mood {
        case .off, .error: f = blip(u, layer)
        case .reading: f = sin(Self.tau * 6 * k * u - 3 * p) * (0.22 + 0.78 * packet(u, layer))
        case .thinking: f = 0.55 * (sin(Self.tau * 4 * k * u - p) + sin(Self.tau * 4.45 * k * u - 1.32 * p))
        case .writing: f = 0.68 * sin(Self.tau * 2.2 * k * u - p) + 0.32 * sin(Self.tau * 5.3 * k * u - 1.9 * p) * (0.6 + kick)
        case .loading: f = sin(Self.tau * 1.4 * k * u - p) * (0.35 + 0.65 * packet(u, layer))
        case .idle: f = 0.72 * sin(Self.tau * 1.5 * k * u - p) + 0.28 * sin(Self.tau * 3.7 * k * u + 0.7 * p)
        }
        if ripple >= 0 {
            let d = abs(u - 0.5)
            f += 0.9 * exp(-ripple * 2.6) * exp(-d * 3) * sin(Self.tau * 9 * d - ripple * 14)
        }
        return max(-1, min(1, energy * env * f))
    }

    private func packet(_ u: Double, _ layer: Int) -> Double {
        let at = (t * (layer == 0 ? 0.55 : 0.4) + Double(layer) * 0.5).truncatingRemainder(dividingBy: 1.3) - 0.15
        return exp(-pow((u - at) / 0.09, 2))
    }

    private func blip(_ u: Double, _ layer: Int) -> Double {
        guard layer == 0 else { return 0 }
        let at = (t / 4).truncatingRemainder(dividingBy: 1) * 1.4 - 0.2, d = u - at
        return abs(d) > 0.04 ? 0 : 1.6 * exp(-pow(d / 0.012, 2)) * sin(d * 260)
    }

    // Braille dot bits by [row in cell][column in cell].
    private static let bit = [[0x01, 0x08], [0x02, 0x10], [0x04, 0x20], [0x40, 0x80]]

    /// Draws y(u) as a connected line of braille dots, w by h dots.
    private func render(_ w: Int, _ h: Int, thick: Int, _ y: (Double) -> Double) -> String {
        var cells = [[Int]](repeating: [Int](repeating: 0, count: cols), count: Self.rows)
        let mid = Double(h - 1) / 2
        var prev = -1
        let upto = Int(Double(w) * (1 - pow(1 - drawn, 3))) // sweeps in once, the first time
        for x in 0..<upto {
            let yi = max(0, min(h - 1, Int((mid - y(Double(x) / Double(w - 1)) * mid).rounded())))
            let lo = prev < 0 ? yi : min(prev, yi), hi = prev < 0 ? yi : max(prev, yi)
            for yy in lo...min(h - 1, hi + thick - 1) { cells[yy / 4][x / 2] |= Self.bit[yy % 4][x % 2] }
            prev = yi
        }
        return cells.map { row in String(row.map { Character(UnicodeScalar(0x2800 + $0)!) }) }.joined(separator: "\n")
    }
}

/// The wave on screen: two braille layers, brightest in the middle.
struct SignalWaveView: View {
    let wave: SignalWave

    var body: some View {
        let c = wave.color
        let mid = Color(red: c.r, green: c.g, blue: c.b)
        let edge = Color(white: 0.149).mix(with: mid, by: 0.18)
        ZStack(alignment: .topLeading) {
            layer(wave.back, LinearGradient(colors: [edge, edge.mix(with: mid, by: 0.3), edge.mix(with: mid, by: 0.5), edge.mix(with: mid, by: 0.3), edge], startPoint: .leading, endPoint: .trailing))
            layer(wave.front, LinearGradient(colors: [edge, edge.mix(with: mid, by: 0.6), mid, edge.mix(with: mid, by: 0.6), edge], startPoint: .leading, endPoint: .trailing))
        }
        .frame(height: CGFloat(SignalWave.rows) * SignalWave.size + 2, alignment: .topLeading)
        .frame(maxWidth: .infinity, alignment: .leading)
        .clipped()
        .background(GeometryReader { g in
            Color.clear.onAppear { wave.cols = max(16, Int(g.size.width / SignalWave.cellWidth)) }
                .onChange(of: g.size.width) { _, w in wave.cols = max(16, Int(w / SignalWave.cellWidth)) }
        })
        .accessibilityHidden(true)
    }

    private func layer(_ text: String, _ fill: LinearGradient) -> some View {
        Text(text)
            .font(.system(size: SignalWave.size, design: .monospaced))
            .lineSpacing(-3) // rows touch, so the line reads as one wave
            .fixedSize()
            .foregroundStyle(fill)
    }
}

extension Color {
    /// Mixes toward another color (t = 0: this one, 1: the other).
    func mix(with other: Color, by t: Double) -> Color {
        let a = NSColor(self).usingColorSpace(.sRGB) ?? .gray, b = NSColor(other).usingColorSpace(.sRGB) ?? .gray
        return Color(red: Double(a.redComponent + (b.redComponent - a.redComponent) * t),
                     green: Double(a.greenComponent + (b.greenComponent - a.greenComponent) * t),
                     blue: Double(a.blueComponent + (b.blueComponent - a.blueComponent) * t))
    }
}
