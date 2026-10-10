// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import java.io.File
import java.util.concurrent.CopyOnWriteArrayList

/**
 * The path a radio bridge published, and the wait for one to be published.
 *
 * The bridge is a separate program — `android/rnode` — and what it publishes is the only
 * thing that can tell the appliance which serial path the transport should be dialled at.
 * It is a path and not a value passed back because the two are different processes: one
 * writes a file atomically, the other reads it, and neither has to be running at the same
 * instant as the other for either to be correct.
 */
object PublishedPath {

    /** How many times the appliance looks for a published path before giving up. */
    const val ATTEMPTS = 60

    /** How long it waits between looks, in milliseconds. */
    const val WAIT_MS = 250L

    /** read returns the path a bridge published, or null when none has. */
    fun read(path: String): String? =
        File(path).takeIf { it.isFile }?.readText()?.trim()?.takeIf { it.isNotEmpty() }

    /**
     * await waits for a bridge to publish a path, and returns it, or null if none arrives.
     *
     * The wait is bounded and the read and the sleep are parameters, because one is a file
     * and the other is time, and neither belongs in a unit test. Giving up is a report rather
     * than a hang: an appliance whose Start stack button did nothing and said nothing is
     * worse than one that is honest about having no radio.
     */
    fun await(
        path: String,
        attempts: Int = ATTEMPTS,
        waitMs: Long = WAIT_MS,
        read: (String) -> String? = ::read,
        sleep: (Long) -> Unit = { Thread.sleep(it) },
    ): String? {
        repeat(attempts) { attempt ->
            read(path)?.let { return it }
            // Having used the last attempt there is nothing left to wait for.
            if (attempt < attempts - 1) {
                sleep(waitMs)
            }
        }
        return null
    }
}

/**
 * Carries a radio's bytes between its endpoints and the bridge's socket, in both directions
 * at once.
 *
 * Two threads, because a radio talks while it is being talked to: a single loop that read
 * one end and then the other would stall every transfer behind the quiet end. Ending either
 * direction ends the pump and closes both ends — the other thread is blocked in a read that
 * only a close releases, and a bridge that outlived its radio would hold the USB device and
 * the pseudo-terminal for the life of the install.
 */
class RadioPump(private val log: (String) -> Unit = {}) {

    /** The ends the pump is carrying between, both of which it closes when it is stopped. */
    private val ends = CopyOnWriteArrayList<ByteStream>()

    @Volatile
    private var stopped = false
    private var threads: List<Thread> = emptyList()

    /** Reports whether the pump is still carrying bytes. */
    fun running(): Boolean = threads.any { it.isAlive }

    /** start carries bytes both ways until either end ends. */
    fun start(radio: ByteStream, socket: ByteStream) {
        stopped = false
        ends.add(radio)
        ends.add(socket)
        threads = listOf(
            Thread({ carry(radio, socket) }, "rnode-radio-to-socket").also { it.isDaemon = true },
            Thread({ carry(socket, radio) }, "rnode-socket-to-radio").also { it.isDaemon = true },
        )
        threads.forEach { it.start() }
    }

    /**
     * stop closes both ends and waits for the threads to notice, for a bounded time.
     *
     * The bound is what makes this safe to call from the UI thread: a thread that will not
     * come back is a thread the appliance cannot wait for, and the radio's claim is released
     * by the closes below whatever the threads do next.
     */
    fun stop(graceMs: Long = GRACE_MS) {
        stopped = true
        for (end in ends) {
            runCatching { end.close() }
        }
        for (thread in threads) {
            runCatching { thread.join(graceMs) }
        }
        threads = emptyList()
    }

    private fun carry(from: ByteStream, to: ByteStream) {
        val buffer = ByteArray(BUFFER_BYTES)
        while (!stopped) {
            val count = try {
                from.read(buffer)
            } catch (failure: Throwable) {
                log("the radio bridge's ${Thread.currentThread().name} read failed: ${failure.message}")
                -1
            }
            if (count < 0) {
                break
            }
            if (count == 0) {
                continue
            }
            val written = try {
                to.write(buffer, count)
            } catch (failure: Throwable) {
                log("the radio bridge's ${Thread.currentThread().name} write failed: ${failure.message}")
                false
            }
            if (!written) {
                break
            }
        }
        // Either end ending ends the pump.
        runCatching { from.close() }
        runCatching { to.close() }
    }

    companion object {
        /** How much is carried in one direction at a time. */
        const val BUFFER_BYTES = 4096

        /** How long a thread is waited for after its end has been closed. */
        const val GRACE_MS = 2_000L
    }
}

/**
 * Brings the radio up for the transport, and puts it down again.
 *
 * The order is not a preference. The device has to be claimed before the bridge is started,
 * because the bridge is what publishes the serial path the transport is dialled at, and a
 * bridge started with no device behind it publishes a path onto nothing. The path has to be
 * published before the transport is configured, and the socket dialled before any of it
 * carries a byte.
 *
 * Every Android- and process-shaped thing it needs is injected: a unit test has no USB
 * device, no second process, and no abstract socket, and this class is where the ordering
 * that decides whether a radio works actually lives.
 */
class RNodeBridge(
    private val runner: ProcessRunner,
    private val finder: RadioFinder,
    private val socketName: () -> String,
    private val bridgeSpec: (socketName: String) -> LaunchSpec,
    private val dial: (socketName: String) -> ByteStream?,
    private val ttyFile: String,
    private val log: (String) -> Unit = {},
    private val sleep: (Long) -> Unit = { Thread.sleep(it) },
    private val awaitAttempts: Int = PublishedPath.ATTEMPTS,
) {

    /** The serial path the transport should be dialled at, or null when there is no radio. */
    var publishedTty: String? = null
        private set

    private var radio: ByteStream? = null
    private var channel: ByteStream? = null
    private var child: ChildProcess? = null
    private val pump = RadioPump(log)

    /** The path the bridge publishes the transport's serial port at. */
    val publishedPathFile: String get() = ttyFile

    /**
     * start claims the radio and brings the bridge up.
     *
     * It returns false when there is no radio to bring up, and that is not a failure: an
     * appliance with no radio is an appliance with a hub, and the transport is rendered with
     * no radio at all rather than with one enabled against a path that names nothing.
     */
    fun start(): Boolean {
        if (publishedTty != null) {
            return true
        }
        val device = finder.find() ?: run {
            log("no radio is attached, so the appliance has none this time")
            return false
        }
        val link = device.open() ?: run {
            log("could not open ${device.description}")
            return false
        }
        radio = link

        val name = socketName()
        val spec = bridgeSpec(name)
        val started = try {
            runner.spawn(spec)
        } catch (failure: Throwable) {
            log("could not start ${spec.name}: ${failure.message}")
            stop()
            return false
        }
        child = started

        val path = PublishedPath.await(
            path = ttyFile,
            attempts = awaitAttempts,
            read = { PublishedPath.read(it) },
            sleep = sleep,
        ) ?: run {
            // A bridge that never published a path has nothing for the transport to dial, so
            // it is put down rather than left running against no radio.
            log("${spec.name} never published a serial path for the radio")
            stop()
            return false
        }

        val socket = dial(name) ?: run {
            log("could not dial the radio bridge on $name")
            stop()
            return false
        }
        channel = socket
        publishedTty = path
        pump.start(link, socket)
        log("the radio is up: ${device.description} at $path")
        return true
    }

    /**
     * stop releases the radio, the socket and the bridge.
     *
     * Closing both ends of the pump is what releases the USB claim and ends the bridge: it
     * sees its socket close, gives up the pseudo-terminal, and removes the path it published.
     * Nothing of the radio is left behind for the next start to find.
     */
    fun stop() {
        pump.stop()
        runCatching { channel?.close() }
        runCatching { radio?.close() }
        channel = null
        radio = null
        child?.let { running ->
            if (running.destroy()) {
                log("stopped the radio bridge")
            }
        }
        child = null
        publishedTty = null
    }

    companion object {
        /** The name the bridge is logged under and staged as. */
        const val NAME = "gornnode"
    }
}
