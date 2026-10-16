// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify it under the
// terms of the GNU General Public License as published by the Free Software
// Foundation, either version 3 of the License, or (at your option) any later version.

package com.gmlewis.gonomadnet

import com.google.zxing.BarcodeFormat
import com.google.zxing.EncodeHintType
import com.google.zxing.common.BitMatrix
import com.google.zxing.qrcode.QRCodeWriter
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * The decoder, round-tripped against zxing's own encoder.
 *
 * The encoder shipping in the same artifact is what makes this testable at all without a
 * device: a QR code can be drawn, turned into the luminance plane a camera preview frame
 * carries, and read back. A failure here is a decoder that would have failed on the tablet
 * too, which is the whole reason the decode is a plain function over a byte array rather than
 * something behind an activity.
 */
class ScanDecoderTest {

    /** The address a QR code in these tests carries. */
    private val address = "2a6105f57145860441a62fe3b2a1352c"

    /**
     * frame draws payload as a QR code and returns the luminance plane a camera would hand
     * over: one byte per pixel, black as 0 and white as 255.
     */
    private fun frame(payload: String, size: Int = 256, quietZone: Int = 4): Triple<ByteArray, Int, Int> {
        val matrix: BitMatrix = QRCodeWriter().encode(
            payload,
            BarcodeFormat.QR_CODE,
            size,
            size,
            mapOf(EncodeHintType.MARGIN to quietZone),
        )
        val luminance = ByteArray(matrix.width * matrix.height)
        for (y in 0 until matrix.height) {
            for (x in 0 until matrix.width) {
                luminance[y * matrix.width + x] = if (matrix.get(x, y)) 0 else 255.toByte()
            }
        }
        return Triple(luminance, matrix.width, matrix.height)
    }

    /** quarterTurned returns frame turned a quarter turn clockwise. */
    private fun quarterTurned(luminance: ByteArray, width: Int, height: Int): Triple<ByteArray, Int, Int> {
        val turned = ByteArray(luminance.size)
        val turnedWidth = height
        for (y in 0 until height) {
            for (x in 0 until width) {
                // (x, y) goes to (height - 1 - y, x) as the image turns clockwise.
                turned[x * turnedWidth + (height - 1 - y)] = luminance[y * width + x]
            }
        }
        return Triple(turned, turnedWidth, width)
    }

    /**
     * inverted returns frame with every pixel complemented: the same code drawn light on dark,
     * quiet zone included, which is what a dark background makes of it.
     */
    private fun inverted(luminance: ByteArray): ByteArray =
        ByteArray(luminance.size) { (255 - (luminance[it].toInt() and 0xFF)).toByte() }

    @Test
    fun `a bare address is read back`() {
        val (luminance, width, height) = frame(address)
        assertEquals(address, ScanDecoder.decode(luminance, width, height))
    }

    @Test
    fun `a code drawn light on dark is read as readily as one drawn dark on light`() {
        // Which way round the ink is is not the reader's to choose: the appliance's own QR
        // dialog draws its code in the terminal's colours, so a dark theme paints its modules
        // light on a dark field — and so does anything else that renders a code for a screen.
        val payload = "lxmf@$address"
        val (luminance, width, height) = frame(payload)
        assertEquals(payload, ScanDecoder.decode(inverted(luminance), width, height))
    }

    @Test
    fun `a code drawn light on dark and turned a quarter turn is still read`() {
        val (luminance, width, height) = frame(address)
        val (turned, turnedWidth, turnedHeight) = quarterTurned(inverted(luminance), width, height)
        assertEquals(address, ScanDecoder.decode(turned, turnedWidth, turnedHeight))
    }

    @Test
    fun `a QR code a link was written into is read back whole`() {
        // The decoder returns the code's text and nothing more: it is the client that
        // decides whether that text is an address, so a link must arrive unaltered.
        val payload = "lxmf@$address"
        val (luminance, width, height) = frame(payload)
        assertEquals(payload, ScanDecoder.decode(luminance, width, height))
    }

    @Test
    fun `a frame that has been turned a quarter turn is still read`() {
        // A tablet held in portrait delivers the sensor's frames with the image turned; a
        // reader should not have to hold the tablet the way the sensor is mounted.
        val (luminance, width, height) = frame(address)
        val (turned, turnedWidth, turnedHeight) = quarterTurned(luminance, width, height)
        assertEquals(address, ScanDecoder.decode(turned, turnedWidth, turnedHeight))
    }

    @Test
    fun `a frame holding no code reads as nothing`() {
        val blank = ByteArray(64 * 64) { 255.toByte() }
        assertNull(ScanDecoder.decode(blank, 64, 64))
    }

    @Test
    fun `a frame of noise reads as nothing rather than as a wrong address`() {
        // The important half of this is not that noise fails, it is that a reader holding
        // the camera at nothing gets nothing: a decoder that guessed would fill an address
        // field with an address the reader never chose. The noise is a xorshift sequence so
        // that it is deterministic and still has no finder patterns to find.
        var state = 0x2545F4914F6CDD1DL
        val noise = ByteArray(128 * 128) {
            state = state xor (state shl 13)
            state = state xor (state ushr 7)
            state = state xor (state shl 17)
            state.toByte()
        }
        assertNull(ScanDecoder.decode(noise, 128, 128))
    }

    @Test
    fun `a frame that is too small to hold its pixels reads as nothing`() {
        assertNull(ScanDecoder.decode(ByteArray(4), 64, 64))
    }

    @Test
    fun `a frame with no pixels reads as nothing`() {
        assertNull(ScanDecoder.decode(ByteArray(0), 0, 0))
    }
}
