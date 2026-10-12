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

import Darwin
import XCTest

@testable import TerminalKit

/// The console session, against a real socket on the machine running the tests.
///
/// A session is the app's half of the console: it dials the bridge, hands the bytes that come
/// back to the emulator, and writes the screen's keystrokes and window size out as frames. The
/// bridge itself is not started here — these tests bind a socket of their own and answer the
/// dial — but everything either side of that is the real thing: a real Unix socket, a real
/// blocking read, and the real frame codec.
///
/// The socket lives under `/tmp` with a short name on purpose. `sun_path` is 104 bytes, so a
/// path from the temporary directory a process is given does not fit and cannot be dialled,
/// which is the one thing about this seam that is easy to get wrong and impossible to see.
final class ConsoleSessionTests: XCTestCase {

    // MARK: - The bridge's end of the connection

    /// One frame the session wrote, read back off the socket byte for byte.
    private struct WireFrame: Equatable {
        let type: UInt8
        let payload: [UInt8]

        /// The payload read as text, for a failure message that says what was sent.
        var text: String { String(decoding: payload, as: UTF8.self) }
    }

    /// A failure on the bridge's side of the socket, which a test reports rather than tolerates.
    private struct PeerError: Error, CustomStringConvertible {
        let description: String

        init(_ description: String) {
            self.description = description
        }
    }

    /// A listening Unix socket the session dials, standing in for the bridge.
    private final class ConsolePeer {
        let path: String
        private var listener: Int32 = -1
        private var connection: Int32 = -1

        init() throws {
            path = "/tmp/gc-\(getpid())-\(UInt32.random(in: 0..<UInt32.max)).sock"
            let fd = socket(AF_UNIX, SOCK_STREAM, 0)
            guard fd >= 0 else { throw PeerError("socket: \(systemError)") }
            listener = fd
            // A path left behind by an earlier run is not a peer, and bind refuses it.
            unlink(path)
            guard let bound = withUnixSocketAddress(path, { Darwin.bind(fd, $0, $1) }) else {
                throw PeerError("the socket path \(path) does not fit in sun_path")
            }
            guard bound == 0 else { throw PeerError("bind \(path): \(systemError)") }
            guard listen(fd, 1) == 0 else { throw PeerError("listen: \(systemError)") }
        }

        /// Takes the session's connection, waiting up to `timeout` for it.
        func accept(timeout: TimeInterval = 5) throws {
            guard connection < 0 else { return }
            var entry = pollfd(fd: listener, events: Int16(POLLIN), revents: 0)
            let ready = poll(&entry, 1, Int32(timeout * 1000))
            guard ready > 0 else { throw PeerError("no session dialled \(path) within \(timeout)s") }
            let fd = Darwin.accept(listener, nil, nil)
            guard fd >= 0 else { throw PeerError("accept: \(systemError)") }
            var one: Int32 = 1
            setsockopt(fd, SOL_SOCKET, SO_NOSIGPIPE, &one, socklen_t(MemoryLayout<Int32>.size))
            connection = fd
        }

        /// Writes bytes the session reads as the bridge's frames.
        func write(_ bytes: [UInt8]) throws {
            guard connection >= 0 else { throw PeerError("nothing is connected to \(path)") }
            var sent = 0
            while sent < bytes.count {
                let count = bytes.withUnsafeBytes { raw -> Int in
                    guard let base = raw.baseAddress else { return 0 }
                    return send(connection, base.advanced(by: sent), bytes.count - sent, 0)
                }
                if count > 0 {
                    sent += count
                    continue
                }
                if count < 0 && errno == EINTR { continue }
                throw PeerError("writing to the session: \(systemError)")
            }
        }

        /// Reads one frame the session wrote, waiting up to `timeout` for it.
        func readFrame(timeout: TimeInterval = 5) throws -> WireFrame {
            let header = try readBytes(5, timeout: timeout)
            let length = (UInt32(header[1]) << 24) | (UInt32(header[2]) << 16)
                | (UInt32(header[3]) << 8) | UInt32(header[4])
            let payload = length > 0 ? try readBytes(Int(length), timeout: timeout) : []
            return WireFrame(type: header[0], payload: payload)
        }

        /// Closes the session's end, which the session reads as the bridge going away.
        func closeConnection() {
            if connection >= 0 {
                close(connection)
                connection = -1
            }
        }

        /// Releases the socket and removes its path.
        func release() {
            closeConnection()
            if listener >= 0 {
                close(listener)
                listener = -1
            }
            unlink(path)
        }

        private func readBytes(_ count: Int, timeout: TimeInterval) throws -> [UInt8] {
            var out: [UInt8] = []
            while out.count < count {
                var entry = pollfd(fd: connection, events: Int16(POLLIN), revents: 0)
                let ready = poll(&entry, 1, Int32(timeout * 1000))
                if ready == 0 {
                    throw PeerError("timed out after \(out.count) of \(count) bytes")
                }
                if ready < 0 {
                    if errno == EINTR { continue }
                    throw PeerError("poll: \(systemError)")
                }
                var buffer = [UInt8](repeating: 0, count: count - out.count)
                let bytesRead = buffer.withUnsafeMutableBytes { raw -> Int in
                    guard let base = raw.baseAddress else { return 0 }
                    return Darwin.read(connection, base, raw.count)
                }
                if bytesRead > 0 {
                    out.append(contentsOf: buffer[0..<bytesRead])
                    continue
                }
                if bytesRead == 0 { throw PeerError("the session closed the connection") }
                if errno == EINTR { continue }
                throw PeerError("reading from the session: \(systemError)")
            }
            return out
        }
    }

    // MARK: - Frame bytes, written the way the bridge writes them

    private func dataFrame(_ bytes: [UInt8]) -> [UInt8] {
        [FrameType.data.rawValue] + length(bytes.count) + bytes
    }

    private func resizeFrame(cols: UInt16, rows: UInt16) -> [UInt8] {
        [FrameType.resize.rawValue] + length(4)
            + [UInt8(cols >> 8), UInt8(cols & 0xFF), UInt8(rows >> 8), UInt8(rows & 0xFF)]
    }

    private func exitFrame(_ code: UInt8) -> [UInt8] {
        [FrameType.exit.rawValue] + length(1) + [code]
    }

    private func length(_ count: Int) -> [UInt8] {
        [
            UInt8((count >> 24) & 0xFF),
            UInt8((count >> 16) & 0xFF),
            UInt8((count >> 8) & 0xFF),
            UInt8(count & 0xFF),
        ]
    }

    // MARK: - The tests

    func testTheSizeTheConsoleStartsWithIsSentWhenTheBridgeAnswers() throws {
        // The screen is measured, and knows its size, before the bridge has answered the dial —
        // and at that instant there is no socket to send it on. A size dropped there is dropped
        // for the whole session: the client keeps the size it was started with and draws in one
        // corner of a screen it never claims.
        let peer = try ConsolePeer()
        defer { peer.release() }
        let session = ConsoleSession(socketPath: peer.path, cols: 80, rows: 24)

        session.start()
        try peer.accept()

        let frame = try peer.readFrame()
        XCTAssertEqual(FrameType.resize.rawValue, frame.type, "the size did not go out as a resize frame")
        XCTAssertEqual([0x00, 0x50, 0x00, 0x18], frame.payload, "the size is columns then rows, big-endian")
        session.stop()
    }

    func testTheNewestSizeIsTheOneThatGoesOutOnConnect() throws {
        let peer = try ConsolePeer()
        defer { peer.release() }
        let session = ConsoleSession(socketPath: peer.path, cols: 80, rows: 24)

        // The view is measured a few times before the bridge answers, and only the newest size
        // is owed: a client is owed its current size, not a history of them.
        session.resize(cols: 100, rows: 30)
        session.resize(cols: 160, rows: 48)
        session.start()
        try peer.accept()

        let frame = try peer.readFrame()
        XCTAssertEqual(FrameType.resize.rawValue, frame.type)
        XCTAssertEqual([0x00, 0xA0, 0x00, 0x30], frame.payload,
                       "an old size was replayed, or the newest one was lost")
        XCTAssertThrowsError(try peer.readFrame(timeout: 0.2), "the owed size went out twice")

        // Once the bridge is there, a size goes straight out: the deferred one is a wait rather
        // than a change of behavior.
        session.resize(cols: 120, rows: 40)
        let next = try peer.readFrame()
        XCTAssertEqual([0x00, 0x78, 0x00, 0x28], next.payload)

        // A size a frame cannot carry is refused rather than truncated: a truncated size is
        // still a plausible terminal size, and the client would lay itself out for it.
        session.resize(cols: 0, rows: 40)
        XCTAssertThrowsError(try peer.readFrame(timeout: 0.2), "an impossible size was sent anyway")
        session.stop()
    }

    func testKeystrokesGoOutAsDataFrames() throws {
        let peer = try ConsolePeer()
        defer { peer.release() }
        let session = ConsoleSession(socketPath: peer.path, cols: 80, rows: 24)

        session.start()
        try peer.accept()
        _ = try peer.readFrame() // the size the session owes on connect

        session.send([0x61]) // "a"
        session.send([0x03]) // what ctrl-c sends
        session.send([]) // an empty write is still a write, and is not sent at all

        let first = try peer.readFrame()
        XCTAssertEqual(FrameType.data.rawValue, first.type, "a keystroke did not go out as a data frame")
        XCTAssertEqual("a", first.text)
        let second = try peer.readFrame()
        XCTAssertEqual([0x03], second.payload)
        XCTAssertThrowsError(try peer.readFrame(timeout: 0.2), "an empty write was sent as a frame")
        session.stop()
    }

    func testIncomingDataFramesReachTheOutputCallback() throws {
        // The client's output is terminal bytes, not text: a data frame is handed on exactly as
        // if it had come off a pty, so an escape sequence in it is acted on rather than printed.
        let peer = try ConsolePeer()
        defer { peer.release() }
        let session = ConsoleSession(socketPath: peer.path, cols: 80, rows: 24)

        var received: [UInt8] = []
        var titles: [String] = []
        let output = expectation(description: "the client's output")
        session.onOutput = { bytes in
            received.append(contentsOf: bytes)
            output.fulfill()
        }
        session.onTitle = { titles.append($0) }
        var ends: [ConsoleEnd] = []
        session.onEnd = { ends.append($0) }

        session.start()
        try peer.accept()
        _ = try peer.readFrame()
        try peer.write(dataFrame(Array("plain\u{1b}[31mred".utf8)))

        wait(for: [output], timeout: 5)
        XCTAssertEqual("plain\u{1b}[31mred", String(decoding: received, as: UTF8.self))
        XCTAssertEqual([], titles, "the connection's protocol carries no title")
        XCTAssertEqual([], ends, "a data frame ended the session")
        session.stop()
    }

    func testAnExitFrameEndsTheSessionWithTheCode() throws {
        let peer = try ConsolePeer()
        defer { peer.release() }
        let session = ConsoleSession(socketPath: peer.path, cols: 80, rows: 24)

        var received: [UInt8] = []
        session.onOutput = { received.append(contentsOf: $0) }
        var ends: [ConsoleEnd] = []
        let ended = expectation(description: "the session ended")
        session.onEnd = {
            ends.append($0)
            ended.fulfill()
        }

        session.start()
        try peer.accept()
        _ = try peer.readFrame()
        try peer.write(dataFrame(Array("bye".utf8)))
        try peer.write(exitFrame(3))

        wait(for: [ended], timeout: 5)
        XCTAssertEqual([.exited(3)], ends, "the exit code was not reported")
        XCTAssertEqual("bye", String(decoding: received, as: UTF8.self),
                       "output written before the exit was lost")

        // The session is over, and says so once: a stop after an exit is the caller tidying up
        // rather than a second end.
        session.stop()
        XCTAssertEqual([.exited(3)], ends, "the session ended twice")

        // Nothing goes out on a connection the bridge has closed.
        session.send([0x41])
        XCTAssertThrowsError(try peer.readFrame(timeout: 0.2), "a keystroke went out after the exit")
    }

    func testAClosedSocketEndsTheSessionOnce() throws {
        let peer = try ConsolePeer()
        defer { peer.release() }
        let session = ConsoleSession(socketPath: peer.path, cols: 80, rows: 24)

        var received: [UInt8] = []
        session.onOutput = { received.append(contentsOf: $0) }
        var ends: [ConsoleEnd] = []
        let ended = expectation(description: "the session ended")
        session.onEnd = {
            ends.append($0)
            ended.fulfill()
        }

        session.start()
        try peer.accept()
        _ = try peer.readFrame()
        try peer.write(dataFrame(Array("one".utf8)))
        try peer.write(dataFrame(Array("two".utf8)))
        peer.closeConnection()

        wait(for: [ended], timeout: 5)
        XCTAssertEqual([.closed], ends, "a closed socket was not reported as the end, or was reported twice")
        XCTAssertEqual("onetwo", String(decoding: received, as: UTF8.self),
                       "frames written before the close were lost")
        session.stop()
        XCTAssertEqual([.closed], ends, "the session ended twice")
    }

    func testAProtocolErrorEndsTheSessionRatherThanThrowing() throws {
        // Every one of these loses the framing, and none of them may reach the caller as an
        // exception: this code runs inside an app, and a terminal that throws into one takes the
        // appliance down with it.
        let cases: [(name: String, bytes: [UInt8])] = [
            ("an unknown frame type", [0x09] + length(0)),
            ("an oversize frame", [FrameType.data.rawValue] + length(2 << 20)),
            ("an exit frame that is two bytes", [FrameType.exit.rawValue] + length(2) + [1, 2]),
        ]

        for testCase in cases {
            let peer = try ConsolePeer()
            defer { peer.release() }
            let session = ConsoleSession(socketPath: peer.path, cols: 80, rows: 24)
            let ended = expectation(description: "the session ended: \(testCase.name)")
            var ends: [ConsoleEnd] = []
            session.onEnd = {
                ends.append($0)
                ended.fulfill()
            }

            session.start()
            try peer.accept()
            _ = try peer.readFrame()
            try peer.write(testCase.bytes)

            wait(for: [ended], timeout: 5)
            guard case .failed(let reason) = ends.first else {
                XCTFail("\(testCase.name): the session did not fail: \(ends)")
                continue
            }
            XCTAssertFalse(reason.isEmpty, "\(testCase.name): a failure with no reason")
            session.stop()
            XCTAssertEqual(1, ends.count, "\(testCase.name): the session ended twice")
        }
    }

    func testAStreamThatEndsInsideAFrameIsNotACleanEnd() throws {
        // A stream that ends exactly at a boundary is how every session finishes; one that ends
        // with bytes in hand was cut off inside a frame, and the bytes seen since the last frame
        // cannot be trusted.
        let peer = try ConsolePeer()
        defer { peer.release() }
        let session = ConsoleSession(socketPath: peer.path, cols: 80, rows: 24)
        let ended = expectation(description: "the session ended")
        var ends: [ConsoleEnd] = []
        session.onEnd = {
            ends.append($0)
            ended.fulfill()
        }

        session.start()
        try peer.accept()
        _ = try peer.readFrame()
        try peer.write([FrameType.data.rawValue] + length(10) + [1, 2, 3])
        peer.closeConnection()

        wait(for: [ended], timeout: 5)
        guard case .failed = ends.first else {
            return XCTFail("a frame cut in half was reported as a clean end: \(ends)")
        }
        session.stop()
    }

    func testAnIncomingResizeIsIgnoredAndTheSessionGoesOn() throws {
        // The window size is the screen's business and never the bridge's, so a resize frame
        // arriving from the bridge is ignored rather than acted on — and ignoring it is not the
        // end of the session.
        let peer = try ConsolePeer()
        defer { peer.release() }
        let session = ConsoleSession(socketPath: peer.path, cols: 80, rows: 24)

        let output = expectation(description: "the client's output")
        var received: [UInt8] = []
        session.onOutput = { bytes in
            received.append(contentsOf: bytes)
            output.fulfill()
        }
        var ends: [ConsoleEnd] = []
        session.onEnd = { ends.append($0) }

        session.start()
        try peer.accept()
        _ = try peer.readFrame()
        try peer.write(resizeFrame(cols: 120, rows: 40))
        try peer.write(dataFrame(Array("hi".utf8)))

        wait(for: [output], timeout: 5)
        XCTAssertEqual("hi", String(decoding: received, as: UTF8.self))
        XCTAssertEqual([], ends, "a resize from the bridge ended the session")
        session.stop()
    }

    func testStoppingIsSafeTwiceAndAfterTheSessionHasEnded() throws {
        let peer = try ConsolePeer()
        defer { peer.release() }
        let session = ConsoleSession(socketPath: peer.path, cols: 80, rows: 24)

        let ended = expectation(description: "the session ended")
        var ends: [ConsoleEnd] = []
        session.onEnd = {
            ends.append($0)
            ended.fulfill()
        }

        session.start()
        try peer.accept()
        _ = try peer.readFrame()

        session.stop()
        session.stop()

        wait(for: [ended], timeout: 5)
        XCTAssertEqual([.stopped], ends, "stopping twice reported twice, or reported something else")

        // A session that has ended sends nothing at all, and a keystroke or a resize after it is
        // the caller's mistake rather than something that reaches the bridge.
        session.send([0x41])
        session.resize(cols: 70, rows: 20)
        XCTAssertThrowsError(try peer.readFrame(timeout: 0.2), "a stopped session wrote a frame")
        XCTAssertEqual([.stopped], ends, "the session ended again after stopping")
    }

    func testStoppingBeforeTheBridgeAnswersEndsTheSessionOnce() throws {
        let peer = try ConsolePeer()
        defer { peer.release() }
        let session = ConsoleSession(socketPath: peer.path, cols: 80, rows: 24)

        let ended = expectation(description: "the session ended")
        var ends: [ConsoleEnd] = []
        session.onEnd = {
            ends.append($0)
            ended.fulfill()
        }

        session.start()
        session.stop()

        wait(for: [ended], timeout: 5)
        XCTAssertEqual([.stopped], ends)
    }

    func testABridgeThatIsNotThereFailsRatherThanThrowing() throws {
        let session = ConsoleSession(
            socketPath: "/tmp/gc-nobody-\(getpid())-\(UInt32.random(in: 0..<UInt32.max)).sock",
            cols: 80,
            rows: 24
        )
        let ended = expectation(description: "the session failed")
        var ends: [ConsoleEnd] = []
        session.onEnd = {
            ends.append($0)
            ended.fulfill()
        }

        session.start()

        wait(for: [ended], timeout: 5)
        guard case .failed(let reason) = ends.first else {
            return XCTFail("a dial to nothing was not a failure: \(ends)")
        }
        XCTAssertTrue(reason.contains("connecting to the console socket"),
                      "the failure does not say what went wrong: \(reason)")
        session.stop()
        XCTAssertEqual(1, ends.count)
    }

    func testASocketPathFromTheTemporaryDirectoryIsRefusedRatherThanTruncated() throws {
        // sun_path is 104 bytes and NSTemporaryDirectory() is far longer, so a path that does
        // not fit has to be refused: a truncated path is some other socket's name, and the
        // session would either fail with a puzzling error or — worse — dial something else.
        let session = ConsoleSession(
            socketPath: String(repeating: "/a-very-long-path", count: 12) + ".sock",
            cols: 80,
            rows: 24
        )
        let ended = expectation(description: "the session failed")
        var ends: [ConsoleEnd] = []
        session.onEnd = {
            ends.append($0)
            ended.fulfill()
        }

        session.start()

        wait(for: [ended], timeout: 5)
        guard case .failed(let reason) = ends.first else {
            return XCTFail("a path that does not fit was not refused: \(ends)")
        }
        XCTAssertTrue(reason.contains("too long"), "the failure does not say why: \(reason)")
    }
}

/// The message the last failed system call left behind, for a failure a test has to explain.
private var systemError: String {
    String(cString: strerror(errno))
}
