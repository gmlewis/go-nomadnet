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

/// The tests for when the client is told its window has changed.
///
/// These are the appliance's device finding written down: it started its client at 80 by 24,
/// the pane could show 75 by 37, and no RESIZE frame was ever sent because the question was
/// asked of the screen the app was drawing rather than of the size the client had been told.
final class TerminalSizingTests: XCTestCase {
    private func metrics(cols: Int, rows: Int) -> TerminalMetrics {
        TerminalMetrics(cellWidth: 10, cellHeight: 20, cols: cols, rows: rows)
    }

    func testAPaneThatFitsMoreThanTheClientWasToldIsATelling() {
        let size = TerminalSizing.resize(fitting: metrics(cols: 75, rows: 37), told: TerminalSize(cols: 80, rows: 24))
        XCTAssertEqual(size, TerminalSize(cols: 75, rows: 37))
    }

    func testAPaneThatFitsExactlyWhatTheClientWasToldIsNot() {
        let told = TerminalSize(cols: 75, rows: 37)
        XCTAssertNil(TerminalSizing.resize(fitting: metrics(cols: 75, rows: 37), told: told))
    }

    /// One axis moving is a telling: a key strip appearing changes the rows and leaves the
    /// columns alone, and a client that is not told draws over the strip.
    func testEitherAxisMovingIsATelling() {
        let told = TerminalSize(cols: 75, rows: 37)
        XCTAssertEqual(TerminalSizing.resize(fitting: metrics(cols: 75, rows: 30), told: told),
                       TerminalSize(cols: 75, rows: 30))
        XCTAssertEqual(TerminalSizing.resize(fitting: metrics(cols: 120, rows: 37), told: told),
                       TerminalSize(cols: 120, rows: 37))
    }

    /// A size that is not a size is not something a client can be told.
    func testASizeIsNeverSmallerThanOneCell() {
        XCTAssertEqual(TerminalSize(cols: 0, rows: 0), TerminalSize(cols: 1, rows: 1))
        XCTAssertEqual(TerminalSize(cols: -5, rows: 3), TerminalSize(cols: 1, rows: 3))
    }
}
