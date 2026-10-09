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
 * The start order, and the one ordering that is a hang rather than a slow start.
 *
 * Opening the read end of a FIFO with no writer blocks until some process opens the write
 * end. The bot does exactly that while it starts, so a bot spawned before the sensor pipes
 * exist hangs with no error message at all — the worst kind of failure, because it looks
 * like a bot that is thinking.
 */
class NodeStartOrderTest {

    @Test
    fun thePipesAreHeldOpenStrictlyBeforeTheBotIsStarted() {
        val fixture = SupervisorFixture()
        val supervisor = fixture.supervisor()

        assertTrue("the stack did not start", supervisor.start())

        val fifoOpened = fixture.runner.indexOf("fifo:open:")
        val botSpawned = fixture.runner.indexOf("spawn:gorrcbot")
        assertTrue("the pipes must be opened", fifoOpened >= 0)
        assertTrue("the bot must be started", botSpawned >= 0)
        assertTrue(
            "the bot was started at $botSpawned, before its pipes were held at $fifoOpened: " +
                "it would block in open(2) with no error message",
            fifoOpened < botSpawned,
        )
    }

    @Test
    fun thePipesAreHeldOpenBeforeAnythingAtAllIsSpawned() {
        val fixture = SupervisorFixture()
        assertTrue(fixture.supervisor().start())

        val fifoOpened = fixture.runner.indexOf("fifo:open:")
        val firstSpawn = fixture.runner.events.indexOfFirst { it.startsWith("spawn:") }
        assertTrue(
            "the pipes were held at $fifoOpened, after the first daemon at $firstSpawn",
            fifoOpened < firstSpawn,
        )
    }

    @Test
    fun theTransportStartsFirstBecauseASharedInstanceIsOwnedByWhoeverAsksFirst() {
        val fixture = SupervisorFixture()
        assertTrue(fixture.supervisor().start())
        assertEquals(listOf("gornsd", "gorrcd", "gorrcbot"), fixture.runner.spawnOrder())
    }

    @Test
    fun theBotIsNotStartedUntilTheHubHasPublishedADestination() {
        val fixture = SupervisorFixture()
        // A hub that never publishes leaves the bot with nothing to dial. The supervisor
        // must say so and stop there, rather than starting a bot that dials the empty
        // string and reports a link failure nobody can explain.
        fixture.hubDestination = null

        assertTrue("the sensor side is still up, so start() reports success", fixture.supervisor().start())
        assertEquals(listOf("gornsd", "gorrcd"), fixture.runner.spawnOrder())
        assertTrue(
            "the reason must be logged: ${fixture.logs}",
            fixture.logs.any { it.contains("never published a destination") },
        )
    }

    @Test
    fun theBotConfigurationNamesThePipesAndThePublishedDestination() {
        val fixture = SupervisorFixture()
        assertTrue(fixture.supervisor().start())

        val botConfig = fixture.written[fixture.paths.gorrcbotConfig]
        assertTrue("the bot's configuration must be written", botConfig != null)
        assertTrue(
            "the bot must read the position pipe the supervisor is holding: $botConfig",
            botConfig!!.contains(fixture.paths.gpsFifo),
        )
        assertTrue(botConfig.contains(fixture.paths.compassFifo))
        assertTrue(
            "the bot must be told the destination the hub published: $botConfig",
            botConfig.contains("a012129c10205c0b9441fcd2b755b2a7"),
        )
    }

    @Test
    fun theHubIsBootstrappedBeforeItIsStartedAsADaemon() {
        // A hub's first run writes its own configuration from the template and exits 0.
        // Starting it as a daemon instead would leave a supervisor believing it had a hub
        // when what it had was a file and a dead process.
        val fixture = SupervisorFixture()
        assertTrue(fixture.supervisor().start())

        val bootstrap = fixture.runner.indexOf("runToCompletion:gorrcd")
        val hubSpawned = fixture.runner.indexOf("spawn:gorrcd")
        assertTrue("the hub must be run to completion on its first run", bootstrap >= 0)
        assertTrue(bootstrap < hubSpawned)
    }

    @Test
    fun aHubThatIsAlreadyConfiguredIsNotBootstrappedAgain() {
        val fixture = SupervisorFixture()
        fixture.installHubFiles()
        fixture.installBotFiles()

        assertTrue(fixture.supervisor().start())
        assertEquals(
            "a configured hub must not be run merely to bootstrap it",
            -1,
            fixture.runner.indexOf("runToCompletion:gorrcd"),
        )
        assertEquals(
            "a configured bot must not be run merely to bootstrap it",
            -1,
            fixture.runner.indexOf("runToCompletion:gorrcbot"),
        )
        assertEquals(listOf("gornsd", "gorrcd", "gorrcbot"), fixture.runner.spawnOrder())
    }

    @Test
    fun aHalfInstalledHubIsBootstrappedAgain() {
        // The configuration exists but the state directory the daemon writes its identity
        // into does not, which is what a restored backup or a failed first run leaves behind.
        // Starting the daemon on it produces an error about a missing identity rather than the
        // files it needs, so the supervisor runs it once more to put them back.
        val fixture = SupervisorFixture()
        java.io.File(fixture.paths.gorrcdConfig).also { it.parentFile?.mkdirs() }.writeText("half installed")

        assertTrue(fixture.supervisor().start())
        assertTrue(
            "a hub with no state directory must be bootstrapped again: ${fixture.runner.events}",
            fixture.runner.indexOf("runToCompletion:gorrcd") >= 0,
        )
    }

    @Test
    fun theBotIsBootstrappedBeforeItIsStartedAsADaemon() {
        // The bot bootstraps its own files exactly as the hub does, and exits 0 on its first
        // run. Starting it as a daemon instead leaves the supervisor believing it has a bot
        // when what it has is a dead process and a template.
        val fixture = SupervisorFixture()
        assertTrue(fixture.supervisor().start())

        val bootstrap = fixture.runner.indexOf("runToCompletion:gorrcbot")
        val botSpawned = fixture.runner.indexOf("spawn:gorrcbot")
        assertTrue("the bot must be run to completion on its first run", bootstrap >= 0)
        assertTrue(bootstrap < botSpawned)
    }

    @Test
    fun theHubAndTheBotAttachThroughTheClientConfigurationDirectory() {
        // The transport owns the shared instance; a client must never be able to take it. The
        // two directories are what separate those roles, and they must not be the same one.
        val fixture = SupervisorFixture()
        assertTrue(fixture.supervisor().start())

        assertTrue(
            "the transport's configuration directory must be written",
            fixture.written.containsKey(fixture.paths.gornsdConfig),
        )
        assertTrue(
            "the client configuration must be written",
            fixture.written.containsKey(fixture.paths.clientConfig),
        )
        assertTrue(
            "the two directories must be different",
            fixture.paths.gornsdConfig != fixture.paths.clientConfig,
        )

        val transport = fixture.written[fixture.paths.gornsdConfig]!!
        val client = fixture.written[fixture.paths.clientConfig]!!
        assertTrue("the transport owns the shared instance: $transport", transport.contains("shared_instance_type = tcp"))
        assertTrue("the client requires it: $client", client.contains("require_shared_instance = yes"))
        assertTrue("the client must own no interface: $client", client.contains("Deliberately empty"))

        // And each daemon has to be pointed at the right one.
        for (name in listOf("gorrcd", "gorrcbot")) {
            val spec = fixture.runner.specs.first { it.name == name }
            assertTrue(
                "$name must attach through the client configuration directory: ${spec.argv}",
                spec.argv.contains(fixture.paths.rnsClientConfigDir),
            )
        }
        val gornsd = fixture.runner.specs.first { it.name == "gornsd" }
        assertTrue(
            "gornsd must be given the transport's own directory: ${gornsd.argv}",
            gornsd.argv.contains(fixture.paths.rnsConfigDir),
        )
    }

    @Test
    fun everythingTheApplianceNeedsIsCreatedInsideItsOwnPrivateDirectory() {
        val fixture = SupervisorFixture()
        assertTrue(fixture.supervisor().start())
        for (path in fixture.written.keys) {
            assertTrue(
                "$path is outside the app's private directory, where nothing can be written",
                path.startsWith(fixture.filesDir),
            )
        }
    }
}
