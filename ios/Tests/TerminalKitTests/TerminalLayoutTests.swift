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

/// The console's arithmetic: the grid a rectangle holds and the strip of keys it takes.
///
/// A view draws with a canvas, so the part of it that is easy to get wrong and impossible
/// to see is here instead: how many columns and rows a rectangle of points holds, where a
/// cell lands on it, and which button of the on-screen key strip a finger landed on. A
/// terminal told it is one column wider than the screen is a line of text with its last
/// character cut off, and a key strip divided by the wrong number is a tap on the button
/// next to the one that was pressed.
final class TerminalLayoutTests: XCTestCase {

    func testTheGridSizeFromARectangleIsWhatFits() {
        let cases: [(name: String, width: Double, height: Double, cols: Int, rows: Int)] = [
            ("the whole fit", 800, 480, 100, 30),
            // A partial cell is not a column: the client's own drawing would be cut off, and
            // the terminal it is told about has to be one it can fill.
            ("a partial cell is not a column", 807, 487, 100, 30),
            ("one point short of another column", 799, 479, 99, 29),
            ("exactly one cell", 8, 16, 1, 1),
            // A view narrower than one character still has to be a screen: zero columns is
            // not a terminal, and the client would be laid out into nothing.
            ("narrower than one cell", 5, 5, 1, 1),
            ("a tablet in portrait", 1200, 2000, 150, 125),
            ("a fractional cell", 803.5, 481.5, 100, 30),
        ]

        for testCase in cases {
            let grid = TerminalMetrics.metrics(
                bounds: (width: testCase.width, height: testCase.height),
                cellWidth: 8,
                cellHeight: 16,
                keyStripHeight: 0
            )
            guard let grid else {
                XCTFail("\(testCase.name): a rectangle that holds a screen measured as unusable")
                continue
            }
            XCTAssertEqual(testCase.cols, grid.cols, "\(testCase.name): the columns")
            XCTAssertEqual(testCase.rows, grid.rows, "\(testCase.name): the rows")
        }
    }

    func testTheKeyStripComesOutOfTheGrid() {
        // The strip is drawn over the client, so the rows it takes are rows the client is
        // not told it has: showing the strip is a resize rather than a redraw.
        let bare = TerminalMetrics.metrics(bounds: (width: 800, height: 480),
                                          cellWidth: 8, cellHeight: 16, keyStripHeight: 0)
        XCTAssertEqual(30, bare?.rows)

        let rowHeight = TerminalMetrics.rowHeight(forCellHeight: 16)
        XCTAssertEqual(24, rowHeight)
        let strip = TerminalMetrics.stripHeight(rowHeight: rowHeight)
        XCTAssertEqual(60, strip)

        let withStrip = TerminalMetrics.metrics(bounds: (width: 800, height: 480),
                                               cellWidth: 8, cellHeight: 16, keyStripHeight: strip)
        XCTAssertEqual(100, withStrip?.cols, "the strip takes nothing from the width")
        XCTAssertEqual(26, withStrip?.rows, "the strip's rows came out of the grid")

        // A rectangle with no room left for a row after the strip is not a terminal.
        XCTAssertNil(TerminalMetrics.metrics(bounds: (width: 800, height: 60),
                                             cellWidth: 8, cellHeight: 16, keyStripHeight: 60))
        XCTAssertNil(TerminalMetrics.metrics(bounds: (width: 800, height: 0),
                                             cellWidth: 8, cellHeight: 16, keyStripHeight: 0))
    }

    func testAnUnusableMeasurementIsNilRatherThanAGrid() {
        // A cell of no points is not a small font, it is a font that has not been measured,
        // and every division below would be by zero.
        XCTAssertNil(TerminalMetrics.metrics(bounds: (width: 800, height: 480),
                                             cellWidth: 0, cellHeight: 16, keyStripHeight: 0))
        XCTAssertNil(TerminalMetrics.metrics(bounds: (width: 800, height: 480),
                                             cellWidth: 8, cellHeight: 0, keyStripHeight: 0))
        XCTAssertNil(TerminalMetrics.metrics(bounds: (width: 0, height: 480),
                                             cellWidth: 8, cellHeight: 16, keyStripHeight: 0))
        XCTAssertNotNil(TerminalMetrics.metrics(bounds: (width: 0.5, height: 0.5),
                                                cellWidth: 0.5, cellHeight: 0.5, keyStripHeight: 0))
    }

    func testTheCellToPointMappingAgreesWithTheFit() {
        let metrics = TerminalMetrics.metrics(bounds: (width: 800, height: 480),
                                              cellWidth: 8, cellHeight: 16, keyStripHeight: 0)
        guard let grid = metrics else { return XCTFail("no metrics for a screen that fits") }
        XCTAssertEqual(100, grid.cols)
        XCTAssertEqual(30, grid.rows)

        XCTAssertEqual(0.0, grid.cellToPixel(col: 0, row: 0).x, "the first column starts at the edge")
        XCTAssertEqual(0.0, grid.cellToPixel(col: 0, row: 0).y, "the first row starts at the top")
        XCTAssertEqual(24.0, grid.cellToPixel(col: 3, row: 0).x)
        XCTAssertEqual(112.0, grid.cellToPixel(col: 0, row: 7).y)

        // The two halves have to agree: every cell the fit promised is inside the view, and
        // the next one is not — otherwise the last column is drawn half off the screen.
        for col in 0..<grid.cols {
            let x = grid.cellToPixel(col: col, row: 0).x
            XCTAssertTrue(x + Double(grid.cellWidth) <= 800, "column \(col) starts past the edge")
        }
        for row in 0..<grid.rows {
            let y = grid.cellToPixel(col: 0, row: row).y
            XCTAssertTrue(y + Double(grid.cellHeight) <= 480, "row \(row) starts past the bottom")
        }
        XCTAssertTrue(grid.cellToPixel(col: grid.cols, row: 0).x + Double(grid.cellWidth) > 800,
                      "a column more would have fitted")
    }

    func testAPointMapsBackToTheCellItIsIn() {
        let metrics = TerminalMetrics.metrics(bounds: (width: 800, height: 480),
                                              cellWidth: 8, cellHeight: 16, keyStripHeight: 0)
        guard let grid = metrics else { return XCTFail("no metrics for a screen that fits") }

        /// The cell a point lands in, as the pair (column, row).
        func cell(_ x: Double, _ y: Double) -> (col: Int, row: Int) { grid.pixelToCell(x: x, y: y) }

        XCTAssertTrue(cell(0, 0) == (0, 0))
        XCTAssertTrue(cell(7.9, 15.9) == (0, 0), "the last point of the first cell is still in it")
        XCTAssertTrue(cell(8, 16) == (1, 1))
        XCTAssertTrue(cell(24, 112) == (3, 7))

        // A point outside the view is clamped into it rather than refused: a finger on the
        // edge is a finger on a cell.
        XCTAssertTrue(cell(-4, -4) == (0, 0))
        XCTAssertTrue(cell(4000, 4000) == (grid.cols - 1, grid.rows - 1))
    }

    func testTheMetricsKeepTheirInvariantAndCompareByValue() {
        let metrics = TerminalMetrics(cellWidth: 8, cellHeight: 16, cols: 100, rows: 30)

        XCTAssertEqual(metrics, TerminalMetrics(cellWidth: 8, cellHeight: 16, cols: 100, rows: 30))
        XCTAssertNotEqual(metrics, TerminalMetrics(cellWidth: 8, cellHeight: 16, cols: 100, rows: 31))

        // A cell of no points is not a cell and a screen of no cells is not a screen, so the
        // invariant holds however the value was built.
        let impossible = TerminalMetrics(cellWidth: 0, cellHeight: -3, cols: 0, rows: 0)
        XCTAssertEqual(1, impossible.cellWidth)
        XCTAssertEqual(1, impossible.cellHeight)
        XCTAssertEqual(1, impossible.cols)
        XCTAssertEqual(1, impossible.rows)
    }

    func testTheKeyStripOffersTheKeysATabletCannotPress() {
        // The keys a soft keyboard has no button for. Without them the client cannot be
        // driven at all: escape backs out of a dialog, home and end and the page keys walk a
        // document, and the arrows are how a menu is walked. They are the Termux extra-keys
        // row's own keys, so a person who has driven this client under Termux finds the same
        // buttons here.
        XCTAssertEqual(
            [
                .key(.escape),
                .key(.rune("/")),
                .key(.rune("-")),
                .key(.home),
                .key(.up),
                .key(.end),
                .key(.pageUp),
            ],
            TerminalMetrics.rowOne,
            "the strip's top row is not escape, the path characters and navigation"
        )
        XCTAssertEqual(
            [
                .key(.tab),
                .control,
                .alt,
                .key(.left),
                .key(.down),
                .key(.right),
                .key(.pageDown),
            ],
            TerminalMetrics.rowTwo,
            "the strip's bottom row is not TAB, the latches and the navigation it should be"
        )
        XCTAssertEqual(TerminalMetrics.rowOne.count, TerminalMetrics.rowTwo.count,
                       "the rows are not the same width")
    }

    func testAKeyRowIsOneAndAHalfConsoleCells() {
        // A terminal cell is a text-sized target, not a finger-sized one, so the strip's rows
        // are one and a half cells tall and its labels one and a half cells' font — the two
        // grow together. The strip is drawn over the client, so those rows come out of what
        // the client is told it has.
        XCTAssertEqual(24, TerminalMetrics.rowHeight(forCellHeight: 16), "a 16-point cell")
        XCTAssertEqual(30, TerminalMetrics.rowHeight(forCellHeight: 20))
        XCTAssertEqual(17, TerminalMetrics.rowHeight(forCellHeight: 11), "a half point rounds up")
        XCTAssertEqual(2, TerminalMetrics.rowHeight(forCellHeight: 0), "a cell that is not measured")

        XCTAssertEqual(6, TerminalMetrics.stripPadding(rowHeight: 24))
        XCTAssertEqual(4, TerminalMetrics.stripPadding(rowHeight: 12), "a small row keeps its margin")
        XCTAssertEqual(4, TerminalMetrics.stripPadding(rowHeight: 0))

        // Two rows and the margin above and below them.
        XCTAssertEqual(60, TerminalMetrics.stripHeight(rowHeight: 24))
        XCTAssertEqual(60, TerminalMetrics.stripHeight(rowHeight: 24, rowCount: 2))
        XCTAssertEqual(12 + 8, TerminalMetrics.stripHeight(rowHeight: 12, rowCount: 1),
                       "a small row keeps its four-point margin")

        // A strip that is not shown takes no room at all, which is how a console with the
        // soft keyboard hidden gets the whole screen.
        XCTAssertEqual(0, TerminalMetrics.stripHeight(rowHeight: 24, rowCount: 0))
    }

    func testTheStripsButtonsAreTheButtonsATapLandsOn() {
        // The strip is drawn as equal shares of each row's width, so a tap is a division: the
        // point decides which button it landed on, and the ends of a row are not one button
        // short. The row is part of the division too, or every tap would be a tap on the top
        // row's buttons.
        let rows = [TerminalMetrics.rowOne, TerminalMetrics.rowTwo]
        let width = Double(TerminalMetrics.rowOne.count) * 100
        let height = 60.0

        func entry(_ x: Double, _ y: Double) -> KeyStripEntry? {
            TerminalMetrics.entry(at: x, y: y, bounds: (width: width, height: height), rows: rows)
        }

        XCTAssertEqual(TerminalMetrics.rowOne[0], entry(0, 0), "the left edge")
        XCTAssertEqual(TerminalMetrics.rowOne[0], entry(99.9, 0), "just inside the first")
        XCTAssertEqual(TerminalMetrics.rowOne[1], entry(100, 0), "just inside the second")
        XCTAssertEqual(TerminalMetrics.rowOne[3], entry(350, 5), "a tap in the middle of the top row")
        XCTAssertEqual(TerminalMetrics.rowOne.last, entry(width - 0.1, 0),
                       "the last button of the top row must be reachable")
        XCTAssertEqual(TerminalMetrics.rowTwo[3], entry(350, 30),
                       "the same place one row down is the row below's button")
        XCTAssertEqual(TerminalMetrics.rowTwo.last, entry(width, height - 0.1),
                       "the last row's last button must be reachable")

        // A row or a column a tap cannot be outside of is clamped rather than rejected,
        // because a finger on an edge is a finger on a button.
        XCTAssertEqual(TerminalMetrics.rowOne[0], entry(-40, -40))
        XCTAssertEqual(TerminalMetrics.rowTwo.last, entry(width + 200, height + 200))

        // A strip drawn with no width yet — a view between construction and its first
        // layout — is its first button rather than a division by zero, and a strip with no
        // buttons at all is nothing a finger can land on.
        XCTAssertEqual(TerminalMetrics.rowOne[0],
                       TerminalMetrics.entry(at: 0, y: 0, bounds: (width: 0, height: 0), rows: rows))
        XCTAssertNil(TerminalMetrics.entry(at: 0, y: 0, bounds: (width: 700, height: 60), rows: []))

        // With the geometry the view itself draws with, the division is that geometry's and
        // not an equal share: the row height and the strip's top are the view's arithmetic.
        XCTAssertEqual(TerminalMetrics.rowOne[3],
                       TerminalMetrics.entry(at: 350, y: 805, width: width, stripTop: 800,
                                              rowHeight: 20, rows: rows))
        XCTAssertEqual(TerminalMetrics.rowTwo[3],
                       TerminalMetrics.entry(at: 350, y: 825, width: width, stripTop: 800,
                                              rowHeight: 20, rows: rows))
        XCTAssertEqual(TerminalMetrics.rowTwo.last,
                       TerminalMetrics.entry(at: width, y: 840, width: width, stripTop: 800,
                                              rowHeight: 20, rows: rows))
        XCTAssertEqual(TerminalMetrics.rowOne[0],
                       TerminalMetrics.entry(at: 0, y: 0, width: 0, stripTop: 0, rowHeight: 0,
                                              rows: rows))
    }
}
