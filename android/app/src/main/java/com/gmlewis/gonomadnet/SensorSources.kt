// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import android.content.Context
import android.hardware.GeomagneticField
import android.hardware.Sensor
import android.hardware.SensorEvent
import android.hardware.SensorEventListener
import android.hardware.SensorManager
import android.location.Location
import android.location.LocationListener
import android.location.LocationManager
import android.os.Bundle
import android.os.Handler
import android.os.Looper

/**
 * The tablet's own position, and the provider that produced it.
 *
 * Both providers are registered, and the fix that arrives is used whichever one sent
 * it: a GPS fix is preferred for its accuracy, but a network fix is a real position and
 * is better than none. Which provider answered travels with the sample, so nothing
 * downstream has to guess, and so a consumer can tell a satellite fix from a cell fix
 * before trusting it.
 */
class LocationSource(
    private val context: Context,
    private val onSample: (String) -> Unit,
    private val log: (String) -> Unit = {},
) : LocationListener, SampleSource {

    private var manager: LocationManager? = null
    private var lastProvider: String = ""
    private var lastFixAtMs: Long = 0

    // The most recent fix, cached.
    //
    // getLastKnownLocation is a binder call into the system location service, and the heading
    // source asks for the position on every sensor event — fifty times a second. Asking the
    // system fifty times a second for a value it produced once is a measurable battery cost and
    // it floods the platform log, which is how this was noticed:
    //     LocationManagerService: gps provider getLastLocation from : 10300/com.gmlewis.gonomadnet
    // The cache is refreshed by onLocationChanged and by a rate-limited re-query, so a client
    // that has never had a fix still picks one up when the receiver produces one.
    private var cachedFix: Location? = null
    private var cachedAtMs: Long = 0

    /** When a fix last arrived, or zero. */
    fun lastFixAgeMs(nowMs: Long): Long = if (lastFixAtMs == 0L) -1 else nowMs - lastFixAtMs

    override fun start(): Boolean {
        val locationManager = context.getSystemService(Context.LOCATION_SERVICE) as? LocationManager
            ?: return false
        manager = locationManager
        var registered = 0
        for (provider in listOf(LocationManager.GPS_PROVIDER, LocationManager.NETWORK_PROVIDER)) {
            try {
                locationManager.requestLocationUpdates(provider, MIN_INTERVAL_MS, 0f, this, Looper.getMainLooper())
                registered += 1
            } catch (failure: Throwable) {
                log("could not request $provider: ${failure.message}")
            }
        }
        log("requesting $registered location provider(s)")
        return registered > 0
    }

    override fun close() {
        manager?.let { runCatching { it.removeUpdates(this) } }
        manager = null
    }

    override fun onLocationChanged(location: Location) {
        lastProvider = location.provider ?: ""
        lastFixAtMs = System.currentTimeMillis()
        cachedFix = location
        cachedAtMs = lastFixAtMs
        onSample(
            SensorJson.location(
                timeMs = location.time,
                provider = lastProvider,
                lat = location.latitude,
                lng = location.longitude,
                altitude = if (location.hasAltitude()) location.altitude else null,
                accuracyMeters = if (location.hasAccuracy()) location.accuracy else null,
                speedKnots = null,
                courseDeg = if (location.hasBearing()) location.bearing.toDouble() else null,
                satellites = location.extras?.getInt("satellites", 0)?.takeIf { it > 0 },
                quality = if (location.hasAccuracy()) 1 else null,
            ),
        )
    }

    override fun onProviderEnabled(provider: String) { log("$provider is enabled") }
    override fun onProviderDisabled(provider: String) { log("$provider is disabled") }

    /**
     * The most recent fix, cached.
     *
     * It is asked for on every sensor event by the heading source, so it must not be a binder
     * call every time. The cache is good for [CACHE_TTL_MS], which is long enough that a
     * fifty-hertz caller costs nothing and short enough that a client which has never had a fix
     * still picks one up as soon as the receiver produces one.
     */
    fun lastKnown(): Location? {
        val now = System.currentTimeMillis()
        if (cachedFix != null && now - cachedAtMs < CACHE_TTL_MS) {
            return cachedFix
        }
        val fresh = try {
            manager?.getLastKnownLocation(LocationManager.GPS_PROVIDER)
                ?: manager?.getLastKnownLocation(LocationManager.NETWORK_PROVIDER)
        } catch (failure: Throwable) {
            null
        }
        cachedAtMs = now
        if (fresh != null) {
            cachedFix = fresh
        }
        return cachedFix
    }

    companion object {
        /** One fix per second; the receiver fixes far slower than that anyway. */
        const val MIN_INTERVAL_MS = 1_000L

        /** How long the cached position is trusted before the platform is asked again. */
        const val CACHE_TTL_MS = 1_000L
    }
}

/**
 * The tablet's heading, from the fused rotation vector.
 *
 * The axis is a setting, and its default is the screen-back normal: the direction the
 * back of the tablet points, which is where the operator holding it is looking. Android's
 * own getOrientation answers along the device's +Y axis instead, which on a tablet in
 * landscape — standing on a stand, rotated ninety degrees — is a stable, confident
 * bearing very nearly ninety degrees away from the truth. See [HeadingReference].
 *
 * The raw magnetometer is never used. On this tablet it reports a field around forty-five
 * times the Earth's and never once reports an accuracy status, while the fused vector
 * reports HIGH and agrees with the accelerometer exactly.
 */
class HeadingSource(
    private val context: Context,
    private val onSample: (String) -> Unit,
    private val log: (String) -> Unit = {},
) : SensorEventListener, SampleSource {

    /** Which axis the heading is measured along. */
    var axis: HeadingAxis = HeadingAxis.SCREEN_BACK

    private var manager: SensorManager? = null
    private var rotation: Sensor? = null
    private var displayRotation: () -> Int = { 0 }
    private var positionAt: () -> Triple<Double, Double, Double>? = { null }
    private var lastAccuracy: Int = -1
    private var reportedDegenerate: Boolean? = null

    /** The most recent attitude, for the status readout. */
    var attitude: Attitude? = null
        private set

    /** The display rotation source, so the screen-top reference follows the display. */
    fun setDisplayRotation(source: () -> Int) {
        displayRotation = source
    }

    /** The position source the true-north correction is computed at. */
    fun setPositionSource(source: () -> Triple<Double, Double, Double>?) {
        positionAt = source
    }

    /** The fused rotation vector's own accuracy status, or -1 before it reports one. */
    fun accuracy(): Int = lastAccuracy

    override fun start(): Boolean {
        val sensorManager = context.getSystemService(Context.SENSOR_SERVICE) as? SensorManager ?: return false
        manager = sensorManager
        rotation = sensorManager.getDefaultSensor(Sensor.TYPE_ROTATION_VECTOR)
        if (rotation == null) {
            log("this device has no rotation vector; there will be no heading")
            return false
        }
        sensorManager.registerListener(this, rotation, SAMPLE_PERIOD_US, Handler(Looper.getMainLooper()))
        log("reading ${rotation!!.name} every ${SAMPLE_PERIOD_US / 1000} ms")
        return true
    }

    override fun close() {
        manager?.let { runCatching { it.unregisterListener(this) } }
        manager = null
    }

    override fun onSensorChanged(event: SensorEvent) {
        if (event.sensor.type != Sensor.TYPE_ROTATION_VECTOR) {
            return
        }
        val measured = HeadingReference.attitude(event.values, axis, displayRotation())
        attitude = measured

        // An ill-conditioned reference axis is reported by withholding the heading, not by
        // publishing the number it happened to produce: the consumer's contract is that a
        // missing heading field means "no heading", which is exactly true.
        //
        // But withholding it in silence is not enough. A reader whose tablet is lying flat and
        // whose heading axis is the screen-back normal gets no heading at all, and without a
        // word about why they would reasonably conclude the compass is broken. The transition
        // is therefore announced once, with the attitude that caused it, and announced again
        // when the heading comes back.
        announceCondition(measured)

        val heading = if (measured.illConditioned) null else measured.headingDeg
        val frame = if (trueHeading(measured.headingDeg) != null) SensorJson.FRAME_TRUE else SensorJson.FRAME_MAGNETIC
        onSample(
            SensorJson.heading(
                timeMs = System.currentTimeMillis(),
                headingDeg = frameHeading(heading, frame),
                frame = frame,
            ),
        )
    }

    /**
     * Reports the ill-conditioned case exactly once per transition, in the operator's terms.
     *
     * The two reference axes degenerate in opposite mountings: the screen-back normal when the
     * tablet lies flat, and the screen-top axis when it stands upright in portrait. Naming
     * which one applies, and what to do about it, is the difference between a compass that is
     * broken and a compass that is being asked the wrong question.
     */
    private fun announceCondition(measured: Attitude) {
        if (reportedDegenerate == measured.illConditioned) {
            return
        }
        reportedDegenerate = measured.illConditioned
        if (!measured.illConditioned) {
            log("heading available again: ${measured.headingDeg.toInt()} deg true")
            return
        }
        val which = when (axis) {
            HeadingAxis.SCREEN_BACK ->
                "the tablet is lying flat, so the direction its back points is nearly straight down"
            HeadingAxis.SCREEN_TOP ->
                "the tablet is standing up in portrait, so the top edge of the screen is nearly straight up"
        }
        log(
            "no heading: $which, which makes the reference axis ill-conditioned and its azimuth " +
                "noise. The reading is withheld rather than guessed. $SUGGESTION",
        )
    }

    override fun onAccuracyChanged(sensor: Sensor?, accuracy: Int) {
        if (sensor?.type == Sensor.TYPE_ROTATION_VECTOR) {
            lastAccuracy = accuracy
            log("rotation vector accuracy is now $accuracy")
        }
    }

    /**
     * Converts a magnetic heading to true north at the tablet's position, using the same
     * World Magnetic Model the bot uses, or null when there is no position to correct
     * against. A heading that cannot be corrected is published as magnetic rather than
     * labelled true, because the two differ by up to twenty degrees.
     */
    private fun trueHeading(magneticDeg: Double): Double? {
        val position = positionAt() ?: return null
        val field = GeomagneticField(position.first.toFloat(), position.second.toFloat(), position.third.toFloat(), System.currentTimeMillis())
        return HeadingReference.normalizeDegrees(magneticDeg + field.declination)
    }

    private fun frameHeading(headingDeg: Double?, frame: String): Double? {
        if (headingDeg == null) {
            return null
        }
        // The rotation vector is referenced to magnetic north, so the correction is what
        // makes a heading true. When there is no position it stays magnetic and the frame
        // says so.
        return if (frame == SensorJson.FRAME_TRUE) trueHeading(headingDeg) else headingDeg
    }

    companion object {
        /** About 50 Hz, which is what the fused sensor reports on this tablet. */
        const val SAMPLE_PERIOD_US = 20_000

        /** What the operator can do about the ill-conditioned case. */
        const val SUGGESTION = "Stand the tablet up to use the screen-back heading, or choose the screen-top axis in the settings."
    }
}
