// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * The view's arithmetic, which is the whole of it that can be tested on a JVM.
 *
 * `TerminalView` draws with a `Canvas`, so no part of it that touches Android can run
 * here. What is left is the part that is easy to get wrong and impossible to see: how many
 * columns and rows a rectangle of pixels holds, where a cell lands on it, which line the
 * top of the window shows once the reader has dragged it, and what the on-screen key row
 * offers. Those are [TerminalMetrics], [TerminalScroll] and [TerminalKeyRow], and the view
 * is the thin drawing half on top of them.
 */
class TerminalViewTest {

    @Test
    fun `the grid size from a pixel width and height is what fits`() {
        val metrics = TerminalMetrics(cellWidth = 8, cellHeight = 16)
        val cases = listOf(
            GridCase("the whole fit", 800, 480, 100, 30),
            // A partial cell is not a column: the client's own drawing would be cut off, and
            // the terminal it is told about has to be one it can fill.
            GridCase("a partial cell is not a column", 807, 487, 100, 30),
            GridCase("one pixel short of another column", 799, 479, 99, 29),
            GridCase("exactly one cell", 8, 16, 1, 1),
            // A view narrower than one character still has to be a screen: zero columns is
            // not a terminal, and the client would be laid out into nothing.
            GridCase("narrower than one cell", 5, 5, 1, 1),
            GridCase("a tablet in portrait", 1200, 2000, 150, 125),
        )

        for (case in cases) {
            val grid = metrics.gridFor(widthPx = case.width, heightPx = case.height)
            assertEquals("${case.name}: the columns", case.cols, grid.cols)
            assertEquals("${case.name}: the rows", case.rows, grid.rows)
        }
    }

    @Test
    fun `the cell to pixel mapping agrees with the fit`() {
        val metrics = TerminalMetrics(cellWidth = 8, cellHeight = 16)
        val grid = metrics.gridFor(widthPx = 800, heightPx = 480)

        assertEquals("the first column starts at the edge", 0, metrics.pixelX(0))
        assertEquals("the first row starts at the top", 0, metrics.pixelY(0))
        assertEquals(3 * 8, metrics.pixelX(3))
        assertEquals(7 * 16, metrics.pixelY(7))

        // The two halves have to agree: every cell the fit promised is inside the view, and
        // the next one is not — otherwise the last column is drawn half off the screen.
        for (col in 0 until grid.cols) {
            assertTrue("column $col starts past the edge", metrics.pixelX(col) + metrics.cellWidth <= 800)
        }
        for (row in 0 until grid.rows) {
            assertTrue("row $row starts past the bottom", metrics.pixelY(row) + metrics.cellHeight <= 480)
        }
        assertTrue("a column more would have fitted", metrics.pixelX(grid.cols) + metrics.cellWidth > 800)
        assertTrue("a row more would have fitted", metrics.pixelY(grid.rows) + metrics.cellHeight > 480)

        // A grid measured from a rectangle the caller measured is the same grid.
        assertEquals(grid, TerminalGrid(cols = 100, rows = 30))
    }

    @Test
    fun `the first visible row follows the scroll offset`() {
        // A hundred lines with twenty of them on screen: at the bottom the window shows the
        // last twenty, and every line the reader drags up moves the top of the window up.
        val cases = listOf(
            ScrollCase("at the bottom", totalLines = 100, visibleRows = 20, offset = 0, first = 80),
            ScrollCase("one line up", 100, 20, 1, 79),
            ScrollCase("a screen up", 100, 20, 20, 60),
            ScrollCase("all the way up", 100, 20, 80, 0),
            // Past the oldest line there is nothing to show, so it stops there rather than
            // counting rows that do not exist.
            ScrollCase("past the oldest line", 100, 20, 500, 0),
            ScrollCase("nothing scrolled off", 20, 20, 0, 0),
            ScrollCase("fewer lines than the screen holds", 10, 20, 0, 0),
            ScrollCase("fewer lines, scrolled anyway", 10, 20, 4, 0),
        )

        for (case in cases) {
            assertEquals(
                "${case.name}: the first visible line",
                case.first,
                TerminalScroll.firstVisibleLine(case.totalLines, case.visibleRows, case.offset),
            )
        }

        // How far there is to drag: exactly the lines that have gone off the top, and not a
        // row more when the whole console already fits.
        assertEquals(80, TerminalScroll.maxOffset(totalLines = 100, visibleRows = 20))
        assertEquals(0, TerminalScroll.maxOffset(totalLines = 20, visibleRows = 20))
        assertEquals(0, TerminalScroll.maxOffset(totalLines = 10, visibleRows = 20))
    }

    @Test
    fun `a scrolled-to-bottom view follows new output`() {
        // At the bottom the window is anchored to the bottom of the console, so output
        // arriving below pushes the older lines up and the reader keeps seeing the newest.
        assertEquals(0, TerminalScroll.offsetAfterLines(offset = 0, addedLines = 1, maxOffset = 81))
        assertEquals(0, TerminalScroll.offsetAfterLines(offset = 0, addedLines = 25, maxOffset = 105))
        assertEquals(0, TerminalScroll.offsetAfterLines(offset = 0, addedLines = 0, maxOffset = 80))
    }

    @Test
    fun `a scrolled-up view stays where the reader put it`() {
        // The reader is up in the scrollback reading something. Lines arriving at the bottom
        // must not move the text under their eyes, so the window keeps the same lines: the
        // offset grows by exactly what arrived.
        assertEquals(15, TerminalScroll.offsetAfterLines(offset = 10, addedLines = 5, maxOffset = 85))
        assertEquals(10, TerminalScroll.offsetAfterLines(offset = 10, addedLines = 0, maxOffset = 80))
        assertEquals(110, TerminalScroll.offsetAfterLines(offset = 10, addedLines = 100, maxOffset = 200))

        // ...but only as far as the oldest line there is. A burst that would push the window
        // past it stops at the top instead.
        assertEquals(80, TerminalScroll.offsetAfterLines(offset = 10, addedLines = 100, maxOffset = 80))

        // And the line the window shows is the same one before and after the burst.
        val before = TerminalScroll.firstVisibleLine(totalLines = 100, visibleRows = 20, offset = 10)
        val after = TerminalScroll.firstVisibleLine(totalLines = 150, visibleRows = 20, offset = 60)
        assertEquals("the burst moved the text under the reader", before, after)
    }

    @Test
    fun `the key strip offers the keys a tablet has no way to press`() {
        // The keys a soft keyboard has no button for. Without them the client cannot be
        // driven at all: escape backs out of a dialog, home and end and the page keys walk a
        // document, and the arrows are how a menu is walked. They are the Termux extra-keys
        // row's own keys, so a person who has driven this client under Termux finds the same
        // buttons here.
        val rows = TerminalKeyRow.rows(collapsed = false)
        assertEquals("the strip is not two rows", 2, rows.size)
        assertEquals("the rows are not the same width", rows[0].size, rows[1].size)

        val top = rows[0].filterIsInstance<KeyRowEntry.Press>()
        assertEquals(
            "the strip's top row is not escape, the path characters and navigation",
            listOf(
                TerminalKey.Special(SpecialKey.ESCAPE),
                TerminalKey.Rune("/"),
                TerminalKey.Rune("-"),
                TerminalKey.Special(SpecialKey.HOME),
                TerminalKey.Special(SpecialKey.UP),
                TerminalKey.Special(SpecialKey.END),
                TerminalKey.Special(SpecialKey.PAGE_UP),
            ),
            top.map { it.key },
        )
        val bottom = rows[1].filterIsInstance<KeyRowEntry.Press>()
        assertEquals(
            "the strip's bottom row is not the navigation it should be",
            listOf(
                TerminalKey.Special(SpecialKey.LEFT),
                TerminalKey.Special(SpecialKey.DOWN),
                TerminalKey.Special(SpecialKey.RIGHT),
                TerminalKey.Special(SpecialKey.PAGE_DOWN),
            ),
            bottom.map { it.key },
        )

        // Control and Alt are latches rather than keys: each is held, and the next character
        // tapped is sent with it. That is how ctrl-c reaches the client at all, and how alt
        // combinations reach it from a touchscreen.
        assertEquals(1, rows[1].filterIsInstance<KeyRowEntry.Control>().size)
        assertEquals("Ctrl", rows[1].filterIsInstance<KeyRowEntry.Control>().single().label)
        assertEquals(1, rows[1].filterIsInstance<KeyRowEntry.Alt>().size)
        assertEquals("Alt", rows[1].filterIsInstance<KeyRowEntry.Alt>().single().label)

        // Every button on the strip sends something, and says what it is: a button that
        // sends nothing is a dead patch of screen.
        for (entry in rows.flatten()) {
            assertTrue("a button has no label", entry.label.isNotEmpty())
            when (entry) {
                is KeyRowEntry.Press -> assertNotNull("${entry.label} sends nothing", TerminalKeys.toBytes(entry.key))
                is KeyRowEntry.Control, is KeyRowEntry.Alt, is KeyRowEntry.Toggle -> Unit
            }
        }
    }

    @Test
    fun `the strip folds away to the button that brings it back`() {
        // The strip costs rows, and the rows are the client's. A reader who wants them back
        // folds it away — and the folded strip is the toggle itself, because a strip that
        // could be folded away and not brought back would be a keyboard with no escape key
        // and no way to ask for one.
        val folded = TerminalKeyRow.rows(collapsed = true)
        assertEquals("a folded strip is not one row", 1, folded.size)
        assertEquals("a folded strip is not one button", 1, folded[0].size)
        val toggle = folded[0].single()
        assertTrue("the folded strip is not the toggle", toggle is KeyRowEntry.Toggle)
        assertEquals("the toggle does not point at unfolding", "▴", toggle.label)
        assertEquals(
            "the unfolded toggle does not point at folding",
            "▾",
            TerminalKeyRow.rows(collapsed = false)[1].first { it is KeyRowEntry.Toggle }.label,
        )

        // Two rows of keys cost two rows of the terminal; a folded one costs one. The strip
        // is drawn over the client, so those rows come out of what the client is told it has.
        val cellHeight = 16
        val padding = 2 * TerminalKeyRow.padding(cellHeight)
        assertEquals(
            "an unfolded strip does not cost its two rows",
            2 * cellHeight + padding,
            TerminalKeyRow.height(TerminalKeyRow.rows(collapsed = false), cellHeight),
        )
        assertEquals(
            "a folded strip does not give a row back",
            cellHeight + padding,
            TerminalKeyRow.height(folded, cellHeight),
        )
    }

    @Test
    fun `a strip that is not shown takes no rows at all`() {
        // The strip comes and goes with the soft keyboard, and the rows it is drawn in are
        // the client's rows: a console with no keyboard is a client with the whole screen —
        // and no rule drawn across its last row.
        val cellHeight = 16
        assertEquals(
            "a strip that is not shown still takes room",
            0,
            TerminalKeyRow.height(rows = emptyList(), cellHeight = cellHeight),
        )
        assertEquals(
            "a shown strip takes its rows and its padding",
            2 * cellHeight + 2 * TerminalKeyRow.padding(cellHeight),
            TerminalKeyRow.height(TerminalKeyRow.rows(collapsed = false), cellHeight),
        )
    }

    @Test
    fun `the strip's buttons are the buttons a tap lands on`() {
        // The strip is drawn as equal shares of each row's width, so a tap is a division: the
        // point decides which button it landed on, and the ends of a row are not one button
        // short. The row is part of the division too, or every tap would be a tap on the top
        // row's buttons.
        val rows = TerminalKeyRow.rows(collapsed = false)
        val width = rows[0].size * 100
        val rowHeight = 20
        val stripTop = 800

        assertEquals("the left edge", rows[0][0], TerminalKeyRow.entryAt(x = 0, y = stripTop, width, stripTop, rowHeight, rows))
        assertEquals(
            "just inside the first",
            rows[0][0],
            TerminalKeyRow.entryAt(x = 99, y = stripTop, width, stripTop, rowHeight, rows),
        )
        assertEquals(
            "just inside the second",
            rows[0][1],
            TerminalKeyRow.entryAt(x = 100, y = stripTop, width, stripTop, rowHeight, rows),
        )
        assertEquals(
            "a tap in the middle of the top row",
            rows[0][3],
            TerminalKeyRow.entryAt(x = 350, y = stripTop + 5, width, stripTop, rowHeight, rows),
        )
        assertEquals(
            "the last button of the top row must be reachable",
            rows[0].last(),
            TerminalKeyRow.entryAt(x = width - 1, y = stripTop, width, stripTop, rowHeight, rows),
        )
        assertEquals(
            "the same place one row down is the row below's button",
            rows[1][3],
            TerminalKeyRow.entryAt(x = 350, y = stripTop + rowHeight, width, stripTop, rowHeight, rows),
        )
        assertEquals(
            "the last row's last button must be reachable",
            rows[1].last(),
            TerminalKeyRow.entryAt(x = width, y = stripTop + 2 * rowHeight, width, stripTop, rowHeight, rows),
        )
        // A view that has not been laid out yet is not a division by zero.
        assertEquals(
            rows[0][0],
            TerminalKeyRow.entryAt(x = 0, y = 0, width = 0, stripTop = 0, rowHeight = 0, rows),
        )

        // With control held, a letter is its control code — and a word an input method
        // committed is not a letter, and neither is a key that has no code.
        assertEquals(TerminalKey.Ctrl('c'), TerminalKeyRow.withControl(armed = true, key = TerminalKey.Rune("c")))
        assertEquals(TerminalKey.Rune("hi"), TerminalKeyRow.withControl(armed = true, key = TerminalKey.Rune("hi")))
        assertEquals(
            TerminalKey.Special(SpecialKey.UP),
            TerminalKeyRow.withControl(armed = true, key = TerminalKey.Special(SpecialKey.UP)),
        )
        assertEquals(TerminalKey.Rune("c"), TerminalKeyRow.withControl(armed = false, key = TerminalKey.Rune("c")))
    }

    @Test
    fun `the console draws with the font the APK carries`() {
        // The client's interface is not plain text. Its menu bar, its list markers and its
        // arrows are Nerd Font glyphs, and the device's own monospace font has no code point
        // for any of them, so a console that fell back to it would draw a working interface
        // out of empty boxes. The font in the APK's assets is therefore not decoration: it is
        // half of what the client draws with.
        val font = File(ASSETS, TerminalView.CONSOLE_FONT)
        assertTrue("the font the console draws with is not in the APK's assets: $font", font.isFile)
        assertTrue("the font is empty: $font", font.length() > 0L)

        // An OpenType font with CFF outlines starts "OTTO" and one with TrueType outlines
        // starts with its version; anything else is not a font, and a file Android cannot
        // parse is a console that draws nothing at all.
        val magic = font.inputStream().use { stream ->
            stream.readNBytes(4).joinToString("") { byte -> "%02x".format(byte) }
        }
        assertTrue("the font is not an OpenType font: $magic", magic in FONT_MAGIC)

        // The licence travels with the font. Both are the asset directory's business, and a
        // font redistributed without its licence is a distribution the licence forbids.
        assertTrue("the font's licence is not with it", File(ASSETS, FONT_LICENCE).isFile)
    }

    @Test
    fun `a modifier latch is spent by the next key wherever it is typed`() {
        // A tablet cannot hold Control or Alt down and press a letter, so each is a switch:
        // armed by a tap, spent by whatever comes next. "Whatever comes next" is the care
        // here — the latch belongs to the console rather than to the strip, because the
        // letter it is spent on is typed on the soft keyboard, or on a keyboard that is not
        // this app's at all. A latch that only the strip consulted would send a plain q where
        // the person meant ^Q, which is the difference between quitting and typing.
        val latch = ModifierLatch()
        assertFalse("the latch starts armed", latch.ctrl)
        assertFalse("the latch starts armed", latch.alt)

        assertEquals(TerminalKey.Rune("q"), latch.spend(TerminalKey.Rune("q")))
        assertFalse("an unarmed latch changed a key", latch.ctrl)

        latch.toggleCtrl()
        assertTrue("a tap on Control must arm the latch", latch.ctrl)
        assertEquals(
            "a character typed while Control is armed is sent with control",
            TerminalKey.Ctrl('q'),
            latch.spend(TerminalKey.Rune("q")),
        )
        assertFalse("the latch was not spent by the character", latch.ctrl)
        assertEquals("the latch was spent twice", TerminalKey.Rune("q"), latch.spend(TerminalKey.Rune("q")))

        // An arrow has no control code, so it is sent as itself and the latch is kept: a
        // keyboard with no control key needs it to still be there afterwards. The same goes
        // for a word committed by an input method, which is not a character to a terminal.
        latch.toggleCtrl()
        assertEquals(TerminalKey.Special(SpecialKey.LEFT), latch.spend(TerminalKey.Special(SpecialKey.LEFT)))
        assertTrue("an arrow spent the latch", latch.ctrl)
        assertEquals(TerminalKey.Rune("hello"), latch.spend(TerminalKey.Rune("hello")))
        assertTrue("a committed word spent the latch", latch.ctrl)
        assertEquals("the next real character is the one Control was armed for", TerminalKey.Ctrl('x'), latch.spend(TerminalKey.Rune("x")))
        assertFalse(latch.ctrl)

        // Alt is a prefix rather than a code, so it applies to whatever the key sends — an
        // arrow as much as a letter — and anything spends it.
        latch.toggleAlt()
        assertTrue("a tap on Alt must arm the latch", latch.alt)
        assertEquals(
            "a character typed while Alt is armed is sent as an alt combination",
            TerminalKey.Alt(TerminalKey.Rune("x")),
            latch.spend(TerminalKey.Rune("x")),
        )
        assertFalse("the latch was not spent by the character", latch.alt)
        latch.toggleAlt()
        assertEquals(
            "alt applies to a key that has no character of its own",
            TerminalKey.Alt(TerminalKey.Special(SpecialKey.LEFT)),
            latch.spend(TerminalKey.Special(SpecialKey.LEFT)),
        )
        assertFalse(latch.alt)

        // Both at once is one key press with both modifiers on it, and both are spent.
        latch.toggleCtrl()
        latch.toggleAlt()
        assertEquals(
            "control and alt together are not both applied",
            TerminalKey.Alt(TerminalKey.Ctrl('c')),
            latch.spend(TerminalKey.Rune("c")),
        )
        assertFalse("Control survived the character", latch.ctrl)
        assertFalse("Alt survived the character", latch.alt)

        // Two taps are off again, so a person who armed a modifier by mistake can disarm it.
        latch.toggleCtrl()
        latch.toggleCtrl()
        assertFalse("two taps must leave the latch disarmed", latch.ctrl)
    }

    /** GridCase is a rectangle of pixels and the grid that fits in it. */
    private data class GridCase(
        val name: String,
        val width: Int,
        val height: Int,
        val cols: Int,
        val rows: Int,
    )
    /** ScrollCase is a scroll position and the line the window's top row shows. */
    private data class ScrollCase(
        val name: String,
        val totalLines: Int,
        val visibleRows: Int,
        val offset: Int,
        val first: Int,
    )

    private companion object {
        /** The asset directory, as Gradle runs these tests from the module directory. */
        val ASSETS = File("src/main/assets")

        /** The font's licence, which is distributed beside it. */
        const val FONT_LICENCE = "AtkinsonHyperlegibleMono-OFL.txt"

        /** The first four bytes of the two OpenType flavours, as lowercase hex. */
        val FONT_MAGIC = listOf("4f54544f", "00010000")
    }
}
