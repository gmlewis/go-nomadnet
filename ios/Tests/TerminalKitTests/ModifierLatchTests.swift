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

/// The Control and Alt switches the touch key strip carries.
///
/// A touch keyboard cannot hold one key while pressing another, so control-c reaches the
/// client only as a latch that a tap arms and the next character spends. The rules below
/// are the whole of that behavior, and they are asserted here because getting one wrong
/// is the difference between quitting the client and typing a `q` into it.
final class ModifierLatchTests: XCTestCase {

    /// sent is what the console would actually write for a key under the latch.
    private func sent(_ latch: inout ModifierLatch, _ key: TerminalKey) -> [Int]? {
        let spent = latch.spend(key)
        return TerminalKeys.bytes(for: key, control: spent.control, alt: spent.alt)?.map { Int($0) }
    }

    func testALatchStartsDisarmed() {
        let latch = ModifierLatch()
        XCTAssertFalse(latch.isArmed(.control))
        XCTAssertFalse(latch.isArmed(.alt))
        XCTAssertTrue(latch.armed.isEmpty)
    }

    func testATapTogglesAndTwoTapsLeaveItWhereItStarted() {
        var latch = ModifierLatch()
        latch.toggle(.control)
        XCTAssertTrue(latch.isArmed(.control))
        latch.toggle(.control)
        XCTAssertFalse(latch.isArmed(.control))
        // Two taps on the lit button clear it, which is the strip's way of taking back a
        // tap that armed the wrong modifier.
        latch.toggle(.control)
        latch.toggle(.alt)
        latch.toggle(.alt)
        XCTAssertTrue(latch.isArmed(.control))
        XCTAssertFalse(latch.isArmed(.alt))

        latch.arm(.alt)
        latch.arm(.alt)
        XCTAssertEqual(latch.armed, [.control, .alt])
        latch.disarm(.control)
        XCTAssertEqual(latch.armed, [.alt])
        latch.clear()
        XCTAssertTrue(latch.armed.isEmpty)
    }

    func testTheNextCharacterSpendsControl() {
        var latch = ModifierLatch()
        latch.toggle(.control)

        XCTAssertEqual(sent(&latch, .rune("q")), [0x11])
        XCTAssertFalse(latch.isArmed(.control), "the character did not spend the latch")
        XCTAssertEqual(sent(&latch, .rune("q")), [0x71], "the latch was spent twice")
    }

    func testControlIsSpentOnlyByAKeyItActuallyChanged() {
        // An arrow has no control code, so it is sent as an arrow and leaves the latch
        // armed: the person armed control for a character, and the arrow was not one.
        var latch = ModifierLatch()
        latch.toggle(.control)

        XCTAssertEqual(sent(&latch, .up), [0x1B, 0x5B, 0x41])
        XCTAssertTrue(latch.isArmed(.control), "the arrow spent a latch it did not change")

        // A word an input method committed at once is not a character to the terminal
        // either.
        XCTAssertEqual(sent(&latch, .rune("hi")), [0x68, 0x69])
        XCTAssertTrue(latch.isArmed(.control))

        // The character it was armed for is the one that spends it.
        XCTAssertEqual(sent(&latch, .rune("c")), [0x03])
        XCTAssertFalse(latch.isArmed(.control))
        XCTAssertEqual(sent(&latch, .rune("c")), [0x63])
    }

    func testAltPrefixesAnEscapeToAnyKeyAndIsSpentByIt() {
        var latch = ModifierLatch()
        latch.toggle(.alt)

        XCTAssertEqual(sent(&latch, .rune("x")), [0x1B, 0x78])
        XCTAssertFalse(latch.isArmed(.alt))

        latch.toggle(.alt)
        XCTAssertEqual(sent(&latch, .up), [0x1B, 0x1B, 0x5B, 0x41])
        XCTAssertFalse(latch.isArmed(.alt), "alt changes any key, so any key spends it")

        // A key that sends nothing sends nothing: the escape in front of it is not a key
        // press.
        latch.toggle(.alt)
        XCTAssertNil(sent(&latch, .rune("")))
        XCTAssertFalse(latch.isArmed(.alt))
    }

    func testBothLatchesTogetherSendTheControlCodeWithAnEscapeInFront() {
        var latch = ModifierLatch()
        latch.toggle(.control)
        latch.toggle(.alt)

        XCTAssertEqual(sent(&latch, .rune("d")), [0x1B, 0x04])
        XCTAssertTrue(latch.armed.isEmpty, "both latches must be spent by the one key")
    }

    func testAControlLatchLeavesAltAloneAndTheOtherWayRound() {
        var latch = ModifierLatch()
        latch.toggle(.control)

        // Spending control on a key does not touch alt, and vice versa: the two buttons on
        // the strip are two switches, and a person can have either, both or neither lit.
        latch.arm(.alt)
        let spent = latch.spend(.rune("a"))
        XCTAssertEqual(spent.control, true)
        XCTAssertEqual(spent.alt, true)
        XCTAssertTrue(latch.armed.isEmpty)

        latch.arm(.alt)
        let onlyAlt = latch.spend(.up)
        XCTAssertEqual(onlyAlt.control, false)
        XCTAssertEqual(onlyAlt.alt, true)
        XCTAssertTrue(latch.armed.isEmpty)

        latch.arm(.control)
        let nothing = latch.spend(.rune(""))
        XCTAssertEqual(nothing.control, false, "an empty key is not a character")
        XCTAssertEqual(nothing.alt, false)
        XCTAssertTrue(latch.isArmed(.control), "the latch was not spent, so it stays armed")
    }

    func testControlSentThroughTheLatchesMatchesTheControlKeys() {
        // The latch and the strip's own ctrl keys must reach the client as the same bytes,
        // or ^Q from a keyboard and ^Q from the soft keyboard would be two different keys.
        for letter in "abcdefghijklmnopqrstuvwxyz" {
            var latch = ModifierLatch()
            latch.toggle(.control)
            let fromLatch = sent(&latch, .rune(String(letter)))
            let fromKey = TerminalKeys.bytes(for: .ctrl(letter))?.map { Int($0) }
            XCTAssertEqual(fromLatch, fromKey, "ctrl-\(letter) from the latch")
        }
    }
}
