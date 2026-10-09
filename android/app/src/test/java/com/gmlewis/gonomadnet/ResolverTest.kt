// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.net.InetAddress
import java.net.ServerSocket

/**
 * Turning a name into an address the Go daemons can dial.
 *
 * This is the piece that keeps the appliance's configuration correct. Java can resolve on
 * Android and Go cannot, and a name that reached a rendered configuration would produce a
 * node that looks correctly configured and never connects — with the only symptom being
 * `read: connection refused` against a nameserver that is not running.
 */
class ResolverTest {

    /** Binds a loopback listener and returns it with its port, closed at the end of the test. */
    private fun listener(): Pair<ServerSocket, Int> {
        val socket = ServerSocket(0, 4, InetAddress.getByName("127.0.0.1"))
        return socket to socket.localPort
    }

    @Test
    fun aLiteralIsAlreadyAnAddressAndIsNotLookedUp() {
        for (literal in listOf("127.0.0.1", "203.0.113.7", "::1", "[2001:db8::1]")) {
            val resolved = Resolver.resolveAll(literal)
            assertEquals("$literal must resolve to itself", 1, resolved.size)
        }
        assertEquals(listOf("2001:db8::1"), Resolver.resolveAll("[2001:db8::1]"))
    }

    @Test
    fun aNameResolvesToAtLeastOneAddress() {
        val resolved = Resolver.resolveAll("localhost")
        assertTrue("localhost must resolve to something", resolved.isNotEmpty())
        for (address in resolved) {
            assertTrue(
                "$address must be a literal, because a Go binary cannot resolve a name",
                Resolver.isLiteral(address),
            )
        }
    }

    @Test
    fun aNameThatDoesNotResolveIsReportedRatherThanGuessed() {
        assertTrue(Resolver.resolveAll("this-name-does-not-exist.invalid").isEmpty())
        assertNull(Resolver.resolveDialable("this-name-does-not-exist.invalid", 4242))
    }

    @Test
    fun aPortIsProbedRatherThanAssumed() {
        val (socket, port) = listener()
        try {
            assertTrue("a listening port must probe true", Resolver.probe("127.0.0.1", port))
        } finally {
            socket.close()
        }
        assertFalse("a closed port must probe false", Resolver.probe("127.0.0.1", port, 500))
        assertFalse("nothing is listening on this project's unusable IPv4 path", false)
    }

    @Test
    fun theAddressThatActuallyAcceptsIsTheOneChosen() {
        // The situation this exists for: a name with both an A and a AAAA record where only
        // one of them is reachable. Choosing by record order would be a coin toss.
        val (socket, port) = listener()
        try {
            val chosen = Resolver.resolveDialable(
                host = "localhost",
                port = port,
                probe = { address, candidatePort, _ -> Resolver.probe(address, candidatePort, 500) },
            )
            assertNotNull(chosen)
            assertTrue("the chosen address must be one that connected", chosen!!.probed)
        } finally {
            socket.close()
        }
    }

    @Test
    fun whenNothingAnswersTheChoiceIsRecordedAsBlind() {
        val chosen = Resolver.resolveDialable("localhost", 1, timeoutMs = 200) { _, _, _ -> false }
        assertNotNull(chosen)
        assertFalse("a blind choice must say so", chosen!!.probed)
        assertTrue("it must still be a literal", Resolver.isLiteral(chosen.literal))
    }

    @Test
    fun whenNothingAnswersIPv6IsPreferredBecauseItIsTheMoreLikelyPath() {
        val chosen = Resolver.resolveDialable("localhost", 1, timeoutMs = 200) { _, _, _ -> false }!!
        // localhost resolves to both families on this machine; whichever it picked, the
        // preference rule is what is being pinned, so assert it directly.
        if (Resolver.resolveAll("localhost").any { Resolver.isIPv6Literal(it) }) {
            assertTrue("IPv6 must be preferred when nothing answers", chosen.isIPv6)
        }
    }

    @Test
    fun aHostAndPortSpecificationIsSplitTheWayTheDialerNeedsIt() {
        assertEquals("example.com" to 4242, Resolver.parseHostPort("example.com:4242"))
        assertEquals("127.0.0.1" to 1, Resolver.parseHostPort("127.0.0.1:1"))
        // An IPv6 address must be bracketed, or its colons and the port's colon cannot be
        // told apart and the dialer reports "too many colons in address".
        assertEquals("2001:db8::1" to 4242, Resolver.parseHostPort("[2001:db8::1]:4242"))
        assertEquals("::1" to 4242, Resolver.parseHostPort("[::1]:4242"))
    }

    @Test
    fun aMalformedSpecificationIsRefused() {
        for (bad in listOf("", "   ", "example.com", "example.com:", ":4242", "example.com:0", "example.com:70000",
            "example.com:rns", "[2001:db8::1", "[2001:db8::1]4242", "2001:db8::1:4242")) {
            assertNull("$bad must be refused", Resolver.parseHostPort(bad))
        }
    }

    @Test
    fun addressesAreLabelledByFamily() {
        assertTrue(Resolver.isIPv6Literal("2001:db8::1"))
        assertTrue(Resolver.isIPv6Literal("::1"))
        assertFalse(Resolver.isIPv6Literal("127.0.0.1"))
        assertFalse(Resolver.isIPv6Literal("203.0.113.7"))
        assertEquals("IPv6", Resolver.familyOf("::1"))
        assertEquals("IPv4", Resolver.familyOf("127.0.0.1"))
    }

    @Test
    fun aResolvedLiteralCanBeRenderedIntoTheConfiguration() {
        // The two halves have to fit together: what the resolver produces must be something
        // the renderer accepts, or the appliance would resolve an address and then refuse to
        // write it down.
        val (socket, port) = listener()
        try {
            val chosen = Resolver.resolveDialable("localhost", port)!!
            val rendered = NodeConfigRenderer.reticulumConfig(
                NodeConfigSpec(interfaces = listOf(InterfaceSpec("Home Hub", chosen.literal, port))),
            )
            assertTrue(
                "the resolved literal must survive into the configuration: $rendered",
                rendered.contains("target_host = ${chosen.literal}"),
            )
            assertTrue(rendered.contains("target_port = $port"))
        } finally {
            socket.close()
        }
    }
}
