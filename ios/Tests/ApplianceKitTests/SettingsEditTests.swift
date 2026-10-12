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

/// The tests for what an edited setting renders to.
///
/// They assert the bytes that will be written rather than anything a screen shows: the
/// reader's edit is real when the file the transport reads names the new host, and the file
/// is what this produces.
///
/// A name is never written, so every one of these either states an address or states that
/// the hub has none: the resolver is injected, which is what makes the whole question
/// answerable with no DNS and no network.
final class SettingsEditTests: XCTestCase {
    /// An address that needs no resolution, because the device may have no DNS at all.
    private let address = "203.0.113.9"
    /// A hub named by a host name, which is what an operator usually has.
    private let hostName = "hub.example.org"
    /// The address that name resolves to.
    private let resolved = "203.0.113.42"
    /// The port the hub listens on.
    private let port = 4242
    /// A key of the shape the appliance mints.
    private let key = String(repeating: "ab", count: ApplianceSettings.rpcKeyBytes)

    /// A lookup that resolves the test's name and nothing else.
    private var lookup: HubResolver.Lookup {
        let hostName = self.hostName
        let resolved = self.resolved
        return { $0 == hostName ? [resolved] : [] }
    }

    /// A lookup that resolves nothing at all.
    private var nothing: HubResolver.Lookup { { _ in [] } }

    func testAnEditedHubIsWhatTheRenderedConfigurationNames() throws {
        let rendered = try SettingsEdit.render(
            hubHost: address, hubPort: port, rpcKey: key, lookup: nothing
        )

        XCTAssertTrue(rendered.transport.contains("target_host = \(address)"), rendered.transport)
        XCTAssertTrue(rendered.transport.contains("target_port = \(port)"), rendered.transport)
        XCTAssertTrue(rendered.client.contains("target_host = \(address)"), rendered.client)
        XCTAssertTrue(rendered.client.contains("target_port = \(port)"), rendered.client)
        XCTAssertEqual(address, rendered.hubAddress)
        XCTAssertNil(rendered.hubProblem)
    }

    /// A hub named by a name is written as the address it resolved to, because the file is
    /// read by a program that cannot resolve one.
    func testAHubNamedByANameIsRenderedAsItsAddress() throws {
        let rendered = try SettingsEdit.render(
            hubHost: hostName, hubPort: port, rpcKey: key, lookup: lookup
        )

        XCTAssertTrue(rendered.transport.contains("target_host = \(resolved)"), rendered.transport)
        XCTAssertTrue(rendered.client.contains("target_host = \(resolved)"), rendered.client)
        XCTAssertFalse(rendered.transport.contains(hostName), "a name must never be rendered")
        XCTAssertEqual(resolved, rendered.hubAddress)
        XCTAssertNil(rendered.hubProblem)
    }

    /// A hub that did not resolve leaves the appliance with no hub to dial, and says so —
    /// and everything else is still rendered, because an appliance with no hub still runs.
    func testAHubThatDidNotResolveIsRenderedWithoutAHubAndSaysWhy() throws {
        let rendered = try SettingsEdit.render(
            hubHost: hostName, hubPort: port, rpcKey: key, lookup: nothing
        )

        XCTAssertFalse(rendered.transport.contains("target_host"), rendered.transport)
        XCTAssertTrue(rendered.transport.contains("Deliberately empty"), rendered.transport)
        XCTAssertNil(rendered.hubAddress)
        let problem = try XCTUnwrap(rendered.hubProblem)
        XCTAssertTrue(problem.contains(hostName), problem)
        // The rest of the file is still the appliance's: the transport owns the shared
        // instance, and a hub that is not there is not a reason to have no transport.
        XCTAssertTrue(rendered.transport.contains("enable_transport = yes"), rendered.transport)
        XCTAssertTrue(rendered.transport.contains(key), rendered.transport)
    }

    /// The transport owns the shared instance; the client only ever attaches to it.
    func testTheTransportOwnsTheInstanceAndTheClientAttachesToIt() throws {
        let rendered = try SettingsEdit.render(
            hubHost: address, hubPort: port, rpcKey: key, lookup: nothing
        )

        XCTAssertTrue(rendered.transport.contains("enable_transport = yes"), rendered.transport)
        XCTAssertTrue(rendered.client.contains("enable_transport = no"), rendered.client)
        XCTAssertTrue(
            rendered.client.contains("require_shared_instance = yes"),
            rendered.client
        )
    }

    /// A hub port of zero is a hub the appliance does not dial, not a hub on port zero, and
    /// it is not a problem to report.
    func testNoPortRendersNoHubInterfaceAndNoProblem() throws {
        let rendered = try SettingsEdit.render(
            hubHost: address, hubPort: 0, rpcKey: key, lookup: nothing
        )

        XCTAssertFalse(rendered.transport.contains("target_host"), rendered.transport)
        XCTAssertTrue(rendered.transport.contains("Deliberately empty"), rendered.transport)
        XCTAssertNil(rendered.hubProblem)
    }

    /// The key both sides authenticate with travels into both files, because the two
    /// run from different directories and would derive different keys from them.
    func testTheRPCKeyIsStatedInBothFiles() throws {
        let rendered = try SettingsEdit.render(
            hubHost: address, hubPort: port, rpcKey: key, lookup: nothing
        )

        XCTAssertTrue(rendered.transport.contains(key), rendered.transport)
        XCTAssertTrue(rendered.client.contains(key), rendered.client)
    }

    /// A radio the appliance has paired renders as the transport's own interface, and
    /// the port is the loopback one the radio bridge serves.
    func testAPairedRadioIsRenderedAsAReachableInterface() throws {
        let radio = RNodeSpec(port: RadioAddress.loopback)
        let rendered = try SettingsEdit.render(
            hubHost: address, hubPort: port, rpcKey: key, rnode: radio, lookup: nothing
        )

        XCTAssertTrue(rendered.transport.contains("type = RNodeInterface"), rendered.transport)
        XCTAssertTrue(
            rendered.transport.contains("port = tcp://127.0.0.1:37431"),
            rendered.transport
        )
    }

    /// No radio is no radio section at all: an interface enabled against a port nothing
    /// is listening on reports itself disconnected for the life of the install.
    func testNoRadioRendersNoRadioSection() throws {
        let rendered = try SettingsEdit.render(
            hubHost: address, hubPort: port, rpcKey: key, lookup: nothing
        )

        XCTAssertFalse(rendered.transport.contains("RNodeInterface"), rendered.transport)
    }
}
