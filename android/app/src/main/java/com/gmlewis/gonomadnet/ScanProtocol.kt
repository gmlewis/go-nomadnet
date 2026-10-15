// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify it under the
// terms of the GNU General Public License as published by the Free Software
// Foundation, either version 3 of the License, or (at your option) any later version.

package com.gmlewis.gonomadnet

/**
 * The camera scan protocol, the appliance's half of it.
 *
 * The client's dialogs offer to read a hex address from a QR code, but the client has no
 * camera: on Android it is a separate process, and a camera belongs to the app. So the
 * client asks the appliance to scan, over a loopback connection [ScanServer] listens on,
 * and the appliance answers with what the camera read.
 *
 * One connection carries one request. The client sends `SCAN <token>`, and the appliance
 * answers `ADDR <payload>` or `CANCEL`. The payload is the QR code's text exactly as the
 * camera read it, because it is the client that knows what an address looks like and
 * normalizing it there is what makes the rule testable. The token is what keeps another
 * application on the tablet from opening the camera through this listener.
 *
 * The Go half is `tui/scanner.go`, and both halves are asserted against one table of bytes
 * in [ScanProtocolTest] and `scanner_test.go`: a disagreement between them shows up to the
 * reader as a scan button that does nothing at all.
 */
object ScanProtocol {

    /** The verb beginning the one request line. */
    const val REQUEST_VERB: String = "SCAN"

    /** The verb beginning a reply that carries a payload. */
    const val REPLY_VERB: String = "ADDR"

    /** The whole of a reply to a reader who backed out of the camera. */
    const val CANCEL_REPLY: String = "CANCEL"

    /**
     * The longest request line the appliance will read.
     *
     * A request is one verb and a 32-character token; anything longer is not a request, and
     * bounding the read is what stops a peer that never sends a newline from holding a
     * server thread for as long as it likes.
     */
    const val MAX_LINE: Int = 1024

    /** requestLine is the line that asks for one scan, carrying the token. */
    fun requestLine(token: String): String = "$REQUEST_VERB $token"

    /** replyLine is the line that answers with what the camera read. */
    fun replyLine(payload: String): String = "$REPLY_VERB $payload"

    /**
     * parseRequest returns the token a request line carries, or null when the line is not a
     * request.
     *
     * A line that is not exactly the verb and one non-empty token is refused rather than
     * guessed at: refusing it costs one closed connection, and accepting it would open a
     * camera for whatever happened to connect.
     */
    fun parseRequest(line: String): String? {
        val parts = line.split(' ')
        if (parts.size != 2 || parts[0] != REQUEST_VERB) {
            return null
        }
        return parts[1].ifEmpty { null }
    }

    /**
     * acceptsToken reports whether a request may open the camera.
     *
     * The comparison is over the whole token, and a blank token matches nothing: a launch
     * that somehow arrived with no token must not be a launch that dropped the guard.
     */
    fun acceptsToken(expected: String, offered: String?): Boolean =
        expected.isNotEmpty() && offered != null && constantTimeEquals(expected, offered)

    /**
     * constantTimeEquals compares two strings without leaking where they first differ.
     *
     * The listener is on the loopback interface, so the timing a comparison leaks is not the
     * whole of an attacker's information — but this guards a camera, the comparison costs
     * nothing, and a guard whose strength depends on how busy the device is has a strength
     * nobody can state.
     */
    fun constantTimeEquals(a: String, b: String): Boolean {
        if (a.length != b.length) {
            return false
        }
        var difference = 0
        for (i in a.indices) {
            difference = difference or (a[i].code xor b[i].code)
        }
        return difference == 0
    }
}

/**
 * ScanRequester presents the scanner and returns what it read.
 *
 * It is an interface with two implementations: [ScanActivity] presenting the camera on a
 * device, and a fake in the tests. Everything about the protocol above is then asserted
 * without a camera, and the part that needs hardware is a screen that either shows a
 * picture or does not.
 */
interface ScanRequester {

    /**
     * scan presents the scanner and returns the text of the first QR code read, or null if
     * the reader backed out.
     *
     * It is called on a server thread rather than the UI thread, and it is expected to block
     * until the reader is done — the connection it is answering is waiting for exactly that.
     */
    fun scan(): String?
}
