// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The colours the console is drawn in.
 *
 * The client sends colours as terminal escapes, so what the screen holds is a colour
 * *name*: which of the 256 palette entries, a 24-bit value, or the terminal's own default.
 * The view draws pixels, so something has to turn one into the other, and that something is
 * pure — a table and a swap — which is why it is asserted here rather than looked at on a
 * tablet. A palette that is off by one entry draws the whole interface in the wrong
 * colours, and nothing about it looks like a bug in the code.
 */
class TerminalPaletteTest {

    /** argb renders a colour the way the table does, so a failure reads as a colour. */
    private fun argb(value: Int): String = "#%08x".format(value)

    @Test
    fun `the sixteen ansi colours are the standard ones`() {
        // The first sixteen entries are the ones a program names directly, and every
        // terminal is expected to agree on them: index 1 is red, index 4 is blue, and the
        // eight bright ones are the same hues a step brighter.
        val cases = listOf(
            PaletteCase("black", 0, 0x000000),
            PaletteCase("red", 1, 0xcd0000),
            PaletteCase("green", 2, 0x00cd00),
            PaletteCase("yellow", 3, 0xcdcd00),
            PaletteCase("blue", 4, 0x0000ee),
            PaletteCase("magenta", 5, 0xcd00cd),
            PaletteCase("cyan", 6, 0x00cdcd),
            PaletteCase("white", 7, 0xe5e5e5),
            PaletteCase("bright black", 8, 0x7f7f7f),
            PaletteCase("bright red", 9, 0xff0000),
            PaletteCase("bright blue", 12, 0x5c5cff),
            PaletteCase("bright white", 15, 0xffffff),
        )

        for (case in cases) {
            val want = (0xff shl 24) or case.rgb
            assertEquals(
                "${case.name}: ${argb(TerminalPalette.foreground(TerminalColor.Palette(case.index)))}",
                argb(want),
                argb(TerminalPalette.foreground(TerminalColor.Palette(case.index))),
            )
        }
    }

    @Test
    fun `the rest of the palette is the colour cube and the grey ramp`() {
        // Entries 16 to 231 are a 6x6x6 cube whose steps are the ones every terminal uses,
        // and the last twenty-four are the greys. This is the part a program reaches for
        // when it asks for 256 colours, which the client does.
        val steps = listOf(0x00, 0x5f, 0x87, 0xaf, 0xd7, 0xff)

        for (r in 0..5) {
            for (g in 0..5) {
                for (b in 0..5) {
                    val index = 16 + 36 * r + 6 * g + b
                    val want = (0xff shl 24) or (steps[r] shl 16) or (steps[g] shl 8) or steps[b]
                    assertEquals(
                        "the cube entry $index",
                        argb(want),
                        argb(TerminalPalette.foreground(TerminalColor.Palette(index))),
                    )
                }
            }
        }

        // The ramp runs from nearly black to nearly white in twenty-four even steps.
        assertEquals(argb(0xff080808.toInt()), argb(TerminalPalette.foreground(TerminalColor.Palette(232))))
        assertEquals(argb(0xffeeeeee.toInt()), argb(TerminalPalette.foreground(TerminalColor.Palette(255))))
        for (i in 233..255) {
            val grey = 8 + 10 * (i - 232)
            assertEquals(
                "the grey $i",
                argb((0xff shl 24) or (grey shl 16) or (grey shl 8) or grey),
                argb(TerminalPalette.foreground(TerminalColor.Palette(i))),
            )
        }

        // An index past the end of the table comes from a program that asked for a colour
        // that does not exist. It is clamped rather than thrown: a terminal that dies on a
        // colour escape is worse than one that draws white.
        assertEquals(
            argb(TerminalPalette.foreground(TerminalColor.Palette(255))),
            argb(TerminalPalette.foreground(TerminalColor.Palette(999))),
        )
        assertEquals(
            argb(TerminalPalette.foreground(TerminalColor.Palette(0))),
            argb(TerminalPalette.foreground(TerminalColor.Palette(-1))),
        )
    }

    @Test
    fun `a 24-bit colour is drawn as it was sent`() {
        // The client is run with COLORTERM=truecolor, so most of what it draws arrives as
        // 24-bit rather than as a palette entry, and it must reach the screen unchanged:
        // rounding it into the 256-colour cube is what makes a themed interface look wrong.
        val cases = listOf(0x0a141e, 0x00a533, 0xfd3, 0xffffff, 0x000000)

        for (value in cases) {
            assertEquals(
                "the colour #$value",
                argb((0xff shl 24) or value),
                argb(TerminalPalette.foreground(TerminalColor.Rgb(value))),
            )
        }

        // Anything above the 24 bits a colour has is not a colour, and must not leak into
        // the alpha byte: a fully transparent glyph is an invisible one.
        assertEquals(
            argb((0xff shl 24) or 0x123456),
            argb(TerminalPalette.foreground(TerminalColor.Rgb(0xff123456.toInt()))),
        )
    }

    @Test
    fun `the terminal's own colours are the theme's`() {
        // A cell the client gave no colour for is drawn in the terminal's own, which is the
        // dark theme the rest of the appliance uses. The two must differ, or the default
        // text is invisible on the default background.
        assertNotEquals(
            "the default text is the same colour as the default background",
            TerminalPalette.DEFAULT_FOREGROUND,
            TerminalPalette.DEFAULT_BACKGROUND,
        )
        assertEquals(
            argb(TerminalPalette.DEFAULT_FOREGROUND),
            argb(TerminalPalette.foreground(TerminalColor.Default)),
        )
        assertEquals(
            argb(TerminalPalette.DEFAULT_BACKGROUND),
            argb(TerminalPalette.background(TerminalColor.Default)),
        )
        assertTrue(
            "the default background is not dark: ${argb(TerminalPalette.DEFAULT_BACKGROUND)}",
            (TerminalPalette.DEFAULT_BACKGROUND and 0xffffff) < 0x404040,
        )
        assertTrue(
            "the default text is not light: ${argb(TerminalPalette.DEFAULT_FOREGROUND)}",
            (TerminalPalette.DEFAULT_FOREGROUND and 0xffffff) > 0xc0c0c0,
        )
    }

    @Test
    fun `a pen resolves to the two colours a cell is drawn with`() {
        assertEquals(
            TerminalColors(TerminalPalette.DEFAULT_FOREGROUND, TerminalPalette.DEFAULT_BACKGROUND),
            TerminalPalette.colorsOf(TerminalPen()),
        )
        assertEquals(
            TerminalColors(
                TerminalPalette.foreground(TerminalColor.Palette(1)),
                TerminalPalette.background(TerminalColor.Palette(4)),
            ),
            TerminalPalette.colorsOf(
                TerminalPen(
                    foreground = TerminalColor.Palette(1),
                    background = TerminalColor.Palette(4),
                ),
            ),
        )

        // Reverse video swaps them, which is how a selected list row or a menu bar is drawn.
        val plain = TerminalPen(
            foreground = TerminalColor.Rgb(0x112233),
            background = TerminalColor.Rgb(0xffee00),
        )
        assertEquals(
            TerminalColors(
                TerminalPalette.foreground(TerminalColor.Rgb(0xffee00)),
                TerminalPalette.background(TerminalColor.Rgb(0x112233)),
            ),
            TerminalPalette.colorsOf(plain.copy(reverse = true)),
        )
        // Reversing a cell that named no colours swaps the theme's own two.
        assertEquals(
            TerminalColors(TerminalPalette.DEFAULT_BACKGROUND, TerminalPalette.DEFAULT_FOREGROUND),
            TerminalPalette.colorsOf(TerminalPen(reverse = true)),
        )
    }

    /** PaletteCase is one palette entry and the colour it must be. */
    private data class PaletteCase(val name: String, val index: Int, val rgb: Int)
}
