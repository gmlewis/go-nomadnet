// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import android.content.Context

/**
 * Where the appliance remembers the hub it should connect to.
 *
 * The appliance cannot read Termux's `~/.reticulum/config` — another application's private
 * data — so the one interface it needs is a setting of its own rather than something it
 * copies out of the client's configuration. That is a deliberate duplication of a single
 * host and port, not of the configuration as a whole: the appliance owns the transport, and
 * the client owns none.
 */
class ApplianceSettings(context: Context) {
    private val preferences = context.getSharedPreferences(FILE, Context.MODE_PRIVATE)

    /** The hub as `host:port`, exactly as the operator typed it. */
    var hubSpec: String
        get() = preferences.getString(KEY_HUB_SPEC, DEFAULT_HUB_SPEC)?.takeIf { it.isNotBlank() }
            ?: DEFAULT_HUB_SPEC
        set(value) {
            preferences.edit().putString(KEY_HUB_SPEC, value.trim()).apply()
        }

    /**
     * Which device axis the heading is measured along.
     *
     * The two references degenerate in opposite mountings, so which one is right depends
     * entirely on how the tablet is being held: the screen-back normal is well conditioned on
     * a stand and useless lying flat, and the screen-top axis is the other way round. It is a
     * setting for that reason, and it persists, because a tablet that lives on a stand should
     * not have to be told twice.
     */
    var headingAxis: HeadingAxis
        get() {
            val stored = preferences.getString(KEY_HEADING_AXIS, null) ?: return HeadingAxis.SCREEN_BACK
            return runCatching { HeadingAxis.valueOf(stored) }.getOrDefault(HeadingAxis.SCREEN_BACK)
        }
        set(value) {
            preferences.edit().putString(KEY_HEADING_AXIS, value.name).apply()
        }

    /**
     * The key the transport's RPC listener authenticates its clients with.
     *
     * Python derives this key from the transport identity in the *same storage directory* the
     * client runs from (Reticulum.py:355-356), and the ordinary setup is one `~/.reticulum`
     * shared by every process on the machine. The appliance cannot share one: the client must
     * have a configuration of its own, or it races the transport for ownership of the shared
     * instance and one of them quietly becomes the other. With separate directories the two
     * identities differ, and every RPC a client makes is answered "unauthorized" — an
     * Interfaces page showing an appliance that is connected as though it were not.
     *
     * So both sides are told the key instead, which is the supported way to run them apart
     * (`[reticulum] rpc_key`, Reticulum.py:494-500). It is generated once per install and
     * persists: a key that changed on every start would be a transport whose clients are all
     * rejected after every restart.
     */
    val rpcKey: String
        get() {
            preferences.getString(KEY_RPC_KEY, null)?.takeIf { it.isNotBlank() }?.let { return it }
            val generated = generateRpcKey()
            preferences.edit().putString(KEY_RPC_KEY, generated).apply()
            return generated
        }

    companion object {
        /**
         * The home hub this project runs against, published with both an A and an AAAA
         * record. Only its IPv6 address answers on 4242, which is exactly why the appliance
         * probes rather than trusting the first answer.
         */
        const val DEFAULT_HUB_SPEC = "go-nomadnet.duckdns.org:4242"

        const val FILE = "gonomadnet-appliance"
        const val KEY_HUB_SPEC = "hub_spec"
        const val KEY_HEADING_AXIS = "heading_axis"
        const val KEY_RPC_KEY = "rpc_key"

        /** How many bytes the RPC key is, and therefore how long its hex form is. */
        const val RPC_KEY_BYTES = 32

        /** generateRpcKey mints one key as lowercase hex, which is what the config takes. */
        fun generateRpcKey(random: java.security.SecureRandom = java.security.SecureRandom()): String {
            val bytes = ByteArray(RPC_KEY_BYTES)
            random.nextBytes(bytes)
            return bytes.joinToString("") { "%02x".format(it) }
        }
    }
}

/** Builds the transport's configuration from what the operator asked for. */
object ApplianceConfig {

    /**
     * Renders the transport's configuration for a `host:port` specification.
     *
     * The resolution happens here, at rendering time, on every start: a DuckDNS address
     * changes, and a literal pinned once at install time would be a node that connects until
     * the day it silently does not. A specification that cannot be parsed, or a name that
     * resolves to nothing, produces a transport with **no interfaces at all** and a reason in
     * [describeFailure] — an isolated node is a working node with no peers, which is far
     * better than a node that refuses to start.
     *
     * [rnode] is the radio's section, and it defaults to one switched on. The radio is not
     * conditional on the hub — an appliance whose hub address resolved nothing still has a
     * radio — and it is not conditional on a radio being attached either: the appliance
     * assumes one might be, so a fresh install carries the interface from its first start.
     * The caller passes the path the appliance's own bridge published, when it has one, so
     * that a radio that *is* attached is reached rather than merely waited for.
     */
    fun nodeConfig(
        hubSpec: String,
        resolve: (String, Int) -> DialableAddress? = { host, port -> Resolver.resolveDialable(host, port) },
        sharedInstancePort: Int = NodeConfigSpec.DEFAULT_SHARED_INSTANCE_PORT,
        instanceName: String = NodeConfigSpec.DEFAULT_INSTANCE_NAME,
        rpcKey: String = "",
        rnode: RNodeSpec? = RNodeSpec(),
    ): NodeConfigSpec {
        val parsed = Resolver.parseHostPort(hubSpec)
            ?: return NodeConfigSpec(
                sharedInstancePort = sharedInstancePort,
                instanceName = instanceName,
                rpcKey = rpcKey,
                rnode = rnode,
            )
        val resolved = resolve(parsed.first, parsed.second)
            ?: return NodeConfigSpec(
                sharedInstancePort = sharedInstancePort,
                instanceName = instanceName,
                rpcKey = rpcKey,
                rnode = rnode,
            )
        return NodeConfigSpec(
            sharedInstancePort = sharedInstancePort,
            instanceName = instanceName,
            rpcKey = rpcKey,
            rnode = rnode,
            interfaces = listOf(
                InterfaceSpec(name = HUB_INTERFACE_NAME, host = resolved.literal, port = parsed.second),
            ),
        )
    }

    /** Explains what the transport will do, in the operator's terms. */
    fun describeFailure(hubSpec: String): String? = when (Resolver.parseHostPort(hubSpec)) {
        null -> "the hub address \"$hubSpec\" is not a host:port, so the transport has no interface"
        else -> null
    }

    /** Explains which address was chosen, and whether that choice was proved. */
    fun describeChoice(hubSpec: String, resolved: DialableAddress?, port: Int): String = when {
        resolved == null -> "could not resolve $hubSpec"
        resolved.probed -> "the hub resolves to ${resolved.literal} (${Resolver.familyOf(resolved.literal)}), and it answered on $port"
        else -> "the hub resolves to ${resolved.literal} (${Resolver.familyOf(resolved.literal)}), which did not answer on " +
            "$port; using it anyway"
    }

    /** The interface's name, which shows up in Reticulum's own diagnostics. */
    const val HUB_INTERFACE_NAME = "Home Hub"
}
