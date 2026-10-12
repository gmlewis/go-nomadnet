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

/// Which device axis the heading is measured along.
///
/// There is no single "heading" for a device in an arbitrary attitude: there are two
/// natural ones, and they only agree when the device lies flat.
///
///  - `screenBack` is the direction the back of the device points, which is where the
///    operator holding it is looking. It is well conditioned while the device stands up —
///    including on a stand — or is aimed at something.
///  - `screenTop` is the device's +Y axis. It is well conditioned while the device lies
///    flat, and it answers "which way does the top edge point", which on a device in
///    landscape is not where the operator is facing at all.
///
/// The two degenerate in opposite mountings, so which one is right depends entirely on how
/// the device is held: it is a persisted setting for that reason.
public enum HeadingAxis: String, Codable, Sendable {
    /// The direction the back of the device points.
    case screenBack = "screen-back"
    /// The device's +Y axis, the top edge of the screen.
    case screenTop = "screen-top"
}

/// The frame a heading is measured in.
public enum HeadingFrame: String, Sendable {
    /// A magnetic heading, which is what a magnetometer measures.
    case magnetic
    /// A heading the producer has already corrected for magnetic declination.
    case `true`
}

/// One attitude: where the chosen reference axis points, and how much to trust it.
///
/// `illConditioned` is true when the reference axis is within
/// `HeadingReference.degenerateBandDegrees` of vertical, which is the case that makes its
/// azimuth pure noise. It is reported rather than hidden, because an operator must never be
/// handed a confident bearing that is really a rounding error; when it is true, `heading` is
/// withheld entirely.
public struct HeadingReading: Equatable, Sendable {
    /// The magnetic heading in `0..<360`, or `nil` when the reference axis is degenerate.
    public var heading: Double?
    /// The declination-corrected heading, when the caller had a position to correct with.
    public var trueHeading: Double?
    /// The frame `heading` is measured in.
    public var frame: HeadingFrame
    /// The pitch in degrees, in the familiar device-orientation convention.
    public var pitch: Double
    /// The roll in degrees, in the familiar device-orientation convention.
    public var roll: Double
    /// Whether the reference axis is too near vertical for its azimuth to mean anything.
    public var illConditioned: Bool

    /// Creates one reading.
    public init(
        heading: Double?,
        trueHeading: Double? = nil,
        frame: HeadingFrame = .magnetic,
        pitch: Double,
        roll: Double,
        illConditioned: Bool
    ) {
        self.heading = heading
        self.trueHeading = trueHeading
        self.frame = frame
        self.pitch = pitch
        self.roll = roll
        self.illConditioned = illConditioned
    }
}

/// The heading of a device in an arbitrary attitude, in pure arithmetic.
///
/// The rotation is derived here rather than from a device framework so that the whole
/// reference-axis decision is a unit test with no device attached. The convention is the
/// platform's: the rotation matrix maps a vector from the device's frame (X right along the
/// screen, Y up along the screen, Z out of the screen) into the world frame (X east, Y north,
/// Z up).
public enum HeadingReference {
    /// How near vertical a reference axis has to be before its azimuth is noise.
    public static let degenerateBandDegrees = 10.0

    /// The smallest up-component of a reference axis that is still well conditioned.
    ///
    /// It is `sin(90° - 10°)`: an axis within ten degrees of vertical points too straight up
    /// (or down) to say which way it leans.
    public static let minUpComponent = sin((90.0 - degenerateBandDegrees) * .pi / 180.0)

    /// A model of magnetic declination, in degrees, for a position and instant.
    ///
    /// It returns `nil` when it cannot answer. The device target injects a real one; the
    /// pure-logic seam exists so the arithmetic below is tested without depending on the
    /// device's magnetic model.
    public typealias Declination = (Double, Double, Double, Date) -> Double?

    /// The declination model used when the caller has none, which never answers.
    ///
    /// A true heading cannot be fabricated from a magnetic one without a declination, so the
    /// default withholds it rather than reporting a magnetic heading as though it were true.
    public static let noDeclination: Declination = { _, _, _, _ in nil }

    /// Computes the attitude for the chosen reference axis from a rotation vector.
    ///
    /// The vector is the quaternion `(x, y, z, w)` taking the device frame to the world frame.
    /// Any trailing element — an accuracy estimate a sensor may append — is ignored.
    public static func attitude(
        rotationVector: (x: Double, y: Double, z: Double, w: Double),
        axis: HeadingAxis,
        displayRotation: Int = 0
    ) -> HeadingReading {
        attitude(
            rotationVectorArray: [
                rotationVector.x, rotationVector.y, rotationVector.z, rotationVector.w,
            ],
            axis: axis,
            displayRotation: displayRotation
        )
    }

    /// Computes the attitude for the chosen reference axis from a rotation vector, as an
    /// array.
    ///
    /// The array form is what lets a five-element reading — a quaternion with a trailing
    /// accuracy estimate — be handled by ignoring the extra element.
    ///
    /// `displayRotation` is the surface rotation in quarter turns (0, 1, 2, 3 for 0, 90, 180
    /// and 270 degrees). It changes nothing for `screenBack` and everything for `screenTop`,
    /// because the "top" of a rotated display is a different piece of hardware.
    public static func attitude(
        rotationVectorArray rotationVector: [Double],
        axis: HeadingAxis,
        displayRotation: Int = 0
    ) -> HeadingReading {
        let rotation = rotationMatrix(rotationVector)
        let reference: [Double]
        switch axis {
        case .screenBack:
            reference = [0.0, 0.0, -1.0]
        case .screenTop:
            reference = displayUpAxis(displayRotation)
        }
        let world = toWorld(rotation: rotation, deviceVector: reference)
        let up = world[2]
        let illConditioned = abs(up) >= minUpComponent
        let pitch = degrees(asin(clamp(-rotation[7], -1.0, 1.0)))
        let roll = degrees(atan2(-rotation[6], rotation[8]))
        let heading = illConditioned
            ? nil
            : normalizeDegrees(degrees(atan2(world[0], world[1])))
        return HeadingReading(
            heading: heading,
            pitch: pitch,
            roll: roll,
            illConditioned: illConditioned
        )
    }

    /// The device-frame axis that appears "up" on the display, for a surface rotation of 0,
    /// 90, 180 or 270 degrees.
    ///
    /// This is the same mapping `remapCoordinateSystem` applies: for a 90-degree rotation the
    /// displayed up direction is the device's -X, and so on. The screen-back normal does not
    /// need it — the direction the back of the device points is a fact about the hardware —
    /// but the screen-top reference does.
    public static func displayUpAxis(_ displayRotation: Int) -> [Double] {
        switch ((displayRotation % 4) + 4) % 4 {
        case 0: return [0.0, 1.0, 0.0]
        case 1: return [-1.0, 0.0, 0.0]
        case 2: return [0.0, -1.0, 0.0]
        default: return [1.0, 0.0, 0.0]
        }
    }

    /// Builds the device-to-world rotation matrix from a rotation-vector reading.
    ///
    /// The vector is the rotation taking the device frame to the world frame, in `(x, y, z,
    /// w)` order; any trailing element — an accuracy estimate a sensor appends — is ignored.
    /// The reading is a unit quaternion, but a sensor is a sensor: it is normalized rather
    /// than trusted, because a matrix built from a non-unit quaternion is not a rotation and
    /// every angle derived from it is wrong.
    public static func rotationMatrix(_ input: [Double]) -> [Double] {
        precondition(input.count >= 4, "a rotation vector needs at least four elements")
        var x = input[0]
        var y = input[1]
        var z = input[2]
        var w = input[3]
        let norm = (x * x + y * y + z * z + w * w).squareRoot()
        if norm > 0.0 && abs(norm - 1.0) > 1e-9 {
            x /= norm
            y /= norm
            z /= norm
            w /= norm
        }
        return [
            1 - 2 * (y * y + z * z), 2 * (x * y - w * z), 2 * (x * z + w * y),
            2 * (x * y + w * z), 1 - 2 * (x * x + z * z), 2 * (y * z - w * x),
            2 * (x * z - w * y), 2 * (y * z + w * x), 1 - 2 * (x * x + y * y),
        ]
    }

    /// Rotates a device-frame vector into the world by a rotation matrix.
    public static func toWorld(rotation: [Double], deviceVector: [Double]) -> [Double] {
        [
            rotation[0] * deviceVector[0] + rotation[1] * deviceVector[1]
                + rotation[2] * deviceVector[2],
            rotation[3] * deviceVector[0] + rotation[4] * deviceVector[1]
                + rotation[5] * deviceVector[2],
            rotation[6] * deviceVector[0] + rotation[7] * deviceVector[1]
                + rotation[8] * deviceVector[2],
        ]
    }

    /// The true heading for a magnetic one at a position, or `nil` when it cannot be known.
    ///
    /// The declination is a property of where the device is and when, so a true heading cannot
    /// be derived without a position: a non-finite latitude or longitude gives `nil`. It also
    /// gives `nil` when the magnetic heading is not finite, or when the injected declination
    /// model cannot answer. Otherwise the declination is added and the result is wrapped into
    /// `0..<360`.
    public static func trueHeading(
        magnetic: Double,
        lat: Double,
        lon: Double,
        altitude: Double,
        at date: Date,
        declination: Declination = HeadingReference.noDeclination
    ) -> Double? {
        guard magnetic.isFinite, lat.isFinite, lon.isFinite else { return nil }
        guard let offset = declination(lat, lon, altitude, date), offset.isFinite else {
            return nil
        }
        return normalizeDegrees(magnetic + offset)
    }

    /// Wraps a heading into `0..<360`, so a boundary crossing carries no sign.
    public static func normalizeDegrees(_ value: Double) -> Double {
        var wrapped = value.truncatingRemainder(dividingBy: 360.0)
        if wrapped < 0 {
            wrapped += 360.0
        }
        return wrapped
    }

    /// Converts radians to degrees.
    private static func degrees(_ radians: Double) -> Double {
        radians * 180.0 / .pi
    }

    /// Clamps a value to a closed range, which is what `asin` needs of a slightly noisy dot
    /// product.
    private static func clamp(_ value: Double, _ low: Double, _ high: Double) -> Double {
        min(max(value, low), high)
    }
}
