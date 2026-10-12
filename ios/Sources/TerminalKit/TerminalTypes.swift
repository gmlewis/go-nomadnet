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

/// A color a cell can be drawn in.
///
/// The three cases are the three ways a terminal names a color, and they are
/// kept apart rather than resolved on the spot because the resolution needs the
/// palette, which belongs to the view and not to the parser. A cell that says
/// `.default` has to follow a theme change; one that says `.rgb` must not.
public enum TerminalColor: Equatable, Hashable, Sendable {
    /// The terminal's own foreground or background, which the theme decides.
    case `default`

    /// An index into the 256-color palette: 0-15 are the ANSI colors, 16-231
    /// the 6x6x6 cube, and 232-255 a 24-step gray ramp.
    case palette(Int)

    /// A 24-bit color, each component 0-255.
    case rgb(r: Int, g: Int, b: Int)

    /// The nearest palette index for the case, or nil for a color that has no
    /// palette entry.
    public var paletteIndex: Int? {
        switch self {
        case .default: return nil
        case .palette(let index): return index
        case .rgb: return nil
        }
    }
}

/// How a cell is drawn: its colors and its attributes.
///
/// This is the state the escape parser carries from one cell to the next. It is
/// a value type on purpose — a screen full of cells shares one pen, and copying
/// it into a cell has to be the end of the relationship between them.
public struct TerminalPen: Equatable, Hashable, Sendable {
    /// The color the glyph is drawn in.
    public var foreground: TerminalColor

    /// The color behind the glyph.
    public var background: TerminalColor

    /// Bold, which the renderer draws as a heavier face and never as a color.
    public var bold: Bool

    /// Italic, drawn as a skew when the face has no italic cut.
    public var italic: Bool

    /// Underlined, drawn as a rule under the glyph.
    public var underline: Bool

    /// Blinking, which the view alternates rather than the screen.
    public var blink: Bool

    /// Reverse video: the foreground and background are exchanged when drawn, so
    /// the stored colors stay the ones the program asked for.
    public var reverse: Bool

    /// The pen a fresh terminal starts with.
    public static let plain = TerminalPen(
        foreground: .default,
        background: .default,
        bold: false,
        italic: false,
        underline: false,
        blink: false,
        reverse: false
    )

    public init(
        foreground: TerminalColor = .default,
        background: TerminalColor = .default,
        bold: Bool = false,
        italic: Bool = false,
        underline: Bool = false,
        blink: Bool = false,
        reverse: Bool = false
    ) {
        self.foreground = foreground
        self.background = background
        self.bold = bold
        self.italic = italic
        self.underline = underline
        self.blink = blink
        self.reverse = reverse
    }
}

/// One position on the screen.
///
/// A cell is either a blank the terminal has never written to, or a glyph with
/// the pen it was written in. `width` is how many columns it occupies: 2 for an
/// East-Asian wide glyph, whose cell carries the glyph and whose neighbor is a
/// spacer with an empty glyph.
public struct TerminalCell: Equatable, Hashable, Sendable {
    /// The glyph drawn at this position. Empty means the position is a spacer
    /// belonging to the wide glyph to its left.
    public var glyph: String

    /// How the glyph is drawn.
    public var pen: TerminalPen

    /// How many columns the glyph occupies: 1, or 2 for a wide glyph.
    public var width: Int

    /// A cell the terminal has not written to.
    public static let blank = TerminalCell(glyph: " ", pen: .plain, width: 1)

    /// The spacer cell that follows a wide glyph.
    public static let spacer = TerminalCell(glyph: "", pen: .plain, width: 0)

    public init(glyph: String, pen: TerminalPen, width: Int) {
        self.glyph = glyph
        self.pen = pen
        self.width = width
    }

    /// Whether this cell is a blank the terminal has never written to.
    public var isBlank: Bool { glyph == " " && pen == .plain }

    /// Whether this cell is the spacer half of a wide glyph.
    public var isSpacer: Bool { width == 0 || glyph.isEmpty }
}
