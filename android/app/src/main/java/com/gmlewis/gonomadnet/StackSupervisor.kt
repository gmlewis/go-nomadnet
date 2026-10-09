// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import java.io.File

/**
 * Runs the three daemons: the transport, the hub, and the bot.
 *
 * It is an optional half of the appliance and it is independently controllable. The
 * sensor service runs with or without it, which is what lets the Nomad Network client
 * have a real position on an appliance that is not running a local bot at all.
 *
 * The order is not a preference:
 *
 *  1. The pipes are created and held, because a bot spawned before its sensor stream
 *     exists blocks forever in `open(2)` with no error message.
 *  2. The transport starts and owns the shared instance. A shared instance is created
 *     by whoever asks first, so the appliance must be first.
 *  3. The hub starts and attaches, then publishes the destination a bot has to dial.
 *  4. The bot starts with that destination already in its configuration.
 *
 * Everything here goes through interfaces so the ordering, the restart policy, and the
 * teardown can be asserted without executing anything.
 */
class StackSupervisor(
    private val nativeDir: String,
    private val paths: StackPaths,
    private val runner: ProcessRunner,
    private val fifos: SensorFifoHolder,
    private val bundled: BundledAssets,
    private val readHubDestination: () -> String?,
    private val writeConfig: (path: String, contents: String) -> Unit,
    private val config: NodeConfigSpec = NodeConfigSpec(),
    private val log: (String) -> Unit = {},
    private val sleep: (Long) -> Unit = { Thread.sleep(it) },
) {
    private val children = LinkedHashMap<String, ChildProcess>()
    private val backoff = BackoffPolicy()

    /**
     * Names waiting out a restart delay, mapped to the instant their next attempt is due.
     *
     * The wait is a timestamp rather than a sleep so that the supervisor never blocks the
     * caller: a service polls it, and a test moves a clock by hand instead of waiting.
     */
    private val pendingRestarts = LinkedHashMap<String, Long>()

    /** Names the supervisor has given up on, so the operator can be told which. */
    private val abandoned = mutableListOf<String>()

    /** How many times the hub destination was polled before it appeared. */
    var hubDestinationPolls: Int = 0
        private set

    /** Starts the whole stack. Returns false when the appliance cannot even begin. */
    fun start(): Boolean {
        // 1. Hold the pipes open before anything is spawned.
        if (!fifos.open()) {
            log("could not create and hold the sensor pipes; not starting the stack")
            return false
        }
        File(paths.runDir).mkdirs()
        File(paths.logDir).mkdirs()
        File(paths.configDir).mkdirs()
        File(paths.rnsConfigDir).mkdirs()
        File(paths.rnsClientConfigDir).mkdirs()

        // 2. The transport, which owns the shared instance and every interface. Its
        //    configuration is a DIRECTORY: gornsd looks for "config" inside it and derives
        //    its instance lock from the same path, and handing it a file makes it exit at
        //    once with "not a directory".
        writeConfig(paths.gornsdConfig, NodeConfigRenderer.reticulumConfig(config))
        //    The hub and the bot attach through a second directory that requires the shared
        //    instance, so neither can win a race for ownership and quietly become the
        //    transport instead of a client of it.
        writeConfig(paths.clientConfig, NodeConfigRenderer.clientReticulumConfig(config))
        if (!spawn(LaunchSpecs.gornsd(nativeDir, paths))) {
            stop()
            return false
        }

        // 3. The hub, which attaches to the shared instance. Its first run writes its
        //    own configuration from the template and exits 0, so a first run is told
        //    apart from a real one by running it to completion and starting a daemon
        //    only once there is something for it to read.
        val hub = LaunchSpecs.gorrcd(nativeDir, paths)
        if (needsBootstrap(paths.gorrcdConfig, paths.gorrcdHome)) {
            log("bootstrapping the hub configuration")
            runner.runToCompletion(hub)
        }
        if (!spawn(hub)) {
            stop()
            return false
        }

        // 4. The bot, which needs the hub's destination before it can start.
        val destination = awaitHubDestination() ?: run {
            log("the hub never published a destination; not starting the bot")
            return true
        }
        //    The bot bootstraps its own files the same way and exits 0 on its first run, so
        //    there is nothing for it to read until it has been run once to completion.
        val bot = LaunchSpecs.gorrcbot(nativeDir, paths)
        if (needsBootstrap(paths.gorrcbotConfig, paths.gorrcbotHome)) {
            log("bootstrapping the bot configuration")
            runner.runToCompletion(bot)
        }
        //    The bot reads the Bible text at the path it is given and nowhere else, so it is
        //    written out before the bot is started. A failure is reported rather than raised:
        //    an appliance that will not start because a text is missing is worse than one
        //    whose Bible command says the file cannot be read.
        if (!bundled.install(KJV_ASSET_NAME, paths.kjvTxtFile)) {
            log("could not install the bundled $KJV_ASSET_NAME; the kjv command will report the file as unreadable")
        }
        writeConfig(
            paths.gorrcbotConfig,
            NodeConfigRenderer.gorrcbotConfig(
                BotConfigSpec(
                    gpsPort = paths.gpsFifo,
                    compassPort = paths.compassFifo,
                    kjvTxtFile = paths.kjvTxtFile,
                    hubDestination = destination,
                ),
            ),
        )
        spawn(bot)
        log("the stack is up: ${LaunchSpecs.startOrder().joinToString(", ")}")
        return true
    }

    /** Stops every child and releases the pipes. Nothing is left running. */
    fun stop() {
        for ((name, child) in children.entries.toList()) {
            if (child.destroy()) {
                log("stopped $name")
            }
        }
        children.clear()
        // A pending restart is a restart that must not happen once the operator has
        // asked for the stack to be down.
        pendingRestarts.clear()
        abandoned.clear()
        fifos.close()
        backoff.reset()
    }

    /** The names of the children currently running, in start order. */
    fun running(): List<String> = children.keys.toList()

    /** The names the supervisor gave up restarting, because the crash loop went on. */
    fun abandoned(): List<String> = abandoned.toList()

    /**
     * Checks on the children and restarts the ones that have died, with a bounded backoff.
     * Returns the names it started this time.
     *
     * A daemon that dies once is a daemon that had a bad day; a daemon that dies ten times
     * in a row is a bug or a misconfiguration, and restarting it forever turns one into a
     * battery fire. The policy doubles the delay up to a ceiling and then stops, and a
     * daemon that comes back up and stays up clears its own history.
     */
    fun poll(nowMs: Long): List<String> {
        val restarted = mutableListOf<String>()

        for ((name, child) in children.entries.toList()) {
            if (child.isAlive() || pendingRestarts.containsKey(name)) {
                continue
            }
            children.remove(name)
            val delay = backoff.recordExit(nowMs)
            if (delay == null) {
                log("$name died ${backoff.failures()} times in a row; not restarting it again")
                abandoned.add(name)
                continue
            }
            log("$name stopped; restarting it in $delay ms")
            pendingRestarts[name] = nowMs + delay
        }

        for ((name, dueAt) in pendingRestarts.entries.toList()) {
            if (nowMs < dueAt) {
                continue
            }
            pendingRestarts.remove(name)
            val spec = specFor(name, nowMs) ?: continue
            if (spawn(spec)) {
                restarted.add(name)
            }
        }
        return restarted
    }

    /**
     * The launch spec for a daemon by name, with the hub's freshly published destination
     * re-rendered for the bot: a hub that restarted has announced again, and a bot that
     * dialed the address it read the first time would be dialing a stale one.
     */
    private fun specFor(name: String, nowMs: Long): LaunchSpec? = when (name) {
        "gornsd" -> LaunchSpecs.gornsd(nativeDir, paths)
        "gorrcd" -> LaunchSpecs.gorrcd(nativeDir, paths)
        "gorrcbot" -> {
            readHubDestination()?.takeIf { it.isNotBlank() }?.let { destination ->
                writeConfig(
                    paths.gorrcbotConfig,
                    NodeConfigRenderer.gorrcbotConfig(
                        BotConfigSpec(
                            gpsPort = paths.gpsFifo,
                            compassPort = paths.compassFifo,
                            kjvTxtFile = paths.kjvTxtFile,
                            hubDestination = destination.trim(),
                        ),
                    ),
                )
                LaunchSpecs.gorrcbot(nativeDir, paths)
            }
        }
        else -> null
    }.also { _ -> nowMs }

    /**
     * Waits for the hub to publish the destination a bot has to dial.
     *
     * The hub derives it from its own identity, which a supervisor in another language
     * cannot do, so the hub writes it down. The wait is bounded, and a hub that never
     * publishes is reported rather than waited on forever.
     */
    private fun awaitHubDestination(): String? {
        repeat(HUB_DESTINATION_POLLS) {
            hubDestinationPolls += 1
            readHubDestination()?.takeIf { it.isNotBlank() }?.let { return it.trim() }
            sleep(HUB_DESTINATION_POLL_MS)
        }
        return null
    }

    /**
     * Reports whether a daemon has still to write its own default files.
     *
     * A daemon whose first run writes a template and exits 0 cannot be started as a daemon:
     * the supervisor would believe it had a running service when what it had was a file and
     * a dead process. Both the hub and the bot behave this way, so both are run to completion
     * once before they are started for real.
     */
    private fun needsBootstrap(configPath: String, home: String): Boolean =
        !File(configPath).isFile || !File(home).isDirectory

    private fun spawn(spec: LaunchSpec): Boolean {
        log("starting ${spec.name}: ${spec.commandLine()}")
        val child = try {
            runner.spawn(spec)
        } catch (failure: Throwable) {
            log("could not start ${spec.name}: ${failure.message}")
            return false
        }
        children[spec.name] = child
        backoff.recordStart(System.currentTimeMillis())
        return true
    }

    companion object {
        /** How many times to look for the hub's published destination. */
        const val HUB_DESTINATION_POLLS = 60

        /** How long to wait between looks, in milliseconds. */
        const val HUB_DESTINATION_POLL_MS = 250L
    }
}
