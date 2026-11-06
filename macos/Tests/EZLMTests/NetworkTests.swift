import XCTest
@testable import EZLM

/// The engine client: how long it waits between tries, and what it tells the
/// person when the engine is not there. Answers come from a stub, so no
/// process and no port are involved.
final class NetworkTests: XCTestCase {
    // MARK: the wait between tries

    func testTheWaitDoublesUpToEightSeconds() {
        var b = Backoff()
        var waits: [TimeInterval] = []
        for _ in 0..<7 { waits.append(b.next()) }
        XCTAssertEqual(waits, [0.5, 1, 2, 4, 8, 8, 8])
    }

    func testAStreamThatStayedUpStartsAgainFromTheShortestWait() {
        var b = Backoff()
        for _ in 0..<4 { _ = b.next() }
        b.reset()
        XCTAssertEqual(b.next(), 0.5)
    }

    func testTheWaitNeverOverflowsOrGoesPastTheCeiling() {
        var b = Backoff()
        for _ in 0..<200 { XCTAssertLessThanOrEqual(b.next(), Backoff.ceiling) }
        XCTAssertEqual(b.next(), Backoff.ceiling)
    }

    // MARK: what the person is told

    func testAnUnreachableEngineSaysWhatToDo() {
        let e = Engine.friendly(URLError(.cannotConnectToHost))
        XCTAssertEqual((e as? EngineError)?.errorDescription, "EZLM's engine isn't reachable. Try again.")
        XCTAssertEqual((Engine.friendly(URLError(.networkConnectionLost)) as? EngineError)?.errorDescription,
                       "EZLM's engine isn't reachable. Try again.")
        XCTAssertEqual((Engine.friendly(URLError(.timedOut)) as? EngineError)?.errorDescription,
                       "EZLM's engine didn't answer in time. Try again.")
    }

    func testCancellingIsLeftAlone() {
        // The caller checks for it: a stopped request is not a failure to show.
        XCTAssertTrue(Engine.friendly(URLError(.cancelled)) is URLError)
    }

    // MARK: the request path

    func testTheEngineSaysWhyItRefusedAndThatReachesThePerson() async {
        let engine = stubEngine(status: 400, body: #"{"error":"No model is running."}"#)
        do {
            let _: Status = try await engine.get("status")
            XCTFail("a 400 has to throw")
        } catch {
            XCTAssertEqual(error.localizedDescription, "No model is running.")
        }
    }

    func testAConnectionFailureIsReportedInPlainWords() async {
        let engine = stubEngine(failure: URLError(.cannotConnectToHost))
        do {
            let _: Status = try await engine.get("status")
            XCTFail("a dead engine has to throw")
        } catch {
            XCTAssertEqual(error.localizedDescription, "EZLM's engine isn't reachable. Try again.")
        }
    }

    func testTheTokenAndThePathReachTheEngine() async throws {
        let engine = stubEngine(status: 200, body: #"{"ok":true}"#)
        let _: [String: Bool] = try await engine.get("status")
        let seen = try XCTUnwrap(Stub.seen.last)
        XCTAssertEqual(seen.path, "/api/status")
        XCTAssertEqual(seen.token, "REDACTED-KEY")
    }

    func testAQueryStringSurvivesThePath() async throws {
        let engine = stubEngine(status: 200, body: #"{"lines":[]}"#)
        struct Lines: Decodable { let lines: [String] }
        let _: Lines = try await engine.get("integrate/log?id=Qwen3-8B")
        XCTAssertEqual(try XCTUnwrap(Stub.seen.last).path, "/api/integrate/log?id=Qwen3-8B")
    }

    private func stubEngine(status: Int, body: String) -> Engine {
        Stub.reply = .init(status: status, body: Data(body.utf8), failure: nil)
        return stubEngine()
    }

    private func stubEngine(failure: URLError) -> Engine {
        Stub.reply = .init(status: 0, body: Data(), failure: failure)
        return stubEngine()
    }

    private func stubEngine() -> Engine {
        let c = URLSessionConfiguration.ephemeral
        c.protocolClasses = [Stub.self]
        let engine = Engine(session: URLSession(configuration: c))
        engine.use(base: URL(string: "http://127.0.0.1:9")!, token: "REDACTED-KEY")
        return engine
    }
}

/// Answers every request with what the test set, and remembers what it was asked.
private final class Stub: URLProtocol {
    struct Reply {
        let status: Int
        let body: Data
        let failure: URLError?
    }
    struct Seen {
        let path: String
        let token: String
    }

    nonisolated(unsafe) static var reply = Reply(status: 200, body: Data(), failure: nil)
    nonisolated(unsafe) static var seen: [Seen] = []

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        let url = request.url ?? URL(string: "http://127.0.0.1:9")!
        Self.seen.append(Seen(path: url.path + (url.query.map { "?" + $0 } ?? ""),
                              token: request.value(forHTTPHeaderField: "X-EZLM-Token") ?? ""))
        if let f = Self.reply.failure {
            client?.urlProtocol(self, didFailWithError: f)
            return
        }
        let resp = HTTPURLResponse(url: url, statusCode: Self.reply.status, httpVersion: "HTTP/1.1",
                                   headerFields: ["Content-Type": "application/json"])!
        client?.urlProtocol(self, didReceive: resp, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: Self.reply.body)
        client?.urlProtocolDidFinishLoading(self)
    }

    override func stopLoading() {}
}
