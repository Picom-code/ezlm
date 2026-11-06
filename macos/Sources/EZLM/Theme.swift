import SwiftUI

enum Theme {
    static let black = Color.black
    static let ink = Color(white: 0.902)        // #e6e6e6
    static let dim = Color(white: 0.42)         // #6b6b6b
    static let faint = Color(white: 0.227)      // #3a3a3a
    static let rule = Color(white: 0.149)       // #262626
    static let signal = Color(red: 0.725, green: 0.659, blue: 1.0)  // #b9a8ff
    static let ok = Color(red: 0.561, green: 0.890, blue: 0.710)    // #8fe3b5
    static let bad = Color(red: 1.0, green: 0.561, blue: 0.525)     // #ff8f86
    static let gold = Color(red: 0.910, green: 0.784, blue: 0.447)  // #e8c872

    static func mono(_ size: CGFloat = 13, _ weight: Font.Weight = .regular) -> Font {
        .system(size: size, weight: weight, design: .monospaced)
    }
}

extension View {
    func mono(_ size: CGFloat = 13, _ weight: Font.Weight = .regular, _ color: Color = Theme.ink) -> some View {
        font(Theme.mono(size, weight)).foregroundStyle(color)
    }
}

/// Section label: dim, sentence case, no decoration.
struct SectionTitle: View {
    let text: String
    init(_ text: String) { self.text = text }
    var body: some View {
        Text(text).mono(13, .regular, Theme.dim)
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.top, 22).padding(.bottom, 8)
    }
}

/// Buttons read like key hints in a terminal.
struct TermButtonStyle: ButtonStyle {
    enum Kind { case primary, normal, small, danger }
    var kind: Kind = .normal
    @Environment(\.isEnabled) private var enabled

    func makeBody(configuration: Configuration) -> some View {
        let pressed = configuration.isPressed
        let label = configuration.label
            .font(Theme.mono(kind == .small ? 12 : 13, kind == .primary ? .bold : .regular))
            .padding(.horizontal, kind == .primary ? 20 : kind == .small ? 9 : 14)
            .padding(.vertical, kind == .primary ? 9 : kind == .small ? 3 : 6)
        switch kind {
        case .primary:
            return AnyView(label
                .foregroundStyle(enabled ? Theme.black : Theme.faint)
                .background(enabled ? (pressed ? Color.white : Theme.ink) : Theme.rule))
        case .danger:
            return AnyView(label
                .foregroundStyle(enabled ? Theme.bad : Theme.faint)
                .overlay(Rectangle().stroke(enabled ? Theme.bad.opacity(pressed ? 0.9 : 0.55) : Theme.rule, lineWidth: 1)))
        default:
            return AnyView(label
                .foregroundStyle(enabled ? (kind == .small ? Theme.dim : Theme.ink) : Theme.faint)
                .overlay(Rectangle().stroke(pressed ? Theme.dim : Theme.faint, lineWidth: 1)))
        }
    }
}

extension ButtonStyle where Self == TermButtonStyle {
    static var primary: TermButtonStyle { TermButtonStyle(kind: .primary) }
    static var term: TermButtonStyle { TermButtonStyle(kind: .normal) }
    static var small: TermButtonStyle { TermButtonStyle(kind: .small) }
    static var danger: TermButtonStyle { TermButtonStyle(kind: .danger) }
}
