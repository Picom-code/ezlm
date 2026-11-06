import SwiftUI
import Observation

enum Screen: Equatable { case booting, scanning, pick, deploying, done, failed(String) }

enum Section: String, CaseIterable, Identifiable {
    case overview = "Overview", chat = "Chat", models = "Models", system = "System", connect = "Connect", keys = "Keys", logs = "Logs", settings = "Settings"
    var id: String { rawValue }
    var icon: String {
        switch self {
        case .overview: return "gauge.with.dots.needle.33percent"
        case .chat: return "text.bubble"
        case .models: return "square.stack.3d.up"
        case .system: return "cpu"
        case .connect: return "puzzlepiece.extension"
        case .keys: return "key"
        case .logs: return "text.alignleft"
        case .settings: return "gearshape"
        }
    }
}

struct ScanLine: Identifiable {
    let id: String
    let label: String
    var value = ""
    var state: StepState = .running
}

enum StepState { case waiting, running, ok, failed }

struct DeployStep: Identifiable, Equatable {
    let id: String
    let label: String
    let weight: Double
    var state: StepState = .waiting
    var sub = ""
    var stat = ""
    var fraction = 0.0
    var showBar = false
    var indeterminate = false
}

/// What the model server is doing, as the sidebar, badges and buttons show it.
enum ServerState: Equatable {
    case live, loading, stopped, notSetUp
    var title: String {
        switch self {
        case .live: return "Live"
        case .loading: return "Loading model…"
        case .stopped: return "Stopped"
        case .notSetUp: return "Not set up"
        }
    }
    var color: Color {
        switch self {
        case .live: return Theme.ok
        case .loading: return Theme.gold
        case .stopped, .notSetUp: return Theme.bad
        }
    }
}

/// An app tile's line while connecting, or its result.
/// An app's connection log, as "Show details" shows it.
struct AppLogView: Identifiable {
    var id: String
    var name: String
    var lines: [String]
}

struct AppTileState: Equatable {
    var text: String
    var color: Color
    var working: Bool
}

struct Toast: Equatable {
    let id: Int
    let text: String
    let color: Color
}

/// A switch that needs disk space: confirmed before anything is deleted.
struct MakeRoom: Identifiable {
    let id = UUID()
    let plan: Plan
    let message: String
}

/// The "Report a problem" sheet, opened with this text in the box.
struct ReportDraft: Identifiable {
    let id = UUID()
    let text: String
}

@MainActor
@Observable
final class AppModel {
    @ObservationIgnored let engine = Engine()

    // Setup wizard
    var screen: Screen = .booting
    var logo: LogoState = .idle
    var scanLines: [ScanLine] = []
    var scan: ScanResult?
    var chosenID: String?
    var pref: Pref = .balanced
    var prefBusy = false
    var reply = ""
    var note = ""

    // Install / switch: the wizard's step 3, or a sheet in the main app
    var steps: [DeployStep] = []
    var installTitle = ""
    var overallETA = ""
    var overallStatus = ""
    var deployError: String?
    var deployHint = ""
    var installRunning = false
    var installSheet = false
    var installFinished = false
    var installDoneText = ""
    var cancelling = false
    /// The memory-filling panel; appears once the model starts loading.
    var fill: MemoryFill?
    @ObservationIgnored private var installInSheet = false
    @ObservationIgnored private var fillName = ""
    @ObservationIgnored private var fillGpuNeed: UInt64 = 0
    @ObservationIgnored private var fillRamNeed: UInt64 = 0
    @ObservationIgnored private var fillLast: Live?
    @ObservationIgnored private var preread = false

    // Main app
    var setupDone = false
    var section: Section = .overview
    /// The built-in chat (ChatModel.swift); it lives as long as the app.
    let chat = ChatModel()
    var status: Status?
    var live: Live?
    /// Writing speed, last seen while the model was writing.
    var lastSpeed = 0.0
    /// The pour panel for loads outside the install flow (start, restart).
    var ovFill: MemoryFill?
    @ObservationIgnored private var ovFillDoneAt: Date?
    var logs: [String] = []
    var installed: [InstalledModel] = []
    var testResults: [TestResult] = []
    var testing = false
    var toast: Toast?
    var makeRoom: MakeRoom?
    /// Model mode (free up memory), while it's open.
    var modelMode: ModelMode?

    // Connect
    var appState: [String: AppTileState] = [:]
    var selectedApps: Set<String> = []
    var makeDefault = true

    // Keys
    var keyWork = false
    var flashKeyID: String?

    // Settings: updates (see Settings.swift)
    var updateInfo: UpdateInfo?
    var updateChecking = false
    /// Downloading or installing an update.
    var updateBusy = false
    /// Download progress, 0...1 (nil until the first progress event).
    var updateFraction: Double?
    var updateNote = ""
    /// Installing by itself failed: the disk image is open in Finder for a drag.
    var updateManual = false
    @ObservationIgnored var updateCheckStarted = false
    @ObservationIgnored var updateInstalling = false

    // Settings: where models are saved
    var storage: StorageInfo?
    var storageError = ""
    /// The drive or folder picked, not saved yet.
    var pendingDir: String?
    var moveModels = true
    var storageBusy = false
    var storageFraction: Double?
    var storageNote = ""

    /// What quitting would interrupt right now, in words; nil when nothing would be lost.
    var quitWarning: String? {
        if installRunning {
            let doing = installTitle.isEmpty ? "setting up your model" : installTitle.prefix(1).lowercased() + installTitle.dropFirst()
            return "EZLM is still \(doing). If you quit now, that stops. What's already downloaded is kept, and it picks up where it stopped next time."
        }
        if storageBusy { return "EZLM is moving your models. If you quit now, the move stops and the files not moved yet stay where they are." }
        if updateBusy && !updateInstalling { return "EZLM is downloading its update. If you quit now, the update stops; you can start it again in Settings." }
        if chat.streaming { return "Your model is still writing an answer in Chat. If you quit now, it stops there (what's written so far is kept)." }
        return nil
    }

    // Use it: Open OpenCode / Open Hermes (see ReportLaunch.swift)
    var launching = false
    var launchNote = ""
    var launchFailed = false
    /// The app the newest launch was for, so its failure can show that app's log.
    var launchID = ""
    /// "Show details": an app's connection log, while it's open.
    var appLog: AppLogView?

    // Report a problem
    var reportDraft: ReportDraft?
    /// A report that couldn't be sent: the box starts with it next time.
    @ObservationIgnored var unsentReport = ""

    @ObservationIgnored private var polling: Task<Void, Never>?
    @ObservationIgnored private var listening = false
    @ObservationIgnored private var engineGone = false
    @ObservationIgnored private var retryScan = false
    @ObservationIgnored private var toastSeq = 0
    /// An integrate job is running (set before its first event can arrive).
    private(set) var connecting = false

    static let startingNote = "Starting. Loading the model can take a minute."

    var chosen: Plan? { scan?.plans?.first { $0.id == chosenID } ?? scan?.recommended }
    var visiblePlans: [Plan] { Array((scan?.plans ?? []).prefix(14)) }
    var overall: Double { steps.reduce(0) { $0 + $1.weight * $1.fraction } }
    var currentModelID: String? { status?.deployed == true ? status?.config?.modelId : nil }
    var appBusy: Bool { connecting || appState.values.contains { $0.working } }

    var serverState: ServerState {
        if live?.healthy ?? status?.healthy ?? false { return .live }
        if live?.starting ?? status?.starting ?? false { return .loading }
        return status?.deployed == true ? .stopped : .notSetUp
    }

    // MARK: lifecycle

    func boot() async {
        screen = .booting
        retryScan = false
        engine.onExit = { [weak self] in Task { @MainActor in self?.engineDied() } }
        do {
            try await engine.start()
            engineGone = false
            // One event listener for the app's lifetime; it reconnects on its own after a restart.
            if !listening { listening = true; Task { await listen() } }
            var s: Status = try await engine.get("status")
            // Opening EZLM means you want your AI on: start it unless you stopped it yourself.
            if s.deployed && !s.healthy && !s.starting && !s.userStopped {
                do { try await engine.post("start"); s.starting = true } catch { debug("autostart: \(error)") }
            }
            status = s
            if s.deployed { enterMain() } else { await runScan() }
        } catch {
            debug("boot failed: \(error.localizedDescription)")
            fail(error.localizedDescription.components(separatedBy: "\n")[0], scan: false)
        }
    }

    private func listen() async {
        for await e in engine.events() { handle(e) }
    }

    private func fail(_ message: String, scan retry: Bool) {
        logo = .idle
        retryScan = retry
        screen = .failed(message)
    }

    func retryAfterFailure() async {
        if retryScan { await runScan() } else { await boot() }
    }

    private func engineDied() {
        guard !engineGone else { return }
        engineGone = true
        polling?.cancel()
        installSheet = false
        installRunning = false
        modelMode = nil
        // Jobs that were running died with the engine.
        updateBusy = false
        updateInstalling = false
        storageBusy = false
        launching = false
        setupDone = false
        fail("EZLM's background engine stopped unexpectedly. Your model server keeps running; Try again reconnects to it.", scan: false)
    }

    // MARK: scan

    func runScan() async {
        screen = .scanning
        logo = .busy
        scanLines = [
            ScanLine(id: "cpu", label: "Processor"),
            ScanLine(id: "mem", label: "System memory"),
            ScanLine(id: "gpu", label: "Graphics"),
            ScanLine(id: "disk", label: "Disk space"),
            ScanLine(id: "fit", label: "Comparing models against your memory"),
        ]
        do {
            async let r: ScanResult = engine.post("scan", ["pref": pref.rawValue])
            try await Task.sleep(nanoseconds: 700_000_000)
            let result = try await r
            let hw = result.hw
            let values: [String: String] = [
                "cpu": "\(hw.cpuName), \(hw.cores) threads",
                "mem": "\(gb(hw.ramAvail)) free of \(gb(hw.ramTotal))",
                "gpu": hw.unified ? "Shares system memory, up to \(gb(hw.gpuUsable)) for AI"
                    : (hw.gpus ?? []).isEmpty ? "No graphics card found, the processor will run it"
                    : (hw.gpus ?? []).map { "\($0.name), \(gb($0.memFree)) free of \(gb($0.memTotal))" }.joined(separator: "; "),
                "disk": "\(gb(hw.diskFree)) free",
                "fit": result.recommended != nil ? "\(result.plans?.count ?? 0) ways to run a model here" : "Nothing fits right now",
            ]
            for i in scanLines.indices {
                try await Task.sleep(nanoseconds: 160_000_000)
                withAnimation(.easeOut(duration: 0.2)) {
                    scanLines[i].value = values[scanLines[i].id] ?? ""
                    scanLines[i].state = scanLines[i].id == "fit" && result.recommended == nil ? .failed : .ok
                }
            }
            try await Task.sleep(nanoseconds: 300_000_000)
            debug("scan done: \(result.plans?.count ?? 0) plans, switching to pick")
            scan = result
            chosenID = result.recommended?.id
            // Switch screens without an animation transaction, then stop the
            // logo glow: animating the switch while the logo's timeline pauses
            // could leave SwiftUI holding a stale frame (seen ~40% of launches).
            screen = .pick
            try? await Task.sleep(nanoseconds: 50_000_000)
            logo = .idle
        } catch {
            debug("scan failed: \(error)")
            fail(error.localizedDescription, scan: true)
        }
    }

    func kill(_ p: Proc) async {
        do { try await engine.post("kill", ["pid": Int(p.pid)]) } catch { note = error.localizedDescription }
        if setupDone { await refreshScan() } else { await runScan() }
    }

    /// Re-scans quietly (no wizard animation) for the Models and System panes.
    func refreshScan() async {
        do {
            let r: ScanResult = try await engine.post("scan", ["pref": pref.rawValue])
            if let p = r.pref, p != pref.rawValue { return } // a newer request (other preference) is on its way
            scan = r
            let ids = Set(visiblePlans.map(\.id))
            if chosenID == nil || !ids.contains(chosenID ?? "") {
                chosenID = currentModelID.flatMap { ids.contains($0) ? $0 : nil } ?? r.recommended?.id
            }
        } catch {
            note = error.localizedDescription
        }
    }

    /// "Scan again": visibly starts over.
    func rescan() async {
        scan = nil
        await refreshScan()
    }

    /// Changing what the user values re-ranks models without leaving the screen.
    func setPref(_ p: Pref) async {
        guard p != pref, !prefBusy else { return }
        pref = p
        prefBusy = true
        await refreshScan()
        prefBusy = false
        chosenID = scan?.recommended?.id
    }

    func refreshInstalled() async {
        if let l: InstalledList = try? await engine.post("installed", ["pref": pref.rawValue]) { installed = l.models }
    }

    func refreshStatus() async {
        if let s: Status = try? await engine.get("status") { status = s }
    }

    // MARK: install / switch

    /// The wizard's button label for the picked model.
    var setUpLabel: String {
        guard let p = chosen else { return "Set it up" }
        return p.isDownloaded ? "Set it up (no download)" : p.partialBytes > 0 ? "Resume and set it up" : "Set it up"
    }

    /// The button on the selected card in Models.
    func switchAction(_ p: Plan) -> (label: String, enabled: Bool) {
        if p.id == currentModelID { return ("Running now", false) }
        if installRunning { return ("Busy with another change…", false) }
        if p.isDownloaded { return ("Switch to \(p.model.name) (no download)", true) }
        if p.partialBytes > 0 { return ("Resume download and switch (\(gb(p.download ?? 0)) left)", true) }
        return ("Download and switch (\(gb(p.downloadBytes)))", true)
    }

    /// Set up (wizard) or switch to (Models) a plan. A download that needs disk
    /// space EZLM would have to make is confirmed first: never delete silently.
    func requestDeploy(_ plan: Plan? = nil) {
        guard !installRunning, let p = plan ?? chosen else { return }
        if setupDone && p.id == currentModelID { return }
        if p.needsSpace, let hw = scan?.hw {
            makeRoom = MakeRoom(plan: p, message: makeRoomText(p, diskFree: hw.diskFree))
            return
        }
        Task { await deploy(p) }
    }

    private func makeRoomText(_ p: Plan, diskFree: UInt64) -> String {
        let need = p.downloadBytes + (2 << 30)
        var others: [String] = []
        for im in installed where !im.running && !others.contains(im.name) { others.append(im.name) }
        var s = "\(p.model.name) needs about \(gb(need)) and \(gb(diskFree)) is free.\n\n"
        if !others.isEmpty { s += "EZLM will first remove models you're not using (\(others.joined(separator: ", "))), oldest first. " }
        if status?.deployed == true, let cur = status?.config?.modelName {
            s += "If that's still not enough, it removes \(cur) too, and you'd be offline until the download finishes. "
        }
        return s + "You can also cancel and free up space yourself."
    }

    func deploy(_ p: Plan) async {
        guard !installRunning else { return }
        chosenID = p.id
        newSteps(quick: p.isDownloaded)
        installTitle = setupDone ? "Switching to \(p.model.name)" : "Setting up \(p.model.name)"
        prepareFill(name: p.model.name, gpu: p.mode == "cpu" ? 0 : p.gpuNeed, ram: p.ramNeed)
        await startJob { try await self.engine.post("deploy", ["model_id": p.id]) }
    }

    /// Switch to a model that's already downloaded: no download, just a restart.
    func use(_ im: InstalledModel) async {
        guard !installRunning else { return }
        newSteps(quick: true)
        chosenID = im.modelId
        installTitle = "Switching to \(im.name)"
        prepareFill(name: im.name, gpu: im.plan.map { $0.mode == "cpu" ? 0 : $0.gpuNeed } ?? 0, ram: im.plan?.ramNeed ?? 0)
        await startJob { try await self.engine.post("use", ["model_id": im.modelId, "quant": im.quant, "pref": self.pref.rawValue]) }
    }

    /// Set the running model up again on the graphics card, after it couldn't load it before.
    func retryGraphicsCard() async {
        guard !installRunning, let c = status?.config else { return }
        newSteps(quick: true)
        installTitle = "Trying your graphics card again"
        prepareFill(name: c.modelName, gpu: 0, ram: 0)
        await startJob { try await self.engine.post("use", ["model_id": c.modelId, "quant": c.quant, "pref": "balanced"]) }
    }

    /// Re-run the current model with the setup EZLM would pick today.
    func applyTune() async {
        guard !installRunning, let c = status?.config, let t = status?.tune else { return }
        newSteps(quick: true)
        installTitle = "Applying the faster setup"
        prepareFill(name: c.modelName, gpu: 0, ram: 0) // the new setup's numbers arrive with the live feed
        await startJob { try await self.engine.post("use", ["model_id": c.modelId, "quant": t.quant, "pref": "balanced"]) }
    }

    private func startJob(_ post: () async throws -> Void) async {
        installRunning = true // set first: the job's first events can arrive before the reply
        installInSheet = setupDone
        do {
            try await post()
        } catch {
            installRunning = false
            note = error.localizedDescription
            return
        }
        logo = .busy
        if installInSheet { installSheet = true } else { screen = .deploying }
    }

    private func newSteps(quick: Bool) {
        steps = quick ? [
            DeployStep(id: "engine", label: "Get the AI engine", weight: 0.05),
            DeployStep(id: "model", label: "Check the model files", weight: 0.05),
            DeployStep(id: "start", label: "Restart your server", weight: 0.2),
            DeployStep(id: "load", label: "Load the model into memory", weight: 0.5),
            DeployStep(id: "test", label: "Ask it a question", weight: 0.2),
        ] : [
            DeployStep(id: "engine", label: "Get the AI engine", weight: 0.08),
            DeployStep(id: "model", label: "Download the model", weight: 0.62),
            DeployStep(id: "start", label: "Start your server", weight: 0.04),
            DeployStep(id: "load", label: "Load the model into memory", weight: 0.16),
            DeployStep(id: "test", label: "Ask it a question", weight: 0.10),
        ]
        deployError = nil
        deployHint = ""
        cancelling = false
        installFinished = false
        installDoneText = ""
        note = ""
        reply = ""
        overallETA = ""
        overallStatus = "Starting"
    }

    private func prepareFill(name: String, gpu: UInt64, ram: UInt64) {
        fill = nil
        fillLast = nil
        preread = false
        fillName = name
        fillGpuNeed = gpu
        fillRamNeed = ram
    }

    private func stepIndex(_ id: String) -> Int? { steps.firstIndex { $0.id == id } }

    private func handle(_ e: EngineEvent) {
        switch e.job {
        case "deploy": handleDeploy(e)
        case "integrate": handleIntegrate(e)
        case "keys", "lan": handleRestartJob(e)
        case "test": handleTest(e)
        case "update": handleUpdate(e)
        case "storage": handleStorage(e)
        case "launch": handleLaunch(e)
        default: break
        }
    }

    private func handleDeploy(_ e: EngineEvent) {
        guard installRunning else { return } // a replay of an earlier job's events
        if e.step == "done" { finishDeploy(e); return }
        guard let i = stepIndex(e.step) else { return }
        if (e.step == "start" && e.state == "run") || e.step == "load" { ensureFill() }
        if e.step == "load", e.state == "progress", let t = e.total, t > 0, let d = e.done {
            fill?.warm(done: d, total: t)
            preread = true
        }
        if e.step == "load", e.state == "ok" { fill?.complete(fillLast) }
        switch e.state {
        case "progress":
            steps[i].state = .running
            if e.step == "load" && (e.total ?? 0) == 0 {
                // The server loading the model: seconds so far ("12s").
                let secs = Double(Int((e.msg ?? "").trimmingCharacters(in: CharacterSet(charactersIn: "s "))) ?? 0)
                let from = preread ? 0.5 : 0 // the pre-read filled the first half
                steps[i].fraction = max(steps[i].fraction, from + (0.9 - from) * (1 - exp(-secs / 25)))
                steps[i].showBar = true
                steps[i].indeterminate = true
                steps[i].stat = "\(Int(secs))s"
            } else if let total = e.total, total > 0, let done = e.done {
                let f = Double(done) / Double(total)
                steps[i].fraction = e.step == "load" ? max(steps[i].fraction, 0.5 * f) : f
                steps[i].showBar = true
                steps[i].indeterminate = false
                let bps = e.bps ?? 0
                let left = eta(done, total, bps)
                steps[i].stat = ["\(gb(done)) of \(gb(total))", bps > 0 ? String(format: "%.0f MB/s", bps / 1e6) : "", left]
                    .filter { !$0.isEmpty }.joined(separator: "   ")
                if e.step == "model" { overallETA = left }
            }
            if !cancelling { overallStatus = steps[i].label }
        case "run":
            steps[i].state = .running
            steps[i].sub = e.msg ?? ""
            if e.step == "start" || e.step == "test" { steps[i].showBar = true; steps[i].indeterminate = true }
            if !cancelling { overallStatus = steps[i].label }
        case "info":
            steps[i].sub = e.msg ?? ""
        case "ok":
            steps[i].state = .ok
            steps[i].fraction = 1
            steps[i].showBar = false
            steps[i].stat = ""
            steps[i].sub = e.step == "test" ? "Answered" : (e.msg ?? "")
            if e.step == "test" { reply = e.msg ?? "" }
            if e.step == "model" { overallETA = "" }
        case "fail":
            steps[i].state = .failed
            steps[i].showBar = false
            steps[i].sub = e.msg ?? ""
        default: break
        }
        // The pour panel shows the loading; the steps' own bars would repeat it.
        if fill != nil {
            for j in steps.indices where steps[j].id == "start" || steps[j].id == "load" { steps[j].showBar = false }
        }
        Dock.progress(overall)
    }

    private func finishDeploy(_ e: EngineEvent) {
        installRunning = false
        if e.state == "cancelled" { Dock.clear() } else { Dock.finished(ok: e.state == "ok") }
        switch e.state {
        case "cancelled":
            logo = .idle
            note = ""
            fill = nil
            showToast("Cancelled. What was already downloaded is kept: it picks up where it stopped next time."
                      + (installInSheet ? " Your model kept running." : ""), Theme.gold)
            if installInSheet {
                installSheet = false
                Task { await refreshAfterJob() }
            } else {
                screen = .pick
                Task { await refreshScan() } // the partial download now shows as "Resume"
            }
        case "ok":
            for i in steps.indices {
                steps[i].fraction = 1
                steps[i].state = .ok
                steps[i].showBar = false
            }
            overallStatus = "Done"
            overallETA = ""
            Task { await afterDeploy() }
        default:
            logo = .idle
            deployError = (e.msg ?? "").isEmpty ? "Setup failed." : e.msg
            debug("\(installTitle) failed: \(deployError ?? "")")
            overallStatus = "Stopped"
            overallETA = ""
            fill = nil
            Task {
                await refreshAfterJob()
                deployHint = failureHint()
            }
        }
    }

    private func afterDeploy() async {
        try? await Task.sleep(nanoseconds: 500_000_000)
        await refreshStatus()
        logo = .done
        if installInSheet {
            installDoneText = installTitle.hasPrefix("Applying")
                ? "Done. Your URL, keys and connected apps didn't change."
                : "Switched. Your URL, keys and connected apps didn't change."
            installFinished = true
            showToast("Now running \(status?.config?.modelName ?? fillName).", Theme.ok)
            await refreshInstalled()
            await refreshScan() // memory use changed: plans are stale
        } else {
            screen = .done
        }
    }

    private func refreshAfterJob() async {
        await refreshStatus()
        await refreshInstalled()
        Task { await refreshScan() }
    }

    /// Says what state a failed switch left the server in, from fresh status.
    private func failureHint() -> String {
        guard installInSheet else {
            return "Nothing was changed on your computer except what was downloaded, which is kept for next time."
        }
        let name = status?.config?.modelName ?? "Your model"
        if status?.healthy == true { return "\(name) is back and running, so your apps keep working." }
        if status?.starting == true { return "\(name) is back and loading; it'll answer in a minute." }
        return "Your server is stopped. Press Start server on Overview to bring it back."
    }

    func cancelInstall() async {
        guard installRunning, !cancelling else { return }
        cancelling = true
        overallStatus = "Cancelling…"
        do { try await engine.post("cancel") } catch { cancelling = false; note = error.localizedDescription }
    }

    func closeInstallSheet() {
        installSheet = false
        logo = .idle
        fill = nil
    }

    private func ensureFill() {
        guard fill == nil else { return }
        let f = MemoryFill(name: fillName.isEmpty ? "the model" : fillName)
        fill = f
        Task { await pollFill(f) }
    }

    /// Feeds the pour panel a memory sample every ~350 ms while the job runs.
    private func pollFill(_ f: MemoryFill) async {
        while installRunning && fill === f {
            if let l: Live = try? await engine.get("live") {
                fillLast = l
                f.feed(l, gpuNeed: fillGpuNeed, ramNeed: fillRamNeed)
            }
            try? await Task.sleep(nanoseconds: 350_000_000)
        }
    }

    private func eta(_ done: UInt64, _ total: UInt64, _ bps: Double) -> String {
        guard bps > 0, total > done else { return "" }
        let s = Double(total - done) / bps
        if s < 60 { return "\(Int(s.rounded(.up)))s left" }
        if s < 3600 { return "\(Int((s / 60).rounded(.up))) min left" }
        return String(format: "%.1f h left", s / 3600)
    }

    // MARK: main app

    /// Leaves the setup wizard for the main app.
    func enterMain() {
        setupDone = true
        screen = .done
        section = .overview
        startPolling()
        // Look for a newer EZLM once per launch; Overview shows a banner if there is one.
        if !updateCheckStarted {
            updateCheckStarted = true
            Task { await checkUpdate() }
        }
    }

    func finishSetup() { enterMain() }

    /// "Choose a model" when nothing is set up: back to the wizard.
    func backToSetup() async {
        polling?.cancel()
        setupDone = false
        await runScan()
    }

    func go(_ s: Section) {
        guard s != section else { return }
        section = s
        note = ""
    }

    func startPolling() {
        polling?.cancel()
        polling = Task { [weak self] in
            while !Task.isCancelled {
                guard let self else { return }
                await self.pollOnce()
                let l = self.live
                let ms: UInt64 = l?.starting == true ? 350 : ((l?.requests ?? 0) > 0 || self.ovFill != nil) ? 1000 : 2000
                try? await Task.sleep(nanoseconds: ms * 1_000_000)
            }
        }
    }

    private func pollOnce() async {
        guard let l: Live = try? await engine.get("live") else { return }
        let was = serverState
        live = l
        if l.activity == "writing" && l.tokPerSec > 0 { lastSpeed = l.tokPerSec }
        // First, so the pour finishes the moment the server is live: the status
        // refresh below can take seconds (the engine re-checks the hardware).
        updateOverviewFill(l)
        if was != serverState {
            // Server came up or went down: refresh once; otherwise numbers only.
            await refreshStatus()
            if serverState == .live && note == Self.startingNote { note = "" }
        }
        if section == .logs { await refreshLogs() }
    }

    /// The pour animation for loads that happen outside the install flow.
    private func updateOverviewFill(_ l: Live) {
        if serverState == .loading && !installRunning {
            if ovFill == nil || ovFillDoneAt != nil {
                ovFill = MemoryFill(name: status?.config?.modelName ?? "your model")
                ovFillDoneAt = nil
            }
            ovFill?.feed(l)
        } else if let f = ovFill {
            if let doneAt = ovFillDoneAt {
                if Date().timeIntervalSince(doneAt) > 5 { ovFill = nil; ovFillDoneAt = nil }
            } else {
                if serverState == .live { f.complete(l) }
                ovFillDoneAt = Date()
            }
        }
    }

    func startServer() async {
        note = Self.startingNote
        var l = live ?? Live()
        l.healthy = false
        l.starting = true
        l.phase = "idle"
        live = l // show "Loading model…" at once; the next poll has the real numbers
        do { try await engine.post("start") } catch { note = error.localizedDescription; live = nil }
    }

    func stopServer() async {
        do { try await engine.post("stop") } catch { note = error.localizedDescription }
        try? await Task.sleep(nanoseconds: 600_000_000)
        await refreshStatus()
        live = nil
        ovFill = nil
    }

    func remove(_ m: InstalledModel) async {
        do { try await engine.post("remove", ["model_id": m.modelId, "quant": m.quant]) } catch { note = error.localizedDescription }
        await refreshInstalled()
        await refreshScan()
    }

    func uninstall() async {
        do { try await engine.post("uninstall") } catch { note = error.localizedDescription; return }
        polling?.cancel()
        status = nil
        live = nil
        ovFill = nil
        installed = []
        scan = nil
        testResults = []
        setupDone = false
        await runScan()
    }

    // MARK: Model mode

    /// Walks through the apps using the most memory and closes the ones the user OKs.
    func startModelMode(_ origin: ModelMode.Origin) {
        guard modelMode == nil, !installRunning else { return }
        let mm = ModelMode(origin: origin, model: self)
        modelMode = mm
        Task { await mm.start() }
    }

    /// Closes Model mode, then what comes next: from the wizard it compares models
    /// again; anywhere else the plans are refreshed once memory has changed.
    func endModelMode(_ then: ModelModeExit = .none) {
        guard let mm = modelMode else { return }
        let changed = mm.closedCount > 0
        modelMode = nil
        switch mm.origin {
        case .wizard: Task { await runScan() }
        case .system: Task { await rescan() }
        case .overview: if changed { Task { await refreshScan() } }
        }
        switch then {
        case .applyTune:
            Task {
                try? await Task.sleep(nanoseconds: 350_000_000) // let this sheet go before the switch sheet opens
                await applyTune()
            }
        case .openModels:
            go(.models)
        case .none:
            break
        }
    }

    // MARK: tests

    func runTests() async {
        guard !testing else { return }
        testResults = []
        testing = true
        logo = .busy
        do { try await engine.post("test") } catch { testing = false; logo = .idle; note = error.localizedDescription }
    }

    private func handleTest(_ e: EngineEvent) {
        guard testing else { return }
        if e.step == "done" {
            testing = false
            logo = .idle
            if e.state == "fail" { note = e.msg ?? "" }
        } else if let r = e.data {
            withAnimation(.easeOut(duration: 0.25)) { testResults.append(r) }
        }
    }

    // MARK: connect

    func toggleApp(_ id: String) {
        guard !appBusy else { return }
        if selectedApps.contains(id) { selectedApps.remove(id) } else { selectedApps.insert(id) }
    }

    /// The tile's status line when nothing is in progress.
    func tileStatus(_ t: Target) -> AppTileState {
        if let s = appState[t.id] { return s }
        // An engine without states: Hermes shows as connected while the model has less than 64K.
        if (t.state ?? "").isEmpty, t.supported, t.connected == true, t.id == "hermes", status?.hermes64 != nil {
            return AppTileState(text: "Connected, but it needs a 64K context: connect again to give the model room", color: Theme.gold, working: false)
        }
        let line = t.stateLine
        return AppTileState(text: line.text, color: Self.color(line.tone), working: false)
    }

    static func color(_ tone: Target.Tone) -> Color {
        switch tone {
        case .ok: return Theme.ok
        case .dim: return Theme.dim
        case .warn: return Theme.gold
        case .bad: return Theme.bad
        }
    }

    /// Connect just this app again: for an app that needs 64K, it gives the model the room.
    func reconnect(_ id: String) async {
        guard !appBusy else { return }
        selectedApps = [id]
        await connectApps()
    }

    /// Hermes needs a 64K context: what that costs the running model, if anything.
    /// `blocked` means it can't have it here at all; `warn` colors it gold.
    func hermesTradeoff() -> (text: String, warn: Bool, blocked: Bool)? {
        guard let h = status?.hermes64 else { return nil }
        let name = status?.config?.modelName ?? "your model"
        if !h.fits {
            return ("Hermes needs a 64K context, which \(name) can't have on this computer. Switch to a smaller model to use it with Hermes.", true, true)
        }
        if h.tokPerSec >= 0.75 * h.nowTokPerSec {
            return ("Hermes needs a 64K context; EZLM gives the model the extra room when you connect (same speed).", false, false)
        }
        return ("Hermes needs a 64K context. With it, \(name) would write about \(max(1, Int(h.tokPerSec * 0.75))) words/s instead of \(max(1, Int(h.nowTokPerSec * 0.75))), for every app.", true, false)
    }

    func connectApps() async {
        guard !selectedApps.isEmpty, !appBusy else { return }
        let ids = selectedApps.sorted()
        for id in ids { appState[id] = AppTileState(text: "Waiting…", color: Theme.dim, working: true) }
        logo = .busy
        note = ""
        connecting = true
        do { try await engine.post("integrate", ["ids": ids, "default": makeDefault]) } catch {
            connecting = false
            note = error.localizedDescription
            appState.removeAll()
            logo = .idle
        }
    }

    private func handleIntegrate(_ e: EngineEvent) {
        guard connecting else { return } // a replay of an earlier job's events
        if e.step == "done" {
            connecting = false
            logo = e.state == "ok" ? .done : .idle
            let failedTile = appState.values.contains { $0.color == Theme.bad }
            note = e.state == "ok" ? "Done. Open the app and pick the model named EZLM."
                : e.state == "cancelled" ? "Cancelled." : failedTile ? "" : (e.msg ?? "")
            if e.state == "ok" { showToast("Connected. Your model now shows up in those apps as EZLM.", Theme.ok) }
            selectedApps.removeAll()
            Task {
                await refreshStatus()
                appState = appState.filter { !$0.value.working }
            }
            return
        }
        let id = e.step.replacingOccurrences(of: "integrate:", with: "")
        switch e.state {
        case "ok": appState[id] = AppTileState(text: "✓ Connected and answering", color: Theme.ok, working: false)
        case "fail": appState[id] = AppTileState(text: e.msg ?? "Failed", color: Theme.bad, working: false)
        default:
            let msg = (e.msg ?? "").trimmingCharacters(in: CharacterSet(charactersIn: "…"))
            appState[id] = AppTileState(text: msg + "…", color: Theme.dim, working: true)
        }
    }

    // MARK: keys

    /// Revoking the first key moves connected apps to the next one.
    func revokeMessage(_ k: APIKey) -> String {
        let keys = status?.config?.keys ?? []
        let apps = (status?.targets ?? []).filter { $0.connected == true }.map(\.name)
        guard keys.count > 1, keys.first?.id == k.id, !apps.isEmpty else { return "Anything using it stops working." }
        let who = apps.count == 1 ? "\(apps[0]) uses this key; EZLM switches it" : "\(apps.joined(separator: " and ")) use this key; EZLM switches them"
        return "\(who) to “\(keys[1].name)” for you. Anything else using it (scripts, other devices) stops working."
    }

    func keyAction(_ path: String, _ body: [String: Any], flash: String? = nil) async {
        guard !keyWork else { return }
        keyWork = true
        logo = .busy
        note = "Working… the server restarts so the change takes effect."
        flashKeyID = flash
        do { try await engine.post(path, body) } catch {
            keyWork = false
            logo = .idle
            note = error.localizedDescription
        }
    }

    private func handleRestartJob(_ e: EngineEvent) {
        guard keyWork else { return } // a replay of an earlier job's events
        guard e.step == "done" else {
            if let m = e.msg, !m.isEmpty { note = m }
            return
        }
        Task {
            let before = Set(status?.config?.keys?.map(\.id) ?? [])
            await refreshStatus()
            if flashKeyID == nil, let added = status?.config?.keys?.first(where: { !before.contains($0.id) }) {
                flashKeyID = added.id
            }
            keyWork = false
            logo = .idle
            note = e.state == "ok" ? "" : (e.msg ?? "")
            if e.state == "ok" {
                showToast(e.job == "lan"
                          ? (status?.config?.lan == true ? "Other devices on your network can use it now." : "Only this computer can use it now.")
                          : "Done. The server is back up with the new keys.", Theme.ok)
            }
            try? await Task.sleep(nanoseconds: 1_600_000_000)
            flashKeyID = nil
        }
    }

    // MARK: logs, support, toasts

    func refreshLogs() async {
        if let l: LogLines = try? await engine.get("logs"), l.lines != logs { logs = l.lines }
    }

    func copy(_ s: String) {
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(s, forType: .string)
    }

    /// Everything someone helping a tester needs, on the clipboard.
    func copyDiagnostics() async {
        let version = Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "dev"
        #if arch(arm64)
        let arch = "arm64"
        #else
        let arch = "x86_64"
        #endif
        func text(_ d: Data) -> String { String(decoding: d, as: UTF8.self).trimmingCharacters(in: .whitespacesAndNewlines) }
        var out = "EZLM \(version) on macOS \(ProcessInfo.processInfo.operatingSystemVersionString) (\(arch))\(Engine.demo ? ", demo" : "")\n"
        do { out += "status: " + text(try await engine.getRaw("status")) + "\n" } catch { out += "status: \(error.localizedDescription)\n" }
        var hw = scan?.hw
        if hw == nil, let r: ScanResult = try? await engine.post("scan", ["pref": pref.rawValue]) { hw = r.hw }
        let enc = JSONEncoder()
        enc.keyEncodingStrategy = .convertToSnakeCase
        enc.outputFormatting = .sortedKeys
        if let hw, let d = try? enc.encode(hw) { out += "hardware: " + text(d) + "\n" }
        if let d = try? await engine.getRaw("live") { out += "live: " + text(d) + "\n" }
        if let l: LogLines = try? await engine.get("logs") { out += "server log:\n" + l.lines.suffix(60).joined(separator: "\n") + "\n" }
        if let crash = CrashNote.latest() { out += crash + "\n" }
        copy(Self.hideKeys(out))
        showToast("Copied (your keys are hidden). Paste it in a message to whoever is helping you.", Theme.ok)
    }

    /// API keys never leave the computer in a support paste: "ezlm-AbCd…(hidden)".
    static func hideKeys(_ s: String) -> String {
        guard let re = try? NSRegularExpression(pattern: "ezlm-[A-Za-z0-9]{6,}") else { return s }
        let ns = s as NSString
        var out = "", last = 0
        for match in re.matches(in: s, range: NSRange(location: 0, length: ns.length)) {
            out += ns.substring(with: NSRange(location: last, length: match.range.location - last))
            out += String(ns.substring(with: match.range).prefix(9)) + "…(hidden)"
            last = match.range.location + match.range.length
        }
        return out + ns.substring(from: last)
    }

    /// A short confirmation at the bottom of the window; fades after ~4 s.
    func showToast(_ text: String, _ color: Color = Theme.ink) {
        toastSeq += 1
        let id = toastSeq
        withAnimation(.easeOut(duration: 0.2)) { toast = Toast(id: id, text: text, color: color) }
        Task {
            try? await Task.sleep(nanoseconds: 4_200_000_000)
            if toast?.id == id { withAnimation(.easeIn(duration: 0.4)) { toast = nil } }
        }
    }
}

/// Notes for the app's log (problem reports include its tail); printed too with EZLM_DEBUG.
func debug(_ s: String) {
    AppLog.shared.add(s)
    if ProcessInfo.processInfo.environment["EZLM_DEBUG"] != nil { FileHandle.standardError.write(Data((s + "\n").utf8)) }
}

/// The app's own log for this session, kept in memory (the last ~16 KB).
final class AppLog: @unchecked Sendable {
    static let shared = AppLog()
    private let lock = NSLock()
    private var text = ""
    private let clock: DateFormatter = {
        let f = DateFormatter()
        f.dateFormat = "HH:mm:ss"
        return f
    }()

    func add(_ s: String) {
        lock.lock()
        defer { lock.unlock() }
        text += "\(clock.string(from: Date())) \(s)\n"
        if text.utf8.count > 24_000 { text = String(text.suffix(16_000)) }
    }

    /// The last `n` characters.
    func tail(_ n: Int) -> String {
        lock.lock()
        defer { lock.unlock() }
        return String(text.suffix(n))
    }
}
