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

import XCTest

@testable import TerminalKit

/// What the appliance sends the client when the console is tapped.
///
/// Both encodings a terminal can use are here, because the client says which it wants: SGR
/// (`CSI ?1006h`) carries the button and both coordinates as decimal numbers, and the older
/// X10 form packs them into three bytes with 32 added to each. A tablet's client asks for
/// SGR — the coordinates can then pass column 223 — but a terminal that only ever wrote one
/// of them would be a terminal that ignored what the client asked for.
///
/// The cells here are the client's own, counted from its top-left corner and zero-based, as
/// everything in the emulator counts them. Both encodings carry them one-based, because the
/// wire's first cell is 1: that conversion is the encoder's, and it is the one thing about
/// these vectors worth reading twice.
final class TerminalMouseTests: XCTestCase {

    private let esc = 0x1B

    /// sent renders what one report is spelled as, as integers, at the console's cell.
    private func sent(
        _ button: TerminalMouseButton, col: Int, row: Int, pressed: Bool, sgr: Bool = true
    ) -> [Int] {
        TerminalMouse.encode(button: button, col: col, row: row, pressed: pressed, sgr: sgr)
            .map { Int($0) }
    }

    /// x10Sent is the older form's own three packed bytes, with its `CSI M` lead-in
    /// checked once rather than in every vector.
    private func x10Sent(
        _ button: TerminalMouseButton, col: Int, row: Int, pressed: Bool
    ) -> [Int] {
        let whole = sent(button, col: col, row: row, pressed: pressed, sgr: false)
        XCTAssertEqual(Array(whole.prefix(3)), [esc, 0x5B, 0x4D], "the X10 lead-in")
        return Array(whole.suffix(3))
    }

    func testAPressAndAReleaseAreBothSentAndAreWhatMakesAClick() {
        // The client derives a click from a press and a release in the same cell: a
        // terminal that sent only the press would move the client's focus and activate
        // nothing, which is exactly a console where tapping an entry does nothing at all.
        XCTAssertEqual(
            sent(.left, col: 2, row: 4, pressed: true),
            [esc, 0x5B, 0x3C] + "0;3;5M".map { Int($0.asciiValue!) })
        XCTAssertEqual(
            sent(.left, col: 2, row: 4, pressed: false),
            [esc, 0x5B, 0x3C] + "0;3;5m".map { Int($0.asciiValue!) })
    }

    func testTheFirstCellOfTheScreenIsOneNotZero() {
        // Both encodings count from one. A terminal that sent the zero-based cell would
        // put every click one cell up and to the left, which on a menu bar is the next
        // entry.
        XCTAssertEqual(
            sent(.left, col: 0, row: 0, pressed: true),
            [esc, 0x5B, 0x3C] + "0;1;1M".map { Int($0.asciiValue!) })
    }

    func testTheButtonsKeepTheNumbersTheWireGivesThem() {
        let cases: [(String, TerminalMouseButton, Int)] = [
            ("left", .left, 0),
            // The wire's numbers are not the order the buttons are named in: the middle
            // button is 1 and the right one is 2.
            ("middle", .middle, 1),
            ("right", .right, 2),
        ]
        for (name, button, number) in cases {
            XCTAssertEqual(
                sent(button, col: 6, row: 8, pressed: true),
                [esc, 0x5B, 0x3C] + "\(number);7;9M".map { Int($0.asciiValue!) },
                "\(name): the button's number")
        }
    }

    func testACellPastTheTwoHundredAndTwentyThirdColumnIsStillSentInFull() {
        // The X10 form cannot express a column past 223: its coordinates are one byte with
        // 32 added, and the byte runs out. SGR writes them as numbers, which is why the
        // client asks for it on a screen this wide.
        XCTAssertEqual(
            sent(.left, col: 299, row: 39, pressed: true),
            [esc, 0x5B, 0x3C] + "0;300;40M".map { Int($0.asciiValue!) })
    }

    func testTheX10EncodingPacksTheButtonAndBothCellsIntoOneByteEach() {
        // CSI M followed by three bytes, each carrying 32 plus its value, so the same cell
        // as the SGR vectors above is 3 and 5 here and reads as 35 and 37. A release has no
        // button of its own in this form: it is button three.
        XCTAssertEqual(x10Sent(.left, col: 2, row: 4, pressed: true), [32, 35, 37])
        XCTAssertEqual(x10Sent(.left, col: 2, row: 4, pressed: false), [35, 35, 37])
        XCTAssertEqual(x10Sent(.right, col: 2, row: 4, pressed: true), [34, 35, 37])
        // The corner cell is 32 plus one, which is the space character: that is why the
        // form adds 32 at all.
        XCTAssertEqual(x10Sent(.left, col: 0, row: 0, pressed: true), [32, 33, 33])
    }

    func testTheWheelIsWhatADragIsSentAs() {
        // A finger cannot click a scrollbar, so a drag over the client's interface is the
        // wheel a trackpad would have turned at the cell the finger is on: what scrolls is
        // whatever the client has there — a page, a guide, a room's messages — which is the
        // only scrolling a full-screen client can be given from outside itself. The wire
        // gives the wheel no buttons of its own, and no release either: one report is one
        // notch.
        XCTAssertEqual(
            sent(.wheelUp, col: 2, row: 4, pressed: true),
            [esc, 0x5B, 0x3C] + "64;3;5M".map { Int($0.asciiValue!) })
        XCTAssertEqual(
            sent(.wheelDown, col: 2, row: 4, pressed: true),
            [esc, 0x5B, 0x3C] + "65;3;5M".map { Int($0.asciiValue!) })
        // The older form has a notch of its own too: 32 added to the wire's number.
        XCTAssertEqual(x10Sent(.wheelUp, col: 2, row: 4, pressed: true), [96, 35, 37])
        XCTAssertEqual(x10Sent(.wheelDown, col: 2, row: 4, pressed: true), [97, 35, 37])
    }

    func testACellIsClampedIntoWhatTheEncodingCanCarry() {
        // A finger cannot land outside the console, but the byte it is written into cannot
        // wrap either: an off-by-one here would be a sequence the client reads as a letter.
        let x10 = sent(.left, col: 9999, row: 9999, pressed: true, sgr: false)
        XCTAssertEqual(Array(x10.suffix(2)), [255, 255])

        // A coordinate before the corner is clamped onto it, and identically for a press
        // and its release, so that the two always name the same cell.
        XCTAssertEqual(
            sent(.left, col: -5, row: -5, pressed: true),
            sent(.left, col: 0, row: 0, pressed: true))
        XCTAssertEqual(
            sent(.left, col: -5, row: -5, pressed: false),
            [esc, 0x5B, 0x3C] + "0;1;1m".map { Int($0.asciiValue!) })
        XCTAssertEqual(
            sent(.left, col: -5, row: -5, pressed: false, sgr: false),
            sent(.left, col: 0, row: 0, pressed: false, sgr: false))
    }

    func testTheRawValuesAreTheWireNumbers() {
        XCTAssertEqual(TerminalMouseButton.left.rawValue, 0)
        XCTAssertEqual(TerminalMouseButton.middle.rawValue, 1)
        XCTAssertEqual(TerminalMouseButton.right.rawValue, 2)
        XCTAssertEqual(TerminalMouseButton.wheelUp.rawValue, 64)
        XCTAssertEqual(TerminalMouseButton.wheelDown.rawValue, 65)
        XCTAssertEqual(TerminalMouse.releaseButton, 3)
        XCTAssertEqual(TerminalMouse.offset, 32)
    }
}
