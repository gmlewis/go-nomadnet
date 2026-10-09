// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File
import java.nio.file.Files
import java.nio.file.attribute.PosixFilePermission

/**
 * The script the appliance publishes into shared storage and asks Termux to run.
 *
 * It is the whole of the Termux side of the installation: Termux's home directory is another
 * application's private data, so the appliance can publish the files but only a process
 * running as Termux can put them where Termux reads them. Everything the appliance cannot do
 * for itself therefore lives in this one script, and it is run here for real — with `bash`, a
 * home directory of its own, and a stand-in for each Termux program it looks for — because a
 * script that is only read is a script whose quoting bugs ship.
 *
 * The rules asserted are the ones that fail badly on a tablet: a client that lands somewhere
 * the launcher does not look, a launcher that is not executable and so does nothing when its
 * icon is tapped, a configuration this overwrites and thereby destroys, and an edit to
 * Termux's own properties file that either loses the settings already in it or accumulates a
 * duplicate of its own line every time it runs.
 */
class SetupScriptTest {

    private val scratch = mutableListOf<File>()

    @After
    fun cleanUp() {
        scratch.forEach { it.deleteRecursively() }
        scratch.clear()
    }

    /**
     * A temporary directory.
     *
     * `/tmp` is named explicitly rather than left to `java.io.tmpdir`, because on macOS that
     * is a path under `/var/folders` long enough to break anything that opens a Unix socket
     * in it, and the same helper in this repository's Go tests exists for the same reason.
     */
    private fun tempDir(prefix: String): File {
        val base = if (System.getProperty("os.name").orEmpty().startsWith("Mac")) File("/tmp") else File(checkNotNull(System.getProperty("java.io.tmpdir")))
        return Files.createTempDirectory(base.toPath(), prefix).toFile().also { scratch += it }
    }

    /** The script under test, exactly as it is shipped inside the APK. */
    private val script = File("src/main/assets/$SETUP_SCRIPT_ASSET_NAME")

    /** What running the script did, in the terms the appliance's screen reads back. */
    private data class Run(val exitCode: Int, val output: String) {
        val failed: Boolean get() = exitCode != 0
    }

    /**
     * Runs the shipped script against a home directory and a shared directory of its own.
     *
     * [path] is the `PATH` the script is given. It is not the test's own, because the script's
     * behaviour depends on which Termux programs it can find: with none of them present it has
     * to install everything short of the reload and say so, which is the case asserted here.
     */
    private fun run(home: File, shared: File, path: String? = null): Run {
        val builder = ProcessBuilder("bash", script.absolutePath, shared.absolutePath)
        builder.directory(script.absoluteFile.parentFile)
        builder.environment()["HOME_DIR"] = home.absolutePath
        builder.environment()["HOME"] = home.absolutePath
        builder.environment().remove("PREFIX")
        builder.environment()["PATH"] = path ?: "/usr/bin:/bin"
        val process = builder.start()
        val output = process.inputStream.bufferedReader().readText() +
            process.errorStream.bufferedReader().readText()
        return Run(process.waitFor(), output)
    }

    /** Every published file the appliance's installer writes, named as MediaStore stores it. */
    private fun publishEverything(shared: File): File {
        shared.mkdirs()
        write(shared, "$CLIENT_ASSET_NAME.bin", "the client binary")
        write(shared, STANDALONE_LAUNCHER_ASSET_NAME, "the standalone launcher")
        write(shared, STACK_LAUNCHER_ASSET_NAME, "the stack launcher")
        write(shared, STACK_CONFIG_ASSET_NAME, "the stack configuration")
        write(shared, "$TERMINAL_FONT_FILE_NAME.ttf", "a font")
        return shared
    }

    private fun write(dir: File, name: String, contents: String) {
        File(dir, name).writeText(contents)
    }

    private fun modeOf(file: File): Set<PosixFilePermission> =
        Files.getPosixFilePermissions(file.toPath())

    @Test
    fun theClientIsInstalledWhereTheLaunchersLookForItAndIsExecutable() {
        // The launchers exec "$HOME/gonomadnet", so this is the one path that cannot be moved,
        // and an installed client that is not executable is an icon that reports
        // "Permission denied" with nothing else to go on.
        val home = tempDir("home-")
        val shared = publishEverything(tempDir("shared-"))

        val run = run(home, shared)

        assertFalse(run.failed)
        val client = File(home, CLIENT_INSTALLED_NAME)
        assertEquals("the client binary", client.readText())
        assertTrue("the client is not executable", client.canExecute())
        assertTrue(modeOf(client).contains(PosixFilePermission.OWNER_EXECUTE))
    }

    @Test
    fun theClientIsFoundUnderTheNameMediaStoreGaveIt() {
        // MediaStore appends the type's own extension to a published name — "font/otf" is why
        // the font arrives as ".otf.ttf" — so the name in Downloads is not the name the asset
        // has, and a script that copies the assumed name copies nothing.
        val home = tempDir("home-")
        val shared = tempDir("shared-")
        shared.mkdirs()
        write(shared, "$CLIENT_ASSET_NAME.bin", "the client binary")

        val run = run(home, shared)

        assertFalse(run.output, run.failed)
        assertEquals("the client binary", File(home, CLIENT_INSTALLED_NAME).readText())
    }

    @Test
    fun theReleaseBinaryIsAlsoAcceptedWhenSomebodyDownloadedItByHand() {
        val home = tempDir("home-")
        val shared = tempDir("shared-")
        shared.mkdirs()
        write(shared, "gonomadnet-0.164.0-linux-arm64", "the release binary")

        val run = run(home, shared)

        assertFalse(run.output, run.failed)
        assertEquals("the release binary", File(home, CLIENT_INSTALLED_NAME).readText())
    }

    @Test
    fun theLaunchersLandInTheShortcutsDirectoryWithTheModesTermuxRequires() {
        // Termux:Widget refuses a ~/.shortcuts that is group- or world-readable, and a target
        // script that is not executable, so the two modes are part of installing them.
        val home = tempDir("home-")
        val shared = publishEverything(tempDir("shared-"))

        assertFalse(run(home, shared).failed)

        val shortcuts = File(home, ".shortcuts")
        assertEquals(setOf(PosixFilePermission.OWNER_READ, PosixFilePermission.OWNER_WRITE, PosixFilePermission.OWNER_EXECUTE), modeOf(shortcuts))
        val standalone = File(shortcuts, STANDALONE_SHORTCUT_NAME)
        val attached = File(shortcuts, "$STANDALONE_SHORTCUT_NAME+stack")
        assertEquals("the standalone launcher", standalone.readText())
        assertEquals("the stack launcher", attached.readText())
        assertTrue(standalone.canExecute())
        assertTrue(attached.canExecute())
    }

    @Test
    fun aLauncherThatWasNotPublishedIsReportedAndDoesNotStopTheClient() {
        // The client is what the whole exercise is for. A launcher that did not arrive is a
        // missing icon, and refusing to install the client over it would be the wrong trade.
        val home = tempDir("home-")
        val shared = tempDir("shared-")
        shared.mkdirs()
        write(shared, "$CLIENT_ASSET_NAME.bin", "the client binary")

        val run = run(home, shared)

        assertFalse(run.output, run.failed)
        assertTrue(run.output, run.output.contains(STANDALONE_LAUNCHER_ASSET_NAME))
        assertTrue("the client is installed anyway", File(home, CLIENT_INSTALLED_NAME).exists())
    }

    @Test
    fun theAttachedConfigurationIsInstalledWithNoInterfacesOfItsOwn() {
        // Without it the attached launcher refuses to start, which is the failure the launcher
        // itself reports as "run the setup script".
        val home = tempDir("home-")
        val shared = publishEverything(tempDir("shared-"))

        assertFalse(run(home, shared).failed)

        val installed = File(home, ".reticulum-stack/config")
        assertEquals("the stack configuration", installed.readText())
        assertEquals("a Reticulum config holds private keys one day", setOf(PosixFilePermission.OWNER_READ, PosixFilePermission.OWNER_WRITE), modeOf(installed))
    }

    @Test
    fun aConfigurationThatIsAlreadyThereIsLeftExactlyAsItIs() {
        // The person who edited their configuration is the reason this is conservative: a
        // setup script that "repairs" their file on every run destroys the one thing on the
        // tablet that is theirs.
        val home = tempDir("home-")
        val shared = publishEverything(tempDir("shared-"))
        val existing = File(home, ".reticulum-stack/config")
        existing.parentFile?.mkdirs()
        existing.writeText("mine")

        assertFalse(run(home, shared).failed)

        assertEquals("mine", existing.readText())
    }

    @Test
    fun theTerminalFontIsInstalledOnlyWhenThereIsNotOneAlready() {
        // Same rule, and this one is visible: replacing a font the operator chose changes how
        // every glyph in the interface looks.
        val home = tempDir("home-")
        val shared = publishEverything(tempDir("shared-"))
        val font = File(home, ".termux/font.ttf")

        assertFalse(run(home, shared).failed)
        assertEquals("a font", font.readText())

        font.writeText("the font I chose")
        assertFalse(run(home, shared).failed)
        assertEquals("the font I chose", font.readText())
    }

    @Test
    fun allowExternalAppsIsSetWithoutLosingTheSettingsAlreadyInTheFile() {
        // Termux reads its own properties from this file, so an edit that rewrites it rather
        // than adding to it silently discards every setting the operator has made — including
        // the ones Termux's own settings screen writes there.
        val home = tempDir("home-")
        val shared = publishEverything(tempDir("shared-"))
        val properties = File(home, ".termux/termux.properties")
        properties.parentFile?.mkdirs()
        properties.writeText("extra-keys = [['ESC']]\n# allow-external-apps = true\n")

        assertFalse(run(home, shared).failed)

        val text = properties.readText()
        assertTrue(text, text.contains("extra-keys = [['ESC']]"))
        // Exactly one effective setting, with the value the appliance needs: the commented-out
        // line the file already had is a comment, not a setting, and a script that mistook one
        // for the other would report the work as already done and leave the buttons dead.
        assertEquals(
            listOf("allow-external-apps = true"),
            text.lines().map { it.trim() }.filter { it.startsWith("allow-external-apps") },
        )
    }

    @Test
    fun runningTwiceDoesNotAddASecondCopyOfTheSetting() {
        // The appliance runs this every time somebody taps the button, and a file that grows a
        // line per run is a file that ends up with a hundred of them.
        val home = tempDir("home-")
        val shared = publishEverything(tempDir("shared-"))
        val properties = File(home, ".termux/termux.properties")

        assertFalse(run(home, shared).failed)
        assertFalse(run(home, shared).failed)

        assertEquals(1, properties.readText().lines().count { it.trim() == "allow-external-apps = true" })
    }

    @Test
    fun storageIsSetUpBeforeAnythingIsLookedForInSharedStorage() {
        // The order is the whole point. Termux cannot read shared storage until it has been
        // granted it, so a script that looks for the client first finds nothing and reports a
        // missing client on a tablet where nothing is missing at all. The stand-in below does
        // what the real program does — makes the storage directory readable — and the client
        // is only reachable through it.
        val home = tempDir("home-")
        val shared = tempDir("shared-")
        shared.mkdirs()
        val bin = tempDir("bin-")
        val setupStorage = File(bin, "termux-setup-storage")
        setupStorage.writeText(
            "#!/bin/sh\n" +
                "mkdir -p \"\$HOME_DIR/storage/downloads\"\n" +
                "cp \"\$SOURCE_CLIENT\" \"\$HOME_DIR/storage/downloads/$CLIENT_ASSET_NAME.bin\"\n",
        )
        assertTrue(setupStorage.setExecutable(true))
        val source = File(shared, "the client, only reachable through the storage directory")
        source.writeText("the client binary")

        val builder = ProcessBuilder("bash", script.absolutePath, shared.absolutePath)
        builder.environment()["HOME_DIR"] = home.absolutePath
        builder.environment()["HOME"] = home.absolutePath
        builder.environment()["SOURCE_CLIENT"] = source.absolutePath
        builder.environment()["PATH"] = "${bin.absolutePath}:/usr/bin:/bin"
        val process = builder.start()
        val output = process.inputStream.bufferedReader().readText() +
            process.errorStream.bufferedReader().readText()
        val code = process.waitFor()

        assertEquals(output, 0, code)
        assertEquals("the client binary", File(home, CLIENT_INSTALLED_NAME).readText())
    }

    @Test
    fun theFirstRunWaitsForTheStorageGrantInsteadOfAskingForASecondRun() {
        // termux-setup-storage returns at once and the grant arrives when the person comes back
        // from the screen it opened, so a script that looked for the client immediately after it
        // would always fail on the first run of a new tablet. The stand-in here takes a second
        // to do what the real one does, which is the only reason the wait is observable at all.
        val home = tempDir("home-")
        val shared = tempDir("shared-")
        shared.mkdirs()
        val bin = tempDir("bin-")
        val setupStorage = File(bin, "termux-setup-storage")
        setupStorage.writeText(
            "#!/bin/sh\n" +
                "sleep 1\n" +
                "mkdir -p \"\$HOME_DIR/storage/downloads\"\n" +
                "cp \"\$SOURCE_CLIENT\" \"\$HOME_DIR/storage/downloads/$CLIENT_ASSET_NAME.bin\"\n",
        )
        assertTrue(setupStorage.setExecutable(true))
        val source = File(shared, "the client, only reachable through the storage directory")
        source.writeText("the client binary")

        val builder = ProcessBuilder("bash", script.absolutePath, shared.absolutePath)
        builder.environment()["HOME_DIR"] = home.absolutePath
        builder.environment()["HOME"] = home.absolutePath
        builder.environment()["SOURCE_CLIENT"] = source.absolutePath
        builder.environment()["PATH"] = "${bin.absolutePath}:/usr/bin:/bin"
        val process = builder.start()
        val output = process.inputStream.bufferedReader().readText() +
            process.errorStream.bufferedReader().readText()

        assertEquals(output, 0, process.waitFor())
        assertEquals("the client binary", File(home, CLIENT_INSTALLED_NAME).readText())
    }

    @Test
    fun aPersonWhoHasNeverGrantedStorageIsNotToldToPublishAgain() {
        // Two different failures with two different fixes. Telling somebody to tap "Publish files
        // for Termux" when Termux cannot see their Downloads at all sends them to do something
        // that changes nothing about what they are looking at.
        val home = tempDir("home-")
        val shared = tempDir("shared-")
        shared.mkdirs()

        val run = run(home, shared)

        assertTrue(run.failed)
        assertTrue(
            "the report has to name the storage grant, not the publishing button",
            run.output.contains("shared storage"),
        )
        assertFalse(
            "the appliance did publish; Termux simply cannot read it",
            run.output.contains("'Publish files for Termux' first"),
        )
    }

    @Test
    fun aSetupWithNoClientAnywhereFailsAndSaysWhereTheClientComesFrom() {
        // The one failure worth a non-zero exit: with no client there is nothing to run, and
        // the person needs to be told which button produces one rather than being left with a
        // home directory that looks installed and is not.
        val home = tempDir("home-")
        val shared = tempDir("shared-")
        shared.mkdirs()

        val run = run(home, shared)

        assertTrue("the exit status has to say it did not work", run.failed)
        assertFalse(File(home, CLIENT_INSTALLED_NAME).exists())
        assertTrue(run.output, run.output.contains(CLIENT_ASSET_NAME))
    }

    @Test
    fun whatItDidIsWrittenWhereTheApplianceCanReadIt() {
        // Termux's home directory cannot be read from adb or from the appliance, so shared
        // storage is the only return channel: without this file a setup that half worked is
        // indistinguishable from one that did nothing.
        val home = tempDir("home-")
        val shared = publishEverything(tempDir("shared-"))

        val run = run(home, shared)

        assertFalse(run.failed)
        val status = File(shared, SETUP_STATUS_FILE_NAME)
        assertTrue("no status file was written", status.isFile)
        assertTrue(status.readText(), status.readText().contains(CLIENT_INSTALLED_NAME))
        assertTrue("the readout has to reach the screen too", run.output.contains(CLIENT_INSTALLED_NAME))
    }

    @Test
    fun termuxIsToldToReloadItsSettingsWhenItCanBe() {
        // A font and a palette are adopted by a running terminal only when Termux re-reads its
        // settings, so an installation that never reloads is one the person has to restart the
        // app to see.
        val home = tempDir("home-")
        val shared = publishEverything(tempDir("shared-"))
        val bin = tempDir("bin-")
        val reloaded = File(shared, "reloaded")
        val reload = File(bin, "termux-reload-settings")
        reload.writeText("#!/bin/sh\ntouch \"\$SHARED_DIR/reloaded\"\n")
        assertTrue(reload.setExecutable(true))

        val builder = ProcessBuilder("bash", script.absolutePath, shared.absolutePath)
        builder.environment()["HOME_DIR"] = home.absolutePath
        builder.environment()["HOME"] = home.absolutePath
        builder.environment()["SHARED_DIR"] = shared.absolutePath
        builder.environment()["PATH"] = "${bin.absolutePath}:/usr/bin:/bin"
        val output = builder.start().let { it.inputStream.bufferedReader().readText() + it.errorStream.bufferedReader().readText() }

        assertTrue(output, reloaded.isFile)
    }

    @Test
    fun theScriptIsBashAndStartsWithTheShebangTermuxHas() {
        // A script run as `bash <path>` does not need a shebang, but the same file is also
        // offered to a person to run directly, and `/usr/bin/bash` does not exist on Android.
        val first = script.readLines().first()
        assertEquals("#!/data/data/com.termux/files/usr/bin/bash", first)
    }

    @Test
    fun theScriptDoesNotAssumeATermuxPrefixIsExported() {
        // RUN_COMMAND starts the program without a login shell, so the prefix the script needs
        // to find Termux's own programs may be absent; a path built from an unset PREFIX is a
        // reload that never happens.
        val text = script.readText()
        assertFalse("the prefix must be defaulted, not required", text.contains("set -u") && !text.contains("PREFIX:-"))
    }

    companion object {
        /** Where a shortcut script has to land inside Termux's home directory. */
        const val STANDALONE_SHORTCUT_NAME = "gonomadnet"

        /** The client's name once the script has installed it, which the launchers exec. */
        const val CLIENT_INSTALLED_NAME = "gonomadnet"
    }
}
