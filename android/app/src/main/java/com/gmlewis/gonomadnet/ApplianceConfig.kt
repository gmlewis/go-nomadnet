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
     */
    fun nodeConfig(
        hubSpec: String,
        resolve: (String, Int) -> DialableAddress? = { host, port -> Resolver.resolveDialable(host, port) },
        sharedInstancePort: Int = NodeConfigSpec.DEFAULT_SHARED_INSTANCE_PORT,
        instanceName: String = NodeConfigSpec.DEFAULT_INSTANCE_NAME,
    ): NodeConfigSpec {
        val parsed = Resolver.parseHostPort(hubSpec)
            ?: return NodeConfigSpec(sharedInstancePort = sharedInstancePort, instanceName = instanceName)
        val resolved = resolve(parsed.first, parsed.second)
            ?: return NodeConfigSpec(sharedInstancePort = sharedInstancePort, instanceName = instanceName)
        return NodeConfigSpec(
            sharedInstancePort = sharedInstancePort,
            instanceName = instanceName,
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
