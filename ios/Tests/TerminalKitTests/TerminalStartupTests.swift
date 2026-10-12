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

/// What the client asks for as it starts, parsed from the bytes it actually writes.
///
/// The other parser tests use sequences written by hand, which is what a parser needs to be
/// pinned against — and it is also what let a whole class of mistake through: a sequence that
/// is plausible but is not what the client sends. These are the bytes the NomadNet client's own
/// tcell writes to the appliance's console on startup, kept verbatim.
final class TerminalStartupTests: XCTestCase {
    /// The client's startup, as captured from the appliance's console.
    private let startup = "\u{1B}[?2004l\u{1B}[?1049h\u{1B}[22;0;0t\u{1B}[22;2t\u{1B}[?1h\u{1B}="
        + "\u{1B}[?25l\u{1B}[?7l\u{1B}[H\u{1B}[2J\u{1B}[>4;2m\u{1B}[>1u\u{1B}[?9001h"
        + "\u{1B}[?1000l\u{1B}[?1002l\u{1B}[?1003l\u{1B}[?1006l"
        + "\u{1B}[?1000h\u{1B}[?1002h\u{1B}[?1003h\u{1B}[?1006h"

    private func parsed() -> TerminalScreen {
        let screen = TerminalScreen(cols: 80, rows: 24)
        TerminalParser(screen: screen).parse(Array(startup.utf8))
        return screen
    }

    /// Without this the console is a screen that can be read and not touched: the client is sent
    /// no clicks and no drags, so every list, button and menu in it is unreachable.
    func testTheClientsOwnStartupAsksForTheMouse() {
        XCTAssertTrue(parsed().mouseReporting)
    }

    /// The SGR encoding is the one the appliance sends, and the one that carries a coordinate
    /// past the 223 columns the older encoding can spell.
    func testTheClientsOwnStartupAsksForTheSgrEncoding() {
        XCTAssertTrue(parsed().mouseSGR)
    }

    /// The client runs on the alternate screen, which is why the appliance cannot scroll it by
    /// moving the console's own scrollback: that screen has none.
    func testTheClientsOwnStartupTakesTheAlternateScreen() {
        let screen = parsed()
        XCTAssertFalse(screen.scrollbackCount > 0, "the alternate screen has no scrollback to move")
    }
}
