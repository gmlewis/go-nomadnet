// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import kotlin.math.abs
import kotlin.math.asin
import kotlin.math.atan2
import kotlin.math.cos
import kotlin.math.sin

/**
 * Which device axis the heading is measured along.
 *
 * There is no single "heading" for a device in an arbitrary attitude: there are
 * two natural ones, and they only agree when the device lies flat.
 *
 *  - [SCREEN_BACK] is the direction the back of the tablet points, which is
 *    where the operator holding it is looking. It is well conditioned while the
 *    device stands up — including on a stand — or is aimed at something.
 *  - [SCREEN_TOP] is the device's +Y axis, which is what Android's
 *    getOrientation reports. It is well conditioned while the device lies flat,
 *    and it answers "which way does the top edge point", which on a device in
 *    landscape is not where the operator is facing at all.
 */
enum class HeadingAxis {
    SCREEN_BACK,
    SCREEN_TOP,
}

/**
 * One attitude: where the chosen reference axis points, and how much to trust it.
 *
 * [illConditioned] is true when the reference axis is within [HeadingReference.DEGENERATE_BAND_DEG]
 * of vertical, which is the case that makes its azimuth pure noise. It is
 * reported rather than hidden, because an operator must never be handed a
 * confident bearing that is really a rounding error.
 */
data class Attitude(
    val headingDeg: Double,
    val pitchDeg: Double,
    val rollDeg: Double,
    val upComponent: Double,
    val illConditioned: Boolean,
)

/**
 * The heading of a device in an arbitrary attitude, in pure arithmetic.
 *
 * The rotation is derived here rather than through SensorManager so that the
 * whole reference-axis decision is a unit test with no device attached. The
 * convention is Android's: the rotation matrix maps a vector from the device's
 * frame (X right along the screen, Y up along the screen, Z out of the screen)
 * into the world frame (X east, Y north, Z up).
 */
object HeadingReference {

    /** How near vertical a reference axis has to be before its azimuth is noise. */
    const val DEGENERATE_BAND_DEG = 10.0

    /** The device frame's +Z, which points out of the screen at the viewer. */
    private val DEVICE_Z = doubleArrayOf(0.0, 0.0, 1.0)

    /**
     * The device-frame axis that appears "up" on the display, for a Surface
     * rotation of 0, 90, 180 or 270 degrees.
     *
     * This is the same mapping remapCoordinateSystem applies: for a 90-degree
     * rotation the displayed up direction is the device's -X, and so on. The
     * screen-back normal does not need it — the direction the back of the tablet
     * points is a fact about the hardware — but the screen-top reference does.
     */
    fun displayUpAxis(displayRotation: Int): DoubleArray = when (((displayRotation % 4) + 4) % 4) {
        0 -> doubleArrayOf(0.0, 1.0, 0.0)
        1 -> doubleArrayOf(-1.0, 0.0, 0.0)
        2 -> doubleArrayOf(0.0, -1.0, 0.0)
        else -> doubleArrayOf(1.0, 0.0, 0.0)
    }

    /**
     * Builds the device-to-world rotation matrix from a TYPE_ROTATION_VECTOR
     * reading. The vector is the rotation taking the device frame to the world
     * frame, in (x, y, z, w) order; any trailing element — the accuracy estimate
     * Android appends — is ignored.
     */
    fun rotationMatrix(rotationVector: FloatArray): DoubleArray {
        require(rotationVector.size >= 4) { "a rotation vector needs at least four elements" }
        var x = rotationVector[0].toDouble()
        var y = rotationVector[1].toDouble()
        var z = rotationVector[2].toDouble()
        var w = rotationVector[3].toDouble()

        // The reading is a unit quaternion, but a sensor is a sensor: normalise
        // rather than trust it, because a matrix built from a non-unit quaternion
        // is not a rotation and every angle derived from it is wrong.
        val norm = kotlin.math.sqrt(x * x + y * y + z * z + w * w)
        if (norm > 0.0 && abs(norm - 1.0) > 1e-9) {
            x /= norm
            y /= norm
            z /= norm
            w /= norm
        }

        return doubleArrayOf(
            1 - 2 * (y * y + z * z), 2 * (x * y - w * z), 2 * (x * z + w * y),
            2 * (x * y + w * z), 1 - 2 * (x * x + z * z), 2 * (y * z - w * x),
            2 * (x * z - w * y), 2 * (y * z + w * x), 1 - 2 * (x * x + y * y),
        )
    }

    /** Rotates a device-frame vector into the world by a rotation matrix. */
    fun toWorld(rotation: DoubleArray, deviceVector: DoubleArray): DoubleArray = doubleArrayOf(
        rotation[0] * deviceVector[0] + rotation[1] * deviceVector[1] + rotation[2] * deviceVector[2],
        rotation[3] * deviceVector[0] + rotation[4] * deviceVector[1] + rotation[5] * deviceVector[2],
        rotation[6] * deviceVector[0] + rotation[7] * deviceVector[1] + rotation[8] * deviceVector[2],
    )

    /**
     * Computes the attitude for the chosen reference axis.
     *
     * [displayRotation] is the Surface rotation in quarter turns (0, 1, 2, 3 for
     * 0, 90, 180 and 270 degrees). It changes nothing for [HeadingAxis.SCREEN_BACK]
     * and everything for [HeadingAxis.SCREEN_TOP], because the "top" of a rotated
     * display is a different piece of hardware.
     */
    fun attitude(
        rotationVector: FloatArray,
        axis: HeadingAxis,
        displayRotation: Int = 0,
    ): Attitude {
        val rotation = rotationMatrix(rotationVector)
        val reference = when (axis) {
            HeadingAxis.SCREEN_BACK -> doubleArrayOf(-DEVICE_Z[0], -DEVICE_Z[1], -DEVICE_Z[2])
            HeadingAxis.SCREEN_TOP -> displayUpAxis(displayRotation)
        }
        val world = toWorld(rotation, reference)
        val heading = normalizeDegrees(Math.toDegrees(atan2(world[0], world[1])))
        val up = world[2]

        // Android's getOrientation convention, kept so the pitch and roll the UI
        // reports are the familiar ones.
        val pitch = Math.toDegrees(asin((-rotation[7]).coerceIn(-1.0, 1.0)))
        val roll = Math.toDegrees(atan2(-rotation[6], rotation[8]))

        return Attitude(
            headingDeg = heading,
            pitchDeg = pitch,
            rollDeg = roll,
            upComponent = up,
            illConditioned = abs(up) >= sin(Math.toRadians(90.0 - DEGENERATE_BAND_DEG)),
        )
    }

    /**
     * The naive Android heading: the azimuth of the device's +Y axis, exactly
     * what SensorManager.getOrientation returns in element zero. It exists so a
     * test can show how far it diverges from the reference axis the operator
     * actually means, and so the comparison can be recorded rather than argued.
     */
    fun naiveAzimuth(rotationVector: FloatArray): Double {
        val rotation = rotationMatrix(rotationVector)
        return normalizeDegrees(Math.toDegrees(atan2(rotation[1], rotation[4])))
    }

    /** Wraps a heading into 0..360, so a boundary crossing carries no sign. */
    fun normalizeDegrees(value: Double): Double {
        var wrapped = value % 360.0
        if (wrapped < 0) {
            wrapped += 360.0
        }
        return wrapped
    }

    /** Builds a unit quaternion for a yaw about the world's up axis, for tests. */
    fun yawQuaternion(yawDeg: Double): FloatArray {
        val half = Math.toRadians(yawDeg) / 2.0
        return floatArrayOf(0f, 0f, sin(half).toFloat(), cos(half).toFloat())
    }
}
