import XCTest
@testable import EZLM

/// Where EZLM is running from: a real install, a mounted disk image, or macOS's
/// hidden App Translocation copy of a quarantined download. Only a real install
/// can be replaced in place; the other two update into Applications instead.
final class UpdaterTests: XCTestCase {
    func testTranslocatedCopyIsTemporary() {
        let p = "/private/var/folders/xy/T/AppTranslocation/3F0C-11/dmg/EZLM.app"
        XCTAssertTrue(AppPlace.isTemporary(path: p, readOnly: false))
        XCTAssertTrue(AppPlace.isTemporary(path: p.lowercased(), readOnly: false))
    }

    func testDiskImageIsTemporary() {
        XCTAssertTrue(AppPlace.isTemporary(path: "/Volumes/EZLM/EZLM.app", readOnly: true))
    }

    func testRealInstallIsNotTemporary() {
        XCTAssertFalse(AppPlace.isTemporary(path: "/Applications/EZLM.app", readOnly: false))
        XCTAssertFalse(AppPlace.isTemporary(path: "/Users/sam/Downloads/EZLM.app", readOnly: false))
    }

    func testUpdateGoesToApplicationsFromTheDownload() {
        let translocated = URL(fileURLWithPath: "/private/var/folders/xy/T/AppTranslocation/3F0C-11/dmg/EZLM.app")
        XCTAssertEqual(Updater.destination(for: translocated).path, "/Applications/EZLM.app")
        let mounted = URL(fileURLWithPath: "/Volumes/EZLM/EZLM.app")
        XCTAssertEqual(Updater.destination(for: mounted, readOnly: true).path, "/Applications/EZLM.app")
    }

    func testUpdateStaysPutForAnInstalledApp() {
        let installed = URL(fileURLWithPath: "/Applications/EZLM.app")
        XCTAssertEqual(Updater.destination(for: installed), installed)
    }
}
