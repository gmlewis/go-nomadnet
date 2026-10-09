// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import java.io.Closeable
import java.io.OutputStream
import java.net.InetAddress
import java.net.ServerSocket
import java.net.Socket

/**
 * Publishes the live sensor stream on loopback for the Nomad Network client in Termux
 * to subscribe to.
 *
 * It has to be a socket. Termux is a different application with a different uid, so
 * nothing is handed across through the filesystem: `/sdcard` supports neither FIFOs nor
 * symlinks, and one app cannot write into another's private data at all. A loopback TCP
 * socket needs nothing but the INTERNET permission, which is why the shared instance
 * uses one too.
 *
 * Every subscriber is handed the same sentences. There is one converter behind the feed,
 * and each subscriber is written the whole of what it produces, so what a subscriber
 * misses is nothing: a second converter would be a second process reading the same
 * sensors for no reason, and a second reader sharing one pipe would take half the
 * sentences each, which parses into nonsense.
 *
 * The stream is multiplexed: position sentences and heading sentences arrive together,
 * and each subscriber's own reader ignores the half it does not understand. Both readers
 * take the same feed, which is what lets the client in Termux resolve a `L construct
 * with a live position while the bot is not running at all.
 */
class SensorFeedServer(
    private val log: (String) -> Unit = {},
) : FeedPublisher {
    private var server: ServerSocket? = null
    private val subscribers = mutableListOf<Socket>()

    /** Starts listening. Returns false when the port is already taken. */
    override fun start(port: Int): Boolean {
        val socket = try {
            ServerSocket(port, BACKLOG, InetAddress.getByName(LOOPBACK))
        } catch (failure: Throwable) {
            log("could not listen on $LOOPBACK:$port: ${failure.message}")
            return false
        }
        server = socket
        Thread({ acceptLoop(socket) }, "sensor-feed").apply {
            isDaemon = true
            start()
        }
        log("the sensor feed is listening on $LOOPBACK:$port")
        return true
    }

    /** Copies this process's own sensor sentences to every connected subscriber. */
    override fun offer(bytes: ByteArray) {
        synchronized(subscribers) {
            for (client in subscribers.toList()) {
                try {
                    client.getOutputStream().write(bytes)
                    client.getOutputStream().flush()
                } catch (failure: Throwable) {
                    log("a sensor feed subscriber disconnected: ${failure.message}")
                    subscribers.remove(client)
                    runCatching { client.close() }
                }
            }
        }
    }

    private fun acceptLoop(socket: ServerSocket) {
        while (!socket.isClosed) {
            val client = try {
                socket.accept()
            } catch (closed: Throwable) {
                return
            }
            synchronized(subscribers) { subscribers.add(client) }
            log("a sensor feed subscriber connected from ${client.inetAddress.hostAddress}")
        }
    }

    override fun close() {
        synchronized(subscribers) {
            for (client in subscribers) {
                runCatching { client.close() }
            }
            subscribers.clear()
        }
        runCatching { server?.close() }
        server = null
    }

    companion object {
        /** The loopback address, which is the only address the feed is ever published on. */
        const val LOOPBACK = "127.0.0.1"

        /**
         * The port the client and the appliance both pin the feed to.
         *
         * It must not collide with the ports Reticulum already owns on this device.
         * `rns/rns.go` defaults to `localInterfacePort: 37428` for the shared instance and
         * `localControlPort: 37429` for its control channel, so publishing the feed on 37429
         * makes the transport fail to come up with:
         *
         * ```
         * Could not initialize Reticulum: listen tcp 127.0.0.1:37429: bind: address already in use
         * ```
         *
         * and the only visible symptom is that the transport restarts until the supervisor
         * gives up on it. 37428 and 37429 are therefore reserved, and the feed takes the next
         * free one.
         */
        const val DEFAULT_FEED_PORT = 37430

        /**
         * The ports Reticulum owns and the feed may never use.
         *
         * Kept as data rather than as a comment so a test can assert the feed stays clear of
         * them.
         */
        val RESERVED_RETICULUM_PORTS = setOf(37428, 37429)

        private const val BACKLOG = 4
    }
}
