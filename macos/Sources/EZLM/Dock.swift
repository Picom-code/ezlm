import AppKit
import SwiftUI

/// The Dock icon shows how far a long job is (like a download in Safari), and
/// asks for attention when one ends while EZLM is in the background: one
/// bounce, plus a "!" badge until EZLM is opened again if it failed.
@MainActor
enum Dock {
    private static let view = DockProgressView()
    private static var watching = false

    static func progress(_ fraction: Double) {
        let tile = NSApp.dockTile
        let f = min(max(fraction, 0), 1)
        if tile.contentView === view && abs(view.fraction - f) < 0.005 { return } // no visible change
        view.fraction = f
        if tile.contentView !== view {
            view.frame = NSRect(origin: .zero, size: tile.size)
            tile.contentView = view
        }
        tile.display()
    }

    static func clear() {
        guard NSApp.dockTile.contentView != nil else { return }
        NSApp.dockTile.contentView = nil
        NSApp.dockTile.display()
    }

    static func finished(ok: Bool) {
        clear()
        guard !NSApp.isActive else { return }
        if !ok { NSApp.dockTile.badgeLabel = "!" }
        NSApp.requestUserAttention(.informationalRequest)
        if !watching {
            watching = true
            NotificationCenter.default.addObserver(forName: NSApplication.didBecomeActiveNotification, object: nil, queue: .main) { _ in
                NSApp.dockTile.badgeLabel = nil
            }
        }
    }
}

/// The app icon with a progress bar along its bottom.
private final class DockProgressView: NSView {
    var fraction = 0.0

    override func draw(_ dirty: NSRect) {
        NSApp.applicationIconImage?.draw(in: bounds)
        let h = bounds.height * 0.085 // inside the icon, under its dashed line
        let track = NSRect(x: bounds.width * 0.2, y: bounds.height * 0.17, width: bounds.width * 0.6, height: h)
        NSColor(white: 0.22, alpha: 1).setFill()
        NSBezierPath(roundedRect: track, xRadius: h / 2, yRadius: h / 2).fill()
        var bar = track.insetBy(dx: h * 0.18, dy: h * 0.18)
        bar.size.width = max(bar.height, bar.width * fraction)
        NSColor(Theme.signal).setFill()
        NSBezierPath(roundedRect: bar, xRadius: bar.height / 2, yRadius: bar.height / 2).fill()
    }
}
