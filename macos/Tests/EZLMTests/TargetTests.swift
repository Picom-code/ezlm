import XCTest
@testable import EZLM

/// The apps EZLM connects to, as the engine sends them: every field after
/// `connected` is optional, and the tile and button words match Windows and the web page.
final class TargetTests: XCTestCase {
    private func decode(_ json: String) throws -> [Target] {
        let d = JSONDecoder()
        d.keyDecodingStrategy = .convertFromSnakeCase
        return try d.decode(Status.self, from: Data(#"{"targets":\#(json)}"#.utf8)).targets
    }

    // The example in internal/integrate/CONTRACT.md, section 5.
    private let full = #"""
    {"id":"opencode","name":"OpenCode","desc":"Coding agent for your terminal","supported":true,"why":"",
     "installed":true,"connected":true,"icon":"opencode","version":"2.2.5","bin":"C:\\Users\\Sam\\.ezlm\\tools\\opencode\\2.2.5\\opencode.exe",
     "source":"ezlm","state":"connected","config":"C:\\Users\\Sam\\.config\\opencode\\opencode.json","min_ctx":0,
     "launch":[{"id":"tui","label":"Open OpenCode"}],
     "last_check":{"at":"2026-11-05T21:02:26.123456789Z","ok":true,"tool_call":true,"secs":14,"stage":"model","msg":"answered and used a tool (14 s)","log_path":"C:\\Users\\Sam\\.ezlm\\logs\\integrate-opencode.log"}}
    """#

    func testDecodesTheContractExample() throws {
        let t = try XCTUnwrap(try decode("[\(full)]").first)
        XCTAssertEqual(t.icon, "opencode")
        XCTAssertEqual(t.version, "2.2.5")
        XCTAssertEqual(t.source, "ezlm")
        XCTAssertEqual(t.state, "connected")
        XCTAssertEqual(t.minCtx, 0)
        XCTAssertEqual(t.launch, [LaunchMode(id: "tui", label: "Open OpenCode")])
        let c = try XCTUnwrap(t.lastCheck)
        XCTAssertTrue(c.ok)
        XCTAssertTrue(c.toolCall)
        XCTAssertEqual(c.secs, 14)
        XCTAssertEqual(c.stage, "model")
        XCTAssertTrue(c.logPath?.hasSuffix("integrate-opencode.log") == true)
        // Go's nanoseconds don't stop the date from reading.
        XCTAssertEqual(c.date, ISO8601DateFormatter().date(from: "2026-11-05T21:02:26Z"))
        XCTAssertEqual(CheckResult(at: "2026-11-05T15:02:26-06:00").date, c.date)
        XCTAssertNil(CheckResult(at: "yesterday").date)
    }

    // What the merged engine really sent (ezlm engine --demo with
    // EZLM_DEMO_APPS=opencode=connected,hermes=broken,dsh=needs_tools): local
    // times with an offset and microseconds.
    private let engineDemo = #"""
    [{"id":"opencode","name":"OpenCode","desc":"Coding agent for your terminal","supported":true,"why":"","installed":true,"connected":true,"icon":"opencode","version":"2.2.5","bin":"C:\\Users\\Sam\\.ezlm\\tools\\opencode\\2.2.5\\opencode.exe","source":"ezlm","state":"connected","config":"C:\\Users\\Sam\\.config\\opencode\\opencode.json","launch":[{"id":"tui","label":"Open OpenCode"}],"last_check":{"at":"2026-11-05T23:23:22.836563-06:00","ok":true,"tool_call":true,"secs":14,"stage":"model","msg":"answered and used a tool (14 s)","log_path":"C:\\Users\\Sam\\.ezlm\\logs\\integrate-opencode.log"}},{"id":"hermes","name":"Hermes","desc":"Personal agent with tools and memory","supported":true,"why":"","installed":true,"connected":false,"icon":"hermes","version":"0.22.5","bin":"C:\\Users\\Sam\\AppData\\Local\\Microsoft\\WindowsApps\\hermes.exe","source":"msix","state":"broken","detail":"Hermes closed right after opening (exit code 1). Press Report a problem to send the details.","config":"C:\\Users\\Sam\\AppData\\Local\\hermes\\config.yaml","min_ctx":64000,"launch":[{"id":"tui","label":"Open Hermes"}],"last_check":{"at":"2026-11-05T23:18:22.836563-06:00","ok":false,"tool_call":false,"secs":0,"stage":"launch","msg":"Hermes closed right after opening (exit code 1). Press Report a problem to send the details.","log_path":"C:\\Users\\Sam\\.ezlm\\logs\\integrate-hermes.log"}},{"id":"dsh","name":"DeepSeek Harness","desc":"DeepSeek's agent harness, in your browser","supported":true,"why":"","installed":true,"connected":false,"icon":"dsh","version":"0.2.1-rc.5","bin":"C:\\Users\\Sam\\.ezlm\\tools\\dsh\\0.2.1-rc.5\\node_modules\\.bin\\dsh.cmd","source":"ezlm","state":"needs_tools","detail":"This model can't use tools, so DeepSeek Harness can't edit files with it. Pick a model marked Tools, or use Chat.","config":"C:\\Users\\Sam\\.dsh\\profiles\\web\\cordis.patch.yml","launch":[{"id":"web","label":"Open DeepSeek Harness (browser)"}]}]
    """#

    func testDecodesTheEngine() throws {
        let ts = try decode(engineDemo)
        XCTAssertEqual(ts.map(\.id), ["opencode", "hermes", "dsh"])
        XCTAssertEqual(ts.map(\.current), ["connected", "broken", "needs_tools"])
        XCTAssertEqual(ts[1].minCtx, 64000)
        XCTAssertEqual(ts[1].lastCheck?.stage, "launch")
        XCTAssertNotNil(ts[0].lastCheck?.date)
        XCTAssertNotNil(ts[1].lastCheck?.date)
        XCTAssertEqual(ts[2].modes, [LaunchMode(id: "web", label: "Open DeepSeek Harness (browser)")])
        XCTAssertEqual(ts[2].versionLine, "0.2.1-rc.5 · installed by EZLM")
        XCTAssertTrue(ts[1].stateLine.text.hasPrefix("Couldn't open last time: Hermes closed right after opening"))
    }

    func testOlderEngineAndOddFields() throws {
        // 0.1.x sends only the first seven fields.
        let old = try decode(#"[{"id":"opencode","name":"OpenCode","desc":"d","supported":true,"why":"","installed":true,"connected":false},{"id":"hermes","name":"Hermes","desc":"d","supported":true,"why":"","installed":false,"connected":false}]"#)
        XCTAssertEqual(old.map(\.current), ["installed", "not_installed"])
        XCTAssertEqual(old[0].stateLine.text, "Installed, not connected yet")
        XCTAssertEqual(old[1].stateLine.text, "Not installed: EZLM will install it")
        XCTAssertEqual(old[0].modes, [LaunchMode(id: "tui", label: "Open OpenCode")])
        XCTAssertNil(old[0].versionLine)
        XCTAssertNil(old[0].checkedLine())
        // A field of the wrong shape is dropped, not the whole list.
        let odd = try decode(#"[{"id":"dsh","name":"DeepSeek Harness","supported":true,"version":7,"launch":"web","last_check":[],"state":"installed"}]"#)
        XCTAssertEqual(odd.count, 1)
        XCTAssertNil(odd[0].version)
        XCTAssertNil(odd[0].lastCheck)
        XCTAssertEqual(odd[0].modes.first?.label, "Open DeepSeek Harness")
        XCTAssertEqual(odd[0].logoKey, "dsh")
    }

    func testStateWords() {
        func t(_ state: String, detail: String? = nil, check: CheckResult? = nil) -> Target {
            Target(id: "dsh", name: "DeepSeek Harness", supported: true, state: state, detail: detail, lastCheck: check)
        }
        XCTAssertEqual(t("connected").stateLine.text, "✓ Connected")
        XCTAssertEqual(t("connected").stateLine.tone, .ok)
        XCTAssertEqual(t("installed").stateLine.text, "Installed, not connected yet")
        XCTAssertEqual(t("not_installed").stateLine.text, "Not installed: EZLM will install it")
        XCTAssertEqual(t("needs_update").stateLine.text, "Points at an old address or key: EZLM fixes it when you open it")
        XCTAssertEqual(t("needs_update").stateLine.tone, .warn)
        XCTAssertEqual(t("needs_64k", detail: "Hermes needs a 64K context.").stateLine.text, "Hermes needs a 64K context.")
        XCTAssertEqual(t("needs_tools", detail: "This model can't use tools.").stateLine.text, "This model can't use tools.")
        XCTAssertTrue(t("needs_64k").stateLine.text.hasPrefix("DeepSeek Harness needs a 64K context."))
        let launch = CheckResult(ok: false, stage: "launch", msg: "DeepSeek Harness closed right after opening (exit code 1).")
        XCTAssertEqual(t("broken", detail: launch.msg, check: launch).stateLine.text,
                       "Couldn't open last time: DeepSeek Harness closed right after opening (exit code 1).")
        XCTAssertEqual(t("broken", check: launch).stateLine.text, // no detail: the check's reason
                       "Couldn't open last time: DeepSeek Harness closed right after opening (exit code 1).")
        XCTAssertEqual(t("broken", detail: "The model didn't answer.", check: CheckResult(stage: "model")).stateLine.text,
                       "The last check failed: The model didn't answer.")
        XCTAssertEqual(t("broken").stateLine.tone, .bad)
        XCTAssertTrue(t("broken").broken)
        var off = t("connected")
        off.supported = false
        off.why = "DeepSeek Harness doesn't run on this computer."
        XCTAssertEqual(off.stateLine.text, "DeepSeek Harness doesn't run on this computer.")
        off.why = ""
        XCTAssertEqual(off.stateLine.text, "Not available on this computer")
    }

    func testVersionCheckAndModes() {
        var t = Target(id: "opencode", name: "OpenCode", supported: true, version: "2.2.5", source: "ezlm")
        XCTAssertEqual(t.versionLine, "2.2.5 · installed by EZLM")
        t.version = "1.19.5"; t.source = "path"
        XCTAssertEqual(t.versionLine, "1.19.5 · yours")
        t.source = nil
        XCTAssertEqual(t.versionLine, "1.19.5 · yours")

        let now = ISO8601DateFormatter().date(from: "2026-11-05T21:04:26Z")!
        t.lastCheck = CheckResult(at: "2026-11-05T21:02:26Z", ok: true, toolCall: true, secs: 14, stage: "model", msg: "answered and used a tool (14 s)")
        XCTAssertEqual(t.checkedLine(now: now), "Checked 2 min ago: answered and used a tool (14 s)")
        t.lastCheck?.at = "2026-11-05T21:04:00Z"
        XCTAssertEqual(t.checkedLine(now: now), "Checked just now: answered and used a tool (14 s)")
        t.lastCheck?.ok = false
        XCTAssertNil(t.checkedLine(now: now)) // a failure shows in the state line instead

        let dsh = Target(id: "dsh", name: "DeepSeek Harness", supported: true, icon: "dsh",
                         launch: [LaunchMode(id: "web", label: "Open DeepSeek Harness (browser)"), LaunchMode(id: "tui", label: "Open in terminal"), LaunchMode(id: "x", label: "")])
        XCTAssertEqual(dsh.modes.map(\.label), ["Open DeepSeek Harness (browser)", "Open in terminal"])
    }

    func testAppLogos() {
        // Only plain keys reach the file system.
        XCTAssertNil(Logos.app(""))
        XCTAssertNil(Logos.app("../deepseek"))
        XCTAssertNil(Logos.app("no-such-app"))
        // The files are in the repo, next to the vendor logos.
        let logos = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent().appendingPathComponent("Logos")
        for k in ["opencode", "hermes", "dsh"] {
            let img = NSImage(contentsOf: logos.appendingPathComponent("app-\(k).png"))
            XCTAssertNotNil(img, k)
            XCTAssertEqual(img?.representations.first?.pixelsWide, 512, k)
        }
    }

    func testAttributionMatchesNotice() throws {
        let notice = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent().appendingPathComponent("Logos/NOTICE.md")
        let text = try String(contentsOf: notice, encoding: .utf8)
        XCTAssertTrue(text.contains(Attribution.apps), "Settings › About must carry the line from Logos/NOTICE.md word for word")
        XCTAssertFalse(Attribution.apps.contains("—"))
    }
}
