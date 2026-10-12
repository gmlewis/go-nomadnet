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
@testable import ApplianceKit

/// Quaternion helpers and attitude fixtures shared by the heading tests.
///
/// They are built from quaternions rather than from matrices so the fixtures stay independent
/// of the code under test: an attitude assembled with the same rotation matrix the appliance
/// derives would prove nothing about that derivation.
enum HeadingFixtures {
    /// Hamilton product of two `(x, y, z, w)` quaternions.
    static func multiply(_ a: [Double], _ b: [Double]) -> [Double] {
        [
            a[3] * b[0] + a[0] * b[3] + a[1] * b[2] - a[2] * b[1],
            a[3] * b[1] - a[0] * b[2] + a[1] * b[3] + a[2] * b[0],
            a[3] * b[2] + a[0] * b[1] - a[1] * b[0] + a[2] * b[3],
            a[3] * b[3] - a[0] * b[0] - a[1] * b[1] - a[2] * b[2],
        ]
    }

    /// A rotation of `bearingDeg` about the world's up axis, expressed as a compass bearing.
    ///
    /// A bearing increases clockwise and a rotation about the up axis increases
    /// counter-clockwise, so the sign is inverted here rather than everywhere else.
    static func yaw(_ bearingDeg: Double) -> [Double] {
        let half = (-bearingDeg * .pi / 180.0) / 2.0
        return [0.0, 0.0, sin(half), cos(half)]
    }

    /// The attitude of a device standing upright in portrait, screen facing the operator, with
    /// the back of the device pointing at `yawDeg` true.
    ///
    /// The upright base attitude is a rotation of +90 degrees about the world's east axis, and
    /// the world-frame yaw is applied on top of it.
    static func standingUpright(_ yawDeg: Double) -> [Double] {
        multiply(yaw(yawDeg), [0.70710678, 0.0, 0.0, 0.70710678])
    }

    /// The attitude of a device lying flat, screen up, with the top edge of the screen pointing
    /// at `yawDeg` true. The identity rotation already aligns the device frame with the world,
    /// so a yaw is the whole attitude.
    static func lyingFlat(_ yawDeg: Double) -> [Double] {
        yaw(yawDeg)
    }

    /// The rotation vector measured on the device standing on its own stand, with the
    /// accuracy estimate a sensor appends as the trailing element.
    static let measuredOnTheStand: [Double] = [
        0.33369225, -0.38858578, 0.52529025, 0.67950034, -1.0,
    ]

    /// Reports whether `got` is within `tolerance` of `want`, the short way round.
    static func close(_ got: Double, _ want: Double, tolerance: Double = 0.5) -> Bool {
        let difference = HeadingReference.normalizeDegrees(got - want)
        return difference <= tolerance || abs(difference - 360.0) <= tolerance
    }
}
