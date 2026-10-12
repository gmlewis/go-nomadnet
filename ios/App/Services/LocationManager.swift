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

import ApplianceKit
import CoreLocation
import Foundation

/// LocationManager is the appliance's GNSS receiver: CoreLocation, turned into the
/// appliance's own position samples.
///
/// It reports a fix only when iOS says the fix is usable. A `CLLocation` with a
/// negative horizontal accuracy is the platform's way of saying it has no position at
/// all, and a fix with no coordinates is not a fix: publishing one would put the
/// appliance at 0°N 0°E in the Gulf of Guinea and, worse, hand the bot a position to
/// answer from.
///
/// The appliance asks for the highest accuracy it can get and accepts that it costs
/// battery, because this is a device whose whole point is to know where it is when
/// nothing else does. It never asks for background location updates on its own: the
/// reader is looking at the screen that shows the position.
public final class LocationManager: NSObject, CLLocationManagerDelegate {
    /// Sample is one fix, in the appliance's own terms.
    ///
    /// Every measurement iOS might not have taken is optional, because a receiver that
    /// did not report a speed has not reported zero knots, and the NMEA sentence this
    /// becomes has an empty field for exactly that.
    public struct Sample {
        /// The position in signed decimal degrees.
        public let latitude: Double
        public let longitude: Double

        /// The antenna height above mean sea level in meters, when it was measured.
        public let altitude: Double?

        /// The radius the position is expected to lie within, in meters.
        public let horizontalAccuracy: Double?

        /// The speed over ground in knots, when iOS reported one.
        public let speedKnots: Double?

        /// The true course in degrees, when iOS reported one.
        public let courseDegrees: Double?

        /// When the fix was taken, by the receiver's own clock.
        public let timestamp: Date
    }

    /// onSample is called on the main queue for every usable fix.
    public var onSample: ((Sample) -> Void)?

    /// authorization is how the reader's permission currently stands.
    public private(set) var authorization: CLAuthorizationStatus = .notDetermined

    private let manager = CLLocationManager()

    /// The last fix, kept so a reader who asks "where am I" between updates is answered
    /// from a measurement rather than from nothing.
    private var lastKnown: Sample?

    public override init() {
        super.init()
        manager.delegate = self
        manager.desiredAccuracy = kCLLocationAccuracyBest
        manager.distanceFilter = kCLDistanceFilterNone
        authorization = manager.authorizationStatus
    }

    /// requestPermission asks the reader for location access.
    ///
    /// It is called from the controls, so the prompt arrives when the reader has decided
    /// to start the sensors, and never at launch.
    public func requestPermission() {
        manager.requestWhenInUseAuthorization()
    }

    /// start begins updates and returns whether they could be started.
    ///
    /// A device with location services switched off is not an error: the appliance runs
    /// without a position, and the bot answers from what it knows instead.
    @discardableResult
    public func start() -> Bool {
        guard CLLocationManager.locationServicesEnabled() else { return false }
        switch manager.authorizationStatus {
        case .authorizedWhenInUse, .authorizedAlways:
            manager.startUpdatingLocation()
            return true
        case .notDetermined:
            manager.requestWhenInUseAuthorization()
            return false
        default:
            return false
        }
    }

    /// stop ends updates.
    public func stop() {
        manager.stopUpdatingLocation()
    }

    /// lastFix is the most recent usable fix, if any has arrived.
    public var lastFix: Sample? { lastKnown }

    // MARK: - CLLocationManagerDelegate

    public func locationManagerDidChangeAuthorization(_ manager: CLLocationManager) {
        authorization = manager.authorizationStatus
        if authorization == .authorizedWhenInUse || authorization == .authorizedAlways {
            manager.startUpdatingLocation()
        }
    }

    public func locationManager(_ manager: CLLocationManager, didUpdateLocations locations: [CLLocation]) {
        guard let location = locations.last else { return }
        guard let sample = Self.sample(from: location) else { return }
        lastKnown = sample
        onSample?(sample)
    }

    public func locationManager(_ manager: CLLocationManager, didFailWithError error: Error) {
        // A receiver that cannot see the sky yet is the ordinary state of a device that
        // has just been switched on, and CoreLocation reports it as an error. Nothing is
        // reported upward: the reader is told by the position readout staying empty, and
        // a log line per failure would bury everything else at one per second.
    }

    /// sample turns a CoreLocation fix into the appliance's own sample, or nil when the
    /// platform did not really have a position.
    static func sample(from location: CLLocation) -> Sample? {
        // A negative horizontal accuracy is CoreLocation's "invalid" marker, and the
        // coordinate in such a fix is meaningless.
        guard location.horizontalAccuracy >= 0 else { return nil }
        guard CLLocationCoordinate2DIsValid(location.coordinate) else { return nil }

        let altitude: Double? = location.verticalAccuracy >= 0 ? location.altitude : nil

        // A negative speed means the receiver could not measure it; a course of -1 is
        // the same statement about the track.
        let speed: Double? = location.speed >= 0 ? location.speed * Self.knotsPerMetrePerSecond : nil
        let course: Double? = location.course >= 0 ? location.course : nil

        return Sample(
            latitude: location.coordinate.latitude,
            longitude: location.coordinate.longitude,
            altitude: altitude,
            horizontalAccuracy: location.horizontalAccuracy,
            speedKnots: speed,
            courseDegrees: course,
            timestamp: location.timestamp
        )
    }

    /// knotsPerMetrePerSecond converts CoreLocation's metres per second into the knots
    /// the NMEA the client parses reports speed in.
    static let knotsPerMetrePerSecond = 1.9438444924406046
}
