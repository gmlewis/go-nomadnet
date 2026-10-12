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

/// The finger on the console, between going down and coming up.
///
/// A view has three things a finger can do to it — tap the client's interface, scroll it,
/// or press one of the keys along the bottom edge — and it is the view's job to know which
/// of them it drew, not the gesture's. So the gesture is told nothing about the console
/// beyond how far a finger may wander and still be a tap, and how tall one line of the
/// console is; what it reports is the movement itself, which the caller turns into a scroll
/// of its scrollback or a wheel report to the client.
///
/// The rule is the one every touch interface has to have: a finger on glass is never
/// perfectly still, so travel within `slop` is jitter and still a tap, and travel past it is
/// a drag — stickily, so a finger that went a long way and came back was a scroll on the
/// way there and is not a tap for having ended where it began.
public struct TouchGesture: Equatable, Sendable {

    /// What a finger did, once it has come up.
    ///
    /// A drag is not one of the cases, and that is deliberate: every line a drag asked for
    /// was asked for as it happened, by ``move(x:y:)``, and a caller that acted on those has
    /// nothing left to do when the finger comes up. A release that reported the drag again
    /// was a second scroll of the same travel, which is what the appliance did.
    public enum Outcome: Equatable, Sendable {
        /// A drag that asks for nothing: it crossed the slop, so it is not a tap, and what it
        /// asked for has already been asked for.
        case none
        /// A tap, at the point the finger came up rather than where it went down.
        case tap(x: Double, y: Double)
    }

    /// The most whole lines one movement, or one drag, is reported as.
    ///
    /// A drag is delivered as a notch of wheel per line it crossed, and one movement of a
    /// fast finger can cross the whole screen: each notch is a write on the console socket,
    /// so the burst is bounded rather than proportional to the screen.
    public static let maxWheelNotches = 48

    /// How far, in points, a finger may wander and still be a tap.
    public let slop: Double

    /// How tall one line of the console is, in points.
    public let cellHeight: Double

    /// Where the finger went down.
    private var downX: Double
    private var downY: Double

    /// Where the finger is now, which is where a tap is reported.
    private var lastX: Double
    private var lastY: Double

    /// Unreported travel, kept so that half a line and half a line are a line.
    private var travel: Double


    /// Whether the finger ever traveled further than `slop`.
    private var moved: Bool

    /// Whether a finger has gone down and not yet come up.
    private var active: Bool

    /// A gesture measured against a touch slop, for a console whose lines are `cellHeight`
    /// points tall.
    ///
    /// A cell height of zero is a console that has not been laid out yet; it is accepted
    /// and reports no lines, because the alternative — a view that refuses to hand a
    /// gesture its own metrics — is a view that cannot be touched before its first layout.
    public init(slop: Double, cellHeight: Double) {
        self.slop = slop
        self.cellHeight = cellHeight
        downX = 0
        downY = 0
        lastX = 0
        lastY = 0
        travel = 0
        moved = false
        active = false
    }

    /// Starts a gesture at a point of the view.
    public mutating func down(x: Double, y: Double) {
        downX = x
        downY = y
        lastX = x
        lastY = y
        travel = 0
        moved = false
        active = true
    }

    /// How far the movement drags the console, in whole lines: **positive is a finger moving
    /// up the screen**, which asks for what is above the window.
    ///
    /// The movement is measured from the last one rather than from the start, so a slow drag
    /// accumulates: half a line and half a line are a line. The part that is not a whole
    /// line is kept for the next movement rather than thrown away, which is what keeps a
    /// slow drag from having to travel a full cell before anything moves at all.
    ///
    /// A movement that never happened — no finger down, or one already lifted — drags
    /// nothing, and nor does one on a console with no cell height yet.
    public mutating func move(x: Double, y: Double) -> Int {
        guard active else { return 0 }
        if !moved, distance(x - downX, y - downY) > slop {
            moved = true
        }
        let delta = y - lastY
        lastX = x
        lastY = y
        guard cellHeight > 0 else { return 0 }
        travel += delta
        let lines = Int(travel / cellHeight)
        if lines == 0 { return 0 }
        travel -= Double(lines) * cellHeight
        // The finger's own sign is the opposite one: it reports where it is on the screen,
        // and this reports which way it went, up being positive. The appliance's Android
        // half reports the same, and the two have to agree, because the same finger on the
        // same client has to scroll the same way on both.
        return clamp(-lines)
    }

    /// Ends the gesture, at the point the finger left the screen.
    ///
    /// A tap is reported where the finger came up rather than where it went down, which is
    /// where every other touch interface reports one and matters only within the slop. A drag
    /// reports nothing: the lines it asked for were asked for as they happened, so a caller
    /// that acted on them has nothing left to do and one that ignored them has nothing to
    /// send.
    public mutating func up() -> Outcome {
        guard active else { return .none }
        let wasMoved = moved
        let x = lastX
        let y = lastY
        reset()
        return wasMoved ? .none : .tap(x: x, y: y)
    }

    /// Leaves the gesture ready for the next finger.
    private mutating func reset() {
        active = false
        moved = false
        travel = 0
    }

    /// One report's notches, bounded so that a fast finger cannot write a screenful of
    /// them at once.
    private func clamp(_ notches: Int) -> Int {
        min(max(notches, -TouchGesture.maxWheelNotches), TouchGesture.maxWheelNotches)
    }

    /// How far apart two offsets are, as a length rather than as two of them.
    private func distance(_ dx: Double, _ dy: Double) -> Double {
        (dx * dx + dy * dy).squareRoot()
    }
}
