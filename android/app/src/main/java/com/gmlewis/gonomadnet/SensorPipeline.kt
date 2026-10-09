// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import java.io.Closeable

/** Where one already-serialized sample goes. */
interface SampleSink : Closeable {
    /** Writes one sample, and reports whether it was accepted. */
    fun offer(line: String): Boolean
}

/**
 * A sink whose converter's sentences are what the loopback subscribers read.
 *
 * The broadcast converter turns the same JSON samples into NMEA on its standard output, and
 * something has to carry those bytes to the socket. Leaving that to the caller is how the feed
 * came to accept connections and then send nothing at all: the pipes were wired, the threads
 * were not, and a subscriber saw a hung socket rather than an error.
 */
interface BroadcastSink : SampleSink {
    /** The converter's sentences, or null when it was not started with a readable output. */
    fun sentences(): java.io.InputStream?
}

/** Something that produces samples, and can be turned off without disturbing anything else. */
interface SampleSource : Closeable {
    /** Starts producing. Returns false when the platform has no such sensor. */
    fun start(): Boolean
}

/** The loopback publisher the client in Termux subscribes to. */
interface FeedPublisher : Closeable {
    /** Starts listening, and reports whether the port was available. */
    fun start(port: Int): Boolean

    /** Hands one chunk of sentences to every subscriber. */
    fun offer(bytes: ByteArray)
}

/** The wake lock a non-wakeup sensor needs to keep delivering. */
interface WakeLockHandle {
    /** Holds the lock, if it is not already held. */
    fun acquire()

    /** Releases it, if it is held. */
    fun release()

    /** Reports whether it is held. */
    fun isHeld(): Boolean
}

/**
 * The sensor half of the appliance, and nothing else.
 *
 * This is one of the two independent axes of the design, and the independence is the whole
 * point of it existing as a type rather than as a paragraph of code inside the service. The
 * sensor half runs with the daemon stack or without it:
 *
 *  - with the stack, a local bot can answer `/msg gobot loc` with the tablet's real position;
 *  - without it, the Nomad Network client in Termux still resolves a `L Micron location
 *    construct with a live distance and bearing, and simply has no local bot to ask;
 *  - and it is also the cheapest configuration for the battery, since a receiver and a
 *    magnetometer cost far less than three more Go processes.
 *
 * The other axis — who owns Reticulum — lives in [StackSupervisor], and the two deliberately
 * share nothing but the paths. Stopping either one must leave the other exactly as it was.
 */
class SensorPipeline(
    private val positionSink: SampleSink,
    private val headingSink: SampleSink,
    private val broadcastSink: BroadcastSink,
    private val feed: FeedPublisher,
    private val location: SampleSource,
    private val heading: SampleSource,
    private val wakeLock: WakeLockHandle,
    private val feedPort: Int = SensorFeedServer.DEFAULT_FEED_PORT,
    private val log: (String) -> Unit = {},
) {
    /** Whether the pipeline is producing samples. */
    var isRunning: Boolean = false
        private set

    /** Whether the loopback feed is listening. */
    var isFeeding: Boolean = false
        private set

    // broadcastPump copies the broadcast converter's sentences to every subscriber. It is a
    // daemon thread so that nothing here can outlive the service.
    private var broadcastPump: Thread? = null

    /** Starts producing samples. Idempotent. */
    fun start() {
        if (isRunning) {
            return
        }
        wakeLock.acquire()
        isFeeding = feed.start(feedPort)
        startBroadcastPump()
        if (!location.start()) {
            log("this device has no usable location provider; there will be no position")
        }
        if (!heading.start()) {
            log("this device has no rotation vector; there will be no heading")
        }
        isRunning = true
        log("the sensor service is running")
    }

    /**
     * Carries the broadcast converter's sentences to the socket.
     *
     * This is the only place the loopback feed is fed from. Without it the feed accepts a
     * connection and then says nothing, which looks exactly like a client that is waiting.
     */
    private fun startBroadcastPump() {
        val source = broadcastSink.sentences() ?: return
        broadcastPump = Thread({
            val buffer = ByteArray(BUFFER_BYTES)
            while (!Thread.currentThread().isInterrupted) {
                val read = try {
                    source.read(buffer)
                } catch (closed: Throwable) {
                    return@Thread
                }
                if (read <= 0) {
                    return@Thread
                }
                feed.offer(buffer.copyOf(read))
            }
        }, "sensor-broadcast").apply {
            isDaemon = true
            start()
        }
    }

    /** Stops producing samples and releases everything it holds. Idempotent. */
    fun stop() {
        if (!isRunning) {
            return
        }
        broadcastPump?.interrupt()
        broadcastPump = null
        location.close()
        heading.close()
        feed.close()
        isFeeding = false
        positionSink.close()
        headingSink.close()
        broadcastSink.close()
        wakeLock.release()
        isRunning = false
    }

    /**
     * Sends one already-serialized sample to every converter and to the loopback feed.
     *
     * The sample goes to all three because each has a different consumer: the position
     * converter feeds the pipe the bot reads as its GNSS source, the heading converter the
     * pipe it reads as its compass, and the broadcast converter the socket the client in
     * Termux subscribes to. Each of those filters for itself, which is why one serialized
     * form of the sample is enough.
     */
    fun offer(line: String) {
        if (!isRunning) {
            return
        }
        positionSink.offer(line)
        headingSink.offer(line)
        broadcastSink.offer(line)
    }

    companion object {
        /** The read size for the broadcast pump. */
        const val BUFFER_BYTES = 4096
    }
}
