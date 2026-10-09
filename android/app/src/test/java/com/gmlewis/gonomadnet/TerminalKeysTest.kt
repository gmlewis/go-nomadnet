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
 * What the appliance sends the client when a key is pressed.
 *
 * The client runs on a pseudo-terminal, so a key is not an event it can be handed: it is
 * the bytes a real terminal would have written, and this is the table that turns one into
 * the other. `tui/embedded-terminal.go`'s `keyToANSI` is the same table in Go, and the
 * vectors below are readable against it — including the two that a terminal does not
 * agree with a keyboard about: backspace sends DEL (0x7f) rather than BS (0x08), and the
 * arrow keys send the CSI forms rather than the SS3 ones.
 *
 * A key that has no bytes sends nothing at all, which is not the same as sending an empty
 * write: the caller must be able to tell "this key means nothing to the terminal" from
 * "this key means an empty string", and one is null while the other is an empty array.
 */
class TerminalKeysTest {

    /** sent renders what a key sends, as integers, or null when it sends nothing. */
    private fun sent(key: TerminalKey): List<Int>? =
        TerminalKeys.toBytes(key)?.map { it.toInt() and 0xff }

    @Test
    fun `ctrl and a letter sends its control code`() {
        val cases = listOf(
            KeyCase("ctrl-a", TerminalKey.Ctrl('A'), listOf(0x01)),
            KeyCase("ctrl-c", TerminalKey.Ctrl('C'), listOf(0x03)),
            KeyCase("ctrl-i", TerminalKey.Ctrl('I'), listOf(0x09)),
            KeyCase("ctrl-m", TerminalKey.Ctrl('M'), listOf(0x0d)),
            KeyCase("ctrl-z", TerminalKey.Ctrl('Z'), listOf(0x1a)),
        )

        for (case in cases) {
            assertEquals("${case.name}: the control code", case.want, sent(case.key))
            // A keyboard reports the letter in whichever case its layout has, and the
            // control code is the same either way.
            val letter = (case.key as TerminalKey.Ctrl).letter.lowercaseChar()
            assertEquals("${case.name}: lower case", case.want, sent(TerminalKey.Ctrl(letter)))
        }

        // Only the alphabet has control codes: ctrl-1 and ctrl-[ are not ctrl-a.
        assertEquals("ctrl-1", null, sent(TerminalKey.Ctrl('1')))
        assertEquals("ctrl-[", null, sent(TerminalKey.Ctrl('[')))
        assertEquals("ctrl-space", null, sent(TerminalKey.Ctrl(' ')))
    }

    @Test
    fun `the arrow keys send the csi cursor sequences`() {
        val cases = listOf(
            KeyCase("up", TerminalKey.Special(SpecialKey.UP), listOf(0x1b, '['.code, 'A'.code)),
            KeyCase("down", TerminalKey.Special(SpecialKey.DOWN), listOf(0x1b, '['.code, 'B'.code)),
            KeyCase("right", TerminalKey.Special(SpecialKey.RIGHT), listOf(0x1b, '['.code, 'C'.code)),
            KeyCase("left", TerminalKey.Special(SpecialKey.LEFT), listOf(0x1b, '['.code, 'D'.code)),
        )

        for (case in cases) {
            assertEquals("${case.name}: the cursor sequence", case.want, sent(case.key))
        }
    }

    @Test
    fun `home end page up and page down send their sequences`() {
        val cases = listOf(
            KeyCase("home", TerminalKey.Special(SpecialKey.HOME), listOf(0x1b, '['.code, 'H'.code)),
            KeyCase("end", TerminalKey.Special(SpecialKey.END), listOf(0x1b, '['.code, 'F'.code)),
            KeyCase("page up", TerminalKey.Special(SpecialKey.PAGE_UP), listOf(0x1b, '['.code, '5'.code, '~'.code)),
            KeyCase("page down", TerminalKey.Special(SpecialKey.PAGE_DOWN), listOf(0x1b, '['.code, '6'.code, '~'.code)),
            // The two editing keys share the tilde form the two above use.
            KeyCase("delete", TerminalKey.Special(SpecialKey.DELETE), listOf(0x1b, '['.code, '3'.code, '~'.code)),
            KeyCase("insert", TerminalKey.Special(SpecialKey.INSERT), listOf(0x1b, '['.code, '2'.code, '~'.code)),
        )

        for (case in cases) {
            assertEquals("${case.name}: the sequence", case.want, sent(case.key))
        }
    }

    @Test
    fun `escape tab backspace and enter send their bytes`() {
        val cases = listOf(
            KeyCase("escape", TerminalKey.Special(SpecialKey.ESCAPE), listOf(0x1b)),
            KeyCase("tab", TerminalKey.Special(SpecialKey.TAB), listOf(0x09)),
            // Backspace is DEL, not BS: the client is an editor and reads 0x7f, which is
            // what a terminal sends and what the Go side sends too.
            KeyCase("backspace", TerminalKey.Special(SpecialKey.BACKSPACE), listOf(0x7f)),
            KeyCase("enter", TerminalKey.Special(SpecialKey.ENTER), listOf(0x0d)),
        )

        for (case in cases) {
            assertEquals("${case.name}: the byte", case.want, sent(case.key))
        }
    }

    @Test
    fun `a plain character sends its utf-8 bytes`() {
        val cases = listOf(
            KeyCase("a letter", TerminalKey.Rune("a"), listOf(0x61)),
            KeyCase("a digit", TerminalKey.Rune("1"), listOf(0x31)),
            KeyCase("punctuation", TerminalKey.Rune("/"), listOf(0x2f)),
            KeyCase("an accented letter", TerminalKey.Rune("é"), listOf(0xc3, 0xa9)),
            KeyCase("a wide character", TerminalKey.Rune("世"), listOf(0xe4, 0xb8, 0x96)),
            KeyCase(
                "a character outside the bmp",
                TerminalKey.Rune("😀"),
                listOf(0xf0, 0x9f, 0x98, 0x80),
            ),
            // An input method commits a word at a time, so a "character" can be several.
            KeyCase("a committed word", TerminalKey.Rune("hi"), listOf(0x68, 0x69)),
        )

        for (case in cases) {
            assertEquals("${case.name}: the bytes", case.want, sent(case.key))
        }
    }

    @Test
    fun `an alt combination is an escape in front of the key`() {
        // A terminal has no alt byte to send. What it writes is the key's own bytes with an
        // escape before them — `xterm`'s metaSendsEscape — and the client reads that as the
        // meta combination, which is how alt reaches it from a keyboard that cannot hold alt
        // down. The escape is the prefix and nothing else about the key changes.
        val cases = listOf(
            KeyCase("alt and a letter", TerminalKey.Alt(TerminalKey.Rune("x")), listOf(0x1b, 0x78)),
            KeyCase(
                "alt and an arrow",
                TerminalKey.Alt(TerminalKey.Special(SpecialKey.UP)),
                listOf(0x1b) + listOf(0x1b, 0x5b, 0x41),
            ),
            KeyCase("alt and escape", TerminalKey.Alt(TerminalKey.Special(SpecialKey.ESCAPE)), listOf(0x1b, 0x1b)),
            KeyCase(
                "alt over control",
                TerminalKey.Alt(TerminalKey.Ctrl('c')),
                listOf(0x1b, 0x03),
            ),
        )

        for (case in cases) {
            assertEquals("${case.name}: the bytes", case.want, sent(case.key))
        }

        // Alt over a key that sends nothing sends nothing: a prefix with no key behind it is
        // not a key press, and a caller that sent one would be telling the client something
        // happened.
        assertEquals(null, sent(TerminalKey.Alt(TerminalKey.Special(SpecialKey.F1))))
        assertEquals(null, sent(TerminalKey.Alt(TerminalKey.Rune(""))))
    }

    @Test
    fun `an unmapped key sends nothing`() {
        // The terminal's own vocabulary has no function keys: nothing in the client's
        // interface reads one, and a hardware keyboard's F1 means nothing to it.
        val cases = listOf(
            KeyCase("f1", TerminalKey.Special(SpecialKey.F1), emptyList()),
            KeyCase("f2", TerminalKey.Special(SpecialKey.F2), emptyList()),
            KeyCase("an empty character", TerminalKey.Rune(""), emptyList()),
            KeyCase("ctrl-1", TerminalKey.Ctrl('1'), emptyList()),
        )

        for (case in cases) {
            assertEquals("${case.name}: nothing is sent", null, sent(case.key))
        }
    }

    /** KeyCase is one key and the bytes it must send, as integers so a failure reads. */
    private data class KeyCase(val name: String, val key: TerminalKey, val want: List<Int>)
}
