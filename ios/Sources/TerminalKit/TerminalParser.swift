// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

import Foundation

/// The emulated terminal's parser: it reads the byte stream the client produces and drives
/// a ``TerminalScreen`` with it.
///
/// The appliance gives the client a pseudo-terminal, so what comes back over the console
/// socket is what a terminal emulator would see on its own input. This is that emulator's
/// reading half — the escape sequences, the UTF-8 decoder and the controls — and
/// ``TerminalScreen`` is its state.
///
/// It follows the Go client's `tui/vterm.go`, which is what the same client draws through on
/// a desktop terminal, with these differences, each of them a sequence the appliance needs
/// read the way a terminal reads it:
///
/// - **Strings are read.** Go's parser discards OSC entirely. Here the title (OSC 2) and
///   the clipboard (OSC 52) are reported to the sinks it was built with, which is how a
///   remote page gets its title onto the screen and its text onto the clipboard.
/// - **DEC special graphics are drawn.** `ESC ( 0` switches G0 to the line-drawing set, so a
///   program that draws a box with `l q k m x` gets `┌ ─ ┐ └ │` rather than letters.
/// - **Wide characters are measured.** A double-width character takes two columns, held
///   whole, as ``TerminalScreen/putGlyph(_:width:)`` describes.
/// - **The decoder belongs to the parser.** Go keeps its partial UTF-8 sequence in a
///   package-level variable shared by every screen; this one holds at most four bytes of its
///   own, which is what lets two screens be fed independently and keeps a malformed stream
///   from growing anything. Go also leaves that accumulator holding a lead byte when the byte
///   after it is plain ASCII, and prints the ASCII on past the rune it broke; here the broken
///   rune is replaced and the byte after it is read as itself.
/// - **A sequence's intermediate bytes are read.** Go abandons a sequence at its intermediate,
///   so the final byte of `CSI 1 SP q` prints as the letter q. Here the final is waited for
///   and the sequence is ignored, as xterm does.
/// - **The cursor can be saved and restored** with `ESC 7` / `ESC 8` as well as `CSI s` /
///   `CSI u`, which Go's parser does not implement; a screen that ignores a save is one where
///   a program's borrowed cursor is left where the program dropped it.
///
/// The stream is hostile by construction — it is whatever the program on the other end
/// printed — so no input reaches the screen without being bounded: the escape buffers are
/// capped, an over-long string is discarded rather than accumulated, and a parameter is
/// capped rather than allowed to overflow. `TerminalParserTests` fuzzes exactly that.
///
/// The parser is a class because its state is the point: it persists across the writes, so a
/// sequence split between two reads off the socket is still one sequence.
public final class TerminalParser {

    /// Where the parser is in the byte stream, byte by byte.
    private enum State {
        /// Ordinary text.
        case ground

        /// The escape byte has been read; what it introduces is next.
        case escape

        /// Inside a control sequence.
        case csi

        /// Inside a string.
        case osc

        /// The escape byte that may end a string has been seen.
        case oscEscape

        /// Waiting for the byte that designates a character set.
        case charset
    }

    /// The screen this parser draws on.
    private let screen: TerminalScreen

    /// Reports the title the client set with `OSC 2`, and says why.
    ///
    /// A remote page's title is its own, and the appliance is the terminal it is shown on, so
    /// the string is handed out rather than kept: only the view knows what to put it on. It is
    /// called on the queue feeding the parser, which for the appliance is the one reading the
    /// console socket.
    public var titleSink: ((String) -> Void)?

    /// Reports the text the client put on the clipboard with `OSC 52`, already decoded.
    ///
    /// A NomadNet page can copy with a link, and the terminal it is talking to is this one, so
    /// the payload is handed to the clipboard rather than dropped. It is called on the queue
    /// feeding the parser.
    public var clipboardSink: ((String) -> Void)?

    private var state = State.ground
    private var csiBuffer = ""
    private var oscBuffer = ""
    private var oscOverflow = false
    private var privateMarker: Character = " "
    private var csiIntermediate = false
    private var g0Special = false
    private var charsetG1 = false

    /// The partial UTF-8 sequence. Four bytes is the longest one there is, so the buffer
    /// cannot grow and a stream that never completes a rune costs four bytes and no more.
    private var utf8: [UInt8] = []

    /// A parser that draws on `screen`.
    public init(screen: TerminalScreen) {
        self.screen = screen
    }

    /// Feeds the parser the bytes a client printed.
    ///
    /// One call is one chunk off the socket, which is not necessarily one sequence: the state
    /// is the parser's, so a sequence split between two writes is completed by the second.
    public func parse(_ bytes: [UInt8]) {
        for b in bytes {
            // Only in the ground state can a byte be text rather than a sequence: a
            // continuation byte inside a sequence is that sequence's, not a character. A byte
            // of an unfinished rune is the rune's for the same reason, even when it is plain
            // ASCII — a lead byte of 0xc3 followed by "B" is a broken rune and a B, not a B
            // with the lead silently kept.
            if state == .ground && (b >= 0x80 || !utf8.isEmpty) {
                writeUtf8(b)
                continue
            }
            feed(b)
        }
    }

    // MARK: - The state machine

    /// Handles one byte in the state the parser is in.
    private func feed(_ b: UInt8) {
        switch state {
        case .ground: ground(b)
        case .escape: escape(b)
        case .csi: csi(b)
        case .osc: osc(b)
        case .oscEscape: oscEscape(b)
        case .charset: charset(b)
        }
    }

    /// Handles one byte of ordinary text.
    private func ground(_ b: UInt8) {
        switch b {
        case 0x1b:
            state = .escape
        case 0x0d:
            screen.carriageReturn()
        case 0x0a, 0x0b, 0x0c:
            screen.lineFeed()
            screen.cancelWrap()
        case 0x08:
            screen.backspace()
        case 0x09:
            screen.tab()
        case 0x07:
            break // a bell is not rung
        default:
            guard b >= 0x20 else { return }
            if g0Special, b >= 0x60, b <= 0x7e {
                screen.putGlyph(String(decSpecialGraphics[Int(b) - 0x60]), width: 1)
            } else {
                screen.putGlyph(String(decoding: [b], as: UTF8.self), width: 1)
            }
        }
    }

    /// Handles the byte that follows the escape byte.
    private func escape(_ b: UInt8) {
        state = .ground
        switch b {
        case UInt8(ascii: "["):
            state = .csi
            csiBuffer = ""
            privateMarker = " "
            csiIntermediate = false
        case UInt8(ascii: "]"):
            state = .osc
            oscBuffer = ""
            oscOverflow = false
        case UInt8(ascii: "M"):
            screen.reverseLineFeed()
        case UInt8(ascii: "("):
            state = .charset
            charsetG1 = false
        case UInt8(ascii: ")"):
            state = .charset
            charsetG1 = true
        case UInt8(ascii: "7"):
            screen.saveCursor()
        case UInt8(ascii: "8"):
            screen.restoreCursor()
        default:
            break
        }
    }

    /// Handles one byte of a control sequence: its marker, its parameters and its final.
    private func csi(_ b: UInt8) {
        if b == UInt8(ascii: "?"), csiBuffer.isEmpty {
            privateMarker = "?"
            return
        }
        if (b >= UInt8(ascii: "0") && b <= UInt8(ascii: "9")) || b == UInt8(ascii: ";")
            || b == UInt8(ascii: ":") {
            // A parameter longer than the cap is dropped while the sequence is still read to
            // its end: the sequence must not be abandoned half-read, or its final byte would
            // print as text.
            if csiBuffer.utf8.count < Self.maxCSI {
                csiBuffer.append(Character(UnicodeScalar(b)))
            }
            return
        }
        if b >= 0x20 && b <= 0x2f {
            // An intermediate byte, as in `CSI 1 SP q` (a cursor style) or `CSI ! p` (a
            // terminal reset). It is not the end of the sequence: the final byte is still to
            // come, and the text after it must not be eaten by a final that was never one.
            csiIntermediate = true
            return
        }
        if b >= 0x40 && b <= 0x7e {
            dispatchCsi(Character(UnicodeScalar(b)))
            state = .ground
            return
        }
        // Anything else — a control byte in the middle of a sequence — ends it where it
        // stands, and the parameters are dropped with it.
        state = .ground
    }

    /// Handles one byte of a string.
    private func osc(_ b: UInt8) {
        switch b {
        case 0x07: // BEL ends it
            finishOsc()
            state = .ground
        case 0x1b:
            state = .oscEscape
        default:
            if oscBuffer.utf8.count < Self.maxOSC {
                oscBuffer.append(Character(UnicodeScalar(b)))
            } else {
                oscOverflow = true
            }
        }
    }

    /// Handles the byte after the escape byte that may end a string.
    private func oscEscape(_ b: UInt8) {
        finishOsc()
        if b == UInt8(ascii: "\\") { // the string terminator
            state = .ground
            return
        }
        // The escape byte ended the string on its own, so what follows it starts a new
        // sequence rather than continuing the old string.
        state = .escape
        feed(b)
    }

    /// Handles the byte that designates a character set.
    private func charset(_ b: UInt8) {
        // Only G0 is used: G1 would take a shift-out byte to select it, which nothing the
        // client emits does. An unknown designation means the ordinary ASCII set, as it does
        // in xterm.
        if !charsetG1 {
            g0Special = b == UInt8(ascii: "0")
        }
        state = .ground
    }

    /// Reports a complete string to the sink its code names: a title, or a clipboard payload.
    ///
    /// A string the parser could not hold in full is dropped rather than reported in part —
    /// half a title or half a payload is not what the client asked for, and a truncated
    /// payload can still look like a well-formed one.
    private func finishOsc() {
        let text = oscBuffer
        oscBuffer = ""
        if oscOverflow {
            oscOverflow = false
            return
        }
        guard let separator = text.firstIndex(of: ";") else { return }
        let code = String(text[text.startIndex..<separator])
        let rest = String(text[text.index(after: separator)...])
        switch code {
        case "2":
            titleSink?(rest)
        case "52":
            guard let second = rest.firstIndex(of: ";") else { return }
            let payload = String(rest[rest.index(after: second)...])
            // "?" is a query: it asks the terminal what the clipboard holds, which is not
            // something to put on the clipboard.
            if payload.isEmpty || payload == "?" { return }
            guard let decoded = decodeBase64(payload) else { return }
            clipboardSink?(decoded)
        default:
            break
        }
    }

    // MARK: - Control sequences

    /// Carries out one control sequence, whose parameters are in ``csiBuffer``.
    private func dispatchCsi(_ final: Character) {
        // Every sequence ends a deferred wrap, which is how vterm.go's dispatchCSI opens.
        screen.cancelWrap()
        let intermediate = csiIntermediate
        csiIntermediate = false
        if intermediate {
            // An intermediate byte makes the final part of a different, private sequence:
            // `CSI 1 SP q` is a cursor style, not a cursor move. Those are ignored rather than
            // risk reading a final as the sequence it would be without it.
            return
        }
        switch final {
        case "A": screen.moveCursor(x: screen.cursorX, y: screen.cursorY - param(0, 1))
        case "B": screen.moveCursor(x: screen.cursorX, y: screen.cursorY + param(0, 1))
        case "C": screen.moveCursor(x: screen.cursorX + param(0, 1), y: screen.cursorY)
        case "D": screen.moveCursor(x: screen.cursorX - param(0, 1), y: screen.cursorY)
        case "E": screen.moveCursor(x: 0, y: screen.cursorY + param(0, 1))
        case "F": screen.moveCursor(x: 0, y: screen.cursorY - param(0, 1))
        case "G", "`": screen.moveCursor(x: param(0, 1) - 1, y: screen.cursorY)
        case "d": screen.moveCursor(x: screen.cursorX, y: param(0, 1) - 1)
        case "H", "f": screen.moveCursor(x: param(1, 1) - 1, y: param(0, 1) - 1)
        case "J": screen.eraseDisplay(mode: param(0, 0))
        case "K": screen.eraseLine(mode: param(0, 0))
        case "S": screen.scrollUp(param(0, 1))
        case "T": screen.scrollDown(param(0, 1))
        case "m": screen.applySgr(splitParams(csiBuffer))
        case "r": screen.setScrollRegion(top: param(0, 1) - 1, bottom: param(1, screen.rows) - 1)
        case "h": setMode(true)
        case "l": setMode(false)
        case "L": screen.insertLines(param(0, 1))
        case "M": screen.deleteLines(param(0, 1))
        case "P": screen.deleteChars(count: param(0, 1))
        case "@": screen.insertChars(count: param(0, 1))
        case "X": screen.eraseChars(count: param(0, 1))
        case "s": screen.saveCursor()
        case "u": screen.restoreCursor()
        default: break
        }
    }

    /// Applies the private modes (`CSI ? n h` and `CSI ? n l`) `vterm.go` knows.
    private func setMode(_ on: Bool) {
        guard privateMarker == "?" else { return }
        switch param(0, 0) {
        case 25:
            screen.setCursorVisible(on)
        case 1049:
            screen.setAltScreen(on, saveCursor: true) // save the cursor on entry, restore it on exit
        case 47, 1047:
            screen.setAltScreen(on, saveCursor: false) // the legacy switches keep the cursor
        case 1000, 1002, 1003:
            screen.setMouseReporting(on)
        case 1006:
            screen.setMouseSGR(on)
        case 2004:
            break // bracketed paste changes what the child receives, not the screen
        default:
            break
        }
    }

    /// Reads one numeric parameter of the sequence in flight, or `fallback` if it has none.
    private func param(_ index: Int, _ fallback: Int) -> Int {
        let parts = splitParams(csiBuffer)
        guard index < parts.count else { return fallback }
        return csiIntOrNull(parts[index]) ?? fallback
    }

    // MARK: - UTF-8

    /// Adds one byte to the rune being decoded, and prints it when it is complete.
    private func writeUtf8(_ b: UInt8) {
        if utf8.isEmpty {
            if b < 0x80 {
                // Plain ASCII, which is only read here when a broken sequence was abandoned
                // just before it.
                screen.putGlyph(String(decoding: [b], as: UTF8.self), width: 1)
                return
            }
            let need = Self.utf8Length(b)
            if need == 0 {
                // A byte that cannot begin a rune is printed as the replacement character, and
                // only itself is consumed: the bytes around it are still the stream's own.
                screen.putGlyph(Self.replacement, width: 1)
                return
            }
            utf8.append(b)
            return
        }
        if b & 0xc0 != 0x80 {
            // The rune was never finished. The replacement stands for the lead byte, and this
            // byte is read again as what it is.
            utf8.removeAll()
            screen.putGlyph(Self.replacement, width: 1)
            writeUtf8(b)
            return
        }
        utf8.append(b)
        let need = Self.utf8Length(utf8[0])
        if utf8.count < need { return }
        let codePoint = Self.decodeUtf8(utf8, length: need)
        utf8.removeAll()
        guard codePoint >= 0, let scalar = UnicodeScalar(UInt32(codePoint)) else {
            // Structurally sound but not a character: an overlong form, a surrogate, or a code
            // point past the last one. It is replaced as a whole.
            screen.putGlyph(Self.replacement, width: 1)
            return
        }
        screen.putGlyph(String(scalar), width: Self.displayWidth(UInt32(codePoint)))
    }

    /// How many bytes a lead byte begins, or 0 when it begins nothing.
    private static func utf8Length(_ lead: UInt8) -> Int {
        switch lead {
        case 0xc2...0xdf: return 2 // 0xc0 and 0xc1 would only spell an overlong form
        case 0xe0...0xef: return 3
        case 0xf0...0xf4: return 4 // 0xf5 and above are past the last code point
        default: return 0
        }
    }

    /// Assembles a complete sequence, or returns -1 if it is not a character.
    ///
    /// The structure has already been checked, so what is left is whether the value it spells
    /// is one: the shortest form, no surrogate half, and no code point past the last one there
    /// is.
    private static func decodeUtf8(_ bytes: [UInt8], length: Int) -> Int {
        var codePoint: Int
        switch length {
        case 1: codePoint = Int(bytes[0] & 0x7f)
        case 2: codePoint = Int(bytes[0] & 0x1f)
        case 3: codePoint = Int(bytes[0] & 0x0f)
        default: codePoint = Int(bytes[0] & 0x07)
        }
        for i in 1..<length {
            codePoint = (codePoint << 6) | Int(bytes[i] & 0x3f)
        }
        let shortest: Int
        switch length {
        case 1: shortest = 0x00
        case 2: shortest = 0x80
        case 3: shortest = 0x800
        default: shortest = 0x10000
        }
        if codePoint < shortest || codePoint > 0x10ffff { return -1 }
        if codePoint >= 0xd800 && codePoint <= 0xdfff { return -1 }
        return codePoint
    }

    /// How many columns a character occupies: two for the wide ones, one for the rest.
    ///
    /// The ranges are the East Asian Wide and Fullwidth ones, which is what a CJK page, a
    /// fullwidth form and most emoji are drawn from. A combining mark is measured as one
    /// column rather than zero, which renders it beside its base character instead of over it
    /// — a page that uses them is legible either way, and this is the one place the emulator
    /// knowingly differs from a desktop terminal.
    public static func displayWidth(_ scalar: UInt32) -> Int {
        switch scalar {
        case 0x1100...0x115f, // Hangul Jamo
             0x2e80...0x303e, // CJK radicals, Kangxi radicals, CJK symbols
             0x3041...0x33ff, // Hiragana, Katakana, Bopomofo, Hangul compatibility
             0x3400...0x4dbf, // CJK unified ideographs extension A
             0x4e00...0x9fff, // CJK unified ideographs
             0xa000...0xa4cf, // Yi syllables and radicals
             0xac00...0xd7a3, // Hangul syllables
             0xf900...0xfaff, // CJK compatibility ideographs
             0xfe10...0xfe19, // vertical forms
             0xfe30...0xfe6f, // CJK compatibility forms, small form variants
             0xff00...0xff60, // fullwidth forms
             0xffe0...0xffe6, // fullwidth signs
             0x1f300...0x1f64f, // pictographs and emoticons
             0x1f680...0x1f6ff, // transport and map symbols
             0x1f900...0x1f9ff, // supplemental symbols
             0x20000...0x3fffd: // CJK unified ideographs extensions B and beyond
            return 2
        default:
            return 1
        }
    }

    /// Decodes a clipboard payload, or returns nil if it is not base64.
    ///
    /// It is decoded here rather than by Foundation so that what counts as base64 is the same
    /// on both sides of the port: a payload whose length is not a multiple of four, or whose
    /// padding is not where the payload ends, is refused whole rather than decoded into
    /// something the sender did not mean.
    private func decodeBase64(_ s: String) -> String? {
        let digits = Array(s.utf8)
        if digits.isEmpty || digits.count % 4 != 0 { return nil }
        var out = [UInt8]()
        out.reserveCapacity(digits.count / 4 * 3)
        var i = 0
        while i < digits.count {
            let chunk = Array(digits[i..<(i + 4)])
            let padding: Int
            if chunk[3] == UInt8(ascii: "=") && chunk[2] == UInt8(ascii: "=") {
                padding = 2
            } else if chunk[3] == UInt8(ascii: "=") {
                padding = 1
            } else {
                padding = 0
            }
            var value = 0
            for j in 0..<4 {
                let c = chunk[j]
                let digit: Int
                if c == UInt8(ascii: "=") && j >= 4 - padding {
                    digit = 0
                } else if c >= UInt8(ascii: "A") && c <= UInt8(ascii: "Z") {
                    digit = Int(c) - Int(UInt8(ascii: "A"))
                } else if c >= UInt8(ascii: "a") && c <= UInt8(ascii: "z") {
                    digit = Int(c) - Int(UInt8(ascii: "a")) + 26
                } else if c >= UInt8(ascii: "0") && c <= UInt8(ascii: "9") {
                    digit = Int(c) - Int(UInt8(ascii: "0")) + 52
                } else if c == UInt8(ascii: "+") {
                    digit = 62
                } else if c == UInt8(ascii: "/") {
                    digit = 63
                } else {
                    return nil
                }
                value = (value << 6) | digit
            }
            out.append(UInt8((value >> 16) & 0xff))
            if padding < 2 { out.append(UInt8((value >> 8) & 0xff)) }
            if padding < 1 { out.append(UInt8(value & 0xff)) }
            i += 4
        }
        return String(decoding: out, as: UTF8.self)
    }

    /// How much of a control sequence's parameters is kept.
    private static let maxCSI = 256

    /// How much of a string is kept before it is discarded.
    private static let maxOSC = 8192

    /// The character a byte that is not part of one is shown as.
    private static let replacement = "\u{FFFD}"
}

/// Splits a control sequence's parameters on their semicolons, keeping the empty ones.
///
/// `1;` is two parameters, the second of which is absent, and a defaulting rule that cannot
/// tell the difference reads a cursor row of "the last one" where the client said "the
/// first".
func splitParams(_ buffer: String) -> [String] {
    buffer.split(separator: ";", omittingEmptySubsequences: false).map(String.init)
}

/// Reads one numerical parameter, which must be digits and nothing else.
///
/// A sub-parameter spelled with colons (ITU T.416) is not an integer, so the attribute it
/// spells is ignored rather than misread as the number before the colon. A parameter that is
/// all digits is capped rather than allowed to overflow: a client can send a cursor position
/// of any length, and a terminal that wrapped around to a negative row would place the cursor
/// somewhere the client never asked for.
func csiIntOrNull(_ text: String) -> Int? {
    if text.isEmpty { return nil }
    var value = 0
    for byte in text.utf8 {
        guard byte >= UInt8(ascii: "0"), byte <= UInt8(ascii: "9") else { return nil }
        value = value * 10 + Int(byte - UInt8(ascii: "0"))
        if value > maxCSIParameter { value = maxCSIParameter }
    }
    return value
}

/// The largest number an escape sequence's parameter is read as.
private let maxCSIParameter = 1_000_000

/// The line-drawing set a client selects with `ESC ( 0`, indexed from 0x60: the byte that
/// would have printed a letter prints a piece of a box instead, which is how a program draws
/// a border without a Unicode font and a UTF-8 stream.
private let decSpecialGraphics: [Character] = Array(
    "◆▒␉␌␍␊°±" + "␤␋┘┐┌└┼⎺⎻─⎼⎽" + "├┤┴┬│≤≥π≠£·"
)
