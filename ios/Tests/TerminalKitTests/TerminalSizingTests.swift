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
/// The question must be asked of the size the client has been told, not of the screen the app
/// is drawing: the view resizes its own screen on every layout, so the two would always agree
/// and no RESIZE frame would ever be sent.
final class TerminalSizingTests: XCTestCase {
    private func metrics(
        cols: Int, rows: Int, pixelWidth: Int = 0, pixelHeight: Int = 0
    ) -> TerminalMetrics {
        TerminalMetrics(
            cellWidth: 10, cellHeight: 20, cols: cols, rows: rows,
            pixelWidth: pixelWidth, pixelHeight: pixelHeight)
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

    /// The console's physical size is part of the size the client is told, so the app learning
    /// it — the first layout that measured the view, or a rotation — is a telling even when the
    /// cells are the same. A client that is not told keeps the shape it had, and a console read
    /// as lying down when it is standing up is every side panel left open.
    func testTheConsolePhysicalSizeIsATelling() {
        let told = TerminalSize(cols: 100, rows: 75, pixelWidth: 1200, pixelHeight: 1920)

        XCTAssertNil(TerminalSizing.resize(
            fitting: metrics(cols: 100, rows: 75, pixelWidth: 1200, pixelHeight: 1920),
            told: told), "the same console was reported as a change")

        // The same cells in a console that has been measured for the first time.
        XCTAssertEqual(
            TerminalSizing.resize(fitting: metrics(cols: 100, rows: 75), told: told),
            TerminalSize(cols: 100, rows: 75))

        // A rotation changes both, and the size that goes out carries both.
        XCTAssertEqual(
            TerminalSizing.resize(
                fitting: metrics(cols: 160, rows: 45, pixelWidth: 1920, pixelHeight: 1080),
                told: told),
            TerminalSize(cols: 160, rows: 45, pixelWidth: 1920, pixelHeight: 1080))
    }

    /// A negative physical size is not a size, and a client comparing one with itself would read
    /// a console as standing up for no reason.
    func testAPhysicalSizeIsNeverNegative() {
        XCTAssertEqual(TerminalSize(cols: 80, rows: 24, pixelWidth: -1, pixelHeight: -1),
                       TerminalSize(cols: 80, rows: 24))
    }
}
