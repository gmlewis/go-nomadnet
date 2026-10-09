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
 * The one decision the pipe holder makes, which is the only part of it that can be examined
 * without a device: `android.system.Os` is a stub on the JVM, so the file-type test itself is
 * made of `Os.lstat` and `OsConstants.S_ISFIFO` and everything downstream of it is asserted
 * here.
 *
 * The decision matters more than it looks. A path that exists but is not a pipe is opened
 * happily as a file, and the bot then reads it from offset zero — the history of every
 * sample ever written — and comes to rest on the last line in it. A heading read that way
 * never changes again, and it is reported as if it were live. It was found on the tablet:
 * `files/run/compass.nmea` was a regular file, and `whereami` reported the same frozen
 * `212° True` across requests two minutes apart.
 */
class SensorFifosTest {

    @Test
    fun aPathThatIsNotThereYetIsCreated() {
        assertEquals(
            "a path that does not exist must be made as a pipe",
            FifoPathAction.CREATE,
            planFifoPath(exists = false, isFifo = false),
        )
    }

    @Test
    fun anHonestPipeIsLeftAlone() {
        // The ordinary case: the last run's pipe is still there, or another instance of the
        // service made it a moment ago. Deleting it would be the one destructive choice
        // available here, so it is reserved for the case that needs it.
        assertEquals(
            "an existing pipe must be used as it is",
            FifoPathAction.USE_AS_IS,
            planFifoPath(exists = true, isFifo = true),
        )
    }

    @Test
    fun aFileThatIsNotAPipeIsReplacedRatherThanOpened() {
        assertEquals(
            "a path that exists as something other than a pipe must be replaced",
            FifoPathAction.REPLACE,
            planFifoPath(exists = true, isFifo = false),
        )
    }

    @Test
    fun nothingButAPipeIsEverOpenedAsAStream() {
        // Stated as a property rather than as a table, because the rule is not about any one
        // combination: the only path that is opened without being made is the one that is
        // already a pipe.
        for (exists in listOf(true, false)) {
            for (isFifo in listOf(true, false)) {
                val action = planFifoPath(exists = exists, isFifo = isFifo)
                if (action == FifoPathAction.USE_AS_IS) {
                    assertTrue("only a pipe may be used as it is", exists && isFifo)
                }
            }
        }
    }
}
