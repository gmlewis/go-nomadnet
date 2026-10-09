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
 * The sample stream is the boundary between Android and Go, so it has to be exactly
 * the shape gonsensor documents. A sample that does not parse is a reading that
 * never reaches the bot, silently.
 */
class SensorJsonTest {

    @Test
    fun aFixSerializesToTheDocumentedLine() {
        val line = SensorJson.location(
            timeMs = 1_791_491_980_123L,
            provider = "gps",
            lat = 35.123456,
            lng = -106.567890,
            altitude = 1620.5,
            accuracyMeters = 3.8f,
            speedKnots = 0.4,
            courseDeg = 271.3,
            satellites = 8,
            quality = 1,
        )
        assertEquals(
            "{\"t\":\"2026-10-08T20:39:40.123Z\",\"provider\":\"gps\",\"valid\":true," +
                "\"lat\":35.123456,\"lng\":-106.56789,\"alt\":1620.5,\"acc\":3.8," +
                "\"speed\":0.4,\"course\":271.3,\"sats\":8,\"quality\":1}",
            line,
        )
    }

    @Test
    fun aHeadingSerializesToTheDocumentedLine() {
        assertEquals(
            "{\"t\":\"2026-10-08T20:39:40.123Z\",\"heading\":47.5,\"frame\":\"magnetic\"}",
            SensorJson.heading(1_791_491_980_123L, 47.5),
        )
        assertEquals(
            "{\"t\":\"2026-10-08T20:39:40.123Z\",\"heading\":47.5,\"frame\":\"true\"}",
            SensorJson.heading(1_791_491_980_123L, 47.5, SensorJson.FRAME_TRUE),
        )
    }

    @Test
    fun anUnmeasuredFieldIsAbsentRatherThanZero() {
        // An altitude of zero is sea level and a speed of zero is standing still;
        // neither is the same as "nobody measured it".
        val line = SensorJson.location(1_791_491_980_123L, "gps", 35.0, -106.0)
        assertFalse("no altitude should be claimed", line.contains("\"alt\""))
        assertFalse("no accuracy should be claimed", line.contains("\"acc\""))
        assertFalse("no speed should be claimed", line.contains("\"speed\""))
        assertFalse("no satellite count should be claimed", line.contains("\"sats\""))
        assertTrue(line.contains("\"valid\":true"))
    }

    @Test
    fun aPositionThatIsNotAFixIsMarkedInvalidRatherThanDropped() {
        // The consumer has to be told there is no fix, or it keeps the last one.
        for ((lat, lng) in listOf(
            Double.NaN to -106.0,
            Double.POSITIVE_INFINITY to -106.0,
            91.0 to -106.0,
            35.0 to 181.0,
            -91.0 to 181.0,
        )) {
            val line = SensorJson.location(1_791_491_980_123L, "gps", lat, lng)
            assertTrue("a bad position must be published as invalid: ", line.contains("\"valid\":false"))
            assertFalse("a bad position must not be published: ", line.contains("\"lat\""))
        }
    }

    @Test
    fun anExplicitlyInvalidFixIsMarkedInvalid() {
        val line = SensorJson.location(1_791_491_980_123L, "gps", 35.0, -106.0, valid = false)
        assertTrue(line.contains("\"valid\":false"))
        assertFalse(line.contains("\"lat\""))
    }

    @Test
    fun aHeadingThatCannotBeTrustedIsWithheldRatherThanGuessed() {
        // The producer's way of saying "I am not confident enough to answer" is to
        // withhold the field. The consumer must receive no heading at all.
        for (bad in listOf(null, Double.NaN, Double.POSITIVE_INFINITY)) {
            val line = SensorJson.heading(1_791_491_980_123L, bad)
            assertFalse("a heading of  must not be published: ", line.contains("\"heading\""))
            assertTrue("the sample is still a sample: ", line.startsWith("{\"t\":\""))
        }
    }

    @Test
    fun aProviderNameIsEscapedRatherThanBreakingTheLine() {
        val provider = "gp\"s\\x"
        val line = SensorJson.location(1_791_498_012_123L, provider, 35.0, -106.0)
        // A quote in the provider must not end the string, and a backslash must not
        // start an escape. Both are written escaped, which is the only form a JSON
        // reader can accept.
        val escaped = "gp\\\"s\\\\x"
        assertTrue("the provider must be escaped: $line", line.contains(escaped))
    }

    @Test
    fun timestampsAreRfc3339WithMillisecondsInUtc() {
        assertEquals("1970-01-01T00:00:00.000Z", SensorJson.rfc3339(0L))
        assertEquals("2026-10-08T15:53:40.123Z", SensorJson.rfc3339(1_791_474_820_123L))
    }
}
