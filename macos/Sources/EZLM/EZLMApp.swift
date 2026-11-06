import SwiftUI

@main
struct EZLMApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) private var delegate
    @State private var model = AppModel()
    @State private var launch = LaunchGate()
    private let memory = SectionMemory()

    var body: some Scene {
        WindowGroup {
            RootView()
                .environment(model)
                .frame(minWidth: 900, idealWidth: 1000, minHeight: 620, idealHeight: 680)
                .preferredColorScheme(.dark)
                .tint(Theme.signal)
                .task { delegate.model = model; await model.boot() }
                .onReceive(NotificationCenter.default.publisher(for: NSApplication.willTerminateNotification)) { _ in
                    // Keep the answer that was streaming when the person quit
                    // (or when an update restarts the app); the quit prompt
                    // promises it is kept.
                    MainActor.assumeIsolated { model.chat.save() }
                    model.engine.stop()
                }
                .onChange(of: model.section) { _, section in memory.save(section) }
        }
        .windowStyle(.hiddenTitleBar)
        .windowResizability(.contentMinSize)
        .defaultSize(width: 1000, height: 680)
        .commands {
            // One window: a second would show the same app model.
            CommandGroup(replacing: .newItem) {}
            // Settings is a page in the window, not a separate scene, so the
            // standard Cmd-, item is ours to add.
            CommandGroup(replacing: .appSettings) {
                Button("Settings…") {
                    model.go(.settings)
                    delegate.showWindow()
                }
                .keyboardShortcut(",", modifiers: .command)
            }
        }
    }
}

final class AppDelegate: NSObject, NSApplicationDelegate {
    weak var model: AppModel?

    func applicationDidFinishLaunching(_ notification: Notification) {
        // Launched from a plain binary (not a bundle) during development:
        // still behave like a regular app with a dock icon and focus.
        NSApp.setActivationPolicy(.regular)
        NSApp.activate(ignoringOtherApps: true)
        // The window can be made again after launch (closed, quit cancelled,
        // reopened from the Dock): style content windows as they appear, not
        // just the ones here now.
        NotificationCenter.default.addObserver(self, selector: #selector(contentWindowAppeared(_:)),
                                               name: NSWindow.didBecomeKeyNotification, object: nil)
        for w in NSApp.windows { style(w) }
    }

    @objc private func contentWindowAppeared(_ note: Notification) {
        guard let w = note.object as? NSWindow else { return }
        style(w)
    }

    /// Black behind the hidden title bar, and the whole background drags.
    private func style(_ w: NSWindow) {
        guard w.sheetParent == nil, !(w is NSPanel) else { return }
        w.backgroundColor = .black
        w.titlebarAppearsTransparent = true
        w.isMovableByWindowBackground = true
    }

    /// Brings the window back from the Dock or a minimized state.
    func showWindow() {
        NSApp.activate(ignoringOtherApps: true)
        guard let w = NSApp.windows.first(where: { !($0 is NSPanel) }) else { return }
        w.deminiaturize(nil)
        w.makeKeyAndOrderFront(nil)
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { true }

    /// Quitting stops whatever EZLM is in the middle of: ask first, so a stray
    /// Cmd-Q doesn't throw away a long download.
    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        // An update has already put the new app on disk and the relaunch
        // script is waiting for this process to exit: don't hold that up.
        if model?.updateInstalling == true { return .terminateNow }
        guard let warning = model?.quitWarning else { return .terminateNow }
        let a = NSAlert()
        a.messageText = "Quit EZLM?"
        a.informativeText = warning
        a.addButton(withTitle: "Keep Going")
        a.addButton(withTitle: "Quit")
        return a.runModal() == .alertFirstButtonReturn ? .terminateCancel : .terminateNow
    }

    func applicationWillTerminate(_ notification: Notification) {
        // The view that used to stop it can already be gone (the window was
        // closed, the quit was cancelled, and Quit came from the Dock menu).
        model?.engine.stop()
    }
}
