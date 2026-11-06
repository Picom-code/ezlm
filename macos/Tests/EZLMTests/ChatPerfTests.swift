import XCTest
@testable import EZLM

/// What a long chat costs the main thread: saving it, listing it, and drawing
/// a growing answer. Every test runs against a throwaway folder.
final class ChatPerfTests: XCTestCase {
    private var dir: URL!

    override func setUp() {
        super.setUp()
        dir = FileManager.default.temporaryDirectory.appendingPathComponent("ezlm-perf-tests-\(UUID().uuidString)")
        setenv("EZLM_CHAT_DIR", dir.path, 1)
        Memory.forgetCachedPrefs()
    }

    override func tearDown() {
        try? FileManager.default.removeItem(at: dir)
        unsetenv("EZLM_CHAT_DIR")
        Memory.forgetCachedPrefs()
        super.tearDown()
    }

    /// A conversation the size of a real long chat: a few dozen turns of prose.
    private func chat(turns: Int, chars: Int) -> ChatStore.Saved {
        var msgs: [ChatStore.SavedMsg] = []
        for i in 0..<turns {
            let body = String(repeating: "The quick brown fox jumps over the lazy dog. ", count: chars / 45 + 1)
            msgs.append(ChatStore.SavedMsg(user: i % 2 == 0, model: "Test 1B", text: body, rate: 30, tokens: 200))
        }
        return ChatStore.Saved(id: "chat-\(turns)-\(chars)", title: "A long chat", model: "Test 1B", updated: Date(), messages: msgs)
    }

    private func time(_ n: Int, _ body: () -> Void) -> Double {
        let start = Date()
        for _ in 0..<n { body() }
        return Date().timeIntervalSince(start) / Double(n) * 1000
    }

    /// Saving one turn re-reads every saved chat, on the main thread. That is
    /// the whole store decoded again for a menu that is usually closed.
    @MainActor func testSavingATurnDoesNotReadEverySavedChat() {
        for i in 0..<40 { ChatStore.save(chat(turns: 60, chars: 1200).with(id: "old-\(i)")) }
        XCTAssertEqual(ChatStore.recent().count, 40)

        let listing = time(3) { _ = ChatStore.recent() }
        print("MEASURE recent() with 40 chats: \(String(format: "%.1f", listing)) ms")

        // The chat being saved is one of the long ones. Chat opens with the
        // list already read once (restoreOnce), then saves a turn at a time.
        let m = ChatModel()
        m.recent = ChatStore.recent()
        let long = chat(turns: 42, chars: 1200)
        m.messages = long.messages.map { x in
            let msg = ChatMsg(user: x.user)
            msg.text = x.text
            msg.done = true
            msg.shown = x.text.count
            return msg
        }
        m.save()
        let saving = time(3) { m.save() }
        print("MEASURE ChatModel.save() of a 42-turn chat: \(String(format: "%.1f", saving)) ms")

        // A single save must cost far less than decoding the whole store again.
        XCTAssertLessThan(saving, listing / 2 + 2, "a save reads the whole store again (\(saving) ms vs \(listing) ms)")
        // The saved chat is at the top of History, once, and the list stays bounded.
        XCTAssertEqual(m.recent.first?.messages.count, 42) // this chat, newest first
        XCTAssertEqual(m.recent.filter { $0.messages.count == 42 }.count, 1)
        XCTAssertEqual(m.recent.count, 40)                 // the list stays as long as the store's
        m.save()
        XCTAssertEqual(m.recent.filter { $0.messages.count == 42 }.count, 1) // and once only
    }

    /// The typewriter redraws the growing answer 15 times a second, and every
    /// redraw parses the whole answer so far.
    func testGrowingAnswerParsesCheaply() {
        let answer = String(repeating: "Some **bold** words and `code` in a sentence that keeps going.\n\n", count: 250)
            + "- a list item with **bold**\n- another item\n\n```swift\nlet x = 1\nprint(x)\n```\n"
        let full = time(5) { _ = MdBlock.parse(answer) }
        print("MEASURE MdBlock.parse(\(answer.count) chars): \(String(format: "%.2f", full)) ms")

        // What one answer costs: 60 frames showing a growing prefix, as the typewriter does.
        let frames = 60
        let perAnswer = time(3) {
            for f in 1...frames {
                let cut = answer.index(answer.startIndex, offsetBy: answer.count * f / frames)
                _ = MdBlock.parse(String(answer[answer.startIndex..<cut]))
            }
        }
        print("MEASURE parsing one answer over \(frames) frames: \(String(format: "%.0f", perAnswer)) ms")

        // A long chat redraws every turn's markdown on every frame; only the last
        // one is growing, but a finished answer should not be parsed again.
        XCTAssertLessThan(perAnswer, 900, "one growing answer costs \(perAnswer) ms of parsing")

        // Dropping a text file is read and measured on the main thread.
        let big = String(repeating: "word ", count: 400_000) // about 2 MB, the size attach() allows
        let tokens = time(2) { _ = ChatModel.tokens(big) }
        print("MEASURE ChatModel.tokens(2 MB): \(String(format: "%.1f", tokens)) ms")
        let decode = time(2) { _ = ChatModel.decodeText(Data(big.utf8)) }
        print("MEASURE ChatModel.decodeText(2 MB): \(String(format: "%.1f", decode)) ms")
    }
}

private extension ChatStore.Saved {
    func with(id: String) -> ChatStore.Saved {
        var s = self
        s.id = id
        s.updated = Date()
        return s
    }
}
