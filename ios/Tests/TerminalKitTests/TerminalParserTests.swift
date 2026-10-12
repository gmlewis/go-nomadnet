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

/// The emulated terminal's parser: the escape sequences a client emits, and what they do
/// to the screen.
///
/// `tui/vterm.go` is the same parser in Go and these vectors are readable against it, but
/// the Swift side has to be more than a port in four places, and each of them is a test
/// below: Go has no OSC handling at all (so no title and no clipboard), no DEC special
/// graphics (so a box-drawing page prints letters), no wide-character handling, and a
/// package-level UTF-8 accumulator shared by every screen.
///
/// A terminal is fed a hostile byte stream by definition — anything the client on the
/// other end of the socket prints arrives here unexamined — so the last two tests are
/// seeded fuzz cases: no input may panic, and none may grow the screen or the parser
/// without limit. They are bounded and deterministic, like `cmd/stress-test-nomadnet`.
final class TerminalParserTests: XCTestCase {

    /// Fixture is a parser and the sinks it reports to, which is everything a test needs.
    private final class Fixture {
        let screen: TerminalScreen
        var titles: [String] = []
        var clipboard: [String] = []
        let parser: TerminalParser

        init(cols: Int = 20, rows: Int = 4) {
            screen = TerminalScreen(cols: cols, rows: rows)
            parser = TerminalParser(screen: screen)
            parser.titleSink = { [weak self] in self?.titles.append($0) }
            parser.clipboardSink = { [weak self] in self?.clipboard.append($0) }
        }

        /// write feeds the parser a string, which is what makes a test readable.
        func write(_ text: String) {
            parser.parse(Array(text.utf8))
        }

        func write(bytes: [UInt8]) {
            parser.parse(bytes)
        }

        /// lines renders the whole grid, so a test can state the screen it expects.
        func lines() -> [String] {
            (0..<screen.rows).map { screen.rowText($0) }
        }
    }

    /// assertSurvives checks the screen is still a screen: its size, and a cursor inside it.
    private func assertSurvives(_ screen: TerminalScreen, file: StaticString = #filePath, line: UInt = #line) {
        XCTAssertTrue(screen.cols > 0 && screen.rows > 0, "the screen lost its size", file: file, line: line)
        XCTAssertTrue(
            screen.cursorX >= 0 && screen.cursorX < screen.cols
                && screen.cursorY >= 0 && screen.cursorY < screen.rows,
            "the cursor left the screen: (\(screen.cursorX),\(screen.cursorY))",
            file: file, line: line
        )
        for y in 0..<screen.rows {
            // A row of wide glyphs spells fewer characters than it has columns, since the
            // spare column after each one carries no character of its own; what no input
            // may do is make a row grow past the two characters a cell can hold.
            XCTAssertTrue(screen.rowText(y).count <= 2 * screen.cols,
                          "row \(y) grew past its cells", file: file, line: line)
        }
    }

    func testPlainTextLandsOnTheScreen() {
        let fixture = Fixture(cols: 16, rows: 3)

        fixture.write("hi\tthere")

        // A tab is the next multiple of eight, as it is in tcell and in vterm.go's tab().
        XCTAssertEqual("hi      there   ", fixture.screen.rowText(0))
        XCTAssertEqual(13, fixture.screen.cursorX)

        // The controls a line of text arrives with: carriage return, backspace, and the
        // line feed, which keeps the column. Each of them also ends a deferred wrap, as
        // vterm.go's groundByte does.
        let controls = Fixture(cols: 8, rows: 2)
        controls.write("abc\rX")
        XCTAssertEqual("Xbc     ", controls.screen.rowText(0))
        controls.write("Y")
        XCTAssertEqual("XYc     ", controls.screen.rowText(0))
        controls.write("\u{8}Z")
        XCTAssertEqual("XZc     ", controls.screen.rowText(0))
        controls.write("\n")
        XCTAssertEqual(1, controls.screen.cursorY)
        XCTAssertEqual(2, controls.screen.cursorX, "the column is kept across the line feed")
        XCTAssertEqual("        ", controls.screen.rowText(1))

        // A bell is not rung and a control byte is not drawn: neither puts anything on the
        // screen, and neither moves the cursor.
        let quiet = Fixture(cols: 8, rows: 2)
        quiet.write("\u{7}\u{1}\u{2}")
        XCTAssertEqual("        ", quiet.screen.rowText(0))
        XCTAssertEqual(0, quiet.screen.cursorX)
    }

    func testAnSgrSequenceSetsAColorAndDoesNotPrint() {
        let cases: [(name: String, sequence: String, pen: TerminalPen)] = [
            ("red", "\u{1b}[31m", TerminalPen(foreground: .palette(1))),
            ("bright red", "\u{1b}[91m", TerminalPen(foreground: .palette(9))),
            ("a background", "\u{1b}[44m", TerminalPen(background: .palette(4))),
            ("a bright background", "\u{1b}[101m", TerminalPen(background: .palette(9))),
            ("a palette color", "\u{1b}[38;5;200m", TerminalPen(foreground: .palette(200))),
            ("a true color", "\u{1b}[38;2;10;20;30m", TerminalPen(foreground: .rgb(r: 10, g: 20, b: 30))),
            ("bold", "\u{1b}[1m", TerminalPen(bold: true)),
            ("italic and underline", "\u{1b}[3;4m", TerminalPen(italic: true, underline: true)),
            ("reverse and blink", "\u{1b}[5;7m", TerminalPen(blink: true, reverse: true)),
        ]

        for testCase in cases {
            let fixture = Fixture()
            fixture.write(testCase.sequence)

            XCTAssertEqual(String(repeating: "                    \n", count: 3) + "                    ",
                           fixture.screen.text(), "\(testCase.name): the sequence printed")
            XCTAssertEqual(testCase.pen, fixture.screen.pen, "\(testCase.name): the pen")
            XCTAssertEqual(0, fixture.screen.cursorX, "\(testCase.name): the cursor did not move")

            // A character written under a pen carries that pen into its cell, which is what
            // lets the renderer draw a screen that mixes colors.
            fixture.write("R")
            XCTAssertEqual(testCase.pen, fixture.screen.cell(col: 0, row: 0).pen,
                           "\(testCase.name): the cell's pen")
        }

        // SGR 0 returns the pen to the terminal's own colors and no attributes.
        let reset = Fixture()
        reset.write("\u{1b}[31;1;4;7m")
        XCTAssertNotEqual(TerminalPen.plain, reset.screen.pen, "the pen was set")
        reset.write("\u{1b}[0m")
        XCTAssertEqual(TerminalPen.plain, reset.screen.pen)

        // 39 and 49 are "the default color" rather than "no color".
        reset.write("\u{1b}[32;46m")
        reset.write("\u{1b}[39;49m")
        XCTAssertEqual(.default, reset.screen.pen.foreground)
        XCTAssertEqual(.default, reset.screen.pen.background)

        // The attributes a terminal can turn off again.
        reset.write("\u{1b}[1;3;4;5;7m")
        reset.write("\u{1b}[22;23;24;25;27m")
        XCTAssertEqual(.plain, reset.screen.pen, "an attribute was not turned off again")
    }

    func testAColonSubParameterSgrSequenceIsTolerated() {
        // ITU T.416 spells some attributes with colons (4:2 is doubly underlined). vterm.go
        // does not implement them and neither does this: the sequence is swallowed whole
        // rather than printing its own digits.
        let fixture = Fixture()

        fixture.write("\u{1b}[4:2m")
        fixture.write("\u{1b}[58:5:1m")
        fixture.write("ok")

        XCTAssertEqual("ok                  ", fixture.screen.rowText(0))
        XCTAssertEqual(TerminalPen.plain, fixture.screen.pen, "a colon sequence changed the pen")
    }

    func testACsiWithASpaceIntermediateIsTolerated() {
        // \x1b[1 q is DECSCUSR, a cursor style the emulator does not draw. The intermediate
        // byte is not a parameter and not a final, so the sequence ends there without
        // printing anything, exactly as vterm.go's csiByte ends it.
        let fixture = Fixture()

        fixture.write("\u{1b}[1 q")
        fixture.write("\u{1b}[ q")
        fixture.write("ok")

        XCTAssertEqual("ok                  ", fixture.screen.rowText(0))
        XCTAssertEqual(TerminalPen.plain, fixture.screen.pen)
    }

    func testCursorAddressingMovesTheCursor() {
        let cases: [(name: String, sequence: String, x: Int, y: Int)] = [
            ("home", "\u{1b}[H", 0, 0),
            ("row and column", "\u{1b}[5;10H", 9, 4),
            ("the f form", "\u{1b}[3;4f", 3, 2),
            ("an omitted row", "\u{1b}[;6H", 5, 0),
            ("beyond the screen", "\u{1b}[99;99H", 19, 5),
            ("a huge parameter", "\u{1b}[99999999;99999999H", 19, 5),
        ]

        for testCase in cases {
            let fixture = Fixture(cols: 20, rows: 6)
            fixture.write(testCase.sequence)

            XCTAssertEqual(testCase.x, fixture.screen.cursorX, "\(testCase.name): the column")
            XCTAssertEqual(testCase.y, fixture.screen.cursorY, "\(testCase.name): the row")
            assertSurvives(fixture.screen)
        }

        // The relative moves, the line moves and the absolute ones are clamped the same way.
        let moves = Fixture(cols: 20, rows: 4)
        moves.write("\u{1b}[3;5H")
        XCTAssertEqual(4, moves.screen.cursorX, "row and column")
        XCTAssertEqual(2, moves.screen.cursorY, "row and column")
        moves.write("\u{1b}[1A")
        XCTAssertEqual(1, moves.screen.cursorY, "up")
        moves.write("\u{1b}[2B")
        XCTAssertEqual(3, moves.screen.cursorY, "down")
        moves.write("\u{1b}[3C")
        XCTAssertEqual(7, moves.screen.cursorX, "right")
        moves.write("\u{1b}[4D")
        XCTAssertEqual(3, moves.screen.cursorX, "left")
        moves.write("\u{1b}[1E")
        XCTAssertEqual(0, moves.screen.cursorX, "a next-line move goes to the first column")
        XCTAssertEqual(3, moves.screen.cursorY)
        moves.write("\u{1b}[999B")
        XCTAssertEqual(3, moves.screen.cursorY, "a move past the bottom is clamped")
        moves.write("\u{1b}[9d\u{1b}[7G")
        XCTAssertEqual(6, moves.screen.cursorX, "absolute row and column")
        XCTAssertEqual(3, moves.screen.cursorY)
        moves.write("\u{1b}[2F")
        XCTAssertEqual(0, moves.screen.cursorX, "a previous-line move goes to the first column")
        XCTAssertEqual(1, moves.screen.cursorY)
        moves.write("\u{1b}`")
        XCTAssertEqual(0, moves.screen.cursorX, "the backtick form is a column move with no parameter")
    }

    func testSaveAndRestoreCursorRoundTrips() {
        // Both spellings: DECSC/DECRC (ESC 7 / ESC 8) and the CSI s / CSI u pair. The save
        // covers the pen as well as the position, which is what a real terminal restores
        // and what a client relies on when it borrows the cursor to draw.
        for (save, restore) in [("\u{1b}7", "\u{1b}8"), ("\u{1b}[s", "\u{1b}[u")] {
            let fixture = Fixture(cols: 20, rows: 4)

            fixture.write("\u{1b}[3;7H\u{1b}[31m")
            fixture.write(save)
            fixture.write("\u{1b}[H\u{1b}[42m")
            fixture.write(restore)

            XCTAssertEqual(6, fixture.screen.cursorX, "\(save): the column comes back")
            XCTAssertEqual(2, fixture.screen.cursorY, "\(save): the row comes back")
            XCTAssertEqual(TerminalPen(foreground: .palette(1)), fixture.screen.pen,
                           "\(save): the pen comes back")
        }
    }

    func testAUtf8SequencePrintsOneRune() {
        let fixture = Fixture(cols: 10, rows: 2)

        fixture.write("aéb")

        XCTAssertEqual("aéb       ", fixture.screen.rowText(0))
        XCTAssertEqual(3, fixture.screen.cursorX, "one rune, one cell")

        // The bytes of a rune can be split across two reads: a pseudo-terminal hands over
        // whatever it has, so a partial sequence waits for its continuation rather than
        // printing a replacement character.
        let split = Fixture(cols: 10, rows: 2)
        split.write(bytes: [0xc3])
        XCTAssertEqual("          ", split.screen.rowText(0),
                       "nothing is printed until the rune is complete")
        split.write(bytes: [0xa9])
        XCTAssertEqual("é         ", split.screen.rowText(0))
        XCTAssertEqual(1, split.screen.cursorX)

        // A byte that cannot begin a rune is replaced rather than swallowed, and the byte
        // after it is still read on its own: the decoder consumes exactly what it decoded,
        // which is what keeps a malformed stream from eating the text behind it.
        let malformed = Fixture(cols: 10, rows: 2)
        malformed.write(bytes: [0x80, UInt8(ascii: "A")])
        XCTAssertEqual("\u{FFFD}A        ", malformed.screen.rowText(0))
        malformed.write(bytes: [0xc3, UInt8(ascii: "B")])
        XCTAssertEqual("\u{FFFD}A\u{FFFD}B      ", malformed.screen.rowText(0))

        // A structurally complete sequence whose value is not a character — an overlong
        // form, a surrogate half, a code point past the last one — is replaced as a whole.
        let strict = Fixture(cols: 10, rows: 2)
        strict.write(bytes: [0xe0, 0x80, 0xaf]) // an overlong "/"
        strict.write(bytes: [0xed, 0xa0, 0x80]) // a surrogate half
        strict.write(bytes: [0xf4, 0x90, 0x80, 0x80]) // U+110000, past the last code point
        XCTAssertEqual(String(repeating: "\u{FFFD}", count: 3) + "       ", strict.screen.rowText(0))
        XCTAssertEqual(3, strict.screen.cursorX)
    }

    func testAWideCharacterOccupiesTwoCells() {
        let fixture = Fixture(cols: 6, rows: 2)

        fixture.write("ab世c")

        // U+4E16 is double width: it takes the cell it is written to and the one after it,
        // and the cursor moves two columns. The spare cell carries no character of its own,
        // so the row spells one character fewer than it has columns.
        XCTAssertEqual("ab世c ", fixture.screen.rowText(0))
        XCTAssertEqual(5, fixture.screen.cursorX)

        // A glyph too wide for the end of a line moves whole to the next one, rather than
        // being split across the margin.
        let margin = Fixture(cols: 3, rows: 2)
        margin.write("ab世")
        XCTAssertEqual("ab ", margin.screen.rowText(0))
        XCTAssertEqual("世 ", margin.screen.rowText(1))

        // A character outside the BMP arrives as one rune in four bytes and is held in the
        // two cells a terminal needs to spell it.
        let supplementary = Fixture(cols: 6, rows: 2)
        supplementary.write("😀")
        XCTAssertEqual("😀    ", supplementary.screen.rowText(0))
        XCTAssertEqual(2, supplementary.screen.cursorX, "two cells")
    }

    func testDisplayWidthIsTwoForTheWideRanges() {
        // The ranges are the East Asian Wide and Fullwidth ones, which is what a CJK page, a
        // fullwidth form and most emoji are drawn from.
        let wide: [UInt32] = [
            0x1100, 0x115f, // Hangul Jamo
            0x2e80, 0x3041, 0x33ff, // radicals, Hiragana, compatibility
            0x4e00, 0x9fff, // CJK unified ideographs
            0xa000, 0xac00, 0xd7a3, // Yi, Hangul syllables
            0xf900, 0xfe10, 0xfe30, 0xff00, 0xffe0, 0xffe6, // compatibility and fullwidth
            0x1f300, 0x1f64f, // pictographs
            0x1f680, 0x1f6ff, 0x1f900, 0x1f9ff, // transport, supplemental
            0x20000, 0x3fffd, // extensions B and beyond
        ]
        for scalar in wide {
            XCTAssertEqual(2, TerminalParser.displayWidth(scalar),
                           String(format: "U+%04X is not two columns", scalar))
        }

        let narrow: [UInt32] = [0x20, 0x41, 0x7f, 0x300, 0x5d0, 0x10ff, 0x2000, 0x1f000, 0x10000, 0x1f700, 0x40000]
        for scalar in narrow {
            XCTAssertEqual(1, TerminalParser.displayWidth(scalar),
                           String(format: "U+%04X is not one column", scalar))
        }
    }

    func testAnUnknownEscapeSequenceIsIgnoredWithoutPrinting() {
        let cases: [(name: String, sequence: String)] = [
            ("an unassigned escape", "\u{1b}Z"),
            ("a charset designation", "\u{1b}(B"),
            ("a charset designation with a digit", "\u{1b}(1"),
            ("a csi ending on a control byte", "\u{1b}[12\u{7}"),
            ("an escape ending on a control byte", "\u{1b}\u{7}"),
        ]

        for testCase in cases {
            let fixture = Fixture(cols: 10, rows: 2)
            fixture.write(testCase.sequence)

            XCTAssertEqual("          ", fixture.screen.rowText(0), "\(testCase.name): something was printed")
            XCTAssertEqual(TerminalPen.plain, fixture.screen.pen,
                           "\(testCase.name): the parser did not return to the ground")

            fixture.write("A")
            XCTAssertEqual("A         ", fixture.screen.rowText(0),
                           "\(testCase.name): the text after it is lost")
        }

        // A sequence the buffer ended in the middle of is not a lost sequence: the next byte
        // it reads is its final, and the text behind that still prints. A client that wrote
        // `CSI z` and a client whose write was split between them look the same to the
        // parser, which is the point.
        let open = Fixture(cols: 10, rows: 2)
        open.write("\u{1b}[")
        open.write("z")
        open.write("A")
        XCTAssertEqual("A         ", open.screen.rowText(0),
                       "the text after the sequence's final still prints")
    }

    func testALoneEscapeAtEndOfInputDoesNotLoseTheNextBuffer() {
        // A read can end anywhere, including in the middle of an escape sequence, so the
        // parser holds its state until the rest of the sequence arrives.
        let fixture = Fixture(cols: 8, rows: 2)

        fixture.write("ab\u{1b}")
        fixture.write("[2J")

        XCTAssertEqual("        ", fixture.screen.rowText(0),
                       "the sequence that was split across two reads was completed")
        XCTAssertEqual(2, fixture.screen.cursorX, "erasing does not move the cursor")

        let split = Fixture(cols: 8, rows: 2)
        split.write("\u{1b}[3")
        split.write("1m")
        split.write("R")
        XCTAssertEqual(TerminalPen(foreground: .palette(1)), split.screen.pen)
        XCTAssertEqual("R       ", split.screen.rowText(0))
    }

    func testOsc2SetsTheTitle() {
        let fixture = Fixture()

        fixture.write("\u{1b}]2;Hello\u{7}")
        fixture.write("\u{1b}]2;World\u{1b}\\")

        XCTAssertEqual(["Hello", "World"], fixture.titles)
        // A string terminator is not text: the parser returns to the ground on the escape
        // and never prints the backslash that ends the sequence.
        XCTAssertEqual("                    ", fixture.screen.rowText(0), "the title was printed")

        // Something other than a title is not a title, and neither is one with no separator.
        fixture.write("\u{1b}]0;ignored\u{7}")
        fixture.write("\u{1b}]2\u{7}")
        XCTAssertEqual(["Hello", "World"], fixture.titles)
    }

    func testOsc52IsDeliveredToTheClipboardSink() {
        let fixture = Fixture()

        fixture.write("\u{1b}]52;c;SGVsbG8=\u{7}")
        XCTAssertEqual(["Hello"], fixture.clipboard)
        XCTAssertEqual("                    ", fixture.screen.rowText(0), "the payload was printed")

        // A query (no payload or just "?") asks the terminal what the clipboard holds; it is
        // not something to put on the clipboard, and neither is nonsense.
        fixture.write("\u{1b}]52;c;?\u{7}")
        fixture.write("\u{1b}]52;c;not base64!!\u{7}")
        fixture.write("\u{1b}]52;c;\u{7}")
        fixture.write("\u{1b}]52;c\u{7}")
        XCTAssertEqual(["Hello"], fixture.clipboard, "only the real payload was delivered")

        // A payload whose length is not a multiple of four is malformed, and so is one whose
        // padding is in the wrong place.
        fixture.write("\u{1b}]52;c;SGVsbG8\u{7}")
        fixture.write("\u{1b}]52;c;SGVsbG8=x\u{7}")
        XCTAssertEqual(["Hello"], fixture.clipboard)

        // Padding is where the payload ends, not part of what it says.
        fixture.write("\u{1b}]52;c;aGk=\u{7}")
        XCTAssertEqual(["Hello", "hi"], fixture.clipboard)
    }

    func testDecSpecialGraphicsDrawsBoxCharacters() {
        let fixture = Fixture(cols: 8, rows: 2)

        fixture.write("\u{1b}(0lqk")
        XCTAssertEqual("┌─┐     ", fixture.screen.rowText(0))

        fixture.write("\u{1b}(0mx")
        XCTAssertEqual("┌─┐└│   ", fixture.screen.rowText(0))

        // Designating the ASCII charset again ends the translation, which is how a client
        // draws a box and then writes ordinary text into it. G1 exists but is not selected:
        // nothing the client emits uses it.
        fixture.write("\u{1b}(B")
        fixture.write("lqk")
        XCTAssertEqual("┌─┐└│lqk", fixture.screen.rowText(0))

        let other = Fixture(cols: 8, rows: 2)
        other.write("\u{1b})0lqk")
        XCTAssertEqual("lqk     ", other.screen.rowText(0), "a G1 designation changed what G0 prints")
    }

    func testTheTcellStartupSequenceLeavesABlankScreen() {
        let fixture = Fixture(cols: 8, rows: 2)
        fixture.write("MAIN")

        fixture.write("\u{1b}[?1049h\u{1b}[?25l\u{1b}[?2004h")

        XCTAssertTrue(fixture.screen.isAltScreen, "the alternate screen is in use")
        XCTAssertFalse(fixture.screen.cursorVisible, "the cursor is hidden")
        XCTAssertEqual("        ", fixture.screen.rowText(0), "the alternate screen starts blank")
        XCTAssertEqual(0, fixture.screen.cursorX, "the alternate screen starts at the origin")
        XCTAssertEqual(0, fixture.screen.cursorY)
    }

    func testTheTcellShutdownSequenceRestoresThePrimaryScreen() {
        let fixture = Fixture(cols: 8, rows: 2)
        fixture.write("MAIN")
        fixture.write("\u{1b}[?1049h\u{1b}[?25l\u{1b}[?2004h")
        fixture.write("\u{1b}[?1000h\u{1b}[?1006h")
        fixture.write("ALT")
        XCTAssertTrue(fixture.screen.mouseReporting, "mouse reporting is on while the child asks for it")
        XCTAssertTrue(fixture.screen.mouseSGR, "and so is the SGR encoding")

        fixture.write("\u{1b}[?2004l\u{1b}[?1000l\u{1b}[?1006l\u{1b}[?25h\u{1b}[?1049l\u{1b}[0m")

        XCTAssertFalse(fixture.screen.isAltScreen, "the primary screen is in use again")
        XCTAssertTrue(fixture.screen.cursorVisible, "the cursor is shown again")
        XCTAssertEqual("MAIN    ", fixture.screen.rowText(0), "the primary screen comes back")
        XCTAssertFalse(fixture.screen.mouseReporting, "mouse reporting is off again")
        XCTAssertFalse(fixture.screen.mouseSGR, "the mouse encoding is off again")
        XCTAssertEqual(TerminalPen.plain, fixture.screen.pen, "the pen is reset again")

        // Only a private mode is a mode this emulator has: the same numbers without the
        // marker are the terminal's own business and change nothing.
        let plain = Fixture(cols: 8, rows: 2)
        plain.write("\u{1b}[25l\u{1b}[1049h\u{1b}[1000h")
        XCTAssertTrue(plain.screen.cursorVisible, "a non-private mode hid the cursor")
        XCTAssertFalse(plain.screen.isAltScreen, "a non-private mode switched screens")
        XCTAssertFalse(plain.screen.mouseReporting, "a non-private mode turned the mouse on")

        // The legacy switches keep the cursor where the alternate screen left it, and the
        // erase and scroll sequences the client uses for a frame are all read.
        let legacy = Fixture(cols: 6, rows: 3)
        legacy.write("\u{1b}[?47h")
        XCTAssertTrue(legacy.screen.isAltScreen, "the legacy switch did not enter the alternate screen")
        legacy.write("\u{1b}[2;3Hxy\u{1b}[?1047l")
        XCTAssertFalse(legacy.screen.isAltScreen, "the legacy switch did not leave it")
        XCTAssertEqual(4, legacy.screen.cursorX, "a legacy switch restored the cursor")
        XCTAssertEqual(1, legacy.screen.cursorY)
    }

    func testEveryByteValueFrom0x00To0xffIsSurvivable() {
        var random = SeededRandom(seed: 0x5eed)

        // Each byte on its own, in a seeded order, in a screen of its own.
        var order = Array(0...255).map { UInt8($0) }
        random.shuffle(&order)

        for value in order {
            let fixture = Fixture(cols: 8, rows: 3)
            fixture.write(bytes: [value])
            assertSurvives(fixture.screen)
        }

        // And every byte in one parser, twice, so the sequences they form between them are
        // survived as well.
        let together = Fixture(cols: 8, rows: 3)
        for pass in 1...2 {
            for value in order {
                together.write(bytes: [value])
            }
            XCTAssertEqual(8, together.screen.cols, "pass \(pass) changed the screen's size")
            assertSurvives(together.screen)
        }
    }

    func testAMegabyteOfRandomBytesNeverPanics() {
        let fixture = Fixture(cols: 80, rows: 24)
        var random = SeededRandom(seed: 0xfeed)

        for _ in 0..<256 { // 256 chunks of 4 KiB: 1 MiB in total
            fixture.write(bytes: random.bytes(count: 4096))
        }

        assertSurvives(fixture.screen)
        XCTAssertEqual(80, fixture.screen.cols, "the screen kept its size")
        XCTAssertEqual(24, fixture.screen.rows)

        // A hostile stream can also open a string that never ends. The parser holds at most
        // a bounded amount of it, so the terminal recovers and is usable again rather than
        // accumulating a megabyte of a title it will never be given. This one starts from a
        // fresh parser, so what it is in the middle of is not left to the random bytes above.
        let hostile = Fixture(cols: 80, rows: 24)
        var unterminated = "\u{1b}]52;c;"
        unterminated += String(repeating: "A", count: 1 << 20)
        hostile.write(unterminated)
        hostile.write("\u{7}\u{1b}[2J")

        assertSurvives(hostile.screen)
        XCTAssertEqual(String(repeating: " ", count: 80), hostile.screen.rowText(0),
                       "the screen was erased after the hostile string")
        XCTAssertEqual([], hostile.clipboard, "the hostile string was not put on the clipboard")
        hostile.write("ok")
        XCTAssertEqual("ok" + String(repeating: " ", count: 78), hostile.screen.rowText(0))
    }
}

/// SeededRandom is a small deterministic generator, so that a fuzz case reports the same
/// bytes on every run and a failure can be looked at again rather than chased.
private struct SeededRandom {
    private var state: UInt64

    init(seed: UInt64) {
        state = seed
    }

    /// next returns the next 64 bits of the xorshift sequence.
    mutating func next() -> UInt64 {
        state ^= state << 13
        state ^= state >> 7
        state ^= state << 17
        return state
    }

    /// bytes returns count random bytes, which is what a hostile peer would send.
    mutating func bytes(count: Int) -> [UInt8] {
        var out = [UInt8]()
        out.reserveCapacity(count)
        while out.count < count {
            var value = next()
            for _ in 0..<8 where out.count < count {
                out.append(UInt8(truncatingIfNeeded: value))
                value >>= 8
            }
        }
        return out
    }

    /// shuffle reorders a sequence in place, the way a seeded test needs it to.
    mutating func shuffle<T>(_ values: inout [T]) {
        guard values.count > 1 else { return }
        for i in stride(from: values.count - 1, to: 0, by: -1) {
            let j = Int(next() % UInt64(i + 1))
            values.swapAt(i, j)
        }
    }
}
