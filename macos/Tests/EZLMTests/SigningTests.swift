import XCTest
@testable import EZLM

/// What the updater must check before it replaces a working app: an app copied
/// out of a quarantined disk image is quarantined too, and a broken copy is
/// still a bundle with the right ID.
final class SigningTests: XCTestCase {
    private let fm = FileManager.default

    // MARK: - A fake EZLM.app and a disk image around it

    private func makeApp(at url: URL, marker: String) throws {
        let contents = url.appendingPathComponent("Contents", isDirectory: true)
        let macos = contents.appendingPathComponent("MacOS", isDirectory: true)
        let resources = contents.appendingPathComponent("Resources", isDirectory: true)
        try fm.createDirectory(at: macos, withIntermediateDirectories: true)
        try fm.createDirectory(at: resources, withIntermediateDirectories: true)
        let exe = macos.appendingPathComponent("EZLM")
        try "#!/bin/sh\nexit 0\n".write(to: exe, atomically: true, encoding: .utf8)
        try fm.setAttributes([.posixPermissions: 0o755], ofItemAtPath: exe.path)
        let plist = """
        <?xml version="1.0" encoding="UTF-8"?>
        <!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
        <plist version="1.0"><dict>
          <key>CFBundleIdentifier</key><string>com.ezlm.app</string>
          <key>CFBundleExecutable</key><string>EZLM</string>
        </dict></plist>
        """
        try plist.write(to: contents.appendingPathComponent("Info.plist"), atomically: true, encoding: .utf8)
        try marker.write(to: resources.appendingPathComponent("marker.txt"), atomically: true, encoding: .utf8)
        // usableApp also requires the engine (T16 a12): an update without it
        // would replace a working app with one that cannot start.
        let engine = resources.appendingPathComponent("ezlm-engine")
        try "#!/bin/sh\nexit 0\n".write(to: engine, atomically: true, encoding: .utf8)
        try fm.setAttributes([.posixPermissions: 0o755], ofItemAtPath: engine.path)
        // build-app.sh signs ad-hoc by default; the test does the same.
        try sh("/usr/bin/codesign", "--force", "--deep", "--sign", "-", url.path)
    }

    /// A disk image with EZLM.app at the top, the shape make-dmg.sh produces.
    private func makeDMG(_ app: URL, quarantined: Bool) throws -> URL {
        let stage = fm.temporaryDirectory.appendingPathComponent("dmg-stage-\(UUID().uuidString)")
        try fm.createDirectory(at: stage, withIntermediateDirectories: true)
        defer { try? fm.removeItem(at: stage) }
        try fm.copyItem(at: app, to: stage.appendingPathComponent("EZLM.app"))
        let dmg = fm.temporaryDirectory.appendingPathComponent("EZLM-\(UUID().uuidString).dmg")
        try sh("/usr/bin/hdiutil", "create", "-volname", "EZLM", "-srcfolder", stage.path,
               "-ov", "-format", "UDZO", "-fs", "HFS+", dmg.path)
        if quarantined {
            // What a browser download looks like.
            try sh("/usr/bin/xattr", "-w", "com.apple.quarantine", "0081;00000000;Safari;", dmg.path)
        }
        return dmg
    }

    @discardableResult
    private func sh(_ tool: String, _ args: String...) throws -> String {
        let p = Process()
        p.executableURL = URL(fileURLWithPath: tool)
        p.arguments = args
        let out = Pipe()
        p.standardOutput = out
        p.standardError = out
        try p.run()
        let data = out.fileHandleForReading.readDataToEndOfFile()
        p.waitUntilExit()
        let text = String(decoding: data, as: UTF8.self)
        guard p.terminationStatus == 0 else {
            throw NSError(domain: "SigningTests", code: Int(p.terminationStatus),
                          userInfo: [NSLocalizedDescriptionKey: text])
        }
        return text
    }

    private func quarantine(_ url: URL) -> String? {
        (try? sh("/usr/bin/xattr", "-p", "com.apple.quarantine", url.path))?
            .trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private func marker(_ app: URL) throws -> String {
        try String(contentsOf: app.appendingPathComponent("Contents/Resources/marker.txt"), encoding: .utf8)
    }

    private func tempDir() throws -> URL {
        let dir = fm.temporaryDirectory.appendingPathComponent("update-\(UUID().uuidString)")
        try fm.createDirectory(at: dir, withIntermediateDirectories: true)
        addTeardownBlock { try? FileManager.default.removeItem(at: dir) }
        return dir
    }

    // MARK: - Tests

    func testInstallFromQuarantinedImageLeavesNoQuarantine() throws {
        let dir = try tempDir()
        let new = dir.appendingPathComponent("new/EZLM.app")
        try fm.createDirectory(at: new.deletingLastPathComponent(), withIntermediateDirectories: true)
        try makeApp(at: new, marker: "new")
        let dmg = try makeDMG(new, quarantined: true)
        let old = dir.appendingPathComponent("EZLM.app")
        try makeApp(at: old, marker: "old")

        try Updater.install(dmg: dmg, over: old)

        XCTAssertEqual(try marker(old), "new")
        // Copying out of a quarantined image marks the copy, and Gatekeeper
        // then refuses an ad-hoc signed app on its next launch.
        XCTAssertNil(quarantine(old))
    }

    func testInstallRejectsBrokenCopyAndKeepsTheOldApp() throws {
        let dir = try tempDir()
        let new = dir.appendingPathComponent("new/EZLM.app")
        try fm.createDirectory(at: new.deletingLastPathComponent(), withIntermediateDirectories: true)
        try makeApp(at: new, marker: "new")
        // Change a sealed file after signing, as a truncated copy would.
        let exe = new.appendingPathComponent("Contents/MacOS/EZLM")
        let h = try FileHandle(forWritingTo: exe)
        try h.seekToEnd()
        try h.write(contentsOf: Data("\n".utf8))
        try h.close()
        let dmg = try makeDMG(new, quarantined: false)
        let old = dir.appendingPathComponent("EZLM.app")
        try makeApp(at: old, marker: "old")

        XCTAssertThrowsError(try Updater.install(dmg: dmg, over: old))
        XCTAssertEqual(try marker(old), "old")
    }
}
