// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import kotlin.math.abs
import kotlin.math.cos
import kotlin.math.sin

/** Quaternion helpers and attitude fixtures shared by the heading tests. */
object HeadingFixtures {

    /** Hamilton product of two (x, y, z, w) quaternions. */
    fun multiply(a: FloatArray, b: FloatArray): FloatArray = floatArrayOf(
        a[3] * b[0] + a[0] * b[3] + a[1] * b[2] - a[2] * b[1],
        a[3] * b[1] - a[0] * b[2] + a[1] * b[3] + a[2] * b[0],
        a[3] * b[2] + a[0] * b[1] - a[1] * b[0] + a[2] * b[3],
        a[3] * b[3] - a[0] * b[0] - a[1] * b[1] - a[2] * b[2],
    )

    /**
     * A rotation of [bearingDeg] about the world's up axis, expressed as a compass
     * bearing. A bearing increases clockwise and a rotation about the up axis increases
     * counter-clockwise, so the sign is inverted here rather than everywhere else.
     */
    fun yaw(bearingDeg: Double): FloatArray {
        val half = Math.toRadians(-bearingDeg) / 2.0
        return floatArrayOf(0f, 0f, sin(half).toFloat(), cos(half).toFloat())
    }

    /**
     * The attitude of a tablet standing upright in portrait, screen facing the
     * operator, with the back of the tablet pointing at [yawDeg] true.
     *
     * Constructed as a world-frame yaw applied to the upright base attitude, which
     * is a rotation of +90 degrees about the world's east axis. Building it from
     * quaternions rather than from a matrix keeps the fixture independent of the
     * code under test.
     */
    fun standingUpright(yawDeg: Double): FloatArray =
        multiply(yaw(yawDeg), floatArrayOf(0.70710678f, 0f, 0f, 0.70710678f))

    /**
     * The attitude of a tablet lying flat, screen up, with the top edge of the
     * screen pointing at [yawDeg] true. The identity rotation already has the
     * device's frame aligned with the world's, so a yaw is the whole attitude.
     */
    fun lyingFlat(yawDeg: Double): FloatArray = yaw(yawDeg)

    /** The rotation vector measured on the tablet standing on its own stand. */
    val measuredOnTheStand = floatArrayOf(
        0.33369225f, -0.38858578f, 0.52529025f, 0.67950034f, -1.0f,
    )

    /** Reports whether got is within tolerance of want. */
    fun close(got: Double, want: Double, tolerance: Double = 0.5): Boolean =
        abs(HeadingReference.normalizeDegrees(got - want)) <= tolerance ||
            abs(HeadingReference.normalizeDegrees(got - want) - 360.0) <= tolerance
}
