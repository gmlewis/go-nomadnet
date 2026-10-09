// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import java.io.File

/**
 * Every path the appliance uses inside its own private data directory.
 *
 * Nothing here is in Termux's home, and nothing here is on /sdcard: an app cannot
 * write to another app's private data, and /sdcard supports neither FIFOs nor
 * symlinks. Everything the daemons need is created under `filesDir`, which is why
 * the appliance needs no manual setup beyond one permission grant.
 */
data class StackPaths(val filesDir: String) {
    val runDir get() = "$filesDir/run"
    val logDir get() = "$filesDir/logs"
    val configDir get() = "$filesDir/etc"

    /** The hub's state directory, which is what RRCD_HOME points at. */
    val gorrcdHome get() = "$configDir/rrcd"

    /** The bot's state directory, which is what GORRCBOT_HOME points at. */
    val gorrcbotHome get() = "$configDir/gorrcbot"

    /**
     * The King James Version text the bot's `kjv` command reads.
     *
     * It sits in the bot's own storage directory, which is `<home>/storage` and is where an
     * operator's own `kjv_txt_file` lives on a desktop install. It is written there from the
     * APK's assets, because an asset is inside a zip that no other process can open.
     */
    val kjvTxtFile get() = "$gorrcbotHome/storage/kjv.txt"

    val gpsFifo get() = "$runDir/gps.nmea"
    val compassFifo get() = "$runDir/compass.nmea"
    /**
     * The transport's Reticulum configuration DIRECTORY.
     *
     * `gornsd --config` takes a directory, not a file: the daemon looks for `config`
     * inside it and derives its instance lock from the same path. Handing it a file
     * produces `acquire instance lock ...: not a directory` and the daemon exits at once.
     */
    val rnsConfigDir get() = "$configDir/reticulum"

    /** The Reticulum configuration file inside [rnsConfigDir]. */
    val gornsdConfig get() = "$rnsConfigDir/config"

    /**
     * The Reticulum configuration directory the hub and the bot attach through.
     *
     * It is separate from [rnsConfigDir] on purpose. The transport's own directory says
     * `share_instance = yes` and lists interfaces; a client's says
     * `require_shared_instance = yes` and lists none, so a client can never win a race for
     * ownership of the shared instance and quietly become the transport instead.
     */
    val rnsClientConfigDir get() = "$configDir/client"

    /** The client-side Reticulum configuration file. */
    val clientConfig get() = "$rnsClientConfigDir/config"

    /**
     * The client's home directory, which is what `HOME` points at when it runs.
     *
     * A program that has not been told where to keep its caches and its logs looks in its
     * home, and on this device the only home the appliance may write is this one. It is a
     * directory of its own rather than the client's configuration directory, because the
     * console host runs in it and a relative path written there must land in the appliance's
     * storage rather than wherever the host happened to be started.
     */
    val clientHome get() = "$filesDir/home"

    /**
     * The client's own Nomad Network configuration directory, which is what `--config`
     * points at: its messages, its pages, its peers and its identity.
     *
     * It is the built-in client's own, and deliberately not Termux's. A client that read
     * Termux's configuration would be a second client announcing one identity from two
     * places at once, and a client that wrote to it would be writing into another
     * application's private data, which Android does not permit.
     */
    val nomadnetworkConfigDir get() = "$configDir/nomadnetwork"

    /**
     * The client's configuration file inside [nomadnetworkConfigDir].
     *
     * It is named here for the same reason the directory is: the appliance writes it before
     * the client has ever run, so that the client's first run is a node rather than a client
     * that serves nothing — see [ClientNode].
     */
    val nomadnetworkConfig get() = "$nomadnetworkConfigDir/config"

    /**
     * The client's storage directory, where its peer directory, its channel store and its
     * message history live.
     *
     * It is named here rather than left to the client to create because the appliance writes
     * one file into it before the client has ever run — the channels it comes with — and a
     * path the appliance only guessed at is a file written somewhere the client will never
     * look.
     */
    val nomadnetworkStorageDir get() = "$nomadnetworkConfigDir/storage"

    /**
     * The client's own channel store.
     *
     * It belongs to the client: `rrc.RRCManager` reads and rewrites it, so the appliance only
     * writes it when there is none, which is the install that has never run the client.
     */
    val clientHubStore get() = "$nomadnetworkStorageDir/rrc_hubs"

    /** The hub's TOML configuration, which `gorrcd --config` takes as a file. */
    val gorrcdConfig get() = "$configDir/rrcd.toml"

    /** The bot's TOML configuration, which `gorrcbot --bot-config` takes as a file. */
    val gorrcbotConfig get() = "$configDir/gorrcbot.toml"

    /**
     * Where the hub publishes the `rrc.hub` destination a bot has to dial.
     *
     * The hub derives it from its own identity, which needs the identity and a running
     * transport, and a supervisor in another language cannot repeat the derivation. So the
     * hub writes it down — beside its identity, which is the one path a supervisor had to
     * name in order to start the hub at all. The suffix is the hub's own; the path here has
     * to match `hubDestinationPath` in `cmd/gorrcd/hub-destination.go`.
     */
    val hubDestinationFile get() = "$gorrcdHome/hub_identity.rrc.hub"

    /**
     * Every directory the appliance runs in, and the one list that creates them.
     *
     * A child is started with its HOME as its working directory, and a working directory
     * that does not exist is not a child that starts somewhere else — it is a child that
     * does not start:
     *
     *	Cannot run program ".../libgorcons.so" (in directory ".../files/home"):
     *	error=2, No such file or directory
     *
     * The program is there, the directory is not, and the error names the program. So the
     * directories are one list rather than five lines in the supervisor and a hope that
     * everyone else remembered: the stack and the console both create what this returns,
     * and neither can be right while the other is wrong.
     *
     * Directories a child only writes *inside* are not here. `files/etc/reticulum/config`
     * is written by its own renderer, which creates its parent as it writes, and a list that
     * had to know about it would be a list that went stale every time a file was added.
     *
     * [gorrcdHome] and [gorrcbotHome] are deliberately not here either, and that omission is
     * load-bearing: a daemon that has not yet written its own files has no state directory,
     * so an empty state directory is how the supervisor knows a hub or a bot was restored
     * from a backup, or died mid-install, and has to be run once more before it is started
     * for real. Creating them here would make every half-installed daemon look installed.
     */
    val directories: List<String>
        get() = listOf(
            runDir,
            logDir,
            configDir,
            rnsConfigDir,
            rnsClientConfigDir,
            clientHome,
            nomadnetworkConfigDir,
        )

    /**
     * ensureDirectories creates every directory in [directories].
     *
     * Creating a directory that exists is not an error, so this is safe to call on every
     * start, which is what makes it usable as the one place that answers "is the appliance
     * ready to run something?". Failures are ignored for the same reason the paths are
     * absolute: what the caller does next will fail with a message naming the path, and a
     * throw here would only replace a good error with a worse one.
     */
    fun ensureDirectories() {
        for (path in directories) {
            File(path).mkdirs()
        }
    }
}

/**
 * The exact argv and environment for one bundled daemon.
 *
 * The argv holds absolute paths only. Android's seccomp policy kills a process on
 * the `faccessat2(2)` that Go's `exec.LookPath` issues while resolving an
 * unqualified name, so a bare program name is not a style preference here — it is
 * a crash. [binary] is therefore the absolute path inside `nativeLibraryDir`.
 */
data class LaunchSpec(
    val name: String,
    val binary: String,
    val argv: List<String>,
    val env: Map<String, String>,
    val logFile: String,
) {
    /** The command line as the log should show it. */
    fun commandLine(): String = (listOf(binary) + argv).joinToString(" ")
}

/**
 * The four programs the appliance runs, each described as an absolute-path argv.
 *
 * Every daemon gets its own home inside the app's private directory, so it writes
 * its identity and its state there and never touches Termux's `~/.nomadnetwork` or
 * `~/.reticulum`. The tablet therefore keeps exactly one NomadNet identity — the
 * one Termux owns — regardless of how the appliance is used.
 */
object LaunchSpecs {

    /**
     * How often the hub re-announces, in seconds.
     *
     * Zero, the hub's own default, means "once, at startup", which is a race against every
     * consumer that is not already listening. Sixty seconds is comfortably inside Reticulum's
     * per-destination announce allowance and costs one small packet a minute.
     */
    const val HUB_ANNOUNCE_PERIOD_SECONDS = "60"

    /** The bundled executable for a daemon, as it lands in `nativeLibraryDir`. */
    fun binary(nativeLibraryDir: String, name: String): String = "$nativeLibraryDir/lib$name.so"

    /**
     * gornsd: the Reticulum transport. It owns every interface and the shared
     * instance, and it must be the first daemon started: a shared instance is
     * created by whoever asks first, and Termux's client is configured to require
     * rather than to steal one.
     */
    fun gornsd(nativeLibraryDir: String, paths: StackPaths): LaunchSpec = LaunchSpec(
        name = "gornsd",
        binary = binary(nativeLibraryDir, "gornsd"),
        // --config is the Reticulum configuration DIRECTORY, not the file inside it: the
        // daemon looks for "config" there and derives its instance lock from the same path.
        argv = listOf("--config", paths.rnsConfigDir),
        env = mapOf("HOME" to paths.runDir),
        logFile = "${paths.logDir}/gornsd.log",
    )

    /** gorrcd: the RRC hub. It attaches to the shared instance gornsd owns. */
    fun gorrcd(nativeLibraryDir: String, paths: StackPaths): LaunchSpec = LaunchSpec(
        name = "gorrcd",
        binary = binary(nativeLibraryDir, "gorrcd"),
        argv = listOf(
            "--config", paths.gorrcdConfig,
            "--configdir", paths.rnsClientConfigDir,
            // The hub announces once at startup by default and then never again, because
            // `announce_period_s` defaults to zero. A bot started a moment later therefore
            // never learns the hub's identity and can only report
            //     gorrcbot: hub "...": cannot connect: Hub identity unknown
            // for as long as it runs. A modest period makes the local pair converge within a
            // minute of anything being restarted, in either order, and also lets a remote peer
            // that was asleep find the hub without waiting for the next full announce cycle.
            "--announce-period", HUB_ANNOUNCE_PERIOD_SECONDS,
            // The hub must name the joiner in the JOINED it fans out to a room's existing
            // members, or none of them can learn who arrived. Without it a bot that is
            // already in a room never learns a peer that joins afterwards, so it has no
            // reply route and silently drops every direct request from that peer:
            //
            //	gorrcbot: no reply route for the direct request from <hash>
            //
            // The flag is off by default and the appliance's own hub is the only hub it
            // affects, so the appliance turns it on rather than changing the shared
            // default. The protocol shape itself is pinned by rrc/router-fanout_test.go to
            // match Python rrcd 0.3.2, and this is the supported way to ask for more.
            "--include-joined-member-list",
        ),
        env = mapOf(
            "HOME" to paths.runDir,
            "RRCD_HOME" to paths.gorrcdHome,
        ),
        logFile = "${paths.logDir}/gorrcd.log",
    )

    /**
     * gorrcbot: the bot that answers `/msg gobot`. It reads the same NMEA feed the
     * sensor service publishes, through the FIFOs rather than a socket, because
     * inside one application a FIFO is cheaper and the bot's reader already
     * understands one.
     */
    fun gorrcbot(nativeLibraryDir: String, paths: StackPaths): LaunchSpec = LaunchSpec(
        name = "gorrcbot",
        binary = binary(nativeLibraryDir, "gorrcbot"),
        argv = listOf(
            // --config is the Reticulum configuration DIRECTORY, and it is the client one:
            // the bot attaches to the transport's shared instance, and a directory that
            // requires a shared instance is what stops it from becoming the transport itself.
            "--config", paths.rnsClientConfigDir,
            "--bot-config", paths.gorrcbotConfig,
            "--home", paths.gorrcbotHome,
        ),
        env = mapOf(
            "HOME" to paths.runDir,
            "GORRCBOT_HOME" to paths.gorrcbotHome,
        ),
        logFile = "${paths.logDir}/gorrcbot.log",
    )

    /** The start order. gornsd must own the shared instance before anything attaches. */
    fun startOrder(): List<String> = listOf("gornsd", "gorrcd", "gorrcbot")
}
