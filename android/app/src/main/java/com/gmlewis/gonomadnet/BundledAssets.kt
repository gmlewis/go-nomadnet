// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import java.io.File
import java.io.IOException
import java.io.InputStream

/**
 * The King James Version text the bot's `kjv` command reads.
 *
 * It is carried in the APK's assets because it is 4.4 MB of verse text, which is far too
 * much to write into a configuration file, and because a bundled appliance should answer a
 * verse lookup with no setup at all. It is the same file the reference install uses, one
 * verse per line:
 *
 *	Ge1:1 In the beginning God created the heaven and the earth.
 */
const val KJV_ASSET_NAME = "kjv.txt"

/**
 * Writes out a read-only file the APK carries, so that a daemon can read it.
 *
 * An asset lives inside the APK, which is a zip archive; no process other than the
 * application itself can open one, and the bundled Go binaries are other processes. A
 * bundled data file therefore has to be copied to a real path inside the app's private
 * directory before the daemon that reads it is started.
 */
fun interface BundledAssets {
    /**
     * Writes the asset named [name] to [destination].
     *
     * Returns false when the asset is not in the APK or the copy could not be completed. In
     * that case [destination] is left exactly as it was: a half-written file at a path a
     * daemon reads is worse than no file at all.
     */
    fun install(name: String, destination: String): Boolean
}

/**
 * The real installer, reading the APK's assets through [open].
 *
 * [open] is a function rather than an `AssetManager` so that every rule below can be
 * asserted on the JVM, where there is no APK to read.
 *
 * The copy is written to [STAGING_SUFFIX] beside the destination and renamed into place,
 * which is the only way to make the destination atomic on Android: a service that is killed
 * mid-copy must not leave a truncated text where the next run will read it, and the previous
 * copy is the better answer until the new one is complete.
 */
class RealBundledAssets(private val open: (String) -> InputStream?) : BundledAssets {

    override fun install(name: String, destination: String): Boolean {
        val source = open(name) ?: return false
        val target = File(destination)
        val staging = File(destination + STAGING_SUFFIX)
        return try {
            source.use { input ->
                staging.parentFile?.mkdirs()
                staging.outputStream().use { output -> input.copyTo(output) }
            }
            if (staging.renameTo(target)) {
                true
            } else {
                staging.delete()
                false
            }
        } catch (failed: IOException) {
            staging.delete()
            false
        }
    }

    companion object {
        /** The suffix of the file a copy is assembled in before it is renamed into place. */
        const val STAGING_SUFFIX = ".new"
    }
}
