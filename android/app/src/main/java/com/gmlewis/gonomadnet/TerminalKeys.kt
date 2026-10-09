// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

/**
 * SpecialKey names the keys that are not characters.
 *
 * They are the vocabulary the appliance can report a press in; which of them the terminal
 * can actually send is [TerminalKeys]'s business, and the ones it cannot send are named
 * here anyway so that the view has something to report them as rather than quietly
 * dropping them.
 */
enum class SpecialKey {
    /** ENTER is the return key, which sends a carriage return. */
    ENTER,

    /** TAB is the tab key. */
    TAB,

    /** BACKSPACE is the backspace key, which sends DEL rather than BS. */
    BACKSPACE,

    /** ESCAPE is the escape key. */
    ESCAPE,

    /** UP, DOWN, LEFT and RIGHT are the arrow keys. */
    UP,
    DOWN,
    LEFT,
    RIGHT,

    /** HOME and END are the start and the end of the line. */
    HOME,
    END,

    /** PAGE_UP and PAGE_DOWN scroll a page. */
    PAGE_UP,
    PAGE_DOWN,

    /** DELETE and INSERT delete and insert at the cursor. */
    DELETE,
    INSERT,

    /** F1 and F2 are function keys, which the terminal has no sequence for. */
    F1,
    F2,
}

/**
 * TerminalKey is one key press, as much of it as the terminal needs: a character, a
 * control code, or one of the named keys.
 */
sealed interface TerminalKey {
    /** Rune is a printable character, or several when an input method commits a word. */
    data class Rune(val text: String) : TerminalKey

    /** Ctrl is a letter held with the control key, which sends the letter's control code. */
    data class Ctrl(val letter: Char) : TerminalKey

    /** Special is one of the keys that are not characters. */
    data class Special(val key: SpecialKey) : TerminalKey
}

/**
 * TerminalKeys turns a key press into the bytes a terminal would have written for it.
 *
 * The client reads a pseudo-terminal, not a keyboard: what it understands is the byte
 * stream a real terminal produces, so a key press has to be spelled the way that terminal
 * would have spelled it. `tui/embedded-terminal.go`'s `keyToANSI` is the same table on
 * the Go side, and the two agree on the two points where a keyboard and a terminal
 * disagree — backspace is DEL, and the arrows are the CSI forms.
 *
 * [toBytes] returns null for a key that means nothing to the terminal. That is not the
 * same as an empty array: an empty write is still a write, and a caller that sent one
 * would be telling the client something happened.
 */
object TerminalKeys {

    /** toBytes is the bytes [key] sends, or null when it sends nothing. */
    fun toBytes(key: TerminalKey): ByteArray? = when (key) {
        is TerminalKey.Ctrl -> ctrlByte(key.letter)
        is TerminalKey.Rune -> if (key.text.isEmpty()) null else key.text.toByteArray(Charsets.UTF_8)
        is TerminalKey.Special -> SEQUENCES[key.key]
    }

    /**
     * ctrlByte is the control code a letter's control key sends: ctrl-A is 0x01 through
     * ctrl-Z at 0x1a, which is the letter's place in the alphabet.
     *
     * The case of the letter is not part of the key — a layout reports whichever it
     * reports — so both are accepted. Only letters have codes; anything else under the
     * control key means nothing to the terminal.
     */
    private fun ctrlByte(letter: Char): ByteArray? {
        val upper = letter.uppercaseChar()
        if (upper !in 'A'..'Z') {
            return null
        }
        return byteArrayOf((upper - 'A' + 1).toByte())
    }

    /** SEQUENCES is what each named key sends. A key that is absent sends nothing. */
    private val SEQUENCES: Map<SpecialKey, ByteArray> = buildMap {
        put(SpecialKey.ENTER, byteArrayOf(0x0d)) // a carriage return, not a line feed
        put(SpecialKey.TAB, byteArrayOf(0x09))
        // Backspace sends DEL: the client is an editor's descendant and reads 0x7f, which
        // is what a terminal sends for the key, and what the Go side sends as well.
        put(SpecialKey.BACKSPACE, byteArrayOf(0x7f))
        put(SpecialKey.ESCAPE, byteArrayOf(0x1b))
        put(SpecialKey.UP, csi("A"))
        put(SpecialKey.DOWN, csi("B"))
        put(SpecialKey.RIGHT, csi("C"))
        put(SpecialKey.LEFT, csi("D"))
        put(SpecialKey.HOME, csi("H"))
        put(SpecialKey.END, csi("F"))
        // The four keys a real terminal spells with a number and a tilde.
        put(SpecialKey.PAGE_UP, csi("5~"))
        put(SpecialKey.PAGE_DOWN, csi("6~"))
        put(SpecialKey.DELETE, csi("3~"))
        put(SpecialKey.INSERT, csi("2~"))
        // SpecialKey.F1 and F2 are absent on purpose: nothing in the client's interface
        // reads a function key, and neither does the Go table.
    }

    /** csi spells a control sequence: the escape byte, the bracket, and the body. */
    private fun csi(body: String): ByteArray =
        byteArrayOf(0x1b, '['.code.toByte()) + body.toByteArray(Charsets.US_ASCII)
}
