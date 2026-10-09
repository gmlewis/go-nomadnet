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

/** A sink that records what it was given. */
class FakeSink(val name: String, val events: MutableList<String>) : SampleSink {
    val samples = mutableListOf<String>()
    var closed = false

    override fun offer(line: String): Boolean {
        samples.add(line)
        return true
    }

    override fun close() {
        closed = true
        events.add("sink:close:$name")
    }
}

/** A source that can be started and turned off. */
class FakeSource(val name: String, val events: MutableList<String>, var available: Boolean = true) : SampleSource {
    var started = false
        private set
    var closed = false
        private set

    override fun start(): Boolean {
        started = available
        events.add("source:start:$name")
        return available
    }

    override fun close() {
        closed = true
        events.add("source:close:$name")
    }
}

/** A feed that records the port it was asked for and everything handed to it. */
class FakeFeed(val events: MutableList<String>, var available: Boolean = true) : FeedPublisher {
    var port: Int? = null
    var closed = false
        private set
    val published = java.io.ByteArrayOutputStream()

    override fun start(port: Int): Boolean {
        this.port = port
        events.add("feed:start:$port")
        return available
    }

    override fun offer(bytes: ByteArray) {
        published.write(bytes)
    }

    override fun close() {
        closed = true
        events.add("feed:close")
    }
}

/**
 * A broadcast sink whose converter output comes from a pipe, so a test can prove that what
 * the converter writes actually reaches the socket. Without the pump the feed accepts a
 * connection and then sends nothing, which looks exactly like a client that is waiting.
 */
class FakeBroadcastSink(private val events: MutableList<String>) : BroadcastSink {
    val samples = mutableListOf<String>()
    var closed = false
    private val source = java.io.PipedInputStream(64 * 1024)
    private val sink = java.io.PipedOutputStream(source)

    override fun offer(line: String): Boolean {
        samples.add(line)
        return true
    }

    override fun sentences(): java.io.InputStream = source

    /** Stands in for the converter writing a sentence to its standard output. */
    fun emit(text: String) {
        sink.write(text.toByteArray())
        sink.flush()
    }

    override fun close() {
        closed = true
        events.add("sink:close:broadcast")
        runCatching { sink.close() }
        runCatching { source.close() }
    }
}

/** A wake lock that records whether it is held. */
class FakeWakeLock(val events: MutableList<String>) : WakeLockHandle {
    private var held = false

    override fun acquire() {
        if (!held) {
            held = true
            events.add("wake:acquire")
        }
    }

    override fun release() {
        if (held) {
            held = false
            events.add("wake:release")
        }
    }

    override fun isHeld(): Boolean = held
}

/** Everything one pipeline is wired to. */
class PipelineFixture {
    val events = mutableListOf<String>()
    val position = FakeSink("position", events)
    val heading = FakeSink("heading", events)
    val broadcast = FakeBroadcastSink(events)
    val feed = FakeFeed(events)
    val location = FakeSource("location", events)
    val compass = FakeSource("heading", events)
    val wakeLock = FakeWakeLock(events)

    fun pipeline(): SensorPipeline = SensorPipeline(
        positionSink = position,
        headingSink = heading,
        broadcastSink = broadcast,
        feed = feed,
        location = location,
        heading = compass,
        wakeLock = wakeLock,
        log = { events.add("log:$it") },
    )
}

/**
 * The two axes of the design, and their independence.
 *
 * The sensor service decides whether the tablet knows where it is. The daemon stack decides
 * who owns Reticulum. They are orthogonal, and the whole reason to state that in a test is
 * that conflating them is the main way to get the design wrong: it would take a working
 * standalone client with a real position and make it depend on a bot that is not running.
 */
class SensorPipelineTest {

    // ---- Axis 2: the sensors run alone -----------------------------------------

    @Test
    fun theConvertersSentencesReachTheLoopbackSubscribers() {
        // The feed accepting a connection is not the same as the feed saying anything. This is
        // the assertion that would have caught the socket that hung: without the pump the
        // subscriber sees no bytes and no error.
        val fixture = PipelineFixture()
        val pipeline = fixture.pipeline()
        pipeline.start()

        // A literal dollar sign, because these are NMEA sentences and Kotlin would otherwise
        // read the GNRMC tag as a template.
        fixture.broadcast.emit("\$GNRMC,211128.006,A,2824.9276,N,08138.5697,W,0,0,081026,,,A*69\r\n")
        fixture.broadcast.emit("\$HCHDM,47.5,M*1F\r\n")

        val deadline = System.currentTimeMillis() + 5_000
        while (System.currentTimeMillis() < deadline &&
            !fixture.feed.published.toString(Charsets.US_ASCII).contains("HCHDM")
        ) {
            Thread.sleep(10)
        }
        val published = fixture.feed.published.toString(Charsets.US_ASCII)
        assertTrue("the subscribers must receive the position sentence: \$published", published.contains("GNRMC"))
        assertTrue("and the heading sentence: \$published", published.contains("HCHDM"))
    }

    @Test
    fun stoppingStopsThePump() {
        val fixture = PipelineFixture()
        val pipeline = fixture.pipeline()
        pipeline.start()

        fixture.broadcast.emit("\$HCHDM,47.5,M*1F\r\n")
        val deadline = System.currentTimeMillis() + 5_000
        while (System.currentTimeMillis() < deadline &&
            !fixture.feed.published.toString(Charsets.US_ASCII).contains("HCHDM")
        ) {
            Thread.sleep(10)
        }
        val afterStart = fixture.feed.published.size()

        pipeline.stop()
        Thread.sleep(300)
        assertEquals(
            "a stopped pipeline must not keep publishing",
            afterStart,
            fixture.feed.published.size(),
        )
    }

    @Test
    fun theFeedPortDoesNotCollideWithReticulum() {
        // Reticulum's own defaults are localInterfacePort 37428 (the shared instance) and
        // localControlPort 37429 (its control channel). Publishing the sensor feed on either
        // of them makes the transport die with
        //     listen tcp 127.0.0.1:37429: bind: address already in use
        // which the supervisor reports only as a daemon that keeps restarting.
        for (reserved in SensorFeedServer.RESERVED_RETICULUM_PORTS) {
            assertFalse(
                "the sensor feed must not take Reticulum's port $reserved",
                SensorFeedServer.DEFAULT_FEED_PORT == reserved,
            )
        }
        assertFalse(
            "the feed must not take the shared instance port either",
            SensorFeedServer.DEFAULT_FEED_PORT == NodeConfigSpec.DEFAULT_SHARED_INSTANCE_PORT,
        )
        assertEquals(37430, SensorFeedServer.DEFAULT_FEED_PORT)
    }

    @Test
    fun theSensorsRunWithNoStackAtAll() {
        val fixture = PipelineFixture()
        val pipeline = fixture.pipeline()

        pipeline.start()

        assertTrue("the sensors must run without a stack", pipeline.isRunning)
        assertTrue("the location source must be reading", fixture.location.started)
        assertTrue("the compass must be reading", fixture.compass.started)
        assertTrue("the loopback feed must be published", pipeline.isFeeding)
        assertEquals("on the port both sides pin", SensorFeedServer.DEFAULT_FEED_PORT, fixture.feed.port)
        assertTrue("a non-wakeup sensor needs a wake lock", fixture.wakeLock.isHeld())
    }

    @Test
    fun aStartedPipelineOffersItsSamplesToTheBotAndToTheFeed() {
        val fixture = PipelineFixture()
        val pipeline = fixture.pipeline()
        pipeline.start()

        val line = """{"t":"2026-10-08T15:53:40.123Z","heading":47.5,"frame":"magnetic"}"""
        pipeline.offer(line)

        assertEquals(listOf(line), fixture.position.samples)
        assertEquals(listOf(line), fixture.heading.samples)
        assertEquals(listOf(line), fixture.broadcast.samples)
    }

    @Test
    fun stoppingTheStackLeavesTheSensorsExactlyAsTheyWere() {
        // This is the §4.1 Axis 2 guarantee. Stopping the daemons must not disturb the
        // sensors, because in Mode A there are no daemons and the sensors are the only
        // reason the client can do anything with a position at all.
        val fixture = PipelineFixture()
        val pipeline = fixture.pipeline()
        val supervisor = SupervisorFixture().supervisor()

        pipeline.start()
        assertTrue(supervisor.start())
        val before = fixture.events.toList()

        supervisor.stop()

        assertTrue("the sensors must still be running", pipeline.isRunning)
        assertTrue("the loopback feed must still be listening", pipeline.isFeeding)
        assertFalse("the location source must not have been closed", fixture.location.closed)
        assertFalse("the compass must not have been closed", fixture.compass.closed)
        assertTrue("the wake lock must still be held", fixture.wakeLock.isHeld())
        assertFalse("the feed must not have been closed", fixture.feed.closed)
        assertEquals(
            "stopping the stack must touch nothing on the sensor side",
            before,
            fixture.events,
        )
    }

    @Test
    fun stoppingTheSensorsLeavesTheStackExactlyAsItWas() {
        val fixture = PipelineFixture()
        val pipeline = fixture.pipeline()
        val supervisorFixture = SupervisorFixture()
        val supervisor = supervisorFixture.supervisor()

        assertTrue(supervisor.start())
        pipeline.start()
        val before = supervisorFixture.runner.events.toList()

        pipeline.stop()
        supervisor.poll(nowMs = 1_000)

        assertEquals(
            "the daemons must still be running, in their start order",
            listOf("gornsd", "gorrcd", "gorrcbot"),
            supervisor.running(),
        )
        assertEquals(
            "stopping the sensors must touch nothing on the stack side",
            before,
            supervisorFixture.runner.events,
        )
    }

    @Test
    fun aPipelineWithNoSensorsAvailableStillRunsAndOnlyTheSensorsAreMissing() {
        // A device with no rotation vector, or a user who refused the location permission,
        // still gets a working appliance. What is lost is the heading, or the position —
        // never the service.
        val fixture = PipelineFixture()
        fixture.location.available = false
        fixture.compass.available = false
        val pipeline = fixture.pipeline()

        pipeline.start()

        assertTrue("the pipeline still runs", pipeline.isRunning)
        assertTrue(
            "the operator must be told why there is no position: ${fixture.events}",
            fixture.events.any { it.contains("no usable location provider") },
        )
        assertTrue(
            "the operator must be told why there is no heading: ${fixture.events}",
            fixture.events.any { it.contains("no rotation vector") },
        )
    }

    @Test
    fun stoppingReleasesEverythingThePipelineHeld() {
        val fixture = PipelineFixture()
        val pipeline = fixture.pipeline()
        pipeline.start()

        pipeline.stop()

        assertFalse(pipeline.isRunning)
        assertFalse(pipeline.isFeeding)
        assertTrue("the location source must be released", fixture.location.closed)
        assertTrue("the compass must be released", fixture.compass.closed)
        assertTrue("the feed must be released", fixture.feed.closed)
        assertTrue("every converter must be released", fixture.position.closed && fixture.heading.closed && fixture.broadcast.closed)
        assertFalse("the wake lock must be released", fixture.wakeLock.isHeld())
    }

    @Test
    fun startingTwiceDoesNotRegisterEverythingTwice() {
        val fixture = PipelineFixture()
        val pipeline = fixture.pipeline()

        pipeline.start()
        pipeline.start()

        assertEquals("the feed must be started once", 1, fixture.events.count { it.startsWith("feed:start:") })
        assertEquals("the wake lock must be acquired once", 1, fixture.events.count { it == "wake:acquire" })
    }

    @Test
    fun stoppingTwiceIsSafe() {
        val fixture = PipelineFixture()
        val pipeline = fixture.pipeline()
        pipeline.start()

        pipeline.stop()
        pipeline.stop()

        assertEquals("the feed must be closed once", 1, fixture.events.count { it == "feed:close" })
    }

    @Test
    fun aSampleOfferedWhileStoppedGoesNowhere() {
        val fixture = PipelineFixture()
        val pipeline = fixture.pipeline()

        pipeline.offer("""{"heading":47.5}""")

        assertTrue("a stopped pipeline must not reach its converters", fixture.position.samples.isEmpty())
        assertTrue(fixture.broadcast.samples.isEmpty())
    }
}
