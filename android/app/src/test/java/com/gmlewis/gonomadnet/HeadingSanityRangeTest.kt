// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Headings normalize into [0, 360), and the boundary at 359.9 to 0.0 carries no sign
 * or wrap artefact into the sample the appliance publishes. A heading that arrives
 * as -0.1 or 360.0 in the JSON would be rejected or misread by the consumer.
 */
class HeadingSanityRangeTest {

    @Test
    fun headingsWrapIntoTheFullTurn() {
        assertEquals(0.0, HeadingReference.normalizeDegrees(360.0), 1e-9)
        assertEquals(0.0, HeadingReference.normalizeDegrees(720.0), 1e-9)
        assertEquals(348.0, HeadingReference.normalizeDegrees(-12.0), 1e-9)
        assertEquals(12.0, HeadingReference.normalizeDegrees(372.0), 1e-9)
        assertEquals(359.9, HeadingReference.normalizeDegrees(359.9), 1e-9)
    }

    @Test
    fun theBoundaryBetweenAFullTurnAndZeroIsContinuous() {
        val justBelow = HeadingReference.normalizeDegrees(359.9)
        val justAbove = HeadingReference.normalizeDegrees(0.1)
        assertTrue("359.9 stays just below a full turn", justBelow > 359.0 && justBelow < 360.0)
        assertTrue("0.1 stays just above zero", justAbove > 0.0 && justAbove < 1.0)
        assertTrue("wrapping 359.9 forward lands on zero", HeadingReference.normalizeDegrees(justBelow + 0.2) < 1.0)
    }

    @Test
    fun everyHeadingTheAppliancePublishesIsInRange() {
        val samples = listOf(-540.0, -360.0, -0.1, 0.0, 359.9, 360.0, 360.1, 720.0, 1e6)
        for (value in samples) {
            val json = SensorJson.heading(1_700_000_000_000L, value)
            val heading = json.substringAfter("\"heading\":").substringBefore(',')
            val parsed = heading.toDouble()
            assertTrue("heading $value rendered as $parsed, which is out of range", parsed >= 0.0 && parsed < 360.0)
        }
    }
}
