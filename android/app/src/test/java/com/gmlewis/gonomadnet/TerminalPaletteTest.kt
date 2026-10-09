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
import java.io.File

/**
 * The colours the console is drawn in.
 *
 * The client sends colours as terminal escapes, so what the screen holds is a colour
 * *name*: which of the 256 palette entries, a 24-bit value, or the terminal's own default.
 * The view draws pixels, so something has to turn one into the other, and that something is
 * pure — a table and a swap — which is why it is asserted here rather than looked at on a
 * tablet. A palette that is off by one entry draws the whole interface in the wrong
 * colours, and nothing about it looks like a bug in the code.
 *
 * The colours the client leaves to the terminal are the ones that matter most, and they are
 * the appliance's published theme: the same `colors.properties` the client installs under
 * Termux is the file these tests read, so the console and the Termux install cannot resolve
 * the operator's theme two different ways.
 */
class TerminalPaletteTest {

    /** argb renders a colour the way the table does, so a failure reads as a colour. */
    private fun argb(value: Int): String = "#%08x".format(value)

    @Test
    fun `the palette is the theme the appliance publishes`() {
        // The theme is Termux's `colors.properties`, which the APK carries and the client
        // installs under Termux: a black background, a bright green foreground, and the
        // gruvbox sixteen. The console resolves the client's unnamed colours and its named
        // palette entries from this file's values, and the file is what is read here — a
        // second copy of the sixteen in this test would be a second copy to drift.
        val theme = publishedTheme()

        assertEquals(
            "the theme's own background",
            theme["background"],
            TerminalPalette.DEFAULT_BACKGROUND and 0xffffff,
        )
        assertEquals(
            "the theme's own foreground",
            theme["foreground"],
            TerminalPalette.DEFAULT_FOREGROUND and 0xffffff,
        )
        // The cursor is drawn as the cell's own colours reversed, so the theme's cursor is
        // what that block looks like only when the theme's cursor is its foreground.
        assertEquals("the theme's cursor", theme["foreground"], theme["cursor"])

        for (index in 0..15) {
            val wanted = theme["color$index"]
            assertTrue("the theme names no color$index", wanted != null)
            assertEquals(
                "palette entry $index is not the theme's",
                argb((0xff shl 24) or wanted!!),
                argb(TerminalPalette.foreground(TerminalColor.Palette(index))),
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
        // A cell the client gave no colour for is drawn in the terminal's own, and the
        // client leaves most of its chrome to the terminal: the frame borders, the pane
        // titles, the key hints along the bottom. So the default pair is the whole look of
        // the interface, and it is the published theme's pair rather than a second one
        // chosen here.
        val theme = publishedTheme()
        assertEquals(
            argb((0xff shl 24) or theme["foreground"]!!),
            argb(TerminalPalette.DEFAULT_FOREGROUND),
        )
        assertEquals(
            argb((0xff shl 24) or theme["background"]!!),
            argb(TerminalPalette.DEFAULT_BACKGROUND),
        )
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

    private companion object {
        /** The terminal theme the APK publishes, as Gradle runs these tests. */
        const val THEME_FILE = "src/main/assets/colors.properties"

        /**
         * publishedTheme is the APK's terminal theme, as name to 24-bit colour.
         *
         * Termux's format is one `key = value` per line with each colour written `#rrggbb`,
         * and a comment is a line that starts with `#`. The `#` inside a value is part of
         * the value, so the split is on the first `=` rather than on the hash.
         */
        fun publishedTheme(): Map<String, Int> {
            val file = File(THEME_FILE)
            assertTrue("the published theme is not in the APK's assets: $file", file.isFile)
            return file.readLines()
                .map { it.trim() }
                .filter { it.isNotEmpty() && !it.startsWith("#") }
                .associate { line ->
                    val (key, value) = line.split("=", limit = 2).map { it.trim() }
                    key to value.removePrefix("#").toInt(16)
                }
        }
    }
}
