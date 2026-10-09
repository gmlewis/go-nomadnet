// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import java.net.Inet4Address
import java.net.Inet6Address
import java.net.InetAddress
import java.net.InetSocketAddress
import java.net.Socket

/** A literal address a bundled Go binary can be told to dial, and how it was chosen. */
data class DialableAddress(
    /** The literal address, without brackets. */
    val literal: String,
    /** True when it is an IPv6 address. */
    val isIPv6: Boolean,
    /**
     * True when a connection to this address and port actually completed.
     *
     * It is not an error for this to be false: the peer may be down, or the probe may have
     * been refused, and a node that refused to be configured until its peer answered would
     * never come up at all. It is recorded so the operator can be told which address was
     * chosen blind.
     */
    val probed: Boolean,
)

/**
 * Turns a host name into a literal address, in Java, because the Go daemons cannot.
 *
 * Android has no `/etc/resolv.conf`: `/etc` is a read-only symlink to `/system/etc`, and
 * Android's resolver is reachable only by bionic over `netd`'s Unix socket. A
 * `CGO_ENABLED=0` Go binary falls back to asking `127.0.0.1:53` and `[::1]:53`, neither of
 * which is running, and every lookup fails with:
 *
 * ```
 * dial tcp: lookup <host> on [::1]:53: read udp ...: read: connection refused
 * ```
 *
 * Java can resolve, because Java on Android goes through bionic. So the appliance resolves
 * every name it is given here and writes the literal into the daemon's configuration before
 * spawning it. [NodeConfigRenderer] refuses a name outright, so a resolution that silently
 * failed cannot produce a configuration that looks correct and never connects.
 *
 * A name can resolve to several addresses, and they are not interchangeable: on this
 * project's own hub, port 4242 is unreachable over IPv4 and reachable over IPv6. Picking the
 * first answer is therefore a coin toss, so each candidate is probed and the one that
 * actually accepts a connection is preferred.
 */
object Resolver {

    /** How long each candidate address is given to accept a connection, in milliseconds. */
    const val DEFAULT_PROBE_TIMEOUT_MS = 3_000

    /** Resolves a host name to every literal address the platform offers, in its own order. */
    fun resolveAll(host: String): List<String> {
        val trimmed = host.trim().removeSurrounding("[", "]")
        if (isLiteral(trimmed)) {
            return listOf(trimmed)
        }
        return try {
            InetAddress.getAllByName(trimmed).map { it.hostAddress ?: "" }.filter { it.isNotEmpty() }
        } catch (failure: Throwable) {
            // The reason is kept rather than swallowed. A name that does not resolve leaves
            // the transport with no interface, and "the node is isolated" is impossible to
            // diagnose without knowing whether the lookup failed, timed out, or was refused
            // by the platform.
            lastFailure = "${failure.javaClass.name}: ${failure.message}"
            emptyList()
        }
    }

    /**
     * Why the most recent resolution attempt failed, or null.
     *
     * Resolution and probing are network operations, and on Android they throw
     * `NetworkOnMainThreadException` if they are run on the UI thread — which is the first
     * thing to check when every name suddenly fails to resolve at once.
     */
    @Volatile
    var lastFailure: String? = null
        private set

    /**
     * Reports whether a connection to the address and port completes.
     *
     * A name having an A record says nothing about whether anything is listening on the far
     * end of it, so a completed connection is the only way to choose between two addresses
     * for the same name.
     */
    fun probe(literal: String, port: Int, timeoutMs: Int = DEFAULT_PROBE_TIMEOUT_MS): Boolean = try {
        Socket().use { socket ->
            socket.connect(InetSocketAddress(literal.removeSurrounding("[", "]"), port), timeoutMs)
            true
        }
    } catch (failure: Throwable) {
        false
    }

    /**
     * Chooses the address to write into a daemon's configuration.
     *
     * Every candidate is probed and the first that accepts a connection wins. When none
     * does, the preference falls to IPv6, because on Android a name that has both records
     * is far more likely to be reachable over IPv6 — carriers and home routers hand out
     * global IPv6 addresses more readily than they forward ports — and `probed` says the
     * choice was made blind so the appliance can tell the operator.
     *
     * Returns null only when the name resolves to nothing at all.
     */
    fun resolveDialable(
        host: String,
        port: Int,
        timeoutMs: Int = DEFAULT_PROBE_TIMEOUT_MS,
        probe: (String, Int, Int) -> Boolean = ::probe,
    ): DialableAddress? {
        lastFailure = null
        val candidates = resolveAll(host)
        if (candidates.isEmpty()) {
            return null
        }
        for (candidate in candidates) {
            if (probe(candidate, port, timeoutMs)) {
                return DialableAddress(candidate, isIPv6Literal(candidate), probed = true)
            }
        }
        val preferred = candidates.firstOrNull { isIPv6Literal(it) } ?: candidates.first()
        return DialableAddress(preferred, isIPv6Literal(preferred), probed = false)
    }

    /**
     * Splits a `host:port` specification.
     *
     * An IPv6 address has to be bracketed — `[2001:db8::1]:4242` — because otherwise the
     * colons in the address and the colon before the port cannot be told apart. This is the
     * same requirement the dialer has, and the same bug it produced when a site joined a
     * host and a port with a bare colon: `too many colons in address`.
     */
    fun parseHostPort(spec: String): Pair<String, Int>? {
        val trimmed = spec.trim()
        if (trimmed.isEmpty()) {
            return null
        }
        if (trimmed.startsWith("[")) {
            val closing = trimmed.indexOf(']')
            if (closing < 0 || closing + 1 >= trimmed.length || trimmed[closing + 1] != ':') {
                return null
            }
            val port = trimmed.substring(closing + 2).toIntOrNull() ?: return null
            return if (validPort(port)) trimmed.substring(1, closing) to port else null
        }
        val colon = trimmed.lastIndexOf(':')
        if (colon <= 0) {
            return null
        }
        val host = trimmed.substring(0, colon)
        // An unbracketed IPv6 address is refused rather than guessed at. "2001:db8::1:4242"
        // could be read as the address 2001:db8::1 with port 4242, or as the address
        // 2001:db8::1:4242 with no port at all, and the same parser would read a bare
        // "2001:db8::1" as the address "2001:db8:" with port 1. Brackets remove the
        // question, and the dialer wants them anyway.
        if (host.contains(':')) {
            return null
        }
        val port = trimmed.substring(colon + 1).toIntOrNull() ?: return null
        if (!validPort(port)) {
            return null
        }
        return host to port
    }

    /** Reports whether a host string is already a literal rather than a name. */
    fun isLiteral(host: String): Boolean =
        NodeConfigRenderer.literalAddress(host) != null

    /** Reports whether a literal is IPv6. */
    fun isIPv6Literal(literal: String): Boolean {
        val bare = literal.removeSurrounding("[", "]")
        return bare.count { it == ':' } >= 2
    }

    /** Parses an address into its family, for a diagnostic. */
    fun familyOf(literal: String): String = when {
        isIPv6Literal(literal) -> "IPv6"
        else -> "IPv4"
    }

    private fun validPort(port: Int): Boolean = port in 1..65535

    /** The InetAddress form of a literal, or null. */
    internal fun addressOf(literal: String): InetAddress? = try {
        InetAddress.getByName(literal.removeSurrounding("[", "]"))
    } catch (failure: Throwable) {
        null
    }

    /** Reports whether a literal is IPv4, which is only used to label diagnostics. */
    internal fun isIPv4(literal: String): Boolean = addressOf(literal) is Inet4Address

    /** Reports whether a literal is IPv6, checked against the platform rather than by eye. */
    internal fun isIPv6(literal: String): Boolean = addressOf(literal) is Inet6Address
}
