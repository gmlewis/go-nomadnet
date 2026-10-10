// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import java.io.ByteArrayOutputStream
import java.io.File

/**
 * The channels a freshly installed appliance already knows about.
 *
 * The client keeps its channels in a store of its own — `rrc_hubs`, where each entry is a hub
 * destination, the name to show it under, and the rooms to join — and it has no notion of a
 * default. An appliance that shipped none therefore arrived with an empty Channels page and an
 * operator with nothing to connect to, while the transport underneath it was already talking
 * to the mesh.
 *
 * So the appliance writes the store before the client ever runs, and writes only into a file
 * that is still its own. A file that is absent has never been written, and a file that is still
 * byte-identical to the seed this appliance writes is one nobody has edited — that is the record
 * that this has been done, and both may be written or completed. The first console open usually
 * comes before the stack has ever run, so the local hub's destination is not known yet and the
 * two public hubs are written; the local hub is added on a later open, once it is. Anything else
 * is the operator's, or the client's own rewrite, and is left exactly alone: a channel the
 * operator removes stays removed, and a store the client has saved is never second-guessed.
 *
 * Three channels, because they are the three an appliance can actually reach:
 *
 *  * the community hub, which carries the wider RRC network;
 *  * this project's public hub, which is the one the README documents;
 *  * the hub running **inside this appliance**, which is where its own bot answers, and whose
 *    destination is different on every install because it is derived from an identity
 *    generated on the device. It is written only when that identity has published its
 *    destination, which the hub does a moment after it starts.
 */
object DefaultHubs {

    /** One channel as the client's store describes it. */
    data class DefaultHub(val name: String, val destination: String, val rooms: List<String>)

    /** The hub the wider Reticulum community uses. */
    const val RNS_COMMUNITY_NAME = "RNS Community"

    /** Its RRC destination. */
    const val RNS_COMMUNITY_DESTINATION = "28c7c1a68c735693aa8e6b8193ed44b2"

    /** This project's public hub, the one the README documents. */
    const val PUBLIC_HUB_NAME = "gonomadnet Public Hub"

    /** Its RRC destination. */
    const val PUBLIC_HUB_DESTINATION = "a012129c10205c0b9441fcd2b755b2a7"

    /** The name the local hub is listed under, which is the name it announces. */
    const val LOCAL_HUB_NAME = "appliance-hub"

    /** The room every one of these hubs carries. */
    const val DEFAULT_ROOM = "general"

    /** The destination name hubs announce under; the client's own default. */
    const val DEST_NAME = "rrc.hub"

    /**
     * The channels to seed, which are the two public ones and, when it is known, the hub
     * inside the appliance.
     *
     * [localHub] is that hub's destination, or null when it has not published one yet — an
     * appliance whose stack has never been started has no local hub to list, and inventing a
     * destination for it would be inventing one that cannot exist.
     */
    fun hubs(localHub: String?): List<DefaultHub> {
        val out = mutableListOf(
            DefaultHub(RNS_COMMUNITY_NAME, RNS_COMMUNITY_DESTINATION, listOf(DEFAULT_ROOM)),
            DefaultHub(PUBLIC_HUB_NAME, PUBLIC_HUB_DESTINATION, listOf(DEFAULT_ROOM)),
        )
        localHubHex(localHub)?.let { out.add(DefaultHub(LOCAL_HUB_NAME, it, listOf(DEFAULT_ROOM))) }
        return out
    }

    /**
     * Writes the channels to [store], and reports whether it did.
     *
     * A store that is absent is written. A store that is still byte-identical to the seed this
     * appliance writes with no local hub is the appliance's own untouched file and is completed
     * with the local hub once [localHub] is known: the two public hubs are written on the first
     * console open, before the stack has ever run, and the local hub is added on a later one.
     * Anything else is somebody's decision and is left exactly as it is — the client owns that
     * file from the moment it runs, and an appliance that rewrote it would undo every channel an
     * operator had added or removed.
     */
    fun seed(store: File, localHub: String?): Boolean {
        val existing = store.takeIf { it.isFile }?.readBytes()
        if (existing == null && store.exists()) {
            // A path that exists but is not a regular file is not a store, and not ours to
            // replace.
            return false
        }
        val wanted = when {
            existing == null -> encode(hubs(localHub))
            localHubHex(localHub) == null -> return false
            existing.contentEquals(encode(hubs(null))) -> encode(hubs(localHub))
            else -> return false
        }
        store.parentFile?.mkdirs()
        store.writeBytes(wanted)
        return true
    }

    /**
     * Encodes the whole store: a map with one `hubs` key holding the list.
     *
     * The format is the client's own — CBOR, written by `rrc.RRCManager.Save` — and this is
     * the second implementation of it, which is why [DefaultHubsTest] pins the exact bytes and
     * a Go test decodes those same bytes with the client's own reader. A store the client
     * cannot read is a store it silently ignores, leaving the appliance with no channels at
     * all and no error anywhere to say so.
     */
    fun encode(hubs: List<DefaultHub>): ByteArray {
        val out = ByteArrayOutputStream()
        writeMapHead(out, 1)
        writeText(out, "hubs")
        writeArrayHead(out, hubs.size)
        for (hub in hubs) {
            writeHub(out, hub)
        }
        return out.toByteArray()
    }

    /**
     * Writes one hub entry, with its keys in the order the client writes them, which is
     * alphabetical.
     *
     * The three auto flags are on, which is a departure from the client's own default of off
     * and is the whole point of seeding them: an appliance is expected to reconnect to its
     * channels by itself, and to be showing who is in a room when somebody looks at it. They
     * are the operator's to change afterwards, from the channel list.
     */
    private fun writeHub(out: ByteArrayOutputStream, hub: DefaultHub) {
        val destination = hexBytes(hub.destination)
            ?: throw IllegalArgumentException("${hub.name} has the destination ${hub.destination}, which is not hex")
        writeMapHead(out, 8)
        writeText(out, "auto_list")
        writeBool(out, true)
        writeText(out, "auto_reconnect")
        writeBool(out, true)
        writeText(out, "auto_who")
        writeBool(out, true)
        writeText(out, "dest_name")
        writeText(out, DEST_NAME)
        writeText(out, "hash")
        writeBytes(out, destination)
        writeText(out, "name")
        writeText(out, hub.name)
        writeText(out, "parted_rooms")
        writeArrayHead(out, 0)
        writeText(out, "rooms")
        writeArrayHead(out, hub.rooms.size)
        for (room in hub.rooms) {
            writeText(out, room)
        }
    }

    /** localHubHex is [localHub] as lowercase hex, or null when it is not one. */
    private fun localHubHex(localHub: String?): String? =
        localHub?.trim()?.lowercase()?.takeIf { hexBytes(it) != null }

    /**
     * hexBytes decodes a destination, which is sixteen bytes — the client stores a hub's
     * truncated destination hash, not the thirty-two byte one — or null when the text is not
     * exactly that much hex.
     */
    fun hexBytes(hex: String): ByteArray? {
        if (hex.length != HUB_HASH_HEX_LENGTH || !hex.all { it.isHexDigit() }) {
            return null
        }
        return ByteArray(hex.length / 2) { i ->
            hex.substring(i * 2, i * 2 + 2).toInt(16).toByte()
        }
    }

    private fun Char.isHexDigit(): Boolean = this in '0'..'9' || this in 'a'..'f'

    /**
     * CBOR's head byte: the major type and, for values under 24, the value itself; otherwise a
     * length marker followed by the value in the fewest whole bytes that hold it.
     */
    private fun writeHead(out: ByteArrayOutputStream, major: Int, value: Int) {
        val prefix = major shl 5
        when {
            value < 24 -> out.write(prefix or value)
            value <= 0xff -> {
                out.write(prefix or 24)
                out.write(value)
            }
            value <= 0xffff -> {
                out.write(prefix or 25)
                out.write(value ushr 8)
                out.write(value and 0xff)
            }
            else -> {
                out.write(prefix or 26)
                out.write(value ushr 24)
                out.write((value ushr 16) and 0xff)
                out.write((value ushr 8) and 0xff)
                out.write(value and 0xff)
            }
        }
    }

    private fun writeMapHead(out: ByteArrayOutputStream, size: Int) = writeHead(out, MAJOR_MAP, size)

    private fun writeArrayHead(out: ByteArrayOutputStream, size: Int) = writeHead(out, MAJOR_ARRAY, size)

    /**
     * Writes a text string. Its bytes are UTF-8, and its length is counted in those bytes
     * rather than in characters: a room name outside ASCII would otherwise be given a length
     * the reader would walk past, which turns the rest of the store into noise.
     */
    private fun writeText(out: ByteArrayOutputStream, text: String) {
        val bytes = text.toByteArray(Charsets.UTF_8)
        writeHead(out, MAJOR_TEXT, bytes.size)
        out.write(bytes, 0, bytes.size)
    }

    private fun writeBytes(out: ByteArrayOutputStream, bytes: ByteArray) {
        writeHead(out, MAJOR_BYTES, bytes.size)
        out.write(bytes, 0, bytes.size)
    }

    private fun writeBool(out: ByteArrayOutputStream, value: Boolean) {
        out.write(if (value) BOOL_TRUE else BOOL_FALSE)
    }

    /** HUB_HASH_HEX_LENGTH is a hub destination's length in hex: sixteen bytes. */
    const val HUB_HASH_HEX_LENGTH = 32

    private const val MAJOR_BYTES = 2
    private const val MAJOR_TEXT = 3
    private const val MAJOR_ARRAY = 4
    private const val MAJOR_MAP = 5
    private const val BOOL_FALSE = 0xf4
    private const val BOOL_TRUE = 0xf5
}
