// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import android.os.ParcelFileDescriptor
import android.system.ErrnoException
import android.system.Os
import android.system.OsConstants
import java.io.Closeable
import java.io.File

/**
 * Creates the two named pipes the bot reads its sensors from, and holds them open.
 *
 * Opening the read end of a FIFO with `O_RDONLY` **blocks until some process opens
 * the write end**, and the bot does exactly that while it starts up. A bot spawned
 * before the write end exists therefore hangs during startup with no error message
 * at all. Opening both ends `O_RDWR` returns immediately and keeps the pipe alive
 * even when no writer holds it, which is why this happens before anything is spawned.
 *
 * The handles are held for the whole life of the service so the bot never sees an end
 * of file, because its reader gives up for good when it does.
 *
 * Nothing but a pipe may ever be at these two paths. A regular file is read from offset
 * zero — the whole history of every sample ever written — and a reader comes to rest on
 * the last line in it, which never changes again and is reported as if it were live. That
 * is not hypothetical: `files/run/compass.nmea` was found on the tablet as a regular file,
 * and the bot reported the same frozen heading across requests minutes apart.
 *
 * A pipe, once made, is a meeting point and not a thing anybody owns. Whoever holds the
 * handles takes them before spawning anything that writes, and gives them back without
 * touching the path: removing it would leave a writer that is already running holding an
 * inode nothing can reach, and leave the next reader holding an empty pipe with no writer
 * at all. Both halves of the appliance therefore do this — the sensor side before it
 * spawns the converters, the daemon side before it spawns the bot — and each of them may
 * find the other's pipe already there and use it.
 */
interface SensorFifoHolder : Closeable {
    /**
     * Creates and holds both pipes.
     *
     * Returns false when either path could not be made a pipe, which is a failure the
     * caller must not ignore: a daemon started against a regular file reports history as
     * though it were live.
     */
    fun open(): Boolean
}

/** What has to happen to one sensor path before it can be opened as a stream. */
enum class FifoPathAction {
    /** Nothing is there, so a pipe has to be made. */
    CREATE,

    /** A pipe is already there, and it may be used as it is. */
    USE_AS_IS,

    /** Something else is there, and it has to go: nothing but a pipe is a live stream. */
    REPLACE,
}

/**
 * Decides what to do with one sensor path.
 *
 * It is separated from the holder because it is the whole of the holder's judgement and
 * `android.system.Os` cannot be called from a test on the JVM.
 */
fun planFifoPath(exists: Boolean, isFifo: Boolean): FifoPathAction = when {
    !exists -> FifoPathAction.CREATE
    isFifo -> FifoPathAction.USE_AS_IS
    else -> FifoPathAction.REPLACE
}

/** The real holder, backed by `Os.mkfifo`. */
class RealSensorFifos(private val paths: StackPaths) : SensorFifoHolder {
    private val handles = mutableListOf<ParcelFileDescriptor>()

    override fun open(): Boolean {
        File(paths.runDir).mkdirs()
        var opened = 0
        for (path in listOf(paths.gpsFifo, paths.compassFifo)) {
            if (!ensureFifo(path)) {
                continue
            }
            val handle = ParcelFileDescriptor.open(
                File(path),
                ParcelFileDescriptor.MODE_READ_WRITE or ParcelFileDescriptor.MODE_CREATE,
            )
            handles.add(handle)
            opened += 1
        }
        return opened == 2
    }

    /**
     * Makes [path] a pipe, replacing whatever else is at it, and reports whether it is one.
     *
     * The answer is read back from the file system rather than assumed from the call that
     * was made, which is what covers the case of two instances of the service racing for the
     * same path: whoever loses the race finds the winner's pipe and uses it.
     */
    private fun ensureFifo(path: String): Boolean {
        when (planFifoPath(File(path).exists(), isFifo(path))) {
            FifoPathAction.USE_AS_IS -> return true
            FifoPathAction.CREATE -> runCatching { Os.mkfifo(path, FIFO_MODE) }
            FifoPathAction.REPLACE -> {
                File(path).delete()
                runCatching { Os.mkfifo(path, FIFO_MODE) }
            }
        }
        return isFifo(path)
    }

    /** Reports whether [path] is a pipe. A path that is not there is not a pipe. */
    private fun isFifo(path: String): Boolean = try {
        OsConstants.S_ISFIFO(Os.lstat(path).st_mode)
    } catch (missing: ErrnoException) {
        false
    }

    /**
     * Gives the handles back, and leaves the pipes where they are.
     *
     * The paths are deliberately not removed. A converter spawned against one of them may
     * still be writing, and a bot spawned against one later must find the same pipe rather
     * than a path that has to be made again — and making it again is what orphaned the
     * writers that were found on the tablet holding `gps.nmea (deleted)`. A pipe that
     * outlives its readers costs an inode; a pipe that is replaced costs the appliance its
     * sensors.
     */
    override fun close() {
        for (handle in handles) {
            runCatching { handle.close() }
        }
        handles.clear()
    }

    companion object {
        /** Owner read and write only. Kotlin has no octal literal, so this is decimal. */
        private const val FIFO_MODE = 384 // 0o600
    }
}
