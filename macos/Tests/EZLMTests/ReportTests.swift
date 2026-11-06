import XCTest
@testable import EZLM

/// A problem report carries a short note about EZLM's last crash, taken from
/// the report macOS keeps, with nothing personal in it.
final class ReportTests: XCTestCase {
    func testCrashNote() throws {
        let head = #"{"app_name":"EZLM","timestamp":"2026-11-03 17:14:34.00 -0600","app_version":"0.1.11","os_version":"macOS 26.3.1 (25D2128)","name":"EZLM","bug_type":"309"}"#
        let body = #"""
        {"procPath":"/Users/sam/Applications/EZLM.app/Contents/MacOS/EZLM","faultingThread":1,
         "exception":{"type":"EXC_BREAKPOINT","signal":"SIGTRAP","codes":"0x1, 0x100918460"},
         "termination":{"namespace":"SIGNAL","indicator":"Trace/BPT trap: 5","code":5},
         "asi":{"libswiftCore.dylib":["Fatal error: Index out of range in /Users/sam/build/x.swift"]},
         "usedImages":[{"name":"EZLM","path":"*/EZLM"},{"name":"libswiftCore.dylib","path":"/usr/lib/swift/libswiftCore.dylib"}],
         "threads":[{"frames":[{"imageIndex":0,"symbol":"main"}]},
                    {"triggered":true,"frames":[{"imageIndex":1,"symbol":"_assertionFailure(_:_:file:line:flags:)"},{"imageIndex":0,"symbol":"ChatModel.restore(_:)"},{"imageIndex":7}]}]}
        """#
        let note = try XCTUnwrap(CrashNote.summary(head + "\n" + body))
        XCTAssertTrue(note.contains("version 0.1.11"))
        XCTAssertTrue(note.contains("EXC_BREAKPOINT SIGTRAP"))
        XCTAssertTrue(note.contains("Trace/BPT trap: 5"))
        XCTAssertTrue(note.contains("Index out of range"))
        XCTAssertTrue(note.contains("EZLM  ChatModel.restore(_:)"))       // the crashed thread, not thread 0
        XCTAssertTrue(note.contains("libswiftCore.dylib  _assertionFailure"))
        XCTAssertTrue(note.contains("?  ?"))                                 // an image it can't name doesn't break it
        XCTAssertFalse(note.contains("sam"), note)                           // no home folder, no paths
        XCTAssertFalse(note.contains("/Users"), note)
        XCTAssertNil(CrashNote.summary("not a crash report"))
        XCTAssertNil(CrashNote.summary("{}\n[1,2]"))
    }
}
