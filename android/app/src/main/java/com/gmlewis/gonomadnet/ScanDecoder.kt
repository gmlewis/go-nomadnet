// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify it under the
// terms of the GNU General Public License as published by the Free Software
// Foundation, either version 3 of the License, or (at your option) any later version.

package com.gmlewis.gonomadnet

import com.google.zxing.BarcodeFormat
import com.google.zxing.BinaryBitmap
import com.google.zxing.DecodeHintType
import com.google.zxing.MultiFormatReader
import com.google.zxing.PlanarYUVLuminanceSource
import com.google.zxing.common.HybridBinarizer

/**
 * Reads QR codes out of camera preview frames.
 *
 * This is the whole of the appliance's dependence on zxing, and it is deliberately the only
 * place that touches it: the rest of the scanner — the preview, the permission, the activity
 * — is the platform's own API. Keeping the call in one pure function is also what makes the
 * decoding testable on the JVM, with zxing's own encoder generating the codes to read back;
 * [ScanDecoderTest] does exactly that, including for a frame that has been turned a quarter
 * turn.
 *
 * Frames arrive as the camera's luminance plane: one byte per pixel, row by row, with the
 * pixel at (x, y) at index y * width + x. That is the plane of the NV21 format the preview
 * callback hands over, and it is all a QR code needs — the decoder works on brightness
 * alone.
 */
object ScanDecoder {

    /**
     * The reader, restricted to QR codes.
     *
     * Restricting the formats is what keeps a scan quick enough to run on every preview
     * frame: asked for every format it knows, a reader spends its time proving that a QR
     * code is not a PDF417 symbol. TRY_HARDER is the other half — a reader aiming a device at
     * a screen holds it still for a moment at a time, and reading a code from a frame that is
     * slightly blurred or slightly angled is worth the extra work.
     *
     * What this reader will be asked for is not set here: see [hints], and the note there about
     * what happens to a reader that is configured before it is called.
     */
    private val reader: MultiFormatReader = MultiFormatReader()

    /**
     * hints are what each decode is asked for, and they have to travel with the call.
     *
     * `decode(bitmap)` is `decode(bitmap, null)`, and that null is passed straight into
     * `setHints` — so a reader configured once and then asked to decode forgets every hint it
     * was configured with. A restriction to QR codes that is never applied is a scan that
     * spends its time on every other format, and `TRY_HARDER` that is never applied is a scan
     * that gives up on the frame the reader is holding still.
     *
     * [DecodeHintType.ALSO_INVERTED] is the one that matters most here, because a code is read
     * from its contrast and not from which way round the ink is. The appliance's own QR dialog
     * draws its code in the terminal's colors, so on a dark theme the modules come out light
     * on a dark field — and so does anything else that draws a code for a screen. A reader
     * holding a device up to one has no say in the polarity it is given, so it is given both.
     * The second reading costs a pass over the frame that has already been binarized, and
     * nothing else.
     */
    private val hints: Map<DecodeHintType, Any> = mapOf(
        DecodeHintType.POSSIBLE_FORMATS to listOf(BarcodeFormat.QR_CODE),
        DecodeHintType.TRY_HARDER to true,
        DecodeHintType.ALSO_INVERTED to true,
    )

    /** decode returns the text of the QR code in one frame, or null when the frame holds none.
     *
     * A frame with no code in it is the ordinary case — it is what every frame looks like
     * until the reader has the camera pointed at something — so it is null rather than a
     * failure, and the caller simply carries on with the next frame.
     *
     * The image is taken at the size it arrives, with no rotation. A QR code is read from its
     * three corner patterns, which the reader locates whichever way the image is turned, so
     * the tablet being held in portrait does not require the frame to be turned back: the
     * code is found regardless of which edge of the sensor is up.
     *
     * The reader keeps state between calls, so it is used under a lock: the preview thread
     * and a test cannot then be inside it at once.
     */
    fun decode(luminance: ByteArray, width: Int, height: Int): String? {
        if (width <= 0 || height <= 0 || luminance.size < width * height) {
            return null
        }
        return try {
            synchronized(reader) {
                val source = PlanarYUVLuminanceSource(luminance, width, height, 0, 0, width, height, false)
                // HybridBinarizer rather than the global-histogram one: a QR code is a
                // two-dimensional symbol photographed at whatever angle the reader is holding
                // the tablet, so its illumination varies across the frame, and the hybrid
                // binarizer is the one that copes with a gradient across the image.
                reader.decode(BinaryBitmap(HybridBinarizer(source)), hints).text
            }
        } catch (e: Exception) {
            // NotFoundException is the common case by far, and every other failure here —
            // a checksum, a format, a malformed symbol — means the same thing to the caller:
            // this frame holds no code worth returning.
            null
        }
    }
}
