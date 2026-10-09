// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

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
