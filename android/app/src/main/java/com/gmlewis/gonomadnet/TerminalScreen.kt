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
 */
class TerminalScreen(cols: Int, rows: Int) {

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

    /** The first row the screen scrolls, 0-based. */
    var scrollTop: Int = 0
        private set

    /** The last row the screen scrolls, 0-based and inclusive. */
    var scrollBottom: Int = rows - 1
        private set

    /** Whether the alternate screen is the one being drawn on (CSI ?1049h / ?1049l). */
    var isAltScreen: Boolean = false
        private set

    private var grid: Array<CharArray> = blankGrid(cols, rows)

    /**
     * Whether the cursor rests on the last column waiting for the next rune to wrap.
     * A terminal wraps when the next character arrives, not when the last one is
     * written, which is what lets a program overtype the last column of a full line.
     */
    private var wrapPending = false

    private var savedGrid: Array<CharArray>? = null
    private var savedCursorX = 0
    private var savedCursorY = 0

    /** rowText renders one row in full, trailing blanks included. */
    fun rowText(y: Int): String = String(grid[y])

    /** text renders the whole screen, one row per line. */
    fun text(): String = (0 until rows).joinToString("\n") { rowText(it) }

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

    /** putRune writes one printable character at the cursor and advances it. */
    fun putRune(ch: Char) {
        if (wrapPending) {
            cursorX = 0
            lineFeed()
            wrapPending = false
        }
        if (cursorY < 0 || cursorY >= rows || cursorX < 0 || cursorX >= cols) {
            return
        }
        grid[cursorY][cursorX] = ch
        cursorX++
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

    /** scrollUp moves the scroll region's rows up and blanks the row it leaves behind. */
    fun scrollUp(count: Int) {
        repeat(count) {
            for (y in scrollTop until scrollBottom) {
                grid[y] = grid[y + 1]
            }
            grid[scrollBottom] = blankRow()
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

    /** eraseLine erases part of the cursor's row: 0 to its end, 1 to its start, 2 all of it. */
    fun eraseLine(mode: Int) {
        if (cursorY < 0 || cursorY >= rows) {
            return
        }
        val row = grid[cursorY]
        when (mode) {
            0 -> for (x in cursorX until cols) row[x] = ' '
            1 -> for (x in 0..minOf(cursorX, cols - 1)) row[x] = ' '
            2 -> row.fill(' ')
        }
    }

    /** eraseDisplay erases part of the screen: 0 below the cursor, 1 above it, 2 or 3 all of it. */
    fun eraseDisplay(mode: Int) {
        when (mode) {
            0 -> {
                if (cursorY >= 0 && cursorY < rows) {
                    for (x in cursorX until cols) {
                        grid[cursorY][x] = ' '
                    }
                    for (y in cursorY + 1 until rows) {
                        grid[y].fill(' ')
                    }
                }
            }

            1 -> {
                for (y in 0 until minOf(cursorY, rows)) {
                    grid[y].fill(' ')
                }
                if (cursorY >= 0 && cursorY < rows) {
                    for (x in 0..minOf(cursorX, cols - 1)) {
                        grid[cursorY][x] = ' '
                    }
                }
            }

            2, 3 -> clearScreen()
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
            row.fill(' ')
        }
    }

    /** blankRow is a fresh, blank row: what a scroll leaves behind and what a resize adds. */
    private fun blankRow(): CharArray = CharArray(cols) { ' ' }

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

/** blankGrid is a grid of spaces: the state of a fresh screen and of every new row. */
private fun blankGrid(cols: Int, rows: Int): Array<CharArray> = Array(rows) { CharArray(cols) { ' ' } }

/** copyGrid returns an independent copy of [grid], which is how the primary screen is saved. */
private fun copyGrid(grid: Array<CharArray>): Array<CharArray> = Array(grid.size) { grid[it].copyOf() }

/** resizeGrid rebuilds [grid] at a new size, preserving the upper-left overlap. */
private fun resizeGrid(grid: Array<CharArray>, cols: Int, rows: Int): Array<CharArray> {
    val next = blankGrid(cols, rows)
    for (y in 0 until minOf(grid.size, rows)) {
        for (x in 0 until minOf(grid[y].size, cols)) {
            next[y][x] = grid[y][x]
        }
    }
    return next
}
