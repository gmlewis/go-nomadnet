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
 * Stopping the stack, and restarting what dies.
 *
 * Two properties matter more than the rest. Stopping must leave nothing behind: a daemon
 * that outlives its supervisor is a daemon nobody can stop from the appliance's screen,
 * holding the shared instance and the sensor pipes. And a crash loop must end: a daemon
 * that dies ten times in a row is a bug or a misconfiguration, and restarting it forever
 * turns one into a battery fire.
 */
class SupervisorLifecycleTest {

    @Test
    fun theBibleTextIsInstalledBeforeTheBotIsSpawnedAndNamedInItsConfiguration() {
        // A daemon reads whatever is at the path it is given, so the order is the whole
        // point: the text has to be on disk before the bot that reads it starts, and the
        // configuration the bot is given has to name the path it was written to.
        val fixture = SupervisorFixture()
        val supervisor = fixture.supervisor()
        assertTrue(supervisor.start())

        assertEquals(
            "the asset must land exactly where the bot's configuration says it is",
            fixture.paths.kjvTxtFile,
            fixture.bundled.installed["kjv.txt"],
        )
        assertTrue(
            "the text must be installed before the bot starts: ${fixture.runner.events}",
            fixture.runner.indexOf("asset:kjv.txt") < fixture.runner.indexOf("spawn:gorrcbot"),
        )
        val botConfig = fixture.written[fixture.paths.gorrcbotConfig]!!
        assertTrue(
            "the bot must be told where the text is:\n$botConfig",
            botConfig.contains("kjv_txt_file = \"${fixture.paths.kjvTxtFile}\""),
        )
    }

    @Test
    fun anAssetThatCannotBeInstalledIsReportedAndTheStackStillStarts() {
        // An appliance that will not start because a Bible text is missing is worse than one
        // whose Bible command says the file cannot be read. The bot reports an unreadable
        // kjv_txt_file itself, which is the report an operator can act on.
        val fixture = SupervisorFixture()
        val supervisor = fixture.supervisor()
        fixture.bundled.succeed = false

        assertTrue("a missing asset must not stop the stack", supervisor.start())
        assertTrue(
            "the operator must be told: ${fixture.logs}",
            fixture.logs.any { it.contains("kjv.txt") },
        )
        assertEquals(listOf("gornsd", "gorrcd", "gorrcbot"), supervisor.running())
    }

    @Test
    fun stoppingDestroysEveryChildAndNoOthers() {
        val fixture = SupervisorFixture()
        val supervisor = fixture.supervisor()
        assertTrue(supervisor.start())
        assertEquals(listOf("gornsd", "gorrcd", "gorrcbot"), supervisor.running())

        supervisor.stop()

        assertTrue("every daemon must be dead", supervisor.running().isEmpty())
        for (name in listOf("gornsd", "gorrcd", "gorrcbot")) {
            assertTrue(
                "$name was not destroyed: ${fixture.runner.events}",
                fixture.runner.events.contains("destroy:$name"),
            )
            assertFalse("$name is still alive", fixture.runner.running[name]?.isAlive() ?: false)
        }
    }

    @Test
    fun stoppingReleasesThePipesSoTheBotSeesAnEndOfFileAndThePipesAreCleanedUp() {
        val fixture = SupervisorFixture()
        val supervisor = fixture.supervisor()
        assertTrue(supervisor.start())

        supervisor.stop()
        assertTrue("the supervisor must release the pipes it was holding", fixture.fifos.closed)
    }

    @Test
    fun aDaemonThatDiesOnceIsRestarted() {
        val fixture = SupervisorFixture()
        val supervisor = fixture.supervisor()
        assertTrue(supervisor.start())

        // The bot crashes. Nothing happens at once: even the first restart waits out the
        // first backoff, so a daemon that dies and comes straight back cannot spin.
        fixture.runner.running["gorrcbot"]!!.alive = false
        assertTrue("nothing restarts inside the first delay", supervisor.poll(nowMs = 1_000).isEmpty())

        val restarted = supervisor.poll(nowMs = 1_600)
        assertTrue("the bot must be restarted: ${fixture.runner.events}", "gorrcbot" in restarted)
        assertEquals(listOf("gornsd", "gorrcd", "gorrcbot"), supervisor.running())
    }

    @Test
    fun aRestartWaitsOutTheBackoffBeforeTrying() {
        val fixture = SupervisorFixture()
        val supervisor = fixture.supervisor()
        assertTrue(supervisor.start())

        fixture.runner.running["gornsd"]!!.alive = false
        assertTrue("nothing should restart inside the first delay", supervisor.poll(nowMs = 1_000).isEmpty())
        assertEquals("the dead daemon is no longer listed as running", listOf("gorrcd", "gorrcbot"), supervisor.running())

        // The first delay is 500 ms, so half of it is not enough and all of it is.
        assertTrue(supervisor.poll(nowMs = 1_200).isEmpty())
        assertEquals(listOf("gornsd"), supervisor.poll(nowMs = 1_600))
    }

    @Test
    fun aCrashLoopGivesUpRatherThanSpinningForever() {
        val fixture = SupervisorFixture()
        val supervisor = fixture.supervisor()
        assertTrue(supervisor.start())

        var now = 1_000L
        var attempts = 0
        repeat(20) {
            // Make every daemon look dead, then poll far enough ahead that any delay has
            // expired. A supervisor that never gave up would restart on all twenty.
            for (child in fixture.runner.running.values) {
                child.alive = false
            }
            now += 10 * 60_000
            if (supervisor.poll(now).isNotEmpty()) {
                attempts += 1
            }
        }

        assertTrue("a crash loop must eventually stop", supervisor.abandoned().isNotEmpty())
        assertTrue("the loop ran $attempts times, so it never stopped", attempts < 20)
        assertTrue(
            "the operator must be told which daemon was given up on: ${fixture.logs}",
            fixture.logs.any { it.contains("not restarting it again") },
        )
    }

    @Test
    fun stoppingCancelsARestartThatIsWaiting() {
        val fixture = SupervisorFixture()
        val supervisor = fixture.supervisor()
        assertTrue(supervisor.start())

        fixture.runner.running["gorrcd"]!!.alive = false
        assertTrue(supervisor.poll(nowMs = 1_000).isEmpty())

        supervisor.stop()
        // Long after the delay would have expired, the operator's decision stands.
        assertTrue("a stopped stack must not come back by itself", supervisor.poll(nowMs = 10_000_000).isEmpty())
        assertTrue(supervisor.running().isEmpty())
    }

    @Test
    fun aRestartedBotIsGivenAFreshlyReadDestination() {
        val fixture = SupervisorFixture()
        val supervisor = fixture.supervisor()
        assertTrue(supervisor.start())

        // The hub restarted and announced a new destination; the bot must dial that one and
        // not the address it read the first time.
        fixture.hubDestination = "ffffffffffffffffffffffffffffffff"
        fixture.runner.running["gorrcbot"]!!.alive = false
        assertTrue(supervisor.poll(nowMs = 1_000).isEmpty())
        assertEquals(listOf("gorrcbot"), supervisor.poll(nowMs = 1_600))

        val botConfig = fixture.written[fixture.paths.gorrcbotConfig]!!
        assertTrue(
            "the restarted bot must dial the current destination: $botConfig",
            botConfig.contains("ffffffffffffffffffffffffffffffff"),
        )
    }
}
