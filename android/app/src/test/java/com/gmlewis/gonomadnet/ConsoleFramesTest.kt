// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The console frame codec's contract with the Go host.
 *
 * `nomadnet/console/frames_test.go` carries this same table. If the two codecs disagree,
 * the appliance attaches to a console that renders nothing and logs nothing, so the
 * table is asserted byte for byte in both languages.
 */
class ConsoleFramesTest {

    private data class Golden(val name: String, val hex: String, val frame: ConsoleFrame)

    private val golden = listOf(
        Golden("data hello", "010000000568656c6c6f", ConsoleFrame.Data("hello".toByteArray())),
        Golden("data empty", "0100000000", ConsoleFrame.Data(ByteArray(0))),
        Golden("resize 80x24", "020000000400500018", ConsoleFrame.Resize(80, 24)),
        Golden("resize 200x60", "020000000400c8003c", ConsoleFrame.Resize(200, 60)),
        Golden("exit 0", "030000000100", ConsoleFrame.Exit(0)),
        Golden("exit 130", "030000000182", ConsoleFrame.Exit(130)),
    )

    @Test
    fun `the golden frame table decodes identically in Kotlin and Go`() {
        for (entry in golden) {
            val wire = hexToBytes(entry.hex)
            assertArrayEquals(
                "${entry.name}: the encoder disagrees with the table",
                wire,
                ConsoleFrames.encode(entry.frame),
            )
            assertEquals(
                "${entry.name}: the decoder disagrees with the table",
                entry.frame,
                ConsoleFrames.decode(wire),
            )
        }
    }

    @Test
    fun `a length over one mebibyte is refused rather than allocated`() {
        // A peer that claims 0xFFFFFFFF bytes must not be able to make the app reserve
        // four gigabytes, so the length is refused before any payload array is built.
        for (wire in listOf(
            hexToBytes("0100100001"),
            hexToBytes("01ffffffff"),
            hexToBytes("0200100001"),
        )) {
            val failure = assertThrows(ConsoleProtocolException::class.java) {
                ConsoleFrames.decode(wire)
            }
            assertTrue(
                "the refusal must say the frame is too large: ${failure.message}",
                failure.message!!.contains("1 MiB"),
            )
        }
    }

    @Test
    fun `an unknown frame type is refused`() {
        for (wire in listOf(
            hexToBytes("0000000000"),
            hexToBytes("0400000000"),
            hexToBytes("ff00000000"),
        )) {
            val failure = assertThrows(ConsoleProtocolException::class.java) {
                ConsoleFrames.decode(wire)
            }
            assertTrue(
                "the refusal must name the type: ${failure.message}",
                failure.message!!.contains("unknown frame type"),
            )
        }
    }

    @Test
    fun `a truncated frame is an error, not a short read`() {
        for (wire in listOf(
            hexToBytes("01000000056865"),
            hexToBytes("0100000005"),
            hexToBytes("010000"),
            hexToBytes("01"),
            hexToBytes(""),
            hexToBytes("02000000040050"),
            hexToBytes("0300000001"),
        )) {
            assertThrows(ConsoleProtocolException::class.java) {
                ConsoleFrames.decode(wire)
            }
        }
    }

    @Test
    fun `a payload of the wrong size for its type is refused`() {
        // A resize frame is four bytes and an exit frame is one. Any other length is a
        // protocol error rather than something to pad or truncate.
        for (wire in listOf(
            hexToBytes("02000000020050"),
            hexToBytes("02000000050050001800"),
            hexToBytes("03000000020000"),
        )) {
            val failure = assertThrows(ConsoleProtocolException::class.java) {
                ConsoleFrames.decode(wire)
            }
            assertTrue(
                "the refusal must say the payload is malformed: ${failure.message}",
                failure.message!!.contains("malformed"),
            )
        }
    }

    @Test
    fun `the streaming reader consumes exactly one frame and leaves the next`() {
        // The session reads frame after frame from one socket, so the reader must stop
        // at a frame boundary rather than swallowing what follows.
        val stream = java.io.ByteArrayInputStream(
            hexToBytes("010000000568656c6c6f0200000004005000180100000005") +
                "world".toByteArray() +
                hexToBytes("030000000182"),
        )
        val reader = ConsoleFrameReader(stream)
        assertEquals(ConsoleFrame.Data("hello".toByteArray()), reader.read())
        assertEquals(ConsoleFrame.Resize(80, 24), reader.read())
        assertEquals(ConsoleFrame.Data("world".toByteArray()), reader.read())
        assertEquals(ConsoleFrame.Exit(130), reader.read())
        assertEquals("a clean end at a frame boundary is not an error", null, reader.read())
    }

    @Test
    fun `the streaming reader reports a frame that ended early`() {
        val reader = ConsoleFrameReader(java.io.ByteArrayInputStream(hexToBytes("01000000056865")))
        assertThrows(ConsoleProtocolException::class.java) { reader.read() }
    }

    /** hexToBytes is the test's own decoder, so the table can be read as it is written. */
    private fun hexToBytes(hex: String): ByteArray {
        require(hex.length % 2 == 0) { "a hex string has two characters per byte: $hex" }
        val out = ByteArray(hex.length / 2)
        for (i in out.indices) {
            out[i] = hex.substring(i * 2, i * 2 + 2).toInt(16).toByte()
        }
        return out
    }
}
