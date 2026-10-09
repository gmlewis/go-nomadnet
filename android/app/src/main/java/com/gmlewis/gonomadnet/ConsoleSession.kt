// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import java.io.IOException
import java.io.InputStream
import java.io.OutputStream

/** The largest terminal dimension a resize frame can carry, which is what its field holds. */
private const val MAX_TERMINAL_DIMENSION = 0xffff

/**
 * One peer on the console socket.
 *
 * The seam between the session and the platform: a session is tested against a fake
 * channel, because no unit test on Android's JVM may open a real socket.
 */
interface ConsoleChannel {
    /** input is the bytes the peer sends: frames, one after another. */
    fun input(): InputStream

    /** output is where frames for the peer are written. */
    fun output(): OutputStream

    /** peerUid is the uid on the other end, which is what decides whether it may connect. */
    fun peerUid(): Int

    /** close releases the connection. It may be called more than once. */
    fun close()
}

/**
 * The console socket, before anyone has connected to it.
 *
 * [accept] returns null once the listener is closed or otherwise cannot take another peer,
 * which the session reads as "no host is coming" rather than as an error to report: a
 * listener that has stopped listening is the session's own end.
 */
interface ConsoleListener {
    /** accept returns the next peer, blocking until one arrives, or null when there is none. */
    fun accept(): ConsoleChannel?

    /** close stops listening and releases the socket. */
    fun close()
}

/** Binds the session's socket. The real one is [LocalConsole]; tests bind a fake. */
fun interface ConsoleListenerFactory {
    /** bind returns a listener on the abstract socket [name]. */
    fun bind(name: String): ConsoleListener
}

/**
 * Why a console session is over.
 *
 * A session ends exactly once and reports once, whichever of these it was, and the caller
 * may treat the report as "the session is finished and must not be pumped again".
 */
sealed interface ConsoleEnd {
    /** The client exited with [code], which the host sent in an exit frame. */
    data class Exited(val code: Int) : ConsoleEnd

    /**
     * The connection ended without an exit frame.
     *
     * The host closed the socket, no peer ever arrived, or the session was ended before it
     * connected. There is no exit code to report because the host is in no position to give
     * one: it was killed, or it was never started.
     */
    data object Closed : ConsoleEnd

    /** The session could not go on. [reason] says what was wrong, for the log. */
    data class Failed(val reason: String) : ConsoleEnd

    /** The caller stopped the session. */
    data object Stopped : ConsoleEnd
}

/**
 * The app's half of the console: it owns the socket, the host process, and the frames.
 *
 * Android gives a bundled `tview` client no terminal, so the appliance supplies one. The
 * Go host (`libgorcons.so`) opens a pty, runs the client on it, and bridges it to this
 * socket; a session is the other end of that bridge — it binds the socket, starts the
 * host, checks who connected, decodes the frames the host sends into the emulator, and
 * encodes the tablet's keystrokes and window size back out.
 *
 * The three things this class deliberately is not:
 *
 *  - It is not a thread. Nothing here starts one, and [pump] is an ordinary blocking call
 *    that the caller runs wherever it wants the reading to happen. In the appliance that
 *    is a dedicated thread, because the socket read blocks for as long as the client runs,
 *    while [sendKey], [resize] and [stop] are called from the UI thread as the user acts.
 *    Every one of those is safe to call while [pump] is blocked.
 *  - It is not a `View` and holds no `Activity`. Callbacks are plain functions, so the
 *    caller decides how to get back to the UI thread — and so this class can be tested
 *    with no Android object at all.
 *  - It does not throw at its caller. A socket that has lost its framing, a host that will
 *    not start, a peer that may not connect: each is reported through a callback, because
 *    this code runs inside an event loop, and a terminal that throws into one takes the
 *    appliance down with it.
 *
 * The two callbacks arrive on whichever thread ended the session, which is usually the one
 * running [pump].
 */
class ConsoleSession(
    /**
     * The host to start, given the socket name that was just bound.
     *
     * It is a function rather than a built spec because the name does not exist until the
     * session binds: the host is told the name on its command line, and a spec built before
     * the bind would carry a name nothing is listening on. See `cmd/gorcons`.
     */
    private val host: (socketName: String) -> LaunchSpec,

    /** Binds the socket. [LocalConsole] in the appliance; a fake in the tests. */
    private val listeners: ConsoleListenerFactory,

    /** Starts the host. The same runner the stack supervisor uses, so nothing else is needed. */
    private val runner: ProcessRunner,

    /** The uid this app runs as, which is the only one whose connection is kept. */
    private val uid: Int,

    /** This app's pid, which is half of the socket name. */
    private val pid: Int,

    /** The emulator's parser. A data frame is handed to it exactly as a pty read would be. */
    private val parser: TerminalParser,

    /** onRefused reports a peer that was turned away, with its uid, for the log. */
    private val onRefused: (Int) -> Unit = {},

    /** onEnded reports the end of the session, exactly once. */
    private val onEnded: (ConsoleEnd) -> Unit = {},
) {

    /** The abstract name this session bound, or null until [start] has run. */
    var socketName: String? = null
        private set

    private val lock = Any()

    private var listener: ConsoleListener? = null

    private var child: ChildProcess? = null

    private var channel: ConsoleChannel? = null

    private var ended = false

    /**
     * start binds the socket and starts the host.
     *
     * The bind comes first, and must: the host dials the name as soon as it runs, so a host
     * started before the bind would dial a name nobody is listening on and the console would
     * never connect. Calling start twice binds nothing and starts nothing the second time.
     */
    fun start() {
        synchronized(lock) {
            if (ended || listener != null) return
        }

        val name = ConsoleSocket.name(pid, ConsoleSocket.randomEntropy())
        val bound = try {
            listeners.bind(name)
        } catch (e: Exception) {
            end(ConsoleEnd.Failed("binding the console socket $name: ${e.message}"))
            return
        }

        val spec = host(name)
        val started = try {
            runner.spawn(spec)
        } catch (e: Exception) {
            // Nothing is listening for a host that will not run, and leaving the socket
            // bound would make the next session's bind fail on a name only this one knows.
            bound.close()
            end(ConsoleEnd.Failed("starting the console host ${spec.commandLine()}: ${e.message}"))
            return
        }

        synchronized(lock) {
            socketName = name
            listener = bound
            child = started
        }
    }

    /**
     * connect accepts one peer, and reports whether it got one this app may talk to.
     *
     * A peer from another uid is closed and refused, and the session goes on listening: the
     * legitimate host dials next, and a stranger on the same tablet must not be able to end
     * the terminal by connecting to it. It is refused without being read from, so it cannot
     * put a byte on the screen either.
     *
     * [pump] calls this for the caller, so it is only needed by a caller that wants the
     * connection established before it starts reading.
     */
    fun connect(): Boolean {
        val bound = synchronized(lock) { if (ended) null else listener } ?: return false
        while (true) {
            val peer = bound.accept() ?: return false
            val peerUid = peer.peerUid()
            if (!ConsoleSocket.peerIsAccepted(uid, peerUid)) {
                peer.close()
                onRefused(peerUid)
                continue
            }
            val accepted = synchronized(lock) {
                if (ended) {
                    false
                } else {
                    channel = peer
                    true
                }
            }
            if (!accepted) {
                peer.close()
                return false
            }
            return true
        }
    }

    /**
     * pump reads frames until the session ends, handing each one on.
     *
     * This is the blocking half: it returns when the host sends an exit frame, when the
     * connection ends, when the frame stream loses its framing, or when [stop] is called
     * from another thread. It reports through [onEnded] and never throws, and a session that
     * has already ended returns at once rather than reporting a second end.
     */
    fun pump() {
        if (finished()) return
        if (synchronized(lock) { channel } == null && !connect()) {
            end(ConsoleEnd.Closed)
            return
        }
        val connection = synchronized(lock) { channel } ?: run {
            end(ConsoleEnd.Closed)
            return
        }

        val reader = ConsoleFrameReader(connection.input())
        while (true) {
            val frame = try {
                reader.read()
            } catch (e: ConsoleProtocolException) {
                end(ConsoleEnd.Failed(e.message ?: "the frame stream lost its framing"))
                return
            }
            when (frame) {
                null -> {
                    end(ConsoleEnd.Closed)
                    return
                }
                is ConsoleFrame.Data -> parser.write(frame.payload)
                is ConsoleFrame.Exit -> {
                    end(ConsoleEnd.Exited(frame.code))
                    return
                }
                // A resize frame is the app's to send, not the host's: the window size is
                // the tablet's business, and the host has nothing to say about it.
                is ConsoleFrame.Resize -> Unit
            }
        }
    }

    /**
     * sendKey writes one key press to the host as a data frame.
     *
     * The host writes these bytes to the pty, so a key is the bytes a terminal would have
     * sent for it — see [TerminalKeys]. A key the terminal has no bytes for sends nothing,
     * and a session that has ended sends nothing at all.
     */
    fun sendKey(key: TerminalKey) {
        val bytes = TerminalKeys.toBytes(key) ?: return
        write(ConsoleFrame.Data(bytes))
    }

    /**
     * resize tells the host the terminal's new size, so the client can reflow.
     *
     * A size a frame cannot carry is refused rather than truncated: a truncated size is
     * still a size, and the client would lay itself out for a terminal that does not exist.
     */
    fun resize(cols: Int, rows: Int) {
        if (cols !in 1..MAX_TERMINAL_DIMENSION || rows !in 1..MAX_TERMINAL_DIMENSION) return
        write(ConsoleFrame.Resize(cols, rows))
    }

    /**
     * stop ends the session and kills the host.
     *
     * A console must never outlive the app that owns it, and the host holds a pty running
     * the client, so stopping means the host goes with it. Stopping again, or pumping after
     * stopping, does nothing and reports nothing.
     */
    fun stop() {
        end(ConsoleEnd.Stopped)
    }

    /** finished reports whether the session has ended, which makes it unusable. */
    private fun finished(): Boolean = synchronized(lock) { ended }

    /** write sends one frame to the host, if there is a host to send it to. */
    private fun write(frame: ConsoleFrame) {
        val connection = synchronized(lock) { if (ended) null else channel } ?: return
        try {
            val out = connection.output()
            out.write(ConsoleFrames.encode(frame))
            out.flush()
        } catch (e: IOException) {
            // A socket that will not take a keystroke is a socket that has gone: the client
            // is not reading it any more, which is the session's end.
            end(ConsoleEnd.Failed("writing to the console socket: ${e.message}"))
        }
    }

    /**
     * end finishes the session, releasing the socket, the connection and the host, and
     * reports it once.
     *
     * The first caller wins and the rest do nothing, which is what makes an end reported
     * from two directions — a stop from the UI thread and a read that fails on the reader's
     * thread — arrive as one end.
     */
    private fun end(reason: ConsoleEnd) {
        val live = synchronized(lock) {
            if (ended) return
            ended = true
            Live(channel, child, listener)
        }
        live.channel?.close()
        live.listener?.close()
        // Destroying a child that has already exited is a no-op, which is why every end
        // releases the host rather than only the ones that killed it.
        live.child?.destroy()
        onEnded(reason)
    }

    /** Live is what a session holds while it is running, so that ending it can release it. */
    private class Live(
        val channel: ConsoleChannel?,
        val child: ChildProcess?,
        val listener: ConsoleListener?,
    )
}
