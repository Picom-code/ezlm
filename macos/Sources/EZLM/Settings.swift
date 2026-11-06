import SwiftUI

// Settings: updates (check, download, install) and where models are saved.

// MARK: - Updates

extension AppModel {
    /// What's installed now: the engine's version (the app and engine ship together).
    var currentVersion: String {
        if let v = status?.version, !v.isEmpty { return v }
        if let v = updateInfo?.current, !v.isEmpty { return v }
        return Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "dev"
    }

    /// The newer release, when there is one.
    var availableUpdate: UpdateRelease? { updateInfo?.available == true ? updateInfo?.latest : nil }

    /// Asks the engine whether a newer EZLM is out (it caches GitHub's answer for an hour).
    func checkUpdate(force: Bool = false) async {
        guard !updateChecking else { return }
        updateChecking = true
        do {
            updateInfo = try await engine.get(force ? "update?force=1" : "update")
        } catch {
            updateInfo = UpdateInfo(current: updateInfo?.current ?? "", error: error.localizedDescription)
        }
        updateChecking = false
    }

    func startUpdate() async {
        guard !updateBusy, !installRunning else { return }
        updateBusy = true // set first: the job's first events can arrive before the reply
        updateInstalling = false
        updateManual = false
        updateFraction = nil
        updateNote = "Starting the download…"
        do { try await engine.post("update/download") } catch {
            updateBusy = false
            updateNote = error.localizedDescription
            debug("update: \(error.localizedDescription)")
        }
    }

    func handleUpdate(_ e: EngineEvent) {
        guard updateBusy, !updateInstalling else { return } // not an update this window started
        if e.step == "download", e.state == "progress", let t = e.total, t > 0, let d = e.done {
            updateFraction = Double(d) / Double(t)
            updateNote = "Downloading: \(gb(d)) of \(gb(t))"
        } else if e.step == "download", e.state == "ok" {
            updateInstalling = true // its "done" follows; the install takes it from here
            Task { await installUpdate(e.msg ?? "") }
        } else if e.step == "done" {
            updateBusy = false
            updateFraction = nil
            if e.state != "ok" {
                updateNote = e.state == "cancelled" ? "Update cancelled." : ((e.msg ?? "").isEmpty ? "The update didn't download." : e.msg ?? "")
                debug("update: \(updateNote)")
            }
        }
    }

    /// Mounts the downloaded disk image and swaps the new EZLM in, then restarts.
    /// If that can't be done (no permission to change Applications, say), the
    /// disk image opens in Finder so the user can drag it over instead.
    private func installUpdate(_ path: String) async {
        guard !Engine.demo, path.hasSuffix(".dmg"), FileManager.default.fileExists(atPath: path) else {
            updateInstalling = false
            updateBusy = false
            updateFraction = nil
            if Engine.demo {
                updateNote = "Downloaded. Demo mode doesn't install anything."
                showToast("Demo: the update downloaded, but nothing is installed in demo mode.", Theme.gold)
            } else {
                updateNote = "The update downloaded, but EZLM can't find the file. Try again."
                debug("update: no installer at \"\(path)\"")
            }
            return
        }
        let dmg = URL(fileURLWithPath: path)
        let app = Bundle.main.bundleURL
        updateFraction = 1
        updateNote = "Installing the update…"
        do {
            try await Task.detached { try Updater.install(dmg: dmg, over: app) }.value
            updateNote = "Installed. EZLM is restarting…"
            showToast("Installing the update. EZLM will open again in a moment.", Theme.ok)
            try? await Task.sleep(nanoseconds: 900_000_000)
            if !Updater.relaunch(app) {
                updateBusy = false
                updateInstalling = false
                updateNote = "Installed. Quit EZLM and open it again to finish."
            }
        } catch {
            debug("update install: \(error.localizedDescription)")
            updateInstalling = false
            updateBusy = false
            updateFraction = nil
            updateManual = true
            NSWorkspace.shared.open(dmg)
            updateNote = "Drag EZLM into Applications to replace the old one, then open it again."
            showToast(updateNote, Theme.gold)
        }
    }
}

// MARK: - Where models are saved

extension AppModel {
    func refreshStorage() async {
        do {
            storage = try await engine.get("storage")
            storageError = ""
        } catch {
            storageError = error.localizedDescription
        }
    }

    /// Where models will actually go for a picked folder, as the engine decides
    /// it (StorageDir in internal/app/settings.go): an "EZLM Models" folder
    /// inside it, unless the folder's own name already says EZLM.
    static func storageTarget(_ picked: String) -> String {
        // The demo's drives come from a Windows PC: keep its separators.
        let sep: Character = picked.contains("\\") && !picked.contains("/") ? "\\" : "/"
        var p = picked
        while p.count > 1, p.last == sep { p.removeLast() }
        let name = p.split(separator: sep).last.map(String.init) ?? p
        if name.lowercased().contains("ezlm") { return p }
        return p + (p.last == sep ? "" : String(sep)) + "EZLM Models"
    }

    static func samePath(_ a: String, _ b: String) -> Bool {
        func norm(_ s: String) -> String {
            var t = s.lowercased()
            while t.count > 1, t.last == "/" || t.last == "\\" { t.removeLast() }
            return t
        }
        return norm(a) == norm(b)
    }

    var pendingTarget: String? { pendingDir.map(Self.storageTarget) }

    /// What "move" would carry to `target`: every model, or only the ones in
    /// other folders when the target is where models are saved already.
    static func moveBytes(_ st: StorageInfo, to target: String) -> UInt64 {
        samePath(target, st.dir) ? st.elsewhere : st.modelsBytes
    }

    /// A drive card: picking the one in use puts things back as they are.
    func pickDrive(_ d: StorageDrive) {
        guard !storageBusy else { return }
        pendingDir = d.current ? nil : d.path
        moveModels = true
        storageNote = ""
    }

    func chooseStorageFolder() {
        guard !storageBusy else { return }
        let panel = NSOpenPanel()
        panel.canChooseDirectories = true
        panel.canChooseFiles = false
        panel.canCreateDirectories = true
        panel.allowsMultipleSelection = false
        panel.prompt = "Choose"
        panel.message = "Pick where EZLM saves models. They're big (often 2 to 70 GB), so choose a drive with room."
        guard panel.runModal() == .OK, let url = panel.url else { return }
        storageNote = ""
        // Already where models go: nothing to change, unless some are elsewhere to gather here.
        if let st = storage, Self.samePath(Self.storageTarget(url.path), st.dir), st.elsewhere == 0 {
            pendingDir = nil
            showToast("Models are already saved there.", Theme.ok)
            return
        }
        pendingDir = url.path
        moveModels = true
    }

    func saveStorage() async {
        guard !storageBusy, let st = storage, let target = pendingTarget else { return }
        storageBusy = true // set first: the job's first events can arrive before the reply
        storageFraction = nil
        storageNote = ""
        do { try await engine.post("storage", ["dir": target, "move": moveModels && Self.moveBytes(st, to: target) > 0]) } catch {
            storageBusy = false
            showToast(error.localizedDescription, Theme.bad)
        }
    }

    func handleStorage(_ e: EngineEvent) {
        guard storageBusy else { return } // not a change this window started
        if e.step == "done" {
            storageBusy = false
            storageFraction = nil
            if e.state == "cancelled" { Dock.clear() } else { Dock.finished(ok: e.state == "ok") }
            if e.state == "ok" {
                pendingDir = nil
                storageNote = ""
            } else {
                storageNote = e.state == "cancelled" ? "Cancelled." : ((e.msg ?? "").isEmpty ? "Couldn't change where models are saved." : e.msg ?? "")
                if e.state != "cancelled" { showToast(storageNote, Theme.bad); debug("storage: \(storageNote)") }
            }
            // Free space, the models' folder and maybe the running model changed.
            Task {
                await refreshStorage()
                await refreshStatus()
                await refreshInstalled()
                await refreshScan()
            }
            return
        }
        if e.state == "progress", let t = e.total, t > 0, let d = e.done {
            storageFraction = Double(d) / Double(t)
            Dock.progress(Double(d) / Double(t))
            storageNote = "\(gb(d)) of \(gb(t)) moved"
        } else if e.state == "ok", let msg = e.msg, !msg.isEmpty {
            showToast(msg, Theme.ok)
        } else if let msg = e.msg, !msg.isEmpty {
            storageNote = msg
        }
    }
}

// MARK: - The pane

struct SettingsPane: View {
    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            PaneHeader(title: "Settings", subtitle: "Keep EZLM up to date and choose where models are saved.")
            UpdatesSection()
            StorageSection()
            AboutSection()
        }
    }
}

private struct AboutSection: View {
    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            SectionTitle("About")
            Text(Attribution.apps).mono(11, .regular, Theme.dim).fixedSize(horizontal: false, vertical: true).textSelection(.enabled)
        }
    }
}

/// The app logos' attribution, word for word as in Logos/NOTICE.md (a test checks it).
enum Attribution {
    static let apps = "App logos identify the apps EZLM connects to and are trademarks of their owners: OpenCode (Anomaly Innovations, Inc.; square logo from github.com/anomalyco/opencode, MIT), Hermes Agent (Nous Research, Inc.; app icon from github.com/NousResearch/hermes-agent, MIT), DeepSeek Harness (DeepSeek; whale icon from github.com/deepseek-ai/deepseek-harness, MIT). EZLM is compatible with these apps but is not affiliated with or endorsed by their makers."
}

private struct UpdatesSection: View {
    @Environment(AppModel.self) private var m

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            SectionTitle("Updates")
            if let latest = m.availableUpdate {
                (Text("EZLM \(latest.version)").foregroundColor(Theme.ok) + Text(" is available. You have \(m.currentVersion).").foregroundColor(Theme.dim))
                    .font(Theme.mono(14))
                ForEach(Array(latest.noteLines.enumerated()), id: \.offset) { _, line in
                    Text(line).mono(12, .regular, Theme.dim).fixedSize(horizontal: false, vertical: true).padding(.top, 6)
                }
                HStack(spacing: 12) {
                    Button(m.updateBusy ? "Updating…" : "Update to \(latest.version)") { Task { await m.startUpdate() } }
                        .buttonStyle(.primary).disabled(m.updateBusy || m.installRunning)
                    Text("\(gb(latest.size)) download. Your model keeps running; EZLM restarts itself.")
                        .mono(11, .regular, Theme.dim).fixedSize(horizontal: false, vertical: true)
                }
                .padding(.top, 12)
            } else {
                Text(line).mono(13).fixedSize(horizontal: false, vertical: true)
                Button(m.updateChecking ? "Checking…" : "Check for updates") { Task { await m.checkUpdate(force: true) } }
                    .buttonStyle(.term).disabled(m.updateChecking || m.updateBusy).padding(.top, 10)
            }
            if m.updateBusy {
                Group {
                    if let f = m.updateFraction { BlockBar(f, height: 6).animation(.easeOut(duration: 0.3), value: f) }
                    else { WorkingBar(height: 6) }
                }
                .frame(maxWidth: 420).padding(.top, 12)
            }
            if !m.updateNote.isEmpty {
                Text(m.updateNote).mono(11, .regular, m.updateManual ? Theme.gold : Theme.dim).monospacedDigit()
                    .fixedSize(horizontal: false, vertical: true).padding(.top, 6)
            }
            if m.updateManual {
                // Finder can't replace an app that's open.
                Button("Quit EZLM") { NSApp.terminate(nil) }.buttonStyle(.small).padding(.top, 8)
            }
        }
        .task { if m.updateInfo == nil { await m.checkUpdate() } }
    }

    private var line: String {
        let cur = m.currentVersion
        guard let info = m.updateInfo, !m.updateChecking else { return "EZLM \(cur). Checking for updates…" }
        if let err = info.error, !err.isEmpty {
            let s = err.prefix(1).uppercased() + err.dropFirst()
            return "EZLM \(cur). \(s)\(s.hasSuffix(".") ? "" : ".")"
        }
        return "EZLM \(cur) is the latest version."
    }
}

extension UpdateRelease {
    /// Up to two lines of the release notes, without Markdown headings and bullets.
    var noteLines: [String] {
        notes.split(whereSeparator: \.isNewline)
            .map { $0.trimmingCharacters(in: .whitespaces) }
            .filter { !$0.isEmpty && !$0.hasPrefix("#") }
            .map { l -> String in
                var t = Substring(l)
                while let c = t.first, "-*•".contains(c) { t = t.dropFirst() }
                let line = t.trimmingCharacters(in: .whitespaces).replacingOccurrences(of: "**", with: "")
                return line.count > 160 ? String(line.prefix(160)) + "…" : line
            }
            .filter { !$0.isEmpty }
            .prefix(2)
            .map { $0 }
    }
}

private struct StorageSection: View {
    @Environment(AppModel.self) private var m

    var body: some View {
        @Bindable var m = m
        VStack(alignment: .leading, spacing: 0) {
            SectionTitle("Where models are saved")
            if let st = m.storage {
                (Text("New downloads go to ").foregroundColor(Theme.dim) + Text(st.dir).foregroundColor(Theme.ink)
                 + Text(" (\(gb(st.free)) free)").foregroundColor(Theme.dim))
                    .font(Theme.mono(13)).textSelection(.enabled).fixedSize(horizontal: false, vertical: true)
                if st.modelsBytes > 0 {
                    Text("EZLM's models on this computer take \(gb(st.modelsBytes))" + (st.elsewhere > 0 ? ", \(gb(st.elsewhere)) of it in other folders." : "."))
                        .mono(12, .regular, Theme.dim).fixedSize(horizontal: false, vertical: true).padding(.top, 4)
                }
                LazyVGrid(columns: [GridItem(.flexible(), spacing: 10), GridItem(.flexible(), spacing: 10)], alignment: .leading, spacing: 10) {
                    ForEach(st.drives) { d in
                        let picked = m.pendingDir.map { AppModel.samePath($0, d.path) } ?? d.current
                        Button { m.pickDrive(d) } label: { DriveCard(drive: d, picked: picked) }
                            .buttonStyle(.plain)
                            .disabled(m.storageBusy)
                            .accessibilityLabel(d.label.isEmpty ? d.path : d.label)
                            .accessibilityValue("\(gb(d.free)) free of \(gb(d.total))\(d.current ? ", in use" : "")")
                            .accessibilityAddTraits(picked ? .isSelected : [])
                    }
                }
                .padding(.top, 14)
                Button("Choose a folder…") { m.chooseStorageFolder() }.buttonStyle(.term).disabled(m.storageBusy).padding(.top, 12)

                if let target = m.pendingTarget {
                    VStack(alignment: .leading, spacing: 8) {
                        (Text("Models will go to ").foregroundColor(Theme.dim) + Text(target).foregroundColor(Theme.ink))
                            .font(Theme.mono(13)).textSelection(.enabled).fixedSize(horizontal: false, vertical: true)
                        let moving = AppModel.moveBytes(st, to: target)
                        if moving > 0 {
                            Toggle(isOn: $m.moveModels) {
                                (Text("Move my downloaded models there ") + Text("(\(gb(moving)))").foregroundColor(Theme.dim)).font(Theme.mono(13))
                            }
                            .toggleStyle(.checkbox).disabled(m.storageBusy)
                            Text(m.moveModels ? "Your running model pauses for a moment while its file moves."
                                              : "Models stay where they are and keep working; only new downloads go to the new place.")
                                .mono(11, .regular, Theme.dim).fixedSize(horizontal: false, vertical: true)
                        }
                        HStack(spacing: 10) {
                            Button(m.storageBusy ? "Saving…" : "Save") { Task { await m.saveStorage() } }
                                .buttonStyle(.primary).disabled(m.storageBusy || m.installRunning)
                            Button("Cancel") { m.pendingDir = nil }.buttonStyle(.term).disabled(m.storageBusy)
                        }
                        .padding(.top, 4)
                    }
                    .padding(EdgeInsets(top: 12, leading: 14, bottom: 12, trailing: 14))
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(Theme.signal.opacity(0.055))
                    .overlay(Rectangle().stroke(Theme.signal.opacity(0.55), lineWidth: 1))
                    .padding(.top, 14)
                }
                if m.storageBusy {
                    Group {
                        if let f = m.storageFraction { BlockBar(f, height: 6).animation(.easeOut(duration: 0.3), value: f) }
                        else { WorkingBar(height: 6) }
                    }
                    .frame(maxWidth: 420).padding(.top, 12)
                }
                if !m.storageNote.isEmpty {
                    Text(m.storageNote).mono(11, .regular, Theme.dim).monospacedDigit().fixedSize(horizontal: false, vertical: true).padding(.top, 6)
                }
            } else if !m.storageError.isEmpty {
                Text(m.storageError).mono(12, .regular, Theme.bad).fixedSize(horizontal: false, vertical: true)
                Button("Try again") { Task { await m.refreshStorage() } }.buttonStyle(.small).padding(.top, 8)
            } else {
                HStack(spacing: 10) { Spinner(); Text("Looking at your drives").mono(12, .regular, Theme.dim) }
            }
        }
        .task { await m.refreshStorage() } // each visit: free space changes
    }
}

/// A drive models could go to: its name, room, a usage bar, and "in use".
private struct DriveCard: View {
    let drive: StorageDrive
    let picked: Bool

    var body: some View {
        let used = drive.total > 0 ? 1 - Double(drive.free) / Double(drive.total) : 0
        VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 8) {
                Text(drive.label.isEmpty ? drive.path : drive.label).mono(13, .bold).lineLimit(1)
                if drive.current { Text("in use").mono(10, .regular, Theme.ok) }
            }
            Text("\(gb(drive.free)) free of \(gb(drive.total))").mono(11, .regular, Theme.dim)
            BlockBar(used, color: drive.free < 20 << 30 ? Theme.gold : Theme.signal, height: 6)
        }
        .padding(12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(picked ? Theme.signal.opacity(0.07) : .clear)
        .overlay(Rectangle().stroke(picked ? Theme.signal : Theme.faint, lineWidth: 1))
        .contentShape(Rectangle())
        .help(drive.path)
    }
}

/// On Overview while a newer EZLM is out.
/// The way out of running from the download: into Applications.
struct MoveToApplicationsNote: View {
    var body: some View {
        HStack(spacing: 14) {
            Text("EZLM is running from the download, so it won't be here after a restart. Drag it into Applications.")
                .mono(12, .regular, Theme.gold).fixedSize(horizontal: false, vertical: true)
            Spacer(minLength: 0)
            Button("Open Applications") { NSWorkspace.shared.open(URL(fileURLWithPath: "/Applications")) }.buttonStyle(.small)
        }
        .padding(EdgeInsets(top: 9, leading: 14, bottom: 9, trailing: 14))
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Theme.gold.opacity(0.05))
        .overlay(Rectangle().stroke(Theme.gold.opacity(0.5), lineWidth: 1))
    }
}

struct UpdateBanner: View {
    @Environment(AppModel.self) private var m
    let latest: UpdateRelease

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 14) {
                Text("EZLM \(latest.version) is available").mono(13, .bold)
                Button(m.updateBusy ? "Updating…" : "Update now") { Task { await m.startUpdate() } }
                    .buttonStyle(.primary).disabled(m.updateBusy || m.installRunning)
                Button("What's new") { m.go(.settings) }.buttonStyle(.term)
            }
            if m.updateBusy || m.updateManual, !m.updateNote.isEmpty {
                Text(m.updateNote).mono(11, .regular, m.updateManual ? Theme.gold : Theme.dim).monospacedDigit()
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
        .padding(EdgeInsets(top: 10, leading: 14, bottom: 10, trailing: 14))
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Theme.ok.opacity(0.05))
        .overlay(Rectangle().stroke(Theme.ok.opacity(0.55), lineWidth: 1))
        .padding(.bottom, 14)
    }
}
