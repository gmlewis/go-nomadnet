// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import kotlin.math.sqrt

/**
 * What a finger did on the console, once it has come up.
 *
 * [None] is a gesture the console acts on as it happens and not at the end of — a drag, which
 * is a scroll — and it is a value of its own so that a caller must say what it does with one
 * rather than falling through to the tap it is not.
 */
sealed interface TouchOutcome {

    /** None is a gesture that was not a tap: a drag, or a finger that wandered. */
    object None : TouchOutcome

    /** KeyRow is a tap on the on-screen keys, at a pixel of the strip. */
    data class KeyRow(val x: Int, val y: Int) : TouchOutcome

    /** Console is a tap on the client's own interface, at a pixel of the view. */
    data class Console(val x: Int, val y: Int) : TouchOutcome
}

/**
 * TouchGesture is the finger on the console, between going down and coming up.
 *
 * The view has three things a finger can do to it and one gesture that means all of them, so
 * the decision is made here, where it can be asserted without a screen, and the view is left
 * with the `Canvas` and the `MotionEvent` that a JVM test cannot reach.
 *
 * The rule is the one a touch interface has to have: the half of the view the finger went down
 * on decides what a tap is — a button on the key row, or the client's interface — and a
 * gesture that ever travelled further than the touch slop is a drag. The slop is what makes a
 * tap possible at all, because a finger on glass is never perfectly still, and it is sticky:
 * a finger that travelled a long way and came back to where it started was a scroll on the
 * way there and is not a tap for having ended where it began.
 *
 * The gesture is told nothing about the console beyond where it is on the screen and how tall
 * one line of it is. What a drag scrolls is the console's scrollback, which the view owns.
 */
class TouchGesture(private val slop: Float) {

    private var downX = 0f
    private var downY = 0f
    private var dragY = 0f
    private var remainder = 0f
    private var moved = false
    private var onKeyRow = false

    /**
     * down starts a gesture at a pixel of the view, [keyRowTop] being where the on-screen
     * keys begin.
     *
     * A view that has not been laid out yet has no room for a key row and reports zero, in
     * which case everything is one — the row is drawn over the console, so a tap the row
     * covers must not be a tap on both.
     */
    fun down(x: Float, y: Float, keyRowTop: Int) {
        downX = x
        downY = y
        dragY = y
        remainder = 0f
        moved = false
        onKeyRow = y >= keyRowTop
    }

    /**
     * move is how far the movement drags the console, in whole lines: a finger moving up the
     * screen is positive.
     *
     * What that means is the caller's: the lines above the screen are what a console has to
     * scroll through, and the next lines of a document are what a client that handles the
     * mouse is told to scroll to, and the same finger movement asks for both. See
     * [TerminalView.scrollOrWheel].
     *
     * The movement is measured from the last one rather than from the start, so a slow drag
     * accumulates: half a line and half a line are a line. The part that is not a whole line is
     * kept for the next movement rather than thrown away, which is what keeps a slow drag from
     * having to travel a whole cell before anything moves at all.
     *
     * A gesture that went down on the strip of keys scrolls nothing: the finger has started on
     * a button and has changed its mind about it, which is not a request to scroll a console
     * the strip is drawn over. It is still a movement, though, and one that has travelled past
     * the slop is no longer a tap on that button.
     */
    fun move(x: Float, y: Float, cellHeight: Int): Int {
        if (!moved && distance(x - downX, y - downY) > slop) {
            moved = true
        }
        if (onKeyRow || cellHeight <= 0) return 0
        remainder += y - dragY
        dragY = y
        val lines = (remainder / cellHeight).toInt()
        if (lines == 0) return 0
        remainder -= lines * cellHeight
        // The finger's own sign is the opposite one: it reports where it is on the screen,
        // and this reports which way it went, up being positive.
        return -lines
    }

    /**
     * up ends the gesture, at the pixel the finger left the screen.
     *
     * A tap is reported where the finger came up rather than where it went down, which is
     * where every other touch interface reports one and matters only within the slop.
     */
    fun up(x: Float, y: Float): TouchOutcome {
        if (moved) return TouchOutcome.None
        return if (onKeyRow) {
            TouchOutcome.KeyRow(x.toInt(), y.toInt())
        } else {
            TouchOutcome.Console(x.toInt(), y.toInt())
        }
    }

    /** distance is how far apart two offsets are, as a length rather than as two of them. */
    private fun distance(dx: Float, dy: Float): Float = sqrt(dx * dx + dy * dy)
}
