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

import XCTest
@testable import ApplianceKit

/// The scan listener, driven the way the Go client drives it.
///
/// Every case here opens a real connection to a real loopback listener and speaks the real
/// protocol; only the camera is a fake. That is deliberate: the framing, the token check and
/// the reply are the parts where a mistake is silent, and they are also the parts that can be
/// asserted without a device.
final class ScanServerTests: XCTestCase {

    /// A camera that answers from a script, and counts the scans it was asked for.
    private final class FakeRequester: ScanRequesting {
        private let payload: String?
        private let lock = NSLock()
        private var count = 0

        init(payload: String?) {
            self.payload = payload
        }

        var calls: Int {
            lock.lock()
            defer { lock.unlock() }
            return count
        }

        func requestScan(completion: @escaping (String?) -> Void) {
            lock.lock()
            count += 1
            lock.unlock()
            completion(payload)
        }
    }

    /// A counter the listener's own thread writes and the test's thread reads.
    ///
    /// The lock is what makes that a fact rather than a hope: the two callbacks this counts are
    /// called on the server's thread, and an `Int` written there and read here is a data race,
    /// however likely the socket's ordering makes it to look like one that is not.
    private final class Counter {
        private let lock = NSLock()
        private var count = 0

        func increment() {
            lock.lock()
            defer { lock.unlock() }
            count += 1
        }

        var value: Int {
            lock.lock()
            defer { lock.unlock() }
            return count
        }
    }

    /// Sends one request line to a running server and returns the reply, or nil if the peer
    /// closed without one.
    private func ask(_ server: ScanServer, _ line: String) throws -> String? {
        let fd = socket(AF_INET, SOCK_STREAM, 0)
        XCTAssertGreaterThanOrEqual(fd, 0, "could not open a client socket")
        defer { close(fd) }

        var address = sockaddr_in()
        address.sin_family = sa_family_t(AF_INET)
        address.sin_port = server.port.bigEndian
        address.sin_addr.s_addr = UInt32(0x7f00_0001).bigEndian
        let connected = withUnsafePointer(to: &address) { pointer in
            pointer.withMemoryRebound(to: sockaddr.self, capacity: 1) { socketAddress in
                connect(fd, socketAddress, socklen_t(MemoryLayout<sockaddr_in>.size))
            }
        }
        XCTAssertEqual(connected, 0, "could not connect to the scanner on \(server.endpoint)")

        let request = Array((line + "\n").utf8)
        _ = request.withUnsafeBytes { send(fd, $0.baseAddress, request.count, 0) }

        var reply = [UInt8]()
        var byte: UInt8 = 0
        while true {
            let read = recv(fd, &byte, 1, 0)
            if read <= 0 {
                return reply.isEmpty ? nil : String(decoding: reply, as: UTF8.self)
            }
            if byte == UInt8(ascii: "\n") {
                return String(decoding: reply, as: UTF8.self)
            }
            reply.append(byte)
        }
    }

    func testARequestCarryingTheTokenIsAnsweredWithWhatTheCameraRead() throws {
        let requester = FakeRequester(payload: "lxmf@2a6105f57145860441a62fe3b2a1352c")
        let server = try ScanServer(requester: requester)
        server.start()
        defer { server.stop() }

        XCTAssertEqual(try ask(server, ScanProtocol.requestLine(token: server.token)),
                       "ADDR lxmf@2a6105f57145860441a62fe3b2a1352c")
        XCTAssertEqual(requester.calls, 1)
    }

    func testAReaderWhoBackedOutIsAnsweredWithTheCancelLine() throws {
        let requester = FakeRequester(payload: nil)
        let server = try ScanServer(requester: requester)
        server.start()
        defer { server.stop() }

        XCTAssertEqual(try ask(server, ScanProtocol.requestLine(token: server.token)),
                       ScanProtocol.cancelReply)
        XCTAssertEqual(requester.calls, 1)
    }

    func testARequestWithTheWrongTokenOpensNoCamera() throws {
        let requester = FakeRequester(payload: "anything")
        let server = try ScanServer(requester: requester)
        server.start()
        defer { server.stop() }

        let wrong = server.token == "wrong" ? "also-wrong" : "wrong"
        XCTAssertNil(try ask(server, ScanProtocol.requestLine(token: wrong)))
        XCTAssertEqual(requester.calls, 0,
                       "the appliance opened the camera for a request it should have refused")
    }

    func testALineThatIsNotARequestOpensNoCamera() throws {
        let requester = FakeRequester(payload: "anything")
        let server = try ScanServer(requester: requester)
        server.start()
        defer { server.stop() }

        for line in ["", "HELLO", "ADDR 2a61", "SCAN", "SCAN a b"] {
            XCTAssertNil(try ask(server, line), "the appliance answered \"\(line)\"")
        }
        XCTAssertEqual(requester.calls, 0,
                       "the appliance opened the camera for a line that is not a request")
    }

    func testARequestThatCarriesTheTokenIsAnnounced() throws {
        // The appliance says out loud that the client asked, before it shows a camera. It is
        // the one moment the two halves of the feature have to agree, and the symptom of their
        // disagreeing is silence: a button that renders and does nothing produces no line at
        // all.
        let requester = FakeRequester(payload: nil)
        let server = try ScanServer(requester: requester)
        let announced = Counter()
        server.onRequest = { announced.increment() }
        server.start()
        defer { server.stop() }

        _ = try ask(server, ScanProtocol.requestLine(token: server.token))
        // The announcement is made before the reply is written, so the reply arriving is what
        // says it has happened.
        XCTAssertEqual(announced.value, 1)
    }

    func testARequestWithoutTheTokenIsNotAnnounced() throws {
        let requester = FakeRequester(payload: nil)
        let server = try ScanServer(requester: requester)
        let announced = Counter()
        server.onRequest = { announced.increment() }
        server.start()
        defer { server.stop() }

        let wrong = server.token == "wrong" ? "also-wrong" : "wrong"
        XCTAssertNil(try ask(server, ScanProtocol.requestLine(token: wrong)))
        XCTAssertEqual(announced.value, 0, "a request with no token was announced as the client's")
    }

    func testTheListenerAnswersMoreThanOnce() throws {        let requester = FakeRequester(payload: "payload")
        let server = try ScanServer(requester: requester)
        server.start()
        defer { server.stop() }

        for _ in 0..<3 {
            XCTAssertEqual(try ask(server, ScanProtocol.requestLine(token: server.token)), "ADDR payload")
        }
        XCTAssertEqual(requester.calls, 3)
    }

    func testTheEndpointNamesTheLoopbackHostAndThePortTheKernelGave() throws {
        let server = try ScanServer(requester: FakeRequester(payload: nil))
        defer { server.stop() }

        // Named rather than resolved, and IPv4 rather than whatever the loopback happens to
        // be: Go refuses an unbracketed IPv6 literal with "too many colons in address", so
        // this endpoint is one Go dials.
        XCTAssertEqual(server.endpoint, "127.0.0.1:\(server.port)")
        XCTAssertFalse(server.endpoint.contains("::"), "the endpoint names an IPv6 literal: \(server.endpoint)")
    }

    func testAnIPv6LiteralIsBracketedAndAnIPv4OneIsNot() {
        // The rule, stated on both addresses rather than on whichever one this machine
        // happens to have. These are the two strings Go's net.Dial accepts.
        XCTAssertEqual(ScanServer.endpointOf(host: "::1", port: 46635), "[::1]:46635")
        XCTAssertEqual(ScanServer.endpointOf(host: "127.0.0.1", port: 46635), "127.0.0.1:46635")
    }

    func testTheTokenIsHexadecimalWhichIsWhatTheGoHalfParses() throws {
        // The client is handed the token in its environment, so it has to be hex digits and
        // nothing else.
        let server = try ScanServer(requester: FakeRequester(payload: nil), entropy: [0x00, 0x0f, 0xff, 0x10])
        defer { server.stop() }
        XCTAssertEqual(server.token, "000fff10")
    }

    func testTheListenerIsOnTheLoopbackInterface() throws {
        // A listener on any other address would be a camera offered to the whole network.
        let server = try ScanServer(requester: FakeRequester(payload: nil))
        defer { server.stop() }
        XCTAssertTrue(ScanServer.loopbackHost.hasPrefix("127."),
                      "the scanner is not bound to the loopback interface: \(ScanServer.loopbackHost)")
    }

    func testStoppingTheServerLetsGoOfItsPort() throws {
        // The port is released when the descriptor is closed, so a console that is closed and
        // reopened does not leave a listener nothing is draining.
        let first = try ScanServer(requester: FakeRequester(payload: nil))
        first.start()
        let port = first.port
        first.stop()

        let second = try ScanServer(requester: FakeRequester(payload: nil))
        defer { second.stop() }
        XCTAssertNotEqual(second.port, port, "the scanner kept a port a closed one had")
    }
}
