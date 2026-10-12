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

/// Turning a hub's name into something the transport can dial.
///
/// The renderer refuses a host name outright, and it is right to: the half that reads the
/// rendered file is a Go binary with no resolver, so a name written into it is dialled as a
/// name and the interface reports itself disconnected forever with no reason anywhere. The
/// appliance's job is to answer the question *before* that, while it can still ask the
/// system.
final class HubResolverTests: XCTestCase {
    /// A lookup that answers nothing at all, so a name is never resolved.
    private let nothing: HubResolver.Lookup = { _ in [] }

    /// A lookup with an answer per host.
    private func answering(_ answers: [String: [String]]) -> HubResolver.Lookup {
        { answers[$0] ?? [] }
    }

    /// An address is its own answer, and nobody is asked.
    func testALiteralAddressIsNotLookedUp() {
        var asked: [String] = []
        let lookup: HubResolver.Lookup = { host in
            asked.append(host)
            return ["198.51.100.7"]
        }

        XCTAssertEqual("203.0.113.9", HubResolver.resolve("203.0.113.9", lookup: lookup))
        XCTAssertTrue(asked.isEmpty, "an address must not be sent to a resolver: \(asked)")
    }

    /// An IPv6 literal is written without the brackets a reader types.
    func testAnIPv6LiteralIsUnwrapped() {
        XCTAssertEqual("2001:db8::1", HubResolver.resolve("[2001:db8::1]", lookup: nothing))
        XCTAssertEqual("2001:db8::1", HubResolver.resolve("2001:db8::1", lookup: nothing))
    }

    /// A name the system can resolve becomes the address the transport is given.
    func testANameIsResolvedToAnAddress() {
        let lookup = answering(["hub.example.org": ["203.0.113.42"]])
        XCTAssertEqual("203.0.113.42", HubResolver.resolve("hub.example.org", lookup: lookup))
    }

    /// A name that resolves to nothing has resolved to nothing, which is an answer.
    func testANameThatResolvesToNothingResolvesToNothing() {
        XCTAssertNil(HubResolver.resolve("nothing.example.org", lookup: nothing))
        XCTAssertNil(HubResolver.resolve("nothing.example.org", lookup: answering(["nothing.example.org": []])))
    }

    /// A resolver that answers with another name has not answered: the renderer refuses
    /// names, so a second one would be refused in its turn.
    func testAResolverThatAnswersWithANameIsNotAnAnswer() {
        let lookup = answering(["hub.example.org": ["hub.example.org", "elsewhere.example.org"]])
        XCTAssertNil(HubResolver.resolve("hub.example.org", lookup: lookup))
    }

    /// An empty host is not a host, which is a reader who cleared the field rather than a
    /// name that failed.
    func testAnEmptyHostResolvesToNothingWithoutAsking() {
        var asked = false
        let lookup: HubResolver.Lookup = { _ in asked = true; return [] }

        XCTAssertNil(HubResolver.resolve("", lookup: lookup))
        XCTAssertNil(HubResolver.resolve("   ", lookup: lookup))
        XCTAssertFalse(asked, "an empty host is not a name to look up")
    }

    /// The first usable address wins, so a resolver that offers an address after something
    /// unusable is still used.
    func testTheFirstUsableAddressWins() {
        let lookup = answering(["hub.example.org": ["not-an-address", "203.0.113.43"]])
        XCTAssertEqual("203.0.113.43", HubResolver.resolve("hub.example.org", lookup: lookup))
    }

    /// The appliance's own shipped default is a name, so it is the case that matters most:
    /// it resolves, or the appliance has no hub.
    func testTheShippedDefaultHubIsANameAndIsThereforeResolved() {
        let shipped = ApplianceSettings.defaultHubHost
        XCTAssertNil(
            NodeConfigRenderer.literalAddress(shipped),
            "the default hub is expected to be a name; if it is an address this test is moot"
        )
        let lookup = answering([shipped: ["203.0.113.44"]])
        XCTAssertEqual("203.0.113.44", HubResolver.resolve(shipped, lookup: lookup))
    }
}
