// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The transport's configuration, built from what the operator asked for.
 *
 * The appliance owns every interface on this tablet, so if this produces a transport with no
 * interface the tablet is isolated from the network — silently, because an isolated node
 * looks exactly like a working one. Hence the assertions on the failure description as well
 * as on the happy path.
 */
class ApplianceConfigTest {

    @Test
    fun aHubBecomesOneTcpClientInterfaceWithALiteralAddress() {
        val spec = ApplianceConfig.nodeConfig(
            hubSpec = "go-nomadnet.duckdns.org:4242",
            resolve = { _, _ -> DialableAddress("2001:db8::1", isIPv6 = true, probed = true) },
        )
        assertEquals(1, spec.interfaces.size)
        val iface = spec.interfaces.first()
        assertEquals(ApplianceConfig.HUB_INTERFACE_NAME, iface.name)
        assertEquals("2001:db8::1", iface.host)
        assertEquals(4242, iface.port)
        assertTrue(iface.enabled)

        val rendered = NodeConfigRenderer.reticulumConfig(spec)
        assertTrue(rendered.contains("target_host = 2001:db8::1"))
        assertTrue(rendered.contains("target_port = 4242"))
        assertTrue(rendered.contains("type = TCPClientInterface"))
    }

    @Test
    fun aHubThatCannotBeParsedLeavesTheTransportIsolatedRatherThanBroken() {
        // An appliance whose hub address is a typo must still start: the transport comes up
        // with no interface, the node is simply isolated, and the operator is told why.
        for (bad in listOf("not-a-host-port", "", "example.com:0", "2001:db8::1:4242")) {
            val spec = ApplianceConfig.nodeConfig(hubSpec = bad)
            assertTrue("$bad must leave the transport with no interface", spec.interfaces.isEmpty())
            assertNotNull("$bad must be explained", ApplianceConfig.describeFailure(bad))
        }
        assertNull("a well-formed address is not a failure", ApplianceConfig.describeFailure("example.com:4242"))
    }

    @Test
    fun aHubThatDoesNotResolveLeavesTheTransportIsolated() {
        val spec = ApplianceConfig.nodeConfig(
            hubSpec = "this-name-does-not-exist.invalid:4242",
            resolve = { _, _ -> null },
        )
        assertTrue(spec.interfaces.isEmpty())
        val rendered = NodeConfigRenderer.reticulumConfig(spec)
        assertTrue("the transport must still be rendered", rendered.contains("[interfaces]"))
        assertTrue(rendered.contains("Deliberately empty"))
    }

    @Test
    fun theSharedInstancePinsSurviveIntoTheRenderedConfiguration() {
        // The contract with Termux's attached mode lives in the same rendered file, so it
        // gets asserted here rather than only through a constant.
        val spec = ApplianceConfig.nodeConfig(
            hubSpec = "example.com:4242",
            resolve = { _, _ -> DialableAddress("203.0.113.7", isIPv6 = false, probed = true) },
        )
        val rendered = NodeConfigRenderer.reticulumConfig(spec)
        assertTrue(rendered.contains("shared_instance_type = tcp"))
        assertTrue(rendered.contains("shared_instance_port = 37428"))
        assertTrue(rendered.contains("instance_name = default"))
    }

    @Test
    fun theChoiceIsDescribedHonestlyIncludingWhenItWasBlind() {
        val proved = ApplianceConfig.describeChoice(
            "example.com:4242",
            DialableAddress("2001:db8::1", isIPv6 = true, probed = true),
            4242,
        )
        assertTrue("a proved choice must say so: $proved", proved.contains("it answered"))
        assertTrue(proved.contains("IPv6"))

        val blind = ApplianceConfig.describeChoice(
            "example.com:4242",
            DialableAddress("203.0.113.7", isIPv6 = false, probed = false),
            4242,
        )
        assertTrue("a blind choice must say so: $blind", blind.contains("did not answer"))
        assertTrue(blind.contains("using it anyway"))

        val none = ApplianceConfig.describeChoice("bad", null, 4242)
        assertTrue(none.contains("could not resolve"))
    }

    @Test
    fun theDefaultHubIsTheOneThisProjectRunsAgainst() {
        assertEquals("go-nomadnet.duckdns.org:4242", ApplianceSettings.DEFAULT_HUB_SPEC)
        assertNotNull(Resolver.parseHostPort(ApplianceSettings.DEFAULT_HUB_SPEC))
    }
}
