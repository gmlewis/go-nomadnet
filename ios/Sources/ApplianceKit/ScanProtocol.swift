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

/// The camera scan protocol, the appliance's half of it.
///
/// A dialog in the client would rather read a hex address from a QR code than have it
/// typed, and an iOS application may not start a program, so the client cannot reach a
/// camera at all. The appliance therefore reads the code and the client asks for the
/// result over a loopback connection — see ``ScanServer``.
///
/// One connection carries one request. The client sends `SCAN <token>`, and the appliance
/// answers `ADDR <payload>` or `CANCEL`. The payload is the QR code's text exactly as the
/// camera read it, because it is the client that knows what an address looks like and
/// normalizing it there is what makes that rule testable. The token is what keeps another
/// application on the device from opening the camera through this listener.
///
/// The other two halves are `tui/scanner.go` and Android's `ScanProtocol.kt`, and all three
/// are asserted against one table of bytes: a disagreement between any two of them shows up
/// to the reader as a Scan button that does nothing at all.
public enum ScanProtocol {

    /// The verb beginning the one request line.
    public static let requestVerb = "SCAN"

    /// The verb beginning a reply that carries a payload.
    public static let replyVerb = "ADDR"

    /// The whole of a reply to a reader who backed out of the camera.
    public static let cancelReply = "CANCEL"

    /// The longest request line the appliance will read.
    ///
    /// A request is one verb and a 32-character token; anything longer is not a request, and
    /// bounding the read is what stops a peer that never sends a newline from holding a
    /// connection open for as long as it likes.
    public static let maxLine = 1024

    /// The line that asks for one scan, carrying the token.
    public static func requestLine(token: String) -> String {
        "\(requestVerb) \(token)"
    }

    /// The line that answers with what the camera read.
    public static func replyLine(payload: String) -> String {
        "\(replyVerb) \(payload)"
    }

    /// The token a request line carries, or nil when the line is not a request.
    ///
    /// A line that is not exactly the verb and one non-empty token is refused rather than
    /// guessed at: refusing it costs one closed connection, and accepting it would open a
    /// camera for whatever happened to connect.
    public static func parseRequest(_ line: String) -> String? {
        let parts = line.split(separator: " ", omittingEmptySubsequences: false)
        guard parts.count == 2, parts[0] == requestVerb, !parts[1].isEmpty else {
            return nil
        }
        return String(parts[1])
    }

    /// Whether a request may open the camera.
    ///
    /// The comparison is over the whole token, and a blank token matches nothing: a launch
    /// that somehow arrived with no token must not be a launch that dropped the guard.
    public static func acceptsToken(expected: String, offered: String?) -> Bool {
        guard let offered, !expected.isEmpty, expected.count == offered.count else {
            return false
        }
        return constantTimeEquals(expected, offered)
    }

    /// Compares two strings without leaking where they first differ.
    ///
    /// The listener is on the loopback interface, so the timing a comparison leaks is not the
    /// whole of an attacker's information — but this guards a camera, the comparison costs
    /// nothing, and a guard whose strength depends on how busy the device is has a strength
    /// nobody can state.
    public static func constantTimeEquals(_ a: String, _ b: String) -> Bool {
        let left = Array(a.utf8)
        let right = Array(b.utf8)
        guard left.count == right.count else {
            return false
        }
        var difference: UInt8 = 0
        for i in left.indices {
            difference |= left[i] ^ right[i]
        }
        return difference == 0
    }
}

/// Something that can present the camera and say what it read.
///
/// It is a protocol with two implementations: the appliance's camera on a device, and a fake
/// in the tests. Everything about the protocol above is then asserted without a camera, and
/// the part that needs hardware is a screen that either shows a picture or does not.
public protocol ScanRequesting: AnyObject {
    /// Presents the scanner and returns the text of the first QR code read, or nil if the
    /// reader backed out.
    ///
    /// It is expected to block until the reader is done — the connection it is answering is
    /// waiting for exactly that — and to call its completion handler on the main queue,
    /// because presenting a camera is a UI action.
    func requestScan(completion: @escaping (String?) -> Void)
}
