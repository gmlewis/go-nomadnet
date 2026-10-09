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

/**
 * Every daemon is launched by absolute path with an explicit configuration and a home
 * inside the app's private directory. Android's seccomp policy kills a process that
 * resolves a bare program name, so this is a crash-avoidance test, not a style test.
 */
class LaunchSpecTest {

    private val nativeDir = "/data/app/~~abc==/com.gmlewis.gonomadnet-xyz==/lib/arm64"
    private val paths = StackPaths("/data/user/0/com.gmlewis.gonomadnet/files")

    private fun specs(): List<LaunchSpec> = listOf(
        LaunchSpecs.gornsd(nativeDir, paths),
        LaunchSpecs.gorrcd(nativeDir, paths),
        LaunchSpecs.gorrcbot(nativeDir, paths),
    )

    @Test
    fun everyBinaryIsAnAbsolutePathInsideNativeLibraryDir() {
        for (spec in specs()) {
            assertTrue("${spec.name} must be launched by absolute path", spec.binary.startsWith("/"))
            assertTrue("${spec.name} must come from nativeLibraryDir", spec.binary.startsWith("$nativeDir/"))
            assertTrue("${spec.name} keeps the .so name the packager requires", spec.binary.endsWith(".so"))
        }
    }

    @Test
    fun noBareProgramNameAppearsAnywhere() {
        // The program name is the only thing that may be a bare word, and it is the absolute
        // path asserted above. Every other element is either a flag or a value, and the one
        // thing none of them may be is the name of a bundled program: a bare name is what
        // Android's seccomp policy kills the process for resolving.
        //
        // The pairing between flags and values is deliberately NOT asserted, because not
        // every flag takes a value — --include-joined-member-list does not — and a test that
        // assumed otherwise would have to be edited every time a boolean flag is added.
        val bundled = listOf("gornsd", "gorrcd", "gorrcbot", "gonsensor", "libgornsd.so", "libgorrcbot.so")
        for (spec in specs()) {
            for (argument in spec.argv) {
                assertFalse(
                    "${spec.name} argv carries $argument, which is a program name; " +
                        "Android kills a process that resolves one, so this must be an absolute path",
                    argument in bundled,
                )
            }
            assertTrue(
                "${spec.name} must be launched with at least one argument",
                spec.argv.isNotEmpty(),
            )
        }
        assertEquals(
            listOf("gornsd", "gorrcd", "gorrcbot"),
            specs().map { it.name },
        )
    }

    @Test
    fun everyArgumentValueIsAnAbsolutePathOrAPlainValue() {
        // Values are a period in seconds, a port, or a path inside the app's private
        // directory. None of them may be a program name, and none may point outside the
        // directory the appliance owns.
        for (spec in specs()) {
            var index = 0
            while (index + 1 < spec.argv.size) {
                val flag = spec.argv[index]
                val value = spec.argv[index + 1]
                if (value.contains('/')) {
                    assertTrue(
                        "${spec.name} $flag points outside the app's private directory: $value",
                        value.startsWith(paths.filesDir),
                    )
                } else {
                    assertTrue(
                        "${spec.name} $flag has the bare value \"$value\", which must be a plain setting",
                        value.isNotEmpty() && !value.endsWith(".so"),
                    )
                }
                index += 2
            }
        }
    }

    @Test
    fun everyDaemonGetsAHomeInsideTheAppsPrivateDirectory() {
        for (spec in specs()) {
            val home = spec.env["HOME"]
            assertTrue("${spec.name} must have a private HOME", home != null && home.startsWith(paths.filesDir))
        }
        assertEquals(paths.gorrcdHome, LaunchSpecs.gorrcd(nativeDir, paths).env["RRCD_HOME"])
        assertEquals(paths.gorrcbotHome, LaunchSpecs.gorrcbot(nativeDir, paths).env["GORRCBOT_HOME"])
    }

    @Test
    fun everyDaemonIsToldItsConfigurationExplicitly() {
        for (spec in specs()) {
            val configIndex = spec.argv.indexOf("--config")
            assertTrue("${spec.name} must be told its configuration", configIndex >= 0)
            val configPath = spec.argv[configIndex + 1]
            assertTrue("${spec.name}'s configuration must be a path", configPath.startsWith("/"))
        }
    }

    @Test
    fun everyDaemonLogsIntoTheAppsOwnLogDirectory() {
        for (spec in specs()) {
            assertTrue("${spec.name} log must live under filesDir", spec.logFile.startsWith(paths.logDir))
        }
    }

    @Test
    fun theHubPublishesItsDestinationBesideItsOwnIdentity() {
        // The hub writes <RRCD_HOME>/hub_identity.rrc.hub, and the supervisor has to look
        // there and nowhere else: a hub whose destination the supervisor cannot find is a
        // hub no local bot can attach to, and "the bot never started" is a hard thing to
        // diagnose from a screen.
        assertEquals("${paths.gorrcdHome}/hub_identity.rrc.hub", paths.hubDestinationFile)
        assertTrue("the published destination lives beside the hub's identity", paths.hubDestinationFile.startsWith(paths.gorrcdHome))
        assertEquals(paths.gorrcdHome, LaunchSpecs.gorrcd(nativeDir, paths).env["RRCD_HOME"])
    }

    @Test
    fun theHubIsToldToReAnnounceSoALaterBotCanFindIt() {
        // The hub's own default is to announce once and never again, so a bot that starts a
        // moment later can only ever say "Hub identity unknown". Without this argument the
        // local pair never converges.
        val hub = LaunchSpecs.gorrcd(nativeDir, paths)
        val index = hub.argv.indexOf("--announce-period")
        assertTrue("the hub must be given an announce period: ${hub.argv}", index >= 0)
        assertEquals("60", hub.argv[index + 1])
        assertEquals("60", LaunchSpecs.HUB_ANNOUNCE_PERIOD_SECONDS)
    }

    @Test
    fun theHubIsToldToNameTheJoinerSoABotCanLearnItsPeers() {
        // The hub fans a JOINED out to a room's existing members, and that fan-out carries
        // the joiner's hash only when this flag is set. Without it a bot already in a room
        // never learns a peer that joins afterwards, so it has no reply route and silently
        // drops every direct request from that peer:
        //     gorrcbot: no reply route for the direct request from <hash>
        // The flag is off by default and affects only this appliance's own hub, so the
        // appliance asks for it rather than the shared default being changed.
        val hub = LaunchSpecs.gorrcd(nativeDir, paths)
        assertTrue(
            "the hub must be told to name the joiner: ${hub.argv}",
            hub.argv.contains("--include-joined-member-list"),
        )
    }

    @Test
    fun theTransportStartsFirstBecauseItOwnsTheSharedInstance() {
        assertEquals(listOf("gornsd", "gorrcd", "gorrcbot"), LaunchSpecs.startOrder())
    }

    @Test
    fun theBundledBibleTextIsInstalledInsideTheBotsOwnHome() {
        // The bot's own storage directory is `<home>/storage`, so the installed text lands
        // where an operator's own `kjv_txt_file` would be on a desktop install. It cannot
        // live in the APK: an asset is inside a zip that no other process can open.
        assertEquals("${paths.gorrcbotHome}/storage/kjv.txt", paths.kjvTxtFile)
        assertTrue(paths.kjvTxtFile.startsWith(paths.filesDir))
        assertTrue(
            "it belongs beside the bot's own state, not in Termux's home or on /sdcard",
            paths.kjvTxtFile.startsWith(paths.gorrcbotHome),
        )
    }

    @Test
    fun theBotReadsTheFifosTheSensorServiceHoldsOpen() {
        val bot = LaunchSpecs.gorrcbot(nativeDir, paths)
        val config = NodeConfigRenderer.gorrcbotConfig(
            BotConfigSpec(paths.gpsFifo, paths.compassFifo, hubDestination = "00".repeat(16)),
        )
        assertTrue("the bot's configuration must point at the FIFOs", config.contains(paths.gpsFifo))
        assertEquals(paths.runDir, bot.env["HOME"])
    }
}
