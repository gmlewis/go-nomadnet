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
import java.io.ByteArrayInputStream
import java.io.File
import java.io.InputStream

/**
 * Handing the client, the launchers and the terminal's configuration over to Termux.
 *
 * The appliance cannot install any of it itself — Termux's home directory is another
 * application's private data — so the whole of the appliance's half is "put the files where
 * both applications can see them and say what to type". The properties asserted here are the
 * ones that fail badly: a font that was not published must never be described by commands that
 * copy a file that is not there, a license that could not be published must not stop the font,
 * the one line a person has to paste must name the file that is really on the tablet rather
 * than the name this code asked MediaStore for, and what the device's font engine says about
 * the font must be reported whether or not it is good news — a font this device will not load
 * is a terminal that keeps drawing boxes, and the reader is standing in front of it.
 */
class HandoffTest {

    /** A sink that records what it was handed and answers with a path of its own choosing. */
    private class FakeDownloads(private val answer: (String) -> String?) : SharedDownloads {
        val handed = mutableListOf<String>()
        val contents = mutableMapOf<String, String>()

        override fun publish(name: String, mimeType: String, source: InputStream): String? {
            handed += name
            contents[name] = source.use { it.readBytes().decodeToString() }
            return answer(name)
        }
    }

    private fun assets(carried: Map<String, String>): (String) -> InputStream? =
        { name -> carried[name]?.let { ByteArrayInputStream(it.toByteArray()) } }

    /** A sink that accepts everything, as MediaStore does on a device with free storage. */
    private fun accepting() = FakeDownloads { name -> "$SHARED_DOWNLOADS_DIR/$name" }

    /** A font engine that says what it is told to say, and records the font it was shown. */
    private class FakeProbe(private val answer: String?) : FontProbe {
        val seen = mutableListOf<String>()

        override fun inspect(source: InputStream): String? {
            seen += source.use { it.readBytes().decodeToString() }
            return answer
        }
    }

    private fun engineSaying(answer: String?) = FakeProbe(answer)

    /** Everything a complete build of the appliance carries. */
    private fun completeBuild(): Map<String, String> = mapOf(
        CLIENT_ASSET_NAME to "the client",
        SETUP_SCRIPT_ASSET_NAME to "the setup script",
        STANDALONE_LAUNCHER_ASSET_NAME to "the standalone launcher",
        STACK_LAUNCHER_ASSET_NAME to "the attached launcher",
        STACK_CONFIG_ASSET_NAME to "the attached configuration",
        TERMINAL_FONT_FILE_NAME to "a font",
        TERMINAL_FONT_LICENSE_FILE_NAME to "the license",
        TMUX_CONFIG_FILE_NAME to "the tmux configuration",
        TERMINAL_COLORS_FILE_NAME to "the terminal colors",
    )

    private fun installed(carried: Map<String, String>, downloads: SharedDownloads = accepting(), probe: FontProbe = engineSaying("the engine is happy")) =
        HandoffInstaller(assets(carried), downloads, probe).install()

    private fun asset(name: String): File = File("src/main/assets/$name")

    @Test
    fun theProbedGlyphsAreTheCodepointsTheInterfaceDraws() {
        // A `\u` escape is exactly four hexadecimal digits, so the supplementary-plane Nerd
        // Font glyphs have to be written as code points: an escape form silently turns the menu
        // bar's decoration into an unrelated basic-plane character followed by a letter, and the
        // device then answers that a font covering everything covers nothing.
        assertEquals(
            listOf(0xF043B, 0xF0002, 0xF064E, 0xF003, 'A'.code),
            PROBED_GLYPHS.map { it.codePointAt(0) },
        )
        assertTrue("every entry is one character", PROBED_GLYPHS.all { it.codePointCount(0, it.length) == 1 })
    }

    @Test
    fun everythingTheTermuxSideNeedsIsPublished() {
        val downloads = accepting()

        val outcome = installed(completeBuild(), downloads)

        assertEquals(
            listOf(
                "$SHARED_DOWNLOADS_DIR/$CLIENT_ASSET_NAME",
                "$SHARED_DOWNLOADS_DIR/$SETUP_SCRIPT_ASSET_NAME",
                "$SHARED_DOWNLOADS_DIR/$STANDALONE_LAUNCHER_ASSET_NAME",
                "$SHARED_DOWNLOADS_DIR/$STACK_LAUNCHER_ASSET_NAME",
                "$SHARED_DOWNLOADS_DIR/$STACK_CONFIG_ASSET_NAME",
                "$SHARED_DOWNLOADS_DIR/$TERMINAL_FONT_FILE_NAME",
                "$SHARED_DOWNLOADS_DIR/$TERMINAL_FONT_LICENSE_FILE_NAME",
                "$SHARED_DOWNLOADS_DIR/$TMUX_CONFIG_FILE_NAME",
                "$SHARED_DOWNLOADS_DIR/$TERMINAL_COLORS_FILE_NAME",
            ),
            outcome.published,
        )
        assertEquals(emptyList<String>(), outcome.failures)
        assertTrue(outcome.clientPublished)
        assertEquals("the client", downloads.contents[CLIENT_ASSET_NAME])
        assertEquals("the setup script", downloads.contents[SETUP_SCRIPT_ASSET_NAME])
        assertEquals("the standalone launcher", downloads.contents[STANDALONE_LAUNCHER_ASSET_NAME])
        assertEquals("the attached launcher", downloads.contents[STACK_LAUNCHER_ASSET_NAME])
        assertEquals("the attached configuration", downloads.contents[STACK_CONFIG_ASSET_NAME])
    }

    @Test
    fun theClientIsPublishedAsABinaryAndTheScriptsAsText() {
        // MediaStore validates the type it is given: a binary published as an unknown type is
        // one it refuses outright, and a script published as a binary is one a person cannot
        // open to read before running.
        val seen = mutableMapOf<String, String>()
        val downloads = object : SharedDownloads {
            override fun publish(name: String, mimeType: String, source: InputStream): String? {
                seen[name] = mimeType
                source.close()
                return "$SHARED_DOWNLOADS_DIR/$name"
            }
        }

        installed(completeBuild(), downloads)

        assertEquals("application/octet-stream", seen[CLIENT_ASSET_NAME])
        assertEquals("text/plain", seen[SETUP_SCRIPT_ASSET_NAME])
        assertEquals("text/plain", seen[STANDALONE_LAUNCHER_ASSET_NAME])
        assertEquals("text/plain", seen[STACK_LAUNCHER_ASSET_NAME])
        assertEquals("text/plain", seen[STACK_CONFIG_ASSET_NAME])
        assertEquals("font/otf", seen[TERMINAL_FONT_FILE_NAME])
        assertEquals("text/plain", seen[TERMINAL_FONT_LICENSE_FILE_NAME])
        assertEquals("text/plain", seen[TMUX_CONFIG_FILE_NAME])
        assertEquals("text/plain", seen[TERMINAL_COLORS_FILE_NAME])
    }

    @Test
    fun theOnlyLineAPersonHasToPasteRunsTheScriptThatWasActuallyPublished() {
        // One line, and it names the path the sink answered with rather than a path this code
        // assumed: MediaStore renames a file rather than overwriting it, so an assumed name is
        // a command that runs nothing.
        val downloads = FakeDownloads { name -> "/storage/emulated/0/Download/pub-$name" }
        val outcome = installed(completeBuild(), downloads)

        assertEquals(
            listOf("bash /storage/emulated/0/Download/pub-$SETUP_SCRIPT_ASSET_NAME"),
            outcome.pasteLines,
        )
        // And the path is recorded rather than recovered from the line later: MediaStore stores
        // a text file under the name Android's type table gives `text/plain`, which appends an
        // extension of its own, so a search for the file name would find nothing.
        assertEquals("/storage/emulated/0/Download/pub-$SETUP_SCRIPT_ASSET_NAME", outcome.setupScript)
    }

    @Test
    fun aBuildWithNoSetupScriptHasNoScriptForTheApplianceToRun() {
        // The appliance hands this path to Termux, so on a build that carries no script it has
        // nothing to hand over, and it must not hand over the font's path instead.
        val outcome = installed(mapOf(TERMINAL_FONT_FILE_NAME to "a font"))

        assertEquals(null, outcome.setupScript)
    }

    @Test
    fun aBuildWithNoSetupScriptFallsBackToTheFontsOwnCopyLines() {
        // A build that carries no script still carries a font, and a font is worth installing:
        // the fallback is the three lines an earlier release printed, and they name the font
        // that is really there.
        val outcome = installed(
            mapOf(
                TERMINAL_FONT_FILE_NAME to "a font",
                TERMINAL_FONT_LICENSE_FILE_NAME to "the license",
            ),
            FakeDownloads { name -> "/storage/emulated/0/Download/pub-$name" },
        )

        assertEquals(
            listOf(
                "mkdir -p ~/.termux",
                "cp /storage/emulated/0/Download/pub-$TERMINAL_FONT_FILE_NAME ~/.termux/font.ttf",
                "termux-reload-settings",
            ),
            outcome.pasteLines,
        )
        assertFalse("no client was published, so nothing can be set up", outcome.clientPublished)
    }

    @Test
    fun aBuildThatCarriesNeitherTheClientNorTheScriptHasNothingToPaste() {
        // The one case where the screen has to say the hand-off did not happen rather than
        // print a command: a command against a file that is not on the tablet sends somebody
        // into Termux to watch it fail.
        val outcome = installed(emptyMap())

        assertEquals(emptyList<String>(), outcome.pasteLines)
        assertEquals(null, outcome.setupScript)
        assertFalse(outcome.usable)
        assertFalse(outcome.clientPublished)
        // Every file the hand-off asks for is named, and not only the two that matter most: a
        // file that went missing without a line about it is a screen that says the install
        // worked while the terminal draws boxes.
        assertEquals(9, outcome.failures.size)
        for (name in completeBuild().keys) {
            assertTrue("$name was not reported", outcome.failures.any { it.contains(name) })
        }
        assertTrue(outcome.failures.any { it.contains(CLIENT_ASSET_NAME) })
        assertTrue(outcome.failures.any { it.contains(SETUP_SCRIPT_ASSET_NAME) })
    }

    @Test
    fun aFontThatCouldNotBePublishedDoesNotCostThePersonTheSetupLine() {
        // The script carries the font too, so a font the sink refused is a font the script
        // reports and works around. Withdrawing the line over it would withhold the client.
        val downloads = FakeDownloads { if (it == TERMINAL_FONT_FILE_NAME) null else "$SHARED_DOWNLOADS_DIR/$it" }

        val outcome = installed(completeBuild(), downloads)

        assertEquals(1, outcome.failures.size)
        assertTrue(outcome.failures[0].contains(SHARED_DOWNLOADS_DIR))
        assertEquals(
            listOf("bash $SHARED_DOWNLOADS_DIR/$SETUP_SCRIPT_ASSET_NAME"),
            outcome.pasteLines,
        )
        assertTrue(outcome.clientPublished)
    }

    @Test
    fun aLicenseThatCouldNotBePublishedDoesNotStopTheFont() {
        val downloads = FakeDownloads { if (it == TERMINAL_FONT_LICENSE_FILE_NAME) null else "$SHARED_DOWNLOADS_DIR/$it" }

        val outcome = installed(completeBuild(), downloads)

        assertEquals(1, outcome.failures.size)
        assertTrue("the font is what was asked for and must stay installable", outcome.engine != null)
        assertTrue(outcome.published.any { it.endsWith(TERMINAL_FONT_FILE_NAME) })
    }

    @Test
    fun theEngineIsAskedAboutTheFontTheApkCarriesAndItsAnswerIsReported() {
        // Asked about the font this appliance carries rather than about the published copy,
        // whose path another application can rename or delete between the two steps.
        val probe = engineSaying("the font loads and covers every glyph")
        val outcome = installed(completeBuild(), probe = probe)

        assertEquals(listOf("a font"), probe.seen)
        assertEquals("the font loads and covers every glyph", outcome.engine)
    }

    @Test
    fun aFontTheEngineWillNotLoadIsStillReportedAsBoxes() {
        // The failure that matters and that publishing cannot see: a font file can be written
        // to storage perfectly and still be one this device will not draw with, which is the
        // case where the reader has done everything asked and still sees empty boxes.
        val outcome = installed(completeBuild(), probe = engineSaying(null))

        assertTrue(outcome.engine!!.contains("boxes"))
    }

    @Test
    fun noFontMeansTheEngineIsNotAskedAndHasNothingToReport() {
        val probe = engineSaying("the engine is happy")
        val outcome = installed(mapOf(CLIENT_ASSET_NAME to "the client"), probe = probe)

        assertEquals(emptyList<String>(), probe.seen)
        assertEquals(null, outcome.engine)
    }

    @Test
    fun theApkCarriesTheFilesTheseTestsClaimItDoes() {
        // Each of these is looked up by the name the appliance publishes it under, so a rename
        // is a file that never arrives. It is asserted on the asset rather than on the
        // installer because the installer's own tests hand it a map of their own.
        for (name in listOf(
            SETUP_SCRIPT_ASSET_NAME,
            STANDALONE_LAUNCHER_ASSET_NAME,
            STACK_LAUNCHER_ASSET_NAME,
            STACK_CONFIG_ASSET_NAME,
            TERMINAL_COLORS_FILE_NAME,
            TMUX_CONFIG_FILE_NAME,
        )) {
            assertTrue("the APK does not carry $name", asset(name).isFile)
        }
    }

    @Test
    fun theLaunchersAreTermuxBashTargets() {
        // Termux:Widget runs a shortcut through its own bash, and the shebang has to be the
        // path inside Termux's prefix: `/usr/bin/bash` does not exist on Android.
        val shebang = "#!/data/data/com.termux/files/usr/bin/bash"
        for (name in listOf(STANDALONE_LAUNCHER_ASSET_NAME, STACK_LAUNCHER_ASSET_NAME)) {
            assertEquals(name, shebang, asset(name).readLines().first())
        }
    }

    @Test
    fun theAttachedConfigurationAgreesWithThisApplianceOnTheSharedInstance() {
        // A mismatch here produces "no shared instance is running" and nothing else, which is
        // the least debuggable failure this appliance can produce. The port is asserted against
        // the Kotlin constant the transport is rendered from, so the two cannot drift apart.
        val text = asset(STACK_CONFIG_ASSET_NAME).readText()

        assertTrue("the shared instance is not a TCP socket", text.contains("shared_instance_type = tcp"))
        assertTrue("the port does not match the appliance", text.contains("shared_instance_port = $STACK_CONFIG_SHARED_INSTANCE_PORT"))
        assertTrue("the instance is not named", text.contains("instance_name = default"))
        assertTrue("the client may take ownership of the transport", text.contains("require_shared_instance = yes"))
        assertTrue("a second transport would build a second network", text.contains("enable_transport = no"))
        assertFalse(
            "no interface may be enabled here: the appliance owns every one of them",
            text.lines().any { it.trim() == "interface_enabled = yes" },
        )
    }

    @Test
    fun theApkCarriesTheTerminalColorsItPromises() {
        val text = asset(TERMINAL_COLORS_FILE_NAME).readText()
        // The three things the client's chrome depends on. The frame borders, pane titles and
        // key hints are drawn in the terminal's *default* foreground, so a colors.properties
        // without a "foreground" key leaves every one of them in whatever white Termux ships
        // while the rest of the interface is themed — and a palette missing an entry leaves
        // whichever accent used it in Termux's own color.
        assertTrue("the default foreground is not set", text.contains("foreground = #"))
        assertTrue("the default background is not set", text.contains("background = #"))
        for (index in 0..15) {
            assertTrue("color$index is not set", text.contains("color$index = #"))
        }
    }

    @Test
    fun theApkCarriesTheTmuxConfigurationItPromises() {
        val text = asset(TMUX_CONFIG_FILE_NAME).readText()
        // True color has to survive both hops. tmux only emits a true-color escape sequence
        // when it believes the terminal it runs in can take one, and the programs inside a
        // pane only emit one when they are told they run under a name whose capabilities
        // include RGB. A configuration missing either half is a tmux that downsamples every
        // color to 256.
        assertTrue("tmux will downsample the terminal's true color", text.contains(":Tc"))
        assertTrue("tmux will downsample a pane's true color", text.contains(":RGB"))
        assertTrue(
            "a pane's programs are not told they run under an RGB-capable terminal",
            text.contains("default-terminal \"tmux-256color\""),
        )
    }

    @Test
    fun theSetupScriptAndTheLaunchersPointAtEachOther() {
        // The launchers' repair message is the only instruction a person sees when the client
        // is not installed, and it has to name the file that is really published.
        for (name in listOf(STANDALONE_LAUNCHER_ASSET_NAME, STACK_LAUNCHER_ASSET_NAME)) {
            assertTrue(
                "$name does not tell the reader how to install the client",
                asset(name).readText().contains(SETUP_SCRIPT_ASSET_NAME),
            )
        }
    }
}
