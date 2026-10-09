// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import java.security.SecureRandom

/**
 * The console socket's name and the rule that decides who may connect to it.
 *
 * An abstract Unix socket has no filesystem entry and no permissions, so anything on the
 * device that knows the name can attach: this socket carries the keystrokes of a terminal
 * whose other end is a Reticulum node. The name is therefore random, and the peer is
 * checked as well — see [peerIsAccepted].
 *
 * The Go half builds and checks the same name with `console.ConsoleSocketName` and
 * `console.CheckPeerUID`, and the host is given this name on its command line, so both
 * halves are asserted against the same rules: `sockaddr_test.go` and this file's test.
 */
object ConsoleSocket {

    /** The prefix of every console socket name. */
    const val PREFIX: String = "gonomadnet-console-"

    /** How much randomness the name carries. */
    const val ENTROPY_BYTES: Int = 16

    /** randomEntropy returns [ENTROPY_BYTES] of cryptographically strong randomness. */
    fun randomEntropy(): ByteArray {
        val entropy = ByteArray(ENTROPY_BYTES)
        SecureRandom().nextBytes(entropy)
        return entropy
    }

    /** name returns the abstract-socket name for a session, from the app's pid and its entropy. */
    fun name(pid: Int, entropy: ByteArray): String =
        PREFIX + pid.toString() + "-" + entropy.toHex()

    /**
     * peerIsAccepted reports whether a connecting peer may use the console.
     *
     * The host runs as this app's own uid, and nothing else on the tablet has any business
     * writing to the appliance's terminal, so any other uid is refused.
     */
    fun peerIsAccepted(ourUid: Int, peerUid: Int): Boolean = ourUid == peerUid

    /** refusalReason names both uids, so the log alone is enough to diagnose a refusal. */
    fun refusalReason(ourUid: Int, peerUid: Int): String =
        "refusing a console connection from uid $peerUid: this app is uid $ourUid"

    /** toHex renders bytes as lower-case hex, which is what the Go half does. */
    private fun ByteArray.toHex(): String {
        val digits = "0123456789abcdef"
        val out = StringBuilder(size * 2)
        for (byte in this) {
            val value = byte.toInt() and 0xff
            out.append(digits[value ushr 4]).append(digits[value and 0x0f])
        }
        return out.toString()
    }
}
