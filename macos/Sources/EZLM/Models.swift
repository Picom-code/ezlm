import Foundation

// Mirrors of the engine's JSON (see internal/app, internal/fit, internal/scan).
// Decoded with .convertFromSnakeCase. Fields added after the first release are
// optional or decoded with defaults, so a newer engine never crashes an older app.

struct GPU: Codable, Hashable {
    var name: String
    var vendor: String
    var memTotal: UInt64
    var memFree: UInt64
    var driver: String?
}

struct Proc: Codable, Hashable, Identifiable {
    var pid: Int32
    var name: String
    var mem: UInt64
    var killable: Bool
    var id: Int32 { pid }
}

struct Hardware: Codable {
    var os: String
    var osName: String?
    var arch: String
    var cpuName: String
    var cores: Int
    var ramTotal: UInt64
    var ramAvail: UInt64
    var diskFree: UInt64
    var modelsDir: String?
    var gpus: [GPU]?
    var backend: String
    var unified: Bool
    var gpuUsable: UInt64
    var gpuProcs: [Proc]?
    var topMem: [Proc]?
    var notes: [String]?

    var gpuTotal: UInt64 { (gpus ?? []).reduce(0) { $0 + $1.memTotal } }
    var gpuFree: UInt64 { (gpus ?? []).reduce(0) { $0 + $1.memFree } }
    var machineWord: String { os == "darwin" ? "Mac" : os == "windows" ? "PC" : "computer" }
}

struct Model: Codable, Hashable {
    var id: String
    var name: String
    var repo: String?
    var vendor: String?
    var country: String?
    var paramsB: Double
    var activeParamsB: Double?
    var moe: Bool
    var score: Double
    var layers: Int
    var toolCalling: Bool
}

struct Quant: Codable, Hashable {
    var name: String
    var bytes: UInt64
    var repo: String?
    var mtp: Bool?
}

struct Plan: Codable, Hashable, Identifiable {
    var model: Model
    var quant: Quant
    var ctx: Int
    var kv: String?
    var spec: String?
    /// Bytes still to fetch: 0 when the files are already here, the remainder
    /// when a cancelled download left part of it on disk.
    var download: UInt64?
    var effective: Double?
    var mode: String
    var ngl: Int
    var nCpuMoe: Int
    var gpuNeed: UInt64
    var gpuBudget: UInt64
    var ramNeed: UInt64
    var ramBudget: UInt64
    var tokPerSec: Double
    var speed: String
    var diskOk: Bool
    /// The model files are already complete on this computer.
    var downloaded: Bool?
    /// Bytes of a cancelled download already on disk.
    var partial: UInt64?
    var id: String { model.id }
    var words: Int { max(1, Int((tokPerSec * 0.75).rounded())) }
    /// How much it can keep in mind, and whether it works with coding agents: a card's at-a-glance facts.
    var ctxLine: String { "\(ctx / 1024)k context" + (model.toolCalling ? ", works with coding agents" : ", chat only") }
}

extension Plan {
    var isDownloaded: Bool { downloaded ?? false }
    var partialBytes: UInt64 { partial ?? 0 }
    /// What a download would fetch (the whole file if the engine didn't say).
    var downloadBytes: UInt64 { (download ?? 0) > 0 ? download ?? 0 : quant.bytes }
    /// Only a download needs free disk; files already here never do.
    var needsSpace: Bool { !diskOk && !isDownloaded }

    /// The "Download" fact: nothing, the rest of a resumed download, or the whole file.
    var downloadText: String {
        if isDownloaded { return "Already on this computer" }
        if partialBytes > 0 { return "\(gb(download ?? 0)) left (\(gb(partialBytes)) already here)" }
        return gb(downloadBytes)
    }

    /// "IQ3_XXS by ISTA-DASLab, MTP" — which file, who made it, and tuning.
    var quantLine: String {
        let who = (quant.repo ?? model.repo ?? "").split(separator: "/").first.map(String.init) ?? ""
        var parts = [quant.name.replacingOccurrences(of: "GSQ-RCO-", with: "GSQ ")]
        if !who.isEmpty { parts[0] += " by \(who)" }
        if let s = spec, !s.isEmpty { parts.append(s == "draft-mtp" ? "MTP speed-up" : "draft model speed-up") }
        if let k = kv, k != "f16" { parts.append("\(k) cache") }
        return parts.joined(separator: ", ")
    }
}

struct ScanResult: Codable {
    var pref: String?
    var hw: Hardware
    var plans: [Plan]?
    var recommended: Plan?
    var unlock: Plan?
    var unlockFreeGb: Double?
    var unlockGpu: Bool?
}

struct APIKey: Codable, Hashable, Identifiable {
    var id: String
    var name: String
    var key: String
}

struct ServerConfig: Codable {
    var key: String
    var keys: [APIKey]?
    var port: Int
    var lan: Bool
    var modelId: String
    var modelName: String
    var quant: String
    var ctx: Int
    var cpuFallback: String?  // why it runs on the processor (the graphics card couldn't load it)
}

/// An app EZLM connects to (internal/integrate Target). Everything after
/// `connected` came later and is optional, so an older engine still decodes.
struct Target: Codable, Hashable, Identifiable {
    var id: String
    var name: String
    var desc: String?
    var supported: Bool
    var why: String?
    var installed: Bool?
    var connected: Bool?
    var icon: String?        // logo key: Logos/app-<icon>.png
    var version: String?     // "2.2.5"; empty until probed
    var bin: String?         // the program EZLM opens
    var source: String?      // "ezlm" (EZLM's own copy), "msix", "script", "npm", "path"
    var state: String?       // see Target.State
    var detail: String?      // one sentence for the tile
    var config: String?      // the config file EZLM edits
    var minCtx: Int?
    var launch: [LaunchMode]?
    var lastCheck: CheckResult?

    enum State {
        static let notInstalled = "not_installed", installed = "installed", connected = "connected"
        static let needsUpdate = "needs_update", needs64K = "needs_64k", needsTools = "needs_tools", broken = "broken"
    }
}

/// What a Connect tile and the launch buttons say about an app (same words on Windows and the web page).
extension Target {
    enum Tone { case ok, dim, warn, bad }

    /// The engine's state; an older engine sends none, so it comes from the two flags.
    var current: String {
        if let s = state, !s.isEmpty { return s }
        return connected == true ? State.connected : installed == true ? State.installed : State.notInstalled
    }

    var logoKey: String { (icon ?? "").isEmpty ? id : icon ?? id }

    /// "2.2.5 · installed by EZLM" or "1.19.5 · yours"; nil before the version is known.
    var versionLine: String? {
        guard let v = version, !v.isEmpty else { return nil }
        return source == "ezlm" ? "\(v) · installed by EZLM" : "\(v) · yours"
    }

    /// The buttons that open it, main one first.
    var modes: [LaunchMode] {
        let l = (launch ?? []).filter { !$0.label.isEmpty }
        return l.isEmpty ? [LaunchMode(id: "tui", label: "Open \(name)")] : l
    }

    /// The tile's status line when nothing is in progress.
    var stateLine: (text: String, tone: Tone) {
        if !supported { return ((why ?? "").isEmpty ? "Not available on this computer" : why ?? "", .dim) }
        let d = (detail ?? "").isEmpty ? lastCheck?.msg ?? "" : detail ?? ""
        switch current {
        case State.connected: return ("✓ Connected", .ok)
        case State.installed: return ("Installed, not connected yet", .dim)
        case State.needsUpdate: return ("Points at an old address or key: EZLM fixes it when you open it", .warn)
        case State.needs64K:
            return (d.isEmpty ? "\(name) needs a 64K context. Connect again to give the model the room (it may run slower), or pick a smaller model." : d, .warn)
        case State.needsTools:
            return (d.isEmpty ? "This model can't use tools, so \(name) can't edit files with it. Pick a model marked Tools, or use Chat." : d, .warn)
        case State.broken:
            let head = lastCheck?.stage == "launch" ? "Couldn't open last time" : "The last check failed"
            return (d.isEmpty ? head + "." : "\(head): \(d)", .bad)
        default: return ("Not installed: EZLM will install it", .dim)
        }
    }

    /// "Checked 2 min ago: answered and used a tool (14 s)" after a check that passed.
    func checkedLine(now: Date = Date()) -> String? {
        guard let c = lastCheck, c.ok, let msg = c.msg, !msg.isEmpty, let at = c.date else { return nil }
        return "Checked \(ChatStore.ago(at, now: now)): \(msg)"
    }

    var broken: Bool { current == State.broken }
}

/// One way to open an app; `label` is the whole button text.
struct LaunchMode: Codable, Hashable, Identifiable {
    var id: String
    var label: String
}

/// The newest check or launch of an app.
struct CheckResult: Codable, Hashable {
    var at: String?
    var ok = false
    var toolCall = false
    var secs = 0.0
    var stage: String?   // "model", "preflight", "tui", "launch": on failure, the one that failed
    var msg: String?     // ok: reads after "Checked 2 min ago: "; failed: the reason
    var logPath: String?

    /// `at` as a date. Go writes RFC 3339 with up to nine fraction digits, which
    /// ISO8601DateFormatter doesn't always take, so the fraction is dropped.
    var date: Date? {
        guard var s = at, !s.isEmpty else { return nil }
        if let r = s.range(of: #"\.\d+"#, options: .regularExpression) { s.removeSubrange(r) }
        return ISO8601DateFormatter().date(from: s)
    }
}

/// A faster way to run the model that's already running.
struct Tune: Codable, Hashable {
    var why = ""
    var quant = ""
}

/// What a bigger (64K) context would cost the running model; Hermes needs 64K.
struct CtxOption: Codable, Hashable {
    var fits = false
    var tokPerSec = 0.0
    var nowTokPerSec = 0.0
}

struct Status: Codable {
    var deployed = false
    var healthy = false
    /// The server process is up but still loading the model.
    var starting = false
    /// The user pressed Stop: never start it on their behalf.
    var userStopped = false
    var config: ServerConfig?
    var localUrl: String?
    var lanUrl: String?
    var busy: String?
    var tune: Tune?
    var hermes64: CtxOption?
    /// The server answers but EZLM can't see or stop its process (started with
    /// administrator rights): stopping and switching won't work.
    var unmanaged = false
    /// Why the last start failed (cleared by the next start).
    var startError: String?
    /// The engine's version, e.g. "0.1.1-beta".
    var version: String?
    var targets: [Target] = []
}

struct EngineEvent: Codable {
    var job: String
    var step: String
    var state: String
    var msg: String?
    var done: UInt64?
    var total: UInt64?
    var bps: Double?
    /// Only test results are read from `data`; other jobs put other shapes
    /// there (e.g. server status on "done"), which must never make the whole
    /// event undecodable, or the app would miss that the job finished.
    var data: TestResult?

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        job = try c.decode(String.self, forKey: .job)
        step = try c.decode(String.self, forKey: .step)
        state = try c.decode(String.self, forKey: .state)
        msg = try? c.decodeIfPresent(String.self, forKey: .msg)
        done = try? c.decodeIfPresent(UInt64.self, forKey: .done)
        total = try? c.decodeIfPresent(UInt64.self, forKey: .total)
        bps = try? c.decodeIfPresent(Double.self, forKey: .bps)
        data = try? c.decodeIfPresent(TestResult.self, forKey: .data)
    }
}

struct TestResult: Codable, Hashable {
    var name: String
    var pass: Bool
    var detail: String
    var value: Double?
    var secs: Double
}

let GB: Double = 1_073_741_824

/// "129 MB" below 1 GB, then "2.4 GB", then "63 GB".
func gb(_ b: UInt64) -> String {
    let mb = Double(b) / 1_048_576
    if mb < 1023.5 { return String(format: "%.0f MB", mb) }
    let v = Double(b) / GB
    return v < 10 ? String(format: "%.1f GB", v) : String(format: "%.0f GB", v)
}

// MARK: - Updates, model storage, problem reports (GET /api/update, /api/storage, POST /api/report)

/// A newer EZLM release and the installer for this computer.
struct UpdateRelease: Codable, Hashable {
    var version = ""
    var name = ""
    var notes = ""
    var page = ""
    var asset = ""
    var url = ""
    var size: UInt64 = 0
}

struct UpdateInfo: Codable {
    var current = ""
    var available = false
    var latest: UpdateRelease?
    var error: String?
}

/// A place models can go: this Mac's own disk, or another drive.
struct StorageDrive: Codable, Hashable, Identifiable {
    var path = ""
    var label = ""
    var free: UInt64 = 0
    var total: UInt64 = 0
    var current = false
    var id: String { path }
}

struct StorageInfo: Codable {
    /// Where new downloads go.
    var dir = ""
    var custom = false
    /// Free space there.
    var free: UInt64 = 0
    /// EZLM's models on this computer, wherever they are.
    var modelsBytes: UInt64 = 0
    /// Of which outside `dir`.
    var elsewhere: UInt64 = 0
    var drives: [StorageDrive] = []
}

struct ReportResult: Codable {
    /// Filed as a GitHub issue (#number); otherwise `url` is a filled-in new-issue page.
    var sent = false
    var url = ""
    var number = 0
}

/// A cheap snapshot of memory and what the server is doing (GET /api/live).
struct Live: Codable {
    var ramTotal: UInt64 = 0
    var ramAvail: UInt64 = 0
    var gpuTotal: UInt64 = 0
    var gpuFree: UInt64 = 0
    var unified = false
    var healthy = false
    /// The server process is up but still loading the model.
    var starting = false
    var tokPerSec = 0.0
    var requests = 0
    var tokens: UInt64 = 0
    var uptimeS: Int64 = 0
    /// Requests waiting for the current one to finish.
    var queued = 0
    /// "idle", "reading" (the prompt) or "writing" (the reply); empty while not live.
    var phase = ""
    var promptDone = 0
    var promptTotal = 0
    var written = 0
    /// While starting: bytes of the model read into memory so far (the pre-read).
    var loadDone: UInt64 = 0
    var loadTotal: UInt64 = 0
    /// How much of each memory the current setup is planned to take.
    var gpuPlan: UInt64 = 0
    var ramPlan: UInt64 = 0

    var activity: String { phase.isEmpty ? "idle" : phase }
}

struct LogLines: Codable { var lines: [String] }

func uptimeText(_ s: Int64) -> String {
    if s < 60 { return "\(s)s" }
    if s < 3600 { return "\(s / 60)m" }
    if s < 86400 { return "\(s / 3600)h \((s % 3600) / 60)m" }
    return "\(s / 86400)d \((s % 86400) / 3600)h"
}

enum Pref: String, CaseIterable, Identifiable {
    case balanced, smartest, fastest
    var id: String { rawValue }
    var title: String { rawValue.capitalized }
    var blurb: String {
        switch self {
        case .balanced: return "Smart, at a comfortable speed"
        case .smartest: return "Most capable model that fits, even if slow"
        case .fastest: return "Snappy replies first"
        }
    }
}

struct InstalledModel: Codable, Hashable, Identifiable {
    var modelId: String
    var name: String
    var vendor: String?
    var country: String?
    var quant: String
    var bytes: UInt64
    var running: Bool
    var plan: Plan?
    var id: String { modelId + "/" + quant }
}

struct InstalledList: Codable { var models: [InstalledModel] }

// MARK: - Lenient decoding for fields newer engines added

extension Tune {
    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        why = (try? c.decodeIfPresent(String.self, forKey: .why)) ?? ""
        quant = (try? c.decodeIfPresent(String.self, forKey: .quant)) ?? ""
    }
}

extension CtxOption {
    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        fits = (try? c.decodeIfPresent(Bool.self, forKey: .fits)) ?? false
        tokPerSec = (try? c.decodeIfPresent(Double.self, forKey: .tokPerSec)) ?? 0
        nowTokPerSec = (try? c.decodeIfPresent(Double.self, forKey: .nowTokPerSec)) ?? 0
    }
}

extension Status {
    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        deployed = (try? c.decodeIfPresent(Bool.self, forKey: .deployed)) ?? false
        healthy = (try? c.decodeIfPresent(Bool.self, forKey: .healthy)) ?? false
        starting = (try? c.decodeIfPresent(Bool.self, forKey: .starting)) ?? false
        userStopped = (try? c.decodeIfPresent(Bool.self, forKey: .userStopped)) ?? false
        config = try c.decodeIfPresent(ServerConfig.self, forKey: .config)
        localUrl = try? c.decodeIfPresent(String.self, forKey: .localUrl)
        lanUrl = try? c.decodeIfPresent(String.self, forKey: .lanUrl)
        busy = try? c.decodeIfPresent(String.self, forKey: .busy)
        tune = try? c.decodeIfPresent(Tune.self, forKey: .tune)
        hermes64 = try? c.decodeIfPresent(CtxOption.self, forKey: .hermes64)
        unmanaged = (try? c.decodeIfPresent(Bool.self, forKey: .unmanaged)) ?? false
        startError = try? c.decodeIfPresent(String.self, forKey: .startError)
        version = try? c.decodeIfPresent(String.self, forKey: .version)
        targets = (try? c.decodeIfPresent([Target].self, forKey: .targets)) ?? []
    }
}

extension Target {
    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(String.self, forKey: .id)
        name = (try? c.decodeIfPresent(String.self, forKey: .name)) ?? id
        supported = (try? c.decodeIfPresent(Bool.self, forKey: .supported)) ?? false
        func o<T: Decodable>(_ k: CodingKeys) -> T? { (try? c.decodeIfPresent(T.self, forKey: k)) ?? nil }
        desc = o(.desc); why = o(.why); installed = o(.installed); connected = o(.connected)
        icon = o(.icon); version = o(.version); bin = o(.bin); source = o(.source); state = o(.state)
        detail = o(.detail); config = o(.config); minCtx = o(.minCtx); launch = o(.launch); lastCheck = o(.lastCheck)
    }
}

extension CheckResult {
    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        func v<T: Decodable>(_ k: CodingKeys, _ d: T) -> T { (try? c.decodeIfPresent(T.self, forKey: k)) ?? d }
        at = v(.at, nil); ok = v(.ok, false); toolCall = v(.toolCall, false); secs = v(.secs, 0)
        stage = v(.stage, nil); msg = v(.msg, nil); logPath = v(.logPath, nil)
    }
}

extension UpdateRelease {
    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        func v<T: Decodable>(_ k: CodingKeys, _ d: T) -> T { (try? c.decodeIfPresent(T.self, forKey: k)) ?? d }
        version = v(.version, ""); name = v(.name, ""); notes = v(.notes, ""); page = v(.page, "")
        asset = v(.asset, ""); url = v(.url, ""); size = v(.size, 0)
    }
}

extension UpdateInfo {
    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        current = (try? c.decodeIfPresent(String.self, forKey: .current)) ?? ""
        available = (try? c.decodeIfPresent(Bool.self, forKey: .available)) ?? false
        latest = try? c.decodeIfPresent(UpdateRelease.self, forKey: .latest)
        error = try? c.decodeIfPresent(String.self, forKey: .error)
    }
}

extension StorageDrive {
    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        func v<T: Decodable>(_ k: CodingKeys, _ d: T) -> T { (try? c.decodeIfPresent(T.self, forKey: k)) ?? d }
        path = v(.path, ""); label = v(.label, ""); free = v(.free, 0); total = v(.total, 0); current = v(.current, false)
    }
}

extension StorageInfo {
    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        func v<T: Decodable>(_ k: CodingKeys, _ d: T) -> T { (try? c.decodeIfPresent(T.self, forKey: k)) ?? d }
        dir = v(.dir, ""); custom = v(.custom, false); free = v(.free, 0)
        modelsBytes = v(.modelsBytes, 0); elsewhere = v(.elsewhere, 0); drives = v(.drives, [])
    }
}

extension ReportResult {
    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        func v<T: Decodable>(_ k: CodingKeys, _ d: T) -> T { (try? c.decodeIfPresent(T.self, forKey: k)) ?? d }
        sent = v(.sent, false); url = v(.url, ""); number = v(.number, 0)
    }
}

extension Live {
    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        func v<T: Decodable>(_ k: CodingKeys, _ d: T) -> T { (try? c.decodeIfPresent(T.self, forKey: k)) ?? d }
        ramTotal = v(.ramTotal, 0)
        ramAvail = v(.ramAvail, 0)
        gpuTotal = v(.gpuTotal, 0)
        gpuFree = v(.gpuFree, 0)
        unified = v(.unified, false)
        healthy = v(.healthy, false)
        starting = v(.starting, false)
        tokPerSec = v(.tokPerSec, 0)
        requests = v(.requests, 0)
        tokens = v(.tokens, 0)
        uptimeS = v(.uptimeS, 0)
        queued = v(.queued, 0)
        phase = v(.phase, "")
        promptDone = v(.promptDone, 0)
        promptTotal = v(.promptTotal, 0)
        written = v(.written, 0)
        loadDone = v(.loadDone, 0)
        loadTotal = v(.loadTotal, 0)
        gpuPlan = v(.gpuPlan, 0)
        ramPlan = v(.ramPlan, 0)
    }
}
