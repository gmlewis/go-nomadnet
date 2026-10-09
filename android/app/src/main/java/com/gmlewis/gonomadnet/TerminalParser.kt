// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

/**
 * The emulated terminal's parser: it reads the byte stream the client produces and
 * drives a [TerminalScreen] with it.
 *
 * The appliance gives the client a pseudo-terminal, so what comes back over the console
 * socket is what a terminal emulator would see on its own input. This is that emulator's
 * reading half — the escape sequences, the UTF-8 decoder and the controls — and
 * [TerminalScreen] is its state.
 *
 * It follows the Go client's `tui/vterm.go`, which is what the same client draws through
 * on a desktop terminal, with these differences, each of them a sequence the appliance
 * needs read the way a terminal reads it:
 *
 * - **Strings are read.** Go's parser discards OSC entirely. Here the title (OSC 2) and
 *   the clipboard (OSC 52) are reported to the sinks it was built with, which is how a
 *   remote page gets its title onto the screen and its text onto the clipboard.
 * - **DEC special graphics are drawn.** `ESC ( 0` switches G0 to the line-drawing set, so
 *   a program that draws a box with `l q k m x` gets `┌ ─ ┐ └ │` rather than letters.
 * - **Wide characters are measured.** A double-width character takes two columns, held
 *   whole, as [TerminalScreen.putGlyph] describes.
 * - **The decoder belongs to the parser.** Go keeps its partial UTF-8 sequence in a
 *   package-level variable shared by every screen; this one holds at most four bytes of
 *   its own, which is what lets two screens be fed independently and keeps a malformed
 *   stream from growing anything. Go also leaves that accumulator holding a lead byte
 *   when the byte after it is plain ASCII, and prints the ASCII on past the rune it
 *   broke; here the broken rune is replaced and the byte after it is read as itself.
 * - **A sequence's intermediate bytes are read.** Go abandons a sequence at its
 *   intermediate, so the final byte of `CSI 1 SP q` prints as the letter q. Here the
 *   final is waited for and the sequence is ignored, as xterm does.
 * - **The cursor can be saved and restored** with `ESC 7` / `ESC 8` as well as `CSI s` /
 *   `CSI u`, which Go's parser does not implement; a screen that ignores a save is one
 *   where a program's borrowed cursor is left where the program dropped it.
 *
 * The stream is hostile by construction — it is whatever the program on the other end
 * printed — so no input reaches the screen without being bounded: the escape buffers are
 * capped, an over-long string is discarded rather than accumulated, and a parameter is
 * capped rather than allowed to overflow. [TerminalParserTest] fuzzes exactly that.
 */
class TerminalParser(
    private val screen: TerminalScreen,
    private val titleSink: (String) -> Unit = {},
    private val clipboardSink: (String) -> Unit = {},
) {

    /** State is where the parser is in the byte stream, byte by byte. */
    private enum class State {
        /** GROUND is ordinary text. */
        GROUND,

        /** ESC has read the escape byte and is waiting for what it introduces. */
        ESC,

        /** CSI is inside a control sequence. */
        CSI,

        /** OSC is inside a string. */
        OSC,

        /** OSC_ESC has seen the escape byte that may end a string. */
        OSC_ESC,

        /** CHARSET is waiting for the byte that designates a character set. */
        CHARSET,
    }

    private var state = State.GROUND
    private val csiBuf = StringBuilder()
    private val oscBuf = StringBuilder()
    private var oscOverflow = false
    private var priv = ' '
    private var csiIntermediate = false
    private var g0Special = false
    private var charsetG1 = false

    // The partial UTF-8 sequence. Four bytes is the longest one there is, so the buffer
    // cannot grow and a stream that never completes a rune costs four bytes and no more.
    private val utf8 = ByteArray(4)
    private var utf8Len = 0

    /** write feeds the parser a string, which tests use for readability. */
    fun write(text: String) {
        write(text.toByteArray(Charsets.UTF_8))
    }

    /** write feeds the parser the bytes a client printed. */
    fun write(data: ByteArray) {
        for (i in data.indices) {
            val b = data[i].toInt() and 0xff
            // Only in the ground state can a byte be text rather than a sequence: a
            // continuation byte inside a sequence is that sequence's, not a character.
            // A byte of an unfinished rune is the rune's for the same reason, even when
            // it is plain ASCII — a lead byte of 0xc3 followed by "B" is a broken rune
            // and a B, not a B with the lead silently kept.
            if (state == State.GROUND && (b >= 0x80 || utf8Len > 0)) {
                utf8Write(b)
                continue
            }
            feedByte(b)
        }
    }

    private fun feedByte(b: Int) {
        when (state) {
            State.GROUND -> groundByte(b)
            State.ESC -> escByte(b)
            State.CSI -> csiByte(b)
            State.OSC -> oscByte(b)
            State.OSC_ESC -> oscEscByte(b)
            State.CHARSET -> charsetByte(b)
        }
    }

    private fun groundByte(b: Int) {
        when (b) {
            0x1b -> state = State.ESC
            0x0d -> screen.carriageReturn()
            0x0a, 0x0b, 0x0c -> {
                screen.lineFeed()
                screen.cancelWrap()
            }

            0x08 -> screen.backspace()
            0x09 -> screen.tab()
            0x07 -> {} // a bell is not rung
            else -> {
                if (b < 0x20) {
                    return
                }
                val ch = if (g0Special && b in 0x60..0x7e) DEC_SPECIAL_GRAPHICS[b - 0x60] else b.toChar()
                screen.putGlyph(ch.toString(), 1)
            }
        }
    }

    private fun escByte(b: Int) {
        state = State.GROUND
        when (b) {
            '['.code -> {
                state = State.CSI
                csiBuf.setLength(0)
                priv = ' '
                csiIntermediate = false
            }

            ']'.code -> {
                state = State.OSC
                oscBuf.setLength(0)
                oscOverflow = false
            }

            'M'.code -> screen.reverseLineFeed()
            '('.code -> {
                state = State.CHARSET
                charsetG1 = false
            }

            ')'.code -> {
                state = State.CHARSET
                charsetG1 = true
            }

            '7'.code -> screen.saveCursor()
            '8'.code -> screen.restoreCursor()
        }
    }

    private fun csiByte(b: Int) {
        if (b == '?'.code && csiBuf.isEmpty()) {
            priv = '?'
            return
        }
        if (b in '0'.code..'9'.code || b == ';'.code || b == ':'.code) {
            // A parameter longer than the cap is dropped while the sequence is still
            // read to its end: the sequence must not be abandoned half-read, or its
            // final byte would print as text.
            if (csiBuf.length < MAX_CSI_LENGTH) {
                csiBuf.append(b.toChar())
            }
            return
        }
        if (b in 0x20..0x2f) {
            // An intermediate byte, as in `CSI 1 SP q` (a cursor style) or `CSI ! p` (a
            // terminal reset). It is not the end of the sequence: the final byte is
            // still to come, and the text after it must not be eaten by a final that
            // was never a final.
            csiIntermediate = true
            return
        }
        if (b in 0x40..0x7e) {
            dispatchCsi(b.toChar())
            state = State.GROUND
            return
        }
        // Anything else — a control byte in the middle of a sequence — ends it where it
        // stands, and the parameters are dropped with it.
        state = State.GROUND
    }

    private fun oscByte(b: Int) {
        when (b) {
            0x07 -> { // BEL ends it
                finishOsc()
                state = State.GROUND
            }

            0x1b -> state = State.OSC_ESC
            else -> {
                if (oscBuf.length < MAX_OSC_LENGTH) {
                    oscBuf.append(b.toChar())
                } else {
                    oscOverflow = true
                }
            }
        }
    }

    private fun oscEscByte(b: Int) {
        finishOsc()
        if (b == '\\'.code) { // the string terminator
            state = State.GROUND
            return
        }
        // The escape byte ended the string on its own, so what follows it starts a new
        // sequence rather than continuing the old string.
        state = State.ESC
        feedByte(b)
    }

    private fun charsetByte(b: Int) {
        // Only G0 is used: G1 would take a shift-out byte to select it, which nothing the
        // client emits does. An unknown designation means the ordinary ASCII set, as it
        // does in xterm.
        if (!charsetG1) {
            g0Special = b == '0'.code
        }
        state = State.GROUND
    }

    /**
     * finishOsc reports a complete string to the sink its code names: a title, or a
     * clipboard payload. A string the parser could not hold in full is dropped rather
     * than reported in part — half a title or half a payload is not what the client
     * asked for, and a truncated payload can still look like a well-formed one.
     */
    private fun finishOsc() {
        val text = oscBuf.toString()
        oscBuf.setLength(0)
        if (oscOverflow) {
            oscOverflow = false
            return
        }
        val separator = text.indexOf(';')
        if (separator < 0) {
            return
        }
        val code = text.substring(0, separator)
        val rest = text.substring(separator + 1)
        when (code) {
            "2" -> titleSink(rest)
            "52" -> {
                val second = rest.indexOf(';')
                if (second < 0) {
                    return
                }
                val payload = rest.substring(second + 1)
                // "?" is a query: it asks the terminal what the clipboard holds, which is
                // not something to put on the clipboard.
                if (payload.isEmpty() || payload == "?") {
                    return
                }
                val decoded = decodeBase64(payload) ?: return
                clipboardSink(decoded)
            }
        }
    }

    /** decodeBase64 decodes a clipboard payload, or returns null if it is not base64. */
    private fun decodeBase64(s: String): String? {
        if (s.isEmpty() || s.length % 4 != 0) {
            return null
        }
        val out = ByteArray(s.length / 4 * 3)
        var outLen = 0
        var i = 0
        while (i < s.length) {
            val chunk = s.substring(i, i + 4)
            val padding = when {
                chunk[3] == '=' && chunk[2] == '=' -> 2
                chunk[3] == '=' -> 1
                else -> 0
            }
            var value = 0
            for (j in 0 until 4) {
                val c = chunk[j]
                val digit = when {
                    c == '=' && j >= 4 - padding -> 0
                    c in 'A'..'Z' -> c - 'A'
                    c in 'a'..'z' -> c - 'a' + 26
                    c in '0'..'9' -> c - '0' + 52
                    c == '+' -> 62
                    c == '/' -> 63
                    else -> return null
                }
                value = (value shl 6) or digit
            }
            out[outLen++] = ((value shr 16) and 0xff).toByte()
            if (padding < 2) {
                out[outLen++] = ((value shr 8) and 0xff).toByte()
            }
            if (padding < 1) {
                out[outLen++] = (value and 0xff).toByte()
            }
            i += 4
        }
        return String(out, 0, outLen, Charsets.UTF_8)
    }

    private fun dispatchCsi(final: Char) {
        // Every sequence ends a deferred wrap, which is how vterm.go's dispatchCSI opens.
        screen.cancelWrap()
        val intermediate = csiIntermediate
        csiIntermediate = false
        if (intermediate) {
            // An intermediate byte makes the final part of a different, private sequence:
            // `CSI 1 SP q` is a cursor style, not a cursor move. Those are ignored rather
            // than risk reading a final as the sequence it would be without it.
            return
        }
        when (final) {
            'A' -> screen.moveCursor(screen.cursorX, screen.cursorY - param(0, 1))
            'B' -> screen.moveCursor(screen.cursorX, screen.cursorY + param(0, 1))
            'C' -> screen.moveCursor(screen.cursorX + param(0, 1), screen.cursorY)
            'D' -> screen.moveCursor(screen.cursorX - param(0, 1), screen.cursorY)
            'E' -> screen.moveCursor(0, screen.cursorY + param(0, 1))
            'F' -> screen.moveCursor(0, screen.cursorY - param(0, 1))
            'G', '`' -> screen.moveCursor(param(0, 1) - 1, screen.cursorY)
            'd' -> screen.moveCursor(screen.cursorX, param(0, 1) - 1)
            'H', 'f' -> screen.moveCursor(param(1, 1) - 1, param(0, 1) - 1)
            'J' -> screen.eraseDisplay(param(0, 0))
            'K' -> screen.eraseLine(param(0, 0))
            'S' -> screen.scrollUp(param(0, 1))
            'T' -> screen.scrollDown(param(0, 1))
            'm' -> screen.applySgr(splitParams(csiBuf.toString()))
            'r' -> screen.setScrollRegion(param(0, 1) - 1, param(1, screen.rows) - 1)
            'h' -> setMode(true)
            'l' -> setMode(false)
            'L' -> screen.insertLines(param(0, 1))
            'M' -> screen.deleteLines(param(0, 1))
            'P' -> screen.deleteChars(param(0, 1))
            '@' -> screen.insertChars(param(0, 1))
            'X' -> screen.eraseChars(param(0, 1))
            's' -> screen.saveCursor()
            'u' -> screen.restoreCursor()
        }
    }

    /** setMode applies the private modes (`CSI ? n h` and `CSI ? n l`) vterm.go knows. */
    private fun setMode(on: Boolean) {
        if (priv != '?') {
            return
        }
        when (param(0, 0)) {
            25 -> screen.setCursorVisible(on)
            1049 -> screen.setAltScreen(on, true) // save the cursor on entry, restore it on exit
            47, 1047 -> screen.setAltScreen(on, false) // the legacy switches keep the cursor
            1000, 1002, 1003 -> screen.setMouseReporting(on)
            1006 -> screen.setMouseSgr(on)
            2004 -> {} // bracketed paste changes what the child receives, not the screen
        }
    }

    /** param reads one numeric parameter of the sequence in flight, or [def] if it has none. */
    private fun param(i: Int, def: Int): Int {
        val parts = splitParams(csiBuf.toString())
        if (i >= parts.size) {
            return def
        }
        return csiIntOrNull(parts[i]) ?: def
    }

    /** utf8Write adds one byte to the rune being decoded, and prints it when it is complete. */
    private fun utf8Write(b: Int) {
        if (utf8Len == 0) {
            if (b < 0x80) {
                // Plain ASCII, which is only read here when a broken sequence was
                // abandoned just before it.
                screen.putGlyph(b.toChar().toString(), 1)
                return
            }
            val need = utf8Length(b)
            if (need == 0) {
                // A byte that cannot begin a rune is printed as the replacement
                // character, and only itself is consumed: the bytes around it are still
                // the stream's own.
                screen.putGlyph(REPLACEMENT, 1)
                return
            }
            utf8[utf8Len++] = b.toByte()
            return
        }
        if (b and 0xc0 != 0x80) {
            // The rune was never finished. The replacement stands for the lead byte, and
            // this byte is read again as what it is.
            utf8Len = 0
            screen.putGlyph(REPLACEMENT, 1)
            utf8Write(b)
            return
        }
        utf8[utf8Len++] = b.toByte()
        val need = utf8Length(utf8[0].toInt() and 0xff)
        if (utf8Len < need) {
            return
        }
        val codePoint = decodeUtf8(utf8, need)
        utf8Len = 0
        if (codePoint < 0) {
            // Structurally sound but not a character: an overlong form, a surrogate, or
            // a code point past the last one. It is replaced as a whole.
            screen.putGlyph(REPLACEMENT, 1)
            return
        }
        screen.putGlyph(String(Character.toChars(codePoint)), displayWidth(codePoint))
    }

    /** utf8Length is how many bytes a lead byte begins, or 0 when it begins nothing. */
    private fun utf8Length(lead: Int): Int = when (lead) {
        in 0xc2..0xdf -> 2 // 0xc0 and 0xc1 would only spell an overlong form
        in 0xe0..0xef -> 3
        in 0xf0..0xf4 -> 4 // 0xf5 and above are past the last code point
        else -> 0
    }

    /**
     * decodeUtf8 assembles a complete sequence, or returns -1 if it is not a character.
     * The structure has already been checked, so what is left is whether the value it
     * spells is one: the shortest form, no surrogate half, and no code point past the
     * last one there is.
     */
    private fun decodeUtf8(bytes: ByteArray, length: Int): Int {
        var codePoint = when (length) {
            1 -> bytes[0].toInt() and 0x7f
            2 -> bytes[0].toInt() and 0x1f
            3 -> bytes[0].toInt() and 0x0f
            else -> bytes[0].toInt() and 0x07
        }
        for (i in 1 until length) {
            codePoint = (codePoint shl 6) or (bytes[i].toInt() and 0x3f)
        }
        val shortest = when (length) {
            1 -> 0x00
            2 -> 0x80
            3 -> 0x800
            else -> 0x10000
        }
        if (codePoint < shortest || codePoint > 0x10ffff) {
            return -1
        }
        if (codePoint in 0xd800..0xdfff) {
            return -1
        }
        return codePoint
    }

    companion object {
        /** REPLACEMENT is the character a byte that is not part of one is shown as. */
        private const val REPLACEMENT = "�"

        /** MAX_CSI_LENGTH is how much of a control sequence's parameters is kept. */
        private const val MAX_CSI_LENGTH = 256

        /** MAX_OSC_LENGTH is how much of a string is kept before it is discarded. */
        private const val MAX_OSC_LENGTH = 8192
    }
}

/**
 * splitParams splits a control sequence's parameters on their semicolons, keeping the
 * empty ones, as Go's `strings.Split` does: `1;` is two parameters, the second of which
 * is absent, and a defaulting rule that cannot tell the difference reads a cursor row of
 * "the last one" where the client said "the first".
 */
internal fun splitParams(buf: String): List<String> {
    val parts = mutableListOf<String>()
    var start = 0
    for (i in buf.indices) {
        if (buf[i] == ';') {
            parts += buf.substring(start, i)
            start = i + 1
        }
    }
    parts += buf.substring(start)
    return parts
}

/**
 * displayWidth is how many columns a character occupies: two for the wide ones, one for
 * the rest.
 *
 * The ranges are the East Asian Wide and Fullwidth ones, which is what a CJK page, a
 * fullwidth form and most emoji are drawn from. A combining mark is measured as one
 * column rather than zero, which renders it beside its base character instead of over
 * it — a page that uses them is legible either way, and this is the one place the
 * emulator knowingly differs from a desktop terminal.
 */
internal fun displayWidth(codePoint: Int): Int = when (codePoint) {
    in 0x1100..0x115f, // Hangul Jamo
    in 0x2e80..0x303e, // CJK radicals, Kangxi radicals, CJK symbols
    in 0x3041..0x33ff, // Hiragana, Katakana, Bopomofo, Hangul compatibility, CJK compatibility
    in 0x3400..0x4dbf, // CJK unified ideographs extension A
    in 0x4e00..0x9fff, // CJK unified ideographs
    in 0xa000..0xa4cf, // Yi syllables and radicals
    in 0xac00..0xd7a3, // Hangul syllables
    in 0xf900..0xfaff, // CJK compatibility ideographs
    in 0xfe10..0xfe19, // vertical forms
    in 0xfe30..0xfe6f, // CJK compatibility forms, small form variants
    in 0xff00..0xff60, // fullwidth forms
    in 0xffe0..0xffe6, // fullwidth signs
    in 0x1f300..0x1f64f, // pictographs and emoticons
    in 0x1f680..0x1f6ff, // transport and map symbols
    in 0x1f900..0x1f9ff, // supplemental symbols
    in 0x20000..0x3fffd, // CJK unified ideographs extensions B and beyond
    -> 2

    else -> 1
}

/**
 * DEC_SPECIAL_GRAPHICS is the line-drawing set a client selects with `ESC ( 0`, indexed
 * from 0x60: the byte that would have printed a letter prints a piece of a box instead,
 * which is how a program draws a border without a Unicode font and a UTF-8 stream.
 */
private const val DEC_SPECIAL_GRAPHICS = "◆▒␉␌␍␊°±" +
    "␤␋┘┐┌└┼⎺⎻─⎼⎽" +
    "├┤┴┬│≤≥π≠£·"
