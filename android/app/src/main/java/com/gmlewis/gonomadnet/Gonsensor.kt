// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import java.io.Closeable
import java.io.OutputStream

/**
 * One `gonsensor` process and the JSON stream fed into it.
 *
 * All the protocol logic stays in Go, where the parser that consumes the sentences
 * lives, so the emitter and the parser are tested against each other by the repository
 * that owns both. Kotlin's whole job is to turn platform objects into JSON lines.
 *
 * A sample is handed over rather than written by the caller, because the converter's own
 * output is a pipe the bot reads, and the bot is not always running. With nobody at the
 * read end that pipe fills, the converter stops reading its input, and a caller that
 * wrote the sample itself would then be parked inside a write for as long as the bot is
 * away — which is a stalled sensor callback, and with it a stalled loopback feed, in a
 * service whose whole job is to keep producing. So the hand-off is to a bounded queue with
 * a thread of its own, and a sample offered while that queue is full is dropped and
 * counted: a reading nobody can take is not worth stalling the sensors for.
 *
 * The queue is deep enough to absorb a converter that is merely between reads and shallow
 * enough that a converter which has stopped reading altogether cannot hold a minute of
 * samples. Dropping the newest rather than the oldest keeps whatever does get through in
 * the order it was taken.
 */
class GonsensorStream internal constructor(
    private val process: ChildProcess,
    private val log: (String) -> Unit,
) : BroadcastSink {
    private val lock = Object()

    /** The samples waiting to be written, oldest first. */
    private val pending = java.util.ArrayDeque<String>()

    /** Whether the converter has gone away for good. */
    private var broken = false

    /** How many samples were dropped because the converter was not keeping up. */
    var dropped: Int = 0
        private set

    // The writer is a daemon thread so that nothing here can outlive the service.
    private val writer = Thread(::drain, "gonsensor-stdin").apply {
        isDaemon = true
        start()
    }

    /** Hands over one sample, and reports whether it was taken. */
    override fun offer(line: String): Boolean = synchronized(lock) {
        if (broken) {
            return false
        }
        if (pending.size >= MAX_PENDING_SAMPLES) {
            dropped += 1
            return false
        }
        pending.addLast(line)
        lock.notifyAll()
        true
    }

    /** Writes every sample that is handed over, until the converter is gone. */
    private fun drain() {
        while (true) {
            val line = synchronized(lock) {
                while (pending.isEmpty() && !broken) {
                    try {
                        lock.wait()
                    } catch (interrupted: InterruptedException) {
                        Thread.currentThread().interrupt()
                        return
                    }
                }
                if (broken || pending.isEmpty()) {
                    return
                }
                pending.removeFirst()
            }
            if (!write(line)) {
                return
            }
        }
    }

    /** Writes one sample, and reports whether the converter is still there to take it. */
    private fun write(line: String): Boolean {
        val sink = process.stdin() ?: return fail("its input is gone")
        return try {
            sink.write((line + "\n").toByteArray())
            sink.flush()
            true
        } catch (failure: Throwable) {
            fail("the sensor converter stopped accepting samples: ${failure.message}")
        }
    }

    /**
     * Reports that the converter is gone, once, and refuses everything after it.
     *
     * A converter that has gone away is reported and then ignored: a sensor stream is a
     * best-effort thing, and a line that cannot be written is a reading nobody will see,
     * not a crash.
     */
    private fun fail(reason: String): Boolean {
        synchronized(lock) {
            if (broken) {
                return false
            }
            broken = true
            lock.notifyAll()
        }
        log(reason)
        return false
    }

    /** Its standard output, when it was started with a readable one. */
    override fun sentences(): java.io.InputStream? = process.stdout()

    /** Stops the converter, and stops handing samples to it. */
    override fun close() {
        synchronized(lock) {
            broken = true
            lock.notifyAll()
        }
        // Destroying it is also what frees a writer parked on a pipe nobody reads, so the
        // thread is reaped rather than left waiting; the wait is bounded so that a child
        // that will not die cannot hold up the service.
        process.destroy()
        runCatching { writer.join(WRITER_JOIN_MS) }
    }

    private companion object {
        /**
         * How many samples may wait for the converter.
         *
         * A converter reading normally takes each one immediately, so this depth is only
         * ever reached by one that has stopped: at fifty samples a second it holds rather
         * more than a second of readings, and no more.
         */
        const val MAX_PENDING_SAMPLES = 64

        /** How long to wait for the writer thread to finish after the converter is killed. */
        const val WRITER_JOIN_MS = 1_000L
    }
}

/**
 * Starts `gonsensor` in the three shapes the appliance needs.
 *
 * `positionOnly` and `headingOnly` exist because the bot reads two separate FIFOs, and
 * two readers sharing one pipe would each receive a fraction of the sentences. The
 * unfiltered stream is what the loopback feed publishes, because a socket subscriber
 * gets its own complete copy.
 *
 * The two pipes belong to this factory, and they are made and held before the first
 * converter is spawned against them. A converter's output is redirected to a path at the
 * moment it starts, so a path that is not yet a pipe becomes a plain file, and the pipe
 * made afterwards replaces it — leaving the converter writing into an inode nothing can
 * reach and the bot reading a pipe no writer will ever touch. See [GonsensorFactoryTest].
 */
class GonsensorFactory(
    private val runner: ProcessRunner,
    private val binary: String,
    private val paths: StackPaths,
    private val pipes: SensorFifoHolder,
    private val log: (String) -> Unit = {},
) : Closeable {
    /** Whether the pipes have been made and are being held. */
    private var pipesHeld = false

    /** The position half, for the pipe the bot reads as `gps_port`. */
    fun positionOnly(): GonsensorStream = fifoStream(listOf("--no-hdm", "--no-hdt"), paths.gpsFifo)

    /** The heading half, for the pipe the bot reads as `compass_port`. */
    fun headingOnly(): GonsensorStream = fifoStream(listOf("--no-rmc", "--no-gga", "--no-gst"), paths.compassFifo)

    /** Everything, for a socket subscriber that filters for itself. */
    fun piped(): GonsensorStream {
        val spec = spec(emptyList())
        return GonsensorStream(
            runner.spawnPiped(spec.binary, spec.argv, spec.env),
            log,
        )
    }

    private fun fifoStream(flags: List<String>, outputPath: String): GonsensorStream {
        holdPipes()
        return GonsensorStream(runner.spawnWritingTo(spec(flags), outputPath), log)
    }

    /** Makes both pipes, once, and holds them for as long as this factory lives. */
    private fun holdPipes() {
        if (pipesHeld) {
            return
        }
        if (!pipes.open()) {
            log("could not make the sensor pipes; a converter spawned against a file would report history as though it were live")
        }
        pipesHeld = true
    }

    /**
     * Releases the pipes.
     *
     * A pipe is a meeting point rather than a thing owned, so releasing the handles is
     * all that happens here: the pipes stay where they are, which is what lets the next
     * reader find the same one this factory's converters are still writing. It is
     * therefore safe to call whether or not the converters have been stopped first.
     */
    override fun close() {
        if (!pipesHeld) {
            return
        }
        pipesHeld = false
        pipes.close()
    }

    private fun spec(flags: List<String>): LaunchSpec = LaunchSpec(
        name = "gonsensor",
        binary = binary,
        // The status line is a diagnostic for whoever is reading the log, and the rate
        // limits are off because the platform already delivers at the rate the sensors
        // are registered for.
        argv = flags + listOf("--rate", "0", "--compass-rate", "0", "--status-interval", "60"),
        env = mapOf("HOME" to paths.runDir),
        logFile = "${paths.logDir}/gonsensor.log",
    )
}
