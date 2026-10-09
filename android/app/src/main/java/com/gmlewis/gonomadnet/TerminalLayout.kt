// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import kotlin.math.max

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
 * One button on the on-screen key strip.
 *
 * A tablet's keyboard has no escape and no arrows, and the client cannot be driven without
 * them: escape backs out of a dialog, tab moves between the regions of a window, and the
 * arrows walk a menu. The strip carries the same keys the Termux extra-keys row does, so a
 * person who has driven this client under Termux has the same buttons here.
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

    /**
     * Alt is the alt (meta) key, which is also a latch.
     *
     * A terminal sends an alt combination as the key's own bytes with an escape in front of
     * them — see [TerminalKey.Alt] — so this is a modifier the strip arms and spends rather
     * than a key it presses.
     */
    data class Alt(override val label: String = "Alt") : KeyRowEntry

    /**
     * Toggle folds the strip away, and brings it back.
     *
     * The strip costs rows, and rows are what the interface is made of; a reader who wants
     * them back for the client's own drawing folds it away and gets the whole screen. It is
     * never gone: the folded strip is the toggle's own button, so the row can always be
     * brought back. Which way it points is what the button says.
     */
    data class Toggle(val collapsed: Boolean) : KeyRowEntry {
        override val label: String get() = if (collapsed) "▴" else "▾"
    }
}

/**
 * The strip of keys the view offers along its bottom edge.
 *
 * [rows] is the whole of it — what is drawn, what is tapped, and what each button sends —
 * so the strip can be asserted without a screen. It is two rows of seven, which is the
 * Termux extra-keys row this appliance's console is modelled on: the keys a terminal needs
 * that no soft keyboard has.
 *
 * The buttons share each row's width equally, and [entryAt] is the division that turns a tap
 * into one of them.
 */
object TerminalKeyRow {

    /** ROW_ONE is the strip's top row: escape, the two path characters, and navigation. */
    private val ROW_ONE: List<KeyRowEntry> = listOf(
        KeyRowEntry.Press("ESC", TerminalKey.Special(SpecialKey.ESCAPE)),
        KeyRowEntry.Press("/", TerminalKey.Rune("/")),
        KeyRowEntry.Press("-", TerminalKey.Rune("-")),
        KeyRowEntry.Press("HOME", TerminalKey.Special(SpecialKey.HOME)),
        KeyRowEntry.Press("↑", TerminalKey.Special(SpecialKey.UP)),
        KeyRowEntry.Press("END", TerminalKey.Special(SpecialKey.END)),
        KeyRowEntry.Press("PGUP", TerminalKey.Special(SpecialKey.PAGE_UP)),
    )

    /** ROW_TWO is the strip's bottom row: the modifiers and the rest of the navigation. */
    private val ROW_TWO: List<KeyRowEntry> = listOf(
        KeyRowEntry.Toggle(collapsed = false),
        KeyRowEntry.Control(),
        KeyRowEntry.Alt(),
        KeyRowEntry.Press("←", TerminalKey.Special(SpecialKey.LEFT)),
        KeyRowEntry.Press("↓", TerminalKey.Special(SpecialKey.DOWN)),
        KeyRowEntry.Press("→", TerminalKey.Special(SpecialKey.RIGHT)),
        KeyRowEntry.Press("PGDN", TerminalKey.Special(SpecialKey.PAGE_DOWN)),
    )

    /**
     * rows is the strip as it stands, top row first.
     *
     * A folded strip is one row holding the toggle and nothing else, which is the button
     * that unfolds it — a strip that could be folded away and not brought back would be a
     * keyboard with no escape key and no way to ask for one.
     */
    fun rows(collapsed: Boolean): List<List<KeyRowEntry>> =
        if (collapsed) {
            listOf(listOf(KeyRowEntry.Toggle(collapsed = true)))
        } else {
            listOf(ROW_ONE, ROW_TWO)
        }

    /**
     * padding is the space above and below the strip's rows, from one cell's height.
     *
     * It is a fraction of the cell so that the strip keeps its proportions with the font
     * size, and never less than a few pixels: a button with no margin is a button whose
     * label touches the row above it.
     */
    fun padding(cellHeight: Int): Int = max(4, cellHeight / 4)

    /**
     * height is how much of the bottom edge [rows] take, given one cell's height.
     *
     * The rows are the client's own: what the strip takes from the bottom of the screen is
     * what the client does not get, which is why showing and hiding it is a resize rather
     * than a redraw. A strip that is not shown takes nothing at all.
     */
    fun height(rows: List<List<KeyRowEntry>>, cellHeight: Int): Int =
        if (rows.isEmpty()) 0 else rows.size * cellHeight + 2 * padding(cellHeight)

    /**
     * entryAt is the button a tap at ([x], [y]) landed on, in the view's own pixels.
     *
     * [stripTop] is where the strip begins and [rowHeight] how tall one of its rows is, both
     * of which are the view's arithmetic: the rows are drawn in those bands, so a tap is
     * divided by them as well. A row or a column a tap cannot be outside of is clamped
     * rather than rejected, because a finger on an edge is a finger on a button.
     *
     * A strip drawn with no width yet — a view between construction and its first layout —
     * is its first button rather than a division by zero.
     */
    fun entryAt(
        x: Int,
        y: Int,
        width: Int,
        stripTop: Int,
        rowHeight: Int,
        rows: List<List<KeyRowEntry>>,
    ): KeyRowEntry {
        val row = if (rowHeight <= 0) {
            0
        } else {
            ((y - stripTop) / rowHeight).coerceIn(0, rows.size - 1)
        }
        val entries = rows[row]
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
 * Control and Alt, on a keyboard that cannot hold them down.
 *
 * A tablet has no Control or Alt key, so each is a switch: a tap on the strip arms it, and
 * the next character spends it. The character is whatever is typed next — on the soft
 * keyboard, or on a keyboard that is not this app's — so the latch belongs to the console
 * and not to the strip. A latch the strip alone consulted would send a plain `q` where the
 * person meant `^Q`, which is the difference between quitting and typing.
 *
 * A modifier is spent only by a key it changed. A key with no control code — an arrow, or a
 * word an input method commits at once — is sent as itself and leaves Control armed: the
 * person armed Control for a character, and the arrow was not one. Alt changes any key, so
 * anything spends it.
 */
class ModifierLatch {

    /** ctrl is whether the next character is to be sent with control. */
    var ctrl: Boolean = false
        private set

    /** alt is whether the next key is to be sent as an alt combination. */
    var alt: Boolean = false
        private set

    /** toggleCtrl arms the latch, or disarms it if it was already armed. */
    fun toggleCtrl() {
        ctrl = !ctrl
    }

    /** toggleAlt arms the latch, or disarms it if it was already armed. */
    fun toggleAlt() {
        alt = !alt
    }

    /**
     * spend applies the latches to [key], and spends the ones that changed it.
     *
     * The return is the key to send, which is [key] itself whenever the latches had nothing
     * to change about it.
     */
    fun spend(key: TerminalKey): TerminalKey {
        var sent = key
        if (ctrl) {
            val ctrled = TerminalKeyRow.withControl(armed = true, key = sent)
            if (ctrled != sent) {
                sent = ctrled
                ctrl = false
            }
        }
        if (alt) {
            sent = TerminalKey.Alt(sent)
            alt = false
        }
        return sent
    }
}
