// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify it under the
// terms of the GNU General Public License as published by the Free Software
// Foundation, either version 3 of the License, or (at your option) any later version.

package com.gmlewis.gonomadnet

import android.util.Log
import java.io.Closeable
import java.io.IOException
import java.io.InputStream
import java.io.OutputStream
import java.net.InetAddress
import java.net.ServerSocket
import java.net.Socket

/**
 * The appliance's end of the camera scan protocol.
 *
 * The client runs as a process of its own and has no camera, so a dialog that would rather
 * read an address than have it typed asks this listener for one. It is bound to the loopback
 * interface on an ephemeral port, and the client is told the port and the token on its
 * command line — see [MainActivity.consoleHostSpec] — so nothing outside this device can
 * reach it at all, and nothing else on the device can reach it without the token.
 *
 * The listener is deliberately thin. It reads one request line, checks the token, calls
 * [ScanRequester.scan], and writes one reply line: the camera, the permission and the
 * activity live behind that interface, so this half — the half where a protocol mistake
 * would be invisible — is tested against a fake in [ScanServerTest].
 *
 * A request the appliance did not expect is answered by closing the connection without
 * opening a camera. That covers a peer that guessed the port but not the token, a peer that
 * sent something that is not a request, and a peer that sent nothing at all and is simply
 * waiting: all three cost one closed socket and nothing else.
 */
class ScanServer(
    private val requester: ScanRequester,
    entropy: ByteArray = ConsoleSocket.randomEntropy(),
    private val onRefused: (() -> Unit)? = null,
) : Closeable {

    /** The token every request must carry, as lower-case hex. */
    val token: String = entropy.toHex()

    /**
     * The server socket, bound before this object exists.
     *
     * Binding here rather than in [start] is what lets the port be read and passed to the
     * client as soon as the console exists: a port that is not known until a thread has run
     * is a port the client cannot be told, and a client told the wrong one has a scan button
     * that silently does nothing. Port 0 asks the kernel for a free port, which is the only
     * way two appliances on one device — or a stale one still shutting down — do not collide.
     */
    private val server = ServerSocket(0, BACKLOG, InetAddress.getLoopbackAddress())

    /** The port the client should dial. */
    val port: Int get() = server.localPort

    /**
     * The host:port the client is told to dial, which is the loopback address.
     *
     * It is built from the address the socket is actually bound to rather than from the
     * constant it was asked for, so that a listener which somehow ended up on a routable
     * address would be reported here rather than described as loopback. That address is what
     * the environment variable carries, and it is the whole of who may reach this listener:
     * nothing off the device can.
     *
     * An IPv6 address is bracketed, and that is not cosmetic: this device's loopback is
     * `::1`, and the client is Go, where `net.Dial("tcp", "::1:46635")` is
     * "too many colons in address" and never connects. A listener that named itself
     * unbracketed would be a Scan button that did nothing at all, on every device whose
     * loopback resolves to IPv6 first — which is the ordinary case.
     */
    val endpoint: String
        get() = endpointOf(server.inetAddress.hostAddress, server.localPort)

    /** The thread accepting connections, once [start] has been called. */
    private var thread: Thread? = null

    /**
     * start begins accepting connections.
     *
     * The thread is a daemon so that a scanner left open cannot hold the process up when
     * everything else is finished with it — though [close] is what ends it in practice, and
     * an appliance that relied on the daemon flag alone would leak a thread per console.
     */
    fun start() {
        if (thread != null) {
            return
        }
        thread = Thread({ acceptLoop() }, "scan-server").apply {
            isDaemon = true
            start()
        }
    }

    /**
     * close stops accepting, and ends the accept loop.
     *
     * A closed server socket makes the blocked accept throw, and that exception is how this
     * loop ends: there is no other way to interrupt one, and no state to leave behind.
     */
    override fun close() {
        try {
            server.close()
        } catch (e: IOException) {
            // Closing a socket that is already closed is not a failure, and there is nothing
            // left to report it to: the console is going away either way.
        }
    }

    /** acceptLoop serves connections until the listener is closed. */
    private fun acceptLoop() {
        while (true) {
            val socket = try {
                server.accept()
            } catch (e: IOException) {
                return
            }
            serve(socket)
        }
    }

    /** serve answers one connection, and always closes it. */
    private fun serve(socket: Socket) {
        try {
            socket.use { answer(it) }
        } catch (e: IOException) {
            // The peer went away before it could be answered, which is not this end's
            // problem: the client has already given up on the scan.
        } catch (e: Exception) {
            // Anything else — including whatever the camera threw — is still only this one
            // connection's problem. Letting it reach the accept loop would end the loop, and
            // the appliance would then have a scanner that a reader could see and never use
            // again for the rest of the session.
            Log.i(TAG, "the scan request failed: ${e.message}")
        }
    }

    /** answer reads one request and writes one reply. */
    private fun answer(socket: Socket) {
        val line = readLine(socket.getInputStream()) ?: return
        if (!ScanProtocol.acceptsToken(token, ScanProtocol.parseRequest(line))) {
            // A request this listener was not given the token for. It is not answered, and
            // the connection is what says so.
            onRefused?.invoke()
            return
        }
        val payload = requester.scan()
        val reply = if (payload == null) ScanProtocol.CANCEL_REPLY else ScanProtocol.replyLine(payload)
        writeLine(socket.getOutputStream(), reply)
    }

    /**
     * readLine reads one newline-terminated line.
     *
     * It returns null for a line longer than the protocol allows, rather than reading on
     * until a newline that a peer has no intention of sending: this read is holding a server
     * thread, and the peer is a process that may already be gone.
     */
    private fun readLine(input: InputStream): String? {
        val buffer = StringBuilder()
        while (true) {
            val next = input.read()
            if (next < 0) {
                return null
            }
            if (next == '\n'.code) {
                return buffer.toString()
            }
            if (buffer.length >= ScanProtocol.MAX_LINE) {
                return null
            }
            buffer.append(next.toChar())
        }
    }

    /** writeLine writes one reply line and flushes it. */
    private fun writeLine(output: OutputStream, line: String) {
        output.write((line + "\n").toByteArray(Charsets.UTF_8))
        output.flush()
    }

    companion object {
        /**
         * BACKLOG is how many connections may be waiting to be accepted.
         *
         * One client asks once per scan and the reader presses the button once at a time, so
         * this is a queue that should never hold more than one — it is depth enough for the
         * requests this listener is for, and short enough that a peer flooding it is refused
         * rather than queued.
         */
        const val BACKLOG = 4

        /** TAG is what this listener's own failures are reported under. */
        const val TAG = "gonomadnet"

        /**
         * endpointOf returns the host:port the client is pointed at for a bound address.
         *
         * An IPv6 literal is bracketed, and that is not cosmetic: Android's loopback is `::1`,
         * and the client is Go, where `net.Dial("tcp", "::1:46635")` is
         *
         *	too many colons in address
         *
         * and never connects. An appliance that named itself unbracketed would offer a Scan
         * button that did nothing at all — which is what it did, on the tablet, until this
         * was found. The rule is a function of its own so that both addresses can be asserted
         * on whichever one the machine running the tests happens to have.
         */
        fun endpointOf(host: String, port: Int): String =
            if (host.contains(':')) "[$host]:$port" else "$host:$port"
    }
}

/** toHex renders bytes as lower-case hex, which is what the Go half expects of a token. */
private fun ByteArray.toHex(): String {
    val digits = "0123456789abcdef"
    val out = StringBuilder(size * 2)
    for (byte in this) {
        val value = byte.toInt() and 0xff
        out.append(digits[value ushr 4]).append(digits[value and 0x0f])
    }
    return out.toString()
}
