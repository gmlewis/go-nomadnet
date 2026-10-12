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

/// Control and Alt, on a keyboard that cannot hold them down.
///
/// A touch keyboard has no Control or Alt key to hold, so each is a switch: a tap on the
/// key strip arms it and the next typed key spends it. The next key is whatever is typed
/// next — on the soft keyboard, or on a keyboard that is not this app's — so the latch
/// belongs to the console rather than to the strip. A latch the strip alone consulted would
/// send a plain `q` where the person meant `^Q`, which is the difference between quitting
/// and typing.
///
/// A modifier is spent only by a key it changed. A key with no control code — an arrow, or
/// a word an input method commits at once — is sent as itself and leaves Control armed:
/// the person armed Control for a character, and the arrow was not one. Alt changes any
/// key, so anything spends it.
public struct ModifierLatch: Equatable, Sendable {

    /// The two switches the strip carries.
    public enum Kind: Equatable, Hashable, Sendable {
        /// Control, which turns the next single character into its control code.
        case control

        /// Alt (meta), which prefixes an escape to whatever the next key sends.
        case alt
    }

    /// The switches that are armed, in no particular order.
    ///
    /// It is private to be set because ``spend(_:)`` is the only thing that may spend a
    /// modifier, and it spends one only where the key it changed asks for it.
    public private(set) var armed: Set<Kind>

    /// A latch with neither modifier armed, which is where the console starts.
    public init() {
        armed = []
    }

    /// Arms `kind`, whether or not it was already armed.
    ///
    /// Arming an already-armed modifier leaves it armed rather than spending it: a tap the
    /// strip has drawn as a second tap on a lit button means "send this with control", and
    /// only ``toggle(_:)`` takes it back.
    public mutating func arm(_ kind: Kind) {
        armed.insert(kind)
    }

    /// Disarms `kind`, whether or not it was armed.
    public mutating func disarm(_ kind: Kind) {
        armed.remove(kind)
    }

    /// Arms `kind`, or disarms it if it was already armed.
    ///
    /// This is what a tap on the strip's button does, and two taps return the latch to
    /// where it started — the strip's way of taking back a tap that armed the wrong
    /// modifier, on a strip with no room for anything else.
    public mutating func toggle(_ kind: Kind) {
        if armed.contains(kind) {
            disarm(kind)
        } else {
            arm(kind)
        }
    }

    /// Whether `kind` is armed and will be applied to the next key.
    public func isArmed(_ kind: Kind) -> Bool { armed.contains(kind) }

    /// Disarms both modifiers, which is what leaving the console does.
    public mutating func clear() { armed.removeAll() }

    /// Applies the latches to `key`, and spends the ones that changed it.
    ///
    /// The return says which modifiers the caller must ask ``TerminalKeys`` to apply, so
    /// that the two files cannot disagree about what a control combination is. Control is
    /// reported and spent only for a key it actually changes — a single character — so that
    /// an arrow pressed while Control is lit is still an arrow, and Control is still armed
    /// afterwards. Alt is reported and spent for any key, an empty one included: the caller
    /// then finds that the key sends nothing either way, which is the same answer the
    /// Kotlin appliance reaches.
    public mutating func spend(_ key: TerminalKey) -> (control: Bool, alt: Bool) {
        var control = false
        if armed.contains(.control), TerminalKeys.controlApplies(to: key) {
            control = true
            armed.remove(.control)
        }
        var alt = false
        if armed.contains(.alt) {
            alt = true
            armed.remove(.alt)
        }
        return (control: control, alt: alt)
    }
}
