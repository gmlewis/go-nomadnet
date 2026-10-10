// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File
import java.nio.file.Files

/**
 * The appliance's port path is the whole of whether a radio can come back by itself: the
 * transport is dialled at it once and never told again, so what it names at any moment is
 * what the transport dials on its next reconnect.
 */
class RNodePortTest {

    @Test
    fun `the link names the target, and reading it back agrees`() {
        val link = scratch("port")

        assertTrue(RNodePortLink.pointAt(link.absolutePath, "/dev/pts/7"))

        assertEquals("/dev/pts/7", RNodePortLink.target(link.absolutePath))
    }

    @Test
    fun `a radio that comes back repoints the link`() {
        // The bridge that had the radio is gone and the kernel has handed the new one a
        // different terminal. Nothing but the link's target changes.
        val link = scratch("port")
        RNodePortLink.pointAt(link.absolutePath, "/dev/pts/7")

        assertTrue(RNodePortLink.pointAt(link.absolutePath, "/dev/pts/9"))

        assertEquals("/dev/pts/9", RNodePortLink.target(link.absolutePath))
    }

    @Test
    fun `a target that is already named is left alone`() {
        // Repointing the link under a running transport is a filesystem change it may be
        // opening through, so an attach that changes nothing must not touch it.
        val link = scratch("port")
        RNodePortLink.pointAt(link.absolutePath, RNodeSpec.DEFAULT_PORT)

        assertFalse(RNodePortLink.pointAt(link.absolutePath, RNodeSpec.DEFAULT_PORT))
    }

    @Test
    fun `the link may name a device that is not there`() {
        // That is the steady state: the appliance always assumes a radio might be available,
        // and a target that cannot be opened is a radio that is not there yet.
        val link = scratch("port")

        assertTrue(RNodePortLink.pointAt(link.absolutePath, RNodeSpec.DEFAULT_PORT))

        assertEquals(RNodeSpec.DEFAULT_PORT, RNodePortLink.target(link.absolutePath))
        assertFalse("a symlink to a missing device was resolved as one", link.exists())
    }

    @Test
    fun `the link replaces one a previous run left, and a stale regular file`() {
        // A killed bridge, or a build that wrote a plain file here, leaves something that
        // names the wrong port; the link has to take its place rather than fail on it.
        val link = scratch("port")
        link.writeText("/dev/pts/3")
        assertNull("a regular file was read as a symlink", RNodePortLink.target(link.absolutePath))

        assertTrue(RNodePortLink.pointAt(link.absolutePath, "/dev/pts/11"))

        assertEquals("/dev/pts/11", RNodePortLink.target(link.absolutePath))
        assertTrue(Files.isSymbolicLink(link.toPath()))
    }

    @Test
    fun `the parent directory is created for the link`() {
        val link = File(scratch("rnodes"), "port")

        assertTrue(RNodePortLink.pointAt(link.absolutePath, "/dev/pts/7"))

        assertEquals("/dev/pts/7", RNodePortLink.target(link.absolutePath))
    }

    @Test
    fun `an attach is bridged only while the stack is up and no radio is`() {
        assertEquals(RadioAttach.STACK_DOWN, radioAttach(stackUp = false, radioUp = false))
        assertEquals(RadioAttach.STACK_DOWN, radioAttach(stackUp = false, radioUp = true))
        assertEquals(RadioAttach.ALREADY_UP, radioAttach(stackUp = true, radioUp = true))
        assertEquals(RadioAttach.REBRIDGE, radioAttach(stackUp = true, radioUp = false))
    }

    /** scratch is a fresh path inside a directory that is cleaned up when the test ends. */
    private fun scratch(name: String): File =
        Files.createTempDirectory("gonomadnet-rnodeport").toFile().also { it.deleteOnExit() }
            .let { File(it, name) }
}
