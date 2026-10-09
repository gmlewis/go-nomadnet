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
 * What a finger on the console means.
 *
 * The console is one grid, and a finger can do three things to it: press one of the keys
 * along the bottom edge, tap the client's interface, or drag it back through the lines that
 * have scrolled off the top. Which one it was is decided here rather than in the view, so
 * that it can be asserted without a screen: [TerminalView] is where a `Canvas` and a
 * `MotionEvent` meet, and neither of those is reachable from a plain JVM test.
 *
 * The rule the vectors below are all instances of: the half of the view the finger went down
 * on decides what a tap is — the key row's buttons, or the client's interface — and a gesture
 * that ever travelled further than the touch slop is a drag, which is a scroll and not a tap
 * at all. The slop is what makes a tap possible on glass: a finger is never perfectly still,
 * and a tap that had to be still would be a tap nobody could make.
 *
 * A drag scrolls the console and never sends a key. On a tablet the drag is the only way back
 * through the scrollback, and a client whose interface is at the newest line is a client whose
 * interface is what the reader is looking at.
 */
class TerminalGestureTest {

    /** slop is the touch slop these vectors are measured against, in pixels. */
    private val slop = 16f

    /** cellHeight is how tall one line of the console is in these vectors. */
    private val cellHeight = 20

    /** keyRowTop is where the on-screen keys begin, well below every other number here. */
    private val keyRowTop = 1000

    private fun gesture(): TouchGesture = TouchGesture(slop)

    @Test
    fun `a tap on the console is a tap on the console`() {
        val gesture = gesture()
        gesture.down(100f, 300f, keyRowTop)

        assertEquals("a tap sent something other than a tap", TouchOutcome.Console(100, 300), gesture.up(100f, 300f))
    }

    @Test
    fun `a tap on the key row is one of its keys and not a tap on the console`() {
        val gesture = gesture()
        gesture.down(300f, 1050f, keyRowTop)

        // The key row is below the console's own lines, so a tap there is a key press and
        // must not also be a tap on the client's interface: the row's buttons are drawn over
        // the console, and a console that saw the tap as well would act twice on one finger.
        // Both coordinates are reported, because the strip is more than one row of keys.
        assertEquals("the key row's button", TouchOutcome.KeyRow(300, 1050), gesture.up(300f, 1050f))
    }

    @Test
    fun `a finger that jittered is still a tap`() {
        val gesture = gesture()
        gesture.down(100f, 300f, keyRowTop)

        // Four pixels of travel is a finger resting on glass, not a scroll.
        assertEquals("a small movement scrolled", 0, gesture.move(103f, 304f, cellHeight))
        assertEquals("a jittered tap", TouchOutcome.Console(103, 304), gesture.up(103f, 304f))
    }

    @Test
    fun `a drag is a scroll and not a tap`() {
        val gesture = gesture()
        gesture.down(100f, 300f, keyRowTop)

        // A finger moving up the screen asks for what is above it: the lines that have
        // scrolled off the top.
        assertEquals("the drag's first line", 1, gesture.move(100f, 280f, cellHeight))
        assertEquals("the drag's second line", 1, gesture.move(100f, 260f, cellHeight))
        assertEquals("a drag was also a tap", TouchOutcome.None, gesture.up(100f, 260f))
    }

    @Test
    fun `a drag is measured in whole lines and keeps the part left over`() {
        val gesture = gesture()
        gesture.down(100f, 500f, keyRowTop)

        // Half a cell is not a line, and it is not thrown away either: the two halves below
        // are one line between them, which is what makes a slow drag scroll smoothly rather
        // than needing a finger to travel a whole cell before anything moves.
        assertEquals("half a cell", 0, gesture.move(100f, 490f, cellHeight))
        assertEquals("the second half", 1, gesture.move(100f, 480f, cellHeight))
        assertEquals("the line after the one", 0, gesture.move(100f, 470f, cellHeight))
    }

    @Test
    fun `a drag that comes back is still a drag`() {
        val gesture = gesture()
        gesture.down(100f, 300f, keyRowTop)
        gesture.move(100f, 200f, cellHeight)
        gesture.move(100f, 300f, cellHeight)

        // The finger went a long way and came back to where it started. It was a scroll on
        // the way there, and it is not a tap for having ended on the same pixel: acting on
        // it would send a click the reader never made.
        assertEquals("a finger that came back was taken for a tap", TouchOutcome.None, gesture.up(100f, 300f))
    }

    @Test
    fun `a finger that wandered sideways is not a tap either`() {
        val gesture = gesture()
        gesture.down(100f, 300f, keyRowTop)

        // Sideways is neither a scroll nor a tap: the console has nothing across the screen
        // to drag, and a swipe across the client's interface is not a press on anything.
        assertEquals("a sideways movement scrolled", 0, gesture.move(200f, 300f, cellHeight))
        assertEquals("a sideways swipe was taken for a tap", TouchOutcome.None, gesture.up(200f, 300f))
    }

    @Test
    fun `a drag on the key row scrolls nothing`() {
        val gesture = gesture()
        gesture.down(300f, 1050f, keyRowTop)

        // A finger that went down on a button and slid off it has changed its mind about the
        // button. It has not asked to scroll the console, which the row is drawn over.
        assertEquals("a drag on the key row scrolled", 0, gesture.move(300f, 1100f, cellHeight))
        assertEquals("a drag on the key row sent a key", TouchOutcome.None, gesture.up(300f, 1100f))
    }
}
