import XCTest
import AppKit
@testable import EZLM

/// Retain-cycle and leak checks for the app model, the chat model and the
/// signal wave. Each test makes an object, drops it, and asks whether it (or
/// its timer) is really gone.
@MainActor
final class MemoryTests: XCTestCase {
    private var dir: URL!

    override func setUp() {
        super.setUp()
        dir = FileManager.default.temporaryDirectory.appendingPathComponent("ezlm-memory-tests-\(UUID().uuidString)")
        setenv("EZLM_CHAT_DIR", dir.path, 1)
        Memory.forgetCachedPrefs()
    }

    override func tearDown() {
        try? FileManager.default.removeItem(at: dir)
        unsetenv("EZLM_CHAT_DIR")
        Memory.forgetCachedPrefs()
        super.tearDown()
    }

    /// The app model, doing nothing, is released when the last reference goes.
    func testAppModelReleasesWhenIdle() {
        weak var weakModel: AppModel?
        do {
            let m = AppModel()
            weakModel = m
            XCTAssertNotNil(weakModel)
        }
        XCTAssertNil(weakModel)
    }

    /// The chat and the app model point at each other; neither may keep the
    /// other alive (the chat's `app` is weak, the app's `chat` is owned).
    func testChatAndAppModelDoNotKeepEachOtherAlive() {
        weak var weakChat: ChatModel?
        do {
            let m = AppModel()
            let c = ChatModel()
            weakChat = c
            c.attach(m)
            XCTAssertTrue(c.app === m)
            XCTAssertEqual(c.modelName, "your model")
        }
        XCTAssertNil(weakChat)
    }

    /// Polling is a long-lived task owned by the app model. It must not keep
    /// the model alive on its own once the model is let go (a cycle of
    /// model -> task -> model means the model, its chats and its engine can
    /// never be released, which a second window or a test would notice).
    func testPollingDoesNotKeepTheAppModelAlive() async {
        weak var weakModel: AppModel?
        do {
            let m = AppModel()
            weakModel = m
            m.startPolling()
        }
        try? await Task.sleep(nanoseconds: 300_000_000)
        XCTAssertNil(weakModel)
    }

    /// The wave's timer is owned by the run loop, not by the wave. If the wave
    /// goes away without stop() (a view torn down without onDisappear, or a
    /// second chat pane), the timer must not keep ticking for the app's life.
    func testWaveTimerStopsWhenTheWaveIsGone() {
        _ = NSApplication.shared
        final class Ticks { var count = 0 }
        let ticks = Ticks()
        var wave: SignalWave? = SignalWave()
        wave!.moodOf = { .off }
        wave!.onFrame = { ticks.count += 1 }
        wave!.start()
        RunLoop.main.run(until: Date().addingTimeInterval(0.2))
        XCTAssertGreaterThan(ticks.count, 0, "the timer did not run at all")
        let timer = wave!.timer
        XCTAssertEqual(timer?.isValid, true)
        wave = nil
        RunLoop.main.run(until: Date().addingTimeInterval(0.1))
        XCTAssertEqual(timer?.isValid, false, "the timer kept running after the wave was gone")
        let stopped = ticks.count
        RunLoop.main.run(until: Date().addingTimeInterval(0.2))
        XCTAssertEqual(ticks.count, stopped, "the timer kept ticking after the wave was gone")
    }

    /// Starting the wave twice must not leave two timers (each stop() would
    /// then only cancel one, and the other would run for the app's life).
    func testWaveStartIsIdempotent() {
        _ = NSApplication.shared
        let wave = SignalWave()
        wave.moodOf = { .off }
        wave.start()
        wave.start()
        XCTAssertTrue(wave.isRunning)
        wave.stop()
        XCTAssertFalse(wave.isRunning)
        wave.start()
        XCTAssertTrue(wave.isRunning)
        wave.stop()
    }
}
