import XCTest
@testable import EZLM

/// Window and menu state: boot once per launch, and remember the page the
/// user was on across relaunches.
@MainActor
final class AppStateTests: XCTestCase {
    func testBootRunsOncePerLaunch() {
        let gate = LaunchGate()
        XCTAssertTrue(gate.take(), "the first window boots the app")
        XCTAssertFalse(gate.take(), "a window made later must not boot it again")
        XCTAssertFalse(gate.take())
    }

    func testSectionMemoryRoundTrip() {
        let name = "com.ezlm.tests.a10.\(UUID().uuidString)"
        let defaults = try! XCTUnwrap(UserDefaults(suiteName: name))
        defer { defaults.removePersistentDomain(forName: name) }
        let memory = SectionMemory(defaults: defaults)

        XCTAssertNil(memory.saved, "nothing saved on a first run")
        for section in Section.allCases {
            memory.save(section)
            XCTAssertEqual(memory.saved, section)
        }
    }

    func testUnknownSavedSectionIsIgnored() throws {
        let name = "com.ezlm.tests.a10.\(UUID().uuidString)"
        let defaults = try XCTUnwrap(UserDefaults(suiteName: name))
        defer { defaults.removePersistentDomain(forName: name) }
        // A section this build no longer has (renamed or removed).
        defaults.set("nonsense", forKey: SectionMemory.key)
        XCTAssertNil(SectionMemory(defaults: defaults).saved)
    }
}
