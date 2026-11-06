import XCTest
@testable import EZLM

/// The app-side helpers the other test files don't reach: the numbers in the
/// pour panel, the Model mode replies, the words for time and size, and the
/// edges of the Markdown reader. Nothing here starts an engine or touches the
/// person's files.
final class AppTests: XCTestCase {
    private let g: UInt64 = 1 << 30

    // MARK: - Words for time and size

    /// "just now", "2 min ago", "3 h ago", "yesterday", then a date. A clock
    /// that is a little ahead must not print a negative time.
    func testTimeWords() {
        let now = Date(timeIntervalSince1970: 1_800_000_000)
        XCTAssertEqual(ChatStore.ago(now, now: now), "just now")
        XCTAssertEqual(ChatStore.ago(now.addingTimeInterval(59), now: now), "just now")
        XCTAssertEqual(ChatStore.ago(now.addingTimeInterval(-60), now: now), "1 min ago")
        XCTAssertEqual(ChatStore.ago(now.addingTimeInterval(-3599), now: now), "59 min ago")
        XCTAssertEqual(ChatStore.ago(now.addingTimeInterval(-3600), now: now), "1 h ago")
        XCTAssertEqual(ChatStore.ago(now.addingTimeInterval(-86399), now: now), "23 h ago")
        XCTAssertEqual(ChatStore.ago(now.addingTimeInterval(-86400), now: now), "yesterday")
        XCTAssertEqual(ChatStore.ago(now.addingTimeInterval(-172799), now: now), "yesterday")
        let older = ChatStore.ago(now.addingTimeInterval(-3 * 86400), now: now)
        XCTAssertFalse(older.contains("ago"), older)
        XCTAssertNotEqual(older, "yesterday")
        // Older than two days: the day it happened, in the reader's own words.
        let f = DateFormatter()
        f.dateFormat = "MMM d"
        XCTAssertEqual(older, f.string(from: now.addingTimeInterval(-3 * 86400)))
    }

    /// Sizes read the way people say them: MB below a gigabyte, one decimal
    /// under ten, whole gigabytes above it.
    func testSizes() {
        XCTAssertEqual(gb(0), "0 MB")
        XCTAssertEqual(gb(1_048_576), "1 MB")
        XCTAssertEqual(gb(1023 * 1_048_576), "1023 MB")
        XCTAssertEqual(gb(1024 * 1_048_576), "1.0 GB")
        XCTAssertEqual(gb(2 * g + g / 2), "2.5 GB")
        XCTAssertEqual(gb(9 * g), "9.0 GB")
        XCTAssertEqual(gb(10 * g), "10 GB")
        XCTAssertEqual(gb(63 * g), "63 GB")
        XCTAssertEqual(gb(128 * g), "128 GB")
    }

    func testUptimeWords() {
        XCTAssertEqual(uptimeText(0), "0s")
        XCTAssertEqual(uptimeText(59), "59s")
        XCTAssertEqual(uptimeText(60), "1m")
        XCTAssertEqual(uptimeText(3599), "59m")
        XCTAssertEqual(uptimeText(3600), "1h 0m")
        XCTAssertEqual(uptimeText(86399), "23h 59m")
        XCTAssertEqual(uptimeText(86400), "1d 0h")
        XCTAssertEqual(uptimeText(90061), "1d 1h")
    }

    // MARK: - The pour panel

    /// Shared memory: one bar, "other apps" is the lowest usage seen (so a
    /// model being unloaded first doesn't count as the machine getting busy),
    /// and the model's share only ever grows, up to the planned size.
    @MainActor
    func testPourSharedMemory() {
        let fill = MemoryFill(name: "Test 1B")
        XCTAssertEqual(fill.title, "Pouring Test 1B into memory")
        XCTAssertNil(fill.percent)
        XCTAssertEqual(fill.rows.count, 0)

        fill.feed(unified(total: 16 * g, avail: 10 * g, plan: 8 * g))
        XCTAssertEqual(fill.rows.map(\.id), ["mem"])
        XCTAssertEqual(fill.rows[0].total, 16 * g)
        XCTAssertEqual(fill.rows[0].need, 8 * g)
        XCTAssertEqual(fill.rows[0].target, 8 * g)
        XCTAssertEqual(fill.rows[0].model, 0)
        XCTAssertEqual(fill.percent, 0)

        fill.feed(unified(total: 16 * g, avail: 6 * g, plan: 8 * g))
        XCTAssertEqual(fill.rows[0].model, 4 * g) // 6 GB was other apps, 4 GB is now the model
        XCTAssertEqual(fill.rows[0].base, 6 * g)
        XCTAssertEqual(fill.percent, 50)

        // The machine gets quieter: that must not shrink the model's share.
        fill.feed(unified(total: 16 * g, avail: 14 * g, plan: 8 * g))
        XCTAssertEqual(fill.rows[0].base, 2 * g)
        XCTAssertEqual(fill.rows[0].model, 4 * g)
        XCTAssertEqual(fill.percent, 50)

        // The pre-read moves the bar even before the driver counts the memory.
        fill.feed(unified(total: 16 * g, avail: 16 * g, plan: 8 * g, loadDone: 8 * g, loadTotal: 8 * g))
        XCTAssertEqual(fill.rows[0].model, 8 * g)
        XCTAssertEqual(fill.rows[0].model, fill.rows[0].target)
        XCTAssertEqual(fill.percent, 100)

        // A sample without a plan keeps the plan it already had.
        fill.feed(unified(total: 16 * g, avail: 16 * g, plan: 0))
        XCTAssertEqual(fill.rows[0].need, 8 * g)

        fill.complete(nil)
        XCTAssertTrue(fill.done)
        XCTAssertEqual(fill.percent, 100)
        XCTAssertEqual(fill.title, "Loaded Test 1B")
        XCTAssertEqual(fill.rows[0].numbers(done: true).0, gb(8 * g))
        XCTAssertEqual(fill.rows[0].numbers(done: true).1, " loaded")
    }

    /// Separate graphics and system memory: a graphics bar always, a system bar
    /// when part of the model lives there (or the Mac has no graphics memory).
    @MainActor
    func testPourSeparateMemory() {
        let fill = MemoryFill(name: "Test 3B")
        fill.feed(separate(gpuTotal: 8 * g, gpuFree: 8 * g, ramTotal: 32 * g, ramAvail: 24 * g, gpuPlan: 6 * g, ramPlan: 2 * g))
        XCTAssertEqual(fill.rows.map(\.id), ["gpu", "ram"])
        XCTAssertEqual(fill.rows[0].need, 6 * g)
        XCTAssertEqual(fill.rows[1].need, 2 * g)
        XCTAssertEqual(fill.percent, 0)

        // Part of the model in system memory: both bars fill.
        fill.feed(separate(gpuTotal: 8 * g, gpuFree: 2 * g, ramTotal: 32 * g, ramAvail: 22 * g, gpuPlan: 6 * g, ramPlan: 2 * g))
        XCTAssertEqual(fill.rows[0].model, 6 * g)
        XCTAssertEqual(fill.rows[1].model, 2 * g)
        XCTAssertEqual(fill.percent, 100)

        // A model that fits in graphics memory needs no system bar.
        let gpuOnly = MemoryFill(name: "Test 1B")
        gpuOnly.feed(separate(gpuTotal: 8 * g, gpuFree: 8 * g, ramTotal: 32 * g, ramAvail: 32 * g, gpuPlan: 6 * g, ramPlan: g / 2))
        XCTAssertEqual(gpuOnly.rows.map(\.id), ["gpu"])

        // No graphics memory at all (an Intel Mac): system memory is the bar.
        let intel = MemoryFill(name: "Test 1B")
        intel.feed(separate(gpuTotal: 0, gpuFree: 0, ramTotal: 16 * g, ramAvail: 12 * g, gpuPlan: 0, ramPlan: 4 * g))
        XCTAssertEqual(intel.rows.map(\.id), ["ram"])
        XCTAssertEqual(intel.rows[0].model, 0) // nothing has grown yet: 4 GB was already in use
        intel.feed(separate(gpuTotal: 0, gpuFree: 0, ramTotal: 16 * g, ramAvail: 8 * g, gpuPlan: 0, ramPlan: 4 * g))
        XCTAssertEqual(intel.rows[0].model, 4 * g)
    }

    /// The pre-read only moves forward, and the bar never overshoots its plan.
    @MainActor
    func testPourWarmth() {
        let fill = MemoryFill(name: "Test 1B")
        fill.feed(unified(total: 16 * g, avail: 16 * g, plan: 8 * g, loadDone: g, loadTotal: 4 * g))
        XCTAssertEqual(fill.rows[0].model, 2 * g) // a quarter of 8 GB
        fill.feed(unified(total: 16 * g, avail: 16 * g, plan: 8 * g, loadDone: 0, loadTotal: 4 * g))
        XCTAssertEqual(fill.rows[0].model, 2 * g) // read backwards? no: it stays
        fill.feed(unified(total: 16 * g, avail: 16 * g, plan: 8 * g, loadDone: 99 * g, loadTotal: 4 * g))
        XCTAssertEqual(fill.rows[0].model, 8 * g) // never more than the plan

        // Loading finished: the bars fill to the plan and settle.
        let done = MemoryFill(name: "Test 1B")
        done.feed(unified(total: 16 * g, avail: 16 * g, plan: 8 * g))
        done.complete(unified(total: 16 * g, avail: 16 * g, plan: 8 * g))
        XCTAssertTrue(done.done)
        XCTAssertEqual(done.rows[0].model, 8 * g)
        done.complete(nil) // a second "complete" changes nothing
        XCTAssertEqual(done.rows[0].model, 8 * g)
    }

    func testPourRowNumbers() {
        let row = MemoryFill.Row(id: "mem", label: "Memory", total: 16 * g, need: 8 * g, model: 4 * g, target: 8 * g, base: 4 * g)
        XCTAssertEqual(row.numbers(done: false).0, "4.0 GB")
        XCTAssertEqual(row.numbers(done: false).1, " of 8.0 GB")
        XCTAssertEqual(row.numbers(done: true).0, "8.0 GB")
        XCTAssertEqual(row.numbers(done: true).1, " loaded")
        XCTAssertEqual(row.appsFraction, 0.25)
        XCTAssertEqual(row.modelFraction, 0.25)
        XCTAssertEqual(row.targetFraction, 0.5)

        // Before a plan arrives there is nothing to say about the size.
        let bare = MemoryFill.Row(id: "mem", label: "Memory", total: 16 * g, model: 2 * g)
        XCTAssertEqual(bare.numbers(done: false).1, "")
        XCTAssertEqual(bare.appsFraction, 0)
        // A zero total must not divide by zero.
        XCTAssertEqual(MemoryFill.Row(id: "x", label: "x").targetFraction, 0)
    }

    // MARK: - Model mode replies

    /// Model mode reads the engine's list defensively: a missing or oddly
    /// shaped field is dropped, the rest of the list still arrives.
    func testMemoryAppDecoding() throws {
        let d = JSONDecoder()
        d.keyDecodingStrategy = .convertFromSnakeCase
        let full = #"""
        {"apps":[{"id":"chrome","name":"Google Chrome","exe":"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
          "pids":[12,13],"ram":8589934592,"gpu":1073741824,"windows":true,"kind":"app","note":"2 windows"},
         {"id":"ollama","name":"Ollama","kind":"ai","note":"another model server"},
         {"id":"odd","pids":"none","ram":"lots","windows":1,"gpu":-1}]}
        """#
        let apps = try d.decode(MemAppList.self, from: Data(full.utf8)).apps
        XCTAssertEqual(apps.map(\.id), ["chrome", "ollama", "odd"])
        XCTAssertEqual(apps[0].pids, [12, 13])
        XCTAssertEqual(apps[0].ram, 8 * g)
        XCTAssertEqual(apps[0].gpu, g)
        XCTAssertTrue(apps[0].windows)
        XCTAssertEqual(apps[1].kind, "ai")
        XCTAssertEqual(apps[1].ram, 0)
        XCTAssertFalse(apps[1].windows)
        XCTAssertEqual(apps[2].pids, [])
        XCTAssertEqual(apps[2].ram, 0)
        XCTAssertEqual(apps[2].windows, false)
        XCTAssertEqual(apps[2].gpu, 0) // a negative count is not a size
        XCTAssertEqual(apps[2].kind, "app")
    }

    func testCloseResultDecoding() throws {
        let d = JSONDecoder()
        d.keyDecodingStrategy = .convertFromSnakeCase
        let closed = try d.decode(CloseResult.self, from: Data(#"{"closed":true,"remaining":0,"freed":3221225472,"need_force":false}"#.utf8))
        XCTAssertTrue(closed.closed)
        XCTAssertEqual(closed.freed, 3 * g)
        XCTAssertFalse(closed.needForce)
        let busy = try d.decode(CloseResult.self, from: Data(#"{"closed":false,"remaining":2,"need_force":true}"#.utf8))
        XCTAssertFalse(busy.closed)
        XCTAssertEqual(busy.remaining, 2)
        XCTAssertTrue(busy.needForce)
        let empty = try d.decode(CloseResult.self, from: Data("{}".utf8))
        XCTAssertFalse(empty.closed)
        XCTAssertEqual(empty.freed, 0)
        XCTAssertFalse(empty.needForce)
    }

    // MARK: - Markdown edges

    /// A heading keeps a trailing "#" that is part of the words ("C#", "F#"),
    /// and drops the closing hashes CommonMark allows after a space.
    func testHeadingWords() {
        func head(_ s: String) -> MdBlock? { MdBlock.parse(s).first }
        XCTAssertEqual(head("# Title")?.text, "Title")
        XCTAssertEqual(head("### Title ###")?.text, "Title")
        XCTAssertEqual(head("## C#")?.text, "C#")
        XCTAssertEqual(head("## F# 7")?.text, "F# 7")
        XCTAssertEqual(head("###### Deep")?.level, 6)
        XCTAssertEqual(head("####### too deep")?.kind, .para) // seven hashes is a paragraph
        XCTAssertEqual(head("#hashtag")?.kind, .para)        // no space after the hash
    }

    /// Fences, tables, quotes and items: the shapes a model's answer actually
    /// takes, including Windows line endings and a fence that never closes.
    func testMarkdownEdges() {
        let kinds = MdBlock.parse("```sh\r\necho hi\r\n```\r\n\r\ntext\r\n").map(\.kind)
        XCTAssertEqual(kinds, [.code, .para])
        let crlf = MdBlock.parse("```sh\r\necho hi\r\n```\r\n")
        XCTAssertEqual(crlf.first?.text, "echo hi")
        XCTAssertEqual(crlf.first?.open, false)

        let tilde = MdBlock.parse("~~~\na\n~~~\nb")
        XCTAssertEqual(tilde.map(\.kind), [.code, .para])
        XCTAssertEqual(tilde[0].text, "a")
        XCTAssertEqual(tilde[0].lang, "")

        // A fence is closed by its own kind, not by the other one.
        let mixed = MdBlock.parse("```\n~~~\nstill code\n```")
        XCTAssertEqual(mixed.count, 1)
        XCTAssertEqual(mixed[0].text, "~~~\nstill code")
        XCTAssertFalse(mixed[0].open)

        // A table ends where the pipes stop.
        let table = MdBlock.parse("| a | b |\n|---|---|\n| 1 | 2 |\nafter")
        XCTAssertEqual(table.map(\.kind), [.table, .para])
        XCTAssertEqual(table[0].text.components(separatedBy: "\n").count, 3)

        // A blank line ends a quote; a quote keeps its own lines.
        let quote = MdBlock.parse("> one\n> two\n\nplain")
        XCTAssertEqual(quote.map(\.kind), [.quote, .para])
        XCTAssertEqual(quote[0].text, "one\ntwo")

        // Numbered items keep their number, however it was written.
        let items = MdBlock.parse("1. first\n2) second\n10. tenth\n    - deep")
        XCTAssertEqual(items.map(\.marker), ["1.", "2.", "10.", "•"])
        XCTAssertEqual(items[3].level, 2)
        XCTAssertEqual(items.map(\.text), ["first", "second", "tenth", "deep"])

        // Indented lines under an item continue it on one line.
        let wrapped = MdBlock.parse("- a long item\n  that wraps\n  twice")
        XCTAssertEqual(wrapped.count, 1)
        XCTAssertEqual(wrapped[0].text, "a long item that wraps twice")

        // Rules are rules, and a dash at the start of a line is an item.
        XCTAssertEqual(MdBlock.parse("***").map(\.kind), [.rule])
        XCTAssertEqual(MdBlock.parse("- - -").map(\.kind), [.rule])
        XCTAssertEqual(MdBlock.parse("---").map(\.kind), [.rule])
        XCTAssertEqual(MdBlock.parse("- not a rule").map(\.kind), [.item])
        XCTAssertEqual(MdBlock.parse("").count, 0)
        XCTAssertEqual(MdBlock.parse("\n\n").count, 0)
    }

    // MARK: - Helpers

    @MainActor
    private func unified(total: UInt64, avail: UInt64, plan: UInt64, loadDone: UInt64 = 0, loadTotal: UInt64 = 0) -> Live {
        var l = Live()
        l.unified = true
        l.ramTotal = total
        l.ramAvail = avail
        l.ramPlan = plan
        l.loadDone = loadDone
        l.loadTotal = loadTotal
        return l
    }

    @MainActor
    private func separate(gpuTotal: UInt64, gpuFree: UInt64, ramTotal: UInt64, ramAvail: UInt64, gpuPlan: UInt64, ramPlan: UInt64) -> Live {
        var l = Live()
        l.gpuTotal = gpuTotal
        l.gpuFree = gpuFree
        l.ramTotal = ramTotal
        l.ramAvail = ramAvail
        l.gpuPlan = gpuPlan
        l.ramPlan = ramPlan
        return l
    }
}
