import Foundation

/// Runs the bundled Go engine (`ezlm engine`) and talks to its local API.
/// The engine exits on its own when this app quits, because it watches stdin.
final class Engine: @unchecked Sendable {
    private let lock = NSLock()
    private var process: Process?
    private var stdin: Pipe?
    private var _base = URL(string: "http://127.0.0.1")!
    private var _token = ""
    private var stopping = false

    /// Called (on a background thread) if the engine exits while the app is open.
    var onExit: (@Sendable () -> Void)?

    private func locked<T>(_ f: () -> T) -> T { lock.lock(); defer { lock.unlock() }; return f() }
    /// Address and token together: after a restart they change as a pair, and a
    /// request carrying the old address with the new token is refused.
    private var endpoint: (base: URL, token: String) { locked { (_base, _token) } }

    private let decoder: JSONDecoder = {
        let d = JSONDecoder()
        d.keyDecodingStrategy = .convertFromSnakeCase
        return d
    }()

    /// The connection to the engine. Tests pass one that answers without a process.
    private let session: URLSession

    init(session: URLSession = .shared) { self.session = session }

    /// Test seam: talk to an engine that is already listening, without starting one.
    func use(base: URL, token: String) { locked { _base = base; _token = token } }

    /// The engine binary: bundled in Resources, or EZLM_ENGINE for development.
    private static func binary() -> URL? {
        if let p = ProcessInfo.processInfo.environment["EZLM_ENGINE"] { return URL(fileURLWithPath: p) }
        return Bundle.main.url(forResource: "ezlm-engine", withExtension: nil)
    }

    static var demo: Bool {
        ProcessInfo.processInfo.environment["EZLM_DEMO"] != nil || CommandLine.arguments.contains("--demo")
    }

    func start() async throws {
        guard let bin = Self.binary() else { throw EngineError.message("EZLM's engine is missing from the app. Reinstall EZLM.") }
        stop() // a retry after the engine died: never leave two running
        let p = Process()
        p.executableURL = bin
        p.arguments = ["engine"] + (Self.demo ? ["--demo"] : [])
        let out = Pipe(), inp = Pipe(), err = Pipe()
        p.standardOutput = out
        p.standardInput = inp
        // What the engine says on stderr (a crash, mostly) goes to the app's log,
        // which problem reports include.
        p.standardError = err
        err.fileHandleForReading.readabilityHandler = { h in
            let d = h.availableData
            if d.isEmpty { h.readabilityHandler = nil; return }
            AppLog.shared.add("engine: " + String(decoding: d, as: UTF8.self).trimmingCharacters(in: .whitespacesAndNewlines))
        }
        p.terminationHandler = { [weak self] proc in
            guard let self else { return }
            let expected = self.locked { self.stopping || self.process !== proc }
            if !expected {
                AppLog.shared.add("engine exited unexpectedly (status \(proc.terminationStatus))")
                self.onExit?()
            }
        }
        locked { stopping = false }
        try p.run()
        locked { process = p; stdin = inp }

        // First stdout line is {"url": "...", "token": "..."}.
        let line: String = try await withCheckedThrowingContinuation { cont in
            let once = Once()
            DispatchQueue.global().asyncAfter(deadline: .now() + 30) {
                if once.claim() { cont.resume(throwing: EngineError.message("EZLM's engine didn't start within 30 seconds. Try again in a moment.")) }
            }
            DispatchQueue.global().async {
                var buf = Data()
                let h = out.fileHandleForReading
                while true {
                    let chunk = h.availableData
                    if chunk.isEmpty {
                        if once.claim() { cont.resume(throwing: EngineError.message("EZLM's engine stopped while starting.")) }
                        return
                    }
                    buf.append(chunk)
                    if let nl = buf.firstIndex(of: 0x0A) {
                        if once.claim() { cont.resume(returning: String(decoding: buf[..<nl], as: UTF8.self)) }
                        return
                    }
                }
            }
        }
        struct Hello: Decodable { let url: String; let token: String }
        let hello = try JSONDecoder().decode(Hello.self, from: Data(line.utf8))
        guard let url = URL(string: hello.url) else { throw EngineError.message("EZLM's engine sent an address it can't use.") }
        locked { _base = url; _token = hello.token }
    }

    func stop() {
        let (p, i) = locked { () -> (Process?, Pipe?) in
            stopping = true
            defer { process = nil; stdin = nil }
            return (process, stdin)
        }
        try? i?.fileHandleForWriting.close()
        if p?.isRunning == true { p?.terminate() }
    }

    /// `path` may carry a query ("update?force=1").
    private func request(_ path: String, body: [String: Any]?) -> URLRequest {
        let (base, token) = endpoint
        let parts = path.split(separator: "?", maxSplits: 1).map(String.init)
        var url = base.appendingPathComponent("api/" + (parts.first ?? ""))
        if parts.count > 1, var c = URLComponents(url: url, resolvingAgainstBaseURL: false) {
            c.percentEncodedQuery = parts[1]
            url = c.url ?? url
        }
        var r = URLRequest(url: url)
        r.setValue(token, forHTTPHeaderField: "X-EZLM-Token")
        r.timeoutInterval = 120
        if let body {
            r.httpMethod = "POST"
            r.setValue("application/json", forHTTPHeaderField: "Content-Type")
            r.httpBody = try? JSONSerialization.data(withJSONObject: body)
        }
        return r
    }

    func get<T: Decodable>(_ path: String, as: T.Type = T.self) async throws -> T {
        try await send(request(path, body: nil))
    }

    /// The response body as the engine sent it (for "Copy details for support").
    func getRaw(_ path: String) async throws -> Data {
        try await sendRaw(request(path, body: nil))
    }

    func post<T: Decodable>(_ path: String, _ body: [String: Any] = [:], as: T.Type = T.self) async throws -> T {
        try await send(request(path, body: body))
    }

    func post(_ path: String, _ body: [String: Any] = [:]) async throws {
        _ = try await sendRaw(request(path, body: body))
    }

    /// A GET gets one more try, because the engine may be restarting under it.
    /// A POST does not: the first one may have run, and running it twice is
    /// worse than showing the error.
    private func fetch(_ r: URLRequest) async throws -> (Data, URLResponse) {
        do {
            return try await session.data(for: r)
        } catch let e as URLError where r.httpMethod == nil && Self.retryable.contains(e.code) {
            try? await Task.sleep(nanoseconds: 250_000_000)
            do { return try await session.data(for: r) } catch { throw Self.friendly(error) }
        } catch {
            throw Self.friendly(error)
        }
    }

    private static let retryable: Set<URLError.Code> = [.cannotConnectToHost, .cannotFindHost, .networkConnectionLost]

    /// URLSession's own words ("Could not connect to the server.") are not
    /// EZLM's, and say nothing about what to do next.
    static func friendly(_ error: Error) -> Error {
        guard let e = error as? URLError else { return error }
        switch e.code {
        case .cancelled:
            return error
        case .cannotConnectToHost, .cannotFindHost, .networkConnectionLost, .notConnectedToInternet:
            return EngineError.message("EZLM's engine isn't reachable. Try again.")
        case .timedOut:
            return EngineError.message("EZLM's engine didn't answer in time. Try again.")
        default:
            return EngineError.message("EZLM's engine didn't answer. Try again.")
        }
    }

    private func sendRaw(_ r: URLRequest) async throws -> Data {
        let (data, resp) = try await fetch(r)
        if let h = resp as? HTTPURLResponse, h.statusCode != 200 {
            throw EngineError.message((try? JSONDecoder().decode(ErrorBody.self, from: data))?.error ?? "Request failed (\(h.statusCode))")
        }
        return data
    }

    private func send<T: Decodable>(_ r: URLRequest) async throws -> T {
        try decoder.decode(T.self, from: try await sendRaw(r))
    }

    /// Live job events (server-sent events), reconnecting if the stream drops
    /// (and to the new address after the engine restarts).
    func events() -> AsyncStream<EngineEvent> {
        AsyncStream { cont in
            let task = Task {
                var backoff = Backoff()
                while !Task.isCancelled {
                    let (base, token) = endpoint
                    var r = URLRequest(url: base.appendingPathComponent("api/events"))
                    r.setValue(token, forHTTPHeaderField: "X-EZLM-Token")
                    // The engine pings every 15 seconds. Silence for a minute and a
                    // half means the socket died (a sleep, a dropped connection),
                    // and the stream has to be opened again.
                    r.timeoutInterval = 90
                    let started = Date()
                    if let (bytes, _) = try? await session.bytes(for: r) {
                        do {
                            for try await line in bytes.lines where line.hasPrefix("data: ") {
                                if let e = try? decoder.decode(EngineEvent.self, from: Data(line.dropFirst(6).utf8)) {
                                    cont.yield(e)
                                }
                            }
                        } catch {}
                    }
                    // A stream that stayed up a while starts again from the
                    // shortest wait; one that keeps failing backs off.
                    if Date().timeIntervalSince(started) > 5 { backoff.reset() }
                    if Task.isCancelled { break }
                    try? await Task.sleep(nanoseconds: UInt64(backoff.next() * 1_000_000_000))
                }
            }
            cont.onTermination = { _ in task.cancel() }
        }
    }
}

/// The wait before trying the engine again: half a second, doubling, up to
/// eight. An engine that is gone, or that quits at every start, is not asked
/// twice a second forever.
struct Backoff {
    private(set) var attempt = 0
    static let first: TimeInterval = 0.5
    static let ceiling: TimeInterval = 8

    mutating func next() -> TimeInterval {
        let wait = min(Self.first * pow(2, Double(attempt)), Self.ceiling)
        attempt = min(attempt + 1, 16)
        return wait
    }

    mutating func reset() { attempt = 0 }
}

/// Lets exactly one of several racing callbacks resume a continuation.
private final class Once: @unchecked Sendable {
    private let lock = NSLock()
    private var done = false
    func claim() -> Bool { lock.lock(); defer { lock.unlock() }; if done { return false }; done = true; return true }
}

private struct ErrorBody: Decodable { let error: String }

enum EngineError: LocalizedError {
    case message(String)
    var errorDescription: String? { if case .message(let m) = self { return m }; return nil }
}
