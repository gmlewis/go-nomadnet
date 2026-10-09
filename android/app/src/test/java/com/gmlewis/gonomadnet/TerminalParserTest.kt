// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The emulated terminal's parser: the escape sequences a client emits, and what they
 * do to the screen.
 *
 * `tui/vterm.go` is the same parser in Go and these vectors are readable against it,
 * but the Kotlin side has to be more than a port in four places, and each of them is a
 * test below: Go has no OSC handling at all (so no title and no clipboard), no DEC
 * special graphics (so a box-drawing page prints letters), no wide-character handling,
 * and a package-level UTF-8 accumulator shared by every screen.
 *
 * A terminal is fed a hostile byte stream by definition — anything the client on the
 * other end of the socket prints arrives here unexamined — so the last two tests are
 * seeded fuzz cases: no input may panic, and none may grow the screen or the parser
 * without limit. They are bounded and deterministic, like `cmd/stress-test-nomadnet`.
 */
class TerminalParserTest {

    /** A parser and the sinks it reports to, which is everything a test needs. */
    private class Fixture(cols: Int = 20, rows: Int = 4) {
        val screen = TerminalScreen(cols = cols, rows = rows)
        val titles = mutableListOf<String>()
        val clipboard = mutableListOf<String>()
        val drawn = mutableListOf<Unit>()
        val parser = TerminalParser(
            screen = screen,
            titleSink = { titles += it },
            clipboardSink = { clipboard += it },
            onOutput = { drawn += Unit },
        )
    }

    @Test
    fun `the parser says when it has been given something to draw`() {
        // The view has to be told there is something new on the screen, and the parser is the
        // only place that can know. It reports once per write rather than once per byte: a
        // repaint per character would be a repaint per character on the screen.
        val fixture = Fixture()
        assertEquals("a fresh parser reported output", 0, fixture.drawn.size)

        fixture.parser.write("hi")
        assertEquals("plain text was not reported", 1, fixture.drawn.size)

        // A write that draws no character is still a write. An escape sequence is the client
        // asking for a repaint, and the screen after it is not the screen before it.
        fixture.parser.write("\u001b[2J")
        assertEquals("a control sequence was not reported", 2, fixture.drawn.size)

        // A write with nothing in it cannot have changed anything, so it is not a repaint:
        // a client that is idle must not keep the view redrawing.
        fixture.parser.write(ByteArray(0))
        assertEquals("an empty write was reported as output", 2, fixture.drawn.size)

        fixture.parser.write("!")
        assertEquals("the parser stopped reporting", 3, fixture.drawn.size)
    }

    /** lines renders the whole grid, so a test can state the screen it expects. */
    private fun Fixture.lines(): List<String> = (0 until screen.rows).map { screen.rowText(it) }

    /** assertSurvives checks the screen is still a screen: its size, and a cursor inside it. */
    private fun assertSurvives(screen: TerminalScreen) {
        assertTrue("the screen lost its size", screen.cols > 0 && screen.rows > 0)
        assertTrue(
            "the cursor left the screen: (${screen.cursorX},${screen.cursorY})",
            screen.cursorX in 0 until screen.cols && screen.cursorY in 0 until screen.rows,
        )
        for (y in 0 until screen.rows) {
            // A row of wide glyphs spells fewer characters than it has columns, since
            // the spare column after each one carries no character of its own; what no
            // input may do is make a row grow past the two characters a cell can hold.
            assertTrue("row $y grew past its cells", screen.rowText(y).length <= 2 * screen.cols)
        }
    }

    @Test
    fun `plain text lands on the screen`() {
        val fixture = Fixture(cols = 16, rows = 3)

        fixture.parser.write("hi\tthere")

        // A tab is the next multiple of eight, as it is in tcell and in vterm.go's tab().
        assertEquals("hi      there   ", fixture.screen.rowText(0))
        assertEquals(13, fixture.screen.cursorX)

        // The controls a line of text arrives with: carriage return, backspace, and the
        // line feed, which keeps the column. Each of them also ends a deferred wrap, as
        // vterm.go's groundByte does.
        val controls = Fixture(cols = 8, rows = 2)
        controls.parser.write("abc\rX")
        assertEquals("Xbc     ", controls.screen.rowText(0))
        controls.parser.write("Y")
        assertEquals("XYc     ", controls.screen.rowText(0))
        controls.parser.write("\bZ")
        assertEquals("XZc     ", controls.screen.rowText(0))
        controls.parser.write("\n")
        assertEquals(1, controls.screen.cursorY)
        assertEquals("the column is kept across the line feed", 2, controls.screen.cursorX)
        assertEquals("        ", controls.screen.rowText(1))
    }

    @Test
    fun `an sgr sequence sets a colour and does not print`() {
        val cases = listOf(
            SgrCase("red", "\u001b[31m", TerminalPen(foreground = TerminalColor.Palette(1))),
            SgrCase("bright red", "\u001b[91m", TerminalPen(foreground = TerminalColor.Palette(9))),
            SgrCase("a background", "\u001b[44m", TerminalPen(background = TerminalColor.Palette(4))),
            SgrCase("a bright background", "\u001b[101m", TerminalPen(background = TerminalColor.Palette(9))),
            SgrCase("a palette colour", "\u001b[38;5;200m", TerminalPen(foreground = TerminalColor.Palette(200))),
            SgrCase("a true colour", "\u001b[38;2;10;20;30m", TerminalPen(foreground = TerminalColor.Rgb(0x0a141e))),
            SgrCase("bold", "\u001b[1m", TerminalPen(bold = true)),
            SgrCase("italic and underline", "\u001b[3;4m", TerminalPen(italic = true, underline = true)),
            SgrCase("reverse and blink", "\u001b[5;7m", TerminalPen(blink = true, reverse = true)),
        )

        for (case in cases) {
            val fixture = Fixture()
            fixture.parser.write(case.sequence)

            assertEquals("${case.name}: the sequence printed", "                    \n".repeat(3) + "                    ", fixture.screen.text())
            assertEquals("${case.name}: the pen", case.pen, fixture.screen.pen)
            assertEquals("${case.name}: the cursor did not move", 0, fixture.screen.cursorX)

            // A character written under a pen carries that pen into its cell, which is
            // what lets the renderer draw a screen that mixes colours.
            fixture.parser.write("R")
            assertEquals("${case.name}: the cell's pen", case.pen, fixture.screen.penAt(0, 0))
        }

        // SGR 0 returns the pen to the terminal's own colours and no attributes.
        val reset = Fixture()
        reset.parser.write("\u001b[31;1;4;7m")
        assertFalse("the pen was set", reset.screen.pen == TerminalPen.DEFAULT)
        reset.parser.write("\u001b[0m")
        assertEquals(TerminalPen.DEFAULT, reset.screen.pen)

        // 39 and 49 are "the default colour" rather than "no colour".
        reset.parser.write("\u001b[32;46m")
        reset.parser.write("\u001b[39;49m")
        assertTrue(
            "39 and 49 return both colours to the default",
            reset.screen.pen.foreground == TerminalColor.Default && reset.screen.pen.background == TerminalColor.Default,
        )
    }

    @Test
    fun `a colon sub-parameter sgr sequence is tolerated`() {
        // ITU T.416 spells some attributes with colons (4:2 is doubly underlined).
        // vterm.go does not implement them and neither does this: the sequence is
        // swallowed whole rather than printing its own digits.
        val fixture = Fixture()

        fixture.parser.write("\u001b[4:2m")
        fixture.parser.write("\u001b[58:5:1m")
        fixture.parser.write("ok")

        assertEquals("ok                  ", fixture.screen.rowText(0))
        assertEquals("a colon sequence changed the pen", TerminalPen.DEFAULT, fixture.screen.pen)
    }

    @Test
    fun `a csi with a space intermediate is tolerated`() {
        // \x1b[1 q is DECSCUSR, a cursor style the emulator does not draw. The
        // intermediate byte is not a parameter and not a final, so the sequence ends
        // there without printing anything, exactly as vterm.go's csiByte ends it.
        val fixture = Fixture()

        fixture.parser.write("\u001b[1 q")
        fixture.parser.write("\u001b[ q")
        fixture.parser.write("ok")

        assertEquals("ok                  ", fixture.screen.rowText(0))
        assertEquals(TerminalPen.DEFAULT, fixture.screen.pen)
    }

    @Test
    fun `cursor addressing moves the cursor`() {
        val cases = listOf(
            CursorCase("home", "\u001b[H", 0, 0),
            CursorCase("row and column", "\u001b[5;10H", 9, 4),
            CursorCase("the f form", "\u001b[3;4f", 3, 2),
            CursorCase("an omitted row", "\u001b[;6H", 5, 0),
            CursorCase("beyond the screen", "\u001b[99;99H", 19, 5),
            CursorCase("a huge parameter", "\u001b[99999999;99999999H", 19, 5),
        )

        for (case in cases) {
            val fixture = Fixture(cols = 20, rows = 6)
            fixture.parser.write(case.sequence)

            assertEquals("${case.name}: the column", case.x, fixture.screen.cursorX)
            assertEquals("${case.name}: the row", case.y, fixture.screen.cursorY)
            assertSurvives(fixture.screen)
        }

        // The relative moves, the line moves and the absolute ones are clamped the same way.
        val moves = Fixture(cols = 20, rows = 4)
        moves.parser.write("\u001b[3;5H")
        assertEquals("row and column", 4, moves.screen.cursorX)
        assertEquals("row and column", 2, moves.screen.cursorY)
        moves.parser.write("\u001b[1A")
        assertEquals("up", 1, moves.screen.cursorY)
        moves.parser.write("\u001b[2B")
        assertEquals("down", 3, moves.screen.cursorY)
        moves.parser.write("\u001b[3C")
        assertEquals("right", 7, moves.screen.cursorX)
        moves.parser.write("\u001b[4D")
        assertEquals("left", 3, moves.screen.cursorX)
        moves.parser.write("\u001b[1E")
        assertEquals("a next-line move goes to the first column", 0, moves.screen.cursorX)
        assertEquals(3, moves.screen.cursorY)
        moves.parser.write("\u001b[999B")
        assertEquals("a move past the bottom is clamped", 3, moves.screen.cursorY)
        moves.parser.write("\u001b[9d\u001b[7G")
        assertEquals("absolute row and column", 6, moves.screen.cursorX)
        assertEquals(3, moves.screen.cursorY)
    }

    @Test
    fun `save and restore cursor round-trips`() {
        // Both spellings: DECSC/DECRC (ESC 7 / ESC 8) and the CSI s / CSI u pair. The
        // save covers the pen as well as the position, which is what a real terminal
        // restores and what a client relies on when it borrows the cursor to draw.
        for (pair in listOf("\u001b7" to "\u001b8", "\u001b[s" to "\u001b[u")) {
            val (save, restore) = pair
            val fixture = Fixture(cols = 20, rows = 4)

            fixture.parser.write("\u001b[3;7H\u001b[31m")
            fixture.parser.write(save)
            fixture.parser.write("\u001b[H\u001b[42m")
            fixture.parser.write(restore)

            assertEquals("$save: the column comes back", 6, fixture.screen.cursorX)
            assertEquals("$save: the row comes back", 2, fixture.screen.cursorY)
            assertEquals(
                "$save: the pen comes back",
                TerminalPen(foreground = TerminalColor.Palette(1)),
                fixture.screen.pen,
            )
        }
    }

    @Test
    fun `a utf-8 sequence prints one rune`() {
        val fixture = Fixture(cols = 10, rows = 2)

        fixture.parser.write("aéb")

        assertEquals("aéb       ", fixture.screen.rowText(0))
        assertEquals("one rune, one cell", 3, fixture.screen.cursorX)

        // The bytes of a rune can be split across two reads: a pseudo-terminal hands
        // over whatever it has, so a partial sequence waits for its continuation
        // rather than printing a replacement character.
        val split = Fixture(cols = 10, rows = 2)
        split.parser.write(byteArrayOf(0xc3.toByte()))
        assertEquals("nothing is printed until the rune is complete", "          ", split.screen.rowText(0))
        split.parser.write(byteArrayOf(0xa9.toByte()))
        assertEquals("é         ", split.screen.rowText(0))
        assertEquals(1, split.screen.cursorX)

        // A byte that cannot begin a rune is replaced rather than swallowed, and the
        // byte after it is still read on its own: the decoder consumes exactly what it
        // decoded, which is what keeps a malformed stream from eating the text behind it.
        val malformed = Fixture(cols = 10, rows = 2)
        malformed.parser.write(byteArrayOf(0x80.toByte(), 'A'.code.toByte()))
        assertEquals("�A        ", malformed.screen.rowText(0))
        malformed.parser.write(byteArrayOf(0xc3.toByte(), 'B'.code.toByte()))
        assertEquals("�A�B      ", malformed.screen.rowText(0))
    }

    @Test
    fun `a wide character occupies two cells`() {
        val fixture = Fixture(cols = 6, rows = 2)

        fixture.parser.write("ab世c")

        // U+4E16 is double width: it takes the cell it is written to and the one after
        // it, and the cursor moves two columns. The spare cell carries no character of
        // its own, so the row spells one character fewer than it has columns.
        assertEquals("ab世c ", fixture.screen.rowText(0))
        assertEquals(5, fixture.screen.cursorX)

        // A glyph too wide for the end of a line moves whole to the next one, rather
        // than being split across the margin.
        val margin = Fixture(cols = 3, rows = 2)
        margin.parser.write("ab世")
        assertEquals("ab ", margin.screen.rowText(0))
        assertEquals("世 ", margin.screen.rowText(1))

        // A character outside the BMP arrives as one rune in four bytes and is held in
        // the two cells Kotlin needs to spell it.
        val supplementary = Fixture(cols = 6, rows = 2)
        supplementary.parser.write("😀")
        assertEquals("😀    ", supplementary.screen.rowText(0))
        assertEquals("two cells", 2, supplementary.screen.cursorX)
    }

    @Test
    fun `an unknown escape sequence is ignored without printing`() {
        val cases = listOf(
            "an unassigned escape" to "\u001bZ",
            "a charset designation" to "\u001b(B",
            "a charset designation with a digit" to "\u001b(1",
            "a csi ending on a control byte" to "\u001b[12\u0007",
            "an escape ending on a control byte" to "\u001b\u0007",
        )

        for ((name, sequence) in cases) {
            val fixture = Fixture(cols = 10, rows = 2)
            fixture.parser.write(sequence)

            assertEquals("$name: something was printed", "          ", fixture.screen.rowText(0))
            assertEquals("$name: the parser did not return to the ground", TerminalPen.DEFAULT, fixture.screen.pen)

            fixture.parser.write("A")
            assertEquals("$name: the text after it is lost", "A         ", fixture.screen.rowText(0))
        }

        // A sequence the buffer ended in the middle of is not a lost sequence: the next
        // byte it reads is its final, and the text behind that still prints. A client
        // that wrote `CSI z` and a client whose write was split between them look the
        // same to the parser, which is the point.
        val open = Fixture(cols = 10, rows = 2)
        open.parser.write("\u001b[")
        open.parser.write("z")
        open.parser.write("A")
        assertEquals("the text after the sequence's final still prints", "A         ", open.screen.rowText(0))
    }

    @Test
    fun `a lone escape at end of input does not lose the next buffer`() {
        // A read can end anywhere, including in the middle of an escape sequence, so
        // the parser holds its state until the rest of the sequence arrives.
        val fixture = Fixture(cols = 8, rows = 2)

        fixture.parser.write("ab\u001b")
        fixture.parser.write("[2J")

        assertEquals("the sequence that was split across two reads was completed", "        ", fixture.screen.rowText(0))
        assertEquals("erasing does not move the cursor", 2, fixture.screen.cursorX)

        val split = Fixture(cols = 8, rows = 2)
        split.parser.write("\u001b[3")
        split.parser.write("1m")
        split.parser.write("R")
        assertEquals(TerminalPen(foreground = TerminalColor.Palette(1)), split.screen.pen)
        assertEquals("R       ", split.screen.rowText(0))
    }

    @Test
    fun `osc 2 sets the title`() {
        val fixture = Fixture()

        fixture.parser.write("\u001b]2;Hello\u0007")
        fixture.parser.write("\u001b]2;World\u001b\\")

        assertEquals(listOf("Hello", "World"), fixture.titles)
        // A string terminator is not text: vterm.go returns to the ground state on the
        // ESC and then prints the backslash that ends the sequence.
        assertEquals("the title was printed", "                    ", fixture.screen.rowText(0))

        // Something other than a title is not a title, and neither is an empty one.
        fixture.parser.write("\u001b]0;ignored\u0007")
        assertEquals(listOf("Hello", "World"), fixture.titles)
    }

    @Test
    fun `osc 52 is delivered to the clipboard sink`() {
        val fixture = Fixture()

        fixture.parser.write("\u001b]52;c;SGVsbG8=\u0007")
        assertEquals(listOf("Hello"), fixture.clipboard)
        assertEquals("the payload was printed", "                    ", fixture.screen.rowText(0))

        // A query (no payload, just "?") asks the terminal what the clipboard holds;
        // it is not something to put on the clipboard, and neither is nonsense.
        fixture.parser.write("\u001b]52;c;?\u0007")
        fixture.parser.write("\u001b]52;c;not base64!!\u0007")
        assertEquals("only the real payload was delivered", listOf("Hello"), fixture.clipboard)

        // A payload whose length is not a multiple of four is malformed.
        fixture.parser.write("\u001b]52;c;SGVsbG8\u0007")
        assertEquals(listOf("Hello"), fixture.clipboard)
    }

    @Test
    fun `dec special graphics draws box characters`() {
        val fixture = Fixture(cols = 8, rows = 2)

        fixture.parser.write("\u001b(0lqk")
        assertEquals("┌─┐     ", fixture.screen.rowText(0))

        fixture.parser.write("\u001b(0mx")
        assertEquals("┌─┐└│   ", fixture.screen.rowText(0))

        // Designating the ASCII charset again ends the translation, which is how a
        // client draws a box and then writes ordinary text into it.
        fixture.parser.write("\u001b(B")
        fixture.parser.write("lqk")
        assertEquals("┌─┐└│lqk", fixture.screen.rowText(0))
    }

    @Test
    fun `the tcell startup sequence leaves a blank screen`() {
        val fixture = Fixture(cols = 8, rows = 2)
        fixture.parser.write("MAIN")

        fixture.parser.write("\u001b[?1049h\u001b[?25l\u001b[?2004h")

        assertTrue("the alternate screen is in use", fixture.screen.isAltScreen)
        assertFalse("the cursor is hidden", fixture.screen.cursorVisible)
        assertEquals("the alternate screen starts blank", "        ", fixture.screen.rowText(0))
        assertEquals("the alternate screen starts at the origin", 0, fixture.screen.cursorX)
        assertEquals(0, fixture.screen.cursorY)
    }

    @Test
    fun `the tcell shutdown sequence restores the primary screen`() {
        val fixture = Fixture(cols = 8, rows = 2)
        fixture.parser.write("MAIN")
        fixture.parser.write("\u001b[?1049h\u001b[?25l\u001b[?2004h")
        fixture.parser.write("\u001b[?1000h\u001b[?1006h")
        fixture.parser.write("ALT")
        assertTrue("mouse reporting is on while the child asks for it", fixture.screen.mouseReporting)
        assertTrue("and so is the SGR encoding", fixture.screen.mouseSgr)

        fixture.parser.write("\u001b[?2004l\u001b[?1000l\u001b[?1006l\u001b[?25h\u001b[?1049l\u001b[0m")

        assertFalse("the primary screen is in use again", fixture.screen.isAltScreen)
        assertTrue("the cursor is shown again", fixture.screen.cursorVisible)
        assertEquals("the primary screen comes back", "MAIN    ", fixture.screen.rowText(0))
        assertFalse("mouse reporting is off again", fixture.screen.mouseReporting)
        assertFalse("the mouse encoding is off again", fixture.screen.mouseSgr)
        assertEquals("the pen is reset again", TerminalPen.DEFAULT, fixture.screen.pen)
    }

    @Test
    fun `every byte value from 0x00 to 0xff is survivable`() {
        val seed = 0x5eedL

        // Each byte on its own, in a seeded order, in a screen of its own.
        val order = (0..255).toMutableList().apply { shuffle(java.util.Random(seed)) }
        for (value in order) {
            val fixture = Fixture(cols = 8, rows = 3)
            fixture.parser.write(byteArrayOf(value.toByte()))
            assertSurvives(fixture.screen)
        }

        // And every byte in one parser, twice, so the sequences they form between them
        // are survived as well.
        val together = Fixture(cols = 8, rows = 3)
        for (pass in 1..2) {
            for (value in order) {
                together.parser.write(byteArrayOf(value.toByte()))
            }
            assertEquals("pass $pass changed the screen's size", 8, together.screen.cols)
            assertSurvives(together.screen)
        }
    }

    @Test
    fun `a megabyte of random bytes never panics`() {
        val fixture = Fixture(cols = 80, rows = 24)
        val random = java.util.Random(0xfeedL)
        val chunk = ByteArray(4096)

        repeat(256) { // 256 KiB... 1 MiB in total
            random.nextBytes(chunk)
            fixture.parser.write(chunk)
        }

        assertSurvives(fixture.screen)
        assertEquals("the screen kept its size", 80, fixture.screen.cols)
        assertEquals(24, fixture.screen.rows)

        // A hostile stream can also open a string that never ends. The parser holds at
        // most a bounded amount of it, so the terminal recovers and is usable again
        // rather than accumulating a megabyte of a title it will never be given. This
        // one starts from a fresh parser, so what it is in the middle of is not left to
        // the random bytes above.
        val hostile = Fixture(cols = 80, rows = 24)
        val unterminated = StringBuilder("\u001b]52;c;")
        repeat(1 shl 20) { unterminated.append('A') }
        hostile.parser.write(unterminated.toString())
        hostile.parser.write("\u0007\u001b[2J")

        assertSurvives(hostile.screen)
        assertEquals("the screen was erased after the hostile string", " ".repeat(80), hostile.screen.rowText(0))
        assertEquals("the hostile string was not put on the clipboard", emptyList<String>(), hostile.clipboard)
        hostile.parser.write("ok")
        assertEquals("ok" + " ".repeat(78), hostile.screen.rowText(0))
    }

    /** SgrCase is one SGR sequence and the pen it must leave behind. */
    private data class SgrCase(val name: String, val sequence: String, val pen: TerminalPen)

    /** CursorCase is one addressing sequence and the 0-based cell it must land on. */
    private data class CursorCase(val name: String, val sequence: String, val x: Int, val y: Int)
}
