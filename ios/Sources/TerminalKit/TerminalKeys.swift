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

/// One key press, as much of it as the terminal needs.
///
/// The client reads a pseudo-terminal, not a keyboard: what it understands is the byte
/// stream a real terminal produces, so a key press has to be named well enough to be
/// spelled that way. The cases are the three kinds of key a terminal has — a character,
/// a control code, and one of the named keys that is not a character at all.
///
/// There is deliberately no case for F1 or F2: nothing in the client's interface reads a
/// function key, and neither does the Go table, so a key this enum cannot name is a key
/// that sends nothing rather than one that sends something wrong.
public enum TerminalKey: Equatable, Hashable, Sendable {
    /// A printable character, or several when an input method commits a word at once.
    case rune(String)

    /// The return key, which sends a carriage return.
    case enter

    /// The tab key.
    case tab

    /// The backspace key, which sends DEL rather than BS.
    case backspace

    /// The escape key.
    case escape

    /// The up arrow.
    case up

    /// The down arrow.
    case down

    /// The right arrow.
    case right

    /// The left arrow.
    case left

    /// The start of the line.
    case home

    /// The end of the line.
    case end

    /// A page towards the start.
    case pageUp

    /// A page towards the end.
    case pageDown

    /// Delete at the cursor.
    case delete

    /// Insert at the cursor.
    case insert

    /// A letter held with the control key, which sends the letter's control code.
    case ctrl(Character)
}

/// Turns a key press into the bytes a terminal would have written for it.
///
/// The Go side's `keyToANSI` (`tui/embedded-terminal.go`) is the same table, and the two
/// agree on the two points where a keyboard and a terminal disagree — backspace is DEL,
/// and the arrows are the CSI forms rather than the SS3 ones. A disagreement here is not
/// a cosmetic one: the client's own key handling reads those exact bytes.
public enum TerminalKeys {

    /// The byte an alt combination is prefixed with, and the escape key's own.
    public static let escape: UInt8 = 0x1B

    /// The bytes `key` sends, or nil when it sends nothing.
    ///
    /// `control` and `alt` are the latches the key strip arms (``ModifierLatch``), applied
    /// in the order a terminal applies them: control turns a single character into its
    /// control code, and alt then rides in front of whatever the key sends, because a
    /// terminal has no alt byte at all — what it writes is the key's own bytes with an
    /// escape before them, which is `xterm`'s `metaSendsEscape` and what the client reads
    /// as a meta combination.
    ///
    /// nil is not the same as an empty array: an empty write is still a write, and a
    /// caller that sent one would be telling the client something happened.
    public static func bytes(
        for key: TerminalKey, control: Bool = false, alt: Bool = false
    ) -> [UInt8]? {
        guard let base = encoded(key, control: control) else { return nil }
        return alt ? [escape] + base : base
    }

    /// Whether the control modifier changes what `key` sends.
    ///
    /// Only a single character has a control code. A word an input method committed at
    /// once is not a character to the terminal, and a key like an arrow has no code
    /// either, so both are sent as themselves — which is why the latch consults this
    /// before spending itself: a modifier is spent by a key it actually changed.
    ///
    /// The test is against the key's UTF-16 width rather than its character count so that
    /// it matches the Kotlin appliance, which length-checks a `String`; a character
    /// outside the basic plane is two units there and is treated as a word, not a letter.
    public static func controlApplies(to key: TerminalKey) -> Bool {
        guard case .rune(let text) = key else { return false }
        return text.utf16.count == 1
    }

    /// The bytes `key` sends before the alt prefix is added.
    private static func encoded(_ key: TerminalKey, control: Bool) -> [UInt8]? {
        switch key {
        case .ctrl(let letter):
            return controlCode(of: letter)
        case .rune(let text):
            guard !text.isEmpty else { return nil }
            // A single character under control becomes its control code and nothing else:
            // a letter's byte is the code, and a character with no code at all sends
            // nothing rather than the character, because the modifier was asked for.
            if control, controlApplies(to: key), let character = text.first {
                return controlCode(of: character)
            }
            return Array(text.utf8)
        case .enter:
            return [0x0D]  // a carriage return, not a line feed
        case .tab:
            return [0x09]
        // Backspace sends DEL: the client is an editor's descendant and reads 0x7F, which
        // is what a terminal sends for the key and what the Go side sends as well.
        case .backspace:
            return [0x7F]
        case .escape:
            return [escape]
        case .up:
            return csi("A")
        case .down:
            return csi("B")
        case .right:
            return csi("C")
        case .left:
            return csi("D")
        case .home:
            return csi("H")
        case .end:
            return csi("F")
        // The four keys a real terminal spells with a number and a tilde.
        case .pageUp:
            return csi("5~")
        case .pageDown:
            return csi("6~")
        case .delete:
            return csi("3~")
        case .insert:
            return csi("2~")
        }
    }

    /// The control code a letter's control key sends: ctrl-A is 0x01 through ctrl-Z at
    /// 0x1A, which is the letter's place in the alphabet.
    ///
    /// The case of the letter is not part of the key — a layout reports whichever it
    /// reports — so both are accepted. Only letters have codes; anything else under the
    /// control key means nothing to the terminal.
    static func controlCode(of letter: Character) -> [UInt8]? {
        let upper = Character(String(letter).uppercased())
        guard upper.unicodeScalars.count == 1,
            let ascii = upper.asciiValue, ascii >= 0x41, ascii <= 0x5A
        else {
            return nil
        }
        return [ascii - 0x40]
    }

    /// Spells a control sequence: the escape byte, the bracket, and the body.
    private static func csi(_ body: String) -> [UInt8] {
        [escape, 0x5B] + Array(body.utf8)
    }
}
