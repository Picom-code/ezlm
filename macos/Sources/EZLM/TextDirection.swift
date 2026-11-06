import SwiftUI

/// Which way a piece of text reads, from its first letter that has a
/// direction. Digits and punctuation alone say nothing, so they're skipped;
/// text with no letters at all reads left to right. Chat uses it to draw an
/// Arabic or Hebrew answer from the right even though the app is in English.
func readsRightToLeft(_ s: String) -> Bool {
    for u in s.unicodeScalars {
        switch u.value {
        case 0x0590...0x08FF,          // Hebrew, Arabic, Syriac, Thaana, NKo, Samaritan, Mandaic
             0xFB1D...0xFDFF,          // Hebrew and Arabic presentation forms
             0xFE70...0xFEFF,
             0x10800...0x10FFF,        // old Italic to Hatran, all right to left
             0x1E800...0x1EFFF:        // Mende Kikakui, Adlam
            return true
        case 0x41...0x5A, 0x61...0x7A, 0xC0...0x24F, 0x370...0x3FF, 0x400...0x58F,
             0x10A0...0x10FF, 0x1E00...0x1EFF, 0x2C80...0x2CFF,
             0x3040...0x30FF, 0x3400...0x9FFF, 0xAC00...0xD7A3, 0xFF21...0xFF3A, 0xFF41...0xFF5A:
            return false
        default:
            continue
        }
    }
    return false
}

/// The layout direction for a run of text, for the views that draw it.
func layoutDirection(of s: String) -> LayoutDirection { readsRightToLeft(s) ? .rightToLeft : .leftToRight }
