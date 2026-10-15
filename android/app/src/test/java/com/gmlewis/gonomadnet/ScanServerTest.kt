// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify it under the
// terms of the GNU General Public License as published by the Free Software
// Foundation, either version 3 of the License, or (at your option) any later version.

package com.gmlewis.gonomadnet

import java.io.BufferedReader
import java.io.InputStreamReader
import java.net.InetAddress
import java.net.Socket
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The scan listener, driven the way the Go client drives it.
 *
 * Every case here opens a real connection to a real loopback listener and speaks the real
 * protocol; only the camera is a fake. That is deliberate: the framing, the token check and
 * the reply are the parts where a mistake is silent, and they are also the parts that can be
 * asserted without a device.
 */
class ScanServerTest {

    /** fakeRequester stands in for the camera, and counts the scans it was asked for. */
    private class fakeRequester(private val payload: String?) : ScanRequester {
        var calls = 0
            private set

        override fun scan(): String? {
            calls++
            return payload
        }
    }

    /** withServer runs body against a started server and always closes it. */
    private fun withServer(requester: ScanRequester, body: (ScanServer) -> Unit) {
        val server = ScanServer(requester)
        server.start()
        try {
            body(server)
        } finally {
            server.close()
        }
    }

    /** ask sends one request line and returns the reply, or null when the peer closed. */
    private fun ask(server: ScanServer, line: String): String? {
        Socket(InetAddress.getLoopbackAddress(), server.port).use { socket ->
            val out = socket.getOutputStream()
            out.write((line + "\n").toByteArray(Charsets.UTF_8))
            out.flush()
            return BufferedReader(InputStreamReader(socket.getInputStream(), Charsets.UTF_8)).readLine()
        }
    }

    @Test
    fun `a request carrying the token is answered with what the camera read`() {
        val requester = fakeRequester("lxmf@2a6105f57145860441a62fe3b2a1352c")
        withServer(requester) { server ->
            val reply = ask(server, ScanProtocol.requestLine(server.token))
            assertEquals("ADDR lxmf@2a6105f57145860441a62fe3b2a1352c", reply)
            assertEquals(1, requester.calls)
        }
    }

    @Test
    fun `a reader who backed out is answered with the cancel line`() {
        val requester = fakeRequester(null)
        withServer(requester) { server ->
            val reply = ask(server, ScanProtocol.requestLine(server.token))
            assertEquals(ScanProtocol.CANCEL_REPLY, reply)
            assertEquals(1, requester.calls)
        }
    }

    @Test
    fun `a request with the wrong token opens no camera`() {
        val requester = fakeRequester("anything")
        withServer(requester) { server ->
            val wrong = if (server.token == "wrong") "also-wrong" else "wrong"
            val reply = ask(server, ScanProtocol.requestLine(wrong))
            assertNull(reply)
            assertEquals("the appliance opened the camera for a request it should have refused", 0, requester.calls)
        }
    }

    @Test
    fun `a line that is not a request opens no camera`() {
        val requester = fakeRequester("anything")
        withServer(requester) { server ->
            for (line in listOf("", "HELLO", "ADDR 2a61", "SCAN", "SCAN a b")) {
                val reply = ask(server, line)
                assertNull("the appliance answered \"$line\"", reply)
            }
            assertEquals("the appliance opened the camera for a line that is not a request", 0, requester.calls)
        }
    }

    @Test
    fun `a peer that sends nothing at all is let go of`() {
        val requester = fakeRequester("anything")
        withServer(requester) { server ->
            // A connection that is opened and closed without a request: the accept loop must
            // survive it, which the request after it proves.
            Socket(InetAddress.getLoopbackAddress(), server.port).close()

            val reply = ask(server, ScanProtocol.requestLine(server.token))
            assertEquals("ADDR anything", reply)
            assertEquals(1, requester.calls)
        }
    }

    @Test
    fun `the listener is on the loopback interface and answers more than once`() {
        val requester = fakeRequester("payload")
        withServer(requester) { server ->
            assertTrue(
                "the listener is not bound to a loopback address: ${server.endpoint}",
                InetAddress.getByName(server.endpoint.hostPart()).isLoopbackAddress,
            )
            repeat(3) {
                assertEquals("ADDR payload", ask(server, ScanProtocol.requestLine(server.token)))
            }
            assertEquals(3, requester.calls)
        }
    }

    @Test
    fun `the endpoint is host and port, with an IPv6 literal bracketed`() {
        // Which loopback this is depends on where it runs — the JVM here resolves the
        // loopback to 127.0.0.1 and Android's to ::1 — and the client is Go, where an
        // unbracketed "::1:46635" is "too many colons in address" and never connects. So the
        // endpoint has to be a form Go dials whichever address it names, and this test names
        // that requirement rather than either address.
        withServer(fakeRequester(null)) { server ->
            val endpoint = server.endpoint
            assertTrue(
                "the endpoint is not host:port for any host: $endpoint",
                Regex("^\\[[0-9a-fA-F:]+]:\\d+$|^[0-9A-Fa-f.]+:\\d+$").matches(endpoint),
            )
            val host = endpoint.hostPart()
            assertTrue("the endpoint names a host: $endpoint", host.isNotEmpty())
            assertEquals("the endpoint does not name the port it is listening on", server.port, endpoint.portPart())
            assertTrue(
                "the endpoint's host $host is not an IP literal, which is the only thing this " +
                    "listener is meant to name",
                InetAddress.getByName(host).isLoopbackAddress || Regex("^[0-9a-fA-F:.]+$").matches(host),
            )
        }
    }

    @Test
    fun `an IPv6 literal is bracketed and an IPv4 one is not`() {
        // The rule, stated on both addresses rather than on whichever one this machine
        // happens to have. These are the two strings Go's net.Dial accepts.
        assertEquals("[::1]:46635", ScanServer.endpointOf("::1", 46635))
        assertEquals("127.0.0.1:46635", ScanServer.endpointOf("127.0.0.1", 46635))
    }

    @Test
    fun `the token is hexadecimal, which is what the Go half parses`() {
        // The client is handed the token as an environment variable, so it has to be hex
        // digits and nothing else.
        val server = ScanServer(fakeRequester(null), byteArrayOf(0x00, 0x0f, 0xff.toByte(), 0x10))
        try {
            assertEquals("000fff10", server.token)
        } finally {
            server.close()
        }
    }

    /** hostPart is the host of a host:port endpoint, with any IPv6 brackets removed. */
    private fun String.hostPart(): String {
        val withoutPort = substringBeforeLast(':')
        return withoutPort.removePrefix("[").removeSuffix("]")
    }

    /** portPart is the port of a host:port endpoint. */
    private fun String.portPart(): Int = substringAfterLast(':').toInt()
}
