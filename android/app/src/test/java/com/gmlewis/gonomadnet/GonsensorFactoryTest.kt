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
 * The order in which a converter and the pipe it writes come into being.
 *
 * It is the whole of the bug that was found on the tablet, and it is invisible until it is
 * fatal. A converter's standard output is redirected to `files/run/gps.nmea` at the moment
 * it is spawned. If the path is not a pipe then, the redirect creates a plain file, and the
 * pipe is made afterwards by deleting what is there — which is, from the converter's side,
 * its own output vanishing. The converter keeps writing into an inode nothing can reach,
 * the bot reads a pipe no process will ever write, and `whereami` answers "no GNSS fix yet"
 * for as long as the appliance runs. It was confirmed on the tablet: both converters held
 * `files/run/gps.nmea (deleted)` and `files/run/compass.nmea (deleted)`.
 *
 * The pipe therefore belongs to whoever spawns the writers, and it is made and held before
 * the first of them exists.
 */
class GonsensorFactoryTest {

    private val paths = StackPaths(
        java.nio.file.Files.createTempDirectory("gonomadnet-gonsensor").toFile().absolutePath,
    )
    private val runner = RecordingProcessRunner()
    private val events = runner.events
    private val fifos = RecordingFifos(paths, events)
    private val logs = mutableListOf<String>()

    private fun factory(): GonsensorFactory =
        GonsensorFactory(runner, "/lib/arm64/libgonsensor.so", paths, fifos, log = { logs.add(it) })

    @Test
    fun thePipeIsMadeBeforeAConverterIsSpawnedAgainstIt() {
        val factory = factory()

        factory.positionOnly()
        factory.headingOnly()

        val madeAt = events.indexOfFirst { it.startsWith("fifo:open:") }
        assertTrue("the pipes must be made: ${events}", madeAt >= 0)
        for (spawn in events.filter { it.startsWith("spawnWritingTo:") }) {
            assertTrue(
                "a converter must never be spawned against a path that is not yet a pipe: $spawn in ${events}",
                events.indexOfFirst { it == spawn } > madeAt,
            )
        }
        assertEquals(
            "and both of them must be writing the pipes this holder made",
            2,
            events.count { it.startsWith("spawnWritingTo:") },
        )
        factory.close()
    }

    @Test
    fun thePipeIsMadeOnceHoweverManyConvertersWriteIt() {
        val factory = factory()

        factory.positionOnly()
        factory.headingOnly()
        factory.positionOnly()

        assertEquals(
            "making the same pipe three times is making it once: ${events}",
            1,
            events.count { it.startsWith("fifo:open:") },
        )
        factory.close()
    }

    @Test
    fun thePipeIsHeldForAsLongAsAConverterWritesIt() {
        val factory = factory()

        factory.positionOnly()
        factory.headingOnly()

        assertTrue("the pipes must be held", fifos.opened)
        assertFalse(
            "a pipe released while a converter writes it is a pipe whose writer is orphaned",
            fifos.closed,
        )
        factory.close()
        assertTrue("and released once nothing writes it any more", fifos.closed)
    }

    @Test
    fun theUnfilteredConverterNeedsNoPipeAtAll() {
        // The socket subscribers are handed the converter's own standard output, so there
        // is no pipe for anybody to make, and making two anyway would leave a pair of pipes
        // nothing writes.
        val factory = factory()

        factory.piped()

        assertFalse("nothing writes a pipe here: ${events}", fifos.opened)
        factory.close()
        assertFalse("so there is nothing for closing to release", fifos.closed)
    }

    @Test
    fun aPipeThatCouldNotBeMadeIsReportedRatherThanPassedOver() {
        fifos.succeed = false
        val factory = factory()

        factory.positionOnly()

        assertTrue(
            "a converter spawned against a plain file reports history as though it were live: ${logs}",
            logs.any { it.contains("could not make the sensor pipes") },
        )
        factory.close()
    }
}
