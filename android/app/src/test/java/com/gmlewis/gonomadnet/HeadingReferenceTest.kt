// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The reference-axis decision, tested without a device.
 *
 * A rotation matrix is only arithmetic, so the whole question of which axis a
 * heading should be measured along can be settled by constructing an attitude and
 * asking. This is the test that would have caught the mistake the design brief's
 * reasoning invited.
 */
class HeadingReferenceTest {

    @Test
    fun screenBackHeadingTracksTheYawOfATabletStandingUpright() {
        for (yaw in listOf(0.0, 45.0, 90.0, 180.0, 270.0, 359.0)) {
            val attitude = HeadingReference.attitude(
                HeadingFixtures.standingUpright(yaw),
                HeadingAxis.SCREEN_BACK,
            )
            assertTrue(
                "standing upright, yaw $yaw: screen-back heading was ${attitude.headingDeg}",
                HeadingFixtures.close(attitude.headingDeg, yaw),
            )
            assertFalse(
                "standing upright is the well-conditioned case for the screen-back normal",
                attitude.illConditioned,
            )
        }
    }

    @Test
    fun screenTopHeadingIsNotTheDirectionTheOperatorFacesWhenStandingUpright() {
        // This is the failure the brief predicted for the wrong reason. On a tablet
        // standing up, the device's +Y axis is vertical, so its azimuth is the
        // degenerate case: whatever number comes out, it does not describe where
        // anybody is looking.
        val attitude = HeadingReference.attitude(
            HeadingFixtures.standingUpright(90.0),
            HeadingAxis.SCREEN_TOP,
        )
        assertTrue(
            "a vertical reference axis must be reported as ill-conditioned",
            attitude.illConditioned,
        )
    }

    @Test
    fun screenTopHeadingIsWellConditionedAndCorrectWhenLyingFlat() {
        // ... and the opposite mounting makes the opposite choice right, which is
        // why the axis is a setting and not a constant.
        for (yaw in listOf(0.0, 90.0, 200.0)) {
            val attitude = HeadingReference.attitude(
                HeadingFixtures.lyingFlat(yaw),
                HeadingAxis.SCREEN_TOP,
            )
            assertFalse(
                "a flat device is the well-conditioned case for the screen-top reference",
                attitude.illConditioned,
            )
            assertTrue(
                "flat, yaw $yaw: screen-top heading was ${attitude.headingDeg}",
                HeadingFixtures.close(attitude.headingDeg, yaw),
            )
        }
        // And a flat device makes the screen-back normal the degenerate one.
        assertTrue(
            HeadingReference.attitude(HeadingFixtures.lyingFlat(0.0), HeadingAxis.SCREEN_BACK).illConditioned,
        )
    }

    @Test
    fun theTwoReferencesDisagreeExceptWhenTheDeviceIsFlat() {
        val standing = 90.0
        val back = HeadingReference.attitude(HeadingFixtures.standingUpright(standing), HeadingAxis.SCREEN_BACK)
        val top = HeadingReference.attitude(HeadingFixtures.lyingFlat(standing), HeadingAxis.SCREEN_TOP)
        assertTrue(
            "the two references must agree when the device is flat",
            HeadingFixtures.close(back.headingDeg, top.headingDeg, 1.0),
        )
    }

    @Test
    fun theMeasuredStandMakesTheNaiveRecipeConfidentlyWrong() {
        // The tablet as it is actually used: standing, rotated into landscape, tipped
        // about 28 degrees back. Both references are well conditioned here, and they
        // disagree by very nearly a right angle — so Android's getOrientation is not
        // noisy on this stand, it is stable and wrong.
        val rv = HeadingFixtures.measuredOnTheStand

        val back = HeadingReference.attitude(rv, HeadingAxis.SCREEN_BACK)
        val naive = HeadingReference.naiveAzimuth(rv)

        assertFalse("the screen-back normal is well conditioned on this stand", back.illConditioned)
        assertTrue(
            "screen-back should be a little past north, was ${back.headingDeg}",
            HeadingFixtures.close(back.headingDeg, 11.6, 2.0),
        )
        assertTrue(
            "getOrientation's azimuth should be about 89 degrees away, was $naive",
            HeadingFixtures.close(naive, -77.0, 2.0),
        )
        // The shortest angular distance between them, which is the quantity that matters:
        // a reader told the wrong bearing by a quarter turn is worse off than one told no
        // bearing at all.
        val divergence = ((naive - back.headingDeg + 540.0) % 360.0) - 180.0
        assertTrue(
            "the two must disagree by about 89 degrees, disagreed by $divergence",
            Math.abs(Math.abs(divergence) - 88.6) < 3.0,
        )
    }

    @Test
    fun displayRotationChangesTheScreenTopReferenceAndNotTheScreenBackOne() {
        val rv = HeadingFixtures.lyingFlat(0.0)

        val back0 = HeadingReference.attitude(rv, HeadingAxis.SCREEN_BACK, 0)
        val back1 = HeadingReference.attitude(rv, HeadingAxis.SCREEN_BACK, 1)
        assertEquals(
            "the direction the back of the tablet points is a fact about the hardware " +
                "and must not depend on how the display is rotated",
            back0.headingDeg,
            back1.headingDeg,
            1e-9,
        )

        val top0 = HeadingReference.attitude(rv, HeadingAxis.SCREEN_TOP, 0).headingDeg
        val top1 = HeadingReference.attitude(rv, HeadingAxis.SCREEN_TOP, 1).headingDeg
        // Android's documented remap for a 90-degree display rotation makes the device's
        // -X axis the top of the display, which is a quarter turn the other way from the
        // device's +Y.
        assertTrue(
            "the screen-top reference must follow the display: $top0 versus $top1",
            Math.abs(HeadingReference.normalizeDegrees(top1 - top0) - 270.0) < 1.0,
        )
    }

    @Test
    fun theRotationMatrixReproducesTheMeasuredGravityVector() {
        // The matrix the appliance derives itself has to agree with the one Android
        // derives, or neither the heading nor the tilt means anything. On the stand,
        // gravity in the device frame was measured at (0.879, 0.044, 0.475), and the
        // device's +X axis is the column that reproduces it.
        val r = HeadingReference.rotationMatrix(HeadingFixtures.measuredOnTheStand)
        val deviceX = doubleArrayOf(1.0, 0.0, 0.0)
        val xInWorld = HeadingReference.toWorld(r, deviceX)
        assertTrue("device +X points almost straight up, was ${xInWorld[2]}", Math.abs(xInWorld[2] - 0.879) < 0.01)
        assertTrue("and slightly east, was ${xInWorld[0]}", Math.abs(xInWorld[0] - 0.146) < 0.01)
    }

    @Test
    fun aRotationVectorWithFiveElementsIgnoresTheAccuracyEstimate() {
        val four = floatArrayOf(0.33369225f, -0.38858578f, 0.52529025f, 0.67950034f)
        val five = HeadingFixtures.measuredOnTheStand
        val a = HeadingReference.attitude(four, HeadingAxis.SCREEN_BACK)
        val b = HeadingReference.attitude(five, HeadingAxis.SCREEN_BACK)
        assertEquals(a.headingDeg, b.headingDeg, 1e-9)
    }
}
