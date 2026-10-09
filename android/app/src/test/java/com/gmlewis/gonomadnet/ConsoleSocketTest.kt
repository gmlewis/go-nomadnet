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
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The console socket's name and its peer rule.
 *
 * The name is a pure function of the app's pid and the session's entropy, so the app and
 * the Go host derive the same name without exchanging it; the peer rule is what stops any
 * other app on the tablet from writing into the appliance's terminal.
 */
class ConsoleSocketTest {

    @Test
    fun `the console socket name is unpredictable`() {
        // An abstract socket has no filesystem entry and no permissions: any process that
        // knows the name can connect. Two sessions must therefore never agree on one.
        val first = ConsoleSocket.randomEntropy()
        val second = ConsoleSocket.randomEntropy()
        assertEquals(ConsoleSocket.ENTROPY_BYTES, first.size)
        assertFalse("two entropy draws were identical", first.contentEquals(second))
        assertNotEquals(ConsoleSocket.name(4242, first), ConsoleSocket.name(4242, second))
        assertNotEquals(
            "one pid's name collided with another's",
            ConsoleSocket.name(1, first),
            ConsoleSocket.name(2, first),
        )
    }

    @Test
    fun `the console socket name is a single abstract name`() {
        val entropy = ByteArray(ConsoleSocket.ENTROPY_BYTES) { it.toByte() }
        val name = ConsoleSocket.name(4242, entropy)

        for (forbidden in listOf('\u0000', '/', ' ', '\t', '\n')) {
            assertFalse("the name $name carries ${forbidden.code}", name.contains(forbidden))
        }
        assertTrue("the name must be recognisable: $name", name.startsWith("gonomadnet-console-"))
        assertTrue("the name must carry the pid: $name", name.contains("4242"))

        // The suffix is the entropy in hex and nothing else, so both halves derive it.
        val suffix = name.substringAfterLast('-')
        assertEquals("the suffix is not the entropy in hex: $suffix", ConsoleSocket.ENTROPY_BYTES * 2, suffix.length)
        assertEquals(entropy.toList(), suffix.chunked(2).map { it.toInt(16).toByte() })

        // An abstract name lives in sun_path, which is 108 bytes: a longer name is silently
        // truncated by the kernel, and two sessions could then collide on the truncated form.
        assertTrue("the name is ${name.length} bytes, too long for sun_path", name.length < 108)
    }

    @Test
    fun `a peer from this app is accepted`() {
        for (uid in listOf(0, 10123, 65534)) {
            assertTrue("uid $uid is the app's own", ConsoleSocket.peerIsAccepted(uid, uid))
        }
    }

    @Test
    fun `a peer from another app is refused with both uids in the reason`() {
        assertFalse(ConsoleSocket.peerIsAccepted(10123, 10124))
        val reason = ConsoleSocket.refusalReason(10123, 10124)
        assertTrue("the refusal must name the peer: $reason", reason.contains("10124"))
        assertTrue("the refusal must name the app: $reason", reason.contains("10123"))
    }
}
