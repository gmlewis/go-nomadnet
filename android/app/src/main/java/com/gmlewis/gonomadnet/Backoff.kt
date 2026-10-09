// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import kotlin.math.min

/**
 * How long to wait before relaunching a daemon that died.
 *
 * A crash loop must not become a spin: the delay doubles on every consecutive
 * failure up to a ceiling, and after [maxConsecutiveFailures] the supervisor stops
 * trying and says so. A daemon that comes back up, or that stays up for
 * [healthyAfterMs], resets the policy, so an ordinary restart is not punished.
 */
class BackoffPolicy(
    private val initialDelayMs: Long = 500L,
    private val maxDelayMs: Long = 30_000L,
    private val healthyAfterMs: Long = 60_000L,
    private val maxConsecutiveFailures: Int = 6,
) {
    private var consecutiveFailures = 0
    private var startedAtMs = 0L

    /** Records that a daemon was started, and when. */
    fun recordStart(nowMs: Long) {
        startedAtMs = nowMs
    }

    /**
     * Records that a daemon exited, and returns how long to wait before the next
     * attempt, or null when the crash loop has gone on long enough to stop.
     */
    fun recordExit(nowMs: Long): Long? {
        if (startedAtMs != 0L && nowMs - startedAtMs >= healthyAfterMs) {
            consecutiveFailures = 0
        }
        consecutiveFailures += 1
        if (consecutiveFailures >= maxConsecutiveFailures) {
            return null
        }
        val shift = min(consecutiveFailures - 1, 20)
        return min(maxDelayMs, initialDelayMs shl shift)
    }

    /** How many consecutive failures have been seen. */
    fun failures(): Int = consecutiveFailures

    /** Forgets the failure history, as an explicit restart by the operator does. */
    fun reset() {
        consecutiveFailures = 0
        startedAtMs = 0L
    }
}
