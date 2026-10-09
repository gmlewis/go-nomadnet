// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.TimeZone
import kotlin.math.abs

/**
 * One sensor reading, serialized to the single JSON line that `gonsensor` reads
 * on its standard input and turns into NMEA-0183 sentences.
 *
 * The format is deliberately the one `gonsensor` documents: a sample describing a
 * fix carries a timestamp, a provider, a position, a validity flag, and optionally
 * an accuracy, a speed, a course, a satellite count and a quality indicator; a
 * sample describing a heading carries an angle and the frame it is measured in.
 * Keeping the serialization here, in pure Kotlin, is what lets the whole Android
 * side be tested on the JVM with no device attached.
 */
object SensorJson {

    /** The frame a magnetic heading is measured in; Android's rotation vector uses it. */
    const val FRAME_MAGNETIC = "magnetic"

    /** The frame a heading the producer has already corrected is measured in. */
    const val FRAME_TRUE = "true"

    /**
     * Renders one position sample. A position that is not finite is written as
     * invalid rather than dropped, so the consumer is told there is no fix instead
     * of being left holding the last one.
     */
    fun location(
        timeMs: Long,
        provider: String,
        lat: Double,
        lng: Double,
        altitude: Double? = null,
        accuracyMeters: Float? = null,
        speedKnots: Double? = null,
        courseDeg: Double? = null,
        satellites: Int? = null,
        quality: Int? = null,
        valid: Boolean = true,
    ): String {
        val usable = valid && finite(lat) && finite(lng) && abs(lat) <= 90.0 && abs(lng) <= 180.0
        return buildString {
            append("{\"t\":\"").append(rfc3339(timeMs)).append('"')
            append(",\"provider\":\"").append(escape(provider)).append('"')
            append(",\"valid\":").append(usable)
            if (usable) {
                append(",\"lat\":").append(lat)
                append(",\"lng\":").append(lng)
                altitude?.let { if (finite(it)) append(",\"alt\":").append(it) }
                accuracyMeters?.let { if (finite(it.toDouble())) append(",\"acc\":").append(it) }
                speedKnots?.let { if (finite(it)) append(",\"speed\":").append(it) }
                courseDeg?.let { if (finite(it)) append(",\"course\":").append(it) }
                satellites?.let { if (it > 0) append(",\"sats\":").append(it) }
                quality?.let { if (it > 0) append(",\"quality\":").append(it) }
            }
            append('}')
        }
    }

    /**
     * Renders one heading sample. A heading that is not a number is written as a
     * sample with no heading field at all, which is how the producer says "I am not
     * confident enough to answer" — the one signal the consumer must honour.
     */
    fun heading(
        timeMs: Long,
        headingDeg: Double?,
        frame: String = FRAME_MAGNETIC,
    ): String = buildString {
        append("{\"t\":\"").append(rfc3339(timeMs)).append('"')
        headingDeg?.let {
            if (finite(it)) {
                append(",\"heading\":").append(HeadingReference.normalizeDegrees(it))
                append(",\"frame\":\"").append(escape(frame)).append('"')
            }
        }
        append('}')
    }

    /**
     * Renders an RFC 3339 UTC timestamp with milliseconds, which is what the
     * consumer parses. The device's own clock is used, since a sensor reading
     * without the instant it was taken is not much use to anybody.
     */
    fun rfc3339(epochMs: Long): String =
        formatter().format(Date(epochMs))

    private fun formatter(): SimpleDateFormat =
        SimpleDateFormat("yyyy-MM-dd'T'HH:mm:ss.SSS'Z'", Locale.US).apply {
            timeZone = TimeZone.getTimeZone("UTC")
        }

    private fun finite(value: Double): Boolean = !value.isNaN() && !value.isInfinite()

    /**
     * Escapes a string for inclusion in a JSON string literal. A provider name comes
     * from the platform and is not attack-controlled, but a quote in it would still
     * truncate the line and silently lose the reading.
     */
    private fun escape(text: String): String {
        val escaped = StringBuilder(text.length + 2)
        for (character in text) {
            when (character) {
                '\\' -> escaped.append("\\\\")
                '"' -> escaped.append("\\\"")
                else -> escaped.append(character)
            }
        }
        return escaped.toString()
    }

}
