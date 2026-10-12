// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

import Foundation

/// Decides which sensor samples are worth publishing, and hands the accepted ones on.
///
/// This is the pure-logic half of the sensor side: it takes a position or a heading,
/// decides whether it is worth publishing, and gives the accepted reading to a sink. It
/// holds no sensor and no socket, so the whole rate-limiting and sanity-checking policy
/// is a unit test with no device attached.
///
/// What it accepts is what is published. There is no second copy of these rules anywhere
/// in the appliance, and no reading that the pipeline approves is re-decided or re-shaped
/// after it: the sink is handed a `SensorReading`, and the appliance feeds that reading
/// to the Go half.
///
/// The pipeline does not block its caller. The caller is a device callback, and a callback
/// that blocks stalls the sensor that invoked it; a short lock around the sampling decision is
/// all this takes, and the publish itself runs outside it.
public final class SensorPipeline: @unchecked Sendable {
    /// The smallest interval between two published positions, in seconds.
    ///
    /// The reference converter's minimum interval is one second. A position stream faster than
    /// that is one the mesh has no use for: LXMF location updates are seconds apart at best,
    /// and a receiver that answers several times a second would publish the same fix over and
    /// over while burying the heading in noise.
    public static let minPositionInterval: TimeInterval = 1.0

    /// Where an accepted reading goes.
    private let publish: (SensorReading) -> Void
    /// The clock the timestamps and the rate limit are read from, injectable for tests.
    private let now: () -> Date
    /// Guards the stopped flag and the last-published instant.
    private let lock = NSLock()
    /// Whether the pipeline has been stopped, after which it publishes nothing.
    private var stopped = false
    /// When the last position was published, for the rate limit.
    private var lastPositionAt: Date?

    /// Creates a pipeline that publishes to `publish` and reads time from `now`.
    ///
    /// `now` is injected so the rate limit can be exercised without waiting a real second.
    public init(publish: @escaping (SensorReading) -> Void, now: @escaping () -> Date = Date.init) {
        self.publish = publish
        self.now = now
    }

    /// Offers one position sample, publishing it when it is fresh and sane.
    ///
    /// A position that is not a fix — a non-finite or out-of-range latitude or longitude — is
    /// dropped rather than published: the feed is for a consumer that would otherwise hold
    /// the last good fix, and a coordinate off the planet is corruption, not a position. A
    /// usable fix is published at most once per `minPositionInterval`.
    ///
    /// The fields the C boundary and NMEA have no room for are not taken here at all: the
    /// appliance publishes a position, a height, an error estimate, a speed and a course,
    /// and a satellite count that nothing in the path could carry would be a field the
    /// pipeline accepted and the appliance silently dropped.
    public func location(
        provider: String,
        lat: Double,
        lng: Double,
        altitude: Double? = nil,
        accuracyMeters: Double? = nil,
        speedKnots: Double? = nil,
        courseDeg: Double? = nil,
        valid: Bool = true
    ) {
        guard valid, lat.isFinite, lng.isFinite, abs(lat) <= 90.0, abs(lng) <= 180.0 else {
            return
        }
        let timestamp = now()
        lock.lock()
        if stopped || isRateLimited(at: timestamp) {
            lock.unlock()
            return
        }
        lastPositionAt = timestamp
        lock.unlock()

        publish(.fix(AcceptedFix(
            time: timestamp,
            provider: provider,
            latitude: lat,
            longitude: lng,
            altitude: altitude,
            accuracyMeters: accuracyMeters,
            speedKnots: speedKnots,
            courseDeg: courseDeg
        )))
    }

    /// Offers one heading sample, publishing it when it is a number.
    ///
    /// A heading is not rate-limited the way a position is: it is cheap, a magnetometer has no
    /// cost to a receiver, and the client wants the freshest bearing when it looks at a page.
    /// A heading that is `nil` is still published, as a sample with no heading in it, which is
    /// how the producer says it is not confident enough to answer; only a non-finite number is
    /// dropped, because that is corruption rather than a withheld reading.
    ///
    /// A heading that is a number is normalized into `0..<360`, so that the feed carries one
    /// convention rather than whatever the sensor's own arithmetic produced.
    public func heading(_ headingDeg: Double?, frame: HeadingFrame = .magnetic) {
        if let headingDeg, !headingDeg.isFinite {
            return
        }
        let timestamp = now()
        lock.lock()
        if stopped {
            lock.unlock()
            return
        }
        lock.unlock()

        publish(.heading(AcceptedHeading(
            time: timestamp,
            headingDeg: headingDeg.map(HeadingReference.normalizeDegrees),
            frame: frame
        )))
    }

    /// Stops the pipeline and releases everything it holds.
    ///
    /// Idempotent. After it returns, a further sample publishes nothing, which is what lets a
    /// device callback that was already in flight when the service stopped be harmless.
    public func stop() {
        lock.lock()
        stopped = true
        lastPositionAt = nil
        lock.unlock()
    }

    /// Reports whether a position arriving at `timestamp` is too soon after the last one.
    /// Must be called with the lock held.
    private func isRateLimited(at timestamp: Date) -> Bool {
        guard let last = lastPositionAt else { return false }
        return timestamp.timeIntervalSince(last) < SensorPipeline.minPositionInterval
    }
}
