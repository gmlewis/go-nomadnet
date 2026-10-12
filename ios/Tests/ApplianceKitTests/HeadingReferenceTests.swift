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

import XCTest
@testable import ApplianceKit

/// The reference-axis decision, tested without a device.
///
/// A rotation matrix is only arithmetic, so the whole question of which axis a heading should
/// be measured along can be settled by constructing an attitude and asking.
final class HeadingReferenceTests: XCTestCase {
    func testScreenBackHeadingTracksTheYawOfADeviceStandingUpright() throws {
        for yaw in [0.0, 45.0, 90.0, 180.0, 270.0, 359.0] {
            let reading = HeadingReference.attitude(
                rotationVectorArray: HeadingFixtures.standingUpright(yaw),
                axis: .screenBack
            )
            let heading = try XCTUnwrap(reading.heading, "standing upright, yaw \(yaw)")
            XCTAssertTrue(
                HeadingFixtures.close(heading, yaw),
                "standing upright, yaw \(yaw): screen-back heading was \(heading)"
            )
            XCTAssertFalse(
                reading.illConditioned,
                "standing upright is the well-conditioned case for the screen-back normal"
            )
        }
    }

    func testScreenTopHeadingIsNotTheDirectionTheOperatorFacesStandingUpright() {
        // On a device standing up, the device's +Y axis is vertical, so its azimuth is the
        // degenerate case: whatever number comes out, it does not describe where anybody is
        // looking. The appliance reports that by withholding the heading rather than by
        // publishing a confident wrong one.
        let reading = HeadingReference.attitude(
            rotationVectorArray: HeadingFixtures.standingUpright(90.0),
            axis: .screenTop
        )
        XCTAssertTrue(reading.illConditioned)
        XCTAssertNil(reading.heading, "a degenerate reference axis must not produce a heading")
    }

    func testScreenTopHeadingIsWellConditionedAndCorrectWhenLyingFlat() throws {
        for yaw in [0.0, 90.0, 200.0] {
            let reading = HeadingReference.attitude(
                rotationVectorArray: HeadingFixtures.lyingFlat(yaw),
                axis: .screenTop
            )
            XCTAssertFalse(
                reading.illConditioned,
                "a flat device is the well-conditioned case for the screen-top reference"
            )
            let heading = try XCTUnwrap(reading.heading, "flat, yaw \(yaw)")
            XCTAssertTrue(
                HeadingFixtures.close(heading, yaw),
                "flat, yaw \(yaw): screen-top heading was \(heading)"
            )
        }
        let back = HeadingReference.attitude(
            rotationVectorArray: HeadingFixtures.lyingFlat(0.0),
            axis: .screenBack
        )
        XCTAssertTrue(
            back.illConditioned,
            "a flat device makes the screen-back normal the degenerate one"
        )
    }

    func testTheTwoReferencesAgreeOnABearingAndDisagreeOnWhetherItCanBeRead() throws {
        // The bearing is the same one measured two ways: the back of a device standing at 90
        // and the top of a device lying flat at 90 point the same way.
        let back = HeadingReference.attitude(
            rotationVectorArray: HeadingFixtures.standingUpright(90.0),
            axis: .screenBack
        )
        let top = HeadingReference.attitude(
            rotationVectorArray: HeadingFixtures.lyingFlat(90.0),
            axis: .screenTop
        )
        XCTAssertTrue(
            HeadingFixtures.close(try XCTUnwrap(back.heading), try XCTUnwrap(top.heading), tolerance: 1.0),
            "the two references must agree on the bearing"
        )

        // What they do not agree on is whether they can be read at all: on a standing device
        // only the screen-back normal is well conditioned, and on a flat one only the
        // screen-top axis is.
        XCTAssertFalse(back.illConditioned)
        XCTAssertFalse(top.illConditioned)
        XCTAssertTrue(
            HeadingReference.attitude(
                rotationVectorArray: HeadingFixtures.standingUpright(90.0),
                axis: .screenTop
            ).illConditioned
        )
        XCTAssertTrue(
            HeadingReference.attitude(
                rotationVectorArray: HeadingFixtures.lyingFlat(90.0),
                axis: .screenBack
            ).illConditioned
        )
    }

    func testDisplayRotationChangesTheScreenTopReferenceAndNotTheScreenBackOne() throws {
        // A standing device, where the screen-back normal is readable, so the comparison is of
        // two real headings rather than of two withheld ones.
        let standing = HeadingFixtures.standingUpright(30.0)
        let back0 = try XCTUnwrap(
            HeadingReference.attitude(
                rotationVectorArray: standing, axis: .screenBack, displayRotation: 0
            ).heading
        )
        let back1 = try XCTUnwrap(
            HeadingReference.attitude(
                rotationVectorArray: standing, axis: .screenBack, displayRotation: 1
            ).heading
        )
        XCTAssertEqual(
            back0, back1, accuracy: 1e-9,
            "the direction the back of the device points is a fact about the hardware and must "
                + "not depend on how the display is rotated"
        )

        let flat = HeadingFixtures.lyingFlat(0.0)
        let top0 = try XCTUnwrap(
            HeadingReference.attitude(
                rotationVectorArray: flat, axis: .screenTop, displayRotation: 0
            ).heading
        )
        let top1 = try XCTUnwrap(
            HeadingReference.attitude(
                rotationVectorArray: flat, axis: .screenTop, displayRotation: 1
            ).heading
        )
        // A 90-degree display rotation makes the device's -X axis the top of the display, which
        // is a quarter turn the other way from the device's +Y.
        XCTAssertEqual(
            270.0, HeadingReference.normalizeDegrees(top1 - top0), accuracy: 1.0,
            "the screen-top reference must follow the display: \(top0) versus \(top1)"
        )
    }

    func testTheRotationMatrixReproducesTheMeasuredGravityVector() {
        // The matrix the appliance derives itself has to agree with the one the platform
        // derives, or neither the heading nor the tilt means anything. On the stand, gravity in
        // the device frame was measured at (0.879, 0.044, 0.475), and the device's +X axis is
        // the column that reproduces it.
        let rotation = HeadingReference.rotationMatrix(HeadingFixtures.measuredOnTheStand)
        let xInWorld = HeadingReference.toWorld(rotation: rotation, deviceVector: [1.0, 0.0, 0.0])
        XCTAssertEqual(0.879, xInWorld[2], accuracy: 0.01, "device +X points almost straight up")
        XCTAssertEqual(0.146, xInWorld[0], accuracy: 0.01, "and slightly east")
    }

    func testAFiveElementRotationVectorIgnoresTheAccuracyEstimate() throws {
        let four = [0.33369225, -0.38858578, 0.52529025, 0.67950034]
        let five = HeadingFixtures.measuredOnTheStand
        let a = HeadingReference.attitude(rotationVectorArray: four, axis: .screenBack)
        let b = HeadingReference.attitude(rotationVectorArray: five, axis: .screenBack)
        XCTAssertEqual(try XCTUnwrap(a.heading), try XCTUnwrap(b.heading), accuracy: 1e-9)
    }

    func testTheFourElementAndLabelledFormsAgree() throws {
        let vector = HeadingFixtures.measuredOnTheStand
        let labeled = HeadingReference.attitude(
            rotationVector: (x: vector[0], y: vector[1], z: vector[2], w: vector[3]),
            axis: .screenBack
        )
        let array = HeadingReference.attitude(rotationVectorArray: vector, axis: .screenBack)
        XCTAssertEqual(labeled, array)
    }

    func testHeadingsWrapIntoTheFullTurn() {
        XCTAssertEqual(0.0, HeadingReference.normalizeDegrees(360.0), accuracy: 1e-9)
        XCTAssertEqual(0.0, HeadingReference.normalizeDegrees(720.0), accuracy: 1e-9)
        XCTAssertEqual(348.0, HeadingReference.normalizeDegrees(-12.0), accuracy: 1e-9)
        XCTAssertEqual(12.0, HeadingReference.normalizeDegrees(372.0), accuracy: 1e-9)
        XCTAssertEqual(359.9, HeadingReference.normalizeDegrees(359.9), accuracy: 1e-9)

        let justBelow = HeadingReference.normalizeDegrees(359.9)
        let justAbove = HeadingReference.normalizeDegrees(0.1)
        XCTAssertTrue(justBelow > 359.0 && justBelow < 360.0)
        XCTAssertTrue(justAbove > 0.0 && justAbove < 1.0)
        XCTAssertTrue(HeadingReference.normalizeDegrees(justBelow + 0.2) < 1.0)
    }

    func testTrueHeadingAddsTheDeclinationAndWraps() throws {
        let at = Date(timeIntervalSince1970: 0)
        let east: HeadingReference.Declination = { _, _, _, _ in 10.0 }
        XCTAssertEqual(
            110.0,
            try XCTUnwrap(
                HeadingReference.trueHeading(
                    magnetic: 100, lat: 35, lon: -106, altitude: 1600, at: at, declination: east
                )
            ),
            accuracy: 1e-9
        )
        XCTAssertEqual(
            5.0,
            try XCTUnwrap(
                HeadingReference.trueHeading(
                    magnetic: 355, lat: 35, lon: -106, altitude: 1600, at: at, declination: east
                )
            ),
            accuracy: 1e-9,
            "the declination-corrected heading must wrap like any other"
        )
    }

    func testTrueHeadingIsWithheldWithoutAPositionOrWithoutAModel() {
        let at = Date(timeIntervalSince1970: 0)
        let east: HeadingReference.Declination = { _, _, _, _ in 10.0 }
        XCTAssertNil(
            HeadingReference.trueHeading(
                magnetic: 100, lat: .nan, lon: -106, altitude: 1600, at: at, declination: east
            ),
            "the declination is a property of where the device is, so a true heading needs one"
        )
        XCTAssertNil(
            HeadingReference.trueHeading(
                magnetic: 100, lat: 35, lon: -106, altitude: 1600, at: at
            ),
            "with no magnetic model the appliance must not pass a magnetic heading off as true"
        )
        let silent: HeadingReference.Declination = { _, _, _, _ in nil }
        XCTAssertNil(
            HeadingReference.trueHeading(
                magnetic: 100, lat: 35, lon: -106, altitude: 1600, at: at, declination: silent
            )
        )
    }
}
