// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * What the appliance sends the client when the console is tapped.
 *
 * The client's interface is a terminal program, so a tap is not an event it can be handed
 * either: it is the mouse report a real terminal would have written, at the cell the finger
 * landed on. Without it the client is a screen that can only be read — its lists, its
 * buttons and its menu bar are all things it draws for a mouse and a keyboard, and a tablet
 * has neither unless the terminal spells one.
 *
 * Both encodings a terminal can use are here, because the client says which it wants: SGR
 * (CSI ?1006h) carries the button and both coordinates as decimal numbers, and the older X10
 * form packs them into three bytes with 32 added to each. A tablet's client asks for SGR —
 * the coordinates can then be past column 223 — but a terminal that only ever wrote one of
 * them would be a terminal that ignores what the client asked for.
 *
 * The cells here are the client's own, counted from its top-left corner and zero-based, as
 * everything in the emulator counts them. Both encodings carry them one-based, because the
 * wire's first cell is 1: that conversion is the encoder's, and it is the one thing about
 * these vectors that is worth reading twice.
 */
class TerminalMouseTest {

    /** sent renders what one report is spelled as, as integers, at the console's cell. */
    private fun sent(report: MouseReport, sgr: Boolean = true): List<Int> =
        TerminalMouse.encode(report, sgr).map { it.toInt() and 0xff }

    /** escape is the byte every one of these sequences begins with. */
    private val escape = 0x1b

    @Test
    fun `a press and a release are both sent, and are what makes a click`() {
        // The client derives a click from a press and a release in the same cell: a terminal
        // that sent only the press would move the client's focus and activate nothing, which
        // is exactly a console where tapping an entry does nothing at all.
        assertEquals(
            "the press",
            listOf(escape, '['.code, '<'.code, '0'.code, ';'.code, '3'.code, ';'.code, '5'.code, 'M'.code),
            sent(MouseReport(MouseButton.LEFT, col = 2, row = 4, down = true)),
        )
        assertEquals(
            "the release",
            listOf(escape, '['.code, '<'.code, '0'.code, ';'.code, '3'.code, ';'.code, '5'.code, 'm'.code),
            sent(MouseReport(MouseButton.LEFT, col = 2, row = 4, down = false)),
        )
    }

    @Test
    fun `the first cell of the screen is one, not zero`() {
        // Both encodings count from one. A terminal that sent the zero-based cell would put
        // every click one cell up and to the left, which on a menu bar is the next entry.
        assertEquals(
            "the corner cell",
            listOf(escape, '['.code, '<'.code, '0'.code, ';'.code, '1'.code, ';'.code, '1'.code, 'M'.code),
            sent(MouseReport(MouseButton.LEFT, col = 0, row = 0, down = true)),
        )
    }

    @Test
    fun `the buttons keep the numbers the wire gives them`() {
        val cases = listOf(
            Triple("left", MouseButton.LEFT, '0'),
            // The wire's numbers are not the order the buttons are named in: the middle
            // button is 1 and the right one is 2.
            Triple("middle", MouseButton.MIDDLE, '1'),
            Triple("right", MouseButton.RIGHT, '2'),
        )

        for ((name, button, number) in cases) {
            assertEquals(
                "$name: the button's number",
                listOf(escape, '['.code, '<'.code, number.code, ';'.code, '7'.code, ';'.code, '9'.code, 'M'.code),
                sent(MouseReport(button, col = 6, row = 8, down = true)),
            )
        }
    }

    @Test
    fun `a cell past the two hundred and twenty third column is still sent in full`() {
        // The X10 form cannot express a column past 223: its coordinates are one byte with
        // 32 added, and the byte runs out. SGR writes them as numbers, which is why the
        // client asks for it on a screen this wide.
        assertEquals(
            "a column past the X10 limit",
            listOf(escape, '['.code, '<'.code, '0'.code, ';'.code) +
                "300;40M".map { it.code },
            sent(MouseReport(MouseButton.LEFT, col = 299, row = 39, down = true)),
        )
    }

    @Test
    fun `the x10 encoding packs the button and both cells into one byte each`() {
        // CSI M followed by three bytes, each carrying 32 plus its value, so the same cell
        // as the SGR vectors above is 3 and 5 here and reads as 35 and 37. A release has no
        // button of its own in this form: it is button three, which is how a terminal that
        // predates SGR says the button came up.
        assertEquals(
            "the press",
            listOf(escape, '['.code, 'M'.code, 32, 35, 37),
            sent(MouseReport(MouseButton.LEFT, col = 2, row = 4, down = true), sgr = false),
        )
        assertEquals(
            "the release",
            listOf(escape, '['.code, 'M'.code, 35, 35, 37),
            sent(MouseReport(MouseButton.LEFT, col = 2, row = 4, down = false), sgr = false),
        )
        assertEquals(
            "the right button",
            listOf(escape, '['.code, 'M'.code, 34, 35, 37),
            sent(MouseReport(MouseButton.RIGHT, col = 2, row = 4, down = true), sgr = false),
        )
    }

    @Test
    fun `the wheel is what a drag is sent as`() {
        // A finger cannot click a scrollbar, so a drag over the client's interface is the
        // wheel a trackpad would have turned at the cell the finger is on: what scrolls is
        // whatever the client has there — a page, a guide, a room's messages — which is the
        // only scrolling a full-screen client can be given from outside itself. The wire
        // gives the wheel no buttons of its own, and no release either: one report is one
        // notch.
        assertEquals(
            "the wheel up",
            listOf(escape, '['.code, '<'.code) + "64;3;5M".map { it.code },
            sent(MouseReport(MouseButton.WHEEL_UP, col = 2, row = 4, down = true)),
        )
        assertEquals(
            "the wheel down",
            listOf(escape, '['.code, '<'.code) + "65;3;5M".map { it.code },
            sent(MouseReport(MouseButton.WHEEL_DOWN, col = 2, row = 4, down = true)),
        )

        // The older form has a notch of its own too: 32 added to the wire's number, the same
        // way a button's is.
        assertEquals(
            "the wheel up, in the older form",
            listOf(escape, '['.code, 'M'.code, 96, 35, 37),
            sent(MouseReport(MouseButton.WHEEL_UP, col = 2, row = 4, down = true), sgr = false),
        )
        assertEquals(
            "the wheel down, in the older form",
            listOf(escape, '['.code, 'M'.code, 97, 35, 37),
            sent(MouseReport(MouseButton.WHEEL_DOWN, col = 2, row = 4, down = true), sgr = false),
        )
    }

    @Test
    fun `a cell is clamped into what the encoding can carry`() {
        // A finger cannot land outside the console, but the byte it is written into cannot
        // wrap either: an off-by-one here would be a sequence the client reads as a letter.
        val x10 = sent(MouseReport(MouseButton.LEFT, col = 9999, row = 9999, down = true), sgr = false)
        assertEquals("the tail of the sequence", listOf(255, 255), x10.takeLast(2))
    }
}
