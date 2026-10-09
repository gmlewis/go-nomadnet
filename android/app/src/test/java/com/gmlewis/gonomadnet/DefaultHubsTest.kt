// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import org.junit.Assert.assertArrayEquals
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/**
 * The channels an appliance comes with, and the file it writes them into.
 *
 * The store is the client's own file, in the client's own format, so this encoder is the
 * second implementation of a format that already has one — `rrc.RRCManager` in the Go client.
 * The golden file in `src/test/resources` is what pins the two together: this test asserts the
 * encoder produces exactly those bytes, and a Go test in `nomadnet/app` decodes those same
 * bytes with the client's own reader. A store the client cannot read is reported as an error
 * at startup and otherwise ignored, so the appliance would show an empty Channels page with
 * nothing anywhere saying why.
 */
class DefaultHubsTest {

    /** fixture is the store as the client's own encoder writes it. */
    private fun fixture(): ByteArray {
        val file = File("src/test/resources/default_hubs.cbor")
        assertTrue("the golden store is missing: ${file.absolutePath}", file.isFile)
        return file.readBytes()
    }

    /** expected is the three channels the fixture was written for. */
    private val expected = listOf(
        DefaultHubs.DefaultHub("RNS Community", "28c7c1a68c735693aa8e6b8193ed44b2", listOf("general")),
        DefaultHubs.DefaultHub("gonomadnet Public Hub", "a012129c10205c0b9441fcd2b755b2a7", listOf("general")),
        DefaultHubs.DefaultHub("appliance-hub", "00112233445566778899aabbccddeeff", listOf("general")),
    )

    @Test
    fun `the encoder writes exactly what the client's own reader expects`() {
        assertArrayEquals(
            "the appliance's channel store does not match the format the client reads",
            fixture(),
            DefaultHubs.encode(expected),
        )
    }

    @Test
    fun `an appliance with a local hub comes with three channels`() {
        val hubs = DefaultHubs.hubs("586c61d4ffc6cfa01642362ff71a5652")
        assertEquals("the appliance should ship three channels", 3, hubs.size)
        assertEquals(
            listOf(DefaultHubs.RNS_COMMUNITY_NAME, DefaultHubs.PUBLIC_HUB_NAME, DefaultHubs.LOCAL_HUB_NAME),
            hubs.map { it.name },
        )
        for (hub in hubs) {
            assertEquals("every channel opens on the room the hubs carry", listOf("general"), hub.rooms)
        }
    }

    @Test
    fun `a local hub that has not published a destination is not invented`() {
        // The local hub's destination is derived from an identity generated on the device, so
        // an appliance whose stack has never run has nothing to list it under. Writing a
        // destination nobody can reach would be a channel that can never connect.
        assertEquals(2, DefaultHubs.hubs(null).size)
        assertEquals(2, DefaultHubs.hubs("").size)
        assertEquals(2, DefaultHubs.hubs("   ").size)
        assertEquals(
            "a destination that is not sixteen bytes of hex is not a destination",
            2,
            DefaultHubs.hubs("not-a-hash").size,
        )
    }

    @Test
    fun `the seed writes once and never again`() {
        val dir = tempDir()
        val store = File(dir, "rrc_hubs")

        // Seeded with the fixture's own local hub, so what lands on disk is the golden store
        // byte for byte — the thing the client's reader is tested against.
        assertTrue("a fresh install is seeded", DefaultHubs.seed(store, "00112233445566778899aabbccddeeff"))
        assertArrayEquals("the seeded store is not the store the client reads", fixture(), store.readBytes())

        // The file belongs to the client from the moment it runs: it holds whatever the
        // operator has added or removed, and an appliance that rewrote it on every start
        // would undo that.
        store.writeBytes("the operator's own store".toByteArray())
        assertFalse("an existing store is left alone", DefaultHubs.seed(store, null))
        assertEquals("the operator's store was overwritten", "the operator's own store", store.readText())
    }

    @Test
    fun `a store the appliance writes into a directory that is not there yet is still written`() {
        // The console is opened on a fresh install before the client has ever created its own
        // storage directory.
        val store = File(tempDir(), "etc/nomadnetwork/storage/rrc_hubs")
        assertTrue(DefaultHubs.seed(store, null))
        assertTrue(store.isFile)
    }

    @Test
    fun `a text length is counted in bytes and not in characters`() {
        // A room or a hub name outside ASCII would otherwise be given a length the reader
        // walks past, turning the rest of the store into noise.
        val name = "ééé"
        val encoded = DefaultHubs.encode(listOf(DefaultHubs.DefaultHub(name, "00112233445566778899aabbccddeeff", listOf("general"))))
        val at = encoded.indexOfSubArray(name.toByteArray(Charsets.UTF_8))
        assertTrue("the name is not in the store at all", at > 0)
        assertEquals(
            "the text string's length byte counts characters instead of bytes",
            name.toByteArray(Charsets.UTF_8).size,
            encoded[at - 1].toInt() and 0x1f,
        )
    }

    @Test
    fun `only sixteen bytes of hex is a destination`() {
        assertNull(DefaultHubs.hexBytes("00112233445566778899aabbccddee"))
        assertNull(DefaultHubs.hexBytes("00112233445566778899aabbccddeeff00"))
        assertNull(DefaultHubs.hexBytes("00112233445566778899aabbccddeefg"))
        assertEquals(16, DefaultHubs.hexBytes("00112233445566778899aabbccddeeff")!!.size)
        assertArrayEquals(
            byteArrayOf(0x28, 0xc7.toByte(), 0xc1.toByte(), 0xa6.toByte()),
            DefaultHubs.hexBytes("28c7c1a68c735693aa8e6b8193ed44b2")!!.copyOf(4),
        )    }

    /** tempDir is a directory for one test, removed with it. */
    private fun tempDir(): File = File("/tmp", "default-hubs-${System.nanoTime()}").apply {
        mkdirs()
        deleteOnExit()
    }

    /** indexOfSubArray is where needle occurs in this array, or -1. */
    private fun ByteArray.indexOfSubArray(needle: ByteArray): Int {
        if (needle.isEmpty() || needle.size > size) {
            return -1
        }
        outer@ for (i in 0..size - needle.size) {
            for (j in needle.indices) {
                if (this[i + j] != needle[j]) {
                    continue@outer
                }
            }
            return i
        }
        return -1
    }
}
