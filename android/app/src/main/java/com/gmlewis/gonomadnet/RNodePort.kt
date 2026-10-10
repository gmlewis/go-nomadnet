// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import java.io.IOException
import java.nio.file.Files
import java.nio.file.Path
import java.nio.file.Paths
import java.nio.file.StandardCopyOption

/**
 * The appliance's own serial port path: the symlink the transport is dialled at.
 *
 * A bridge's pseudo-terminal is allocated by the kernel, so its number is not the
 * appliance's to choose and does not survive the radio being unplugged: the next bridge is
 * given a different one, and a freed terminal can be handed to another process — the
 * console host holds one of its own. The transport is therefore never dialled at a
 * pseudo-terminal directly.
 *
 * It is dialled at this symlink instead, which the appliance owns and repoints at whatever
 * the current bridge published, and at [RNodeSpec.DEFAULT_PORT] while no bridge has
 * published anything. A radio that comes back repoints the symlink, and the transport's own
 * five-second reconnect opens it again and finds the new radio — nothing is restarted, so
 * the console does not drop out from under the operator when a radio is unplugged and
 * plugged back in.
 *
 * The swap is a `rename` of a fully-written second link over the first, which is atomic:
 * a transport opening the path resolves either the whole old target or the whole new one,
 * never a half-written link, and never a moment with no link at all.
 */
object RNodePortLink {

    /**
     * pointAt makes the link at [link] name [target], replacing whatever it named before.
     *
     * Returns true when the link was written, and false when it already named [target] —
     * which is the common case, and the reason an attach that changes nothing does not touch
     * the filesystem under a running transport.
     */
    fun pointAt(link: String, target: String): Boolean {
        val path = Paths.get(link)
        path.parent?.let { runCatching { Files.createDirectories(it) } }
        if (target(link) == target) {
            return false
        }
        val staged = path.resolveSibling("${path.fileName}.new")
        runCatching { Files.deleteIfExists(staged) }
        return try {
            Files.createSymbolicLink(staged, Paths.get(target))
            Files.move(staged, path, StandardCopyOption.ATOMIC_MOVE)
            true
        } catch (_: IOException) {
            // A filesystem that will not rename atomically — or a target no symlink may be
            // made to — still gets the link replaced rather than left naming the old radio.
            runCatching { Files.deleteIfExists(staged) }
            replace(path, target)
        }
    }

    /** target is what the link at [link] names, or null when there is no link there. */
    fun target(link: String): String? =
        runCatching { Files.readSymbolicLink(Paths.get(link)).toString() }.getOrNull()

    private fun replace(path: Path, target: String): Boolean = try {
        Files.deleteIfExists(path)
        Files.createSymbolicLink(path, Paths.get(target))
        true
    } catch (_: IOException) {
        false
    }
}

/**
 * What an attached USB device means to the appliance, as a decision rather than as a side
 * effect, so that the rule can be read and tested without a device or a service.
 */
enum class RadioAttach {
    /** The stack is down, so nothing dials a port and there is nothing to bridge for. */
    STACK_DOWN,

    /** A radio is already bridged, so the attach changes nothing. */
    ALREADY_UP,

    /** The radio that was bridged has gone, so the one that just arrived is bridged. */
    REBRIDGE,
}

/**
 * radioAttach decides what to do when the system hands the appliance an attached USB
 * device.
 *
 * The stack has to be up, because the port path means nothing to a transport that is not
 * running: bridging a radio for a stack that is down would hold the USB device for the life
 * of the install with nothing reading it. And a radio that is already bridged is left alone,
 * because a second bridge for the same port publishes a second path and leaves the first
 * one's terminal unread.
 */
fun radioAttach(stackUp: Boolean, radioUp: Boolean): RadioAttach = when {
    !stackUp -> RadioAttach.STACK_DOWN
    radioUp -> RadioAttach.ALREADY_UP
    else -> RadioAttach.REBRIDGE
}
