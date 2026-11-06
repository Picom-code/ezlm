import Foundation

/// Boot runs once per launch, however many windows open. The window can be
/// closed (and a quit the user cancelled) and opened again: booting a second
/// time would restart the engine under a job that is still running.
@MainActor
final class LaunchGate {
    private var booted = false

    /// True the first time, false after that.
    func take() -> Bool {
        if booted { return false }
        booted = true
        return true
    }
}

/// The section the app was on last. Window restoration brings the window
/// back; this brings back the page in it, instead of always Overview.
struct SectionMemory {
    static let key = "lastSection"

    private let defaults: UserDefaults

    init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
    }

    /// The saved section, or nil on a first run (or a section this build no
    /// longer has).
    var saved: Section? {
        defaults.string(forKey: Self.key).flatMap(Section.init(rawValue:))
    }

    func save(_ section: Section) {
        defaults.set(section.rawValue, forKey: Self.key)
    }
}
