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
 * The focus mode a scan puts the camera in.
 *
 * It is asserted here rather than on a device because the failure it guards against is silent:
 * the platform's default focus mode is `FOCUS_MODE_AUTO`, which does nothing at all until it is
 * triggered, so a scanner that never names a mode looks exactly like a scanner whose camera
 * cannot focus — a soft preview, no error, and a code that never reads. The strings are pinned
 * as literals because they are the framework's own contract; a misspelling is a camera that
 * quietly keeps the default.
 */
class ScanFocusTest {

    @Test
    fun `a camera that can focus continuously is asked to`() {
        // The Pixel 10 Pro XL's own list, which is what the appliance meets in the hand.
        val camera = listOf(
            "infinity", "auto", "macro", "continuous-video", "continuous-picture", "edof",
        )
        assertEquals("continuous-picture", chooseFocusMode(camera))
    }

    @Test
    fun `continuous video is taken when continuous picture is not offered`() {
        assertEquals("continuous-video", chooseFocusMode(listOf("auto", "continuous-video")))
    }

    @Test
    fun `auto is the last resort, and is still worth setting`() {
        // A camera that only autofocuses on request is still better asked than not: the ticker
        // in ScanActivity is what makes this mode work.
        assertEquals("auto", chooseFocusMode(listOf("infinity", "auto", "macro")))
    }

    @Test
    fun `a camera with no mode worth setting is left alone`() {
        // EDOF and infinity are focused by construction — there is no lens to move, and setting
        // a mode would only take the camera away from what it already does well.
        assertNull(chooseFocusMode(listOf("infinity", "macro", "edof")))
        assertNull(chooseFocusMode(emptyList()))
        assertNull(chooseFocusMode(null))
    }

    @Test
    fun `the preference is continuous before on-request`() {
        // Stated as the rule rather than as a list: whatever else a camera offers, a mode that
        // keeps the lens moving beats one that has to be prodded.
        val camera = listOf("auto", "continuous-picture")
        assertEquals("continuous-picture", chooseFocusMode(camera))
        assertEquals("continuous-video", chooseFocusMode(listOf("auto", "continuous-video")))
    }
}
