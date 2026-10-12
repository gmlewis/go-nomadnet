// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

import Foundation

/// The colors the console is drawn in.
///
/// A terminal screen holds color *names* — a palette index, a 24-bit value, or
/// "whatever the terminal uses" — because that is what the client says in its escape
/// sequences. A view draws pixels, so the names have to be resolved, and this is the
/// whole of that: the 256-entry table every terminal agrees on, and the theme's own
/// two colors for the cells the client left alone.
///
/// The client is run with `COLORTERM=truecolor`, so most of what it draws arrives as
/// 24-bit and is passed through untouched. The table still matters for the palettes
/// the client deliberately asks for, and the default pair matters for everything
/// else: the client draws its frame borders, pane titles and key hints in the
/// terminal's own colors, so a mismatch there is an interface drawn in the wrong
/// colors rather than a visible error.
///
/// The theme is the appliance's own Gruvbox dark, and there is deliberately only one
/// of it: the `colors.properties` the client installs under Termux is the file the
/// constants below are taken from, and ``parseTheme(_:)`` reads that same file rather
/// than trusting a second copy, so the console and the published theme cannot drift
/// apart into two terminals that look different.
public struct TerminalPalette: Equatable, Sendable {

    /// The color behind everything, and behind a cell the client gave no background.
    ///
    /// It is the published theme's own `background`, so a cell the client left alone
    /// is drawn exactly as the Termux theme would draw it.
    public static let defaultBackground: UInt32 = 0xFF00_0000

    /// The color of text the client gave no color for.
    public static let defaultForeground: UInt32 = 0xFF00_FF00

    /// The alpha every color is given: a translucent glyph is an unreadable one.
    static let opaque: UInt32 = 0xFF00_0000

    /// The shipped theme, which is the theme every instance starts from.
    public static let shipped = TerminalPalette()

    /// The color of text the client gave no color for, in this theme.
    public let foreground: UInt32

    /// The color behind text the client gave no color for, in this theme.
    public let background: UInt32

    /// The first sixteen palette entries, in the order every terminal has them.
    public let ansi: [UInt32]

    /// The appliance's own theme: a black background, a bright green foreground, and
    /// the Gruvbox sixteen.
    public init() {
        // The first eight are the hues every terminal has and the second eight the
        // same eight a step brighter. They are the published theme's `color0` to
        // `color15` rather than the VGA hues a terminal falls back to without one.
        self.init(
            foreground: TerminalPalette.defaultForeground,
            background: TerminalPalette.defaultBackground,
            ansi: [
                0xFF1D_2021,  // 0 black
                0xFFCC_241D,  // 1 red
                0xFF98_971A,  // 2 green
                0xFFD7_9921,  // 3 yellow
                0xFF45_8588,  // 4 blue
                0xFFB1_6286,  // 5 magenta
                0xFF68_9D6A,  // 6 cyan
                0xFFA8_9984,  // 7 white
                0xFF7C_6F64,  // 8 bright black
                0xFFFB_4934,  // 9 bright red
                0xFFB8_BB26,  // 10 bright green
                0xFFFA_BD2F,  // 11 bright yellow
                0xFF83_A598,  // 12 bright blue
                0xFFD3_869B,  // 13 bright magenta
                0xFF8E_C07C,  // 14 bright cyan
                0xFFFB_F1C7,  // 15 bright white
            ])
    }

    /// A palette drawn from an already-resolved theme.
    init(foreground: UInt32, background: UInt32, ansi: [UInt32]) {
        self.foreground = foreground
        self.background = background
        self.ansi = ansi
    }

    /// The color a cell's text is drawn in, or the one behind it when `isBackground`.
    ///
    /// The two resolutions differ only in what "the terminal's own color" means, which
    /// is the whole point of resolving them separately: a cell with no color of its own
    /// has the theme's foreground as its text and the theme's background behind it.
    ///
    /// An index past the end of the table is clamped rather than refused: it comes
    /// from a client asking for a color that does not exist, and a terminal that
    /// fails on a color escape is worse than one that draws the nearest thing it has.
    public func argb(_ color: TerminalColor, isBackground: Bool) -> UInt32 {
        switch color {
        case .default:
            return isBackground ? background : foreground
        case .rgb(let r, let g, let b):
            // Anything above the 24 bits a color has must not leak into the alpha
            // byte: a fully transparent glyph is an invisible one.
            return TerminalPalette.opaque
                | (UInt32(r & 0xFF) << 16) | (UInt32(g & 0xFF) << 8) | UInt32(b & 0xFF)
        case .palette(let index):
            return paletteEntry(index)
        }
    }

    /// The two colors a pen draws its cell with, reverse video included.
    ///
    /// Reverse is a swap of the resolved pair and not of the names, because that is
    /// what a terminal does: a cell with no colors of its own and reverse set is
    /// drawn in the default background on the default foreground.
    public func colors(of pen: TerminalPen) -> (foreground: UInt32, background: UInt32) {
        let text = argb(pen.foreground, isBackground: false)
        let behind = argb(pen.background, isBackground: true)
        guard pen.reverse else { return (foreground: text, background: behind) }
        return (foreground: behind, background: text)
    }

    /// One entry of the 256-color palette, in the order every terminal has them.
    public func paletteEntry(_ index: Int) -> UInt32 {
        let i = min(max(index, 0), 255)
        if i < ansi.count { return ansi[i] }
        if i < 232 {
            // Entries 16 to 231 are a 6x6x6 cube, red varying slowest and blue fastest.
            let cube = i - 16
            let r = TerminalPalette.cubeSteps[cube / 36]
            let g = TerminalPalette.cubeSteps[(cube / 6) % 6]
            let b = TerminalPalette.cubeSteps[cube % 6]
            return TerminalPalette.opaque | (UInt32(r) << 16) | (UInt32(g) << 8) | UInt32(b)
        }
        // The last twenty-four are an even gray ramp from nearly black to nearly white.
        let gray = 8 + 10 * (i - 232)
        return TerminalPalette.opaque | (UInt32(gray) << 16) | (UInt32(gray) << 8) | UInt32(gray)
    }

    /// The six levels the color cube's axes step through.
    ///
    /// They are the values terminals have used for decades and they are not evenly
    /// spaced: the first step is wider, because a cube with even steps has no
    /// near-black in it.
    static let cubeSteps = [0x00, 0x5F, 0x87, 0xAF, 0xD7, 0xFF]

    /// The theme a Termux-format `colors.properties` describes, or nil if it cannot be
    /// read as one.
    ///
    /// Termux's format is one `key = value` per line with each color written
    /// `#rrggbb`, and a comment is a line that starts with `#`. The `#` inside a value
    /// is part of the value, so the split is on the first `=` rather than on the hash.
    ///
    /// A theme that does not name both its own two colors and all sixteen palette
    /// entries is refused rather than half-read: a client asking for entry 7 of a
    /// theme with no entry 7 would be drawn in a color the publisher never chose,
    /// and nothing about that looks like a bug in the code. `cursor` is accepted and
    /// ignored, because a terminal draws its cursor as the cell's own colors
    /// reversed rather than in the cursor color.
    public static func parseTheme(_ text: String) -> TerminalPalette? {
        var values: [String: UInt32] = [:]
        for raw in text.split(separator: "\n", omittingEmptySubsequences: false) {
            let line = raw.trimmingCharacters(in: .whitespacesAndNewlines)
            guard !line.isEmpty, !line.hasPrefix("#") else { continue }
            guard let equals = line.firstIndex(of: "=") else { continue }
            let key = line[line.startIndex..<equals]
                .trimmingCharacters(in: .whitespacesAndNewlines)
            let value = line[line.index(after: equals)...]
                .trimmingCharacters(in: .whitespacesAndNewlines)
            guard let color = themeColor(value) else { continue }
            values[key] = color
        }
        guard let foreground = values["foreground"], let background = values["background"] else {
            return nil
        }
        var ansi = [UInt32]()
        ansi.reserveCapacity(16)
        for index in 0..<16 {
            guard let entry = values["color\(index)"] else { return nil }
            ansi.append(entry)
        }
        return TerminalPalette(foreground: foreground, background: background, ansi: ansi)
    }

    /// One `#rrggbb` value as an opaque color, or nil if it is not one.
    static func themeColor(_ value: String) -> UInt32? {
        let digits = value.hasPrefix("#") ? String(value.dropFirst()) : value
        guard digits.count == 6, let rgb = UInt32(digits, radix: 16) else { return nil }
        return opaque | rgb
    }
}
