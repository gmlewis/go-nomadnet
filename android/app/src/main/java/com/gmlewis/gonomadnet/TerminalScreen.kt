// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

/**
 * The emulated terminal's screen: its grid, its cursor and its scroll region.
 *
 * The appliance runs the client on a pseudo-terminal, so what arrives over the console
 * socket is the byte stream a terminal emulator would receive, and the app is the
 * emulator. This class is the part of it that holds state — cells, cursor, scroll
 * region, alternate screen — while [TerminalParser] is the part that reads the escape
 * sequences and drives it.
 *
 * It is a port of the Go client's `tui/vterm.go`, deliberately function for function:
 * that file is what the same client draws through on a desktop terminal, and two
 * emulators that disagree are not an error anyone sees, they are a screen with the
 * wrong characters on it. The Go file's vectors are asserted here against the same
 * behaviour rather than retyped, and [TerminalScreenTest] keeps each one readable
 * beside its original.
 *
 * One thing here has no counterpart in `vterm.go`, and cannot have: a scrollback. On a
 * desktop the client draws through a terminal emulator that owns the lines which leave the
 * window, and the Go file is only ever the screen. In the appliance this emulator *is* the
 * terminal, so the lines that scroll off the top are kept here — see [historyRowText] — and
 * the view is what a reader drags back through them.
 */
class TerminalScreen(cols: Int, rows: Int) {

    /** The fixed facts of this screen. */
    companion object {
        /**
         * How many lines of scrollback are kept.
         *
         * A tablet runs for weeks, so this is a bound rather than a policy: the oldest lines
         * go as the newest arrive, and the memory a session can hold does not depend on how
         * long it has been running.
         */
        const val MAX_SCROLLBACK_LINES: Int = 500
    }

    /** The screen's width, in columns. */
    var cols: Int = cols
        private set

    /** The screen's height, in rows. */
    var rows: Int = rows
        private set

    /** The cursor's column, 0-based. */
    var cursorX: Int = 0
        private set

    /** The cursor's row, 0-based. */
    var cursorY: Int = 0
        private set

    /** Whether the child has asked for a visible cursor (CSI ?25h / ?25l). */
    var cursorVisible: Boolean = true
        private set

    /** Whether the child wants mouse events forwarded (CSI ?1000h / ?1002h / ?1003h). */
    var mouseReporting: Boolean = false
        private set

    /** Whether those events use the SGR encoding the modern clients ask for (CSI ?1006h). */
    var mouseSgr: Boolean = false
        private set

    /** The colour and attributes a character written now would be drawn with. */
    var pen: TerminalPen = TerminalPen.DEFAULT
        private set

    /** The first row the screen scrolls, 0-based. */
    var scrollTop: Int = 0
        private set

    /** The last row the screen scrolls, 0-based and inclusive. */
    var scrollBottom: Int = rows - 1
        private set

    /** Whether the alternate screen is the one being drawn on (CSI ?1049h / ?1049l). */
    var isAltScreen: Boolean = false
        private set

    private var grid: Array<Array<TerminalCell>> = blankGrid(cols, rows)

    // The lines that have left the top of the screen, oldest first. Each entry is a row
    // array the grid no longer holds — see recordHistory — so nothing writes to one again.
    private val history = ArrayDeque<Array<TerminalCell>>()

    /**
     * Whether the cursor rests on the last column waiting for the next rune to wrap.
     * A terminal wraps when the next character arrives, not when the last one is
     * written, which is what lets a program overtype the last column of a full line.
     */
    private var wrapPending = false

    private var savedGrid: Array<Array<TerminalCell>>? = null
    private var savedCursorX = 0
    private var savedCursorY = 0

    // The cursor the child saves for itself with DECSC/DECRC or CSI s / CSI u. It is
    // held apart from the alternate screen's own save, which the client's startup makes
    // for itself (CSI ?1049h): a child that borrows the cursor to draw with must not
    // disturb the cursor the screen switch will restore.
    private var decSavedX = 0
    private var decSavedY = 0
    private var decSavedPen = TerminalPen.DEFAULT

    /** rowText renders one row's characters, the wide glyphs' spare columns left out. */
    fun rowText(y: Int): String = grid[y].joinToString("") { it.glyph }

    /** text renders the whole screen, one row per line. */
    fun text(): String = (0 until rows).joinToString("\n") { rowText(it) }

    /** penAt reports the pen of one cell, which is what the renderer draws it with. */
    fun penAt(x: Int, y: Int): TerminalPen = grid[y][x].pen

    /** cellAt is one cell of the screen, glyph and pen together, for the renderer to draw. */
    fun cellAt(x: Int, y: Int): TerminalCell = grid[y][x]

    /** historySize is how many lines have left the top of the screen and been kept. */
    val historySize: Int
        get() = history.size

    /**
     * linesScrolledOff is how many lines have left the top of the screen over this screen's
     * whole life, whether or not [historySize] still holds them.
     *
     * It is what a view re-anchors a reader by. The history is bounded, so `historySize`
     * stops growing once the buffer is full and cannot say how much output has arrived
     * since; this keeps counting, and the difference between two readings is exactly the
     * number of lines that have appeared below a reader parked in the scrollback.
     *
     * Only the console's own lines count. A scroll inside a region and a full-screen
     * client's repaint are the client drawing over itself, and they add nothing to the
     * console for a reader to have been pushed down by.
     */
    var linesScrolledOff: Long = 0L
        private set

    /** historyRowText renders one kept line's characters, oldest first. */
    fun historyRowText(line: Int): String = history[line].joinToString("") { it.glyph }

    /** historyCell is one cell of one kept line, for the renderer to draw. */
    fun historyCell(x: Int, line: Int): TerminalCell = history[line][x]

    /** setPen changes the pen a character written now would be drawn with. */
    fun setPen(pen: TerminalPen) {
        this.pen = pen
    }

    /**
     * applySgr applies an SGR sequence's parameters — already split on their semicolons —
     * to the pen. The parsing is [TerminalPen.withSgr]'s; this is where the pen lives.
     */
    fun applySgr(params: List<String>) {
        pen = pen.withSgr(params)
    }

    /** setCursorVisible shows or hides the cursor (CSI ?25h / ?25l). */
    fun setCursorVisible(on: Boolean) {
        cursorVisible = on
    }

    /** setMouseReporting turns the forwarding of mouse events on or off. */
    fun setMouseReporting(on: Boolean) {
        mouseReporting = on
    }

    /** setMouseSgr turns the SGR mouse encoding on or off. */
    fun setMouseSgr(on: Boolean) {
        mouseSgr = on
    }

    /**
     * moveCursor puts the cursor at a cell and clamps it into the screen.
     *
     * Every cursor-addressing escape sequence ends here, so the move also cancels a
     * pending wrap: a terminal that is about to place the cursor has no wrap to defer.
     */
    fun moveCursor(x: Int, y: Int) {
        wrapPending = false
        cursorX = x
        cursorY = y
        clampCursor()
    }

    /** carriageReturn returns the cursor to the start of its row (CR). */
    fun carriageReturn() {
        cursorX = 0
        wrapPending = false
    }

    /**
     * cancelWrap drops a wrap the cursor is deferring without moving it.
     *
     * Every escape sequence and every control byte arrives with the terminal about to do
     * something of its own, and each of them ends the deferral: vterm.go clears its
     * `wrapPending` in `dispatchCSI` and beside every control byte, and a Kotlin emulator
     * that did not would wrap where the Go one writes in place.
     */
    fun cancelWrap() {
        wrapPending = false
    }

    /** backspace moves the cursor one column left, no further than the first (BS). */
    fun backspace() {
        if (cursorX > 0) {
            cursorX--
        }
        wrapPending = false
    }

    /** tab advances the cursor to the next multiple of eight columns (HT). */
    fun tab() {
        cursorX = ((cursorX / 8) + 1) * 8
        if (cursorX >= cols) {
            cursorX = cols - 1
        }
        wrapPending = false
    }

    /** putRune writes one printable character at the cursor and advances it. */
    fun putRune(ch: Char) {
        putGlyph(ch.toString(), 1)
    }

    /**
     * putGlyph writes one printable character at the cursor and advances it by the
     * columns it occupies.
     *
     * [width] is 2 for the double-width characters a CJK page or an emoji is made of.
     * Such a glyph is held whole: a lead cell that carries it and a spare cell after it
     * that carries nothing, which is what keeps a row's columns countable. A glyph that
     * would not fit at the end of a row moves whole to the next one rather than being
     * split across the margin.
     */
    fun putGlyph(glyph: String, width: Int) {
        if (wrapPending) {
            cursorX = 0
            lineFeed()
            wrapPending = false
        }
        if (cursorY < 0 || cursorY >= rows) {
            return
        }
        val columns = minOf(width, cols)
        if (cursorX + columns > cols) {
            cursorX = 0
            lineFeed()
        }
        if (cursorY < 0 || cursorY >= rows || cursorX < 0 || cursorX >= cols) {
            return
        }
        grid[cursorY][cursorX] = TerminalCell(glyph, pen, columns)
        for (i in 1 until columns) {
            if (cursorX + i < cols) {
                grid[cursorY][cursorX + i] = TerminalCell("", pen, 0)
            }
        }
        cursorX += columns
        if (cursorX >= cols) {
            wrapPending = true
            cursorX = cols - 1 // the cursor rests on the last column until the next rune
        }
    }

    /** lineFeed moves the cursor down one row, scrolling the region at its bottom. */
    fun lineFeed() {
        if (cursorY == scrollBottom) {
            scrollUp(1)
            return
        }
        if (cursorY < rows - 1) {
            cursorY++
        }
    }

    /** reverseLineFeed moves the cursor up one row, scrolling the region at its top (RI). */
    fun reverseLineFeed() {
        if (cursorY == scrollTop) {
            scrollDown(1)
            return
        }
        if (cursorY > 0) {
            cursorY--
        }
    }

    /** saveCursor remembers the cursor and the pen for [restoreCursor] (DECSC, CSI s). */
    fun saveCursor() {
        decSavedX = cursorX
        decSavedY = cursorY
        decSavedPen = pen
    }

    /** restoreCursor returns to the cursor and pen [saveCursor] remembered (DECRC, CSI u). */
    fun restoreCursor() {
        moveCursor(decSavedX, decSavedY)
        pen = decSavedPen
    }

    /** scrollUp moves the scroll region's rows up and blanks the row it leaves behind. */
    fun scrollUp(count: Int) {
        repeat(count) {
            // A row leaving the top of the whole screen is a line of the console that has
            // gone by, and is kept. A scroll inside a region below the top is the client
            // drawing in part of the screen, and the lines it moves did not leave anything.
            if (scrollTop == 0 && !isAltScreen) {
                recordHistory(grid[0])
            }
            for (y in scrollTop until scrollBottom) {
                grid[y] = grid[y + 1]
            }
            grid[scrollBottom] = blankRow()
        }
    }

    /**
     * recordHistory keeps a row that has left the top of the screen.
     *
     * It is called before the row shuffle, and that is what makes keeping the array itself
     * safe: the shuffle only ever moves references up and drops the top one, so the array
     * handed over here is one no part of the grid holds any more and nothing will write to
     * again. Copying it instead would only cost memory on every line of output.
     */
    private fun recordHistory(row: Array<TerminalCell>) {
        history.addLast(row)
        linesScrolledOff++
        while (history.size > MAX_SCROLLBACK_LINES) {
            history.removeFirst()
        }
    }

    /** scrollDown moves the scroll region's rows down and blanks the row it leaves behind. */
    fun scrollDown(count: Int) {
        repeat(count) {
            for (y in scrollBottom downTo scrollTop + 1) {
                grid[y] = grid[y - 1]
            }
            grid[scrollTop] = blankRow()
        }
    }

    /**
     * insertLines inserts blank rows at the cursor, pushing the region's rows below it
     * down and off the bottom (CSI L). Outside the scroll region it does nothing.
     */
    fun insertLines(count: Int) {
        if (cursorY < scrollTop || cursorY > scrollBottom) {
            return
        }
        repeat(count) {
            for (y in scrollBottom downTo cursorY + 1) {
                grid[y] = grid[y - 1]
            }
            grid[cursorY] = blankRow()
        }
    }

    /** deleteLines deletes rows at the cursor, pulling the region's rows below it up (CSI M). */
    fun deleteLines(count: Int) {
        if (cursorY < scrollTop || cursorY > scrollBottom) {
            return
        }
        repeat(count) {
            for (y in cursorY until scrollBottom) {
                grid[y] = grid[y + 1]
            }
            grid[scrollBottom] = blankRow()
        }
    }

    /** eraseLine erases part of the cursor's row: 0 to its end, 1 to its start, 2 all of it. */
    fun eraseLine(mode: Int) {
        if (cursorY < 0 || cursorY >= rows) {
            return
        }
        val row = grid[cursorY]
        when (mode) {
            0 -> for (x in cursorX until cols) row[x] = blankCell()
            1 -> for (x in 0..minOf(cursorX, cols - 1)) row[x] = blankCell()
            2 -> row.fill(blankCell())
        }
    }

    /** eraseDisplay erases part of the screen: 0 below the cursor, 1 above it, 2 or 3 all of it. */
    fun eraseDisplay(mode: Int) {
        when (mode) {
            0 -> {
                if (cursorY >= 0 && cursorY < rows) {
                    for (x in cursorX until cols) {
                        grid[cursorY][x] = blankCell()
                    }
                    for (y in cursorY + 1 until rows) {
                        grid[y].fill(blankCell())
                    }
                }
            }

            1 -> {
                for (y in 0 until minOf(cursorY, rows)) {
                    grid[y].fill(blankCell())
                }
                if (cursorY >= 0 && cursorY < rows) {
                    for (x in 0..minOf(cursorX, cols - 1)) {
                        grid[cursorY][x] = blankCell()
                    }
                }
            }

            2, 3 -> clearScreen()
        }
    }

    /** eraseChars blanks the columns from the cursor to the right (CSI X). */
    fun eraseChars(count: Int) {
        if (cursorY < 0 || cursorY >= rows) {
            return
        }
        for (i in 0 until count) {
            if (cursorX + i >= cols) {
                break
            }
            grid[cursorY][cursorX + i] = blankCell()
        }
    }

    /** deleteChars deletes columns at the cursor, pulling the rest of the row left (CSI P). */
    fun deleteChars(count: Int) {
        if (cursorY < 0 || cursorY >= rows) {
            return
        }
        val row = grid[cursorY]
        for (x in cursorX until cols - count) {
            row[x] = row[x + count]
        }
        for (x in maxOf(cols - count, 0) until cols) {
            if (x >= cursorX) {
                row[x] = blankCell()
            }
        }
    }

    /** insertChars inserts blank columns at the cursor, pushing the row's right along (CSI @). */
    fun insertChars(count: Int) {
        if (cursorY < 0 || cursorY >= rows) {
            return
        }
        val row = grid[cursorY]
        for (x in cols - 1 downTo cursorX + count) {
            row[x] = row[x - count]
        }
        for (x in cursorX until minOf(cursorX + count, cols)) {
            row[x] = blankCell()
        }
    }

    /**
     * setScrollRegion sets the rows the screen scrolls, 0-based and inclusive, exactly as
     * vterm.go sets them from CSI r: the rows are clamped to the screen, a region with no
     * height is refused in favour of the whole screen, and the cursor is homed with it.
     */
    fun setScrollRegion(top: Int, bottom: Int) {
        val clampedTop = maxOf(top, 0)
        val clampedBottom = minOf(bottom, rows - 1)
        if (clampedTop < clampedBottom) {
            scrollTop = clampedTop
            scrollBottom = clampedBottom
        } else {
            scrollTop = 0
            scrollBottom = rows - 1
        }
        moveCursor(0, 0)
    }

    /**
     * setAltScreen switches between the primary and alternate screens.
     *
     * Entering saves the primary screen and its cursor and clears a blank alternate one;
     * leaving restores them. [saveCursor] is false for the legacy switches (?47h and
     * ?1047h), which restore the grid but leave the cursor where the alternate screen
     * left it. A switch to the screen already in use does nothing, so entering the
     * alternate screen twice does not overwrite the screen saved the first time.
     */
    fun setAltScreen(on: Boolean, saveCursor: Boolean) {
        if (on == isAltScreen) {
            return
        }
        if (on) {
            savedGrid = copyGrid(grid)
            savedCursorX = cursorX
            savedCursorY = cursorY
            clearScreen()
            cursorX = 0
            cursorY = 0
            wrapPending = false
        } else {
            val saved = savedGrid
            if (saved != null) {
                grid = saved
                savedGrid = null
                if (saveCursor) {
                    cursorX = savedCursorX
                    cursorY = savedCursorY
                }
            } else {
                clearScreen()
                cursorX = 0
                cursorY = 0
            }
            wrapPending = false
        }
        isAltScreen = on
    }

    /**
     * resize changes the screen's size, keeping the upper-left cells that still fit and
     * blanking the ones the new size adds.
     *
     * A size that is not a screen is ignored rather than applied, as vterm.go ignores it:
     * the child is told the size the surface actually has, so a zero can only come from a
     * surface that has not been laid out yet.
     */
    fun resize(newCols: Int, newRows: Int) {
        if (newCols <= 0 || newRows <= 0) {
            return
        }
        grid = resizeGrid(grid, newCols, newRows)
        cols = newCols
        rows = newRows
        scrollTop = 0
        scrollBottom = rows - 1
        clampCursor()
        // A saved primary screen is resized with the screen it will be restored onto, so
        // that leaving the alternate screen after a resize still fits.
        savedGrid?.let { savedGrid = resizeGrid(it, newCols, newRows) }
    }

    /** clearScreen blanks every cell of the screen the way vterm.go's clearScreen does. */
    private fun clearScreen() {
        for (row in grid) {
            row.fill(blankCell())
        }
    }

    /** blankCell is a space drawn with the pen in use: what an erase and a scroll leave. */
    private fun blankCell(): TerminalCell = TerminalCell(" ", pen, 1)

    /** blankRow is a fresh, blank row: what a scroll leaves behind and what a resize adds. */
    private fun blankRow(): Array<TerminalCell> = Array(cols) { blankCell() }

    /** clampCursor keeps the cursor inside the screen. */
    private fun clampCursor() {
        if (cursorX < 0) {
            cursorX = 0
        }
        if (cursorY < 0) {
            cursorY = 0
        }
        if (cursorX >= cols) {
            cursorX = cols - 1
        }
        if (cursorY >= rows) {
            cursorY = rows - 1
        }
    }
}

/**
 * TerminalColor is a colour a cell can be drawn in.
 *
 * The three cases are the three things a client can say: nothing (the theme's own
 * colour), an entry from the 256-colour palette, or a 24-bit colour of its own.
 */
sealed interface TerminalColor {
    /** Default is the terminal's own colour, which is what the theme decides. */
    object Default : TerminalColor

    /**
     * Palette is an entry in the terminal's 256-colour palette, 0-based.
     *
     * The first sixteen are the ANSI colours a program names directly; the rest are the
     * 6x6x6 colour cube and the grey ramp. The index is whatever the client asked for and
     * may be past the end of the table, which [TerminalPalette] clamps rather than rejects.
     */
    data class Palette(val index: Int) : TerminalColor

    /** Rgb is a 24-bit colour, `0xRRGGBB`. */
    data class Rgb(val value: Int) : TerminalColor
}

/**
 * TerminalPen is the colour and the attributes a character is drawn with: what a real
 * terminal holds in the graphic rendition the SGR sequences set.
 *
 * It is a value, so a save and a restore are a copy rather than a set of flags to undo,
 * and [withSgr] computes the pen an SGR sequence asks for from the pen in use — the way
 * a terminal applies one, which is incrementally rather than from nothing.
 */
data class TerminalPen(
    val foreground: TerminalColor = TerminalColor.Default,
    val background: TerminalColor = TerminalColor.Default,
    val bold: Boolean = false,
    val italic: Boolean = false,
    val underline: Boolean = false,
    val blink: Boolean = false,
    val reverse: Boolean = false,
) {

    /** withSgr returns the pen an SGR sequence's parameters leave behind. */
    fun withSgr(params: List<String>): TerminalPen {
        // A bare CSI m, or one with no parameters at all, is a reset.
        if (params.isEmpty() || (params.size == 1 && params[0].isEmpty())) {
            return DEFAULT
        }

        var fg = foreground
        var bg = background
        var bold = this.bold
        var italic = this.italic
        var underline = this.underline
        var blink = this.blink
        var reverse = this.reverse

        var i = 0
        while (i < params.size) {
            val n = csiIntOrNull(params[i])
            if (n == null) {
                i++
                continue
            }
            when {
                n == 0 -> { // the reset: no colour and no attributes
                    fg = TerminalColor.Default
                    bg = TerminalColor.Default
                    bold = false
                    italic = false
                    underline = false
                    blink = false
                    reverse = false
                }

                n == 1 -> bold = true
                n == 2 -> bold = false
                n == 3 -> italic = true
                n == 4 -> underline = true
                n == 5 -> blink = true
                n == 7 -> reverse = true
                n == 22 -> bold = false
                n == 23 -> italic = false
                n == 24 -> underline = false
                n == 25 -> blink = false
                n == 27 -> reverse = false
                n in 30..37 -> fg = TerminalColor.Palette(n - 30)
                n == 38 || n == 48 -> {
                    val parsed = parseSgrColor(params, i)
                    if (parsed != null) {
                        if (n == 38) fg = parsed.color else bg = parsed.color
                        i += parsed.consumed
                    }
                }

                n == 39 -> fg = TerminalColor.Default
                n in 40..47 -> bg = TerminalColor.Palette(n - 40)
                n == 49 -> bg = TerminalColor.Default
                n in 90..97 -> fg = TerminalColor.Palette(n - 90 + 8)
                n in 100..107 -> bg = TerminalColor.Palette(n - 100 + 8)
            }
            i++
        }

        return TerminalPen(
            foreground = fg,
            background = bg,
            bold = bold,
            italic = italic,
            underline = underline,
            blink = blink,
            reverse = reverse,
        )
    }

    companion object {
        /** DEFAULT is the pen a terminal starts with: the theme's own colours, no attributes. */
        val DEFAULT = TerminalPen()
    }
}

/**
 * TerminalCell is one column of the screen: the character drawn in it and the pen it is
 * drawn with.
 *
 * [width] is 2 on a double-width glyph's first column and 0 on the spare one after it,
 * whose [glyph] is empty; a row's columns are then countable without re-measuring the
 * characters, which is what a cursor move and a wide glyph's wrap both need.
 */
data class TerminalCell(val glyph: String, val pen: TerminalPen, val width: Int)

/** SgrColor is a colour an SGR sequence named, and how many parameters spelling it took. */
private data class SgrColor(val color: TerminalColor, val consumed: Int)

/**
 * parseSgrColor reads the colour an SGR 38 or 48 introduces: `5;n` for a palette colour,
 * `2;r;g;b` for a 24-bit one. It returns null when the parameters do not spell a colour,
 * which leaves the pen as it was rather than guessing.
 */
private fun parseSgrColor(params: List<String>, i: Int): SgrColor? {
    if (i + 1 >= params.size) {
        return null
    }
    return when (csiIntOrNull(params[i + 1])) {
        5 -> {
            if (i + 2 >= params.size) {
                return null
            }
            val index = csiIntOrNull(params[i + 2]) ?: return null
            SgrColor(TerminalColor.Palette(index), 2)
        }

        2 -> {
            if (i + 4 >= params.size) {
                return null
            }
            val r = (csiIntOrNull(params[i + 2]) ?: 0).coerceIn(0, 255)
            val g = (csiIntOrNull(params[i + 3]) ?: 0).coerceIn(0, 255)
            val b = (csiIntOrNull(params[i + 4]) ?: 0).coerceIn(0, 255)
            SgrColor(TerminalColor.Rgb((r shl 16) or (g shl 8) or b), 4)
        }

        else -> null
    }
}

/**
 * csiIntOrNull reads one SGR parameter, which must be digits and nothing else: a
 * sub-parameter spelled with colons (ITU T.416) is not an integer, so the attribute it
 * spells is ignored rather than misread as the number before the colon.
 *
 * A parameter that is all digits is capped rather than allowed to overflow: a client can
 * send a cursor position of any length, and a terminal that wrapped around to a negative
 * row would place the cursor somewhere the client never asked for.
 */
internal fun csiIntOrNull(s: String): Int? {
    if (s.isEmpty()) {
        return null
    }
    var n = 0L
    for (c in s) {
        if (c < '0' || c > '9') {
            return null
        }
        n = n * 10 + (c - '0')
        if (n > MAX_CSI_PARAM) {
            n = MAX_CSI_PARAM.toLong()
        }
    }
    return n.toInt()
}

/** MAX_CSI_PARAM is the largest number an escape sequence's parameter is read as. */
private const val MAX_CSI_PARAM = 1_000_000

/** blankGrid is a grid of spaces: the state of a fresh screen and of every new row. */
private fun blankGrid(cols: Int, rows: Int): Array<Array<TerminalCell>> =
    Array(rows) { Array(cols) { BLANK_CELL } }

/** BLANK_CELL is a space drawn with no particular pen: the state of a fresh cell. */
private val BLANK_CELL = TerminalCell(" ", TerminalPen.DEFAULT, 1)

/** copyGrid returns an independent copy of [grid], which is how the primary screen is saved. */
private fun copyGrid(grid: Array<Array<TerminalCell>>): Array<Array<TerminalCell>> =
    Array(grid.size) { grid[it].copyOf() }

/**
 * resizeGrid rebuilds [grid] at a new size, preserving the upper-left overlap.
 *
 * A wide glyph whose spare column falls outside the new width is replaced by a space:
 * half of a glyph is not a glyph, and a cell that claims a column it does not have would
 * put every column after it out by one.
 */
private fun resizeGrid(grid: Array<Array<TerminalCell>>, cols: Int, rows: Int): Array<Array<TerminalCell>> {
    val next = blankGrid(cols, rows)
    for (y in 0 until minOf(grid.size, rows)) {
        for (x in 0 until minOf(grid[y].size, cols)) {
            val cell = grid[y][x]
            val split = cell.width > 1 && x + cell.width > cols
            next[y][x] = if (split) TerminalCell(" ", TerminalPen.DEFAULT, 1) else cell
        }
    }
    return next
}
