// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify it under the
// terms of the GNU General Public License as published by the Free Software
// Foundation, either version 3 of the License, or (at your option) any later version.

package com.gmlewis.gonomadnet

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

/**
 * The preview size a scan asks a camera for.
 *
 * It is asserted here rather than on the tablet because it is a decision, and a decision made
 * per device is one that is only ever checked on the device in hand. The rule matters: every
 * preview frame is searched for a QR code between the frames the reader is aiming with, so a
 * camera asked for its largest size is a stuttering preview that reads a code no sooner.
 *
 * The floor is what a code in front of the lens costs in pixels. A code printed on a page and
 * filling the frame resolves at VGA; a code on a screen, held far enough away to be readable as
 * a whole screen, fills about a fifth of it and resolves at 720p — and no smaller.
 */
class ScanPreviewSizeTest {

    /** sizes is a list of supported sizes, as a camera reports them. */
    private fun sizes(vararg pairs: Pair<Int, Int>): List<PreviewSize> =
        pairs.map { PreviewSize(it.first, it.second) }

    @Test
    fun `the smallest size that is big enough is chosen`() {
        val camera = sizes(
            320 to 240,
            640 to 480,
            1280 to 720,
            1920 to 1080,
            4032 to 3024,
        )
        assertEquals(PreviewSize(1280, 720), choosePreviewSize(camera))
    }

    @Test
    fun `the pixel's own list gives 720p`() {
        // The tablet's own list, pinned whole: 640x480 is on it, and a code on a screen read
        // at that size is about four pixels to a module.
        val camera = sizes(
            1920 to 1920, 2560 to 1280, 2048 to 1536, 1920 to 1440, 1920 to 1080,
            1600 to 1200, 1920 to 960, 1792 to 1008, 1440 to 1080, 1344 to 1008,
            1280 to 960, 1080 to 1080, 1280 to 720, 1024 to 768, 1024 to 576,
            800 to 600, 720 to 480, 640 to 480, 640 to 360, 352 to 288,
            320 to 240, 176 to 144,
        )
        assertEquals(PreviewSize(1280, 720), choosePreviewSize(camera))
    }

    @Test
    fun `a camera with nothing as big as the floor gets its largest size`() {
        // A camera that cannot manage the floor still reads a code held close enough, and a
        // refusal would be a device with no scanner at all.
        val camera = sizes(176 to 144, 320 to 240)
        assertEquals(PreviewSize(320, 240), choosePreviewSize(camera))
    }

    @Test
    fun `a VGA-only camera still gets VGA`() {
        // The floor is a preference, not a floor: the largest a camera has is still asked for.
        val camera = sizes(320 to 240, 640 to 480)
        assertEquals(PreviewSize(640, 480), choosePreviewSize(camera))
    }

    @Test
    fun `a camera whose sizes are all large gets its smallest`() {
        val camera = sizes(1920 to 1080, 2560 to 1440, 3840 to 2160)
        assertEquals(PreviewSize(1920, 1080), choosePreviewSize(camera))
    }

    @Test
    fun `a portrait size is turned to landscape before it is judged`() {
        // Some devices report their preview sizes with the long edge as the height. The
        // frame is stretched to the view it is drawn in either way, so what is being chosen
        // is the pixel count, not the shape.
        val camera = sizes(480 to 640, 1080 to 1920)
        assertEquals(PreviewSize(1920, 1080), choosePreviewSize(camera))
    }

    @Test
    fun `a size of no pixels is not a size`() {
        val camera = sizes(0 to 0, 640 to 480)
        assertEquals(PreviewSize(640, 480), choosePreviewSize(camera))
    }

    @Test
    fun `a camera that offers no sizes at all chooses nothing`() {
        assertNull(choosePreviewSize(emptyList()))
        assertNull(choosePreviewSize(sizes(0 to 0)))
    }

    @Test
    fun `the floor is 720p`() {
        // The rule the choice is made on, pinned where it is stated: a code on a screen fills
        // a fifth of the frame, and a fifth of 720p is what resolves into finder patterns.
        assertEquals(1280 * 720, MIN_SCAN_PIXELS)
    }
}
