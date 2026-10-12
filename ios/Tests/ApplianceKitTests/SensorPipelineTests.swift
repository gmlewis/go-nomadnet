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

/// The sensor policy: what reaches the feed, how often, and what does not.
///
/// The pipeline holds no sensor and no socket, so this is settled with no device attached. The
/// two things it must get right are that a caller's device callback is never stalled and that a
/// bad or too-frequent reading never reaches a receiver that would hold it.
///
/// What the pipeline accepts is what the appliance publishes, so these tests assert the
/// reading itself: a value the pipeline hands over is the value the Go half is fed, and
/// there is no later step that could reshape it.
final class SensorPipelineTests: XCTestCase {
    /// Collects the readings a pipeline publishes.
    private final class Recorder {
        private(set) var readings: [SensorReading] = []
        func record(_ reading: SensorReading) { readings.append(reading) }

        /// The fixes the pipeline accepted, in order.
        var fixes: [AcceptedFix] {
            readings.compactMap { if case .fix(let fix) = $0 { return fix } else { return nil } }
        }

        /// The headings the pipeline accepted, in order.
        var headings: [AcceptedHeading] {
            readings.compactMap { if case .heading(let h) = $0 { return h } else { return nil } }
        }
    }

    /// A clock the test drives by hand, so the rate limit needs no real second.
    private final class Clock {
        private(set) var now: Date
        init(_ start: Date) { now = start }
        func advance(_ seconds: TimeInterval) { now += seconds }
    }

    private let start = Date(timeIntervalSince1970: 1_791_491_980.123)

    /// Builds a pipeline and its recorder and clock.
    private func makePipeline() -> (SensorPipeline, Recorder, Clock) {
        let recorder = Recorder()
        let clock = Clock(start)
        let pipeline = SensorPipeline(publish: { recorder.record($0) }, now: { clock.now })
        return (pipeline, recorder, clock)
    }

    func testAnAcceptedFixIsPublishedWithTheValuesItWasAcceptedFrom() {
        let (pipeline, recorder, _) = makePipeline()
        pipeline.location(
            provider: "gps",
            lat: 35.0,
            lng: -106.0,
            altitude: 1_610.5,
            accuracyMeters: 4.5,
            speedKnots: 3.25,
            courseDeg: 271.0
        )

        XCTAssertEqual(
            [
                AcceptedFix(
                    time: start,
                    provider: "gps",
                    latitude: 35.0,
                    longitude: -106.0,
                    altitude: 1_610.5,
                    accuracyMeters: 4.5,
                    speedKnots: 3.25,
                    courseDeg: 271.0
                ),
            ],
            recorder.fixes
        )
    }

    /// A measurement nobody took is absent, not zero.
    func testAnUnmeasuredFieldIsAbsentRatherThanZero() {
        let (pipeline, recorder, _) = makePipeline()
        pipeline.location(provider: "gps", lat: 35.0, lng: -106.0, speedKnots: 0)

        let fix = try? XCTUnwrap(recorder.fixes.first)
        XCTAssertNil(fix?.altitude, "a height nobody measured is not sea level")
        XCTAssertNil(fix?.accuracyMeters)
        XCTAssertNil(fix?.courseDeg)
        XCTAssertEqual(fix?.speedKnots, 0, "a measured speed of zero is standing still")
    }

    func testAnAcceptedHeadingIsNormalizedIntoOneTurn() {
        let (pipeline, recorder, _) = makePipeline()
        pipeline.heading(-10.0)
        pipeline.heading(370.0)

        XCTAssertEqual([350.0, 10.0], recorder.headings.map(\.headingDeg))
        XCTAssertEqual([.magnetic, .magnetic], recorder.headings.map(\.frame))
    }

    /// The frame travels with the heading, because a reader holds one heading and a
    /// magnetic one reported as true is wrong by up to twenty degrees.
    func testTheFrameTravelsWithTheHeading() {
        let (pipeline, recorder, _) = makePipeline()
        pipeline.heading(47.5, frame: .`true`)

        XCTAssertEqual([.`true`], recorder.headings.map(\.frame))
    }

    func testASecondPositionInTheSameSecondDoesNotReachThePublisher() {
        let (pipeline, recorder, clock) = makePipeline()
        pipeline.location(provider: "gps", lat: 35.0, lng: -106.0)
        XCTAssertEqual(1, recorder.fixes.count)

        pipeline.location(provider: "gps", lat: 35.0001, lng: -106.0)
        XCTAssertEqual(1, recorder.fixes.count, "a position inside the second must be dropped")

        clock.advance(0.5)
        pipeline.location(provider: "gps", lat: 35.0002, lng: -106.0)
        XCTAssertEqual(1, recorder.fixes.count)

        clock.advance(0.5)
        pipeline.location(provider: "gps", lat: 35.0003, lng: -106.0)
        XCTAssertEqual(2, recorder.fixes.count, "a position a whole second later must be published")
    }

    func testAnOutOfRangeOrNonFinitePositionDoesNotReachThePublisher() {
        let (pipeline, recorder, _) = makePipeline()
        pipeline.location(provider: "gps", lat: 91.0, lng: -106.0)
        pipeline.location(provider: "gps", lat: 35.0, lng: 181.0)
        pipeline.location(provider: "gps", lat: .nan, lng: -106.0)
        pipeline.location(provider: "gps", lat: 35.0, lng: -106.0, valid: false)
        XCTAssertTrue(recorder.readings.isEmpty, "\(recorder.readings)")

        pipeline.location(provider: "gps", lat: 35.0, lng: -106.0)
        XCTAssertEqual(1, recorder.fixes.count)
    }

    func testAWithheldHeadingIsStillPublishedButANonFiniteOneIsNot() {
        let (pipeline, recorder, _) = makePipeline()
        pipeline.heading(nil)
        XCTAssertEqual(1, recorder.headings.count)
        XCTAssertNil(recorder.headings[0].headingDeg, "a withheld heading carries no angle")

        pipeline.heading(.nan)
        XCTAssertEqual(1, recorder.headings.count, "a non-finite heading is corruption, not a reading")
    }

    func testStopRefusesFurtherSamples() {
        let (pipeline, recorder, _) = makePipeline()
        pipeline.location(provider: "gps", lat: 35.0, lng: -106.0)
        XCTAssertEqual(1, recorder.readings.count)

        pipeline.stop()
        pipeline.location(provider: "gps", lat: 36.0, lng: -106.0)
        pipeline.heading(90.0)
        XCTAssertEqual(1, recorder.readings.count, "a stopped pipeline must publish nothing")

        // Stopping twice is harmless, and the pipeline stays stopped.
        pipeline.stop()
        XCTAssertEqual(1, recorder.readings.count)
    }
}
