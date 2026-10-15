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

/// The camera scan protocol's half that can be asserted without a camera.
///
/// The lines here are the ones `tui/scanner.go` reads and writes, and the same table is
/// asserted by Android's `ScanProtocolTest`. A disagreement between any two of the three
/// halves does not fail anywhere: it shows up to the reader as a Scan button that does
/// nothing at all, which is why the exact bytes are pinned rather than the shape of them.
final class ScanProtocolTests: XCTestCase {

    func testTheRequestLineIsTheVerbAndTheToken() {
        XCTAssertEqual(ScanProtocol.requestLine(token: "0f1e2d3c"), "SCAN 0f1e2d3c")
    }

    func testTheReplyLineIsTheVerbAndThePayload() {
        XCTAssertEqual(ScanProtocol.replyLine(payload: "lxmf@2a61"), "ADDR lxmf@2a61")
    }

    func testARequestParsesToItsToken() {
        XCTAssertEqual(ScanProtocol.parseRequest(ScanProtocol.requestLine(token: "0f1e2d3c")), "0f1e2d3c")
    }

    func testALineThatIsNotARequestParsesToNothing() {
        let lines = [
            "",
            "SCAN",
            "SCAN ",
            "SCAN a b",
            "scan 0f1e2d3c",
            "ADDR 0f1e2d3c",
            "SCAN\t0f1e2d3c",
            "0f1e2d3c",
        ]
        for line in lines {
            XCTAssertNil(ScanProtocol.parseRequest(line), "parseRequest(\"\(line)\") parsed a token")
        }
    }

    func testATokenIsAcceptedOnlyWhenItIsTheWholeOfTheOfferedOne() {
        XCTAssertTrue(ScanProtocol.acceptsToken(expected: "0f1e2d3c", offered: "0f1e2d3c"))
        XCTAssertFalse(ScanProtocol.acceptsToken(expected: "0f1e2d3c", offered: "0f1e2d3"))
        XCTAssertFalse(ScanProtocol.acceptsToken(expected: "0f1e2d3c", offered: "0f1e2d3cd"))
        XCTAssertFalse(ScanProtocol.acceptsToken(expected: "0f1e2d3c", offered: "0f1e2d3d"))
        XCTAssertFalse(ScanProtocol.acceptsToken(expected: "0f1e2d3c", offered: nil))
        XCTAssertFalse(ScanProtocol.acceptsToken(expected: "0f1e2d3c", offered: ""))
    }

    func testAListenerWithNoTokenAcceptsNothing() {
        // A launch that somehow arrived without a token must not be one that dropped the
        // guard: the empty string is not a token that anything can offer.
        XCTAssertFalse(ScanProtocol.acceptsToken(expected: "", offered: ""))
        XCTAssertFalse(ScanProtocol.acceptsToken(expected: "", offered: "anything"))
    }

    func testTheConstantTimeComparisonAgreesWithEquality() {
        XCTAssertTrue(ScanProtocol.constantTimeEquals("", ""))
        XCTAssertTrue(ScanProtocol.constantTimeEquals("abc", "abc"))
        XCTAssertFalse(ScanProtocol.constantTimeEquals("abc", "abd"))
        XCTAssertFalse(ScanProtocol.constantTimeEquals("abc", "ab"))
    }
}
