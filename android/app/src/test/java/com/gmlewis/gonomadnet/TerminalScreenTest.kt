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
 * The emulated terminal's grid, cursor and scroll region.
 *
 * `tui/vterm.go` holds the same screen in Go, and every vector here is readable against
 * its Go original: the deferred autowrap in `putRune`, the row shuffles in `scrollUp`
 * and `scrollDown`, the three erase modes, the alternate screen's save and restore, and
 * the upper-left overlap `Resize` keeps. A disagreement between the two emulators does
 * not surface as an error — it is a screen that draws the wrong characters — so the
 * grid is asserted as text rather than sampled cell by cell.
 */
class TerminalScreenTest {

    /** type prints text the way a shell's output arrives: runes and nothing else. */
    private fun TerminalScreen.type(text: String) {
        for (ch in text) {
            putRune(ch)
        }
    }

    /** lines renders the whole grid, so a test can state the screen it expects. */
    private fun TerminalScreen.lines(): List<String> = (0 until rows).map { rowText(it) }

    /** filledScreen is a 3x3 screen holding one distinct letter per row. */
    private fun filledScreen(): TerminalScreen {
        val screen = TerminalScreen(cols = 3, rows = 3)
        listOf("abc", "def", "ghi").forEachIndexed { y, text ->
            screen.moveCursor(0, y)
            screen.type(text)
        }
        return screen
    }

    @Test
    fun `a new screen is blank and the cursor is at the origin`() {
        val screen = TerminalScreen(cols = 5, rows = 3)

        assertEquals("     \n     \n     ", screen.text())
        assertEquals(0, screen.cursorX)
        assertEquals(0, screen.cursorY)
        assertTrue("a fresh screen shows its cursor", screen.cursorVisible)
        assertFalse("a fresh screen is the primary one", screen.isAltScreen)
        assertEquals(
            "a fresh screen scrolls in all of itself",
            listOf(0, 2),
            listOf(screen.scrollTop, screen.scrollBottom),
        )
    }

    @Test
    fun `printing advances the cursor and writes the rune`() {
        val screen = TerminalScreen(cols = 5, rows = 3)

        screen.type("Hi")

        assertEquals("Hi   ", screen.rowText(0))
        assertEquals("     ", screen.rowText(1))
        assertEquals(2, screen.cursorX)
        assertEquals(0, screen.cursorY)
    }

    @Test
    fun `printing past the last column wraps to the next row`() {
        val screen = TerminalScreen(cols = 3, rows = 3)

        screen.type("abc")

        // The wrap is deferred, exactly as in vterm.go's putRune: after the last
        // column the cursor rests on it until the next rune arrives, so the rest of
        // the line can still be overtyped.
        assertEquals("abc", screen.rowText(0))
        assertEquals("   ", screen.rowText(1))
        assertEquals(2, screen.cursorX)
        assertEquals(0, screen.cursorY)

        screen.type("d")

        assertEquals("abc", screen.rowText(0))
        assertEquals("d  ", screen.rowText(1))
        assertEquals(1, screen.cursorX)
        assertEquals(1, screen.cursorY)

        // A cursor move cancels the pending wrap, as every CSI does in vterm.go.
        val moved = TerminalScreen(cols = 3, rows = 2)
        moved.type("abc")
        moved.moveCursor(0, 0)
        moved.type("Z")
        assertEquals("Zbc", moved.rowText(0))
        assertEquals("   ", moved.rowText(1))
    }

    @Test
    fun `the cursor is clamped to the screen`() {
        val screen = TerminalScreen(cols = 5, rows = 3)

        screen.moveCursor(-3, -3)
        assertEquals(0, screen.cursorX)
        assertEquals(0, screen.cursorY)

        // Cursor addressing is clamped the way vterm.go's clampCursor clamps it:
        // a coordinate past the edge becomes the edge, never an out-of-range write.
        screen.moveCursor(99, 99)
        assertEquals(4, screen.cursorX)
        assertEquals(2, screen.cursorY)

        screen.resize(2, 1)
        assertEquals("shrinking clamps the cursor into the new screen", 1, screen.cursorX)
        assertEquals(0, screen.cursorY)

        val clamped = TerminalScreen(cols = 2, rows = 1)
        clamped.moveCursor(99, 0)
        clamped.type("X")
        assertEquals(" X", clamped.rowText(0))
    }

    @Test
    fun `erasing the display erases the whole screen`() {
        val cases = listOf(
            EraseCase("mode 0 erases from the cursor down", 0, listOf("abc", "d  ", "   ")),
            EraseCase("mode 1 erases up to the cursor", 1, listOf("   ", "  f", "ghi")),
            EraseCase("mode 2 erases the whole screen", 2, listOf("   ", "   ", "   ")),
            EraseCase("mode 3 erases the whole screen", 3, listOf("   ", "   ", "   ")),
        )

        for (case in cases) {
            val screen = filledScreen()
            screen.moveCursor(1, 1)

            screen.eraseDisplay(case.mode)

            assertEquals("${case.name}: the display", case.want, screen.lines())
            assertEquals("${case.name}: erasing does not move the cursor", 1, screen.cursorX)
            assertEquals("${case.name}: erasing does not move the cursor", 1, screen.cursorY)
        }
    }

    @Test
    fun `erasing the line erases only to the end when asked`() {
        val cases = listOf(
            EraseCase("mode 0 erases to the end of the line", 0, listOf("d  ")),
            EraseCase("mode 1 erases to the start of the line", 1, listOf("  f")),
            EraseCase("mode 2 erases the whole line", 2, listOf("   ")),
        )

        for (case in cases) {
            val screen = filledScreen()
            screen.moveCursor(1, 1)

            screen.eraseLine(case.mode)

            assertEquals("${case.name}: the cursor's line", case.want[0], screen.rowText(1))
            assertEquals("${case.name}: the line above is untouched", "abc", screen.rowText(0))
            assertEquals("${case.name}: the line below is untouched", "ghi", screen.rowText(2))
            assertEquals("${case.name}: erasing does not move the cursor", 1, screen.cursorX)
        }
    }

    @Test
    fun `a scroll region confines scrolling to its rows`() {
        val screen = TerminalScreen(cols = 3, rows = 4)
        listOf("aaa", "bbb", "ccc", "ddd").forEachIndexed { y, text ->
            screen.moveCursor(0, y)
            screen.type(text)
        }
        assertEquals(listOf("aaa", "bbb", "ccc", "ddd"), screen.lines())

        screen.setScrollRegion(1, 2)

        assertEquals("the region's top row", 1, screen.scrollTop)
        assertEquals("the region's bottom row", 2, screen.scrollBottom)
        // The region is set the way vterm.go sets it from CSI r, cursor homing and all.
        assertEquals(0, screen.cursorX)
        assertEquals(0, screen.cursorY)

        screen.scrollUp(1)

        assertEquals(
            "scrolling up moves the region's rows, blanks its bottom, and moves nothing else",
            listOf("aaa", "ccc", "   ", "ddd"),
            screen.lines(),
        )

        screen.scrollDown(1)

        assertEquals(
            "scrolling down moves them back and blanks the region's top",
            listOf("aaa", "   ", "ccc", "ddd"),
            screen.lines(),
        )

        // A line feed on the region's bottom row scrolls the region, not the screen.
        screen.moveCursor(0, 2)
        screen.lineFeed()

        assertEquals(
            "a line feed at the region's bottom scrolls only the region",
            listOf("aaa", "ccc", "   ", "ddd"),
            screen.lines(),
        )
    }

    @Test
    fun `a line that scrolls off the top is kept in the history`() {
        // On a desktop the terminal emulator owns the scrollback and the client's screen is
        // only what fits in the window. In the appliance this emulator *is* the terminal, so
        // the lines that leave the screen have to be kept, or a reader who drags the console
        // upwards finds nothing above the top row.
        val screen = TerminalScreen(cols = 3, rows = 2)
        assertEquals("a fresh screen has nothing behind it", 0, screen.historySize)

        listOf("abc", "def").forEachIndexed { y, text ->
            screen.moveCursor(0, y)
            screen.type(text)
        }
        screen.moveCursor(0, 1)
        screen.lineFeed()

        assertEquals("the screen scrolled", listOf("def", "   "), screen.lines())
        assertEquals("the line that left the screen was not kept", 1, screen.historySize)
        assertEquals("the line that left is not the one that was there", "abc", screen.historyRowText(0))

        screen.moveCursor(0, 1)
        screen.type("ghi")
        screen.lineFeed()

        assertEquals("the screen scrolled again", listOf("ghi", "   "), screen.lines())
        assertEquals(
            "the history holds the lines in the order they left",
            listOf("abc", "def"),
            (0 until screen.historySize).map { screen.historyRowText(it) },
        )
    }

    @Test
    fun `a scroll inside a region is not the console's history`() {
        // A region is the client drawing in part of the screen — a dialog, a scrolling list.
        // Those lines did not leave the console, they were overwritten, and a reader looking
        // back should not find the client's half-drawn frames there.
        val screen = TerminalScreen(cols = 3, rows = 3)
        listOf("aaa", "bbb", "ccc").forEachIndexed { y, text ->
            screen.moveCursor(0, y)
            screen.type(text)
        }
        screen.setScrollRegion(1, 2)
        screen.moveCursor(0, 2)
        screen.lineFeed()

        assertEquals("the region scrolled", listOf("aaa", "ccc", "   "), screen.lines())
        assertEquals("a region's scroll was kept as history", 0, screen.historySize)
    }

    @Test
    fun `the alternate screen does not fill the history`() {
        // The alternate screen is where a full-screen client draws itself: its frames are
        // not the console's output, and keeping them would bury the conversation under a
        // repaint for every key press.
        val screen = TerminalScreen(cols = 4, rows = 2)
        screen.type("MAIN")
        screen.setAltScreen(true, saveCursor = true)
        screen.moveCursor(0, 1)
        screen.type("alt")
        screen.lineFeed()

        assertEquals("the alternate screen wrote history", 0, screen.historySize)

        screen.setAltScreen(false, saveCursor = true)

        assertEquals("leaving the alternate screen restores the main one", "MAIN", screen.rowText(0))
        assertEquals("the restored screen brought history with it", 0, screen.historySize)
    }

    @Test
    fun `the history is bounded`() {
        // A tablet runs for weeks. An unbounded history is a leak that ends with the OOM
        // killer, so the oldest lines go as the newest arrive.
        val screen = TerminalScreen(cols = 4, rows = 1)
        val lines = TerminalScreen.MAX_SCROLLBACK_LINES + 100

        for (i in 0 until lines) {
            screen.moveCursor(0, 0)
            screen.type("%03d ".format(i))
            screen.lineFeed()
        }

        assertEquals("the history grew past its bound", TerminalScreen.MAX_SCROLLBACK_LINES, screen.historySize)
        assertEquals("the oldest lines were not the ones dropped", "100 ", screen.historyRowText(0))
        assertEquals(
            "the newest line is not the last one written",
            "%03d ".format(lines - 1),
            screen.historyRowText(screen.historySize - 1),
        )
    }

    @Test
    fun `the count of lines that have scrolled off does not saturate`() {
        // The scrollback is bounded, so the number of lines it holds stops growing. The
        // number that have gone off the top does not: the view re-anchors a reader parked in
        // the scrollback by how much output arrived, and a count that stopped moving once
        // the buffer filled would let the text drift under their eyes for the rest of the
        // session — a reader losing their place with no sign that anything went wrong.
        val screen = TerminalScreen(cols = 4, rows = 1)
        assertEquals("a fresh screen has scrolled nothing off", 0L, screen.linesScrolledOff)

        val lines = TerminalScreen.MAX_SCROLLBACK_LINES + 25
        for (i in 0 until lines) {
            screen.moveCursor(0, 0)
            screen.type("%03d ".format(i))
            screen.lineFeed()
        }

        assertEquals("the history is bounded", TerminalScreen.MAX_SCROLLBACK_LINES, screen.historySize)
        assertEquals("the count of what left the screen is not", lines.toLong(), screen.linesScrolledOff)
    }

    @Test
    fun `a scroll that is not the console's does not count as output`() {
        // Only a line leaving the whole screen is console output. A region scroll and a
        // full-screen client's repaint are the client drawing over itself, and counting
        // them would drag a reader's window upwards on every frame the client drew.
        val screen = TerminalScreen(cols = 3, rows = 3)
        screen.setScrollRegion(1, 2)
        screen.moveCursor(0, 2)
        screen.lineFeed()
        assertEquals("a region's scroll counted as output", 0L, screen.linesScrolledOff)

        screen.setScrollRegion(1, 3)
        screen.setAltScreen(on = true, saveCursor = true)
        screen.moveCursor(0, 2)
        screen.lineFeed()
        assertEquals("the alternate screen's scroll counted as output", 0L, screen.linesScrolledOff)
    }

    @Test
    fun `the alternate screen is restored with its own contents`() {
        val screen = TerminalScreen(cols = 4, rows = 2)
        screen.type("MAIN")
        screen.moveCursor(1, 1)

        screen.setAltScreen(on = true, saveCursor = true)

        assertTrue(screen.isAltScreen)
        assertEquals("the alternate screen starts blank", listOf("    ", "    "), screen.lines())
        assertEquals("the alternate screen starts at the origin", 0, screen.cursorX)
        assertEquals("the alternate screen starts at the origin", 0, screen.cursorY)

        screen.type("ALT")
        screen.moveCursor(2, 1)
        // Entering the alternate screen twice is not a second save: the primary
        // screen saved by the first switch is the one that comes back.
        screen.setAltScreen(on = true, saveCursor = true)

        assertEquals("the alternate screen keeps its own contents", listOf("ALT ", "    "), screen.lines())
        assertEquals("a repeated switch does not move the cursor", 2, screen.cursorX)
        assertEquals("a repeated switch does not move the cursor", 1, screen.cursorY)

        screen.setAltScreen(on = false, saveCursor = true)

        assertFalse(screen.isAltScreen)
        assertEquals(
            "the primary screen comes back with its own contents",
            listOf("MAIN", "    "),
            screen.lines(),
        )
        assertEquals("the saved cursor comes back with it", 1, screen.cursorX)
        assertEquals("the saved cursor comes back with it", 1, screen.cursorY)

        // A legacy switch (?47h, ?1047h) restores the grid but leaves the cursor
        // where the alternate screen left it, as vterm.go's saveCursor flag says.
        val legacy = TerminalScreen(cols = 4, rows = 2)
        legacy.type("MAIN")
        legacy.setAltScreen(on = true, saveCursor = false)
        legacy.moveCursor(3, 1)
        legacy.setAltScreen(on = false, saveCursor = false)

        assertEquals(
            "the primary screen comes back under a legacy switch too",
            listOf("MAIN", "    "),
            legacy.lines(),
        )
        assertEquals("a legacy switch does not restore the cursor", 3, legacy.cursorX)
        assertEquals("a legacy switch does not restore the cursor", 1, legacy.cursorY)
    }

    @Test
    fun `resizing keeps the top-left contents`() {
        val screen = TerminalScreen(cols = 3, rows = 4)
        screen.setScrollRegion(1, 2)
        screen.type("abc")
        screen.moveCursor(0, 1)
        screen.type("de")

        screen.resize(5, 6)

        assertEquals(5, screen.cols)
        assertEquals(6, screen.rows)
        assertEquals(
            "growing keeps the upper-left cells and blanks the new ones",
            listOf("abc  ", "de   ", "     ", "     ", "     ", "     "),
            screen.lines(),
        )
        assertEquals("the cursor stays where it was inside the screen", 2, screen.cursorX)
        assertEquals("the cursor stays where it was inside the screen", 1, screen.cursorY)
        assertEquals(
            "a resize restores the whole screen as the scroll region",
            listOf(0, 5),
            listOf(screen.scrollTop, screen.scrollBottom),
        )

        screen.resize(2, 2)

        assertEquals(
            "shrinking keeps the overlap and drops what no longer fits",
            listOf("ab", "de"),
            screen.lines(),
        )
        assertEquals("shrinking clamps the cursor into the new screen", 1, screen.cursorX)
        assertEquals("shrinking clamps the cursor into the new screen", 1, screen.cursorY)
    }

    /** EraseCase is one erase mode and the screen it must leave behind. */
    private data class EraseCase(val name: String, val mode: Int, val want: List<String>)
}
