// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File
import java.io.IOException
import java.io.PipedInputStream
import java.io.PipedOutputStream

/**
 * The bridge's ordering is the whole of whether a radio works: the device is claimed before
 * the bridge is started, the path is published before the transport is configured, and both
 * ends are released together when the stack comes down. None of that can be observed with a
 * real USB device in a unit test, so it is observed here with fakes.
 */
class RNodeBridgeTest {

    @Test
    fun `the pump carries the radio's bytes both ways until an end closes`() {
        val radio = TestStream()
        val socket = TestStream()
        val pump = RadioPump()

        pump.start(radio, socket)
        try {
            // The radio's bytes reach the appliance.
            radio.offer("from the radio")
            assertEquals("from the radio", socket.take(14))

            // And what the appliance sends reaches the radio.
            socket.offer("to the radio")
            assertEquals("to the radio", radio.take(12))
        } finally {
            pump.stop()
        }

        assertFalse("the pump outlived both of its ends", pump.running())
    }

    @Test
    fun `the pump ends when either end closes, and closes the other`() {
        val radio = TestStream()
        val socket = TestStream()
        val pump = RadioPump()
        pump.start(radio, socket)

        // The radio going away is what unplugging it looks like, and it has to release the
        // bridge rather than leave two threads reading a device that is gone.
        radio.close()

        assertTrue("the pump did not notice its radio going away", await { !pump.running() })
        pump.stop()
    }

    @Test
    fun `the bridge claims the radio before it starts the bridge program`() {
        val fixture = BridgeFixture()
        val radio = fixture.attachedRadio()

        assertTrue("the bridge did not come up: ${fixture.logs}", fixture.bridge().start())

        assertEquals(
            "the radio bridge was started before the device was claimed",
            listOf("open-radio", "spawn:gornnode"),
            fixture.events.take(2),
        )
        assertEquals("/dev/pts/7", fixture.publishedTty())
    }

    @Test
    fun `the bridge is handed the socket name the appliance will dial`() {
        val fixture = BridgeFixture()
        fixture.attachedRadio()
        val bridge = fixture.bridge()
        assertTrue(bridge.start())

        val spec = fixture.runner.specs.single()
        assertEquals("gornnode", spec.name)
        assertTrue(
            "the bridge is not told where to write the radio's serial path: ${spec.argv}",
            spec.argv.contains(fixture.paths.rnodeTtyFile),
        )
        assertTrue(
            "the bridge is not told the socket the appliance dials: ${spec.argv}",
            spec.argv.contains(fixture.socketName),
        )
    }

    @Test
    fun `a bridge that never publishes a path is put down rather than left running`() {
        // Without a published path there is nothing for the transport to dial, and a bridge
        // left running against no radio holds the USB device for the life of the install.
        val fixture = BridgeFixture()
        fixture.attachedRadio()
        // Nothing writes the file, so the wait ends at once rather than after its polls.
        File(fixture.paths.rnodeTtyFile).delete()

        val bridge = fixture.bridge(awaitAttempts = 0)
        assertFalse("a bridge with nothing to dial was reported as up", bridge.start())
        assertNull(bridge.publishedTty)
        assertTrue("the bridge program was left running", fixture.events.contains("destroy:gornnode"))
        assertTrue("the radio was left claimed", fixture.attached!!.released)
        assertTrue("the bridge's socket was never dialled, so none was left open", fixture.dialled == null)
    }

    @Test
    fun `an appliance with no radio attached starts no bridge program at all`() {
        // An appliance with no radio is an appliance with a hub. It is not a failure, and it is
        // not a bridge started against a device that is not there: the transport is rendered
        // with no radio at all rather than one enabled against a path that names nothing.
        val fixture = BridgeFixture()
        val bridge = fixture.bridge()
        assertFalse(bridge.start())
        assertNull(bridge.publishedTty)
        assertTrue("a bridge program was started with no radio", fixture.runner.specs.isEmpty())
        assertTrue(fixture.events.none { it.startsWith("open-radio") })
    }

    @Test
    fun `stopping the bridge releases the radio and the socket and stops the program`() {
        val fixture = BridgeFixture()
        val radio = fixture.attachedRadio()
        val bridge = fixture.bridge()
        assertTrue(bridge.start())

        val socket = fixture.dialled!!.single()
        bridge.stop()

        assertTrue("the radio was left claimed", radio.released)
        assertTrue("the bridge's socket was left open", socket.closed)
        assertTrue("the bridge program was left running", fixture.events.contains("destroy:gornnode"))
        assertNull("a stopped bridge still names a serial path", bridge.publishedTty)
    }

    @Test
    fun `await gives up on the last look rather than sleeping after it`() {
        var sleeps = 0
        val path = PublishedPath.await(
            path = "/nonexistent/tty",
            attempts = 3,
            waitMs = 1,
            read = { null },
            sleep = { sleeps += 1 },
        )
        assertNull(path)
        // Sleeping after the final attempt would only postpone the report.
        assertEquals("the wait slept after it had nothing left to look at", 2, sleeps)
    }

    @Test
    fun `await returns what the bridge published`() {
        val dir = java.nio.file.Files.createTempDirectory("gonomadnet-rnode-publish").toFile()
        val path = File(dir, "tty")
        path.writeText("/dev/pts/9\n")
        assertEquals("/dev/pts/9", PublishedPath.await(path.absolutePath, attempts = 1, sleep = { }))
        // A file that has been truncated, or written empty, is not a path the transport can
        // be dialled at.
        path.writeText("")
        assertNull(PublishedPath.await(path.absolutePath, attempts = 1, sleep = { }))
    }

    /** await waits for a condition, for a bound. It is a settle, not a delay. */
    private fun await(timeoutMs: Long = 5_000, condition: () -> Boolean): Boolean {
        val deadline = System.currentTimeMillis() + timeoutMs
        while (System.currentTimeMillis() < deadline) {
            if (condition()) {
                return true
            }
            Thread.sleep(1)
        }
        return condition()
    }
}

/**
 * A byte stream a test can carry bytes through.
 *
 * Each direction is a pipe: the test writes into one end and reads out of the other, so a
 * byte put in at one side is what a reader on the other side sees, with nothing in between
 * but the code under test.
 */
class TestStream : ByteStream {

    private val arrival = PipedOutputStream()
    private val reader = PipedInputStream(arrival, PIPE_BYTES)
    private val departure = PipedInputStream(PIPE_BYTES)
    private val writer = PipedOutputStream(departure)

    /** Whether this end has been closed, which is what releases the radio or the socket. */
    var closed = false
        private set

    /** offer makes this stream's reader see these bytes. */
    fun offer(text: String) {
        arrival.write(text.toByteArray())
        arrival.flush()
    }

    /** take returns the next [count] bytes this stream has written. */
    fun take(count: Int): String {
        val buffer = ByteArray(count)
        var read = 0
        while (read < count) {
            val n = departure.read(buffer, read, count - read)
            if (n < 0) {
                break
            }
            read += n
        }
        return String(buffer, 0, read)
    }

    override fun read(into: ByteArray): Int = try {
        val n = reader.read(into)
        n
    } catch (failure: IOException) {
        -1
    }

    override fun write(bytes: ByteArray, length: Int): Boolean = try {
        writer.write(bytes, 0, length)
        writer.flush()
        true
    } catch (failure: IOException) {
        false
    }

    override fun close() {
        closed = true
        runCatching { reader.close() }
        runCatching { writer.close() }
        runCatching { arrival.close() }
        runCatching { departure.close() }
    }

    companion object {
        private const val PIPE_BYTES = 64 * 1024
    }
}

/**
 * A radio the appliance has been given.
 *
 * It records the claim in the same event log as the process starts, which is how the order
 * of the two is asserted, and it reports whether the link it handed over was released — the
 * USB claim going back to the system is what that close is.
 */
class FakeRadio(private val events: MutableList<String>) : RadioDevice {

    private var link: TestStream? = null

    /** Whether the endpoints and the device claim have been released. */
    val released: Boolean get() = link?.closed == true

    override val description: String get() = "RNode on 303a:1001"

    override fun open(): ByteStream? {
        events.add("open-radio")
        return TestStream().also { link = it }
    }
}

/** Everything the bridge needs, wired to recorders and to a radio that is always attached. */
class BridgeFixture(
    val paths: StackPaths = StackPaths(
        java.nio.file.Files.createTempDirectory("gonomadnet-rnode-bridge").toFile().absolutePath,
    ),
) {
    val runner = RecordingProcessRunner()
    val events: MutableList<String> = runner.events
    val logs = mutableListOf<String>()
    val socketName = RNodeSocket.name(4242, ByteArray(RNodeSocket.ENTROPY_BYTES) { 0xab.toByte() })
    var dialled: MutableList<TestStream>? = null

    /** The radio the fixture attached, if one was. */
    var attached: FakeRadio? = null
        private set

    /** Attaches the radio, and arranges for the bridge to publish a serial path. */
    fun attachedRadio(): FakeRadio {
        val radio = FakeRadio(events)
        attached = radio
        File(paths.rnodeTtyFile).also { it.parentFile?.mkdirs() }.writeText("/dev/pts/7")
        return radio
    }

    /** publishedTty is what the appliance must render the transport against. */
    fun publishedTty(): String? = File(paths.rnodeTtyFile).takeIf { it.isFile }?.readText()?.trim()

    fun bridge(
        awaitAttempts: Int = PublishedPath.ATTEMPTS,
    ): RNodeBridge = RNodeBridge(
        runner = runner,
        finder = object : RadioFinder {
            override fun find(): RadioDevice? = attached
        },
        socketName = { socketName },
        bridgeSpec = { name -> LaunchSpecs.rnodeBridge("/data/lib", paths, name) },
        dial = { name ->
            assertEquals("the bridge dialled a socket it was not told about", socketName, name)
            TestStream().also { stream ->
                dialled = (dialled ?: mutableListOf<TestStream>()).also { it.add(stream) }
            }
        },
        ttyFile = paths.rnodeTtyFile,
        log = { logs.add(it) },
        sleep = { },
        awaitAttempts = awaitAttempts,
    )
}
