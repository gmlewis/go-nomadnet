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

/// What the radio bridge does with bytes.
///
/// The one thing it must not do is change them. The RNode protocol — KISS framing, the
/// handshake, the configuration commands — is spoken by the transport on one side and
/// answered by the firmware on the other, so the bridge is a witness and not a
/// participant, and a bridge that framed, split or merged anything would corrupt a
/// conversation between two parties that are not it.
///
/// The frames below are real KISS frames, escapes and all, because the point of the test
/// is that the bridge does not care what is inside them.
final class RadioPumpTests: XCTestCase {
    /// A radio that records what it was written.
    private final class FakeRadio: RadioPump.Radio {
        var maxWrite: Int
        private(set) var writes: [[UInt8]] = []

        init(maxWrite: Int) { self.maxWrite = maxWrite }

        func write(_ bytes: [UInt8]) { writes.append(bytes) }

        /// Everything the radio was written, concatenated, which is the byte stream the
        /// firmware sees.
        var stream: [UInt8] { writes.flatMap { $0 } }
    }

    /// Collects what the pump hands back to the transport.
    private final class FakeTransport {
        private(set) var stream: [UInt8] = []
        private(set) var deliveries: [[UInt8]] = []

        func receive(_ bytes: [UInt8]) {
            deliveries.append(bytes)
            stream.append(contentsOf: bytes)
        }
    }

    // MARK: - KISS frames, as the transport writes them

    private static let fend: UInt8 = 0xC0
    private static let fesc: UInt8 = 0xDB
    private static let tfend: UInt8 = 0xDC
    private static let tfesc: UInt8 = 0xDD

    /// A KISS frame with its payload's reserved bytes escaped, the way an RNode frame is
    /// spelled on the wire.
    private static func kissFrame(command: UInt8, payload: [UInt8]) -> [UInt8] {
        var out: [UInt8] = [fend, command]
        for byte in payload {
            switch byte {
            case fend: out += [fesc, tfend]
            case fesc: out += [fesc, tfesc]
            default: out.append(byte)
            }
        }
        out.append(fend)
        return out
    }

    /// Builds a pump and its two fakes.
    private func makePump(maxWrite: Int) -> (RadioPump, FakeRadio, FakeTransport) {
        let radio = FakeRadio(maxWrite: maxWrite)
        let transport = FakeTransport()
        let pump = RadioPump(radio: radio, sink: { transport.receive($0) })
        return (pump, radio, transport)
    }

    // MARK: - Outbound: transport to radio

    /// A frame longer than one characteristic write arrives at the radio whole, in as many
    /// writes as the characteristic takes.
    func testAFrameLargerThanOneWriteReachesTheRadioIntact() {
        let (pump, radio, _) = makePump(maxWrite: 20)
        let payload = Array("the quick brown fox jumps over the lazy dog".utf8)
        let frame = Self.kissFrame(command: 0x00, payload: payload)
        XCTAssertGreaterThan(frame.count, 20, "the fixture must not fit in one write")

        pump.receivedFromTransport(frame)
        pump.flush()

        XCTAssertGreaterThan(radio.writes.count, 1, "the frame should have been split")
        XCTAssertEqual(frame, radio.stream, "the radio must receive the frame the transport wrote")
        for write in radio.writes {
            XCTAssertLessThanOrEqual(write.count, 20, "a write over the characteristic's size would be refused")
        }
    }

    /// Payload bytes that had to be escaped survive the bridge: it does not unescape what it
    /// is carrying, and a bridge that did would hand the firmware a frame it would reject.
    func testEscapedPayloadBytesSurviveTheBridge() {
        let (pump, radio, _) = makePump(maxWrite: 508)
        // A payload of exactly the bytes KISS has to escape.
        let frame = Self.kissFrame(command: 0x00, payload: [Self.fend, Self.fesc, 0x01, 0x02])

        pump.receivedFromTransport(frame)
        pump.flush()

        XCTAssertEqual(frame, radio.stream)
        XCTAssertTrue(radio.stream.contains(Self.fesc), "the escapes must still be there")
    }

    /// Several frames offered in one TCP read are all delivered, in order.
    func testSeveralFramesInOneReadAllReachTheRadio() {
        let (pump, radio, _) = makePump(maxWrite: 508)
        let first = Self.kissFrame(command: 0x00, payload: [1, 2, 3])
        let second = Self.kissFrame(command: 0x01, payload: [4, 5, 6])

        pump.receivedFromTransport(first + second)
        pump.flush()

        XCTAssertEqual(first + second, radio.stream)
    }

    /// Nothing queued is nothing written.
    func testAnEmptyQueueWritesNothing() {
        let (pump, radio, _) = makePump(maxWrite: 20)
        pump.flush()
        XCTAssertTrue(radio.writes.isEmpty)
        XCTAssertEqual(0, pump.pendingBytes)
    }

    /// A write size the radio did not report is bounded rather than unbounded.
    func testAWriteSizeOfZeroIsBoundedRatherThanUnbounded() {
        let (pump, radio, _) = makePump(maxWrite: 0)
        let frame = Self.kissFrame(
            command: 0x00,
            payload: Array(repeating: 0x41, count: RadioPump.defaultMaxWrite * 2)
        )

        pump.receivedFromTransport(frame)
        pump.flush()

        XCTAssertEqual(frame, radio.stream)
        for write in radio.writes {
            XCTAssertLessThanOrEqual(write.count, RadioPump.defaultMaxWrite)
        }
    }

    // MARK: - Inbound: radio to transport

    /// A notification goes to the transport as it arrived, in one delivery.
    func testANotificationReachesTheTransportInOnePiece() {
        let (pump, _, transport) = makePump(maxWrite: 20)
        let frame = Self.kissFrame(command: 0x00, payload: [9, 9, 9])

        pump.receivedFromRadio(frame)

        XCTAssertEqual([frame], transport.deliveries)
        XCTAssertEqual(frame, transport.stream)
    }

    /// A frame the radio notified in two pieces is one frame by the time the transport has
    /// read it: the notification boundary is the radio's business, not the protocol's.
    func testAFrameNotifiedInTwoPiecesIsAWholeFrameAtTheTransport() {
        let (pump, _, transport) = makePump(maxWrite: 20)
        let frame = Self.kissFrame(command: 0x00, payload: Array("hello, radio".utf8))
        let split = frame.count / 2

        pump.receivedFromRadio(Array(frame[..<split]))
        pump.receivedFromRadio(Array(frame[split...]))

        XCTAssertEqual(frame, transport.stream, "the transport must read the frame the radio sent")
    }

    /// An empty notification is not delivered at all, so a transport is never woken for
    /// nothing.
    func testAnEmptyNotificationIsNotDelivered() {
        let (pump, _, transport) = makePump(maxWrite: 20)
        pump.receivedFromRadio([])
        XCTAssertTrue(transport.deliveries.isEmpty)
    }

    // MARK: - Both directions, and stopping

    /// The two directions are independent: traffic in one does not disturb the other.
    func testTheTwoDirectionsDoNotInterfere() {
        let (pump, radio, transport) = makePump(maxWrite: 20)
        let outbound = Self.kissFrame(command: 0x00, payload: [1, 2, 3, 4, 5])
        let inbound = Self.kissFrame(command: 0x00, payload: [6, 7, 8, 9, 10])

        pump.receivedFromRadio(inbound)
        pump.receivedFromTransport(outbound)
        pump.flush()

        XCTAssertEqual(outbound, radio.stream)
        XCTAssertEqual(inbound, transport.stream)
    }

    /// An unpaired radio's bytes are not the next radio's bytes.
    func testAStoppedPumpRefusesFurtherTraffic() {
        let (pump, radio, transport) = makePump(maxWrite: 20)
        pump.receivedFromTransport(Self.kissFrame(command: 0x00, payload: [1]))
        pump.stop()

        pump.receivedFromTransport(Self.kissFrame(command: 0x00, payload: [2]))
        pump.flush()

        XCTAssertTrue(radio.writes.isEmpty, "a stopped pump must write nothing to the radio")
        XCTAssertEqual(0, pump.pendingBytes, "a stopped pump must hold nothing for the next radio")

        // The inbound direction is the transport's own connection, which is closed by the
        // bridge rather than by the pump; what the pump must not do is queue.
        pump.receivedFromRadio(Self.kissFrame(command: 0x00, payload: [3]))
        XCTAssertEqual(1, transport.deliveries.count)
    }

    // MARK: - The address and the scan

    /// The address the transport dials is one whose port is not one of the appliance's own.
    func testTheRadioAddressIsItsOwnPort() {
        XCTAssertEqual("tcp://127.0.0.1:37431", RadioAddress.loopback)
        XCTAssertNotEqual(RadioAddress.port, NodeConfigRenderer.sharedInstancePort)
        XCTAssertNotEqual(RadioAddress.port, NodeConfigRenderer.feedPort)
        XCTAssertEqual(RadioAddress.interface().port, RadioAddress.loopback)
    }

    /// The scan attaches to a radio and to nothing else.
    ///
    /// The name rule is the reference's own, space included: the firmware names a radio
    /// `RNode <hex>`, and a bare `RNode` is not a name it gives one.
    func testTheScanRecognizesARadioAndRefusesAStranger() {
        XCTAssertTrue(RadioService.isRadioName("RNode 1A2B"))
        XCTAssertTrue(RadioService.isRadioName("RNode ABCDEF"))
        XCTAssertFalse(RadioService.isRadioName("RNode"), "the firmware's names carry a suffix")
        XCTAssertFalse(RadioService.isRadioName("Speaker"))
        XCTAssertFalse(RadioService.isRadioName(nil))
        XCTAssertFalse(RadioService.isRadioName(""))
    }

    /// The identifiers are the radio's published ones, which is what makes an appliance and
    /// a desktop able to reach the same radio.
    func testTheServiceAndCharacteristicsAreTheRadiosOwn() {
        XCTAssertEqual("6E400001-B5A3-F393-E0A9-E50E24DCCA9E", RadioService.service)
        XCTAssertEqual("6E400002-B5A3-F393-E0A9-E50E24DCCA9E", RadioService.toRadio)
        XCTAssertEqual("6E400003-B5A3-F393-E0A9-E50E24DCCA9E", RadioService.fromRadio)
    }
}
