// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * A crash loop must not become a spin. The delay grows, stays bounded, and the
 * supervisor eventually stops trying and says so instead of burning the battery.
 */
class BackoffPolicyTest {

    @Test
    fun theDelayGrowsAndStaysBounded() {
        val policy = BackoffPolicy(initialDelayMs = 500, maxDelayMs = 30_000, maxConsecutiveFailures = 10)
        val delays = (1..6).mapNotNull { policy.recordExit(0) }
        assertEquals(listOf(500L, 1000L, 2000L, 4000L, 8000L, 16_000L), delays)
        for (delay in delays) {
            assertTrue("a delay of $delay exceeded the ceiling", delay <= 30_000)
        }
    }

    @Test
    fun theCeilingHoldsNoMatterHowLongTheLoopRuns() {
        val policy = BackoffPolicy(initialDelayMs = 500, maxDelayMs = 30_000, maxConsecutiveFailures = 20)
        val delays = (1..18).mapNotNull { policy.recordExit(0) }
        assertEquals("the ceiling must be reached", 30_000L, delays.last())
        assertTrue(delays.all { it <= 30_000 })
    }

    @Test
    fun aCrashLoopEventuallyStopsRatherThanSpinningForever() {
        val policy = BackoffPolicy(initialDelayMs = 500, maxDelayMs = 30_000, maxConsecutiveFailures = 6)
        val delays = (1..5).mapNotNull { policy.recordExit(0) }
        assertEquals(5, delays.size)
        assertNull("the sixth consecutive failure must give up", policy.recordExit(0))
        assertNull("and stay given up", policy.recordExit(0))
    }

    @Test
    fun aDaemonThatStayedUpLongEnoughResetsThePolicy() {
        val policy = BackoffPolicy(initialDelayMs = 500, maxDelayMs = 30_000, healthyAfterMs = 60_000, maxConsecutiveFailures = 3)
        policy.recordStart(0)
        policy.recordExit(0)
        policy.recordStart(1000)
        // This one ran for two minutes before dying, which is a healthy daemon that
        // happened to stop, not a crash loop.
        assertEquals(500L, policy.recordExit(121_000))
        assertEquals("a healthy run clears the history", 1, policy.failures())
    }

    @Test
    fun anOperatorsExplicitRestartClearsTheHistory() {
        val policy = BackoffPolicy()
        repeat(3) { policy.recordExit(0) }
        assertEquals(3, policy.failures())
        policy.reset()
        assertEquals(0, policy.failures())
        assertEquals(500L, policy.recordExit(0))
    }
}
