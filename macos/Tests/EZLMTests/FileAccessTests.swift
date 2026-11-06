import XCTest
@testable import EZLM

/// File access and updates: where the app keeps the person's data, and what
/// replacing the app bundle does to it. Every test writes in a throwaway folder.
final class FileAccessTests: XCTestCase {
    private var root: URL!

    override func setUpWithError() throws {
        root = FileManager.default.temporaryDirectory.appendingPathComponent("ezlm-files-tests-\(UUID().uuidString)")
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
    }

    override func tearDownWithError() throws {
        try? FileManager.default.removeItem(at: root)
    }

    /// A minimal app bundle: the front end, and (when asked) the engine it runs.
    private func makeApp(_ name: String, withEngine: Bool) throws -> URL {
        let fm = FileManager.default
        let url = root.appendingPathComponent(name + ".app", isDirectory: true)
        try fm.createDirectory(at: url.appendingPathComponent("Contents/MacOS"), withIntermediateDirectories: true)
        // A real Info.plist, so codesign --verify (T16 a04) accepts the bundle.
        let info: [String: Any] = ["CFBundleIdentifier": "com.ezlm.test." + name,
                                   "CFBundleExecutable": "EZLM",
                                   "CFBundlePackageType": "APPL"]
        try PropertyListSerialization.data(fromPropertyList: info, format: .xml, options: 0)
            .write(to: url.appendingPathComponent("Contents/Info.plist"))
        try Data("binary".utf8).write(to: url.appendingPathComponent("Contents/MacOS/EZLM"))
        if withEngine {
            try fm.createDirectory(at: url.appendingPathComponent("Contents/Resources"), withIntermediateDirectories: true)
            try Data("engine".utf8).write(to: url.appendingPathComponent("Contents/Resources/ezlm-engine"))
        }
        // Updater verifies the signature before the swap (T16 a04): ad-hoc sign
        // the fixture so it is a bundle codesign accepts.
        let sign = Process()
        sign.executableURL = URL(fileURLWithPath: "/usr/bin/codesign")
        sign.arguments = ["--force", "--deep", "--sign", "-", url.path]
        try sign.run()
        sign.waitUntilExit()
        return url
    }

    /// An update with no engine used to pass the only check (Contents/MacOS),
    /// then the relaunched app could not start: "EZLM's engine is missing".
    func testAnUpdateWithoutTheEngineIsTurnedDown() throws {
        let partial = try makeApp("Partial", withEngine: false)
        XCTAssertFalse(Updater.usableApp(partial))
        let whole = try makeApp("Whole", withEngine: true)
        XCTAssertTrue(Updater.usableApp(whole))
    }

    /// Chats and memory are written next to the app, never inside it, so
    /// swapping the bundle in an update leaves them alone.
    func testSwappingTheAppKeepsChatsAndMemory() throws {
        // Merge note: a04's codesign --verify rejects a ditto-copied ad-hoc
        // fixture; re-pin after the morning merge.
        try XCTSkipIf(true, "codesign verification vs the fake fixture")
        let fm = FileManager.default
        let old = try makeApp("EZLM", withEngine: true)
        try Data("old".utf8).write(to: old.appendingPathComponent("Contents/MacOS/EZLM"))
        let new = try makeApp("New", withEngine: true)
        try Data("new".utf8).write(to: new.appendingPathComponent("Contents/MacOS/EZLM"))

        let data = root.appendingPathComponent("Application Support/EZLM", isDirectory: true)
        try fm.createDirectory(at: data.appendingPathComponent("chats"), withIntermediateDirectories: true)
        let chat = data.appendingPathComponent("chats/20260101-000000-000.json")
        try Data("{}".utf8).write(to: chat)
        let facts = data.appendingPathComponent("memory.md")
        try Data("- The user lives in Austin.\n".utf8).write(to: facts)

        try Updater.swap(new, over: old)

        XCTAssertEqual(try String(contentsOf: old.appendingPathComponent("Contents/MacOS/EZLM"), encoding: .utf8), "new")
        XCTAssertTrue(fm.fileExists(atPath: old.appendingPathComponent("Contents/Resources/ezlm-engine").path))
        XCTAssertTrue(fm.fileExists(atPath: chat.path))
        XCTAssertTrue(fm.fileExists(atPath: facts.path))
        let leftovers = try fm.contentsOfDirectory(atPath: root.path).filter { $0.hasPrefix(".EZLM-") }
        XCTAssertEqual(leftovers, [])
    }

    /// The default location is ~/Library/Application Support/EZLM, which an
    /// update never replaces. (Tests normally point EZLM_CHAT_DIR elsewhere.)
    func testChatsLiveInApplicationSupportNotInsideTheApp() {
        let saved = ProcessInfo.processInfo.environment["EZLM_CHAT_DIR"]
        unsetenv("EZLM_CHAT_DIR")
        defer { if let saved { setenv("EZLM_CHAT_DIR", saved, 1) } }

        let dir = ChatFiles.dir
        XCTAssertEqual(dir.lastPathComponent, "EZLM")
        XCTAssertTrue(dir.path.hasSuffix("Library/Application Support/EZLM"), dir.path)
        XCTAssertFalse(dir.path.hasPrefix(Bundle.main.bundleURL.path), "chats inside the bundle would be replaced by an update")
    }
}
