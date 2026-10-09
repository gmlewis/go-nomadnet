// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The transport's configuration is the contract between the appliance and anything
 * that attaches to it, so every value that both sides have to agree on is asserted
 * here rather than left to a default.
 */
class NodeConfigRendererTest {

    @Test
    fun theSharedInstanceIsATcpSocketWithThePinnedPortAndName() {
        val text = NodeConfigRenderer.reticulumConfig(NodeConfigSpec())
        assertTrue("the shared instance must not be an abstract Unix socket", text.contains("shared_instance_type = tcp"))
        assertTrue(text.contains("share_instance = yes"))
        assertTrue("the port is pinned", text.contains("shared_instance_port = 37428"))
        assertTrue("the name is pinned", text.contains("instance_name = default"))
    }

    @Test
    fun thePinnedValuesAreTheDocumentedOnes() {
        assertEquals(37428, NodeConfigSpec.DEFAULT_SHARED_INSTANCE_PORT)
        assertEquals("default", NodeConfigSpec.DEFAULT_INSTANCE_NAME)
    }

    @Test
    fun anInterfaceIsRenderedAsATcpClientWithALiteralAddress() {
        val text = NodeConfigRenderer.reticulumConfig(
            NodeConfigSpec(
                interfaces = listOf(InterfaceSpec("Home Hub", "203.0.113.7", 4242)),
            ),
        )
        assertTrue(text.contains("[[Home Hub]]"))
        assertTrue(text.contains("type = TCPClientInterface"))
        assertTrue(text.contains("target_host = 203.0.113.7"))
        assertTrue(text.contains("target_port = 4242"))
    }

    @Test
    fun aHostNameIsRefusedRatherThanWritten() {
        // Go cannot resolve names on Android: /etc/resolv.conf does not exist, and a
        // CGO_ENABLED=0 binary falls back to a nameserver that is not running. A
        // configuration that carried a name would look perfectly fine and never
        // connect, so the renderer refuses it outright.
        val failure = assertThrows(IllegalArgumentException::class.java) {
            NodeConfigRenderer.reticulumConfig(
                NodeConfigSpec(interfaces = listOf(InterfaceSpec("Home Hub", "go-nomadnet.duckdns.org", 4242))),
            )
        }
        assertTrue(
            "the refusal must name the host and say why: ${failure.message}",
            failure.message!!.contains("go-nomadnet.duckdns.org"),
        )
        assertTrue(failure.message!!.contains("cannot resolve names"))
    }

    @Test
    fun noHostNameEverSurvivesIntoTheRenderedConfiguration() {
        val rendered = NodeConfigRenderer.reticulumConfig(
            NodeConfigSpec(
                interfaces = listOf(
                    InterfaceSpec("Hub", "203.0.113.7", 4242),
                    InterfaceSpec("IPv6 Hub", "[2001:db8::1]", 4242),
                    InterfaceSpec("Loopback", "127.0.0.1", 37428),
                ),
            ),
        )
        assertFalse(rendered.contains("duckdns.org"))
        assertTrue(rendered.contains("target_host = 203.0.113.7"))
        // An IPv6 literal is written without its brackets, which is what the dialer wants,
        // and without quotes, because Reticulum's parser keeps them.
        assertTrue(rendered.contains("target_host = 2001:db8::1"))
    }

    @Test
    fun nothingInTheReticulumConfigIsQuoted() {
        // Reticulum's parser keeps quotation marks as part of a value, so a quoted address is
        // dialled as a name containing quote characters and every connection fails with
        //     lookup "203.0.113.7": no such host
        // The same applies to an interface's section header. This is asserted on the whole
        // document rather than key by key, because the rule is about the format, not about
        // any one key: if a quote ever appears here again, one of the values is being
        // written for a parser that does not read this file.
        val rendered = NodeConfigRenderer.reticulumConfig(
            NodeConfigSpec(
                interfaces = listOf(
                    InterfaceSpec("Home Hub", "203.0.113.7", 4242),
                    InterfaceSpec("IPv6 Hub", "2001:db8::1", 4242),
                ),
            ),
        )
        assertFalse(
            "the transport's configuration must contain no quotation marks at all:\n$rendered",
            rendered.contains('"'),
        )
    }

    @Test
    fun theClientConfigIsUnquotedForTheSameReason() {
        val client = NodeConfigRenderer.clientReticulumConfig(NodeConfigSpec())
        assertFalse("the client configuration must contain no quotation marks:\n$client", client.contains('"'))
    }

    @Test
    fun theClientListsTheInterfacesTheTransportDials() {
        // The client's Interfaces page reads this file and shows what it finds there, so a
        // client configuration that listed nothing showed the operator an appliance with no
        // interfaces at all while its transport was dialling the hub. The two sections are
        // the same list because they are the same interfaces.
        val spec = NodeConfigSpec(
            interfaces = listOf(InterfaceSpec("Home Hub", "203.0.113.7", 4242)),
        )
        val transport = NodeConfigRenderer.reticulumConfig(spec)
        val client = NodeConfigRenderer.clientReticulumConfig(spec)

        assertTrue("the client does not list the hub:\n$client", client.contains("[[Home Hub]]"))
        assertTrue(client.contains("target_host = 203.0.113.7"))
        assertTrue(client.contains("target_port = 4242"))
        assertEquals(
            "the two configuration files disagree about what this appliance connects through",
            transport.substringAfter("[interfaces]\n"),
            client.substringAfter("[interfaces]\n"),
        )

        // Listing an interface is not owning it: the client must still refuse to become the
        // shared instance, which is the only thing that would make it dial on its own.
        assertTrue(client.contains("require_shared_instance = yes"))
        assertTrue(client.contains("enable_transport = no"))
    }

    @Test
    fun theClientStillStatesAnEmptyInterfaceListRatherThanLeavingItBlank() {
        val client = NodeConfigRenderer.clientReticulumConfig(NodeConfigSpec())
        assertTrue(client.contains("[interfaces]"))
        assertTrue(
            "an empty list is a deliberate configuration and should say so:\n$client",
            client.contains("Deliberately empty"),
        )
    }

    @Test
    fun literalAddressAcceptsAddressesAndRefusesNames() {
        for (address in listOf("127.0.0.1", "203.0.113.7", "0.0.0.0", "[2001:db8::1]", "::1")) {
            assertTrue("$address is a literal", NodeConfigRenderer.literalAddress(address) != null)
        }
        for (name in listOf("", "  ", "example.com", "host", "1.2.3", "1.2.3.4.5", "999.1.1.1", "1.2.3.a")) {
            assertEquals("$name is a name", null, NodeConfigRenderer.literalAddress(name))
        }
    }

    @Test
    fun anEmptyInterfaceListIsStatedRatherThanLeftBlank() {
        val text = NodeConfigRenderer.reticulumConfig(NodeConfigSpec())
        assertTrue(text.contains("[interfaces]"))
        assertTrue(
            "an empty transport is a deliberate configuration and should say so",
            text.contains("Deliberately empty"),
        )
    }

    @Test
    fun theBotReadsTheLiveFeedFromTheTwoFifos() {
        val paths = StackPaths("/data/user/0/com.gmlewis.gonomadnet/files")
        val text = NodeConfigRenderer.gorrcbotConfig(
            BotConfigSpec(
                gpsPort = paths.gpsFifo,
                compassPort = paths.compassFifo,
                hubDestination = "a012129c10205c0b9441fcd2b755b2a7",
            ),
        )
        assertTrue(text.contains("gps_port = \"/data/user/0/com.gmlewis.gonomadnet/files/run/gps.nmea\""))
        assertTrue(text.contains("compass_port = \"/data/user/0/com.gmlewis.gonomadnet/files/run/compass.nmea\""))
        assertTrue(text.contains("[[hubs]]"))
        assertTrue(text.contains("destination = \"a012129c10205c0b9441fcd2b755b2a7\""))
        assertTrue(text.contains("rooms = [\"general\"]"))
        assertTrue(text.contains("respond_to = { \"general\" = \"gobot\" }"))
    }

    @Test
    fun theBotIsToldWhereTheBundledBibleTextIs() {
        // The text is installed from the APK's assets, so the path is the appliance's own
        // and not one an operator typed. Without the key the bot's one Bible command answers
        // "kjv is not configured" for the whole life of the install.
        val paths = StackPaths("/data/user/0/com.gmlewis.gonomadnet/files")
        val text = NodeConfigRenderer.gorrcbotConfig(
            BotConfigSpec(
                gpsPort = paths.gpsFifo,
                compassPort = paths.compassFifo,
                kjvTxtFile = paths.kjvTxtFile,
                hubDestination = "a012129c10205c0b9441fcd2b755b2a7",
            ),
        )
        assertTrue(
            "the bot must be told where the Bible text is:\n$text",
            text.contains("kjv_txt_file = \"${paths.kjvTxtFile}\""),
        )
    }

    @Test
    fun aBotWithNoBibleTextIsNotToldToReadOne() {
        // The key's own default is the empty string, which disables the command and makes it
        // say so. Writing an empty path would name a file that cannot exist.
        val text = NodeConfigRenderer.gorrcbotConfig(
            BotConfigSpec(gpsPort = "/gps", compassPort = "/compass", hubDestination = "ab"),
        )
        assertFalse("an unset Bible text must not be written at all:\n$text", text.contains("kjv_txt_file"))
    }

    @Test
    fun everyPathTheApplianceUsesIsUnderItsOwnPrivateDirectory() {
        val paths = StackPaths("/data/user/0/com.gmlewis.gonomadnet/files")
        for (path in listOf(
            paths.runDir, paths.logDir, paths.configDir, paths.gpsFifo,
            paths.compassFifo, paths.gornsdConfig, paths.gorrcbotConfig,
        )) {
            assertTrue("$path must live under filesDir", path.startsWith("/data/user/0/com.gmlewis.gonomadnet/files"))
        }
        assertTrue(
            "nothing may be written into Termux's home or onto /sdcard",
            !paths.gpsFifo.startsWith("/data/data/com.termux") && !paths.gpsFifo.startsWith("/sdcard"),
        )
    }
}
