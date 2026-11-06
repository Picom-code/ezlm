import AppKit

/// Puts a downloaded EZLM update in place of the running app, then restarts it.
///
/// The old app is only removed once the new one is fully copied and renamed
/// into its place; if any step fails, the running app stays exactly where it was.
enum Updater {
    struct Failure: LocalizedError {
        let message: String
        var errorDescription: String? { message }
    }

    /// Replaces the app bundle at `app` with the EZLM.app inside the disk image `dmg`.
    /// Opened from the download there is no writable bundle to replace, so the new
    /// copy is installed into Applications instead (see `destination(for:)`).
    static func install(dmg: URL, over running: URL) throws {
        guard running.pathExtension == "app" else { throw Failure(message: "EZLM isn't running from an app (\(running.path))") }
        let app = destination(for: running)
        let fm = FileManager.default
        let mount = fm.temporaryDirectory.appendingPathComponent("EZLM-update-\(UUID().uuidString)", isDirectory: true)
        try fm.createDirectory(at: mount, withIntermediateDirectories: true)
        defer { try? fm.removeItem(at: mount) } // runs last, after the detach below
        try run("/usr/bin/hdiutil", "attach", "-nobrowse", "-readonly", "-noautoopen", "-mountpoint", mount.path, dmg.path)
        defer { detach(mount) }

        let new = mount.appendingPathComponent("EZLM.app", isDirectory: true)
        guard usableApp(new) else {
            throw Failure(message: "The update doesn't contain EZLM.app")
        }
        if let want = bundleID(app) ?? bundleID(running), let got = bundleID(new), want != got {
            throw Failure(message: "The update is a different app (\(got))")
        }
        try swap(new, over: app)
    }

    /// Whether `app` is a whole EZLM: the front end and the engine it runs.
    /// An update with only one of them would replace a working app with one
    /// that can't start ("EZLM's engine is missing from the app").
    static func usableApp(_ app: URL) -> Bool {
        let fm = FileManager.default
        return fm.fileExists(atPath: app.appendingPathComponent("Contents/MacOS").path)
            && fm.fileExists(atPath: app.appendingPathComponent("Contents/Resources/ezlm-engine").path)
    }

    /// Copies `new` next to `app` and swaps it in. If anything fails, `app`
    /// is left exactly as it was. The person's chats and memory are outside
    /// the bundle (see ChatFiles), so replacing it never touches them.
    static func swap(_ new: URL, over app: URL) throws {
        let fm = FileManager.default
        // Copy next to the running app under a temporary name (same folder, so the swap is a rename).
        let dir = app.deletingLastPathComponent()
        let tag = ProcessInfo.processInfo.processIdentifier
        let staged = dir.appendingPathComponent(".EZLM-update-\(tag).app", isDirectory: true)
        let old = dir.appendingPathComponent(".EZLM-old-\(tag).app", isDirectory: true)
        try? fm.removeItem(at: staged)
        try? fm.removeItem(at: old)
        do { try run("/usr/bin/ditto", new.path, staged.path) } catch {
            try? fm.removeItem(at: staged)
            throw error
        }
        // A disk image downloaded in a browser is quarantined, and copying the
        // app out of it marks the copy too. An ad-hoc signed app that carries
        // the flag is blocked by Gatekeeper on its next launch, so clear it.
        _ = try? run("/usr/bin/xattr", "-dr", "com.apple.quarantine", staged.path)
        // The copy must still be a valid bundle before the old one goes: a
        // truncated copy would be moved into place and never launch.
        do { try run("/usr/bin/codesign", "--verify", "--deep", "--strict", staged.path) } catch {
            try? fm.removeItem(at: staged)
            throw error
        }

        // Swap: old aside, new in; if the new one can't go in, the old one goes back.
        // From the download there may be no old copy in Applications yet.
        var hadOld = false
        if fm.fileExists(atPath: app.path) {
            do { try fm.moveItem(at: app, to: old); hadOld = true } catch {
                try? fm.removeItem(at: staged)
                throw error
            }
        }
        do { try fm.moveItem(at: staged, to: app) } catch {
            if hadOld { try? fm.moveItem(at: old, to: app) }
            try? fm.removeItem(at: staged)
            throw error
        }
        if hadOld, (try? fm.removeItem(at: old)) == nil { try? fm.trashItem(at: old, resultingItemURL: nil) }
    }

    /// Where a new copy belongs. Running from the download (the disk image, or
    /// macOS's hidden copy of it) the running bundle is read-only and goes away,
    /// so the update goes into Applications and the app reopens from there.
    static func destination(for running: URL, readOnly: Bool? = nil) -> URL {
        guard AppPlace.isTemporary(path: running.path, readOnly: readOnly) else { return running }
        return URL(fileURLWithPath: "/Applications/EZLM.app", isDirectory: true)
    }

    /// Quits and opens the app at `app` again once this process has exited.
    /// Returns false (and doesn't quit) if the relaunch couldn't be arranged.
    @MainActor
    static func relaunch(_ app: URL) -> Bool {
        let app = destination(for: app)
        let p = Process()
        p.executableURL = URL(fileURLWithPath: "/bin/sh")
        // Wait for this process to actually exit, however long the person takes
        // with the quit prompt; opening before then would run two copies, and
        // both would write the same chats.
        p.arguments = ["-c", "sleep 1; while kill -0 \"$1\" 2>/dev/null; do sleep 0.25; done; open \"$2\"",
                       "sh", String(ProcessInfo.processInfo.processIdentifier), app.path]
        do { try p.run() } catch { return false }
        NSApp.terminate(nil)
        return true
    }

    private static func bundleID(_ app: URL) -> String? {
        NSDictionary(contentsOf: app.appendingPathComponent("Contents/Info.plist"))?["CFBundleIdentifier"] as? String
    }

    private static func detach(_ mount: URL) {
        if (try? run("/usr/bin/hdiutil", "detach", mount.path)) == nil {
            _ = try? run("/usr/bin/hdiutil", "detach", "-force", mount.path)
        }
    }

    @discardableResult
    private static func run(_ tool: String, _ args: String...) throws -> String {
        let p = Process()
        p.executableURL = URL(fileURLWithPath: tool)
        p.arguments = args
        let out = Pipe()
        p.standardOutput = out
        p.standardError = out
        try p.run()
        let data = out.fileHandleForReading.readDataToEndOfFile() // before waiting: a full pipe would block it
        p.waitUntilExit()
        let text = String(decoding: data, as: UTF8.self).trimmingCharacters(in: .whitespacesAndNewlines)
        guard p.terminationStatus == 0 else {
            throw Failure(message: "\(URL(fileURLWithPath: tool).lastPathComponent) failed: \(text.isEmpty ? "status \(p.terminationStatus)" : text)")
        }
        return text
    }
}

/// Opened straight from the disk image, or from Downloads (macOS then runs a
/// hidden read-only copy, "App Translocation"): EZLM works, but it won't be
/// there after a restart, and it can't update itself in place.
enum AppPlace {
    static let temporary: Bool = isTemporary(path: Bundle.main.bundleURL.path)

    /// Whether the app at `path` is somewhere it can't stay: a read-only disk
    /// image or macOS's App Translocation copy of a quarantined app. `readOnly`
    /// is the volume's answer when it has already been read; nil reads it here.
    static func isTemporary(path: String, readOnly: Bool? = nil) -> Bool {
        if path.lowercased().contains("/apptranslocation/") { return true }
        if let readOnly { return readOnly }
        return (try? URL(fileURLWithPath: path).resourceValues(forKeys: [.volumeIsReadOnlyKey]).volumeIsReadOnly) ?? false
    }
}
