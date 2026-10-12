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

/// The emulated terminal's grid, cursor and scroll region.
///
/// `tui/vterm.go` holds the same screen in Go, and every vector here is readable
/// against its Go original: the deferred autowrap in `putRune`, the row shuffles in
/// `scrollUp` and `scrollDown`, the three erase modes, the alternate screen's save
/// and restore, and the upper-left overlap `resize` keeps. A disagreement between the
/// two emulators does not surface as an error — it is a screen that draws the wrong
/// characters — so the grid is asserted as text rather than sampled cell by cell.
final class TerminalScreenTests: XCTestCase {

    /// type prints text the way a shell's output arrives: runes and nothing else.
    private func type(_ screen: TerminalScreen, _ text: String) {
        for ch in text {
            screen.putGlyph(String(ch), width: 1)
        }
    }

    /// lines renders the whole grid, so a test can state the screen it expects.
    private func lines(_ screen: TerminalScreen) -> [String] {
        (0..<screen.rows).map { screen.rowText($0) }
    }

    /// filledScreen is a 3x3 screen holding one distinct letter per row.
    private func filledScreen() -> TerminalScreen {
        let screen = TerminalScreen(cols: 3, rows: 3)
        for (y, text) in ["abc", "def", "ghi"].enumerated() {
            screen.moveCursor(x: 0, y: y)
            type(screen, text)
        }
        return screen
    }

    func testANewScreenIsBlankAndTheCursorIsAtTheOrigin() {
        let screen = TerminalScreen(cols: 5, rows: 3)

        XCTAssertEqual("     \n     \n     ", screen.text())
        XCTAssertEqual(0, screen.cursorX)
        XCTAssertEqual(0, screen.cursorY)
        XCTAssertTrue(screen.cursorVisible, "a fresh screen shows its cursor")
        XCTAssertFalse(screen.isAltScreen, "a fresh screen is the primary one")
        XCTAssertEqual([0, 2], [screen.scrollTop, screen.scrollBottom],
                       "a fresh screen scrolls in all of itself")
        XCTAssertEqual(0, screen.scrollbackCount, "a fresh screen has nothing behind it")
        XCTAssertEqual(0, screen.linesScrolledOff)
    }

    func testPrintingAdvancesTheCursorAndWritesTheRune() {
        let screen = TerminalScreen(cols: 5, rows: 3)

        type(screen, "Hi")

        XCTAssertEqual("Hi   ", screen.rowText(0))
        XCTAssertEqual("     ", screen.rowText(1))
        XCTAssertEqual(2, screen.cursorX)
        XCTAssertEqual(0, screen.cursorY)
    }

    func testPrintingPastTheLastColumnWrapsToTheNextRow() {
        let screen = TerminalScreen(cols: 3, rows: 3)

        type(screen, "abc")

        // The wrap is deferred, exactly as in vterm.go's putRune: after the last
        // column the cursor rests on it until the next rune arrives, so the rest of
        // the line can still be overtyped.
        XCTAssertEqual("abc", screen.rowText(0))
        XCTAssertEqual("   ", screen.rowText(1))
        XCTAssertEqual(2, screen.cursorX)
        XCTAssertEqual(0, screen.cursorY)

        type(screen, "d")

        XCTAssertEqual("abc", screen.rowText(0))
        XCTAssertEqual("d  ", screen.rowText(1))
        XCTAssertEqual(1, screen.cursorX)
        XCTAssertEqual(1, screen.cursorY)

        // A cursor move cancels the pending wrap, as every CSI does in vterm.go.
        let moved = TerminalScreen(cols: 3, rows: 2)
        type(moved, "abc")
        moved.moveCursor(x: 0, y: 0)
        type(moved, "Z")
        XCTAssertEqual("Zbc", moved.rowText(0))
        XCTAssertEqual("   ", moved.rowText(1))
    }

    func testTheCursorIsClampedToTheScreen() {
        let screen = TerminalScreen(cols: 5, rows: 3)

        screen.moveCursor(x: -3, y: -3)
        XCTAssertEqual(0, screen.cursorX)
        XCTAssertEqual(0, screen.cursorY)

        // Cursor addressing is clamped the way vterm.go's clampCursor clamps it:
        // a coordinate past the edge becomes the edge, never an out-of-range write.
        screen.moveCursor(x: 99, y: 99)
        XCTAssertEqual(4, screen.cursorX)
        XCTAssertEqual(2, screen.cursorY)

        screen.resize(cols: 2, rows: 1)
        XCTAssertEqual(1, screen.cursorX, "shrinking clamps the cursor into the new screen")
        XCTAssertEqual(0, screen.cursorY)

        let clamped = TerminalScreen(cols: 2, rows: 1)
        clamped.moveCursor(x: 99, y: 0)
        type(clamped, "X")
        XCTAssertEqual(" X", clamped.rowText(0))

        // setCursor is the same clamped placement, which is what the parser addresses
        // the cursor with.
        let setter = TerminalScreen(cols: 4, rows: 2)
        setter.setCursor(x: 9, y: 9)
        XCTAssertEqual(3, setter.cursorX)
        XCTAssertEqual(1, setter.cursorY)
    }

    func testErasingTheDisplayErasesTheWholeScreen() {
        let cases: [(name: String, mode: Int, want: [String])] = [
            ("mode 0 erases from the cursor down", 0, ["abc", "d  ", "   "]),
            ("mode 1 erases up to the cursor", 1, ["   ", "  f", "ghi"]),
            ("mode 2 erases the whole screen", 2, ["   ", "   ", "   "]),
            ("mode 3 erases the whole screen", 3, ["   ", "   ", "   "]),
        ]

        for testCase in cases {
            let screen = filledScreen()
            screen.moveCursor(x: 1, y: 1)

            screen.eraseDisplay(mode: testCase.mode)

            XCTAssertEqual(testCase.want, lines(screen), testCase.name)
            XCTAssertEqual(1, screen.cursorX, "\(testCase.name): erasing does not move the cursor")
            XCTAssertEqual(1, screen.cursorY, "\(testCase.name): erasing does not move the cursor")
        }
    }

    func testErasingTheLineErasesOnlyToTheEndWhenAsked() {
        let cases: [(name: String, mode: Int, want: String)] = [
            ("mode 0 erases to the end of the line", 0, "d  "),
            ("mode 1 erases to the start of the line", 1, "  f"),
            ("mode 2 erases the whole line", 2, "   "),
        ]

        for testCase in cases {
            let screen = filledScreen()
            screen.moveCursor(x: 1, y: 1)

            screen.eraseLine(mode: testCase.mode)

            XCTAssertEqual(testCase.want, screen.rowText(1), "\(testCase.name): the cursor's line")
            XCTAssertEqual("abc", screen.rowText(0), "\(testCase.name): the line above is untouched")
            XCTAssertEqual("ghi", screen.rowText(2), "\(testCase.name): the line below is untouched")
            XCTAssertEqual(1, screen.cursorX, "\(testCase.name): erasing does not move the cursor")
        }
    }

    func testErasingInsertingAndDeletingCharacters() {
        let erased = TerminalScreen(cols: 6, rows: 1)
        type(erased, "abcdef")
        erased.moveCursor(x: 2, y: 0)
        erased.eraseChars(count: 2)
        XCTAssertEqual("ab  ef", erased.rowText(0), "eraseChars blanks the columns from the cursor")

        let deleted = TerminalScreen(cols: 6, rows: 1)
        type(deleted, "abcdef")
        deleted.moveCursor(x: 2, y: 0)
        deleted.deleteChars(count: 2)
        XCTAssertEqual("abef  ", deleted.rowText(0), "deleteChars pulls the rest of the line left")

        let inserted = TerminalScreen(cols: 6, rows: 1)
        type(inserted, "abcdef")
        inserted.moveCursor(x: 2, y: 0)
        inserted.insertChars(count: 2)
        XCTAssertEqual("ab  cd", inserted.rowText(0), "insertChars pushes the line's right along")

        // A count past the end of the row is clamped rather than a write past it, and the
        // columns to the left of the cursor are the cursor's own: only what it operates on
        // is touched.
        let past = TerminalScreen(cols: 4, rows: 1)
        type(past, "abcd")
        past.moveCursor(x: 1, y: 0)
        past.deleteChars(count: 99)
        XCTAssertEqual("a   ", past.rowText(0))

        let long = TerminalScreen(cols: 4, rows: 1)
        type(long, "abcd")
        long.moveCursor(x: 1, y: 0)
        long.insertChars(count: 99)
        XCTAssertEqual("a   ", long.rowText(0))
    }

    func testAScrollRegionConfinesScrollingToItsRows() {
        let screen = TerminalScreen(cols: 3, rows: 4)
        for (y, text) in ["aaa", "bbb", "ccc", "ddd"].enumerated() {
            screen.moveCursor(x: 0, y: y)
            type(screen, text)
        }
        XCTAssertEqual(["aaa", "bbb", "ccc", "ddd"], lines(screen))

        screen.setScrollRegion(top: 1, bottom: 2)

        XCTAssertEqual(1, screen.scrollTop, "the region's top row")
        XCTAssertEqual(2, screen.scrollBottom, "the region's bottom row")
        // The region is set the way vterm.go sets it from CSI r, cursor homing and all.
        XCTAssertEqual(0, screen.cursorX)
        XCTAssertEqual(0, screen.cursorY)

        screen.scrollUp(1)

        XCTAssertEqual(["aaa", "ccc", "   ", "ddd"], lines(screen),
                       "scrolling up moves the region's rows, blanks its bottom, and moves nothing else")

        screen.scrollDown(1)

        XCTAssertEqual(["aaa", "   ", "ccc", "ddd"], lines(screen),
                       "scrolling down moves them back and blanks the region's top")

        // A line feed on the region's bottom row scrolls the region, not the screen.
        screen.moveCursor(x: 0, y: 2)
        screen.lineFeed()

        XCTAssertEqual(["aaa", "ccc", "   ", "ddd"], lines(screen),
                       "a line feed at the region's bottom scrolls only the region")

        // A line feed on the top row of the whole screen scrolls it up, as RI's
        // mirror does for reverseLineFeed.
        let top = TerminalScreen(cols: 3, rows: 2)
        for (y, text) in ["aaa", "bbb"].enumerated() {
            top.moveCursor(x: 0, y: y)
            type(top, text)
        }
        top.moveCursor(x: 0, y: 0)
        top.reverseLineFeed()
        XCTAssertEqual(["   ", "aaa"], lines(top), "reverseLineFeed scrolls the screen down")
        XCTAssertEqual(0, top.cursorY)
    }

    func testInsertAndDeleteLinesMoveTheRegionsRows() {
        let screen = TerminalScreen(cols: 3, rows: 4)
        for (y, text) in ["aaa", "bbb", "ccc", "ddd"].enumerated() {
            screen.moveCursor(x: 0, y: y)
            type(screen, text)
        }

        screen.setScrollRegion(top: 0, bottom: 3)
        screen.moveCursor(x: 0, y: 1)
        screen.insertLines(1)

        XCTAssertEqual(["aaa", "   ", "bbb", "ccc"], lines(screen), "insertLines pushes the rows down")

        screen.deleteLines(1)

        XCTAssertEqual(["aaa", "bbb", "ccc", "   "], lines(screen), "deleteLines pulls them back up")

        // Outside the scroll region both do nothing, as vterm.go returns early there.
        screen.setScrollRegion(top: 1, bottom: 2)
        screen.moveCursor(x: 0, y: 0)
        screen.insertLines(1)
        screen.deleteLines(1)
        XCTAssertEqual(["aaa", "bbb", "ccc", "   "], lines(screen),
                       "a line operation outside the region changed the screen")
    }

    func testALineThatScrollsOffTheTopIsKeptInTheScrollback() {
        // On a desktop the terminal emulator owns the scrollback and the client's screen
        // is only what fits in the window. In the appliance this emulator *is* the
        // terminal, so the lines that leave the screen have to be kept, or a reader who
        // drags the console upwards finds nothing above the top row.
        let screen = TerminalScreen(cols: 3, rows: 2)
        XCTAssertEqual(0, screen.scrollbackCount, "a fresh screen has nothing behind it")

        for (y, text) in ["abc", "def"].enumerated() {
            screen.moveCursor(x: 0, y: y)
            type(screen, text)
        }
        screen.moveCursor(x: 0, y: 1)
        screen.lineFeed()

        XCTAssertEqual(["def", "   "], lines(screen), "the screen scrolled")
        XCTAssertEqual(1, screen.scrollbackCount, "the line that left the screen was not kept")
        XCTAssertEqual("abc", screen.scrollbackLine(0).map { $0.glyph }.joined(),
                       "the kept line is the one that left")
        XCTAssertEqual("abc".map { String($0) },
                       screen.scrollbackLine(0).map { $0.glyph },
                       "the kept line's cells are the characters that left")

        screen.moveCursor(x: 0, y: 1)
        type(screen, "ghi")
        screen.lineFeed()

        XCTAssertEqual(["ghi", "   "], lines(screen), "the screen scrolled again")
        XCTAssertEqual(
            ["abc", "def"],
            (0..<screen.scrollbackCount).map { line in
                screen.scrollbackLine(line).map { $0.glyph }.joined()
            },
            "the scrollback holds the lines in the order they left"
        )
        XCTAssertEqual(2, screen.linesScrolledOff)
    }

    func testAScrollInsideARegionIsNotTheConsoleScrollback() {
        // A region is the client drawing in part of the screen — a dialog, a scrolling
        // list. Those lines did not leave the console, they were overwritten, and a reader
        // looking back should not find the client's half-drawn frames there.
        let screen = TerminalScreen(cols: 3, rows: 3)
        for (y, text) in ["aaa", "bbb", "ccc"].enumerated() {
            screen.moveCursor(x: 0, y: y)
            type(screen, text)
        }
        screen.setScrollRegion(top: 1, bottom: 2)
        screen.moveCursor(x: 0, y: 2)
        screen.lineFeed()

        XCTAssertEqual(["aaa", "ccc", "   "], lines(screen), "the region scrolled")
        XCTAssertEqual(0, screen.scrollbackCount, "a region's scroll was kept as scrollback")
    }

    func testTheAlternateScreenDoesNotFillTheScrollback() {
        // The alternate screen is where a full-screen client draws itself: its frames are
        // not the console's output, and keeping them would bury the conversation under a
        // repaint for every key press.
        let screen = TerminalScreen(cols: 4, rows: 2)
        type(screen, "MAIN")
        screen.setAltScreen(true, saveCursor: true)
        screen.moveCursor(x: 0, y: 1)
        type(screen, "alt")
        screen.lineFeed()

        XCTAssertEqual(0, screen.scrollbackCount, "the alternate screen wrote scrollback")

        screen.setAltScreen(false, saveCursor: true)

        XCTAssertEqual("MAIN", screen.rowText(0), "leaving the alternate screen restores the main one")
        XCTAssertEqual(0, screen.scrollbackCount, "the restored screen brought scrollback with it")
    }

    func testTheScrollbackIsBounded() {
        // A tablet runs for weeks. An unbounded scrollback is a leak that ends with the
        // OOM killer, so the oldest lines go as the newest arrive.
        let bound = 40
        let screen = TerminalScreen(cols: 4, rows: 1, maxScrollback: bound)
        let count = bound + 100

        for i in 0..<count {
            screen.moveCursor(x: 0, y: 0)
            type(screen, String(format: "%03d ", i))
            screen.lineFeed()
        }

        XCTAssertEqual(bound, screen.scrollbackCount, "the scrollback grew past its bound")
        XCTAssertEqual("100 ", String(screen.scrollbackLine(0).map { $0.glyph }.joined()),
                       "the oldest lines were not the ones dropped")
        XCTAssertEqual(
            String(format: "%03d ", count - 1),
            String(screen.scrollbackLine(screen.scrollbackCount - 1).map { $0.glyph }.joined()),
            "the newest line is not the last one written"
        )
    }

    func testTheCountOfLinesThatHaveScrolledOffDoesNotSaturate() {
        // The scrollback is bounded, so the number of lines it holds stops growing. The
        // number that have gone off the top does not: the view re-anchors a reader parked
        // in the scrollback by how much output arrived, and a count that stopped moving
        // once the buffer filled would let the text drift under their eyes for the rest of
        // the session — a reader losing their place with no sign that anything went wrong.
        let bound = 40
        let screen = TerminalScreen(cols: 4, rows: 1, maxScrollback: bound)
        XCTAssertEqual(0, screen.linesScrolledOff, "a fresh screen has scrolled nothing off")

        let count = bound + 25
        for i in 0..<count {
            screen.moveCursor(x: 0, y: 0)
            type(screen, String(format: "%03d ", i))
            screen.lineFeed()
        }

        XCTAssertEqual(bound, screen.scrollbackCount, "the scrollback is bounded")
        XCTAssertEqual(count, screen.linesScrolledOff, "the count of what left the screen is not")
    }

    func testAScrollThatIsNotTheConsolesDoesNotCountAsOutput() {
        // Only a line leaving the whole screen is console output. A region scroll and a
        // full-screen client's repaint are the client drawing over itself, and counting
        // them would drag a reader's window upwards on every frame the client drew.
        let screen = TerminalScreen(cols: 3, rows: 3)
        screen.setScrollRegion(top: 1, bottom: 2)
        screen.moveCursor(x: 0, y: 2)
        screen.lineFeed()
        XCTAssertEqual(0, screen.linesScrolledOff, "a region's scroll counted as output")

        screen.setScrollRegion(top: 1, bottom: 3)
        screen.setAltScreen(true, saveCursor: true)
        screen.moveCursor(x: 0, y: 2)
        screen.lineFeed()
        XCTAssertEqual(0, screen.linesScrolledOff, "the alternate screen's scroll counted as output")
    }

    func testTheAlternateScreenIsRestoredWithItsOwnContents() {
        let screen = TerminalScreen(cols: 4, rows: 2)
        type(screen, "MAIN")
        screen.moveCursor(x: 1, y: 1)

        screen.setAltScreen(true, saveCursor: true)

        XCTAssertTrue(screen.isAltScreen)
        XCTAssertEqual(["    ", "    "], lines(screen), "the alternate screen starts blank")
        XCTAssertEqual(0, screen.cursorX, "the alternate screen starts at the origin")
        XCTAssertEqual(0, screen.cursorY, "the alternate screen starts at the origin")

        type(screen, "ALT")
        screen.moveCursor(x: 2, y: 1)
        // Entering the alternate screen twice is not a second save: the primary screen
        // saved by the first switch is the one that comes back.
        screen.setAltScreen(true, saveCursor: true)

        XCTAssertEqual(["ALT ", "    "], lines(screen), "the alternate screen keeps its own contents")
        XCTAssertEqual(2, screen.cursorX, "a repeated switch does not move the cursor")
        XCTAssertEqual(1, screen.cursorY, "a repeated switch does not move the cursor")

        screen.setAltScreen(false, saveCursor: true)

        XCTAssertFalse(screen.isAltScreen)
        XCTAssertEqual(["MAIN", "    "], lines(screen),
                       "the primary screen comes back with its own contents")
        XCTAssertEqual(1, screen.cursorX, "the saved cursor comes back with it")
        XCTAssertEqual(1, screen.cursorY, "the saved cursor comes back with it")

        // A legacy switch (?47h, ?1047h) restores the grid but leaves the cursor where the
        // alternate screen left it, as vterm.go's saveCursor flag says.
        let legacy = TerminalScreen(cols: 4, rows: 2)
        type(legacy, "MAIN")
        legacy.setAltScreen(true, saveCursor: false)
        legacy.moveCursor(x: 3, y: 1)
        legacy.setAltScreen(false, saveCursor: false)

        XCTAssertEqual(["MAIN", "    "], lines(legacy),
                       "the primary screen comes back under a legacy switch too")
        XCTAssertEqual(3, legacy.cursorX, "a legacy switch does not restore the cursor")
        XCTAssertEqual(1, legacy.cursorY, "a legacy switch does not restore the cursor")
    }

    func testResizingKeepsTheTopLeftContents() {
        let screen = TerminalScreen(cols: 3, rows: 4)
        screen.setScrollRegion(top: 1, bottom: 2)
        type(screen, "abc")
        screen.moveCursor(x: 0, y: 1)
        type(screen, "de")

        screen.resize(cols: 5, rows: 6)

        XCTAssertEqual(5, screen.cols)
        XCTAssertEqual(6, screen.rows)
        XCTAssertEqual(["abc  ", "de   ", "     ", "     ", "     ", "     "], lines(screen),
                       "growing keeps the upper-left cells and blanks the new ones")
        XCTAssertEqual(2, screen.cursorX, "the cursor stays where it was inside the screen")
        XCTAssertEqual(1, screen.cursorY, "the cursor stays where it was inside the screen")
        XCTAssertEqual([0, 5], [screen.scrollTop, screen.scrollBottom],
                       "a resize restores the whole screen as the scroll region")

        screen.resize(cols: 2, rows: 2)

        XCTAssertEqual(["ab", "de"], lines(screen),
                       "shrinking keeps the overlap and drops what no longer fits")
        XCTAssertEqual(1, screen.cursorX, "shrinking clamps the cursor into the new screen")
        XCTAssertEqual(1, screen.cursorY, "shrinking clamps the cursor into the new screen")

        // A size that is not a screen is ignored rather than applied: a surface that has
        // not been laid out yet has no size, and a zero-column screen is not one.
        screen.resize(cols: 0, rows: 0)
        XCTAssertEqual([2, 2], [screen.cols, screen.rows], "an impossible size was applied")
    }

    func testResizingReplacesAWideGlyphItSplits() {
        let screen = TerminalScreen(cols: 4, rows: 1)
        screen.putGlyph("世", width: 2)

        XCTAssertEqual("世  ", screen.rowText(0))
        XCTAssertEqual(2, screen.cell(col: 0, row: 0).width, "the wide glyph's lead cell keeps its width")
        XCTAssertTrue(screen.cell(col: 1, row: 0).isSpacer, "the glyph's spare column is a spacer")

        screen.resize(cols: 1, rows: 1)

        // Half of a glyph is not a glyph, and a cell that claims a column it does not have
        // would put every column after it out by one.
        XCTAssertEqual(" ", screen.rowText(0))
        XCTAssertEqual(1, screen.cell(col: 0, row: 0).width)
        XCTAssertNotEqual(2, screen.cell(col: 0, row: 0).width, "a split wide glyph was kept whole")
    }

    func testACellOutsideTheScreenIsBlank() {
        let screen = TerminalScreen(cols: 2, rows: 2)
        type(screen, "ab")

        XCTAssertEqual("a", screen.cell(col: 0, row: 0).glyph)
        XCTAssertEqual("b", screen.cell(col: 1, row: 0).glyph)
        XCTAssertEqual(" ", screen.cell(col: 9, row: 9).glyph, "a cell off the screen is blank")
        XCTAssertEqual(.blank, screen.cell(col: -1, row: 0))
        XCTAssertEqual(.blank, screen.cell(col: 0, row: 2))
        XCTAssertEqual([], screen.scrollbackLine(0).filter { $0.glyph != " " },
                       "a line that does not exist has no characters")
    }

    func testResetReturnsTheScreenToAFreshState() {
        let screen = TerminalScreen(cols: 4, rows: 2)
        type(screen, "MAIN")
        screen.setAltScreen(true, saveCursor: true)
        type(screen, "ALT")
        screen.setCursorVisible(false)
        screen.setMouseReporting(true)
        screen.setMouseSGR(true)
        screen.setScrollRegion(top: 0, bottom: 0)
        screen.setPen(TerminalPen(foreground: .palette(1), bold: true))
        screen.saveCursor()

        screen.reset()

        XCTAssertFalse(screen.isAltScreen, "a reset leaves the alternate screen")
        XCTAssertTrue(screen.cursorVisible)
        XCTAssertFalse(screen.mouseReporting)
        XCTAssertFalse(screen.mouseSGR)
        XCTAssertEqual(.plain, screen.pen)
        XCTAssertEqual(0, screen.cursorX)
        XCTAssertEqual(0, screen.cursorY)
        XCTAssertEqual([0, 1], [screen.scrollTop, screen.scrollBottom])
        XCTAssertEqual(["    ", "    "], lines(screen), "a reset blanks the screen")
        XCTAssertEqual(0, screen.scrollbackCount)
        XCTAssertEqual(0, screen.linesScrolledOff)

        // The saved cursor a reset leaves behind is the origin, not the borrowed one.
        screen.restoreCursor()
        XCTAssertEqual(0, screen.cursorX)
        XCTAssertEqual(0, screen.cursorY)
    }
}
