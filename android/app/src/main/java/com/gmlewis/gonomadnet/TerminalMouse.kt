// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

/**
 * A mouse button, as far as this terminal can report one.
 *
 * A finger has no buttons and a trackpad's are its own business, so the left button is the
 * one the console sends: it is the one that selects and activates, and the middle and right
 * buttons are here because the wire has numbers for them and a terminal that named only one
 * button would be a terminal that could not spell the other two.
 */
enum class MouseButton {
    /** LEFT is the primary button, which is what a tap is. */
    LEFT,

    /** MIDDLE is the middle button of a three-button mouse. */
    MIDDLE,

    /** RIGHT is the secondary button. */
    RIGHT,

    /**
     * WHEEL_UP and WHEEL_DOWN are the wheel, which is what a drag is.
     *
     * A finger cannot click a scrollbar, so a drag over the client's interface is sent as
     * the wheel a trackpad would have turned at the cell the finger is on. What scrolls is
     * then whatever the client has there — a page, a guide, a room's messages — which is the
     * only scrolling a full-screen client can be given from outside.
     */
    WHEEL_UP,
    WHEEL_DOWN,
}

/**
 * One mouse event, at a cell of the client's screen.
 *
 * The cell is the client's own, counted from the top left corner of the terminal it is
 * drawing on and zero-based, which is how every other cell in the emulator is counted. Both
 * encodings below carry them one-based, and that is [TerminalMouse]'s business rather than
 * this one's.
 */
data class MouseReport(val button: MouseButton, val col: Int, val row: Int, val down: Boolean)

/**
 * TerminalMouse spells a mouse event the way a terminal sends one.
 *
 * The client is a terminal program reading a pseudo-terminal, so a tap has to reach it as
 * the byte sequence a real terminal writes when its mouse is clicked — there is no other way
 * to tell it that its list was touched. Which sequence that is, the client says itself: it
 * turns mouse reporting on with `CSI ?1000h` (`?1002h` and `?1003h` as well, for motion) and
 * asks for the SGR encoding on top of it with `CSI ?1006h`. That last one is the difference
 * between the two forms below, and it is not a detail: the older X10 form packs a cell into
 * one byte, so it cannot report a column past 223, which is a third of the way across this
 * tablet's screen.
 *
 * [TerminalParser] is what reads those mode settings into [TerminalScreen].
 */
object TerminalMouse {

    /** RELEASE_BUTTON is how the X10 form says a button came up. */
    private const val RELEASE_BUTTON = 3

    /** OFFSET is what both encodings add to a cell to make it printable and unambiguous. */
    private const val OFFSET = 32

    /**
     * encode is the bytes [report] is sent as, in the encoding the client asked for.
     *
     * A report is one event and not a click: the client makes a click out of a press and a
     * release in the same cell, so a caller that sends only the press has moved the client's
     * focus and activated nothing.
     */
    fun encode(report: MouseReport, sgr: Boolean): ByteArray =
        if (sgr) encodeSgr(report) else encodeX10(report)

    /**
     * encodeSgr is the SGR form: `CSI < button ; column ; row M`, with a lower-case `m` in
     * place of the `M` when the button came up.
     *
     * The numbers are decimal and are what the client's own terminal library reads, so a cell
     * of any size can be reported.
     */
    private fun encodeSgr(report: MouseReport): ByteArray {
        val end = if (report.down) 'M' else 'm'
        val body = "<${buttonNumber(report.button)};${report.col + 1};${report.row + 1}$end"
        return byteArrayOf(0x1b, '['.code.toByte()) + body.toByteArray(Charsets.US_ASCII)
    }

    /**
     * encodeX10 is the older form: `CSI M` followed by the button, the column and the row,
     * each as one byte carrying [OFFSET] plus its value.
     *
     * A button that has come up has no number of its own here — the form predates the idea —
     * so it is button three. A cell that does not fit in a byte is clamped rather than
     * wrapped: a wrapped coordinate is a different cell, which is a click somewhere the
     * reader did not touch.
     */
    private fun encodeX10(report: MouseReport): ByteArray {
        val button = if (report.down) buttonNumber(report.button) else RELEASE_BUTTON
        return byteArrayOf(
            0x1b,
            '['.code.toByte(),
            'M'.code.toByte(),
            packed(button),
            packed(report.col + 1),
            packed(report.row + 1),
        )
    }

    /** buttonNumber is the number the wire gives each button, which is not the order above. */
    private fun buttonNumber(button: MouseButton): Int = when (button) {
        MouseButton.LEFT -> 0
        MouseButton.MIDDLE -> 1
        MouseButton.RIGHT -> 2
        // The wheel has no buttons of its own; the wire gives it 64 and 65, which is what a
        // terminal sends when its wheel is turned a notch.
        MouseButton.WHEEL_UP -> 64
        MouseButton.WHEEL_DOWN -> 65
    }

    /** packed is one X10 value as a byte. */
    private fun packed(value: Int): Byte = (value + OFFSET).coerceIn(0, 0xff).toByte()
}
