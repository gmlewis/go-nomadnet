// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

/**
 * One Reticulum interface the appliance's own transport should dial.
 *
 * [host] must be a literal address, never a name. Android has no
 * `/etc/resolv.conf`, and a `CGO_ENABLED=0` Go binary therefore falls back to
 * asking `127.0.0.1:53` and `[::1]:53`, which are not running:
 *
 * ```
 * dial tcp: lookup <host> on [::1]:53: read udp ...: read: connection refused
 * ```
 *
 * Java *can* resolve, so the appliance resolves every host name it is given and
 * writes the literal address into the daemon's configuration before spawning it.
 * [NodeConfigRenderer] refuses to render a host name at all, so a resolver that
 * silently failed cannot produce a configuration that looks fine and never
 * connects.
 */
data class InterfaceSpec(
    val name: String,
    val host: String,
    val port: Int,
    val enabled: Boolean = true,
)

/**
 * The transport's configuration.
 *
 * [sharedInstancePort] and [instanceName] are the contract between this app and
 * anything that attaches to its transport, so they are part of the rendered
 * configuration rather than a hidden default. Termux's `~/.reticulum-stack/config`
 * must name exactly the same pair, and `scripts/build-android-apk.sh` compares the
 * two because a silent mismatch produces "no shared instance is running" and
 * nothing else.
 */
data class NodeConfigSpec(
    val sharedInstancePort: Int = DEFAULT_SHARED_INSTANCE_PORT,
    val instanceName: String = DEFAULT_INSTANCE_NAME,
    /**
     * The key the transport's RPC listener authenticates its clients with, as hex.
     *
     * It is stated by the appliance rather than derived from an identity because the two
     * sides run from different directories and would derive different keys from them. See
     * [ApplianceSettings.rpcKey].
     */
    val rpcKey: String = "",
    val interfaces: List<InterfaceSpec> = emptyList(),
    val logLevel: Int = 4,
    val enableTransport: Boolean = true,
) {
    companion object {
        /** The loopback port the appliance's transport listens on for local clients. */
        const val DEFAULT_SHARED_INSTANCE_PORT = 37428

        /** The shared instance's name; both sides must agree on it. */
        const val DEFAULT_INSTANCE_NAME = "default"
    }
}

/**
 * The bot's configuration.
 *
 * [hubDestination] is the `rrc.hub` destination hash of the local hub, which the
 * appliance derives from the hub's own identity at start time rather than
 * shipping a name that would have to be resolved.
 *
 * [kjvTxtFile] is where the Bible text the `kjv` command reads was installed. An
 * empty value leaves the key out entirely, which is the bot's own way of saying the
 * command is not configured; naming a path that is not there would instead make the
 * bot report the file as unreadable for the life of the install.
 */
data class BotConfigSpec(
    val gpsPort: String,
    val compassPort: String,
    val kjvTxtFile: String = "",
    val hubName: String = "gonomadnet local hub",
    val hubDestination: String,
    val rooms: List<String> = listOf("general"),
    val nick: String = "gobot",
    val respondTo: String = "gobot",
)

/** Renders the configuration files the daemons are started with. */
object NodeConfigRenderer {

    /**
     * Renders the transport's configuration.
     *
     * `shared_instance_type = tcp` is not a preference: on Linux the default shared instance
     * is an abstract Unix socket, and two different applications cannot be assumed to reach
     * each other's abstract sockets because their SELinux categories differ. A loopback TCP
     * socket needs nothing but the INTERNET permission, which removes the question.
     *
     * **Nothing in this file is quoted.** Reticulum's configuration is INI-like and its
     * parser keeps quotation marks as part of the value, so a quoted address is dialled as
     * the *name* `"203.0.113.7"` — with the quotation marks in it — giving:
     *
     * ```
     * Go TCPClientInterface "Home Hub" connect failed: dial tcp: lookup "203.0.113.7": no such host
     * ```
     *
     * where the interface name has kept its quotes too. A name with spaces is written bare,
     * as the shipped configuration does, and a value that genuinely needs a comma or a
     * bracket is not representable here at all — which is a reason to reject it rather than
     * to quote it.
     */
    fun reticulumConfig(spec: NodeConfigSpec): String {
        val out = StringBuilder()
        out.append("# Rendered by the gonomadnet appliance. Do not edit by hand.\n")
        out.append("[reticulum]\n")
        out.append("  enable_transport = ").append(if (spec.enableTransport) "yes" else "no").append('\n')
        out.append("  share_instance = yes\n")
        out.append("  shared_instance_type = tcp\n")
        out.append("  shared_instance_port = ").append(spec.sharedInstancePort).append('\n')
        out.append("  instance_name = ").append(spec.instanceName).append('\n')
        out.append("  # The appliance owns the shared instance, so it must never be the\n")
        out.append("  # one that gives up quietly when something else already holds it.\n")
        out.append("  panic_on_interface_error = no\n")
        out.append(rpcKeyLine(spec))
        out.append('\n')
        out.append("[logging]\n")
        out.append("  loglevel = ").append(spec.logLevel).append('\n')
        out.append('\n')
        out.append(renderInterfaces(spec))
        return out.toString()
    }

    /**
     * rpcKeyLine renders the transport's RPC key, or nothing when the appliance has none.
     *
     * Both configuration files carry the same key: the instance's listener authenticates
     * against it, and every client presents it. Without it the two would each derive a key
     * from the identity in their own storage directory, which are different identities here
     * (see [ApplianceSettings.rpcKey]), and every RPC would be refused.
     */
    private fun rpcKeyLine(spec: NodeConfigSpec): String =
        if (spec.rpcKey.isBlank()) "" else "  rpc_key = " + spec.rpcKey.trim() + "\n"

    /**
     * Renders the `[interfaces]` section: the transport's own, and the same one again for a
     * client.
     *
     * A client renders them because its own interface list is a *page* — the client's
     * Interfaces display reads this file and shows what it finds there — and a section that
     * said "deliberately empty" produced an appliance whose client reported no interfaces at
     * all while its transport was dialling one. The interfaces are rendered once, here, so
     * the page and the wire cannot disagree about what this appliance connects through.
     *
     * Listing them does not make the client dial them: an instance that attaches to a shared
     * instance starts none of its own (an attached client's interfaces are owned by the
     * instance it attached to), and the client's `require_shared_instance = yes` is what
     * keeps it attaching rather than becoming one.
     */
    private fun renderInterfaces(spec: NodeConfigSpec): String {
        val out = StringBuilder()
        out.append("[interfaces]\n")
        if (spec.interfaces.isEmpty()) {
            out.append("  # Deliberately empty: the appliance resolved no interface to connect through.\n")
        }
        for (iface in spec.interfaces) {
            val host = literalAddress(iface.host)
                ?: throw IllegalArgumentException(
                    "interface ${iface.name} names the host ${iface.host}, which is not a " +
                        "literal address; resolve it in Java and pass the address, because " +
                        "a Go binary on Android cannot resolve names",
                )
            out.append("  [[").append(iface.name).append("]]\n")
            out.append("    type = TCPClientInterface\n")
            out.append("    interface_enabled = ").append(if (iface.enabled) "true" else "false").append('\n')
            out.append("    target_host = ").append(host).append('\n')
            out.append("    target_port = ").append(iface.port).append('\n')
        }
        return out.toString()
    }

    /**
     * Renders the client-side configuration the hub and the bot attach through.
     *
     * It says `require_shared_instance = yes`, because a client that merely *prefers* the
     * shared instance tries to become the owner first and only falls back to attaching when
     * the bind fails. Two clients racing for ownership of the transport's shared instance is
     * a bug that shows up as one of them mysteriously being the transport, so the race is
     * removed rather than made unlikely.
     *
     * It lists the same interfaces the transport does, and they are the same list for the
     * same reason: they are what this appliance connects through. The client owns none of
     * them — the shared instance does — but the client's Interfaces page is a view of this
     * file, and a file that said "deliberately empty" showed an operator an appliance with no
     * interfaces at all while its transport was dialling the hub.
     */
    fun clientReticulumConfig(spec: NodeConfigSpec): String {
        val out = StringBuilder()
        out.append("# Rendered by the gonomadnet appliance. Do not edit by hand.\n")
        out.append("# A client of the appliance's own transport: it owns no interface and it\n")
        out.append("# must never take the shared instance, only attach to it.\n")
        out.append("[reticulum]\n")
        out.append("  enable_transport = no\n")
        out.append("  share_instance = yes\n")
        out.append("  shared_instance_type = tcp\n")
        out.append("  shared_instance_port = ").append(spec.sharedInstancePort).append('\n')
        out.append("  instance_name = ").append(spec.instanceName).append('\n')
        out.append("  require_shared_instance = yes\n")
        out.append(rpcKeyLine(spec))
        out.append('\n')
        out.append("[logging]\n")
        out.append("  loglevel = ").append(spec.logLevel).append('\n')
        out.append('\n')
        out.append(renderInterfaces(spec))
        return out.toString()
    }

    /**
     * Renders the bot's configuration.
     *
     * The two sensor ports point at FIFOs under the app's private directory. A bot
     * that reads a FIFO has to find its writer already holding the other end, which
     * the sensor service arranges before anything is spawned; opening the read end
     * of a FIFO with no writer blocks forever, with no error message at all.
     */
    fun gorrcbotConfig(spec: BotConfigSpec): String {
        val out = StringBuilder()
        out.append("# Rendered by the gonomadnet appliance. Do not edit by hand.\n")
        out.append("[bot]\n")
        out.append("  nick = ").append(escapeTomlString(spec.nick)).append('\n')
        out.append('\n')
        out.append("# The live sensor streams the appliance publishes. They are named\n")
        out.append("# exactly as the Nomad Network client's [location] section names them.\n")
        out.append("gps_port = ").append(escapeTomlString(spec.gpsPort)).append('\n')
        out.append("compass_port = ").append(escapeTomlString(spec.compassPort)).append('\n')
        if (spec.kjvTxtFile.isNotBlank()) {
            out.append('\n')
            out.append("# The Bible text the kjv command reads, installed from the APK's\n")
            out.append("# assets on every start because it is far too large to live in a\n")
            out.append("# configuration file.\n")
            out.append("kjv_txt_file = ").append(escapeTomlString(spec.kjvTxtFile)).append('\n')
        }
        out.append('\n')
        out.append("[[hubs]]\n")
        out.append("name = ").append(escapeTomlString(spec.hubName)).append('\n')
        out.append("destination = ").append(escapeTomlString(spec.hubDestination)).append('\n')
        out.append("rooms = [")
        out.append(spec.rooms.joinToString(", ") { escapeTomlString(it) })
        out.append("]\n")
        out.append("nick = ").append(escapeTomlString(spec.nick)).append('\n')
        out.append("respond_to = { ")
        out.append(spec.rooms.joinToString(", ") { "${escapeTomlString(it)} = ${escapeTomlString(spec.respondTo)}" })
        out.append(" }\n")
        return out.toString()
    }

    /**
     * Returns the address when it is already a literal, and null when it is a name.
     *
     * IPv4 is four dotted decimal octets; IPv6 is anything with a colon in it that
     * is not an IPv4 address, which is generous and deliberate — the point is to
     * refuse names, and a malformed IPv6 literal is refused by the dialer with a
     * message of its own.
     */
    fun literalAddress(host: String): String? {
        val trimmed = host.trim()
        if (trimmed.isEmpty()) {
            return null
        }
        if (trimmed.count { it == ':' } >= 2) {
            return trimmed.removeSurrounding("[", "]")
        }
        val octets = trimmed.split('.')
        if (octets.size != 4) {
            return null
        }
        for (octet in octets) {
            if (octet.isEmpty() || octet.length > 3 || !octet.all { it.isDigit() }) {
                return null
            }
            if (octet.toInt() > 255) {
                return null
            }
        }
        return trimmed
    }

    /** Quotes a TOML string, which is what every value here is. */
    fun escapeTomlString(value: String): String =
        "\"" + value.replace("\\", "\\\\").replace("\"", "\\\"") + "\""
}
