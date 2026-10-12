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

/// A mouse button, as far as this terminal can report one.
///
/// A finger has no buttons and a trackpad's are its own business, so the left button is
/// the one a tap sends: it is the one that selects and activates. The middle and right
/// buttons are here because the wire has numbers for them, and a terminal that could name
/// only one button could not spell the other two.
///
/// The raw values are the wire's own, which are not the order the buttons are named in:
/// the wheel has no buttons at all, and it is numbered 64 and 65 above the three real
/// ones.
public enum TerminalMouseButton: Int, CaseIterable, Sendable {
    /// The primary button, which is what a tap is.
    case left = 0

    /// The middle button of a three-button mouse.
    case middle = 1

    /// The secondary button.
    case right = 2

    /// The wheel, one notch up. A drag over the client's interface is sent as this.
    case wheelUp = 64

    /// The wheel, one notch down.
    case wheelDown = 65
}

/// Spells a mouse event the way a terminal sends one.
///
/// The client is a terminal program reading a pseudo-terminal, so a tap has to reach it as
/// the byte sequence a real terminal writes when its mouse is clicked — there is no other
/// way to tell it that its list was touched. Which sequence that is, the client says
/// itself: it turns mouse reporting on with `CSI ?1000h` (and `?1002h` or `?1003h` for
/// motion) and asks for the SGR encoding on top of it with `CSI ?1006h`.
///
/// That last one is the whole difference between the two forms, and it is not a detail: the
/// older X10 form packs a cell into one byte, so it cannot report a column past 223, which
/// is a third of the way across a tablet's screen.
public enum TerminalMouse {

    /// How the X10 form says a button came up: it has no button number of its own for a
    /// release, so it is button three.
    public static let releaseButton = 3

    /// What both encodings add to a value to make it printable and unambiguous.
    public static let offset = 32

    /// The bytes one event is sent as, in the encoding the client asked for.
    ///
    /// A report is one event and not a click: the client makes a click out of a press and
    /// a release in the same cell, so a caller that sends only the press has moved the
    /// client's focus and activated nothing.
    public static func encode(
        button: TerminalMouseButton, col: Int, row: Int, pressed: Bool, sgr: Bool
    ) -> [UInt8] {
        sgr
            ? self.sgr(button: button, col: col, row: row, pressed: pressed)
            : x10(button: button, col: col, row: row, pressed: pressed)
    }

    /// The SGR form: `CSI < button ; column ; row M`, with a lower-case `m` in place of the
    /// `M` when the button came up.
    ///
    /// The numbers are decimal and are what the client's own terminal library reads, so a
    /// cell of any size can be reported and no byte ever runs out.
    public static func sgr(
        button: TerminalMouseButton, col: Int, row: Int, pressed: Bool
    ) -> [UInt8] {
        let terminator = pressed ? "M" : "m"
        let body = "<\(button.rawValue);\(cell(col) + 1);\(cell(row) + 1)\(terminator)"
        return [0x1B, 0x5B] + Array(body.utf8)
    }

    /// The older form: `CSI M` followed by the button, the column and the row, each as one
    /// byte carrying ``offset`` plus its value.
    ///
    /// A button that has come up has no number of its own here — the form predates the idea
    /// — so it is ``releaseButton``. A cell that does not fit in a byte is clamped rather
    /// than wrapped: a wrapped coordinate is a different cell, which is a click somewhere
    /// the reader never touched.
    public static func x10(
        button: TerminalMouseButton, col: Int, row: Int, pressed: Bool
    ) -> [UInt8] {
        [
            0x1B, 0x5B, 0x4D,
            packed(pressed ? button.rawValue : releaseButton),
            packed(cell(col) + 1),
            packed(cell(row) + 1),
        ]
    }

    /// A cell as both encodings carry it.
    ///
    /// The cell is the client's own, counted from its top left corner and zero-based, as
    /// every other cell in the emulator is counted; both encodings carry it one-based, and
    /// that is the encoder's business. A cell before the corner is clamped onto it rather
    /// than written as a negative number, and the clamp is applied identically to a press
    /// and its release so that the two always name one cell.
    static func cell(_ value: Int) -> Int { max(0, value) }

    /// One X10 value as a byte.
    static func packed(_ value: Int) -> UInt8 {
        UInt8(min(max(value + offset, 0), 0xFF))
    }
}
