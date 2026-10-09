// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import java.io.InputStream
import java.io.OutputStream
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** The uid the appliance runs as, and the only one whose console connection is kept. */
private const val OUR_UID = 10123

/** Another app on the tablet, which may not write to the appliance's terminal. */
private const val OTHER_UID = 10124

/**
 * The console session, against a fake host.
 *
 * A session is the app's half of the console: it binds an abstract socket, starts the
 * console host out of `nativeLibraryDir`, checks that whoever connects is this app, and
 * then bridges the two — the client's terminal output into the emulator, the tablet's
 * keystrokes and window size back out as frames.
 *
 * Every one of those steps is asserted here with no real socket and no real process: a
 * unit test on Android's JVM may start neither. The socket is [FakeChannel], whose bytes
 * the test writes and reads, and the host is the same [ProcessRunner] interface the
 * supervisor uses, so nothing is executed and nothing can be left running.
 *
 * The session never touches a `View` or an `Activity`: it reports through the callbacks
 * the caller passed and is driven by the caller, which is what lets it be tested here at
 * all, and what keeps the emulator's threading the caller's decision rather than a
 * library's.
 */
class ConsoleSessionTest {

    @Test
    fun `the session binds a name before it spawns the host`() {
        val fixture = Fixture()

        fixture.session.start()

        // The order is the whole point. The host is given the socket name on its command
        // line and dials it at once, so a host spawned before the bind would dial a name
        // nothing is listening on and the console would never connect.
        assertEquals("the bind must come before the spawn", listOf("bind", "spawn"), fixture.events)

        val name = requireNotNull(fixture.session.socketName) { "the session bound no name" }
        assertEquals("the listener was bound to another name", fixture.listener.name, name)
        assertTrue("the name is not the console form: $name", name.startsWith(ConsoleSocket.PREFIX))
        assertTrue("the name does not carry the app's pid: $name", name.contains(fixture.pid.toString()))

        val spawned = fixture.spawnedSpec()
        assertTrue(
            "the host was not told the bound name: ${spawned.commandLine()}",
            spawned.argv.contains(name),
        )
        assertEquals("the console host is not the bundled one", fixture.hostBinary, spawned.binary)

        // Starting twice does not bind a second socket or start a second host: the second
        // start is the caller's mistake, and two hosts on one terminal is not a recoverable
        // state for it to be in.
        fixture.session.start()
        assertEquals("a second start bound and spawned again", listOf("bind", "spawn"), fixture.events)
    }

    @Test
    fun `a peer from another uid is refused`() {
        // Another app on the tablet has learned the name. It is refused, and refused
        // without being read from: a peer that may not use the console must not be able to
        // put a single byte on the screen.
        val stranger = FakeChannel(peerUid = OTHER_UID, frames = listOf(dataFrame("hacked")))
        val ours = FakeChannel(peerUid = OUR_UID, frames = listOf(dataFrame("hi")))
        val fixture = Fixture(peers = listOf(stranger, ours))

        fixture.session.start()
        assertTrue("the session accepted no peer", fixture.session.connect())
        fixture.session.pump()

        assertEquals("the stranger was not refused", listOf(OTHER_UID), fixture.refused)
        assertTrue("the refused peer's socket was left open", stranger.closed)
        // The legitimate host dialed after the stranger, and the same session was still
        // there to accept it: refusing one peer must not end the session.
        assertEquals("the refused peer was read from", "hi", fixture.screen.rowText(0).trim())
        assertEquals(
            "the refusal ended the session rather than just the peer",
            listOf<ConsoleEnd>(ConsoleEnd.Closed),
            fixture.ended,
        )
    }

    @Test
    fun `incoming data frames reach the parser`() {
        // The client's output is terminal bytes, not text: a data frame is handed to the
        // parser exactly as if it had come off a pty, so an escape sequence in it is acted
        // on rather than printed.
        val channel = FakeChannel(
            peerUid = OUR_UID,
            frames = listOf(dataFrame("plain"), dataFrame("\u001b[31mred\u001b[0m")),
        )
        val fixture = Fixture(peers = listOf(channel))

        fixture.session.start()
        fixture.session.pump()

        assertEquals("plainred", fixture.screen.rowText(0).trim())
        assertEquals(
            "the sequence was printed rather than interpreted",
            TerminalColor.Palette(1),
            fixture.screen.penAt(6, 0).foreground,
        )
        assertEquals("the session ended early", listOf<ConsoleEnd>(ConsoleEnd.Closed), fixture.ended)
    }

    @Test
    fun `a resize sends a resize frame`() {
        val channel = FakeChannel(peerUid = OUR_UID)
        val fixture = Fixture(peers = listOf(channel))

        fixture.session.start()
        assertTrue(fixture.session.connect())

        fixture.session.resize(132, 43)
        assertEquals(
            "the window size did not go out as a resize frame",
            listOf<ConsoleFrame>(ConsoleFrame.Resize(132, 43)),
            channel.sentFrames(),
        )

        // A size a frame cannot carry is refused rather than truncated: a truncated size is
        // still a plausible terminal size, and the client would lay itself out for it.
        fixture.session.resize(0, 43)
        fixture.session.resize(70000, 43)
        assertEquals("an impossible size was sent anyway", 1, channel.sentFrames().size)
    }

    @Test
    fun `a size reported before the host connects is sent when it does`() {
        // The view is measured, and reports its size, the first time the page is laid out —
        // and at that instant the host has not dialled back yet, so there is no socket to
        // send it on. A size dropped there is dropped for the whole session: the client keeps
        // the size it was started with, and a terminal started at 80x24 on a 1920-pixel-wide
        // tablet draws in one corner of a screen it never claims.
        val channel = FakeChannel(peerUid = OUR_UID)
        val fixture = Fixture(peers = listOf(channel))

        fixture.session.start()
        fixture.session.resize(100, 30)
        fixture.session.resize(160, 48)
        assertEquals("a frame was sent before the host connected", 0, channel.sentFrames().size)

        assertTrue(fixture.session.connect())
        assertEquals(
            "the size the view reported was never sent, or an old one was replayed",
            listOf<ConsoleFrame>(ConsoleFrame.Resize(160, 48)),
            channel.sentFrames(),
        )

        // Once the host is there, a size goes straight out: the deferred one is a wait, not
        // a change of behaviour.
        fixture.session.resize(120, 40)
        assertEquals(
            listOf<ConsoleFrame>(ConsoleFrame.Resize(160, 48), ConsoleFrame.Resize(120, 40)),
            channel.sentFrames(),
        )
    }

    @Test
    fun `keystrokes are sent as data frames`() {
        val channel = FakeChannel(peerUid = OUR_UID)
        val fixture = Fixture(peers = listOf(channel))

        fixture.session.start()
        assertTrue(fixture.session.connect())

        fixture.session.sendKey(TerminalKey.Rune("a"))
        fixture.session.sendKey(TerminalKey.Ctrl('C'))
        fixture.session.sendKey(TerminalKey.Special(SpecialKey.UP))

        assertEquals(
            "the keys did not go out as the bytes a terminal sends",
            listOf<ConsoleFrame>(
                dataFrame("a"),
                ConsoleFrame.Data(byteArrayOf(0x03)),
                ConsoleFrame.Data(byteArrayOf(0x1b, '['.code.toByte(), 'A'.code.toByte())),
            ),
            channel.sentFrames(),
        )

        // A key the terminal has no bytes for sends nothing, and sends no empty frame
        // either: an empty write would tell the client that something had happened.
        fixture.session.sendKey(TerminalKey.Special(SpecialKey.F1))
        assertEquals("an unmapped key was sent as a frame", 3, channel.sentFrames().size)
    }

    @Test
    fun `an exit frame ends the session and reports the code`() {
        val channel = FakeChannel(
            peerUid = OUR_UID,
            frames = listOf(
                dataFrame("bye"),
                // The window size is the app's to send and never the host's, so one arriving
                // from the host is ignored rather than acted on.
                ConsoleFrame.Resize(120, 40),
                ConsoleFrame.Exit(3),
                dataFrame("too late"),
            ),
        )
        val fixture = Fixture(peers = listOf(channel))

        fixture.session.start()
        fixture.session.pump()

        assertEquals("the exit code was not reported", listOf<ConsoleEnd>(ConsoleEnd.Exited(3)), fixture.ended)
        assertEquals("output written before the exit was lost", "bye", fixture.screen.rowText(0).trim())
        // The exit frame is the end of the session: the bytes after it belong to a socket
        // the host has already closed, and are not read.
        assertTrue("the session read past the exit frame", channel.unread() > 0)
        assertTrue("the connection was left open", channel.closed)
        assertTrue("the listener was left bound", fixture.listener.closed)
        assertTrue("the host was left running", fixture.child().destroyed)
    }

    @Test
    fun `a closed socket ends the session once, not per frame`() {
        val channel = FakeChannel(
            peerUid = OUR_UID,
            frames = listOf(dataFrame("one"), dataFrame("two"), dataFrame("three")),
        )
        val fixture = Fixture(peers = listOf(channel))

        fixture.session.start()
        fixture.session.pump()

        assertEquals("three frames reported three ends", listOf<ConsoleEnd>(ConsoleEnd.Closed), fixture.ended)
        assertEquals("onetwothree", fixture.screen.rowText(0).trim())

        // The session is finished, and pumping a finished session is not a second end.
        fixture.session.pump()
        assertEquals("a second pump reported another end", 1, fixture.ended.size)
    }

    @Test
    fun `stopping the session kills the host`() {
        val channel = FakeChannel(peerUid = OUR_UID)
        val fixture = Fixture(peers = listOf(channel))

        fixture.session.start()
        assertTrue(fixture.session.connect())

        fixture.session.stop()

        assertTrue("the host was left running", fixture.child().destroyed)
        assertTrue("the connection was left open", channel.closed)
        assertTrue("the listener was left bound", fixture.listener.closed)
        assertEquals("stopping reported something other than a stop", listOf<ConsoleEnd>(ConsoleEnd.Stopped), fixture.ended)

        // Stopping twice is not two stops, and neither is a pump after one.
        fixture.session.stop()
        fixture.session.pump()
        assertEquals("a second stop reported another end", 1, fixture.ended.size)
    }

    @Test
    fun `a protocol error closes the session instead of throwing`() {
        // A stream that has lost its framing cannot be resynchronised — there is no way to
        // know where the next frame begins — so the session ends. It ends by reporting,
        // not by throwing: this code runs on the caller's thread, and a terminal that
        // throws into an event loop takes the appliance with it.
        val unknownType = byteArrayOf(0x7f, 0, 0, 0, 0)
        // A header that declares two payload bytes, followed by one.
        val truncated = ConsoleFrames.encode(dataFrame("ab")).copyOfRange(0, 6)
        val channel = FakeChannel(peerUid = OUR_UID, raw = unknownType + truncated)
        val fixture = Fixture(peers = listOf(channel))

        fixture.session.start()
        fixture.session.pump()

        assertEquals("the protocol error was not reported once", 1, fixture.ended.size)
        val end = fixture.ended.single()
        assertTrue("the end was reported as $end", end is ConsoleEnd.Failed)
        assertTrue(
            "the report does not say what was wrong: $end",
            (end as ConsoleEnd.Failed).reason.contains("0x7f"),
        )
        assertTrue("the connection was left open", channel.closed)
        assertTrue("the listener was left bound", fixture.listener.closed)
        assertTrue("the host was left running", fixture.child().destroyed)
    }

    /** dataFrame is the frame the host sends for a run of terminal bytes. */
    private fun dataFrame(text: String): ConsoleFrame.Data =
        ConsoleFrame.Data(text.toByteArray(Charsets.UTF_8))

    /**
     * One session with everything it needs faked out.
     *
     * The event list is shared by the listener factory and the runner, which is how the
     * bind-before-spawn order is a fact rather than an assumption.
     */
    private class Fixture(peers: List<ConsoleChannel> = emptyList()) {
        val pid = 4242
        val uid = OUR_UID
        val hostBinary = "/data/app/com.gmlewis.gonomadnet/lib/arm64/libgorcons.so"
        val clientBinary = "/data/app/com.gmlewis.gonomadnet/lib/arm64/libgonomadnetclient.so"

        val events = mutableListOf<String>()
        val listener = FakeListener(peers)
        val runner = FakeRunner(events)
        val screen = TerminalScreen(cols = 20, rows = 4)
        val ended = mutableListOf<ConsoleEnd>()
        val refused = mutableListOf<Int>()

        val session = ConsoleSession(
            host = { socketName ->
                LaunchSpec(
                    name = "gorcons",
                    binary = hostBinary,
                    argv = listOf(
                        "--socket", socketName,
                        "--command", clientBinary,
                        "--cols", "80",
                        "--rows", "24",
                    ),
                    env = mapOf("HOME" to "/data/data/com.gmlewis.gonomadnet/files/run"),
                    logFile = "/data/data/com.gmlewis.gonomadnet/files/logs/console.log",
                )
            },
            listeners = ConsoleListenerFactory { name ->
                events += "bind"
                listener.name = name
                listener
            },
            runner = runner,
            uid = uid,
            pid = pid,
            parser = TerminalParser(screen = screen),
            onRefused = { refused += it },
            onEnded = { ended += it },
        )

        fun spawnedSpec(): LaunchSpec = runner.spawned ?: error("the session spawned no host")

        fun child(): FakeChild = runner.child ?: error("the session started no child")
    }

    /**
     * A socket with a scripted input and a captured output.
     *
     * The input is a real stream over the encoded frames, so the session reads it through
     * the same reader it will use on a real socket.
     */
    private class FakeChannel(
        private val peerUid: Int,
        frames: List<ConsoleFrame> = emptyList(),
        raw: ByteArray? = null,
    ) : ConsoleChannel {
        private val incoming = ByteArrayInputStream(
            raw ?: frames.fold(ByteArray(0)) { bytes, frame -> bytes + ConsoleFrames.encode(frame) },
        )
        private val outgoing = ByteArrayOutputStream()

        var closed = false
            private set

        override fun input(): InputStream = incoming

        override fun output(): OutputStream = outgoing

        override fun peerUid(): Int = peerUid

        override fun close() {
            closed = true
        }

        /** sentFrames decodes everything the session has written. */
        fun sentFrames(): List<ConsoleFrame> {
            val reader = ConsoleFrameReader(ByteArrayInputStream(outgoing.toByteArray()))
            val frames = mutableListOf<ConsoleFrame>()
            while (true) {
                val frame = reader.read() ?: break
                frames += frame
            }
            return frames
        }

        /** unread is how many bytes the session has not taken off this socket. */
        fun unread(): Int = incoming.available()
    }

    /** A listener whose peers are scripted, and which remembers its bound name. */
    private class FakeListener(private val peers: List<ConsoleChannel>) : ConsoleListener {
        var name: String? = null

        var closed = false
            private set

        private var next = 0

        override fun accept(): ConsoleChannel? {
            if (closed || next >= peers.size) return null
            return peers[next++]
        }

        override fun close() {
            closed = true
        }
    }

    /** A runner that starts nothing, and records the spawn so the order can be asserted. */
    private class FakeRunner(private val events: MutableList<String>) : ProcessRunner {
        var spawned: LaunchSpec? = null

        var child: FakeChild? = null

        override fun spawn(spec: LaunchSpec): ChildProcess {
            events += "spawn"
            spawned = spec
            return FakeChild().also { child = it }
        }

        override fun spawnWritingTo(spec: LaunchSpec, outputPath: String): ChildProcess =
            error("the console never starts a converter writing to a FIFO")

        override fun spawnPiped(binary: String, argv: List<String>, env: Map<String, String>): ChildProcess =
            error("the console never starts a converter with a pipe")

        override fun runToCompletion(spec: LaunchSpec): Int =
            error("the console never runs a host to completion")
    }

    /** A child that only says whether it was killed. */
    private class FakeChild : ChildProcess {
        var destroyed = false
            private set

        override fun isAlive(): Boolean = !destroyed

        override fun destroy(): Boolean {
            destroyed = true
            return true
        }

        override fun stdin(): OutputStream? = null

        override fun stdout(): InputStream? = null
    }
}
