// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import android.net.LocalServerSocket
import android.net.LocalSocket
import java.io.IOException
import java.io.InputStream
import java.io.OutputStream

/**
 * The console socket as the platform provides it.
 *
 * Android's own local socket is an abstract Unix socket: `LocalSocketAddress` defaults to
 * the abstract namespace, and the name is carried in `sun_path` with a leading NUL, which
 * is the same address the Go host dials when it is given `@name`. An abstract socket has no
 * filesystem entry and no permissions, so nothing on the device can be used to keep another
 * app out: the name is random ([ConsoleSocket]) and the peer is checked by uid
 * ([ConsoleSocket.peerIsAccepted]) instead.
 *
 * This is the one part of the console that no unit test can reach — Android's JVM tests
 * have no sockets to bind — so it is deliberately thin: everything it does beyond opening
 * the socket and reading the peer's credentials is in [ConsoleSession], which is tested
 * against a fake. What is left here is exercised on the tablet, which is where an interop
 * mistake would show up as a console that never connects.
 */
object LocalConsole : ConsoleListenerFactory {

    /** bind listens on the abstract socket [name]. */
    override fun bind(name: String): ConsoleListener = LocalListener(LocalServerSocket(name))
}

/** A listener on a real abstract socket. */
private class LocalListener(private val server: LocalServerSocket) : ConsoleListener {

    /**
     * accept waits for the next peer.
     *
     * A closed server socket reports an IOException rather than a null, and that is how this
     * listener ends: it was closed by the session, which has already decided the session is
     * over. So the two are the same answer here, and the session reports one end rather than
     * an error about a socket it closed itself.
     */
    override fun accept(): ConsoleChannel? = try {
        LocalChannel(server.accept())
    } catch (e: IOException) {
        null
    }

    override fun close() {
        try {
            server.close()
        } catch (e: IOException) {
            // Closing a socket that is already closed is not a failure, and there is
            // nothing left to report it to: the session is over either way.
        }
    }
}

/** One accepted peer, on a real socket. */
private class LocalChannel(private val socket: LocalSocket) : ConsoleChannel {

    override fun input(): InputStream = socket.inputStream

    override fun output(): OutputStream = socket.outputStream

    /**
     * peerUid is the uid of the process on the other end.
     *
     * Credentials that cannot be read report [NO_UID], which is no process's uid and so is
     * refused: a peer that cannot be identified is not one this app may trust with its
     * terminal.
     */
    override fun peerUid(): Int = socket.peerCredentials?.uid ?: NO_UID

    override fun close() {
        try {
            socket.close()
        } catch (e: IOException) {
            // As above: the connection is going away regardless.
        }
    }

    private companion object {
        /** NO_UID is no process's uid, so it matches nothing and is always refused. */
        const val NO_UID = -1
    }
}
