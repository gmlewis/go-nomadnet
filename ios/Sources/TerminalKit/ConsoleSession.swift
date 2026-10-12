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
import Foundation

/// Why a console session is over.
///
/// A session ends exactly once and reports once, whichever of these it was, and the caller may
/// treat the report as "the session is finished and must not be pumped again" — which is what
/// makes it safe for a stop from the screen and a read that fails on the reader's queue to
/// arrive as one end rather than two.
public enum ConsoleEnd: Equatable {

    /// The client exited with `code`, which the bridge sent in an exit frame.
    case exited(UInt8)

    /// The connection ended without an exit frame.
    ///
    /// The bridge closed the socket, or the session was ended before it connected. There is no
    /// exit code to report because the bridge is in no position to give one: it was killed, or
    /// it was never started.
    case closed

    /// The session could not go on. The reason says what was wrong, for the log.
    case failed(String)

    /// The caller stopped the session.
    case stopped
}

/// The appliance's half of the console: it owns the connection to the Go bridge and the
/// frames that cross it.
///
/// The bridge (`ios/bridge`) runs the client on a pseudo-terminal and listens on a Unix socket
/// at a path the app names; a session dials it, decodes the frames the bridge sends into the
/// emulator's byte stream, and encodes the tablet's keystrokes and window size back out. The
/// frame codec is ``ConsoleFrames``, which the bridge implements in Go and which both halves
/// assert against one table of bytes.
///
/// Three things this deliberately is not:
///
/// - **It is not a thread.** ``start()`` returns at once and the connecting, the reading and
///   the frame decoding all happen on a queue of the session's own, because the socket read
///   blocks for as long as the client runs while ``send(_:)``, ``resize(cols:rows:)`` and
///   ``stop()`` are called from wherever the user acted. Every one of those is safe to call
///   while the reading is blocked, and none of them touches the socket on the caller's thread.
/// - **It holds no view.** The callbacks are plain closures, so the caller decides how to get
///   back to the screen, and a console session can be tested against a real socket with no
///   screen at all.
/// - **It does not throw at its caller.** A socket that has lost its framing, a bridge that is
///   not listening, a write that fails: each is reported through ``onEnd``, because this code
///   runs inside an app, and a terminal that throws into one takes the appliance down with it.
///
/// The callbacks arrive on the session's own queue, which is not the main one.
public final class ConsoleSession {

    /// The bytes the client printed, one data frame at a time and in the order they arrived.
    ///
    /// They are raw terminal bytes, not text: they are what the emulator's parser is fed, so an
    /// escape sequence in them has to arrive whole rather than as something to draw. An empty
    /// data frame is not reported at all, because a write that cannot have drawn anything is
    /// not something to repaint for.
    public var onOutput: (([UInt8]) -> Void)?

    /// The end of the session, reported exactly once.
    public var onEnd: ((ConsoleEnd) -> Void)?

    /// Where a terminal title would be reported, if the connection ever carried one.
    ///
    /// It does not today: the bridge's protocol has three frame types — data, resize and exit
    /// — and none of them is a title. A title is the terminal's own business, and the client
    /// sets it with `OSC 2`, which is a sequence ``TerminalParser`` reads out of the data
    /// stream and reports to its own sink. This exists so that the appliance has one place to
    /// be told a title, whatever ends up carrying it, and stays quiet until something does.
    public var onTitle: ((String) -> Void)?

    /// The Unix socket the bridge is listening on.
    private let socketPath: String

    /// The queue every socket operation of this session happens on.
    private let queue: DispatchQueue

    /// Guards the descriptor, the pending size and the two flags below.
    private let lock = NSLock()

    /// The connected socket, or -1 while there is none.
    private var descriptor: Int32 = -1

    /// Whether ``start()`` has done its work, which it does once.
    private var started = false

    /// Whether the session has ended, which makes it unusable and un-reportable a second time.
    private var ended = false

    /// The newest window size that had no socket to go out on.
    ///
    /// The screen is measured, and knows its size, before the bridge has answered the dial — a
    /// size dropped there is dropped for the whole session, and the client would keep the size
    /// it was started with and draw in one corner of a screen it never claims. Only the newest
    /// size is owed, because a client is owed its current size and not a history of them.
    private var pendingResize: (cols: UInt16, rows: UInt16)?

    /// A session that will dial `socketPath`, and owes the bridge a `cols` by `rows` terminal.
    ///
    /// The size is the window the client is owed from its first frame. It is kept rather than
    /// sent, because nothing is connected yet, and it goes out the moment the bridge answers.
    public init(socketPath: String, cols: UInt16, rows: UInt16) {
        self.socketPath = socketPath
        self.queue = DispatchQueue(label: "com.gmlewis.gonomadnet.console")
        if cols > 0, rows > 0 {
            self.pendingResize = (cols: cols, rows: rows)
        }
    }

    /// Dials the bridge and begins reading its frames.
    ///
    /// A dial that is refused is reported as ``ConsoleEnd/failed(_:)`` and the session is over:
    /// the bridge is started by the app before this, so a refusal means there is no bridge to
    /// talk to, and whether to try again is the caller's decision and not this class's.
    /// Calling start twice dials once.
    public func start() {
        lock.lock()
        if started || ended {
            lock.unlock()
            return
        }
        started = true
        lock.unlock()
        // The session is what the read loop runs on, so the loop holds it: a caller that drops
        // its reference without stopping the session leaves a console that is still connected
        // and still drawing, which is why stop() is what finishes it.
        queue.async { self.run() }
    }

    /// Writes terminal bytes to the bridge as one data frame.
    ///
    /// These are the bytes a terminal would have sent for a key press — see `TerminalKeys` —
    /// and the sequences a mouse report is spelled with. An empty write is not sent, because an
    /// empty write is still a write and the client would read it as something having happened,
    /// and a session that has ended sends nothing at all.
    public func send(_ bytes: [UInt8]) {
        guard !bytes.isEmpty else { return }
        writeFrame(.data(bytes))
    }

    /// Tells the bridge the terminal's new size, so the client can reflow.
    ///
    /// A size a frame cannot carry is refused rather than truncated: a truncated size is still
    /// a plausible terminal size, and the client would lay itself out for a terminal that does
    /// not exist. A size reported before the bridge answered the dial is kept rather than
    /// dropped, because the first one always is.
    public func resize(cols: UInt16, rows: UInt16) {
        guard cols > 0, rows > 0 else { return }
        lock.lock()
        if descriptor < 0 || ended {
            if !ended {
                pendingResize = (cols: cols, rows: rows)
            }
            lock.unlock()
            return
        }
        lock.unlock()
        writeFrame(.resize(cols: cols, rows: rows))
    }

    /// Ends the session, releasing the connection and reporting the end once.
    ///
    /// It is safe to call twice and safe to call after the session has already ended, and a
    /// session stopped while the bridge is still writing is stopped just the same: the
    /// descriptor is shut down, which is what wakes the reading queue. The bridge's process is
    /// the app's to reap; what this releases is the connection, which the bridge sees as the
    /// end of its own run.
    public func stop() {
        end(.stopped)
    }

    // MARK: - The session's own queue

    /// Connects, sends the size it owes, and reads frames until the session ends.
    private func run() {
        let socket = socket(AF_UNIX, SOCK_STREAM, 0)
        guard socket >= 0 else {
            end(.failed("creating the console socket: \(lastSystemError)"))
            return
        }
        // A write to a socket the bridge has closed must fail rather than kill the app: on
        // Darwin a write past a closed peer raises SIGPIPE, whose default action is to end the
        // process, which for a terminal would take the appliance with it.
        var one: Int32 = 1
        setsockopt(socket, SOL_SOCKET, SO_NOSIGPIPE, &one, socklen_t(MemoryLayout<Int32>.size))

        let dialled = withUnixSocketAddress(socketPath) { address, length in
            connect(socket, address, length)
        }
        guard let dialled else {
            close(socket)
            end(.failed("the console socket path \(socketPath) is too long for a Unix socket"))
            return
        }
        guard dialled == 0 else {
            let reason = lastSystemError
            close(socket)
            end(.failed("connecting to the console socket \(socketPath): \(reason)"))
            return
        }

        lock.lock()
        if ended {
            lock.unlock()
            close(socket)
            return
        }
        descriptor = socket
        let owed = pendingResize
        pendingResize = nil
        lock.unlock()

        // The size that was owed goes out before the first byte is read: the client is about to
        // draw, and it must draw the size the screen actually has.
        if let owed {
            writeFrame(.resize(cols: owed.cols, rows: owed.rows))
        }

        readFrames(socket)

        close(socket)
        lock.lock()
        if descriptor == socket {
            descriptor = -1
        }
        lock.unlock()
    }

    /// Reads frames until the bridge stops talking, handing each one on.
    private func readFrames(_ socket: Int32) {
        var reader = ConsoleFrameReader()
        while true {
            do {
                while let frame = try reader.next() {
                    if !deliver(frame) { return }
                }
            } catch {
                end(.failed("the console frame stream lost its framing: \(error)"))
                return
            }
            switch readSome(socket) {
            case .bytes(let chunk):
                reader.append(chunk)
            case .endOfStream:
                // A stream that ended exactly at a frame boundary is a clean end of the
                // session. One that ended with bytes still in hand was cut off inside a frame,
                // and the bytes seen since the last one cannot be trusted.
                if reader.pendingBytes > 0 {
                    end(.failed("the console stream ended inside a frame"))
                } else {
                    end(.closed)
                }
                return
            case .failed(let reason):
                end(.failed(reason))
                return
            }
        }
    }

    /// Hands one frame to the callback it belongs to, and says whether the session goes on.
    private func deliver(_ frame: ConsoleFrame) -> Bool {
        switch frame {
        case .data(let payload):
            if !payload.isEmpty {
                onOutput?(payload)
            }
            return true
        case .resize:
            // A resize frame is the app's to send, not the bridge's: the window size is the
            // screen's business, and the bridge has nothing to say about it, so one arriving
            // from the bridge is ignored rather than acted on.
            return true
        case .exit(let code):
            end(.exited(code))
            return false
        }
    }

    /// What a read of the socket found.
    private enum ReadOutcome {
        /// Bytes that arrived, at least one of them.
        case bytes([UInt8])

        /// The peer closed its end, exactly at a read boundary.
        case endOfStream

        /// The socket could not be read at all.
        case failed(String)
    }

    /// Reads whatever has arrived, up to a chunk at a time.
    ///
    /// The size is a chunk and not a frame: a socket hands over what it has, so one read can
    /// hold three frames and half of a fourth, and the reader in front of this is the buffer
    /// that puts them back together.
    private func readSome(_ socket: Int32) -> ReadOutcome {
        var buffer = [UInt8](repeating: 0, count: Self.readChunk)
        while true {
            let count = buffer.withUnsafeMutableBytes { raw -> Int in
                guard let base = raw.baseAddress else { return 0 }
                return Darwin.read(socket, base, raw.count)
            }
            if count > 0 {
                return .bytes(Array(buffer[0..<count]))
            }
            if count == 0 {
                return .endOfStream
            }
            if errno == EINTR {
                continue
            }
            return .failed("reading the console socket \(socketPath): \(lastSystemError)")
        }
    }

    /// Writes one frame to the bridge, and reports whether it went.
    @discardableResult
    private func writeFrame(_ frame: ConsoleFrame) -> Bool {
        lock.lock()
        let socket = descriptor
        let live = !ended && socket >= 0
        lock.unlock()
        guard live else { return false }

        let bytes = ConsoleFrames.encode(frame)
        var sent = 0
        while sent < bytes.count {
            let count = bytes.withUnsafeBytes { raw -> Int in
                guard let base = raw.baseAddress else { return 0 }
                return Darwin.send(socket, base.advanced(by: sent), bytes.count - sent, 0)
            }
            if count > 0 {
                sent += count
                continue
            }
            if count < 0 && errno == EINTR {
                continue
            }
            // A socket that will not take a keystroke is a socket that has gone: the bridge is
            // not reading it any more, which is the session's end.
            end(.failed("writing to the console socket \(socketPath): \(lastSystemError)"))
            return false
        }
        return true
    }

    /// The chunk a socket read asks for, which is larger than a keystroke and smaller than the
    /// largest frame: the reader in front of it is what puts whole frames back together.
    private static let readChunk = 64 * 1024

    /// Finishes the session, releasing the connection, and reports it once.
    ///
    /// The first caller wins and the rest do nothing, which is what makes an end reported from
    /// two directions — a stop from the screen and a read that fails on the session's queue —
    /// arrive as one end.
    private func end(_ reason: ConsoleEnd) {
        lock.lock()
        if ended {
            lock.unlock()
            return
        }
        ended = true
        let socket = descriptor
        descriptor = -1
        pendingResize = nil
        lock.unlock()

        // Shutting down rather than closing is what wakes the queue blocked in read(), and it
        // leaves the descriptor's release to the one place that opened it. The bridge sees its
        // own end of the connection go, which is how it knows the app has gone.
        if socket >= 0 {
            shutdown(socket, SHUT_RDWR)
        }
        onEnd?(reason)
    }
}

/// The message the last failed system call left behind, for a log line.
private var lastSystemError: String {
    String(cString: strerror(errno))
}

/// Builds the address of a Unix socket path and passes it to `body`, or returns nil when the
/// path does not fit.
///
/// `sun_path` is 104 bytes on the platforms this runs on, so a path under `NSTemporaryDirectory`
/// does not fit and cannot be dialled: the address is built here rather than at the call sites
/// so that the session and the tests that bind a socket against it use one piece of code.
func withUnixSocketAddress(
    _ path: String,
    _ body: (UnsafePointer<sockaddr>, socklen_t) -> Int32
) -> Int32? {
    var address = sockaddr_un()
    address.sun_family = sa_family_t(AF_UNIX)
    address.sun_len = UInt8(MemoryLayout<sockaddr_un>.size)
    let bytes = Array(path.utf8)
    let capacity = MemoryLayout.size(ofValue: address.sun_path)
    guard bytes.count < capacity else { return nil }
    withUnsafeMutableBytes(of: &address.sun_path) { raw in
        for (index, byte) in bytes.enumerated() {
            raw[index] = byte
        }
        raw[bytes.count] = 0
    }
    return withUnsafePointer(to: &address) { pointer in
        pointer.withMemoryRebound(to: sockaddr.self, capacity: 1) { sockaddrPointer in
            body(sockaddrPointer, socklen_t(MemoryLayout<sockaddr_un>.size))
        }
    }
}
