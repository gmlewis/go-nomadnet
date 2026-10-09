// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

/** A terminal's size in cells. */
data class TerminalGrid(val cols: Int, val rows: Int)

/**
 * How big one character is, and what fits in a rectangle of pixels.
 *
 * The appliance has to tell the client what terminal it is on: the client lays itself out
 * for the columns and rows it is given, and a terminal that is one column wider than the
 * screen is a line of text with its last character cut off. So the size comes from the
 * font the view actually draws with and the rectangle it actually has, rather than from a
 * guess, and [gridFor] rounds down: half a character is not a character.
 *
 * The cell is measured from the `Paint` the view draws with (see [TerminalView]), which is
 * why this holds a size rather than measuring anything itself.
 */
data class TerminalMetrics(val cellWidth: Int, val cellHeight: Int) {

    init {
        // A cell of no pixels is not a small font, it is a font that has not been measured,
        // and every division below would be by zero.
        require(cellWidth > 0 && cellHeight > 0) {
            "a terminal cell is at least one pixel, and this one is ${cellWidth}x$cellHeight"
        }
    }

    /** colsFor is how many whole columns fit in [widthPx], and never fewer than one. */
    fun colsFor(widthPx: Int): Int = (widthPx / cellWidth).coerceAtLeast(1)

    /** rowsFor is how many whole rows fit in [heightPx], and never fewer than one. */
    fun rowsFor(heightPx: Int): Int = (heightPx / cellHeight).coerceAtLeast(1)

    /** gridFor is the terminal size a [widthPx] by [heightPx] view can hold. */
    fun gridFor(widthPx: Int, heightPx: Int): TerminalGrid =
        TerminalGrid(colsFor(widthPx), rowsFor(heightPx))

    /** pixelX is where a column starts, from the view's left edge. */
    fun pixelX(col: Int): Int = col * cellWidth

    /** pixelY is where a row starts, from the view's top edge. */
    fun pixelY(row: Int): Int = row * cellHeight
}

/**
 * Where the window sits on the console's lines.
 *
 * The console is the client's screen plus the lines that have scrolled off it, and the
 * window is the part of that the view shows. The position is an [offset] counted up from
 * the bottom — zero means the newest line is on screen — because that is the position the
 * reader actually cares about, and because it is the position that survives new output:
 * the anchor is either the bottom of the console or a particular line, and both are
 * expressed here.
 */
object TerminalScroll {

    /** maxOffset is how far the window can be dragged: the lines above the screen. */
    fun maxOffset(totalLines: Int, visibleRows: Int): Int = (totalLines - visibleRows).coerceAtLeast(0)

    /**
     * firstVisibleLine is the index of the console line the window's top row shows.
     *
     * A window taller than the console has nothing above its first line, so it shows that
     * line rather than counting rows that do not exist.
     */
    fun firstVisibleLine(totalLines: Int, visibleRows: Int, offset: Int): Int =
        (totalLines - visibleRows - offset).coerceAtLeast(0)

    /**
     * offsetAfterLines is the window's offset once [addedLines] have arrived below it.
     *
     * At the bottom the window follows the output — the reader is watching the newest line,
     * and it must stay the newest line. Anywhere else the reader is reading something, and
     * text that moved under their eyes while they read it is worse than no scrollback at
     * all: the offset grows by exactly what arrived, which keeps the same lines on screen.
     * It stops at the oldest line there is.
     */
    fun offsetAfterLines(offset: Int, addedLines: Int, maxOffset: Int): Int =
        if (offset <= 0) {
            0
        } else {
            (offset + addedLines).coerceIn(0, maxOffset.coerceAtLeast(0))
        }
}

/**
 * One button on the on-screen key row.
 *
 * A tablet's keyboard has no escape and no arrows, and the client cannot be driven without
 * them: escape backs out of a dialog, tab moves between the regions of a window, and the
 * arrows walk a menu.
 */
sealed interface KeyRowEntry {

    /** label is what the button shows. */
    val label: String

    /** Press is a key that is sent when the button is tapped. */
    data class Press(override val label: String, val key: TerminalKey) : KeyRowEntry

    /**
     * Control is the control key, which is a latch rather than a key.
     *
     * A tablet cannot hold one key and press another, so control is armed by a tap and
     * spent by the next character: that is how ctrl-c reaches the client at all.
     */
    data class Control(override val label: String = "Ctrl") : KeyRowEntry
}

/**
 * The row of keys the view offers along its bottom edge.
 *
 * [entries] is the whole of it — what is drawn, what is tapped, and what each button sends
 * — so the row can be asserted without a screen. The buttons share the width equally, and
 * [entryAt] is the division that turns a tap into one of them.
 */
object TerminalKeyRow {

    /** entries is the row's buttons, left to right. */
    val entries: List<KeyRowEntry> = listOf(
        KeyRowEntry.Control(),
        KeyRowEntry.Press("Esc", TerminalKey.Special(SpecialKey.ESCAPE)),
        KeyRowEntry.Press("Tab", TerminalKey.Special(SpecialKey.TAB)),
        KeyRowEntry.Press("↑", TerminalKey.Special(SpecialKey.UP)),
        KeyRowEntry.Press("↓", TerminalKey.Special(SpecialKey.DOWN)),
        KeyRowEntry.Press("←", TerminalKey.Special(SpecialKey.LEFT)),
        KeyRowEntry.Press("→", TerminalKey.Special(SpecialKey.RIGHT)),
    )

    /**
     * entryAt is the button a tap [x] pixels from the row's left edge landed on.
     *
     * A row that has been drawn with no width yet — a view between construction and its
     * first layout — is its first button rather than a division by zero.
     */
    fun entryAt(x: Int, width: Int): KeyRowEntry {
        if (width <= 0) return entries.first()
        val index = (x.toLong() * entries.size / width).toInt()
        return entries[index.coerceIn(0, entries.size - 1)]
    }

    /**
     * withControl is the key a tap sends, given whether control is armed.
     *
     * Only a single character has a control code. A word committed by an input method is
     * not a character to the terminal, and a key like the arrows has no code either, so
     * both are sent as themselves — the latch changes what it can and leaves the rest.
     */
    fun withControl(armed: Boolean, key: TerminalKey): TerminalKey {
        if (!armed) return key
        val rune = key as? TerminalKey.Rune ?: return key
        if (rune.text.length != 1) return key
        return TerminalKey.Ctrl(rune.text[0])
    }
}

/**
 * Control, on a keyboard that cannot hold it down.
 *
 * A tablet has no Control key, so Control is a switch: a tap on the key row arms it, and the
 * next character spends it. The character is whatever is typed next — on the soft keyboard,
 * or on a keyboard that is not this app's — so the latch belongs to the console and not to
 * the key row. A latch the row alone consulted would send a plain `q` where the person meant
 * `^Q`, which is the difference between quitting and typing.
 *
 * A key with no control code — an arrow, or a word an input method commits at once — is sent
 * as itself and leaves the latch armed: the person armed Control for a character, and the
 * arrow was not one.
 */
class ControlLatch {

    /** armed is whether the next character is to be sent with control. */
    var armed: Boolean = false
        private set

    /** toggle arms the latch, or disarms it if it was already armed. */
    fun toggle() {
        armed = !armed
    }

    /**
     * spend applies the latch to [key], and spends it if it changed anything.
     *
     * The return is the key to send, which is [key] itself whenever the latch had nothing to
     * change about it.
     */
    fun spend(key: TerminalKey): TerminalKey {
        val sent = TerminalKeyRow.withControl(armed, key)
        if (sent != key) {
            armed = false
        }
        return sent
    }
}
