// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import java.io.File
import java.io.InputStream
import java.io.OutputStream

/** One running child. */
interface ChildProcess {
    /** Reports whether it is still alive, which is how a supervisor notices a crash. */
    fun isAlive(): Boolean

    /** Terminates it, and reports whether it stopped. */
    fun destroy(): Boolean

    /** Its standard input, which is where a JSON sample stream is fed in. */
    fun stdin(): OutputStream?

    /** Its standard output, when it was started with a readable one. */
    fun stdout(): InputStream?
}

/**
 * Starts children. It is an interface so the supervisor's ordering, restart and
 * teardown behaviour can be asserted against a fake: no unit test may execute a real
 * binary, and no unit test may leave one behind.
 */
interface ProcessRunner {
    /** Starts a long-running daemon. Its output is appended to the spec's log file. */
    fun spawn(spec: LaunchSpec): ChildProcess

    /**
     * Starts a converter whose sentences are what another process reads. Its standard
     * output is the given path — a FIFO the bot is holding open — and its diagnostics
     * still go to the log file.
     */
    fun spawnWritingTo(spec: LaunchSpec, outputPath: String): ChildProcess

    /** Starts a converter whose standard output is read back by the caller. */
    fun spawnPiped(binary: String, argv: List<String>, env: Map<String, String>): ChildProcess

    /**
     * Runs a program to completion and returns its exit code.
     *
     * A daemon that bootstraps its own state file, writes the template, and exits 0
     * cannot be started as a daemon at all: the first run would leave a supervisor
     * believing it had a hub on its hands when what it had was a file. Running it to
     * completion first is how a first run and a real run are told apart.
     */
    fun runToCompletion(spec: LaunchSpec): Int
}

/**
 * The real runner. Every child is started from an absolute path with the environment
 * the spec names, which is the whole point of a [LaunchSpec]: an unqualified program
 * name makes Android's seccomp policy kill the process outright, and an inherited
 * environment makes a daemon write its identity somewhere nobody chose.
 */
class RealProcessRunner : ProcessRunner {

    override fun spawn(spec: LaunchSpec): ChildProcess = start(spec, logRedirect())

    override fun spawnWritingTo(spec: LaunchSpec, outputPath: String): ChildProcess =
        start(spec, ProcessBuilder.Redirect.appendTo(prepare(outputPath)))

    override fun spawnPiped(binary: String, argv: List<String>, env: Map<String, String>): ChildProcess {
        val builder = ProcessBuilder(listOf(binary) + argv)
        builder.environment().putAll(env)
        return RealChild(builder.start())
    }

    private fun start(spec: LaunchSpec, output: ProcessBuilder.Redirect): ChildProcess {
        val builder = ProcessBuilder(listOf(spec.binary) + spec.argv)
        builder.environment().putAll(spec.env)
        builder.directory(prepare(spec.env["HOME"] ?: "/"))
        builder.redirectOutput(output)
        builder.redirectError(ProcessBuilder.Redirect.appendTo(prepare(spec.logFile)))
        return RealChild(builder.start())
    }

    override fun runToCompletion(spec: LaunchSpec): Int {
        val builder = ProcessBuilder(listOf(spec.binary) + spec.argv)
        builder.environment().putAll(spec.env)
        builder.directory(prepare(spec.env["HOME"] ?: "/"))
        builder.redirectOutput(ProcessBuilder.Redirect.appendTo(prepare(spec.logFile)))
        builder.redirectError(ProcessBuilder.Redirect.appendTo(prepare(spec.logFile)))
        val process = builder.start()
        return try {
            process.waitFor()
        } catch (interrupted: InterruptedException) {
            Thread.currentThread().interrupt()
            process.destroy()
            -1
        }
    }

    private fun logRedirect(): ProcessBuilder.Redirect = ProcessBuilder.Redirect.PIPE

    private fun prepare(path: String): File = File(path).also { it.parentFile?.mkdirs() }
}

/** A child the real runner started. */
private class RealChild(private val process: Process) : ChildProcess {
    override fun isAlive(): Boolean = process.isAlive

    override fun destroy(): Boolean {
        // Destroy the child alone rather than its whole tree: none of these daemons
        // deliberately starts anything, and a tree kill would take out a helper the
        // operator had attached.
        process.destroy()
        return try {
            process.waitFor()
            true
        } catch (interrupted: InterruptedException) {
            Thread.currentThread().interrupt()
            false
        }
    }

    override fun stdin(): OutputStream? = process.outputStream
    override fun stdout(): InputStream? = process.inputStream
}
