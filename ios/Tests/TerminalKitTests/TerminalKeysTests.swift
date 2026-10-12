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

/// What the appliance sends the client when a key is pressed.
///
/// The vectors below are readable against `tui/embedded-terminal.go`'s `keyToANSI`, which
/// is the same table on the Go side, including the two points a terminal does not agree
/// with a keyboard about: backspace sends DEL (0x7F) rather than BS (0x08), and the arrows
/// send the CSI forms rather than the SS3 ones.
final class TerminalKeysTests: XCTestCase {

    private let esc = 0x1B

    /// sent renders what a key sends, as integers, or nil when it sends nothing.
    private func sent(
        _ key: TerminalKey, control: Bool = false, alt: Bool = false
    ) -> [Int]? {
        TerminalKeys.bytes(for: key, control: control, alt: alt)?.map { Int($0) }
    }

    func testControlAndALetterSendsItsControlCode() {
        let cases: [(String, TerminalKey, [Int])] = [
            ("ctrl-a", .ctrl("A"), [0x01]),
            ("ctrl-c", .ctrl("C"), [0x03]),
            ("ctrl-i", .ctrl("I"), [0x09]),
            ("ctrl-m", .ctrl("M"), [0x0D]),
            ("ctrl-z", .ctrl("Z"), [0x1A]),
        ]
        for (name, key, want) in cases {
            XCTAssertEqual(sent(key), want, "\(name): the control code")
            // A keyboard reports the letter in whichever case its layout has, and the
            // control code is the same either way.
            guard case .ctrl(let letter) = key else { return XCTFail("\(name): not a ctrl key") }
            let lowered = Character(String(letter).lowercased())
            XCTAssertEqual(sent(.ctrl(lowered)), want, "\(name): lower case")
        }

        // Only the alphabet has control codes: ctrl-1 and ctrl-[ are not ctrl-a.
        XCTAssertNil(sent(.ctrl("1")))
        XCTAssertNil(sent(.ctrl("[")))
        XCTAssertNil(sent(.ctrl(" ")))
    }

    func testTheControlLatchTurnsASingleLetterIntoItsCode() {
        XCTAssertEqual(sent(.rune("c"), control: true), [0x03])
        XCTAssertEqual(sent(.rune("C"), control: true), [0x03])
        XCTAssertEqual(sent(.rune("z"), control: true), [0x1A])
        XCTAssertTrue(TerminalKeys.controlApplies(to: .rune("c")))
        XCTAssertTrue(TerminalKeys.controlApplies(to: .rune("/")))

        // Control changes only a single character: a word an input method committed is
        // sent as itself, and so is a key that is not a character at all.
        XCTAssertEqual(sent(.rune("hi"), control: true), [0x68, 0x69])
        XCTAssertFalse(TerminalKeys.controlApplies(to: .rune("hi")))
        XCTAssertEqual(sent(.up, control: true), [esc, 0x5B, 0x41])
        XCTAssertFalse(TerminalKeys.controlApplies(to: .up))

        // A single character with no control code of its own sends nothing under the
        // latch rather than sending the character: the modifier was asked for.
        XCTAssertNil(sent(.rune("/"), control: true))
        XCTAssertNil(sent(.rune("1"), control: true))
    }

    func testTheArrowKeysSendTheCSICursorSequences() {
        let cases: [(String, TerminalKey, [Int])] = [
            ("up", .up, [esc, 0x5B, 0x41]),
            ("down", .down, [esc, 0x5B, 0x42]),
            ("right", .right, [esc, 0x5B, 0x43]),
            ("left", .left, [esc, 0x5B, 0x44]),
        ]
        for (name, key, want) in cases {
            XCTAssertEqual(sent(key), want, "\(name): the cursor sequence")
        }
    }

    func testHomeEndPageUpAndPageDownSendTheirSequences() {
        let cases: [(String, TerminalKey, [Int])] = [
            ("home", .home, [esc, 0x5B, 0x48]),
            ("end", .end, [esc, 0x5B, 0x46]),
            ("page up", .pageUp, [esc, 0x5B, 0x35, 0x7E]),
            ("page down", .pageDown, [esc, 0x5B, 0x36, 0x7E]),
            // The two editing keys share the tilde form the two above use.
            ("delete", .delete, [esc, 0x5B, 0x33, 0x7E]),
            ("insert", .insert, [esc, 0x5B, 0x32, 0x7E]),
        ]
        for (name, key, want) in cases {
            XCTAssertEqual(sent(key), want, "\(name): the sequence")
        }
    }

    func testEscapeTabBackspaceAndEnterSendTheirBytes() {
        let cases: [(String, TerminalKey, [Int])] = [
            ("escape", .escape, [esc]),
            ("tab", .tab, [0x09]),
            // Backspace is DEL, not BS: the client is an editor and reads 0x7F, which is
            // what a terminal sends and what the Go side sends too.
            ("backspace", .backspace, [0x7F]),
            ("enter", .enter, [0x0D]),
        ]
        for (name, key, want) in cases {
            XCTAssertEqual(sent(key), want, "\(name): the byte")
        }
    }

    func testAPlainCharacterSendsItsUTF8Bytes() {
        let cases: [(String, TerminalKey, [Int])] = [
            ("a letter", .rune("a"), [0x61]),
            ("a digit", .rune("1"), [0x31]),
            ("punctuation", .rune("/"), [0x2F]),
            ("an accented letter", .rune("é"), [0xC3, 0xA9]),
            ("a wide character", .rune("世"), [0xE4, 0xB8, 0x96]),
            ("a character outside the bmp", .rune("😀"), [0xF0, 0x9F, 0x98, 0x80]),
            // An input method commits a word at a time, so a "character" can be several.
            ("a committed word", .rune("hi"), [0x68, 0x69]),
        ]
        for (name, key, want) in cases {
            XCTAssertEqual(sent(key), want, "\(name): the bytes")
        }
    }

    func testAnAltCombinationIsAnEscapeInFrontOfTheKey() {
        // A terminal has no alt byte to send. What it writes is the key's own bytes with
        // an escape before them — xterm's metaSendsEscape — and the client reads that as
        // the meta combination, which is how alt reaches it from a keyboard that cannot
        // hold alt down.
        XCTAssertEqual(sent(.rune("x"), alt: true), [esc, 0x78])
        XCTAssertEqual(sent(.up, alt: true), [esc, esc, 0x5B, 0x41])
        XCTAssertEqual(sent(.escape, alt: true), [esc, esc])
        XCTAssertEqual(sent(.ctrl("c"), alt: true), [esc, 0x03])
        XCTAssertEqual(sent(.enter, alt: true), [esc, 0x0D])

        // Alt over a key that sends nothing sends nothing: a prefix with no key behind it
        // is not a key press.
        XCTAssertNil(sent(.rune(""), alt: true))
        XCTAssertNil(sent(.rune(""), control: true, alt: true))

        // Both latches together: the control code, then the escape in front of it.
        XCTAssertEqual(sent(.rune("d"), control: true, alt: true), [esc, 0x04])
    }

    func testAnEmptyCharacterSendsNothing() {
        // The client is a program reading a terminal, so "this key means nothing" has to
        // be distinguishable from "this key means an empty string".
        XCTAssertNil(sent(.rune("")))
        XCTAssertNil(TerminalKeys.bytes(for: .rune("")))
    }
}
