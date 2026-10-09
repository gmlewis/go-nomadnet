// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import java.io.File

/**
 * A recorder of everything that happened, in order.
 *
 * Every assertion below is about *when* something happened relative to something else —
 * a pipe held open before a bot is spawned, a daemon stopped and nothing else touched —
 * so the fake records an ordered event log rather than a set of booleans.
 */
class RecordingProcessRunner : ProcessRunner {
    val events = mutableListOf<String>()
    val specs = mutableListOf<LaunchSpec>()
    val running = LinkedHashMap<String, FakeChildProcess>()
    private val exitCodes = mutableMapOf<String, Int>()

    /** Makes the next spawn of this daemon return a child that is already dead. */
    fun failNextStartFor(name: String, exitCode: Int = 1) {
        exitCodes[name] = exitCode
    }

    override fun spawn(spec: LaunchSpec): ChildProcess {
        events.add("spawn:${spec.name}")
        specs.add(spec)
        val child = FakeChildProcess(spec.name, events)
        if (exitCodes.containsKey(spec.name)) {
            child.alive = false
        }
        running[spec.name] = child
        return child
    }

    override fun spawnWritingTo(spec: LaunchSpec, outputPath: String): ChildProcess {
        events.add("spawnWritingTo:${spec.name}:$outputPath")
        return FakeChildProcess(spec.name, events)
    }

    override fun spawnPiped(binary: String, argv: List<String>, env: Map<String, String>): ChildProcess {
        events.add("spawnPiped:${File(binary).name}")
        return FakeChildProcess(File(binary).name, events)
    }

    override fun runToCompletion(spec: LaunchSpec): Int {
        events.add("runToCompletion:${spec.name}")
        return 0
    }

    /** The index of the first event with the given prefix, or -1. */
    fun indexOf(prefix: String): Int = events.indexOfFirst { it.startsWith(prefix) }

    /** The order of the names recorded by the spawn events only. */
    fun spawnOrder(): List<String> = events.filter { it.startsWith("spawn:") }.map { it.removePrefix("spawn:") }
}

/** A child that can be killed from a test and that records its own death. */
class FakeChildProcess(val name: String, private val events: MutableList<String>) : ChildProcess {
    var alive = true

    override fun isAlive(): Boolean = alive

    override fun destroy(): Boolean {
        val wasAlive = alive
        alive = false
        events.add("destroy:$name")
        return wasAlive
    }

    override fun stdin(): java.io.OutputStream? = null
    override fun stdout(): java.io.InputStream? = null
}

/** A pipe holder that records when it was opened and closed, and what it made. */
class RecordingFifos(private val paths: StackPaths, private val events: MutableList<String>) : SensorFifoHolder {
    var opened = false
        private set
    var closed = false
        private set
    var succeed = true

    override fun open(): Boolean {
        if (!succeed) {
            events.add("fifo:failed")
            return false
        }
        opened = true
        events.add("fifo:open:${paths.gpsFifo}:${paths.compassFifo}")
        return true
    }

    override fun close() {
        closed = true
        events.add("fifo:close")
    }
}

/** An asset installer that records what it was asked for, and can be made to fail. */
class RecordingBundledAssets(private val events: MutableList<String>) : BundledAssets {
    val installed = LinkedHashMap<String, String>()
    var succeed = true

    override fun install(name: String, destination: String): Boolean {
        events.add("asset:$name")
        if (!succeed) {
            return false
        }
        installed[name] = destination
        return true
    }
}

/** Builds a supervisor wired to recorders, with a hub destination that appears on demand. */
class SupervisorFixture(
    val nativeDir: String = "/data/app/~~x==/com.gmlewis.gonomadnet-y==/lib/arm64",
    // A real directory on this machine, because the supervisor checks whether the hub's
    // configuration already exists. It is laid out exactly as the appliance's own private
    // directory is, so every path assertion below is about the shape rather than the host.
    val filesDir: String = java.nio.file.Files.createTempDirectory("gonomadnet-supervisor")
        .toFile().absolutePath,
) {
    val paths = StackPaths(filesDir)
    val runner = RecordingProcessRunner()
    lateinit var fifos: RecordingFifos
    lateinit var bundled: RecordingBundledAssets
    val written = LinkedHashMap<String, String>()
    val logs = mutableListOf<String>()
    var hubDestination: String? = "a012129c10205c0b9441fcd2b755b2a7"

    /** Leaves behind exactly what a successful first run of the hub creates. */
    fun installHubFiles() {
        java.io.File(paths.gorrcdConfig).also { it.parentFile?.mkdirs() }.writeText("configured")
        java.io.File(paths.gorrcdHome).mkdirs()
    }

    /** Leaves behind exactly what a successful first run of the bot creates. */
    fun installBotFiles() {
        java.io.File(paths.gorrcbotConfig).also { it.parentFile?.mkdirs() }.writeText("configured")
        java.io.File(paths.gorrcbotHome).mkdirs()
    }

    fun supervisor(): StackSupervisor {
        fifos = RecordingFifos(paths, runner.events)
        bundled = RecordingBundledAssets(runner.events)
        return StackSupervisor(
            nativeDir = nativeDir,
            paths = paths,
            runner = runner,
            fifos = fifos,
            bundled = bundled,
            readHubDestination = { hubDestination },
            writeConfig = { path, contents -> written[path] = contents },
            log = { logs.add(it) },
            sleep = { },
        )
    }
}
