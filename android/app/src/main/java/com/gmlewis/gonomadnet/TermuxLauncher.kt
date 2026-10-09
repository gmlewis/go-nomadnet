// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import android.content.ComponentName
import android.content.Context
import android.content.Intent

/**
 * The intent that starts a `gonomadnet` terminal session in Termux.
 *
 * A tview/tcell application needs a real PTY and a terminal emulator, so the
 * appliance does not host the user interface itself: it starts the one Termux already
 * provides. This type builds the request as plain data — [RunCommandRequest] — and only
 * then turns it into an intent, which is what makes the whole contract testable on the
 * JVM against the documented constants rather than against the implementation.
 */
data class RunCommandRequest(
    val commandPath: String,
    val arguments: List<String>,
    val workdir: String,
    val background: Boolean = false,
    val sessionAction: Int = SESSION_ACTION_CREATE,
) {
    /** The intent's extras, exactly as Termux documents them. */
    fun extras(): Map<String, Any> = linkedMapOf(
        EXTRA_COMMAND_PATH to commandPath,
        EXTRA_ARGUMENTS to arguments.toTypedArray(),
        EXTRA_WORKDIR to workdir,
        EXTRA_BACKGROUND to background,
        EXTRA_SESSION_ACTION to sessionAction.toString(),
    )

    companion object {
        const val ACTION = "com.termux.RUN_COMMAND"
        const val PACKAGE = "com.termux"
        const val SERVICE = "com.termux.app.RunCommandService"
        const val PERMISSION = "com.termux.permission.RUN_COMMAND"

        const val EXTRA_COMMAND_PATH = "com.termux.RUN_COMMAND_PATH"
        const val EXTRA_ARGUMENTS = "com.termux.RUN_COMMAND_ARGUMENTS"
        const val EXTRA_WORKDIR = "com.termux.RUN_COMMAND_WORKDIR"
        const val EXTRA_BACKGROUND = "com.termux.RUN_COMMAND_BACKGROUND"
        const val EXTRA_SESSION_ACTION = "com.termux.RUN_COMMAND_SESSION_ACTION"

        /** Attach to an existing session when the command is the same. */
        const val SESSION_ACTION_CREATE = 0

        /** Always open a new session, so two modes cannot share a pane by accident. */
        const val SESSION_ACTION_NEW = 1
    }
}

/** Turns a [RunCommandRequest] into the intent Termux expects. */
object TermuxLauncher {

    /** The Termux home directory, which is another application's private data. */
    const val TERMUX_HOME = "/data/data/com.termux/files/home"

    /**
     * The shell inside Termux's prefix, by its absolute path.
     *
     * Termux's `$PREFIX` is `/data/data/com.termux/files/usr`, so its bash is not at
     * `/usr/bin/bash` — Android has no such file. It is named absolutely rather than as `bash`
     * for the reason every program started from this appliance is: Android's seccomp policy
     * terminates a process that resolves a program name through the dynamic linker, so a bare
     * name is a process that dies without a message.
     */
    const val TERMUX_BASH = "/data/data/com.termux/files/usr/bin/bash"

    /**
     * The request that starts the standalone client: the one whose Reticulum
     * configuration is the tablet's own, untouched.
     */
    fun standaloneRequest(): RunCommandRequest = RunCommandRequest(
        commandPath = "$TERMUX_HOME/gonomadnet",
        arguments = listOf("-t"),
        workdir = TERMUX_HOME,
        sessionAction = RunCommandRequest.SESSION_ACTION_NEW,
    )

    /**
     * The request that starts the client attached to this appliance's transport. It
     * names its own Reticulum configuration directory, which has no interfaces of its
     * own and requires the shared instance this appliance owns.
     */
    fun attachedRequest(): RunCommandRequest = RunCommandRequest(
        commandPath = "$TERMUX_HOME/gonomadnet",
        arguments = listOf("-t", "--rnsconfig", "$TERMUX_HOME/.reticulum-stack"),
        workdir = TERMUX_HOME,
        sessionAction = RunCommandRequest.SESSION_ACTION_NEW,
    )

    /**
     * The line a person pastes into Termux to set the terminal up, given the published script.
     *
     * The appliance sends the same thing as [setupRequest], so the two ways in cannot describe
     * different work, and it is one line because the script it names does all of it. The script
     * is given no arguments: it finds what it needs in shared storage, or reports that it
     * cannot, and it is the same script either way.
     */
    fun setupLine(scriptPath: String): String = "bash $scriptPath"

    /**
     * The request that runs the published setup script inside Termux.
     *
     * Termux is the only process on this device that may write into Termux's own home
     * directory, so the appliance's part is to name the script and get out of the way. The
     * session is a new one, because a setup that reported into a pane somebody was already
     * reading would be a setup whose output went somewhere else.
     */
    fun setupRequest(scriptPath: String): RunCommandRequest = RunCommandRequest(
        commandPath = TERMUX_BASH,
        arguments = listOf(scriptPath),
        workdir = TERMUX_HOME,
        sessionAction = RunCommandRequest.SESSION_ACTION_NEW,
    )

    /** Builds the intent for a request, ready to hand to `startService`. */
    fun intent(request: RunCommandRequest): Intent {
        val intent = Intent()
        intent.component = ComponentName(RunCommandRequest.PACKAGE, RunCommandRequest.SERVICE)
        intent.action = RunCommandRequest.ACTION
        for ((key, value) in request.extras()) {
            when (value) {
                is String -> intent.putExtra(key, value)
                is Array<*> -> intent.putExtra(key, value.map { it.toString() }.toTypedArray())
                else -> intent.putExtra(key, value.toString())
            }
        }
        return intent
    }

    /** Dispatches the request, reporting the failure rather than swallowing it. */
    fun launch(context: Context, request: RunCommandRequest): Result<Unit> = runCatching {
        context.startService(intent(request))
        Unit
    }
}
