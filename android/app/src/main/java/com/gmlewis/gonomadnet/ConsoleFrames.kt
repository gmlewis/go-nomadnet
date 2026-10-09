// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import java.io.EOFException
import java.io.InputStream

/**
 * One frame on the console socket.
 *
 * The Kotlin half of the codec the Go host implements in `nomadnet/console/frames.go`.
 * Both halves are asserted against one table of bytes in [ConsoleFramesTest] and
 * `frames_test.go`; a disagreement between them produces a console that renders
 * nothing and reports no error.
 */
sealed class ConsoleFrame {
    /** The wire type byte this frame is sent as. */
    abstract val type: Int

    /** Raw terminal bytes, in either direction. */
    class Data(val payload: ByteArray) : ConsoleFrame() {
        override val type: Int get() = ConsoleFrames.TYPE_DATA

        override fun equals(other: Any?): Boolean = other is Data && payload.contentEquals(other.payload)

        override fun hashCode(): Int = payload.contentHashCode()

        override fun toString(): String = "Data(${payload.size} bytes)"
    }

    /** A new window size: columns then rows. */
    data class Resize(val cols: Int, val rows: Int) : ConsoleFrame() {
        override val type: Int get() = ConsoleFrames.TYPE_RESIZE
    }

    /** The client's exit status, 0..255. Sent only from the host to the app. */
    data class Exit(val code: Int) : ConsoleFrame() {
        override val type: Int get() = ConsoleFrames.TYPE_EXIT
    }
}

/** A frame could not be read: the stream has lost its framing and cannot be resynchronised. */
class ConsoleProtocolException(message: String) : Exception(message)

/**
 * The console frame codec.
 *
 * Every frame is `type(1) | length(4, big-endian) | payload`. The declared length is
 * refused above [MAX_PAYLOAD] before anything is allocated, an unknown type is refused
 * outright, and a stream that ends inside a frame is an error rather than a short read
 * to be acted on.
 */
object ConsoleFrames {
    /** The largest payload either half accepts. Frames are keystrokes and screen text. */
    const val MAX_PAYLOAD: Int = 1 shl 20

    const val TYPE_DATA: Int = 0x01
    const val TYPE_RESIZE: Int = 0x02
    const val TYPE_EXIT: Int = 0x03

    /** The five bytes a frame's header occupies. */
    const val HEADER_SIZE: Int = 5

    /** encode returns the wire form of [frame]. */
    fun encode(frame: ConsoleFrame): ByteArray {
        val payload = payloadOf(frame)
        val out = ByteArray(HEADER_SIZE + payload.size)
        out[0] = frame.type.toByte()
        putUint32(out, 1, payload.size)
        payload.copyInto(out, HEADER_SIZE)
        return out
    }

    /**
     * decode returns the single frame [bytes] holds.
     *
     * Anything that is not exactly one well-formed frame — a short buffer, a length that
     * does not match the bytes present, an unknown type, an impossible payload size for
     * the type — is a [ConsoleProtocolException].
     */
    fun decode(bytes: ByteArray): ConsoleFrame {
        if (bytes.size < HEADER_SIZE) {
            throw ConsoleProtocolException(
                "a frame header is $HEADER_SIZE bytes, got ${bytes.size}",
            )
        }
        val type = bytes[0].toInt() and 0xff
        // The length is read as an unsigned 32-bit value: as an Int, a peer's 0xFFFFFFFF
        // becomes -1 and slips past the size check entirely.
        val length = uint32(bytes, 1)
        // Both checks run before the payload is copied: neither may allocate.
        if (length > MAX_PAYLOAD) {
            throw ConsoleProtocolException("the frame is $length bytes, over the 1 MiB limit")
        }
        if (!isKnown(type)) {
            throw ConsoleProtocolException("unknown frame type 0x${type.toString(16)}")
        }
        if (bytes.size < HEADER_SIZE + length) {
            throw ConsoleProtocolException(
                "the frame declares $length payload bytes but only ${bytes.size - HEADER_SIZE} are present",
            )
        }
        if (bytes.size > HEADER_SIZE + length) {
            throw ConsoleProtocolException(
                "${bytes.size - HEADER_SIZE - length} bytes follow the frame",
            )
        }
        return assemble(type, bytes.copyOfRange(HEADER_SIZE, HEADER_SIZE + length.toInt()))
    }

    /** isKnown reports whether [type] names a frame this protocol defines. */
    fun isKnown(type: Int): Boolean = type == TYPE_DATA || type == TYPE_RESIZE || type == TYPE_EXIT

    /** payloadOf returns the bytes [frame] carries, in wire order. */
    fun payloadOf(frame: ConsoleFrame): ByteArray = when (frame) {
        is ConsoleFrame.Data -> frame.payload
        is ConsoleFrame.Resize -> ByteArray(4).also {
            putUint16(it, 0, frame.cols)
            putUint16(it, 2, frame.rows)
        }
        is ConsoleFrame.Exit -> byteArrayOf(frame.code.toByte())
    }

    /**
     * assemble turns a validated type and payload into a frame.
     *
     * A resize frame is exactly four bytes and an exit frame exactly one; any other size
     * is a protocol error rather than something to pad or truncate.
     */
    fun assemble(type: Int, payload: ByteArray): ConsoleFrame = when (type) {
        TYPE_DATA -> ConsoleFrame.Data(payload)
        TYPE_RESIZE -> {
            if (payload.size != 4) {
                throw ConsoleProtocolException("a malformed resize frame is ${payload.size} bytes, want 4")
            }
            ConsoleFrame.Resize(uint16(payload, 0), uint16(payload, 2))
        }
        TYPE_EXIT -> {
            if (payload.size != 1) {
                throw ConsoleProtocolException("a malformed exit frame is ${payload.size} bytes, want 1")
            }
            ConsoleFrame.Exit(payload[0].toInt() and 0xff)
        }
        else -> throw ConsoleProtocolException("unknown frame type 0x${type.toString(16)}")
    }

    private fun putUint16(dst: ByteArray, at: Int, value: Int) {
        dst[at] = ((value ushr 8) and 0xff).toByte()
        dst[at + 1] = (value and 0xff).toByte()
    }

    private fun putUint32(dst: ByteArray, at: Int, value: Int) {
        dst[at] = ((value ushr 24) and 0xff).toByte()
        dst[at + 1] = ((value ushr 16) and 0xff).toByte()
        dst[at + 2] = ((value ushr 8) and 0xff).toByte()
        dst[at + 3] = (value and 0xff).toByte()
    }

    private fun uint16(src: ByteArray, at: Int): Int =
        ((src[at].toInt() and 0xff) shl 8) or (src[at + 1].toInt() and 0xff)

    /**
     * uint32 reads a big-endian 32-bit length as an unsigned value.
     *
     * It is a Long, not an Int, so that a declared length with the high bit set is a
     * large number rather than a negative one.
     */
    private fun uint32(src: ByteArray, at: Int): Long =
        ((src[at].toLong() and 0xff) shl 24) or
            ((src[at + 1].toLong() and 0xff) shl 16) or
            ((src[at + 2].toLong() and 0xff) shl 8) or
            (src[at + 3].toLong() and 0xff)
}

/**
 * Reads frame after frame from one stream.
 *
 * [read] returns null only when the stream ended exactly at a frame boundary, which is a
 * clean shutdown; ending inside a frame throws instead, because acting on the bytes seen
 * so far would mean guessing where the next frame begins.
 */
class ConsoleFrameReader(private val input: InputStream) {

    /** read returns the next frame, or null at a clean end of stream. */
    fun read(): ConsoleFrame? {
        val header = ByteArray(ConsoleFrames.HEADER_SIZE)
        val headerBytes = readSome(header)
        if (headerBytes == 0) return null
        if (headerBytes < header.size) {
            throw ConsoleProtocolException("the stream ended inside a frame header")
        }

        val type = header[0].toInt() and 0xff
        // Unsigned, as in decode: a peer's 0xFFFFFFFF must not become -1.
        val length = ((header[1].toLong() and 0xff) shl 24) or
            ((header[2].toLong() and 0xff) shl 16) or
            ((header[3].toLong() and 0xff) shl 8) or
            (header[4].toLong() and 0xff)

        if (length > ConsoleFrames.MAX_PAYLOAD) {
            throw ConsoleProtocolException("the frame is $length bytes, over the 1 MiB limit")
        }
        if (!ConsoleFrames.isKnown(type)) {
            throw ConsoleProtocolException("unknown frame type 0x${type.toString(16)}")
        }

        val payload = ByteArray(length.toInt())
        if (readSome(payload) < payload.size) {
            throw ConsoleProtocolException("the stream ended inside a frame payload")
        }
        return ConsoleFrames.assemble(type, payload)
    }

    /** readSome fills all of [buffer], returning how many bytes were read before the end. */
    private fun readSome(buffer: ByteArray): Int {
        var filled = 0
        while (filled < buffer.size) {
            val read = try {
                input.read(buffer, filled, buffer.size - filled)
            } catch (ended: EOFException) {
                -1
            }
            if (read < 0) return filled
            filled += read
        }
        return filled
    }
}
