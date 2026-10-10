// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import java.security.SecureRandom

/**
 * The radio bridge's socket name.
 *
 * The bridge binds it and this app dials it, so the two halves have to arrive at the same
 * name from the same two inputs: the app's pid and a random value the app generates. It
 * follows [ConsoleSocket] exactly — same shape, same length of entropy, same reason — but
 * with a prefix of its own, because a socket named after the console that turned out to
 * carry a radio's transmit stream is a name that sends whoever reads the log looking in the
 * wrong place.
 *
 * The name is random because an abstract Unix socket has no filesystem entry and no
 * permissions: anything on the device that knows the name can connect to it, and what this
 * one reaches is the serial stream of a transmitter. The Go half builds and checks the same
 * name with `rnode.SocketName`, and both halves are asserted against the literal below.
 */
object RNodeSocket {

    /** The prefix of every radio bridge socket name. */
    const val PREFIX: String = "gonomadnet-rnode-"

    /** How much randomness the name carries. */
    const val ENTROPY_BYTES: Int = 16

    /** randomEntropy returns [ENTROPY_BYTES] of cryptographically strong randomness. */
    fun randomEntropy(): ByteArray {
        val entropy = ByteArray(ENTROPY_BYTES)
        SecureRandom().nextBytes(entropy)
        return entropy
    }

    /** name returns the abstract-socket name for a bridge, from the app's pid and its entropy. */
    fun name(pid: Int, entropy: ByteArray): String =
        PREFIX + pid.toString() + "-" + entropy.toHex()

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
