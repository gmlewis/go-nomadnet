// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify it under the
// terms of the GNU General Public License as published by the Free Software
// Foundation, either version 3 of the License, or (at your option) any later version.

package com.gmlewis.gonomadnet

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The camera scan protocol's half that can be asserted without a camera.
 *
 * The lines here are the ones `tui/scanner.go` reads and writes: a disagreement between the
 * two halves does not fail anywhere, it shows up to the reader as a scan button that does
 * nothing at all, which is why the exact bytes are pinned rather than the shape of them.
 */
class ScanProtocolTest {

    @Test
    fun `the request line is the verb and the token`() {
        assertEquals("SCAN 0f1e2d3c", ScanProtocol.requestLine("0f1e2d3c"))
    }

    @Test
    fun `the reply line is the verb and the payload`() {
        assertEquals("ADDR lxmf@2a61", ScanProtocol.replyLine("lxmf@2a61"))
    }

    @Test
    fun `a request parses to its token`() {
        assertEquals("0f1e2d3c", ScanProtocol.parseRequest(ScanProtocol.requestLine("0f1e2d3c")))
    }

    @Test
    fun `a line that is not a request parses to nothing`() {
        val lines = listOf(
            "",
            "SCAN",
            "SCAN ",
            "SCAN a b",
            "scan 0f1e2d3c",
            "ADDR 0f1e2d3c",
            "SCAN\t0f1e2d3c",
            "0f1e2d3c",
        )
        for (line in lines) {
            assertNull("parseRequest(${line.quote()}) parsed a token", ScanProtocol.parseRequest(line))
        }
    }

    @Test
    fun `a token is accepted only when it is the whole of the offered one`() {
        assertTrue(ScanProtocol.acceptsToken("0f1e2d3c", "0f1e2d3c"))
        assertFalse(ScanProtocol.acceptsToken("0f1e2d3c", "0f1e2d3"))
        assertFalse(ScanProtocol.acceptsToken("0f1e2d3c", "0f1e2d3cd"))
        assertFalse(ScanProtocol.acceptsToken("0f1e2d3c", "0f1e2d3d"))
        assertFalse(ScanProtocol.acceptsToken("0f1e2d3c", null))
        assertFalse(ScanProtocol.acceptsToken("0f1e2d3c", ""))
    }

    @Test
    fun `a listener with no token accepts nothing`() {
        // A launch that somehow arrived without a token must not be one that dropped the
        // guard: the empty string is not a token that anything can offer.
        assertFalse(ScanProtocol.acceptsToken("", ""))
        assertFalse(ScanProtocol.acceptsToken("", "anything"))
    }

    @Test
    fun `the constant time comparison agrees with equality`() {
        assertTrue(ScanProtocol.constantTimeEquals("", ""))
        assertTrue(ScanProtocol.constantTimeEquals("abc", "abc"))
        assertFalse(ScanProtocol.constantTimeEquals("abc", "abd"))
        assertFalse(ScanProtocol.constantTimeEquals("abc", "ab"))
    }

    /** quote renders a line for a failure message. */
    private fun String.quote(): String = "\"" + this + "\""
}
