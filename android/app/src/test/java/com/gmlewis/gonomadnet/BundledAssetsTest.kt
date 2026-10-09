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
import java.io.IOException
import java.io.InputStream

/**
 * Installing an APK asset as a file the daemons can read.
 *
 * An asset lives inside the APK, which is a zip: no process outside the application can open
 * one, so a bundled data file has to be written out before the daemon that reads it starts.
 * The Bible text is the case this exists for — 4.4 MB, far too much to put in a configuration
 * file — but the installer knows nothing about Bibles.
 *
 * Two properties are asserted here because they are the two that fail badly: a copy that
 * cannot complete must not leave a half-written file where a daemon will read it, and a
 * missing asset must be reported rather than thrown, because a bot with no Bible is a bot
 * whose one command answers "the file cannot be read", not a bot the appliance cannot start.
 */
class BundledAssetsTest {

    private fun tempDir(): File =
        java.nio.file.Files.createTempDirectory("gonomadnet-assets").toFile().also { it.deleteOnExit() }

    /** An input stream that dies partway through, which is what a killed process looks like. */
    private class FailingStream(private val byte: Int, private val failsAfter: Int) : InputStream() {
        private var served = 0

        override fun read(): Int {
            if (served >= failsAfter) {
                throw IOException("the application went away")
            }
            served += 1
            return byte
        }
    }

    @Test
    fun anAssetIsWrittenOutWhole() {
        val dir = tempDir()
        val destination = File(dir, "kjv.txt")
        val installed = RealBundledAssets { ByteArrayInputStream("Ge1:1 In the beginning".toByteArray()) }

        assertTrue(installed.install("kjv.txt", destination.absolutePath))
        assertEquals("Ge1:1 In the beginning", destination.readText())
    }

    @Test
    fun anExistingCopyIsReplacedWithTheWholeAsset() {
        // An app update carries a new asset at the same path, and a copy left over from the
        // previous version must never win: the daemon reads whatever is at the path.
        val dir = tempDir()
        val destination = File(dir, "kjv.txt")
        destination.writeText("an older, shorter text")

        val installed = RealBundledAssets { ByteArrayInputStream("a whole new text".toByteArray()) }
        assertTrue(installed.install("kjv.txt", destination.absolutePath))
        assertEquals("a whole new text", destination.readText())
    }

    @Test
    fun anAssetThatIsNotInTheApkIsReportedRatherThanThrown() {
        val dir = tempDir()
        val installed = RealBundledAssets { null }
        assertFalse(installed.install("kjv.txt", File(dir, "kjv.txt").absolutePath))
    }

    @Test
    fun aCopyThatFailsLeavesNeitherAPartialFileNorScratchBehind() {
        val dir = tempDir()
        val destination = File(dir, "kjv.txt")
        val installed = RealBundledAssets { FailingStream('x'.code, failsAfter = 32) }

        assertFalse("an interrupted copy must report failure", installed.install("kjv.txt", destination.absolutePath))
        assertFalse(
            "a half-written file must never be left where a daemon will read it",
            destination.exists(),
        )
        assertEquals(
            "the staging file must not be left behind either",
            emptyList<String>(),
            dir.listFiles()?.map { it.name } ?: emptyList<String>(),
        )
    }

    @Test
    fun aMissingDestinationDirectoryIsCreated() {
        val dir = tempDir()
        val destination = File(dir, "gorrcbot/storage/kjv.txt")
        val installed = RealBundledAssets { ByteArrayInputStream("verse".toByteArray()) }

        assertTrue(installed.install("kjv.txt", destination.absolutePath))
        assertEquals("verse", destination.readText())
    }
}
