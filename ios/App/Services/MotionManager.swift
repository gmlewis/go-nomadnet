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
import CoreMotion
import Foundation

/// MotionManager is the appliance's compass: CoreMotion's fused device attitude,
/// turned into the heading the reader's chosen axis measures.
///
/// It uses the device-motion attitude, which CoreMotion has already fused from the
/// gyroscope, the accelerometer and the magnetometer. The raw magnetometer is never
/// read: it is noisy, it is disturbed by everything magnetic within a metre of the
/// device, and a heading that jumps is worse than one that is briefly unavailable.
///
/// The heading itself is not computed here. `HeadingReference` in ApplianceKit does
/// that from the attitude and the reader's axis, because that is the part worth
/// testing, and it is tested without a device.
public final class MotionManager {
    /// Reading is one orientation sample, in the appliance's own terms.
    public struct Reading {
        /// The magnetic heading in degrees, when the chosen axis measures one.
        public let magnetic: Double?

        /// The true heading in degrees, when the appliance has a position to compute
        /// the magnetic variation from.
        public let trueHeading: Double?

        /// The tilt angles in degrees.
        public let pitch: Double
        public let roll: Double

        /// Whether a heading could be measured at all.
        ///
        /// It cannot when the reader's reference axis points near vertical: the compass
        /// then has no direction to point in, and a heading computed from it would be a
        /// wild guess. The appliance withholds the heading rather than publishing one.
        public let illConditioned: Bool
    }

    /// onReading is called on the main queue for every attitude sample.
    public var onReading: ((Reading) -> Void)?

    /// axis is which way the device measures the world: from the back of the case, or
    /// from the top edge of the screen.
    ///
    /// It is the reader's choice because it is a question about how the device is being
    /// held. On a stand or in a keyboard case, the back of the case faces the way the
    /// device is pointing. Lying flat on a table, the top edge does.
    public var axis: HeadingAxis = .screenBack

    /// location supplies the position the magnetic variation is computed from.
    ///
    /// It is a closure rather than a value because the position arrives on its own
    /// schedule, and a copy taken at start-up would be the position the appliance had
    /// when it was switched on.
    public var location: (() -> LocationManager.Sample?)?

    /// displayRotation is the screen's current orientation as a quarter turn, which is
    /// what tells `screen-top` which edge of the case the screen's top is.
    public var displayRotation: Int = 0

    private let manager = CMMotionManager()
    private let queue = OperationQueue()

    public init() {
        queue.name = "gonomadnet.motion"
        queue.qualityOfService = .utility
    }

    /// isAvailable reports whether the device has an attitude sensor at all.
    public var isAvailable: Bool { manager.isDeviceMotionAvailable }

    /// start begins sampling and returns whether sampling could be started.
    ///
    /// The sample period is 20 ms, the rate the Android appliance samples at. It is far
    /// more than a heading needs, and it is what makes the reading settle quickly after
    /// the reader turns the device: the appliance publishes one sentence per sample, so
    /// the client and the bot always have the freshest one.
    @discardableResult
    public func start() -> Bool {
        guard manager.isDeviceMotionAvailable else { return false }
        guard !manager.isDeviceMotionActive else { return true }

        manager.deviceMotionUpdateInterval = Self.samplePeriod
        manager.startDeviceMotionUpdates(to: queue) { [weak self] motion, _ in
            guard let self, let motion else { return }
            let reading = self.reading(from: motion)
            OperationQueue.main.addOperation { self.onReading?(reading) }
        }
        return true
    }

    /// stop ends sampling.
    public func stop() {
        manager.stopDeviceMotionUpdates()
    }

    /// reading turns one CoreMotion attitude into the appliance's reading.
    ///
    /// The rotation is CoreMotion's quaternion: the rotation that takes a vector in the
    /// device's own frame and gives it in the reference frame, where x is east, y is
    /// north and z is up.
    func reading(from motion: CMDeviceMotion) -> Reading {
        let q = motion.attitude.quaternion

        // `magneticHeadingDegrees` is the raw magnetometer's answer and is deliberately
        // not used: `HeadingReference` computes the heading from the fused attitude, so
        // that the axis the reader chose is honoured, and so that the same maths is
        // tested on a device and in a unit test.
        let attitude = HeadingReference.attitude(
            rotationVector: (x: q.x, y: q.y, z: q.z, w: q.w),
            axis: axis,
            displayRotation: displayRotation
        )

        return Reading(
            magnetic: attitude.heading,
            trueHeading: trueHeading(attitude),
            pitch: attitude.pitch,
            roll: attitude.roll,
            illConditioned: attitude.illConditioned
        )
    }

    /// trueHeading corrects a magnetic heading for the local variation, or returns nil
    /// when the appliance has no position to compute it from.
    ///
    /// A magnetic heading reported as a true one is wrong by up to twenty degrees, and
    /// on a device whose whole purpose is to say which way to walk, that is not a
    /// rounding error. So a heading with no position stays magnetic, and says so.
    private func trueHeading(_ attitude: HeadingReading) -> Double? {
        guard let attitudeMagnetic = attitude.heading else { return nil }
        guard let fix = location?() else { return nil }
        return HeadingReference.trueHeading(
            magnetic: attitudeMagnetic,
            lat: fix.latitude,
            lon: fix.longitude,
            altitude: fix.altitude ?? 0,
            at: fix.timestamp
        )
    }

    /// samplePeriod is how often the attitude is sampled: 20 ms, or about 50 Hz.
    static let samplePeriod: TimeInterval = 0.02
}
