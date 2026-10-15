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

/// The wire type byte of one frame on the console socket.
///
/// The raw values are the protocol, not a convenience: the Go bridge writes this
/// byte first (`ios/bridge/frames.go`), so a case that moved would make the two
/// halves of the console speak different languages and the app would render a
/// blank screen while reporting no error at all.
public enum FrameType: UInt8 {
    /// Raw terminal bytes, in either direction.
    case data = 0x01

    /// A new window size: the columns and rows of cells, then the width and height
    /// in pixels.
    case resize = 0x02

    /// The client's exit status, sent only from the bridge to the app.
    case exit = 0x03
}

/// One frame on the console socket.
///
/// The frame is `type(1) | length(4, big-endian) | payload`, which is what the Go
/// bridge writes and reads. A `data` frame's payload is the raw terminal stream in
/// whichever direction it is traveling; the type exists so that a resize or an exit
/// is not mistaken for bytes to be written into the terminal.
public enum ConsoleFrame: Equatable, Sendable {
    /// Raw terminal bytes.
    case data([UInt8])

    /// A new window size: the columns and rows of cells, then the width and height
    /// in pixels.
    ///
    /// Zero pixels mean the window has no physical size to report, which is what a
    /// terminal that keeps no font metrics says. A window size is both because the
    /// two answer different questions — how much text fits, and how much room there
    /// is — and the client's own interface needs the second one: the console's font
    /// draws a cell about twice as tall as it is wide, so a console standing up
    /// holds more columns than rows and only its pixel size says so.
    case resize(cols: UInt16, rows: UInt16, xPixel: UInt16 = 0, yPixel: UInt16 = 0)

    /// The client's exit status, 0-255.
    case exit(code: UInt8)

    /// The wire type byte this frame is sent as.
    public var type: FrameType {
        switch self {
        case .data: return .data
        case .resize: return .resize
        case .exit: return .exit
        }
    }
}

/// A frame could not be read: the byte stream has lost its framing.
///
/// Every one of these means the connection is unusable. Resynchronising would mean
/// guessing where the next frame begins, and a guess that is wrong writes terminal
/// bytes into the client's input and a keystroke into its screen.
public enum ConsoleFrameError: Error, Equatable {
    /// The declared payload length is larger than ``ConsoleFrames/maxPayload``.
    ///
    /// The length is refused before anything is allocated, so a peer that claims
    /// 0xFFFFFFFF bytes cannot make this app reserve four gigabytes.
    case oversizeFrame(bytes: UInt32)

    /// The type byte names no frame this protocol defines.
    case unknownFrameType(UInt8)

    /// The bytes ended inside a frame: a header or a payload that was not all there.
    case truncatedFrame

    /// The payload length is impossible for the frame's type — a resize frame that
    /// is not four bytes, an exit frame that is not one, or bytes after a frame
    /// where a single frame was expected.
    case malformedFrame(String)
}

/// The console frame codec.
///
/// This is the Swift half of a format whose other halves are the Go bridge
/// (`ios/bridge/frames.go`) and the Android appliance (`ConsoleFrames.kt`). All
/// three are asserted against one table of golden bytes, because a disagreement
/// between them produces a console that renders nothing and reports no error.
public enum ConsoleFrames {

    /// The largest payload either half accepts.
    ///
    /// Frames are keystrokes and terminal output, so a megabyte is far beyond
    /// anything legitimate; the limit is what keeps a declared length from becoming
    /// a reservation.
    public static let maxPayload = 1 << 20

    /// The five bytes a frame's header occupies: the type and a 32-bit length.
    public static let headerSize = 5

    /// The wire form of `frame`: the type, the big-endian payload length, the payload.
    public static func encode(_ frame: ConsoleFrame) -> [UInt8] {
        let payload = payload(of: frame)
        var out = [UInt8]()
        out.reserveCapacity(headerSize + payload.count)
        out.append(frame.type.rawValue)
        appendUInt32(UInt32(payload.count), to: &out)
        out.append(contentsOf: payload)
        return out
    }

    /// The single frame `bytes` holds.
    ///
    /// Anything that is not exactly one well-formed frame — a short buffer, a length
    /// that does not match the bytes present, an unknown type, an impossible payload
    /// size for the type — throws. Trailing bytes are an error rather than the next
    /// frame: a caller that has two frames in hand has a reader to give them to, and
    /// silently ignoring the second would lose a resize or an exit.
    public static func decode(_ bytes: [UInt8]) throws -> ConsoleFrame {
        guard bytes.count >= headerSize else {
            throw ConsoleFrameError.truncatedFrame
        }
        let type = bytes[0]
        // The length is read as unsigned: as a signed value a peer's 0xFFFFFFFF
        // becomes -1 and slips past the size check entirely.
        let length = uint32(bytes, at: 1)
        // Both checks run before the payload is touched, so neither can allocate.
        if length > UInt32(maxPayload) {
            throw ConsoleFrameError.oversizeFrame(bytes: length)
        }
        guard isKnown(type) else {
            throw ConsoleFrameError.unknownFrameType(type)
        }
        let declared = headerSize + Int(length)
        guard bytes.count >= declared else {
            throw ConsoleFrameError.truncatedFrame
        }
        guard bytes.count == declared else {
            throw ConsoleFrameError.malformedFrame(
                "\(bytes.count - declared) bytes follow the frame")
        }
        return try assemble(type: type, payload: Array(bytes[headerSize...]))
    }

    /// Whether `type` names a frame this protocol defines.
    public static func isKnown(_ type: UInt8) -> Bool {
        FrameType(rawValue: type) != nil
    }

    /// The bytes `frame` carries, in wire order.
    static func payload(of frame: ConsoleFrame) -> [UInt8] {
        switch frame {
        case .data(let bytes):
            return bytes
        case .resize(let cols, let rows, let xPixel, let yPixel):
            return [
                UInt8(cols >> 8), UInt8(cols & 0xFF),
                UInt8(rows >> 8), UInt8(rows & 0xFF),
                UInt8(xPixel >> 8), UInt8(xPixel & 0xFF),
                UInt8(yPixel >> 8), UInt8(yPixel & 0xFF),
            ]
        case .exit(let code):
            return [code]
        }
    }

    /// Turns a validated type and payload into a frame.
    ///
    /// A resize frame is exactly eight bytes — two cells and two pixel dimensions —
    /// and an exit frame exactly one; any other size is a protocol error rather than
    /// something to pad or truncate, because a padded resize would name a window size
    /// the sender never asked for.
    static func assemble(type: UInt8, payload: [UInt8]) throws -> ConsoleFrame {
        switch FrameType(rawValue: type) {
        case .data:
            return .data(payload)
        case .resize:
            guard payload.count == 8 else {
                throw ConsoleFrameError.malformedFrame(
                    "a resize frame is \(payload.count) bytes, want 8")
            }
            return .resize(
                cols: uint16(payload, at: 0),
                rows: uint16(payload, at: 2),
                xPixel: uint16(payload, at: 4),
                yPixel: uint16(payload, at: 6))
        case .exit:
            guard payload.count == 1 else {
                throw ConsoleFrameError.malformedFrame(
                    "an exit frame is \(payload.count) bytes, want 1")
            }
            return .exit(code: payload[0])
        case nil:
            throw ConsoleFrameError.unknownFrameType(type)
        }
    }

    private static func appendUInt32(_ value: UInt32, to out: inout [UInt8]) {
        out.append(UInt8((value >> 24) & 0xFF))
        out.append(UInt8((value >> 16) & 0xFF))
        out.append(UInt8((value >> 8) & 0xFF))
        out.append(UInt8(value & 0xFF))
    }

    /// Reads a big-endian 16-bit field.
    static func uint16(_ bytes: [UInt8], at offset: Int) -> UInt16 {
        (UInt16(bytes[offset]) << 8) | UInt16(bytes[offset + 1])
    }

    /// Reads a big-endian 32-bit length as an unsigned value.
    ///
    /// It is a `UInt32` and not an `Int` so that a declared length with the high bit
    /// set is a large number rather than a negative one.
    static func uint32(_ bytes: [UInt8], at offset: Int) -> UInt32 {
        (UInt32(bytes[offset]) << 24) | (UInt32(bytes[offset + 1]) << 16)
            | (UInt32(bytes[offset + 2]) << 8) | UInt32(bytes[offset + 3])
    }
}

/// Reads frame after frame from bytes that arrive in pieces.
///
/// A socket hands over whatever has arrived, which is not a frame: one delivery can
/// hold three frames and half of a fourth, or one byte. The reader is the buffer
/// between them, and `next()` returns nil while the bytes in hand do not complete a
/// frame, so a caller can pull frames as they become whole. The Kotlin half reads a
/// blocking `InputStream` instead and is told by it that the stream has ended, which
/// is what a push buffer cannot know: ``finish()`` is how a caller here says so, and
/// it is where a frame left half-read becomes the error it is.
public struct ConsoleFrameReader {

    /// The bytes seen but not yet handed out as a frame.
    private var buffer: [UInt8]

    /// A reader with an empty buffer.
    public init() {
        buffer = []
    }

    /// How many bytes are held: a frame the reader cannot yet complete.
    ///
    /// A caller that reaches the end of the stream with bytes still here has been cut
    /// off inside a frame, which is what ``finish()`` refuses.
    public var pendingBytes: Int { buffer.count }

    /// Adds `bytes` to the buffer, to be handed out as frames complete.
    public mutating func append(_ bytes: [UInt8]) {
        buffer.append(contentsOf: bytes)
    }

    /// The next frame that is complete in the buffer, or nil if none is.
    ///
    /// A declared length over ``ConsoleFrames/maxPayload`` and an unknown type throw
    /// before the payload is buffered, so a peer cannot make this app hold more than
    /// the limit. A thrown error empties the reader: the framing is gone, and there
    /// is nothing left to resynchronise to.
    ///
    /// nil means the bytes in hand do not make a frame, which is the ordinary case
    /// while a socket is still delivering one — not that the stream has ended. A
    /// caller that is told the stream has ended says so with ``finish()``, which is
    /// where a frame that was left half-read becomes the error it is.
    public mutating func next() throws -> ConsoleFrame? {
        guard buffer.count >= ConsoleFrames.headerSize else { return nil }
        let type = buffer[0]
        let length = ConsoleFrames.uint32(buffer, at: 1)
        if length > UInt32(ConsoleFrames.maxPayload) {
            buffer.removeAll()
            throw ConsoleFrameError.oversizeFrame(bytes: length)
        }
        guard ConsoleFrames.isKnown(type) else {
            buffer.removeAll()
            throw ConsoleFrameError.unknownFrameType(type)
        }
        let declared = ConsoleFrames.headerSize + Int(length)
        guard buffer.count >= declared else { return nil }
        let payload = Array(buffer[ConsoleFrames.headerSize..<declared])
        buffer.removeFirst(declared)
        return try ConsoleFrames.assemble(type: type, payload: payload)
    }

    /// Reports that the byte stream has ended: throws if a frame was left half-read.
    ///
    /// This is the half of a reader's contract that a blocking stream gets for free. A
    /// session reads until the socket closes and then does not know whether it saw a
    /// clean shutdown or a lost frame; the two have to be told apart, because one is
    /// how every session finishes and the other means the bytes since the last frame
    /// cannot be trusted. Bytes still here after the call would be a reader that kept
    /// them, so the buffer is emptied either way.
    public mutating func finish() throws {
        let leftover = buffer.count
        buffer.removeAll()
        guard leftover == 0 else { throw ConsoleFrameError.truncatedFrame }
    }
}
