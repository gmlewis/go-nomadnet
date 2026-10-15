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

@testable import TerminalKit

/// The console frame codec's contract with the Go bridge.
///
/// `ios/bridge/frames_test.go` and the Android appliance's `ConsoleFramesTest` carry
/// this same table. If two of the halves disagree, the app attaches to a console that
/// renders nothing and logs nothing, so the table is asserted byte for byte in every
/// language.
final class ConsoleFramesTests: XCTestCase {

    private struct Golden {
        let name: String
        let hex: String
        let frame: ConsoleFrame
    }

    private let golden: [Golden] = [
        Golden(name: "data hello", hex: "010000000568656c6c6f", frame: .data(Array("hello".utf8))),
        Golden(name: "data empty", hex: "0100000000", frame: .data([])),
        Golden(name: "resize 80x24", hex: "02000000080050001800000000",
               frame: .resize(cols: 80, rows: 24)),
        Golden(name: "resize 200x60", hex: "020000000800c8003c00000000",
               frame: .resize(cols: 200, rows: 60)),
        Golden(name: "resize 100x75 at 1200x1920", hex: "02000000080064004b04b00780",
               frame: .resize(cols: 100, rows: 75, xPixel: 1200, yPixel: 1920)),
        Golden(name: "resize 160x45 at 1920x1080", hex: "020000000800a0002d07800438",
               frame: .resize(cols: 160, rows: 45, xPixel: 1920, yPixel: 1080)),
        Golden(name: "exit 0", hex: "030000000100", frame: .exit(code: 0)),
        Golden(name: "exit 130", hex: "030000000182", frame: .exit(code: 130)),
    ]

    func testGoldenTableRoundTrips() throws {
        for entry in golden {
            let wire = hex(entry.hex)
            XCTAssertEqual(
                ConsoleFrames.encode(entry.frame), wire,
                "\(entry.name): the encoder disagrees with the table")
            XCTAssertEqual(
                try ConsoleFrames.decode(wire), entry.frame,
                "\(entry.name): the decoder disagrees with the table")
        }
    }

    func testHeaderIsFiveBytesAndTheTypeIsFirst() {
        XCTAssertEqual(ConsoleFrames.headerSize, 5)
        XCTAssertEqual(ConsoleFrames.maxPayload, 1 << 20)
        XCTAssertEqual(ConsoleFrames.encode(.data([]))[0], 0x01)
        XCTAssertEqual(ConsoleFrames.encode(.resize(cols: 1, rows: 1))[0], 0x02)
        XCTAssertEqual(ConsoleFrames.encode(.exit(code: 0))[0], 0x03)
        XCTAssertEqual(ConsoleFrame.resize(cols: 3, rows: 4).type, .resize)
    }

    func testALengthOverOneMebibyteIsRefusedRatherThanAllocated() {
        // A peer that claims 0xFFFFFFFF bytes must not be able to make the app reserve
        // four gigabytes, so the length is refused before any payload is built. The
        // refusals name the length so the failure can be read.
        let cases: [(String, ConsoleFrameError)] = [
            ("0100100001", .oversizeFrame(bytes: 0x100001)),
            ("01ffffffff", .oversizeFrame(bytes: 0xFFFF_FFFF)),
            ("0200100001", .oversizeFrame(bytes: 0x100001)),
        ]
        for (wire, want) in cases {
            XCTAssertThrowsError(try ConsoleFrames.decode(hex(wire))) { error in
                XCTAssertEqual(error as? ConsoleFrameError, want)
            }
        }
    }

    func testAnUnknownFrameTypeIsRefused() {
        for byte in [0x00, 0x04, 0xFF] {
            let wire = [UInt8(byte), 0, 0, 0, 0]
            XCTAssertThrowsError(try ConsoleFrames.decode(wire)) { error in
                XCTAssertEqual(error as? ConsoleFrameError, .unknownFrameType(UInt8(byte)))
            }
        }
    }

    func testATruncatedFrameIsAnErrorNotAShortRead() {
        let cases = [
            "01000000056865",
            "0100000005",
            "010000",
            "01",
            "",
            "02000000040050",
            "0300000001",
        ]
        for wire in cases {
            XCTAssertThrowsError(try ConsoleFrames.decode(hex(wire)), wire) { error in
                XCTAssertEqual(error as? ConsoleFrameError, .truncatedFrame)
            }
        }
    }

    func testTrailingBytesAfterOneFrameAreRefused() {
        // decode holds one frame and only one: a second frame belongs to the reader,
        // and ignoring it would lose a resize or an exit.
        let cases = [
            "010000000568656c6c6f" + "00",
            "030000000100" + "030000000182",
        ]
        for wire in cases {
            XCTAssertThrowsError(try ConsoleFrames.decode(hex(wire)), wire) { error in
                guard case .malformedFrame = error as? ConsoleFrameError else {
                    return XCTFail("want a malformed frame, got \(error)")
                }
            }
        }
    }

    func testAPayloadOfTheWrongSizeForItsTypeIsRefused() {
        // A resize frame is eight bytes — two cells and two pixel dimensions — and an
        // exit frame is one. Any other length is a protocol error rather than something to
        // pad or truncate.
        for wire in ["02000000020050", "020000000400500018", "02000000050050001800",
                     "02000000070050001804b007", "02000000090050001804b0072000",
                     "03000000020000"] {
            XCTAssertThrowsError(try ConsoleFrames.decode(hex(wire)), wire) { error in
                guard case .malformedFrame = error as? ConsoleFrameError else {
                    return XCTFail("want a malformed frame, got \(error)")
                }
            }
        }
    }

    func testTheReaderConsumesExactlyOneFrameAndLeavesTheNext() throws {
        // The session reads frame after frame from one socket, so the reader must stop
        // at a frame boundary rather than swallowing what follows.
        var reader = ConsoleFrameReader()
        reader.append(hex("010000000568656c6c6f"))
        reader.append(hex("02000000080050001800000000"))
        reader.append(hex("02000000080064004b04b00780"))
        reader.append(hex("0100000005") + Array("world".utf8))
        reader.append(hex("030000000182"))

        XCTAssertEqual(try reader.next(), .data(Array("hello".utf8)))
        XCTAssertEqual(try reader.next(), .resize(cols: 80, rows: 24))
        XCTAssertEqual(try reader.next(), .resize(cols: 100, rows: 75, xPixel: 1200, yPixel: 1920))
        XCTAssertEqual(try reader.next(), .data(Array("world".utf8)))
        XCTAssertEqual(try reader.next(), .exit(code: 130))
        XCTAssertNil(try reader.next(), "a clean end at a frame boundary is not an error")
        XCTAssertEqual(reader.pendingBytes, 0)
    }

    func testTheReaderWaitsForAFrameToComplete() throws {
        // A socket hands over whatever has arrived, which is half a frame as often as
        // it is three: the reader must report nothing until a frame is whole, and then
        // hand it over exactly once.
        let wire = ConsoleFrames.encode(.resize(cols: 80, rows: 24))
        var reader = ConsoleFrameReader()
        for byte in wire.dropLast() {
            reader.append([byte])
            XCTAssertNil(try reader.next(), "a partial frame was handed out as a frame")
        }
        reader.append([wire.last!])
        XCTAssertEqual(try reader.next(), .resize(cols: 80, rows: 24))
        XCTAssertNil(try reader.next())
    }

    func testTheReaderRefusesAnOversizeFrameBeforeBufferingItsPayload() throws {
        var reader = ConsoleFrameReader()
        reader.append(hex("01ffffffff"))
        XCTAssertThrowsError(try reader.next()) { error in
            XCTAssertEqual(error as? ConsoleFrameError, .oversizeFrame(bytes: 0xFFFF_FFFF))
        }
        XCTAssertEqual(reader.pendingBytes, 0, "the refused header must not be kept")
    }

    func testTheReaderRefusesAnUnknownTypeBeforeBufferingItsPayload() throws {
        var reader = ConsoleFrameReader()
        reader.append([0x7F, 0, 0, 0, 0x10])
        XCTAssertThrowsError(try reader.next()) { error in
            XCTAssertEqual(error as? ConsoleFrameError, .unknownFrameType(0x7F))
        }
        XCTAssertEqual(reader.pendingBytes, 0)
    }

    func testTheReaderRefusesAMalformedPayload() {
        var reader = ConsoleFrameReader()
        reader.append(hex("020000000300500018"))
        XCTAssertThrowsError(try reader.next()) { error in
            guard case .malformedFrame = error as? ConsoleFrameError else {
                return XCTFail("want a malformed frame, got \(error)")
            }
        }
    }

    func testTheReaderReportsAFrameCutOffByTheEndOfTheStream() throws {
        // The Kotlin half reads a blocking stream and so sees an end of stream for free.
        // A push buffer does not, which is why the end is reported to it: a session that
        // acted on a half-read frame would be acting on bytes that mean nothing.
        var cut = ConsoleFrameReader()
        cut.append(hex("01000000056865"))
        XCTAssertNil(try cut.next())
        XCTAssertEqual(cut.pendingBytes, 7, "the header and half its payload are here")
        XCTAssertThrowsError(try cut.finish()) { error in
            XCTAssertEqual(error as? ConsoleFrameError, .truncatedFrame)
        }
        XCTAssertEqual(cut.pendingBytes, 0, "the half-read frame is not kept")
        XCTAssertNoThrow(try cut.finish())

        var clean = ConsoleFrameReader()
        clean.append(hex("030000000182"))
        XCTAssertEqual(try clean.next(), .exit(code: 130))
        XCTAssertNoThrow(try clean.finish(), "a clean end at a frame boundary is not an error")
    }

    func testAnEmptyReaderIsACleanBoundary() throws {
        var reader = ConsoleFrameReader()
        XCTAssertNil(try reader.next())
    }

    /// hex is the test's own decoder, so the table above can be read as it is written.
    private func hex(_ text: String) -> [UInt8] {
        precondition(text.count % 2 == 0, "a hex string has two characters per byte: \(text)")
        var out = [UInt8]()
        out.reserveCapacity(text.count / 2)
        var index = text.startIndex
        while index < text.endIndex {
            let next = text.index(index, offsetBy: 2)
            out.append(UInt8(text[index..<next], radix: 16)!)
            index = next
        }
        return out
    }
}
