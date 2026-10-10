// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The radio bridge's socket name is the one thing the two halves build in two languages, so
 * it is pinned here as a literal — the same literal `nomadnet/rnode`'s `SocketNamePrefix`
 * is pinned to in Go. A name the two disagree about is an app dialling a socket nobody is
 * listening on: the radio never reaches the transport, the bridge waits for a connection
 * that never comes, and nothing anywhere says why.
 */
class RNodeSocketTest {

    @Test
    fun `the prefix is the one the Go half builds the same name from`() {
        assertEquals("gonomadnet-rnode-", RNodeSocket.PREFIX)
        assertTrue(
            "the name is not built from the prefix: ${RNodeSocket.name(1, ByteArray(16))}",
            RNodeSocket.name(1, ByteArray(16)).startsWith(RNodeSocket.PREFIX),
        )
    }

    @Test
    fun `a socket name is hexadecimal and has no separator an abstract name cannot carry`() {
        val name = RNodeSocket.name(4242, RNodeSocket.randomEntropy())
        assertTrue("the name carries $name", name.removePrefix(RNodeSocket.PREFIX).matches(Regex("[0-9a-f]+-[0-9a-f]+")))
        assertEquals("/ and NUL end an abstract name early", -1, name.indexOfAny(charArrayOf('/', '\u0000')))
    }

    @Test
    fun `two sessions never agree on a name`() {
        // An abstract socket is reachable by any process that knows its name, and this one
        // carries the bytes of a transmitter.
        val first = RNodeSocket.randomEntropy()
        val second = RNodeSocket.randomEntropy()
        assertNotEquals(
            "two sessions drew the same entropy",
            first.joinToString(),
            second.joinToString(),
        )
        assertNotEquals(RNodeSocket.name(4242, first), RNodeSocket.name(4242, second))
        assertNotEquals(RNodeSocket.name(1, first), RNodeSocket.name(2, first))
    }

    @Test
    fun `the entropy is the length the Go half reads`() {
        assertEquals(16, RNodeSocket.ENTROPY_BYTES)
        assertEquals(16, RNodeSocket.randomEntropy().size)
    }
}
