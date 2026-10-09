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
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.w3c.dom.Element
import java.io.File
import javax.xml.parsers.DocumentBuilderFactory

/**
 * How the appliance runs the client itself, and what the package is made of.
 *
 * The built-in client is started entirely out of the APK: it is this app's own binary,
 * running in this app's own storage, attached to the transport this app owns. So there is
 * no other app in the story, and the two halves of that — the command line the client is
 * started with, and what the package asks the system for — are asserted here rather than
 * trusted, because neither can be seen from a passing run: a client pointed at the
 * transport's own Reticulum directory silently becomes a second transport, a package that
 * still asks to install apps shows a person a permission dialog nothing in the app
 * explains, and a package that still carries another app's setup files is a download that
 * is larger than it needs to be and a lie about what it does.
 *
 * [MainActivity] is never instantiated. These are its companion's pure helpers plus the
 * module's own manifest and assets, and the class is only named — the tests below are
 * arithmetic on a command line and a reading of the files Gradle packages.
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
    fun `the client is told one wheel notch is one row`() {
        // A finger drag reaches the client as wheel notches, one per row of the screen it
        // travelled, so what the page does is what the client does with a notch. The client's
        // own default moves several rows a notch, and a drag then runs away from the finger —
        // the "scrolling at 2X the speed of my finger" report. A multiplier the wire cannot
        // express is not a fix, so the client is told what one notch means instead.
        val paths = StackPaths(FILES_DIR)
        val client = MainActivity.builtInClient(NATIVE_LIBRARY_DIR, paths)
        val spec = MainActivity.consoleHostSpec(
            nativeLibraryDir = NATIVE_LIBRARY_DIR,
            paths = paths,
            client = client,
            socketName = SOCKET_NAME,
            grid = TerminalGrid(cols = 100, rows = 30),
        )

        assertEquals(
            "the console host does not tell the client how far a notch should move",
            "GONOMADNET_WHEEL_LINES=1",
            spec.argv.argAfter("--env"),
        )
        assertEquals(
            "the wheel-lines variable the host passes is not the one the client reads",
            MainActivity.WHEEL_LINES_ENV,
            "GONOMADNET_WHEEL_LINES=1",
        )

        // It is the client's environment and not the host's: the host would ignore it, and
        // the console would look fixed while nothing had changed.
        assertNull("the wheel lines were set on the host instead of the client", spec.env["GONOMADNET_WHEEL_LINES"])
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
        val document = manifest()
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

    @Test
    fun `the appliance asks the system for nothing it does not need`() {
        // The appliance is one APK that starts both halves out of its own storage, so it
        // installs nothing and it runs nothing in another app. Every permission here is
        // something the system puts in front of a person, and a permission the app no longer
        // uses is a dialog nobody can explain — on the install it is a line in the app's
        // listing that reads like a request to install software behind the person's back.
        val document = manifest()
        val permissions = document.elements("uses-permission")
            .map { it.getAttributeNS(ANDROID_NAMESPACE, "name") }
        for (gone in UNUSED_PERMISSIONS) {
            assertFalse(
                "the appliance still asks the system for $gone, which nothing in it uses: " +
                    "the manifest declares ${permissions.joinToString(", ")}",
                gone in permissions,
            )
        }

        // And it asks about no other app: a `<queries>` entry is how a package says it means
        // to resolve another app's components, and this one resolves none.
        val queried = document.elements("queries")
            .flatMap { query -> query.childNodes.asElementList().map { it.getAttributeNS(ANDROID_NAMESPACE, "name") } }
        assertTrue(
            "the appliance still asks the system about another app's package: " +
                queried.joinToString(", "),
            queried.none { it == OTHER_PACKAGE },
        )

        // The receiver that answer arrived at is gone with the question. A manifest entry
        // naming a class that no longer exists is a package that fails to install, so this
        // is also the check that the removal was complete.
        val receivers = document.elements("receiver")
            .map { it.getAttributeNS(ANDROID_NAMESPACE, "name") }
        for (gone in UNUSED_RECEIVERS) {
            assertFalse(
                "the manifest still declares the receiver $gone, which no longer exists",
                gone in receivers.map { it.substringAfterLast('.') },
            )
            assertFalse(
                "the package still carries the source for $gone, which nothing starts",
                File(SOURCES_DIR, "$gone.kt").isFile,
            )
        }
    }

    @Test
    fun `the package carries nothing for another app to run`() {
        // What is in the APK is what it downloads as, and a setup script for an app this one
        // no longer talks to is dead weight in every download — and worse than dead weight,
        // because it says the appliance does something it does not. The files that are kept
        // are kept for reasons of their own: the font is what the console draws with, the
        // client installs the terminal colours and the tmux configuration into its own home,
        // and the Bible text is what the bundled offline page reads.
        val assets = File(ASSETS_DIR)
        assertTrue("the assets are not with the module: $assets", assets.isDirectory)
        val present = assets.list()?.toList().orEmpty()

        for (gone in SETUP_ASSETS) {
            assertFalse(
                "the package still carries $gone, which nothing in the appliance reads: " +
                    "it carries ${present.joinToString(", ")}",
                gone in present,
            )
        }
        for (kept in KEPT_ASSETS) {
            assertTrue(
                "the package no longer carries $kept, which it reads at runtime: " +
                    "it carries ${present.joinToString(", ")}",
                kept in present,
            )
        }
    }

    @Test
    fun `the appliance is dark, and its system bars are the console's own dark`() {
        // The appliance is a terminal, and a terminal is a dark surface. Left on the
        // platform's bare default, the window takes the device's own accent instead: a
        // bright bar across the top of a dark terminal, and on a device whose default is a
        // light theme a light bar with dark icons. Neither is the console's colour, and the
        // console is the whole of what this screen shows.
        //
        // The colour is not written down twice: the theme's bar is the console's own
        // background, and the test reads the palette rather than repeating it, so a bar and
        // a console that have drifted apart are a failure here instead of something nobody
        // notices until it is on a tablet.
        //
        // The window's own background is the same colour and for the same reason. The
        // console is inset from the system bars, so the window shows through in a band
        // along the bottom of the screen: on the platform's default that band is the
        // device theme's grey, a seam across a terminal that is otherwise one surface.
        val application = manifest().elements("application").single()
        assertEquals(
            "the appliance does not wear its own theme: the console is then drawn under a " +
                "bar in the device's accent colour",
            "@style/ApplianceTheme",
            application.getAttributeNS(ANDROID_NAMESPACE, "theme"),
        )

        // The theme carries no title bar, and that is not a detail of style. A bar that
        // exists and is hidden at runtime is a bar the window has already measured itself
        // around — `ActionBar.hide()` slides the content view up by the bar's height and
        // never grows it, so the client's first rows end up above the top of the screen and
        // a band exactly that tall is left empty at the bottom. The console is the client's
        // whole screen, and there is nothing a title would say.
        assertEquals(
            "the appliance's theme has an action bar, which the console cannot spare the rows for",
            "@android:style/Theme.DeviceDefault.NoActionBar",
            resource("styles.xml", "style", "ApplianceTheme").getAttribute("parent"),
        )

        val style = resource("styles.xml", "style", "ApplianceTheme").elements("item")
        for (name in listOf("android:statusBarColor", "android:windowBackground")) {
            val item = style.firstOrNull { it.getAttribute("name") == name }
            assertNotNull("ApplianceTheme sets no $name", item)
            assertEquals(
                "the appliance does not take the console's own background for $name",
                "@color/console_background",
                item!!.textContent.trim(),
            )
        }

        val background = resource("colors.xml", "color", "console_background").textContent.trim()
        assertEquals(
            "the system bars and the console have drifted apart: the console draws " +
                "${String.format("#%08x", TerminalPalette.DEFAULT_BACKGROUND)}",
            String.format("#%08x", TerminalPalette.DEFAULT_BACKGROUND),
            background,
        )
    }

    /** resource is one named element of one of the module's value files. */
    private fun resource(file: String, tag: String, name: String): Element {
        val path = File(RES_DIR, "values/$file")
        assertTrue("the resource file is not with the module: $path", path.isFile)
        val factory = DocumentBuilderFactory.newInstance().apply { isNamespaceAware = true }
        val document = factory.newDocumentBuilder().parse(path)
        val found = document.elements(tag).firstOrNull { it.getAttribute("name") == name }
        assertNotNull("$path declares no <$tag name=\"$name\">", found)
        return found!!
    }

    /** manifest is the module's manifest, parsed so its android: attributes can be read. */
    private fun manifest(): org.w3c.dom.Document {
        val file = File(MANIFEST)
        assertTrue("the manifest is not with the module: $file", file.isFile)
        // The manifest's attributes are all in the `android:` namespace and none of them are
        // in its element namespace, so a parser that is not namespace-aware sees an activity
        // with no name at all.
        val factory = DocumentBuilderFactory.newInstance().apply { isNamespaceAware = true }
        return factory.newDocumentBuilder().parse(file)
    }

    /** elements is every element of one tag name, in document order. */
    private fun org.w3c.dom.Document.elements(tag: String): List<Element> =
        (0 until getElementsByTagName(tag).length).map { getElementsByTagName(tag).item(it) as Element }

    /** elements is every descendant of one tag name, in document order. */
    private fun Element.elements(tag: String): List<Element> =
        (0 until getElementsByTagName(tag).length).map { getElementsByTagName(tag).item(it) as Element }

    /** asElementList is the element children of a node, with text and comments dropped. */
    private fun org.w3c.dom.NodeList.asElementList(): List<Element> =
        (0 until length).mapNotNull { item(it) as? Element }

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

        /** The module's sources, so a class the package no longer has can be looked for. */
        const val SOURCES_DIR = "src/main/java/com/gmlewis/gonomadnet"

        /** The module's resources, where the appliance's own theme is written down. */
        const val RES_DIR = "src/main/res"

        /** What Gradle packages into the APK, which is what a person downloads. */
        const val ASSETS_DIR = "src/main/assets"

        /** Where the manifest's own attributes live, which is not its element namespace. */
        const val ANDROID_NAMESPACE = "http://schemas.android.com/apk/res/android"

        /** The configuration changes a rotation produces, every one of which must be handled. */
        val ROTATION_CHANGES = listOf("orientation", "screenSize", "screenLayout", "smallestScreenSize")

        /**
         * Permissions for a thing the appliance no longer does.
         *
         * `REQUEST_INSTALL_PACKAGES` was for installing another app, and
         * `com.termux.permission.RUN_COMMAND` for asking it to run a program. The appliance
         * starts both halves out of its own storage, so it needs neither — and a listing
         * that asks to install software is the kind of thing a person is right to refuse.
         */
        val UNUSED_PERMISSIONS = listOf(
            "android.permission.REQUEST_INSTALL_PACKAGES",
            "com.termux.permission.RUN_COMMAND",
        )

        /** The package the appliance no longer asks the system about. */
        const val OTHER_PACKAGE = "com.termux"

        /** The receiver the install answer arrived at, gone with the question. */
        val UNUSED_RECEIVERS = listOf("InstallResultReceiver")

        /**
         * Setup files for another app, which no code in the appliance reads.
         *
         * `gonomadnet-client` is the client as an asset — the copy that used to be handed
         * over; the app's client is `libgonomadnetclient.so` in its own native library
         * directory, built by the same script. It is not tracked by git, so a package built
         * after this change is what proves it is gone.
         */
        val SETUP_ASSETS = listOf(
            "gonomadnet-setup.sh",
            "gonomadnet-standalone",
            "gonomadnet-stack",
            "reticulum-stack-config",
            "gonomadnet-client",
        )

        /**
         * What the package is supposed to carry, and what reads each one.
         *
         * The font is the console's, and its licence has to travel with it; the colours and
         * the tmux configuration are installed into the client's own home by the client's
         * install-from-storage feature; and the Bible text is the bundled offline page.
         */
        val KEPT_ASSETS = listOf(
            "AtkynsonMonoNerdFontMono-Regular.otf",
            "AtkinsonHyperlegibleMono-OFL.txt",
            "colors.properties",
            "tmux.conf",
            "kjv.txt",
        )
    }
}
