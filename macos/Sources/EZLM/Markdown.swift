import SwiftUI
import AppKit

/// A model's markdown answer drawn in EZLM's style: headings, lists, quotes,
/// code panels with light highlighting and a Copy button, and tables drawn
/// with box lines. Same rules as the Windows app.
struct MarkdownView: View {
    let text: String
    var cursor = false

    var body: some View {
        let blocks = MdBlock.parse(text)
        VStack(alignment: .leading, spacing: 10) {
            ForEach(Array(blocks.enumerated()), id: \.offset) { i, b in
                // Finished blocks compare equal frame to frame, so only the growing one redraws.
                MdBlockView(block: b, cursor: cursor && i == blocks.count - 1).equatable()
            }
            if blocks.isEmpty && cursor { Text("▌").mono(13, .regular, Theme.signal) }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

struct MdBlock: Equatable {
    enum Kind { case para, heading, item, quote, code, table, rule }
    var kind: Kind
    var text: String
    var level = 0
    var marker = ""
    var lang = ""
    var open = false

    private static func rx(_ p: String) -> NSRegularExpression { try! NSRegularExpression(pattern: p) }
    private static let heading = rx(#"^(#{1,6})\s+(.*?)(?:\s+#+)?\s*$"#), item = rx(#"^(\s*)([-*+]|\d{1,3}[.)])\s+(.*)$"#),
                       rule = rx(#"^\s*([-*_])(\s*\1){2,}\s*$"#), quote = rx(#"^\s*>\s?(.*)$"#)

    private static func match(_ r: NSRegularExpression, _ s: String) -> [String]? {
        guard let m = r.firstMatch(in: s, range: NSRange(s.startIndex..., in: s)) else { return nil }
        return (0..<m.numberOfRanges).map { i in Range(m.range(at: i), in: s).map { String(s[$0]) } ?? "" }
    }

    static func parse(_ text: String) -> [MdBlock] {
        let lines = text.replacingOccurrences(of: "\r\n", with: "\n").components(separatedBy: "\n")
        var out: [MdBlock] = []
        var i = 0
        while i < lines.count {
            let line = lines[i]
            let trimmed = line.trimmingCharacters(in: .whitespaces)
            if trimmed.hasPrefix("```") || trimmed.hasPrefix("~~~") {
                let fence = String(trimmed.prefix(3)), lang = trimmed.dropFirst(3).trimmingCharacters(in: .whitespaces)
                var code: [String] = []
                var closed = false
                i += 1
                while i < lines.count {
                    if lines[i].trimmingCharacters(in: .whitespaces).hasPrefix(fence) { closed = true; i += 1; break }
                    code.append(lines[i]); i += 1
                }
                out.append(MdBlock(kind: .code, text: code.joined(separator: "\n"), lang: lang, open: !closed))
                continue
            }
            if trimmed.isEmpty { i += 1; continue }
            if let h = match(heading, trimmed) {
                out.append(MdBlock(kind: .heading, text: h[2].trimmingCharacters(in: .whitespaces), level: h[1].count)); i += 1; continue
            }
            if match(rule, line) != nil { out.append(MdBlock(kind: .rule, text: "")); i += 1; continue }
            if trimmed.hasPrefix("|") {
                var rows: [String] = []
                while i < lines.count, lines[i].trimmingCharacters(in: .whitespaces).hasPrefix("|") { rows.append(lines[i].trimmingCharacters(in: .whitespaces)); i += 1 }
                out.append(MdBlock(kind: .table, text: rows.joined(separator: "\n")))
                continue
            }
            if match(quote, line) != nil {
                var q: [String] = []
                while i < lines.count, let m = match(quote, lines[i]) { q.append(m[1]); i += 1 }
                out.append(MdBlock(kind: .quote, text: q.joined(separator: "\n")))
                continue
            }
            if let it = match(item, line) {
                var body = it[3]
                let indent = it[1].replacingOccurrences(of: "\t", with: "    ").count
                i += 1
                // Lines indented under the item continue it.
                while i < lines.count, let f = lines[i].first, f.isWhitespace, match(item, lines[i]) == nil, !lines[i].trimmingCharacters(in: .whitespaces).isEmpty {
                    body += " " + lines[i].trimmingCharacters(in: .whitespaces); i += 1
                }
                let marker = it[2].first?.isNumber == true ? it[2].trimmingCharacters(in: CharacterSet(charactersIn: ".)")) + "." : "•"
                out.append(MdBlock(kind: .item, text: body, level: min(indent / 2, 4), marker: marker))
                continue
            }
            var para = [trimmed]
            i += 1
            while i < lines.count {
                let l = lines[i], t = l.trimmingCharacters(in: .whitespaces)
                if t.isEmpty || t.hasPrefix("```") || t.hasPrefix("~~~") || t.hasPrefix("|") || match(heading, t) != nil || match(item, l) != nil || match(quote, l) != nil || match(rule, l) != nil { break }
                para.append(t); i += 1
            }
            out.append(MdBlock(kind: .para, text: para.joined(separator: "\n")))
        }
        return out
    }
}

private let bodyColor = Color(white: 0.84)

/// Inline **bold**, *italic*, `code` and [links](https://…), with an optional cursor.
func inlineText(_ s: String, cursor: Bool, color: Color = bodyColor) -> Text {
    var a = (try? AttributedString(markdown: s, options: .init(interpretedSyntax: .inlineOnlyPreservingWhitespace))) ?? AttributedString(s)
    a.foregroundColor = color
    for run in a.runs {
        if let intent = run.inlinePresentationIntent {
            if intent.contains(.code) { a[run.range].foregroundColor = Theme.signal }
            if intent.contains(.stronglyEmphasized) { a[run.range].foregroundColor = .white }
        }
        if let link = run.link {
            if link.scheme == "http" || link.scheme == "https" {
                a[run.range].foregroundColor = Theme.signal
                a[run.range].underlineStyle = .single
            } else {
                a[run.range].link = nil // only web links open from an answer
            }
        }
    }
    if cursor {
        var c = AttributedString(" ▌")
        c.foregroundColor = Theme.signal
        a += c
    }
    return Text(a)
}

struct MdBlockView: View, Equatable {
    let block: MdBlock
    let cursor: Bool

    var body: some View {
        switch block.kind {
        case .heading:
            inlineText(block.text, cursor: cursor, color: .white)
                .font(Theme.mono(block.level <= 2 ? 16 : 14, .bold))
                .padding(.top, block.level <= 2 ? 8 : 4)
                .textSelection(.enabled)
                .environment(\.layoutDirection, layoutDirection(of: block.text))
        case .item:
            HStack(alignment: .firstTextBaseline, spacing: 10) {
                Text(block.marker).mono(13, .regular, block.marker == "•" ? Theme.signal : Theme.dim)
                inlineText(block.text, cursor: cursor).font(Theme.mono(13)).lineSpacing(5).textSelection(.enabled)
            }
            .padding(.leading, CGFloat(block.level) * 18)
            .environment(\.layoutDirection, layoutDirection(of: block.text))
        case .quote:
            HStack(alignment: .top, spacing: 12) {
                Rectangle().fill(Theme.faint).frame(width: 2)
                inlineText(block.text, cursor: cursor, color: Theme.dim).font(Theme.mono(13)).lineSpacing(5).textSelection(.enabled)
            }
            .fixedSize(horizontal: false, vertical: true)
            .environment(\.layoutDirection, layoutDirection(of: block.text))
        case .code:
            CodePanel(code: block.text, lang: block.lang, cursor: cursor && block.open)
        case .table:
            TableBlock(text: block.text)
        case .rule:
            Rectangle().fill(Theme.rule).frame(height: 1).padding(.vertical, 6)
        case .para:
            inlineText(block.text, cursor: cursor).font(Theme.mono(13)).lineSpacing(5).textSelection(.enabled)
                .fixedSize(horizontal: false, vertical: true)
                .environment(\.layoutDirection, layoutDirection(of: block.text))
        }
    }
}

/// A code panel: language and Copy on top, highlighted code below that
/// scrolls sideways instead of wrapping.
struct CodePanel: View {
    let code: String
    let lang: String
    let cursor: Bool
    @State private var copied = false

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack {
                Text(lang.isEmpty ? "code" : lang).mono(11, .regular, Theme.dim)
                Spacer()
                if !cursor {
                    Button(copied ? "Copied" : "Copy") {
                        NSPasteboard.general.clearContents()
                        NSPasteboard.general.setString(code, forType: .string)
                        copied = true
                        Task { try? await Task.sleep(nanoseconds: 1_400_000_000); copied = false }
                    }
                    .buttonStyle(.small)
                    .accessibilityLabel("Copy code")
                }
            }
            .padding(.horizontal, 12).padding(.vertical, 6)
            .background(Color(white: 0.078))
            ScrollView(.horizontal, showsIndicators: true) {
                // While it's still being written, plain text: highlighting a growing block every frame is what made long answers stutter.
                ((cursor ? Text(code).foregroundColor(bodyColor) : highlight(code, lang)) + (cursor ? Text(" ▌").foregroundColor(Theme.signal) : Text("")))
                    .font(Theme.mono(12.5))
                    .lineSpacing(4)
                    .fixedSize()
                    .textSelection(.enabled)
                    .padding(.horizontal, 14).padding(.vertical, 10)
            }
        }
        .background(Color(white: 0.055))
        .overlay(Rectangle().stroke(Theme.rule, lineWidth: 1))
    }
}

private let keywords: Set<String> = Set(("if else elif for while do return def class function func fn const let var mut import from export " +
    "public private protected static void int long float double string bool char byte true false null nil None True False async await new try catch " +
    "finally raise throw throws with as in is not and or package type struct interface enum impl use pub match case switch break continue default " +
    "yield lambda self this super extends implements namespace using select where echo then fi done local guard").split(separator: " ").map(String.init))

private let codeRx = try! NSRegularExpression(pattern: #"(//.*$|#(?![\w{]).*$|--\s.*$)|("(?:[^"\\]|\\.)*"?|'(?:[^'\\\n]|\\.)*'?|`[^`]*`?)|(\b\d+(?:\.\d+)?\b)|([A-Za-z_][A-Za-z0-9_]*)"#)

/// A light touch: keywords, strings, numbers and comments, whatever the language.
func highlight(_ code: String, _ lang: String) -> Text {
    let hashComments = ["", "python", "py", "bash", "sh", "shell", "zsh", "ruby", "rb", "yaml", "yml", "toml", "powershell", "ps1", "r", "perl"].contains(lang.lowercased())
    var out = AttributedString()
    let lines = code.components(separatedBy: "\n")
    for (li, line) in lines.enumerated() {
        if li > 0 { out += AttributedString("\n") }
        var at = line.startIndex
        let ns = NSRange(line.startIndex..., in: line)
        for m in codeRx.matches(in: line, range: ns) {
            guard let r = Range(m.range, in: line) else { continue }
            let word = String(line[r])
            var color: Color?
            if m.range(at: 1).location != NSNotFound, word.first != "#" || hashComments { color = Theme.dim }
            else if m.range(at: 2).location != NSNotFound { color = Theme.ok }
            else if m.range(at: 3).location != NSNotFound { color = Theme.gold }
            else if m.range(at: 4).location != NSNotFound, keywords.contains(word) { color = Theme.signal }
            guard let c = color else { continue }
            if r.lowerBound > at { var plain = AttributedString(String(line[at..<r.lowerBound])); plain.foregroundColor = bodyColor; out += plain }
            var piece = AttributedString(word)
            piece.foregroundColor = c
            out += piece
            at = r.upperBound
        }
        if at < line.endIndex { var plain = AttributedString(String(line[at...])); plain.foregroundColor = bodyColor; out += plain }
    }
    return Text(out)
}

/// Tables as box drawings: columns padded to line up, header set apart.
/// Widths are counted the way the monospaced font draws them, so a Chinese
/// character takes two columns and a combining mark none.
struct TableBlock: View {
    let text: String

    var body: some View {
        let t = TableLayout(text)
        return ScrollView(.horizontal, showsIndicators: false) {
            VStack(alignment: .leading, spacing: 0) {
                if !t.rows.isEmpty {
                    Text(t.rule("┌", "┬", "┐")).foregroundColor(Theme.dim)
                    Text(t.row(t.rows[0])).bold().foregroundColor(.white)
                    if t.rows.count > 1 { Text(t.rule("├", "┼", "┤")).foregroundColor(Theme.dim) }
                    ForEach(Array(t.rows.dropFirst().enumerated()), id: \.offset) { _, r in Text(t.row(r)).foregroundColor(bodyColor) }
                    Text(t.rule("└", "┴", "┘")).foregroundColor(Theme.dim)
                }
            }
            .font(Theme.mono(13))
            .textSelection(.enabled)
            .fixedSize()
        }
    }
}

/// The rows of a markdown table and the width of each column, in font columns.
struct TableLayout {
    static let maxColumn = 48
    let rows: [[String]]
    let widths: [Int]

    init(_ text: String) {
        let rows = text.components(separatedBy: "\n")
            .map { $0.trimmingCharacters(in: .whitespaces).trimmingCharacters(in: CharacterSet(charactersIn: "|")).components(separatedBy: "|").map { $0.trimmingCharacters(in: .whitespaces) } }
            .filter { !$0.allSatisfy { $0.range(of: #"^:?-{2,}:?$"#, options: .regularExpression) != nil } }
        var w = [Int](repeating: 0, count: rows.map(\.count).max() ?? 0)
        for r in rows { for (c, cell) in r.enumerated() { w[c] = min(Self.maxColumn, max(w[c], displayWidth(cell))) } }
        self.rows = rows
        self.widths = w
    }

    func rule(_ l: String, _ m: String, _ r: String) -> String {
        l + widths.map { String(repeating: "─", count: $0 + 2) }.joined(separator: m) + r
    }

    /// The cell, cut to its column with an ellipsis when it doesn't fit there.
    static func clip(_ cell: String, to width: Int) -> String {
        guard displayWidth(cell) > width else { return cell }
        var out = "", used = 0
        for ch in cell {
            let w = displayWidth(ch)
            if used + w > max(0, width - 1) { break }
            out.append(ch)
            used += w
        }
        return out + "…"
    }

    func row(_ r: [String]) -> String {
        "│" + (0..<widths.count).map { c in
            let cell = Self.clip(c < r.count ? r[c] : "", to: widths[c])
            return " " + cell + String(repeating: " ", count: max(0, widths[c] - displayWidth(cell))) + " "
        }.joined(separator: "│") + "│"
    }
}

/// How many columns text takes in a monospaced font: two for a Chinese,
/// Japanese or Korean character and for an emoji, none for a mark that sits on
/// the letter before it, one for everything else. Counting characters instead
/// makes every table with those scripts lean out of its box.
func displayWidth(_ s: String) -> Int { s.reduce(0) { $0 + displayWidth($1) } }

func displayWidth(_ c: Character) -> Int {
    if c.unicodeScalars.contains(where: { isWide($0.value) }) { return 2 }
    if c.unicodeScalars.allSatisfy({ isZeroWidth($0.value) }) { return 0 }
    return 1
}

private func isWide(_ v: UInt32) -> Bool {
    (0x1100...0x115F).contains(v) || v == 0x2329 || v == 0x232A
        || (0x2E80...0x303E).contains(v) || (0x3041...0x33FF).contains(v)
        || (0x3400...0x4DBF).contains(v) || (0x4E00...0x9FFF).contains(v)
        || (0xA000...0xA4CF).contains(v) || (0xA960...0xA97F).contains(v)
        || (0xAC00...0xD7A3).contains(v) || (0xF900...0xFAFF).contains(v)
        || (0xFE10...0xFE19).contains(v) || (0xFE30...0xFE6F).contains(v)
        || (0xFF00...0xFF60).contains(v) || (0xFFE0...0xFFE6).contains(v)
        || (0x1F000...0x1F02F).contains(v) || (0x1F300...0x1F64F).contains(v)
        || (0x1F900...0x1F9FF).contains(v) || (0x20000...0x3FFFD).contains(v)
}

private func isZeroWidth(_ v: UInt32) -> Bool {
    (0x0300...0x036F).contains(v) || (0x0483...0x0489).contains(v)
        || (0x0591...0x05BD).contains(v) || v == 0x05BF || (0x05C1...0x05C2).contains(v)
        || (0x0610...0x061A).contains(v) || (0x064B...0x065F).contains(v) || v == 0x0670
        || (0x06D6...0x06ED).contains(v) || (0x0711...0x0711).contains(v)
        || (0x0E31...0x0E3A).contains(v) || (0x0E47...0x0E4E).contains(v)
        || (0x1AB0...0x1AFF).contains(v) || (0x1DC0...0x1DFF).contains(v)
        || (0x200B...0x200F).contains(v) || (0x202A...0x202E).contains(v)
        || (0x2060...0x2069).contains(v) || (0x20D0...0x20FF).contains(v)
        || (0xFE00...0xFE0F).contains(v) || (0xFE20...0xFE2F).contains(v)
}

/// Big letters drawn with half blocks (5 dots tall, 3 text rows), for the
/// model's name at the top of an empty chat.
enum BigType {
    private static let g: [Character: String] = [
        "A": ".##.|#..#|####|#..#|#..#", "B": "###.|#..#|###.|#..#|###.", "C": ".###|#...|#...|#...|.###",
        "D": "###.|#..#|#..#|#..#|###.", "E": "####|#...|###.|#...|####", "F": "####|#...|###.|#...|#...",
        "G": ".###|#...|#.##|#..#|.###", "H": "#..#|#..#|####|#..#|#..#", "I": "###|.#.|.#.|.#.|###",
        "J": "...#|...#|...#|#..#|.##.", "K": "#..#|#.#.|##..|#.#.|#..#", "L": "#...|#...|#...|#...|####",
        "M": "#...#|##.##|#.#.#|#...#|#...#", "N": "#..#|##.#|#.##|#..#|#..#", "O": ".##.|#..#|#..#|#..#|.##.",
        "P": "###.|#..#|###.|#...|#...", "Q": ".##.|#..#|#..#|#.#.|.#.#", "R": "###.|#..#|###.|#.#.|#..#",
        "S": ".###|#...|.##.|...#|###.", "T": "###|.#.|.#.|.#.|.#.", "U": "#..#|#..#|#..#|#..#|.##.",
        "V": "#...#|#...#|.#.#.|.#.#.|..#..", "W": "#...#|#...#|#.#.#|##.##|#...#", "X": "#..#|#..#|.##.|#..#|#..#",
        "Y": "#...#|.#.#.|..#..|..#..|..#..", "Z": "####|...#|.##.|#...|####",
        "0": ".##.|#.##|##.#|#..#|.##.", "1": ".#.|##.|.#.|.#.|###", "2": "###.|...#|.##.|#...|####",
        "3": "###.|...#|.##.|...#|###.", "4": "#..#|#..#|####|...#|...#", "5": "####|#...|###.|...#|###.",
        "6": ".##.|#...|###.|#..#|.##.", "7": "####|...#|..#.|.#..|.#..", "8": ".##.|#..#|.##.|#..#|.##.",
        "9": ".##.|#..#|.###|...#|.##.", ".": ".|.|.|.|#", "-": "...|...|###|...|...", " ": "..|..|..|..|..",
        "/": "..#|..#|.#.|#..|#..", ":": ".|#|.|#|.",
    ]

    /// The words of a model name that fit in about max characters.
    static func fit(_ name: String, max: Int = 14) -> String {
        var out = ""
        for w in name.split(separator: " ") {
            let next = out.isEmpty ? String(w) : out + " " + w
            if next.count > max && !out.isEmpty { break }
            out = next
        }
        return String(out.prefix(max))
    }

    static func lines(_ text: String) -> [String] {
        let glyphs = text.uppercased().compactMap { g[$0]?.components(separatedBy: "|") }
        let rows = (0..<6).map { r in glyphs.map { (r < 5 ? $0[r] : String(repeating: ".", count: $0[0].count)) + "." }.joined() }
        return stride(from: 0, to: 6, by: 2).map { r in
            String(zip(rows[r], rows[r + 1]).map { a, b in
                switch (a == "#", b == "#") { case (true, true): return "█"; case (true, false): return "▀"; case (false, true): return "▄"; default: return " " }
            })
        }
    }
}
