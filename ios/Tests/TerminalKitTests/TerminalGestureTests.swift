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

import XCTest

@testable import TerminalKit

/// What a finger on the console means.
///
/// Which of the three things a finger did is decided without a screen, so it can be asserted
/// here: the view is where a touch and a canvas meet, and neither of those is reachable from
/// a plain unit test. The vectors below are all instances of one rule — travel within the
/// slop is jitter and still a tap, travel past it is a drag for the rest of the gesture, and
/// a drag is measured in whole lines with the part left over kept.
final class TerminalGestureTests: XCTestCase {

    /// slop is the touch slop these vectors are measured against, in points.
    private let slop = 16.0

    /// cellHeight is how tall one line of the console is in these vectors.
    private let cellHeight = 20.0

    private func newGesture() -> TouchGesture {
        TouchGesture(slop: slop, cellHeight: cellHeight)
    }

    func testATapIsATapWhereTheFingerCameUp() {
        var gesture = newGesture()
        gesture.down(x: 100, y: 300)
        XCTAssertEqual(gesture.up(), .tap(x: 100, y: 300))
    }

    func testAFingerThatJitteredIsStillATap() {
        // Four points of travel is a finger resting on glass, not a scroll.
        var gesture = newGesture()
        gesture.down(x: 100, y: 300)
        XCTAssertEqual(gesture.move(x: 103, y: 304), 0, "a small movement scrolled")
        // A tap is reported where the finger came up, which is where every other touch
        // interface reports one.
        XCTAssertEqual(gesture.up(), .tap(x: 103, y: 304))
    }

    func testADragIsAScrollAndNotATap() {
        var gesture = newGesture()
        gesture.down(x: 100, y: 300)

        // A finger moving up the screen asks for what is below the window — the next lines
        // of a document and the older lines of a console are the same gesture — so the drag
        // is positive here and a caller sends the wheel's own down.
        XCTAssertEqual(gesture.move(x: 100, y: 280), 1, "the drag's first line")
        XCTAssertEqual(gesture.move(x: 100, y: 260), 1, "the drag's second line")
        // And the release says nothing: the two lines were asked for as they happened, so a
        // caller that acted on them has nothing left to do.
        XCTAssertEqual(gesture.up(), .none, "a drag was also a tap")
    }

    func testADragIsMeasuredInWholeLinesAndKeepsThePartLeftOver() {
        var gesture = newGesture()
        gesture.down(x: 100, y: 500)

        // Half a cell is not a line, and it is not thrown away either: the two halves below
        // are one line between them, which is what makes a slow drag scroll smoothly rather
        // than needing a finger to travel a whole cell before anything moves.
        XCTAssertEqual(gesture.move(x: 100, y: 490), 0, "half a cell")
        XCTAssertEqual(gesture.move(x: 100, y: 480), 1, "the second half")
        XCTAssertEqual(gesture.move(x: 100, y: 470), 0, "the line after the one")

        // Downwards is the other direction, and the remainder carries across a change of
        // direction too: ten points of travel are still owed from the way back up.
        XCTAssertEqual(gesture.move(x: 100, y: 490), 0)
        XCTAssertEqual(gesture.move(x: 100, y: 500), -1)
        XCTAssertEqual(gesture.move(x: 100, y: 520), -1)
    }

    func testADragThatComesBackIsStillADrag() {
        var gesture = newGesture()
        gesture.down(x: 100, y: 300)
        XCTAssertEqual(gesture.move(x: 100, y: 200), 5)
        XCTAssertEqual(gesture.move(x: 100, y: 300), -5)

        // The finger went a long way and came back to where it started. It was a scroll on
        // the way there, and it is not a tap for having ended on the same point: acting on
        // it would send a click the reader never made. It named no line in the end, so there
        // is nothing left to send either.
        XCTAssertEqual(gesture.up(), .none)
    }

    func testAFingerThatWanderedSidewaysIsNotATapEither() {
        var gesture = newGesture()
        gesture.down(x: 100, y: 300)

        // Sideways is neither a scroll nor a tap: the console has nothing across the screen
        // to drag, and a swipe across the client's interface is not a press on anything.
        XCTAssertEqual(gesture.move(x: 200, y: 300), 0, "a sideways movement scrolled")
        XCTAssertEqual(gesture.up(), .none, "a sideways swipe was taken for a tap")
    }

    func testTheSlopIsMeasuredAsALengthAndNotAsOneAxis() {
        // A finger that moved eight points across and fifteen down has traveled more than
        // the slop even though neither axis did, which is why the test is on the distance
        // and not on either coordinate.
        var diagonal = newGesture()
        diagonal.down(x: 100, y: 300)
        XCTAssertEqual(diagonal.move(x: 108, y: 315), 0)
        XCTAssertEqual(diagonal.up(), .none, "a diagonal drag was taken for a tap")

        // Two axes that are each inside the slop are still a tap.
        var inside = newGesture()
        inside.down(x: 100, y: 300)
        XCTAssertEqual(inside.move(x: 110, y: 311), 0)
        XCTAssertEqual(inside.up(), .tap(x: 110, y: 311))

        // The slop itself is not past the slop: a finger that has traveled exactly the slop
        // has not yet been judged to be dragging.
        var edge = newGesture()
        edge.down(x: 100, y: 300)
        XCTAssertEqual(edge.move(x: 116, y: 300), 0)
        XCTAssertEqual(edge.up(), .tap(x: 116, y: 300))
    }

    func testOneMovementIsCappedAtTheNotchLimit() {
        // A fast finger crosses the whole screen in one movement, and each notch is a write
        // on the console socket, so the burst is bounded rather than proportional.
        var gesture = newGesture()
        gesture.down(x: 100, y: 3000)
        XCTAssertEqual(gesture.move(x: 100, y: 1000), 48, "the burst is capped")
        // The cap bounds what is sent, not what was traveled: what is left of the drag is
        // dropped rather than arriving on the next movement.
        XCTAssertEqual(gesture.up(), .none)

        var downwards = newGesture()
        downwards.down(x: 100, y: 0)
        XCTAssertEqual(downwards.move(x: 100, y: 2000), -48)
        XCTAssertEqual(downwards.up(), .none)
    }

    func testTheNotchLimitIsTheOneTheViewBounds() {
        XCTAssertEqual(TouchGesture.maxWheelNotches, 48)
    }

    func testAGestureWithNoFingerDownDoesNothing() {
        // A movement or a lift with no finger down is not a gesture, and reporting one would
        // be a scroll or a click on a screen nothing touched.
        var gesture = newGesture()
        XCTAssertEqual(gesture.move(x: 100, y: 200), 0)
        XCTAssertEqual(gesture.up(), .none)
        XCTAssertEqual(gesture.move(x: 100, y: 100), 0)

        // A lift ends the gesture: what comes after it is not part of the same finger.
        var lifted = newGesture()
        lifted.down(x: 10, y: 10)
        XCTAssertEqual(lifted.up(), .tap(x: 10, y: 10))
        XCTAssertEqual(lifted.move(x: 10, y: 200), 0)
        XCTAssertEqual(lifted.up(), .none)
    }

    func testAConsoleWithNoCellHeightYetScrollsNothing() {
        // A cell of no points is a console that has not been laid out, and every division
        // below would be by zero. The gesture is still a gesture — the finger that crossed
        // the slop is not a tap — it just has no lines to report.
        var gesture = TouchGesture(slop: slop, cellHeight: 0)
        gesture.down(x: 100, y: 300)
        XCTAssertEqual(gesture.move(x: 100, y: 100), 0)
        XCTAssertEqual(gesture.up(), .none)
    }

    func testTheSlopIsThePointWhereATapStops() {
        // A slop of nothing makes every movement of the finger a drag, which is what a
        // caller that measures its own slop asks for by passing one.
        var abrupt = TouchGesture(slop: 0, cellHeight: cellHeight)
        abrupt.down(x: 100, y: 300)
        XCTAssertEqual(abrupt.move(x: 100, y: 300.5), 0)
        XCTAssertEqual(abrupt.up(), .none)
    }
}
