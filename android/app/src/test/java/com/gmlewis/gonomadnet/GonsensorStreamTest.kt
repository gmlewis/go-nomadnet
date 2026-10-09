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
import java.io.InputStream
import java.io.OutputStream
import java.util.concurrent.CountDownLatch

/** A converter that is not there: its input and its fate are whatever the test decides. */
private class FakeConverter : ChildProcess {
    var input: OutputStream? = null
    var alive = true
    var destroyed = false

    override fun isAlive(): Boolean = alive

    override fun destroy(): Boolean {
        destroyed = true
        alive = false
        return true
    }

    override fun stdin(): OutputStream? = input

    override fun stdout(): InputStream? = null
}

/** An input that never accepts a byte, which is a converter that has stopped reading. */
private class StallingStream : OutputStream() {
    private val gate = CountDownLatch(1)

    fun release() {
        gate.countDown()
    }

    override fun write(b: Int) {
        gate.await()
    }
}

/** Waits up to a deadline for one newline-terminated line, rather than blocking forever. */
private fun awaitLine(source: InputStream, timeoutMs: Long = 5_000): String {
    val text = StringBuilder()
    val deadline = System.currentTimeMillis() + timeoutMs
    while (System.currentTimeMillis() < deadline) {
        while (source.available() > 0) {
            val next = source.read()
            if (next == '\n'.code) {
                return text.toString()
            }
            text.append(next.toChar())
        }
        Thread.sleep(5)
    }
    throw AssertionError("no line arrived within ${timeoutMs}ms; got ${text}")
}

/** Waits up to a deadline for a condition to hold, rather than blocking forever. */
private fun awaitUntil(timeoutMs: Long = 5_000, condition: () -> Boolean) {
    val deadline = System.currentTimeMillis() + timeoutMs
    while (System.currentTimeMillis() < deadline) {
        if (condition()) {
            return
        }
        Thread.sleep(5)
    }
    throw AssertionError("the condition never held within ${timeoutMs}ms")
}

/**
 * The hand-off of a sample to a converter.
 *
 * The converter is a separate process whose standard input is a pipe, and whose standard
 * output is a pipe the bot reads. That second pipe can have no reader for a long time —
 * the stack is started and stopped by hand, and the bot only exists while it runs — and
 * when it fills, the converter stops reading its input. Written by the caller, that is a
 * sensor callback parked inside a write for as long as the bot is away, which stalls the
 * sensor half and with it the loopback feed the client in Termux depends on.
 *
 * So the sample is handed over rather than written, and the tests below are about the two
 * properties that makes: the caller never waits, and what the converter does read is
 * exactly what was handed over, in order.
 */
class GonsensorStreamTest {

    @Test
    fun aSampleReachesTheConverterWhole() {
        val pipe = java.io.PipedOutputStream()
        val reader = java.io.PipedInputStream(pipe, 64 * 1024)
        val child = FakeConverter().also { it.input = pipe }
        val stream = GonsensorStream(child) { }

        stream.offer("""{"heading":47.5,"frame":"magnetic"}""")

        assertEquals(
            "the converter must read the sample as one line",
            """{"heading":47.5,"frame":"magnetic"}""",
            awaitLine(reader),
        )
        stream.close()
    }

    @Test
    fun samplesReachTheConverterInTheOrderTheyWereOffered() {
        val pipe = java.io.PipedOutputStream()
        val reader = java.io.PipedInputStream(pipe, 64 * 1024)
        val child = FakeConverter().also { it.input = pipe }
        val stream = GonsensorStream(child) { }

        stream.offer("first")
        stream.offer("second")
        stream.offer("third")

        assertEquals(listOf("first", "second", "third"), listOf(awaitLine(reader), awaitLine(reader), awaitLine(reader)))
        stream.close()
    }

    @Test
    fun aConverterThatStopsReadingNeverStallsTheCaller() {
        // The real shape of it: the pipe the converter writes is full because the bot is
        // not running, so the converter is parked inside its own write and takes nothing
        // more from its input. The sensors must not notice.
        val stalled = StallingStream()
        val child = FakeConverter().also { it.input = stalled }
        val stream = GonsensorStream(child) { }

        val started = System.nanoTime()
        assertTrue("the first sample must be taken", stream.offer("first"))
        repeat(500) { stream.offer("sample $it") }
        val elapsedMs = (System.nanoTime() - started) / 1_000_000

        assertTrue("handing over samples must never wait for the converter: ${elapsedMs}ms", elapsedMs < 1_000)
        assertTrue(
            "a sample that could not be passed on must be counted: ${stream.dropped}",
            stream.dropped > 0,
        )
        stalled.release()
        stream.close()
    }

    @Test
    fun aConverterThatHasGoneAwayIsReportedOnceAndThenLeftAlone() {
        val logs = mutableListOf<String>()
        val stream = GonsensorStream(FakeConverter()) { logs.add(it) }

        assertTrue("the first sample is taken", stream.offer("sample"))
        awaitUntil { logs.isNotEmpty() }

        assertTrue("the operator must be told what happened: ${logs}", logs[0].contains("its input is gone"))
        assertFalse("a stream with no converter takes nothing more", stream.offer("another"))
        assertEquals("and it must be reported once, not once per sample", 1, logs.size)
        stream.close()
    }

    @Test
    fun closingStopsTheConverterAndRefusesFurtherSamples() {
        val child = FakeConverter().also { it.input = java.io.ByteArrayOutputStream() }
        val stream = GonsensorStream(child) { }

        assertTrue(stream.offer("sample"))
        stream.close()

        assertTrue("the converter must be stopped", child.destroyed)
        assertFalse("a closed stream must take nothing more", stream.offer("another"))
    }
}
