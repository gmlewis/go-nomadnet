// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify it under the
// terms of the GNU General Public License as published by the Free Software
// Foundation, either version 3 of the License, or (at your option) any later version.

package com.gmlewis.gonomadnet

/**
 * The camera preview size a scan should ask for.
 *
 * It is a data class of its own rather than [android.hardware.Camera.Size] because the
 * platform's type cannot be built off a device, and the rule for choosing among the sizes a
 * camera offers is the one part of the preview worth asserting without one.
 */
data class PreviewSize(val width: Int, val height: Int) {

    /** pixels is the frame's area, which is what the choice below is made on. */
    val pixels: Int get() = width * height

    /** landscape returns the size with the long edge as its width. */
    fun landscape(): PreviewSize = if (width >= height) this else PreviewSize(height, width)
}

/**
 * choosePreviewSize returns the preview size to ask a camera for.
 *
 * The smallest size that is at least [MIN_SCAN_PIXELS], or the largest there is when the
 * camera offers nothing that big.
 *
 * Small is the point. Every preview frame is copied, turned into a bitmap and searched for
 * finder patterns, and that work happens between the frames the reader is aiming with: a
 * camera asked for its largest size decodes a code about as often as one asked for VGA, while
 * spending several times as long per frame doing it, which shows up as a preview that stutters
 * and a code that takes longer to catch. 640x480 leaves a QR code far more pixels than the
 * decoder needs and leaves the preview smooth.
 *
 * The aspect ratio is deliberately not part of the rule. A camera's preview sizes are the
 * shapes its sensor can produce, and the one chosen here is stretched to fill whatever view
 * it is drawn in: a reader aiming a tablet at a code cares that the picture is legible, not
 * that a circle in it is round.
 */
fun choosePreviewSize(sizes: List<PreviewSize>): PreviewSize? {
    val landscape = sizes.map { it.landscape() }.filter { it.width > 0 && it.height > 0 }
    if (landscape.isEmpty()) {
        return null
    }
    val bigEnough = landscape.filter { it.pixels >= MIN_SCAN_PIXELS }
    if (bigEnough.isNotEmpty()) {
        return bigEnough.minByOrNull { it.pixels }
    }
    // Nothing as big as VGA: the largest there is, because a camera that small is the one
    // where every pixel counts, and the reader can still hold the code closer.
    return landscape.maxByOrNull { it.pixels }
}

/**
 * MIN_SCAN_PIXELS is the smallest preview worth scanning: 640x480.
 *
 * It is VGA because that is the size at which a QR code printed on a page and held at arm's
 * length still resolves into enough pixels for the decoder's finder patterns, which is the
 * furthest a reader can reasonably be asked to hold a tablet from a code.
 */
const val MIN_SCAN_PIXELS: Int = 640 * 480
