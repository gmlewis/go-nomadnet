// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify it under the
// terms of the GNU General Public License as published by the Free Software
// Foundation, either version 3 of the License, or (at your option) any later version.

package com.gmlewis.gonomadnet

import android.hardware.Camera

/**
 * The focus mode a scan puts the camera in.
 *
 * The platform's own default is [Camera.Parameters.FOCUS_MODE_AUTO], and that default is inert:
 * the lens is left wherever it was until something calls `autoFocus()`, so a camera opened and
 * left alone photographs whatever distance it was already focused at. A screen held at arm's
 * length is not that distance, and the result is not an error but a preview that is quietly
 * soft — which is a code that never reads, with nothing anywhere to say why.
 *
 * So a mode is named, and the ones that keep the lens moving are named first. Continuous picture
 * is what a still subject wants; continuous video is the same with a lower frame rate and is
 * what some cameras will only offer; [Camera.Parameters.FOCUS_MODE_AUTO] is the last resort and
 * only works because [ScanActivity] keeps prodding it.
 *
 * A camera that offers none of the three — one with only `infinity`, `macro` or `edof` — is
 * returned null for, and is left exactly as it is. That is not a failure: an EDOF or fixed-focus
 * camera has no lens to move and is already as sharp as it will ever be, and setting a mode on
 * one would only take it away from what it does well.
 *
 * [ScanFocusTest] pins the preference, because the strings are the framework's contract and a
 * misspelling fails silently.
 */
fun chooseFocusMode(supported: List<String>?): String? {
    if (supported == null) {
        // A camera that will not say what it can do is not one to be guessed at.
        return null
    }
    return FOCUS_PREFERENCE.firstOrNull { supported.contains(it) }
}

/**
 * FOCUS_PREFERENCE is the order the modes are tried in: the two that focus continuously, then
 * the one that has to be asked.
 */
private val FOCUS_PREFERENCE = listOf(
    Camera.Parameters.FOCUS_MODE_CONTINUOUS_PICTURE,
    Camera.Parameters.FOCUS_MODE_CONTINUOUS_VIDEO,
    Camera.Parameters.FOCUS_MODE_AUTO,
)

/**
 * FOCUS_INTERVAL_MS is how often a camera that cannot focus continuously is asked to.
 *
 * It is the pace of a person moving a phone over a code rather than the pace of the sensor: the
 * lens takes a moment to travel and settle, and asking faster than it can answer only fills the
 * queue with stale requests.
 */
const val FOCUS_INTERVAL_MS: Long = 1500
