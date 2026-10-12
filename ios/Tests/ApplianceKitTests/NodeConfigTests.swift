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

/// The transport's configuration is the contract between the appliance and anything that
/// attaches to it, so every value that both sides have to agree on is asserted here rather
/// than left to a default.
final class NodeConfigTests: XCTestCase {
    /// The serial path a bridge published, as one would look on the device.
    ///
    /// It is a pseudo-terminal's slave because that is the only serial device a sandboxed
    /// application can allocate; the kernel's own node for a USB radio is root-only.
    private let bridgedPort = "/dev/pts/7"

    func testThePinnedValuesAreTheDocumentedOnes() {
        XCTAssertEqual(37428, NodeConfigRenderer.sharedInstancePort)
        XCTAssertEqual("default", NodeConfigRenderer.instanceName)
        XCTAssertEqual(37430, NodeConfigRenderer.feedPort)
        XCTAssertEqual("Home Hub", NodeConfigRenderer.hubInterfaceName)
        XCTAssertEqual("rrc.hub", NodeConfigRenderer.destName)
        XCTAssertEqual(32, NodeConfigRenderer.hubHashHexLength)
        XCTAssertEqual(37428, NodeConfigSpec.defaultSharedInstancePort)
        XCTAssertEqual("default", NodeConfigSpec.defaultInstanceName)
    }

    func testTheSharedInstanceIsATcpSocketWithThePinnedPortAndName() throws {
        let text = try NodeConfigRenderer.reticulumConfig(NodeConfigSpec())
        XCTAssertTrue(
            text.contains("shared_instance_type = tcp"),
            "the shared instance must not be an abstract Unix socket"
        )
        XCTAssertTrue(text.contains("share_instance = yes"))
        XCTAssertTrue(text.contains("shared_instance_port = 37428"))
        XCTAssertTrue(text.contains("instance_name = default"))
        XCTAssertTrue(text.contains("enable_transport = yes"))
    }

    func testTheRadioIsRenderedForTheTransport() throws {
        let rendered = try NodeConfigRenderer.reticulumConfig(
            NodeConfigSpec(rnode: RNodeSpec(port: bridgedPort))
        )
        let block = [
            "  [[RNode LoRa]]",
            "    type = RNodeInterface",
            "    interface_enabled = true",
            "    port = \(bridgedPort)",
            "    frequency = 915000000",
            "    bandwidth = 125000",
            "    txpower = 17",
            "    spreadingfactor = 9",
            "    codingrate = 5",
        ].joined(separator: "\n")
        XCTAssertTrue(rendered.contains(block), "the radio is not rendered:\n\(rendered)")
    }

    func testTheRadioIsRenderedForAClientToo() throws {
        let rendered = try NodeConfigRenderer.clientReticulumConfig(
            NodeConfigSpec(rnode: RNodeSpec(port: bridgedPort))
        )
        XCTAssertTrue(rendered.contains("[[RNode LoRa]]"))
        XCTAssertTrue(rendered.contains("port = \(bridgedPort)"))
    }

    func testARadioCanBeLeftOut() throws {
        let rendered = try NodeConfigRenderer.reticulumConfig(NodeConfigSpec(rnode: nil))
        XCTAssertFalse(rendered.contains("RNodeInterface"))
    }

    func testAFreshInstallCarriesAnEnabledRadioBeforeAnyRadioHasBeenPluggedIn() throws {
        let spec = NodeConfigSpec()
        XCTAssertNotNil(spec.rnode, "a fresh install has no radio interface at all")
        XCTAssertTrue(spec.rnode?.enabled ?? false, "the default radio is switched off")

        let rendered = try NodeConfigRenderer.reticulumConfig(spec)
        XCTAssertTrue(rendered.contains("[[RNode LoRa]]"))
        XCTAssertTrue(rendered.contains("    interface_enabled = true"))
        XCTAssertTrue(rendered.contains("    port = \(RNodeSpec.defaultPort)"))
    }

    func testTheRadioDefaultsAreTheDocumentedOnes() {
        let radio = RNodeSpec()
        XCTAssertEqual("RNode LoRa", radio.name)
        XCTAssertEqual("/dev/ttyACM0", radio.port)
        XCTAssertEqual(915_000_000, radio.frequency)
        XCTAssertEqual(125_000, radio.bandwidth)
        XCTAssertEqual(17, radio.txpower)
        XCTAssertEqual(9, radio.spreadingFactor)
        XCTAssertEqual(5, radio.codingRate)
        XCTAssertTrue(radio.enabled)
    }

    func testAnInterfaceIsRenderedAsATcpClientWithALiteralAddress() throws {
        let text = try NodeConfigRenderer.reticulumConfig(
            NodeConfigSpec(interfaces: [InterfaceSpec(name: "Home Hub", host: "203.0.113.7", port: 4242)])
        )
        XCTAssertTrue(text.contains("[[Home Hub]]"))
        XCTAssertTrue(text.contains("type = TCPClientInterface"))
        XCTAssertTrue(text.contains("target_host = 203.0.113.7"))
        XCTAssertTrue(text.contains("target_port = 4242"))
    }

    func testAHostNameIsRefusedRatherThanWritten() throws {
        XCTAssertThrowsError(
            try NodeConfigRenderer.reticulumConfig(
                NodeConfigSpec(
                    interfaces: [
                        InterfaceSpec(name: "Home Hub", host: "go-nomadnet.duckdns.org", port: 4242)
                    ]
                )
            )
        ) { error in
            let message = (error as? LocalizedError)?.errorDescription ?? "\(error)"
            XCTAssertTrue(message.contains("go-nomadnet.duckdns.org"), message)
            XCTAssertTrue(message.contains("cannot resolve names"), message)
        }
    }

    func testNoHostNameEverSurvivesIntoTheRenderedConfiguration() throws {
        let rendered = try NodeConfigRenderer.reticulumConfig(
            NodeConfigSpec(
                interfaces: [
                    InterfaceSpec(name: "Hub", host: "203.0.113.7", port: 4242),
                    InterfaceSpec(name: "IPv6 Hub", host: "[2001:db8::1]", port: 4242),
                    InterfaceSpec(name: "Loopback", host: "127.0.0.1", port: 37428),
                ]
            )
        )
        XCTAssertFalse(rendered.contains("duckdns.org"))
        XCTAssertTrue(rendered.contains("target_host = 203.0.113.7"))
        XCTAssertTrue(
            rendered.contains("target_host = 2001:db8::1"),
            "an IPv6 literal is written without its brackets:\n\(rendered)"
        )
    }

    func testNothingInTheReticulumConfigIsQuoted() throws {
        let rendered = try NodeConfigRenderer.reticulumConfig(
            NodeConfigSpec(
                interfaces: [
                    InterfaceSpec(name: "Home Hub", host: "203.0.113.7", port: 4242),
                    InterfaceSpec(name: "IPv6 Hub", host: "2001:db8::1", port: 4242),
                ]
            )
        )
        XCTAssertFalse(
            rendered.contains("\""),
            "the transport's configuration must contain no quotation marks:\n\(rendered)"
        )
    }

    func testTheClientConfigIsUnquotedForTheSameReason() throws {
        let client = try NodeConfigRenderer.clientReticulumConfig(NodeConfigSpec())
        XCTAssertFalse(
            client.contains("\""),
            "the client configuration must contain no quotation marks:\n\(client)"
        )
    }

    func testTheClientListsTheInterfacesTheTransportDials() throws {
        let spec = NodeConfigSpec(
            interfaces: [InterfaceSpec(name: "Home Hub", host: "203.0.113.7", port: 4242)]
        )
        let transport = try NodeConfigRenderer.reticulumConfig(spec)
        let client = try NodeConfigRenderer.clientReticulumConfig(spec)

        XCTAssertTrue(client.contains("[[Home Hub]]"))
        XCTAssertTrue(client.contains("target_host = 203.0.113.7"))
        XCTAssertTrue(client.contains("target_port = 4242"))
        XCTAssertEqual(
            transport.components(separatedBy: "[interfaces]\n").last,
            client.components(separatedBy: "[interfaces]\n").last,
            "the two configuration files disagree about what this appliance connects through"
        )
        XCTAssertTrue(client.contains("require_shared_instance = yes"))
        XCTAssertTrue(client.contains("enable_transport = no"))
    }

    func testAnEmptyInterfaceListIsStatedRatherThanLeftBlank() throws {
        let text = try NodeConfigRenderer.reticulumConfig(NodeConfigSpec(rnode: nil))
        XCTAssertTrue(text.contains("[interfaces]"))
        XCTAssertTrue(
            text.contains("Deliberately empty"),
            "an empty transport is a deliberate configuration and should say so"
        )
    }

    func testTheClientStillStatesAnEmptyInterfaceListRatherThanLeavingItBlank() throws {
        let client = try NodeConfigRenderer.clientReticulumConfig(NodeConfigSpec(rnode: nil))
        XCTAssertTrue(client.contains("[interfaces]"))
        XCTAssertTrue(client.contains("Deliberately empty"))
    }

    func testAnApplianceWithNoHubInterfaceSaysSoAndStillCarriesTheRadio() throws {
        let text = try NodeConfigRenderer.reticulumConfig(
            NodeConfigSpec(rnode: RNodeSpec(port: bridgedPort))
        )
        XCTAssertFalse(text.contains("Deliberately empty"))
        XCTAssertTrue(text.contains("No hub interface"))
        XCTAssertTrue(text.contains("[[RNode LoRa]]"))
    }

    func testTheTransportAndClientCarryTheRpcKeyWhenTheApplianceStatesOne() throws {
        let key = String(repeating: "ab", count: 32)
        let spec = NodeConfigSpec(rpcKey: " \(key) ")
        let transport = try NodeConfigRenderer.reticulumConfig(spec)
        let client = try NodeConfigRenderer.clientReticulumConfig(spec)
        XCTAssertTrue(transport.contains("rpc_key = \(key)"), transport)
        XCTAssertTrue(client.contains("rpc_key = \(key)"), client)
    }

    func testATransportWithNoKeyStatedIsNotGivenOne() throws {
        let transport = try NodeConfigRenderer.reticulumConfig(NodeConfigSpec())
        let client = try NodeConfigRenderer.clientReticulumConfig(NodeConfigSpec())
        XCTAssertFalse(transport.contains("rpc_key"))
        XCTAssertFalse(client.contains("rpc_key"))
    }

    func testTheBotReadsTheLiveFeedFromTheTwoFifos() {
        let text = NodeConfigRenderer.gorrcbotConfig(
            BotConfigSpec(
                gpsPort: "/files/run/gps.nmea",
                compassPort: "/files/run/compass.nmea",
                hubDestination: "a012129c10205c0b9441fcd2b755b2a7"
            )
        )
        XCTAssertTrue(text.contains("[bot]"))
        XCTAssertTrue(text.contains("  nick = \"gobot\""), text)
        XCTAssertTrue(text.contains("gps_port = \"/files/run/gps.nmea\""))
        XCTAssertTrue(text.contains("compass_port = \"/files/run/compass.nmea\""))
        XCTAssertTrue(text.contains("[[hubs]]"))
        XCTAssertTrue(text.contains("destination = \"a012129c10205c0b9441fcd2b755b2a7\""))
        XCTAssertTrue(text.contains("rooms = [\"general\"]"))
        XCTAssertTrue(text.contains("respond_to = { \"general\" = \"gobot\" }"))
    }

    func testTheBotIsToldWhereTheBundledBibleTextIs() {
        let text = NodeConfigRenderer.gorrcbotConfig(
            BotConfigSpec(
                gpsPort: "/gps",
                compassPort: "/compass",
                kjvTxtFile: "/files/kjv.txt",
                hubDestination: "a012129c10205c0b9441fcd2b755b2a7"
            )
        )
        XCTAssertTrue(text.contains("kjv_txt_file = \"/files/kjv.txt\""), text)
    }

    func testABotWithNoBibleTextIsNotToldToReadOne() {
        let text = NodeConfigRenderer.gorrcbotConfig(
            BotConfigSpec(gpsPort: "/gps", compassPort: "/compass", hubDestination: "ab")
        )
        XCTAssertFalse(text.contains("kjv_txt_file"), text)
    }

    func testLiteralAddressAcceptsAddressesAndRefusesNames() {
        for address in ["127.0.0.1", "203.0.113.7", "0.0.0.0", "[2001:db8::1]", "::1"] {
            XCTAssertNotNil(NodeConfigRenderer.literalAddress(address), "\(address) is a literal")
        }
        for name in ["", "  ", "example.com", "host", "1.2.3", "1.2.3.4.5", "999.1.1.1", "1.2.3.a"] {
            XCTAssertNil(NodeConfigRenderer.literalAddress(name), "\(name) is a name")
        }
    }

    func testTomlStringsAreQuotedAndEscaped() {
        XCTAssertEqual("\"plain\"", NodeConfigRenderer.escapeTomlString("plain"))
        XCTAssertEqual("\"a\\\"b\"", NodeConfigRenderer.escapeTomlString("a\"b"))
        XCTAssertEqual("\"a\\\\b\"", NodeConfigRenderer.escapeTomlString("a\\b"))
    }
}
