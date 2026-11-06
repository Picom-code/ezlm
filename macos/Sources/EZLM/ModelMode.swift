import SwiftUI
import Observation

// MARK: - Engine JSON (GET /api/apps, POST /api/apps/close)

/// An app using a lot of memory, as Model mode offers to close it.
struct MemApp: Codable, Hashable, Identifiable {
    var id = ""
    var name = ""
    /// Its main executable, e.g. /Applications/Google Chrome.app/Contents/MacOS/Google Chrome
    var exe = ""
    var pids: [Int32] = []
    var ram: UInt64 = 0
    var gpu: UInt64 = 0
    /// Has a window, so it can be asked to quit like choosing Quit.
    var windows = false
    /// "app", "background" or "ai" (another model server).
    var kind = "app"
    var note = ""
}

struct MemAppList: Codable { var apps: [MemApp] }

struct CloseResult: Codable {
    var closed = false
    var remaining = 0
    var freed: UInt64 = 0
    var needForce = false
}

extension MemApp {
    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        func v<T: Decodable>(_ k: CodingKeys, _ d: T) -> T { (try? c.decodeIfPresent(T.self, forKey: k)) ?? d }
        id = v(.id, ""); name = v(.name, ""); exe = v(.exe, ""); pids = v(.pids, [])
        ram = v(.ram, 0); gpu = v(.gpu, 0); windows = v(.windows, false); kind = v(.kind, "app"); note = v(.note, "")
    }
}

extension CloseResult {
    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        func v<T: Decodable>(_ k: CodingKeys, _ d: T) -> T { (try? c.decodeIfPresent(T.self, forKey: k)) ?? d }
        closed = v(.closed, false); remaining = v(.remaining, 0); freed = v(.freed, 0); needForce = v(.needForce, false)
    }
}

// MARK: - The walk-through

/// Model mode: walks through the apps using the most memory, one at a time,
/// and closes only the ones the user says yes to. Gently first (like choosing
/// Quit); force-close only when asked, with a warning.
@MainActor
@Observable
final class ModelMode: Identifiable {
    enum Origin { case wizard, overview, system }

    enum Step: Equatable {
        case loading
        case listFailed(String)
        case empty
        case ask
        case closing(force: Bool)
        case closed(freed: UInt64)
        case stillRunning
        case closeFailed(String)
        case finished
    }

    let origin: Origin
    private(set) var apps: [MemApp] = []
    private(set) var index = 0
    private(set) var step: Step = .loading
    private(set) var freed: UInt64 = 0
    private(set) var freedGpu: UInt64 = 0
    private(set) var closedCount = 0
    /// Finished: checking what the model can do with the room (fresh status).
    private(set) var checking = false
    @ObservationIgnored private weak var model: AppModel?

    nonisolated var id: ObjectIdentifier { ObjectIdentifier(self) }

    init(origin: Origin, model: AppModel) {
        self.origin = origin
        self.model = model
    }

    var app: MemApp? { apps.indices.contains(index) ? apps[index] : nil }
    var busy: Bool { if case .closing = step { return true }; return false }
    var total: UInt64 { freed + freedGpu }
    private var potential: UInt64 { apps.reduce(0) { $0 + $1.ram + $1.gpu } }
    var fraction: Double { potential > 0 ? Double(total) / Double(potential) : 1 }

    func start() async {
        step = .loading
        guard let model else { return }
        do {
            let l: MemAppList = try await model.engine.get("apps")
            apps = l.apps
            index = 0
            step = apps.isEmpty ? .empty : .ask
        } catch {
            step = .listFailed(error.localizedDescription)
        }
    }

    /// Close (or stop, for background programs) the current app.
    func close() async {
        guard let a = app else { return }
        await close(a, force: !a.windows && a.kind == "background")
    }

    func forceClose() async {
        guard let a = app else { return }
        await close(a, force: true)
    }

    private func close(_ a: MemApp, force: Bool) async {
        guard let model else { return }
        step = .closing(force: force)
        do {
            let r: CloseResult = try await model.engine.post("apps/close", ["id": a.id, "force": force])
            if r.closed || r.freed > 0 {
                let f = r.freed > 0 ? r.freed : a.ram
                freed += f
                freedGpu += a.gpu
                closedCount += 1
                step = .closed(freed: f + a.gpu)
                try? await Task.sleep(nanoseconds: 900_000_000)
                if case .closed = step, app?.id == a.id { next() }
            } else if r.needForce {
                step = .stillRunning // explain, and only force-close if they say so
            } else {
                step = .closeFailed("\(a.name) didn't close.")
            }
        } catch {
            step = .closeFailed(error.localizedDescription)
        }
    }

    func keep() { next() }

    func stopHere() { Task { await finish() } }

    private func next() {
        if index + 1 < apps.count {
            index += 1
            step = .ask
        } else {
            Task { await finish() }
        }
    }

    private func finish() async {
        step = .finished
        guard closedCount > 0, let model else { return }
        checking = true
        await model.refreshStatus()
        checking = false
    }
}

/// What to do as Model mode closes.
enum ModelModeExit { case none, applyTune, openModels }

// MARK: - The sheet

struct ModelModeSheet: View {
    @Environment(AppModel.self) private var m
    let mm: ModelMode

    private let intro = "Closing apps you're not using gives your AI more memory. EZLM asks about each one; nothing closes without your OK."

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            switch mm.step {
            case .loading:
                header(intro)
                HStack(spacing: 10) { Spinner(); Text("Looking at what's using memory…").mono(13, .regular, Theme.dim) }
            case .listFailed(let message):
                header(intro)
                Text(message).mono(12, .regular, Theme.bad).fixedSize(horizontal: false, vertical: true)
                Button("Close") { m.endModelMode() }.buttonStyle(.primary).keyboardShortcut(.defaultAction)
            case .empty:
                header("Nothing big is running. Your AI already has all the memory this computer can give it.")
                doneButton()
            case .finished:
                finished
            default:
                if let a = mm.app { appStep(a) }
            }
        }
        .padding(30)
        .frame(width: 600, alignment: .leading)
        .background(Theme.black)
        .preferredColorScheme(.dark)
        .interactiveDismissDisabled(mm.busy)
    }

    private func header(_ sub: String) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            Text("Model mode").mono(18, .bold)
            Text(sub).mono(12, .regular, Theme.dim).fixedSize(horizontal: false, vertical: true)
        }
    }

    private func doneButton() -> some View {
        Button { m.endModelMode() } label: { HStack(spacing: 14) { Text("Done"); Text("↩").foregroundStyle(Theme.dim) } }
            .buttonStyle(.primary).keyboardShortcut(.defaultAction)
    }

    @ViewBuilder private func appStep(_ a: MemApp) -> some View {
        HStack(alignment: .firstTextBaseline) {
            Text("Model mode").mono(18, .bold)
            Spacer()
            Text("\(mm.index + 1) of \(mm.apps.count)").mono(12, .regular, Theme.dim)
        }
        HStack(spacing: 14) {
            AppIcon(exe: a.exe, name: a.name)
            VStack(alignment: .leading, spacing: 3) {
                Text(a.name).mono(16, .bold)
                Text("\(gb(a.ram)) of memory" + (a.gpu >= 64 << 20 ? ", \(gb(a.gpu)) of graphics memory" : "")).mono(12, .regular, Theme.dim)
            }
        }
        .padding(.top, 4)
        if !a.note.isEmpty {
            Text(a.note).mono(12, .regular, a.kind == "ai" ? Theme.gold : Theme.dim).fixedSize(horizontal: false, vertical: true)
        }

        switch mm.step {
        case .ask:
            HStack(spacing: 10) {
                Button { Task { await mm.close() } } label: {
                    HStack(spacing: 14) { Text("\(a.kind == "background" ? "Stop" : "Close") \(a.name)"); Text("↩").foregroundStyle(Theme.dim) }
                }
                .buttonStyle(.primary).keyboardShortcut(.defaultAction)
                Button("Keep it") { mm.keep() }.buttonStyle(.term)
                Button("Stop here") { mm.stopHere() }.buttonStyle(.small).keyboardShortcut(.cancelAction)
            }
        case .closing(let force):
            HStack(spacing: 10) { Spinner(); Text("\(force ? "Stopping" : "Closing") \(a.name)…").mono(13, .regular, Theme.dim) }
        case .closed(let f):
            (Text("✓ ").foregroundColor(Theme.ok) + Text("\(a.name) closed. Freed \(gb(f)).").foregroundColor(Theme.ink)).font(Theme.mono(13))
        case .stillRunning:
            Text(a.windows ? "\(a.name) is still running (it may have asked to save something, or it keeps going in the background)."
                           : "\(a.name) has no window to close.")
                .mono(13, .regular, Theme.gold).fixedSize(horizontal: false, vertical: true)
            Text("Force-closing stops it right away: anything unsaved in it is lost.").mono(12, .regular, Theme.dim)
            HStack(spacing: 10) {
                Button("Force close \(a.name)") { Task { await mm.forceClose() } }.buttonStyle(.danger)
                Button("Keep it") { mm.keep() }.buttonStyle(.term).keyboardShortcut(.cancelAction)
            }
        case .closeFailed(let message):
            Text(message).mono(12, .regular, Theme.bad).fixedSize(horizontal: false, vertical: true)
            HStack(spacing: 10) {
                Button("Try again") { Task { await mm.close() } }.buttonStyle(.term)
                Button("Keep it") { mm.keep() }.buttonStyle(.term).keyboardShortcut(.cancelAction)
            }
        default:
            EmptyView()
        }

        // Freed so far
        VStack(spacing: 10) {
            Rectangle().fill(Theme.rule).frame(height: 1)
            HStack(spacing: 12) {
                Text("Freed so far").mono(12, .regular, Theme.dim)
                BlockBar(mm.fraction, color: Theme.ok, height: 8).animation(.easeOut(duration: 0.5), value: mm.fraction)
                Text(gb(mm.total)).mono(12, .regular, Theme.ok).monospacedDigit()
            }
        }
        .padding(.top, 6)
    }

    @ViewBuilder private var finished: some View {
        Text("Model mode").mono(18, .bold)
        if mm.closedCount == 0 {
            Text("Nothing was closed. You can run Model mode again any time from Overview or System.").mono(13, .regular, Theme.dim)
                .fixedSize(horizontal: false, vertical: true)
            doneButton()
        } else {
            Text("Freed \(gb(mm.total))").mono(22, .bold, Theme.ok)
            Text("Closed \(mm.closedCount) app\(mm.closedCount == 1 ? "" : "s")."
                 + (mm.freedGpu >= 256 << 20 ? " \(gb(mm.freedGpu)) of that was graphics memory, the most valuable kind for AI." : ""))
                .mono(13, .regular, Theme.dim).fixedSize(horizontal: false, vertical: true)
            VStack(alignment: .leading, spacing: 10) {
                if mm.checking {
                    HStack(spacing: 10) { Spinner(); Text("Checking what your AI can do with the room…").mono(12, .regular, Theme.dim) }
                } else if mm.origin != .wizard, let tune = m.status?.tune, m.status?.config != nil {
                    Text("Your model can use it: " + tune.why).mono(13).fixedSize(horizontal: false, vertical: true)
                    HStack(spacing: 10) {
                        Button { m.endModelMode(.applyTune) } label: { HStack(spacing: 14) { Text("Apply the faster setup"); Text("↩").foregroundStyle(Theme.dim) } }
                            .buttonStyle(.primary).keyboardShortcut(.defaultAction)
                        Button("Not now") { m.endModelMode() }.buttonStyle(.term)
                    }
                } else if mm.origin == .wizard {
                    Text("EZLM will compare models again with the extra room.").mono(13, .regular, Theme.dim)
                    Button { m.endModelMode() } label: { HStack(spacing: 14) { Text("See what fits now"); Text("↩").foregroundStyle(Theme.dim) } }
                        .buttonStyle(.primary).keyboardShortcut(.defaultAction)
                } else {
                    Text("Your model has more breathing room now. Models that didn't fit before may fit now: have a look in Models.")
                        .mono(13, .regular, Theme.dim).fixedSize(horizontal: false, vertical: true)
                    HStack(spacing: 10) {
                        doneButton()
                        Button("Open Models") { m.endModelMode(.openModels) }.buttonStyle(.term)
                    }
                }
            }
        }
    }
}

/// The app's real icon (from its .app bundle), or its initial in a box.
struct AppIcon: View {
    let exe: String
    let name: String

    var body: some View {
        if let img = Self.icon(exe) {
            Image(nsImage: img).resizable().interpolation(.high).frame(width: 40, height: 40)
        } else {
            Text(String(name.prefix(1))).mono(18, .bold, Theme.dim)
                .frame(width: 40, height: 40)
                .overlay(Rectangle().stroke(Theme.faint, lineWidth: 1))
        }
    }

    /// The outermost .app bundle in the executable's path, e.g.
    /// /Applications/Google Chrome.app for its helpers too.
    static func icon(_ exe: String) -> NSImage? {
        guard !exe.isEmpty else { return nil }
        var path = ""
        for part in exe.split(separator: "/") {
            path += "/" + part
            if part.hasSuffix(".app") {
                return FileManager.default.fileExists(atPath: path) ? NSWorkspace.shared.icon(forFile: path) : nil
            }
        }
        return nil
    }
}
