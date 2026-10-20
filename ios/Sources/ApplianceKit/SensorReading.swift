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

/// One sample the sensor pipeline accepted, as the value the appliance publishes.
///
/// The pipeline is the only thing that decides what is worth publishing — it holds the
/// rate limit and the sanity checks, and nothing else in the appliance repeats them — so
/// what it accepts is what is published. This is that decision's result as a value.
///
/// The appliance feeds the Go half these structured values, and the Go half emits the NMEA
/// sentences its receivers parse; nothing is serialized here.
public enum SensorReading: Equatable, Sendable {
    /// A position the appliance will publish.
    case fix(AcceptedFix)
    /// A heading the appliance will publish, or one it deliberately will not.
    case heading(AcceptedHeading)
}

/// A position the pipeline accepted.
///
/// Every measurement that might not have been taken is optional, and absent means absent:
/// a height nobody measured is not sea level, and a speed nobody measured is not standing
/// still. The Go half writes an empty NMEA field for each of them, and a reader cannot
/// tell a measured zero from a fabricated one — so the difference is carried here.
public struct AcceptedFix: Equatable, Sendable {
    /// When the sample arrived, which is what the NMEA sentences are stamped with.
    public var time: Date
    /// What measured it, such as a GNSS receiver or a network source.
    public var provider: String
    /// The position in signed decimal degrees.
    public var latitude: Double
    public var longitude: Double
    /// The antenna height above mean sea level, when the receiver reported one.
    public var altitude: Double?
    /// The receiver's own estimate of the radius it expects the position to be within.
    public var accuracyMeters: Double?
    /// Speed over ground in knots, when the receiver reported one.
    public var speedKnots: Double?
    /// True track in degrees, when the receiver reported one.
    public var courseDeg: Double?

    /// Creates one accepted fix.
    public init(
        time: Date,
        provider: String,
        latitude: Double,
        longitude: Double,
        altitude: Double? = nil,
        accuracyMeters: Double? = nil,
        speedKnots: Double? = nil,
        courseDeg: Double? = nil
    ) {
        self.time = time
        self.provider = provider
        self.latitude = latitude
        self.longitude = longitude
        self.altitude = altitude
        self.accuracyMeters = accuracyMeters
        self.speedKnots = speedKnots
        self.courseDeg = courseDeg
    }
}

/// A heading the pipeline accepted.
public struct AcceptedHeading: Equatable, Sendable {
    /// When the sample arrived.
    public var time: Date
    /// The heading in `0..<360`, or `nil` for a reading the device could not make.
    ///
    /// A withheld reading is published as an accepted sample with no heading in it: that
    /// is how the producer says it is not confident enough to answer, and it is not the
    /// same as a heading of zero, which is due north.
    public var headingDeg: Double?
    /// The frame the heading is measured in.
    public var frame: HeadingFrame

    /// Creates one accepted heading.
    public init(time: Date, headingDeg: Double?, frame: HeadingFrame = .magnetic) {
        self.time = time
        self.headingDeg = headingDeg
        self.frame = frame
    }
}
