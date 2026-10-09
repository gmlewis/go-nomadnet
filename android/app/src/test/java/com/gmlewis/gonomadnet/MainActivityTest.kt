// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.w3c.dom.Element
import java.io.File
import javax.xml.parsers.DocumentBuilderFactory

/**
 * How the appliance runs the client itself.
 *
 * The built-in client is the same client Termux runs, started the same way, with two
 * differences that are the whole point of it: it is the APK's own binary rather than a
 * copy in somebody else's home, and it keeps its configuration in the appliance's private
 * storage rather than in Termux's. Both are facts about a command line, and a command line
 * is the one part of starting a process that can be asserted without a device — which
 * matters here, because the failure modes are silent: a client pointed at the transport's
 * own Reticulum directory becomes the transport, and a client pointed at a home it cannot
 * write to writes nothing and says nothing.
 *
 * [MainActivity] is never instantiated. These are its companion's pure helpers, and the
 * class is only named — the tests below are arithmetic on a command line.
 */
class MainActivityTest {

    @Test
    fun `the built-in client uses the appliance transport in attached mode`() {
        val paths = StackPaths(FILES_DIR)
        val client = MainActivity.builtInClient(NATIVE_LIBRARY_DIR, paths)

        val rnsConfig = client.argv.argAfter("--rnsconfig")
        assertNotNull("the client was not told which Reticulum configuration to use", rnsConfig)
        assertEquals(
            "the client must attach to the appliance's transport",
            paths.rnsClientConfigDir,
            rnsConfig,
        )
        // The two directories differ by one word and by everything that matters: one lists
        // interfaces and owns the shared instance, and the other requires it. A client given
        // the transport's directory becomes a second transport and the two silently split
        // the network between them.
        assertNotEquals(
            "the client was given the transport's own configuration directory",
            paths.rnsConfigDir,
            rnsConfig,
        )
        assertTrue(
            "the built-in client does not run the text interface: ${client.argv}",
            client.argv.contains("-t"),
        )
    }

    @Test
    fun `the built-in client keeps its own nomadnetwork config directory`() {
        val paths = StackPaths(FILES_DIR)
        val client = MainActivity.builtInClient(NATIVE_LIBRARY_DIR, paths)

        val config = client.argv.argAfter("--config")
        assertNotNull("the client was not told where to keep its configuration", config)
        assertTrue(
            "the client's configuration is not inside the appliance's own storage: $config",
            config!!.startsWith("$FILES_DIR/"),
        )
        assertFalse(
            "the client was pointed at Termux's home, which it can neither read nor write: $config",
            config.contains("com.termux"),
        )
        assertEquals(paths.nomadnetworkConfigDir, config)
        // The Nomad Network directory and the Reticulum one are different: the client's
        // messages, its peers and its pages live in the first, and its interfaces in the
        // second. One path for both would overwrite a configuration with a different one.
        assertNotEquals(paths.rnsClientConfigDir, config)

        assertTrue(
            "the client's home is not the appliance's: ${client.home}",
            client.home.startsWith("$FILES_DIR/"),
        )
        assertEquals(
            "the client is not the APK's own binary",
            "$NATIVE_LIBRARY_DIR/lib${MainActivity.CLIENT_NAME}.so",
            client.binary,
        )
    }

    @Test
    fun `the console host is started from nativeLibraryDir with an absolute path`() {
        val paths = StackPaths(FILES_DIR)
        val client = MainActivity.builtInClient(NATIVE_LIBRARY_DIR, paths)
        val spec = MainActivity.consoleHostSpec(
            nativeLibraryDir = NATIVE_LIBRARY_DIR,
            paths = paths,
            client = client,
            socketName = SOCKET_NAME,
            grid = TerminalGrid(cols = 100, rows = 30),
        )

        // Android's seccomp policy kills a process that resolves an unqualified program
        // name, so the host is named by its absolute path rather than looked up. This is
        // not a style preference: the alternative is a session that dies with no output.
        assertEquals("$NATIVE_LIBRARY_DIR/libgorcons.so", spec.binary)
        assertTrue("the host is not named absolutely: ${spec.binary}", spec.binary.startsWith("/"))

        // The terminal the client is started on, and the name it will connect back on, are
        // both stated on the command line: the host has nowhere else to read them from.
        assertEquals(SOCKET_NAME, spec.argv.argAfter("--socket"))
        assertEquals("100", spec.argv.argAfter("--cols"))
        assertEquals("30", spec.argv.argAfter("--rows"))

        // Every path on that command line is absolute too, for the same reason the host's is.
        val command = spec.argv.argAfter("--command")
        assertNotNull("the console host was given nothing to run", command)
        assertTrue("the client is not named absolutely: $command", command!!.startsWith("/"))

        // The host runs in the client's home, so a client that writes a relative path writes
        // it into the appliance's storage rather than wherever the host happened to start.
        assertEquals(client.home, spec.env["HOME"])
        assertEquals("${paths.logDir}/gorcons.log", spec.logFile)
    }

    @Test
    fun `the console is given the client and not a shell`() {
        val paths = StackPaths(FILES_DIR)
        val client = MainActivity.builtInClient(NATIVE_LIBRARY_DIR, paths)
        val spec = MainActivity.consoleHostSpec(
            nativeLibraryDir = NATIVE_LIBRARY_DIR,
            paths = paths,
            client = client,
            socketName = SOCKET_NAME,
            grid = TerminalGrid(cols = 80, rows = 24),
        )

        val command = spec.argv.argAfter("--command")
        assertEquals(
            "the console host is running something other than the built-in client",
            client.binary,
            command,
        )
        assertTrue(
            "the console host was pointed at the Termux client asset instead of the APK's own: $command",
            command!!.endsWith("/lib${MainActivity.CLIENT_NAME}.so"),
        )
        for (shell in SHELLS) {
            assertNotEquals("the console host was given a shell", shell, command)
        }

        // The client's own arguments have to arrive at the client, not at the host: the host
        // takes its own flags and nothing else, so a client argument left bare is a host that
        // refuses to start.
        assertEquals(client.argv, spec.argv.clientArguments())
    }

    @Test
    fun `the client is not started until the transport is up`() {
        // The built-in client requires the shared instance the appliance's transport owns,
        // and it looks for one for a fifth of a second before giving up with "no shared
        // instance is running". A console opened the instant Start stack was tapped — which
        // is what one tap means — would therefore be a console showing a client that died
        // during startup, and the tap would look like the client being broken.
        //
        // So the wait is bounded and it is waited out before the console exists at all. The
        // probe is injected: it is a socket connection on the device, and the arithmetic of
        // how often and how long is what can be asserted here.
        val probes = mutableListOf<Int>()
        val waits = mutableListOf<Long>()
        val ready = MainActivity.awaitTransport(
            port = PORT,
            attempts = 4,
            waitMs = 250L,
            probe = { probes += it; false },
            sleep = { waits += it },
        )

        assertFalse("a transport that never came up was reported as up", ready)
        assertEquals("the wait did not use up its attempts", 4, probes.size)
        assertEquals("the probe was not given the shared instance's port", listOf(PORT, PORT, PORT, PORT), probes)
        // The last attempt is not followed by a wait: having given up, there is nothing left
        // to wait for.
        assertEquals(listOf(250L, 250L, 250L), waits)

        // A transport that comes up is not waited on any further.
        val lateProbes = mutableListOf<Int>()
        val lateWaits = mutableListOf<Long>()
        var up = false
        val lateReady = MainActivity.awaitTransport(
            port = PORT,
            attempts = 40,
            waitMs = 250L,
            probe = { lateProbes += it; up },
            sleep = { lateWaits += it; if (lateWaits.size == 2) up = true },
        )

        assertTrue("a transport that came up was not noticed", lateReady)
        assertEquals("the wait kept probing after the transport answered", 3, lateProbes.size)
        assertEquals("the wait kept sleeping after the transport answered", 2, lateWaits.size)

        // And a transport that is already up costs nothing at all: the common case is Start
        // stack having been tapped earlier, and a console that slept before opening on
        // every tap would be a console that felt broken in the other direction.
        val instantWaits = mutableListOf<Long>()
        assertTrue(
            "a transport that was already up was not noticed",
            MainActivity.awaitTransport(
                port = PORT,
                attempts = 40,
                waitMs = 250L,
                probe = { true },
                sleep = { instantWaits += it },
            ),
        )
        assertEquals("an already-running transport was waited for", emptyList<Long>(), instantWaits)
    }

    @Test
    fun `the console is not rebuilt when the tablet is turned`() {
        // The console is a socket, a child process and a thread, and the terminal is a page
        // of this activity rather than an activity of its own so that all three outlive a
        // rotation. That only holds if Android never recreates the activity: a rotation is a
        // configuration change, and an activity that has not said it handles the change is
        // destroyed and rebuilt, which stops the session and kills the client. Turning the
        // tablet is then a terminal that closes itself.
        //
        // The claim has to be made in the manifest — there is nowhere else to make it — and
        // it is the kind of claim that is silently lost when an activity is edited, so it is
        // asserted here rather than trusted.
        val manifest = File(MANIFEST)
        assertTrue("the manifest is not with the module: $manifest", manifest.isFile)

        // The manifest's attributes are all in the `android:` namespace and none of them are
        // in its element namespace, so a parser that is not namespace-aware sees an activity
        // with no name at all.
        val factory = DocumentBuilderFactory.newInstance().apply { isNamespaceAware = true }
        val document = factory.newDocumentBuilder().parse(manifest)
        val activities = document.getElementsByTagName("activity")
        val declared = (0 until activities.length)
            .map { activities.item(it) as Element }
            .firstOrNull { it.getAttributeNS(ANDROID_NAMESPACE, "name") == ".${MainActivity::class.java.simpleName}" }
        assertNotNull("the manifest declares no activity for MainActivity", declared)

        val handled = declared!!
            .getAttributeNS(ANDROID_NAMESPACE, "configChanges")
            .split("|")
            .map { it.trim() }
            .filter { it.isNotEmpty() }

        // Every one of these is a way a rotation reaches the activity. Any one of them left
        // out is a rotation that recreates the activity, and a rotation that recreates the
        // activity ends the console.
        for (change in ROTATION_CHANGES) {
            assertTrue(
                "turning the tablet recreates the activity, which stops the console: the " +
                    "manifest does not declare $change, and declares ${handled.joinToString("|")}",
                change in handled,
            )
        }
    }

    /** argAfter is the value of a flag, or null when the flag is absent. */
    private fun List<String>.argAfter(flag: String): String? {
        val at = indexOf(flag)
        return if (at >= 0 && at + 1 < size) get(at + 1) else null
    }

    /** clientArguments is the arguments a console host command line hands to the client. */
    private fun List<String>.clientArguments(): List<String> {
        val out = mutableListOf<String>()
        var i = 0
        while (i < size) {
            if (this[i] == "--arg" && i + 1 < size) {
                out.add(this[i + 1])
                i += 2
            } else {
                i++
            }
        }
        return out
    }

    private companion object {
        /** A native library directory as the packager leaves it on a device. */
        const val NATIVE_LIBRARY_DIR = "/data/app/~~7Yq==/com.gmlewis.gonomadnet-2Qw==/lib/arm64"

        /** The app's private data directory, which is the only place it may write. */
        const val FILES_DIR = "/data/user/0/com.gmlewis.gonomadnet/files"

        /** A bound console socket name, which is what the host is told to dial. */
        const val SOCKET_NAME = "gonomadnet-console-1234-0123456789abcdef0123456789abcdef"

        /** The shared instance the appliance's transport owns, on the loopback interface. */
        const val PORT = NodeConfigSpec.DEFAULT_SHARED_INSTANCE_PORT

        /** The programs that are not the client, and must never be given to the console. */
        val SHELLS = listOf("/sh", "/system/bin/sh", "sh", "/bash", "bash", "/data/data/com.termux/files/usr/bin/bash")

        /** The manifest, as Gradle runs these tests from the module directory. */
        const val MANIFEST = "src/main/AndroidManifest.xml"

        /** Where the manifest's own attributes live, which is not its element namespace. */
        const val ANDROID_NAMESPACE = "http://schemas.android.com/apk/res/android"

        /** The configuration changes a rotation produces, every one of which must be handled. */
        val ROTATION_CHANGES = listOf("orientation", "screenSize", "screenLayout", "smallestScreenSize")
    }
}
