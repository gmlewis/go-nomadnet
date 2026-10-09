// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

/** The two colours a cell is drawn with, as ARGB packed ints. */
data class TerminalColors(val foreground: Int, val background: Int)

/**
 * The colours the console is drawn in.
 *
 * A terminal screen holds colour *names* — a palette index, a 24-bit value, or "whatever
 * the terminal uses" — because that is what the client says in its escape sequences. A
 * view draws pixels, so the names have to be resolved, and this is the whole of that: the
 * 256-entry table every terminal agrees on, and the theme's own two colours for the cells
 * the client left alone.
 *
 * The client is run with `COLORTERM=truecolor`, so most of what it draws arrives as 24-bit
 * and is passed through untouched. The table still matters for the palettes the client
 * deliberately asks for, and the default pair matters for everything else: a mismatch there
 * is an interface drawn in the wrong colours rather than a visible error.
 *
 * What is *not* here is the theme's own palette for its named colours. The colour a client
 * sends as entry 1 *is* the palette's red, not the theme's; the theme's colours reach the
 * screen inside the 24-bit values, and its default pair is [DEFAULT_FOREGROUND] and
 * [DEFAULT_BACKGROUND].
 */
object TerminalPalette {

    /**
     * DEFAULT_BACKGROUND is the colour behind everything, and behind a cell the client gave
     * no background for. It is the dark theme's own background.
     */
    const val DEFAULT_BACKGROUND: Int = 0xff0a141e.toInt()

    /** DEFAULT_FOREGROUND is the colour of text the client gave no colour for. */
    const val DEFAULT_FOREGROUND: Int = 0xffd8d8d8.toInt()

    /** OPAQUE is the alpha every colour is given: a translucent glyph is an unreadable one. */
    private const val OPAQUE: Int = 0xff shl 24

    // The first sixteen are the ANSI colours in the order every terminal has them: eight
    // hues, then the same eight a step brighter.
    private val ANSI: IntArray = intArrayOf(
        0x000000, // 0 black
        0xcd0000, // 1 red
        0x00cd00, // 2 green
        0xcdcd00, // 3 yellow
        0x0000ee, // 4 blue
        0xcd00cd, // 5 magenta
        0x00cdcd, // 6 cyan
        0xe5e5e5, // 7 white
        0x7f7f7f, // 8 bright black
        0xff0000, // 9 bright red
        0x00ff00, // 10 bright green
        0xffff00, // 11 bright yellow
        0x5c5cff, // 12 bright blue
        0xff00ff, // 13 bright magenta
        0x00ffff, // 14 bright cyan
        0xffffff, // 15 bright white
    )

    // The six levels the colour cube's axes step through. They are the values terminals
    // have used for decades, and they are not evenly spaced: the first step is wider,
    // because a cube with even steps has no near-black in it.
    private val CUBE_STEPS: IntArray = intArrayOf(0x00, 0x5f, 0x87, 0xaf, 0xd7, 0xff)

    /**
     * foreground is the colour a cell's text is drawn in.
     *
     * An index past the end of the table is clamped rather than rejected: it comes from a
     * client asking for a colour that does not exist, and a terminal that fails on a colour
     * escape is worse than one that draws the nearest thing it has.
     */
    fun foreground(color: TerminalColor): Int = when (color) {
        is TerminalColor.Default -> DEFAULT_FOREGROUND
        is TerminalColor.Rgb -> OPAQUE or (color.value and 0xffffff)
        is TerminalColor.Palette -> paletteEntry(color.index)
    }

    /**
     * background is the colour behind a cell's text.
     *
     * It differs from [foreground] only in what "the terminal's own colour" means, which is
     * the whole point of resolving the two separately.
     */
    fun background(color: TerminalColor): Int = when (color) {
        is TerminalColor.Default -> DEFAULT_BACKGROUND
        is TerminalColor.Rgb -> OPAQUE or (color.value and 0xffffff)
        is TerminalColor.Palette -> paletteEntry(color.index)
    }

    /**
     * colorsOf is the two colours a pen draws its cell with, reverse video included.
     *
     * Reverse is a swap of the resolved pair rather than of the names, because that is what
     * a terminal does: a cell with no colours of its own and reverse set is drawn in the
     * default background on the default foreground.
     */
    fun colorsOf(pen: TerminalPen): TerminalColors {
        val text = foreground(pen.foreground)
        val behind = background(pen.background)
        return if (pen.reverse) TerminalColors(behind, text) else TerminalColors(text, behind)
    }

    /** paletteEntry is one entry of the 256-colour palette, in the standard order. */
    private fun paletteEntry(index: Int): Int {
        val i = index.coerceIn(0, 255)
        if (i < ANSI.size) return OPAQUE or ANSI[i]
        if (i < 232) {
            // Entries 16 to 231 are a 6x6x6 cube, red varying slowest and blue fastest.
            val cube = i - 16
            val r = CUBE_STEPS[cube / 36]
            val g = CUBE_STEPS[(cube / 6) % 6]
            val b = CUBE_STEPS[cube % 6]
            return OPAQUE or (r shl 16) or (g shl 8) or b
        }
        // The last twenty-four are an even grey ramp from nearly black to nearly white.
        val grey = 8 + 10 * (i - 232)
        return OPAQUE or (grey shl 16) or (grey shl 8) or grey
    }
}
