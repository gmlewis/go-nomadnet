
// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import android.os.PowerManager

/**
 * The wake lock a non-wakeup sensor needs, behind the interface the pipeline takes.
 *
 * Every sensor this appliance reads — the accelerometer, the two fused rotation vectors,
 * the magnetometer — is a non-wakeup sensor. Android stops delivering from them the moment
 * the process stops holding a wake lock, so a position that updates while the screen is on
 * and freezes when it goes off is not a position at all. Termux's own launcher achieves the
 * same thing with `termux-wake-lock`.
 */
class PowerWakeLock(private val lock: () -> PowerManager.WakeLock) : WakeLockHandle {

    override fun acquire() {
        val held = lock()
        if (!held.isHeld) {
            held.acquire()
        }
    }

    override fun release() {
        val held = lock()
        if (held.isHeld) {
            runCatching { held.release() }
        }
    }

    override fun isHeld(): Boolean = lock().isHeld
}
