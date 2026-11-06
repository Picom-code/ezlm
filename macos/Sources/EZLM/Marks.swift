import SwiftUI

// Block-pixel flags and vendor marks, drawn in the same ASCII/blocky language
// as the rest of the app. Marks are stylized evocations, not logo copies.

// MARK: - Pixel grid renderer

/// Draws rows of characters as square pixels; each character maps to a color.
struct PixelArt: View {
    let rows: [String]
    let palette: [Character: Color]
    var pixel: CGFloat = 2
    var gap: CGFloat = 0.4

    var body: some View {
        let w = CGFloat(rows.map(\.count).max() ?? 0), h = CGFloat(rows.count)
        Canvas { ctx, _ in
            for (y, row) in rows.enumerated() {
                for (x, ch) in row.enumerated() {
                    guard let c = palette[ch] else { continue }
                    ctx.fill(Path(CGRect(x: CGFloat(x) * pixel, y: CGFloat(y) * pixel,
                                         width: pixel - gap, height: pixel - gap)), with: .color(c))
                }
            }
        }
        .frame(width: w * pixel, height: h * pixel)
    }
}

// MARK: - Flags (16 × 10)

enum Flags {
    static let red = Color(red: 0.87, green: 0.16, blue: 0.16)
    static let yellow = Color(red: 1.0, green: 0.84, blue: 0.0)
    static let blue = Color(red: 0.0, green: 0.22, blue: 0.62)
    static let white = Color(white: 0.95)
    static let green = Color(red: 0.0, green: 0.55, blue: 0.27)
    static let black = Color(white: 0.08)
    static let gold = Color(red: 1.0, green: 0.8, blue: 0.0)

    static let names: [String: String] = [
        "CN": "China", "US": "United States", "FR": "France", "GB": "United Kingdom", "AE": "UAE",
        "CA": "Canada", "JP": "Japan", "KR": "South Korea", "DE": "Germany", "IL": "Israel",
        "IN": "India", "CH": "Switzerland", "SG": "Singapore", "NL": "Netherlands",
    ]

    static func art(_ code: String) -> ([String], [Character: Color])? {
        switch code.uppercased() {
        case "CN":
            return ([
                "rrrrrrrrrrrrrrrr",
                "rryrrryrrrrrrrrr",
                "ryyyrrrrrrrrrrrr",
                "yyyyyryrrrrrrrrr",
                "ryyyrrrrrrrrrrrr",
                "ryryrryrrrrrrrrr",
                "rrrrrrrrrrrrrrrr",
                "rrrrrrrrrrrrrrrr",
                "rrrrrrrrrrrrrrrr",
                "rrrrrrrrrrrrrrrr",
            ], ["r": red, "y": yellow])
        case "US":
            return ([
                "bbbbbbbrrrrrrrrr",
                "bwbwbwbwwwwwwwww",
                "bbbbbbbrrrrrrrrr",
                "bwbwbwbwwwwwwwww",
                "bbbbbbbrrrrrrrrr",
                "wwwwwwwwwwwwwwww",
                "rrrrrrrrrrrrrrrr",
                "wwwwwwwwwwwwwwww",
                "rrrrrrrrrrrrrrrr",
                "wwwwwwwwwwwwwwww",
            ], ["b": blue, "r": red, "w": white])
        case "FR":
            return (Array(repeating: "bbbbbwwwwwwrrrrr", count: 10), ["b": blue, "w": white, "r": red])
        case "DE":
            return (Array(repeating: "kkkkkkkkkkkkkkkk", count: 3) + Array(repeating: "rrrrrrrrrrrrrrrr", count: 4) + Array(repeating: "gggggggggggggggg", count: 3),
                    ["k": black, "r": red, "g": gold])
        case "GB":
            return ([
                "wbbbbbbrrbbbbbbw",
                "bwbbbbbrrbbbbbwb",
                "bbwbbbbrrbbbbwbb",
                "wwwwwwwrrwwwwwww",
                "rrrrrrrrrrrrrrrr",
                "rrrrrrrrrrrrrrrr",
                "wwwwwwwrrwwwwwww",
                "bbwbbbbrrbbbbwbb",
                "bwbbbbbrrbbbbbwb",
                "wbbbbbbrrbbbbbbw",
            ], ["b": blue, "r": red, "w": white])
        case "AE":
            return ([
                "rrrrgggggggggggg",
                "rrrrgggggggggggg",
                "rrrrgggggggggggg",
                "rrrrwwwwwwwwwwww",
                "rrrrwwwwwwwwwwww",
                "rrrrwwwwwwwwwwww",
                "rrrrwwwwwwwwwwww",
                "rrrrkkkkkkkkkkkk",
                "rrrrkkkkkkkkkkkk",
                "rrrrkkkkkkkkkkkk",
            ], ["r": red, "g": green, "w": white, "k": black])
        case "CA":
            return ([
                "rrrrwwwwwwwwrrrr",
                "rrrrwwwwrwwwrrrr",
                "rrrrwwwrrrwwrrrr",
                "rrrrwrwrrrwrrrrr",
                "rrrrwrrrrrrrrrrr",
                "rrrrwwrrrrrwrrrr",
                "rrrrwwwrrrwwrrrr",
                "rrrrwwwwrwwwrrrr",
                "rrrrwwwwrwwwrrrr",
                "rrrrwwwwwwwwrrrr",
            ], ["r": red, "w": white])
        case "JP":
            return ([
                "wwwwwwwwwwwwwwww",
                "wwwwwwwwwwwwwwww",
                "wwwwwwrrrrwwwwww",
                "wwwwwrrrrrrwwwww",
                "wwwwwrrrrrrwwwww",
                "wwwwwrrrrrrwwwww",
                "wwwwwrrrrrrwwwww",
                "wwwwwwrrrrwwwwww",
                "wwwwwwwwwwwwwwww",
                "wwwwwwwwwwwwwwww",
            ], ["w": white, "r": red])
        case "KR":
            return ([
                "wwwwwwwwwwwwwwww",
                "wkwwwwwwwwwwwkww",
                "wwkwwwrrrrwwkwww",
                "wwwwwrrrrrrwwwww",
                "wwwwwrrrrrrwwwww",
                "wwwwwbbbbbbwwwww",
                "wwwwwbbbbbbwwwww",
                "wwkwwwbbbbwwkwww",
                "wkwwwwwwwwwwwkww",
                "wwwwwwwwwwwwwwww",
            ], ["w": white, "r": red, "b": blue, "k": black])
        default:
            return nil
        }
    }
}

struct FlagView: View {
    let code: String
    var pixel: CGFloat = 1.5
    var body: some View {
        if let (rows, pal) = Flags.art(code) {
            PixelArt(rows: rows, palette: pal, pixel: pixel, gap: pixel * 0.18)
        } else {
            Text(code.uppercased()).mono(9, .bold, Theme.dim)
                .frame(width: 16 * pixel, height: 10 * pixel)
                .overlay(Rectangle().stroke(Theme.faint, lineWidth: 1))
        }
    }
}

/// Flag + country name, e.g. [▦] China
struct Origin: View {
    let code: String?
    var body: some View {
        if let code, !code.isEmpty {
            HStack(spacing: 6) {
                FlagView(code: code)
                Text(Flags.names[code.uppercased()] ?? code.uppercased()).mono(10, .regular, Theme.dim)
            }
        }
    }
}

// MARK: - Vendor marks (9 × 9)

enum Vendors {
    struct Mark { let rows: [String]; let color: Color; let short: String }

    static func mark(_ vendor: String?) -> Mark? {
        let v = (vendor ?? "").lowercased()
        func m(_ rows: [String], _ hex: UInt32, _ short: String) -> Mark {
            Mark(rows: rows, color: Color(red: Double((hex >> 16) & 0xff) / 255, green: Double((hex >> 8) & 0xff) / 255, blue: Double(hex & 0xff) / 255), short: short)
        }
        if v.contains("qwen") || v.contains("alibaba") {
            return m(["..#####..", ".##...##.", "##..#..##", "#..###..#", "#.##.##.#", "#..###..#", "##..#..##", ".##...##.", "..#####.."], 0x8B7CF6, "Qwen")
        }
        if v.contains("openai") {
            return m(["...###...", ".##...##.", ".#.###.#.", "#.#...#.#", "#.#.#.#.#", "#.#...#.#", ".#.###.#.", ".##...##.", "...###..."], 0xE8E8E8, "OpenAI")
        }
        if v.contains("google") || v.contains("gemma") {
            return m(["....#....", "....#....", "...###...", "..#####..", "#########", "..#####..", "...###...", "....#....", "....#...."], 0x4F8DF5, "Google")
        }
        if v.contains("deepseek") {
            return m([".........", "......##.", ".#...####", "##..#####", "#########", ".#######.", "..#####..", ".........", "........."], 0x4D6BFE, "DeepSeek")
        }
        if v.contains("zhipu") || v.contains("z.ai") || v.contains("glm") {
            return m(["#########", "#########", "......##.", ".....##..", "....##...", "...##....", "..##.....", "#########", "#########"], 0x3D7BFF, "Z.ai")
        }
        if v.contains("nvidia") {
            return m([".........", "..#####..", ".##...###", "##..#..##", "#..###..#", "##..#..##", ".##...###", "..#####..", "........."], 0x76B900, "NVIDIA")
        }
        if v.contains("prism") || v.contains("bonsai") {
            return m(["..#####..", ".#######.", "###.#####", ".#######.", "...###...", "....#....", "....#....", "..#####..", ".#######."], 0x5FBF77, "PrismML")
        }
        if v.contains("mistral") {
            return m(["##.....##", "###...###", "####.####", "##.###.##", "##..#..##", "##.....##", "##.....##", "##.....##", "##.....##"], 0xFF7000, "Mistral")
        }
        if v.contains("meta") {
            return m([".........", ".........", ".##...##.", "#..#.#..#", "#...#...#", "#..#.#..#", ".##...##.", ".........", "........."], 0x0081FB, "Meta")
        }
        if v.contains("microsoft") {
            return m(["####.####", "####.####", "####.####", "####.####", ".........", "####.####", "####.####", "####.####", "####.####"], 0x00A4EF, "Microsoft")
        }
        if v.contains("ibm") {
            return m(["#########", ".........", "#########", ".........", "#########", ".........", "#########", ".........", "#########"], 0x0F62FE, "IBM")
        }
        if v.contains("liquid") {
            return m(["....#....", "...###...", "..#####..", ".#######.", ".#######.", "#########", "#########", ".#######.", "..#####.."], 0x22D3EE, "Liquid")
        }
        if v.contains("moonshot") || v.contains("kimi") {
            return m(["...####..", "..##.....", ".##......", "##.......", "##.......", "##.......", ".##......", "..##.....", "...####.."], 0xE5E5E5, "Moonshot")
        }
        if v.contains("minimax") {
            return m([".........", "#...#...#", "##..#..##", "#.#.#.#.#", "#..###..#", "#...#...#", "#.......#", "#.......#", "........."], 0xF23F5D, "MiniMax")
        }
        if v.contains("nous") {
            return m(["##.....##", "###....##", "####...##", "##.##..##", "##..##.##", "##...####", "##....###", "##.....##", "##.....##"], 0xE8C872, "Nous")
        }
        return nil
    }
}

// MARK: - Real vendor logos (bundled PNGs), pixel marks as fallback

enum Logos {
    /// Vendor string from the catalog -> bundled logo file name.
    static func key(_ vendor: String?) -> String? {
        let v = (vendor ?? "").lowercased()
        let table: [(String, String)] = [
            ("qwen", "qwen"), ("alibaba", "qwen"), ("openai", "openai"), ("google", "google"), ("gemma", "google"),
            ("deepseek", "deepseek"), ("zhipu", "zhipu"), ("z.ai", "zhipu"), ("nvidia", "nvidia"), ("mistral", "mistral"),
            ("meta", "meta"), ("microsoft", "microsoft"), ("ibm", "ibm"), ("moonshot", "moonshot"), ("kimi", "moonshot"),
            ("minimax", "minimax"), ("nous", "nous"), ("tencent", "tencent"), ("xiaomi", "xiaomi"), ("liquid", "liquid"),
            ("prism", "prismml"), ("nanbeige", "nanbeige"), ("kanzhun", "nanbeige"),
        ]
        return table.first { v.contains($0.0) }?.1
    }

    private static var cache: [String: NSImage] = [:]

    static func image(_ vendor: String?) -> NSImage? {
        guard let k = key(vendor) else { return nil }
        return load(k)
    }

    /// The logo of an app EZLM connects to (Target.icon), app-<icon>.png.
    /// Its own name space, so "deepseek" the model vendor and "dsh" the app never mix.
    static func app(_ icon: String) -> NSImage? {
        guard !icon.isEmpty, icon.allSatisfy({ $0.isLetter || $0.isNumber || $0 == "-" }) else { return nil }
        return load("app-" + icon.lowercased())
    }

    private static var missing: Set<String> = []

    private static func load(_ k: String) -> NSImage? {
        if let img = cache[k] { return img }
        if missing.contains(k) { return nil }
        let candidates = [
            Bundle.main.url(forResource: k, withExtension: "png", subdirectory: "Logos"),
            ProcessInfo.processInfo.environment["EZLM_LOGOS"].map { URL(fileURLWithPath: $0).appendingPathComponent(k + ".png") },
        ]
        for case let url? in candidates {
            if let img = NSImage(contentsOf: url) { cache[k] = img; return img }
        }
        missing.insert(k)
        return nil
    }
}

struct VendorMark: View {
    let vendor: String?
    var pixel: CGFloat = 2
    var body: some View {
        if let img = Logos.image(vendor) {
            Image(nsImage: img)
                .resizable()
                .interpolation(.high)
                .aspectRatio(contentMode: .fit)
                .frame(width: 9 * pixel + 2, height: 9 * pixel + 2)
        } else if let mk = Vendors.mark(vendor) {
            PixelArt(rows: mk.rows, palette: ["#": mk.color], pixel: pixel, gap: pixel * 0.15)
        } else {
            Text(String((vendor ?? "?").prefix(1))).mono(11, .bold, Theme.dim)
                .frame(width: 9 * pixel, height: 9 * pixel)
                .overlay(Rectangle().stroke(Theme.faint, lineWidth: 1))
        }
    }
}

/// Model identity line: vendor mark, name, and origin flag.
struct ModelIdentity: View {
    let model: Model
    var nameSize: CGFloat = 13
    var body: some View {
        HStack(spacing: 9) {
            VendorMark(vendor: model.vendor, pixel: nameSize > 16 ? 2.6 : 1.9)
            Text(model.name).mono(nameSize, .bold)
            Spacer(minLength: 8)
            Origin(code: model.country)
        }
    }
}
