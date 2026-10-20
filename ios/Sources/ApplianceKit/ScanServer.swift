// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify it under the
// terms of the GNU General Public License as published by the Free Software
// Foundation, either version 3 of the License, or (at your option) any later version.
//
// This program is distributed in the hope that it will be useful, but WITHOUT ANY
// WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR A
// PARTICULAR PURPOSE. See the GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License along with
// this program. If not, see <https://www.gnu.org/licenses/>.

import Foundation

/// A scan could not be listened for.
public enum ScanServerError: Error, CustomStringConvertible {
    /// The listening socket could not be created.
    case socketFailed(errno: Int32)
    /// The socket could not be bound to the loopback address.
    case bindFailed(errno: Int32)
    /// The bound port could not be read back.
    case portUnknown(errno: Int32)
    /// The socket could not be told to accept connections.
    case listenFailed(errno: Int32)

    public var description: String {
        switch self {
        case .socketFailed(let e): return "creating the scanner's socket: \(String(cString: strerror(e)))"
        case .bindFailed(let e): return "binding the scanner to the loopback address: \(String(cString: strerror(e)))"
        case .portUnknown(let e): return "reading back the scanner's port: \(String(cString: strerror(e)))"
        case .listenFailed(let e): return "listening for scans: \(String(cString: strerror(e)))"
        }
    }
}

/// The appliance's end of the camera scan protocol.
///
/// The client runs inside this process and still cannot reach a camera: an iOS application
/// may not start a program, and the camera belongs to the app. So a dialog that would rather
/// read an address than have it typed asks this listener for one, over a loopback connection.
/// The client is told the endpoint and the token in its environment, which is how the
/// appliance tells the client everything it needs to know — see `GonomadnetSetScanEndpoint`.
///
/// The listener is deliberately thin. It reads one request line, checks the token, asks the
/// requester to read a code, and writes one reply line; the camera itself lives behind
/// ``ScanRequesting``, so this half — the half where a protocol mistake would be invisible —
/// is tested against a fake in `ScanServerTests`.
///
/// It is built on `bind`/`listen`/`accept` rather than on `NWListener`, which is what the
/// radio bridge in the app uses. The reason is the port: the client is given the endpoint
/// *before* it starts, and `NWListener` reports the port it was given only once it is ready,
/// asynchronously — which would make opening a console into a callback chain for the sake of
/// one number. Binding by hand leaves the port known the moment the server exists, and the
/// accept loop is a thread either way.
///
/// The address is the IPv4 loopback, stated rather than resolved, so that the endpoint is a
/// plain `host:port` the client can dial. A host that resolves to `::1` first would otherwise
/// hand Go an unbracketed IPv6 literal, which it refuses as "too many colons in address".
public final class ScanServer {

    /// The token every request must carry, as lower-case hex.
    public let token: String

    /// The port the client should dial.
    public let port: UInt16

    /// The host:port the client is told to dial.
    public var endpoint: String { ScanServer.endpointOf(host: ScanServer.loopbackHost, port: port) }

    /// Said out loud when a peer asks for a scan without the token.
    ///
    /// It is a closure rather than a log line here because this type is in a package that has
    /// no idea where the app's log goes; the app is told, and the app decides.
    public var onRefused: (() -> Void)?

    /// Said out loud when the client asks for a scan, before the camera is presented.
    ///
    /// It is the one moment the two halves of the feature have to agree, and the symptom of
    /// their disagreeing is silence: a Scan button that renders and does nothing looks exactly
    /// like one whose endpoint nobody is listening on. Said before the camera is shown, so it
    /// is on the record even if the camera itself refuses to start.
    public var onRequest: (() -> Void)?

    /// The camera, as this listener sees it.
    private let requester: ScanRequesting

    /// The listening socket, or -1 once it has been closed.
    private var listener: Int32

    /// The thread accepting connections, once ``start()`` has been called.
    private var thread: Thread?

    /// Set when the server is closing, so the accept loop stops rather than spinning on a
    /// closed descriptor.
    private let closing = NSLock()
    private var isClosing = false

    /// How long one scan may be waited on before the client is told there is no address.
    ///
    /// It is minutes rather than seconds because a reader aiming a camera is not a
    /// transaction: they have to find the code, hold the iPad steady, and wait for the
    /// preview to catch it. It exists at all so that a reader who walks away cannot leave a
    /// server thread waiting for the rest of the session.
    public static let scanWaitSeconds: Int = 300

    /// The loopback address the listener is bound to, and the one the client is given.
    public static let loopbackHost = "127.0.0.1"

    /// Binds a listener and returns it, without beginning to accept anything.
    ///
    /// Binding here rather than in ``start()`` is what lets the endpoint be read and handed
    /// to the client as soon as the server exists: a port that is not known until a thread
    /// has run is a port the client cannot be told. Port 0 asks the kernel for a free port,
    /// which is the only way two appliances — or a stale one still shutting down — do not
    /// collide.
    public init(requester: ScanRequesting, entropy: [UInt8] = ScanServer.randomEntropy()) throws {
        self.requester = requester
        self.token = entropy.map { String(format: "%02x", $0) }.joined()

        let fd = socket(AF_INET, SOCK_STREAM, 0)
        guard fd >= 0 else {
            throw ScanServerError.socketFailed(errno: errno)
        }

        var address = sockaddr_in()
        address.sin_family = sa_family_t(AF_INET)
        address.sin_port = 0 // any port the kernel will give
        address.sin_addr.s_addr = ScanServer.loopbackAddress

        let bound = withUnsafePointer(to: &address) { pointer in
            pointer.withMemoryRebound(to: sockaddr.self, capacity: 1) { socketAddress in
                bind(fd, socketAddress, socklen_t(MemoryLayout<sockaddr_in>.size))
            }
        }
        guard bound == 0 else {
            let failure = errno
            close(fd)
            throw ScanServerError.bindFailed(errno: failure)
        }

        var actual = sockaddr_in()
        var length = socklen_t(MemoryLayout<sockaddr_in>.size)
        let read = withUnsafeMutablePointer(to: &actual) { pointer in
            pointer.withMemoryRebound(to: sockaddr.self, capacity: 1) { socketAddress in
                getsockname(fd, socketAddress, &length)
            }
        }
        guard read == 0 else {
            let failure = errno
            close(fd)
            throw ScanServerError.portUnknown(errno: failure)
        }
        self.port = UInt16(bigEndian: actual.sin_port)

        guard listen(fd, Int32(ScanServer.backlog)) == 0 else {
            let failure = errno
            close(fd)
            throw ScanServerError.listenFailed(errno: failure)
        }
        self.listener = fd
    }

    deinit {
        stop()
    }

    /// Begins accepting connections.
    ///
    /// The thread is not a daemon — Foundation on this platform has no such flag — so
    /// ``stop()`` is what ends it in practice, and the app calls it when the console closes.
    public func start() {
        guard thread == nil else {
            return
        }
        let thread = Thread { [weak self] in self?.acceptLoop() }
        thread.name = "scan-server"
        self.thread = thread
        thread.start()
    }

    /// Stops accepting, and ends the accept loop.
    ///
    /// Closing the descriptor is what unblocks a thread already waiting in `accept`: there is
    /// no other way to interrupt one, and no state to leave behind.
    public func stop() {
        closing.lock()
        let alreadyClosed = isClosing
        isClosing = true
        closing.unlock()
        guard !alreadyClosed, listener >= 0 else {
            return
        }
        close(listener)
        listener = -1
    }

    /// Serves connections until the listener is closed.
    private func acceptLoop() {
        while !isStopping {
            var peer = sockaddr_in()
            var length = socklen_t(MemoryLayout<sockaddr_in>.size)
            let connection = withUnsafeMutablePointer(to: &peer) { pointer in
                pointer.withMemoryRebound(to: sockaddr.self, capacity: 1) { socketAddress in
                    accept(listener, socketAddress, &length)
                }
            }
            if connection < 0 {
                return
            }
            serve(connection)
        }
    }

    /// Whether the accept loop should stop rather than wait for another peer.
    private var isStopping: Bool {
        closing.lock()
        defer { closing.unlock() }
        return isClosing
    }

    /// Answers one connection, and always closes it.
    private func serve(_ connection: Int32) {
        defer { close(connection) }

        guard let line = ScanServer.readLine(connection) else {
            return
        }
        guard ScanProtocol.acceptsToken(expected: token, offered: ScanProtocol.parseRequest(line)) else {
            // A request this listener was not given the token for. It is not answered, and the
            // closed connection is what says so.
            onRefused?()
            return
        }
        onRequest?()

        let payload = readACode()
        let reply = payload.map(ScanProtocol.replyLine) ?? ScanProtocol.cancelReply
        ScanServer.writeLine(connection, reply)
    }
    /// Presents the scanner and waits for what it read.
    ///
    /// This is called on the server's thread, which is the thread holding the client's
    /// connection open, so it blocks until the reader is done: the client asked a question
    /// and is waiting for the answer. The wait is bounded because everything it is waiting on
    /// is a person.
    private func readACode() -> String? {
        let answered = DispatchSemaphore(value: 0)
        let box = AnswerBox()
        requester.requestScan { payload in
            box.set(payload)
            answered.signal()
        }
        if answered.wait(timeout: .now() + .seconds(ScanServer.scanWaitSeconds)) == .timedOut {
            return nil
        }
        return box.get()
    }

    /// The host:port the client is pointed at for a bound address.
    ///
    /// An IPv6 literal is bracketed, and that is not cosmetic: the client is Go, where
    /// `net.Dial("tcp", "::1:46635")` is "too many colons in address" and never connects. The
    /// rule is a function of its own so that both addresses can be asserted on whichever one
    /// the machine running the tests happens to have.
    public static func endpointOf(host: String, port: UInt16) -> String {
        host.contains(":") ? "[\(host)]:\(port)" : "\(host):\(port)"
    }

    /// Cryptographically strong randomness for a token.
    public static func randomEntropy() -> [UInt8] {
        var bytes = [UInt8](repeating: 0, count: 16)
        let status = SecRandomCopyBytes(kSecRandomDefault, bytes.count, &bytes)
        if status != errSecSuccess {
            // A token that is not random is still a token, and a listener bound to the
            // loopback interface with a guessable one is not a listener anyone is watching:
            // the alternative is a scan button that does not work at all.
            for i in bytes.indices {
                bytes[i] = UInt8.random(in: .min ... .max)
            }
        }
        return bytes
    }

    /// INADDR_LOOPBACK, in network byte order: 127.0.0.1.
    private static let loopbackAddress = UInt32(0x7f00_0001).bigEndian

    /// How many connections may be waiting to be accepted.
    ///
    /// One client asks once per scan and the reader presses the button once at a time, so this
    /// is a queue that should never hold more than one.
    private static let backlog = 4

    /// Reads one newline-terminated line, or nil for a line that is too long or a stream that
    /// ended.
    ///
    /// A line longer than the protocol allows is refused rather than read on past: this read
    /// is holding a server thread, and the peer may already be gone.
    private static func readLine(_ connection: Int32) -> String? {
        var line = [UInt8]()
        var byte: UInt8 = 0
        while true {
            let read = recv(connection, &byte, 1, 0)
            if read <= 0 {
                return nil
            }
            if byte == UInt8(ascii: "\n") {
                return String(decoding: line, as: UTF8.self)
            }
            if line.count >= ScanProtocol.maxLine {
                return nil
            }
            line.append(byte)
        }
    }

    /// Writes one reply line.
    private static func writeLine(_ connection: Int32, _ line: String) {
        let bytes = Array((line + "\n").utf8)
        var written = 0
        while written < bytes.count {
            let sent = bytes.withUnsafeBytes { buffer in
                send(connection, buffer.baseAddress!.advanced(by: written), bytes.count - written, 0)
            }
            if sent <= 0 {
                return
            }
            written += sent
        }
    }
}

/// Holds one answer between the queue the camera calls back on and the thread waiting for it.
private final class AnswerBox {
    private let lock = NSLock()
    private var payload: String?
    private var delivered = false

    /// set records the camera's answer.
    func set(_ value: String?) {
        lock.lock()
        defer { lock.unlock() }
        payload = value
        delivered = true
    }

    /// get returns the answer, or nil if the camera never gave one.
    func get() -> String? {
        lock.lock()
        defer { lock.unlock() }
        return payload
    }
}
