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

/// What the appliance remembers, and what it must keep remembering.
///
/// The RPC key is the one value here whose loss is silent: both the transport and the client are
/// told it, and a key that changed on a restart would leave every RPC answered "unauthorized" —
/// an Interfaces page showing an appliance that is connected as though it were not.
final class ApplianceConfigTests: XCTestCase {
    func testTheDefaultsAreTheApplianceOnes() {
        let settings = ApplianceSettings()
        XCTAssertEqual("go-nomadnet.duckdns.org", settings.hubHost)
        XCTAssertEqual(4242, settings.hubPort)
        XCTAssertEqual(.screenBack, settings.headingAxis)
        XCTAssertTrue(settings.enableSensors)
        XCTAssertTrue(settings.enableStack)
        XCTAssertEqual("go-nomadnet.duckdns.org:4242", settings.hubSpec)
        XCTAssertEqual("go-nomadnet.duckdns.org", ApplianceSettings.defaultHubHost)
        XCTAssertEqual(4242, ApplianceSettings.defaultHubPort)
    }

    func testAnRpcKeyIsThirtyTwoBytesOfHexAndDifferentEveryTime() {
        let key = ApplianceSettings.generateRPCKey()
        XCTAssertEqual(
            ApplianceSettings.rpcKeyBytes * 2, key.count,
            "the key must be 32 bytes: the transport reads it as hex"
        )
        XCTAssertTrue(key.allSatisfy { $0.isHexDigit && !$0.isUppercase }, key)
        XCTAssertNotEqual(ApplianceSettings.generateRPCKey(), ApplianceSettings.generateRPCKey())
        XCTAssertEqual(ApplianceSettings.rpcKeyBytes * 2, ApplianceSettings().rpcKey.count)
    }

    func testTheSettingsSurviveACodableRoundTrip() throws {
        var settings = ApplianceSettings()
        settings.hubHost = "203.0.113.7"
        settings.hubPort = 4242
        settings.headingAxis = .screenTop
        settings.enableSensors = false
        settings.enableStack = false

        let data = try JSONEncoder().encode(settings)
        XCTAssertEqual(settings, try JSONDecoder().decode(ApplianceSettings.self, from: data))
    }

    func testTheRpcKeyIsPreservedAcrossSaves() {
        // A key that changed here would be a transport whose clients are all rejected after a
        // restart.
        let store = InMemorySettingsStore()
        var settings = ApplianceSettings()
        settings.hubHost = "203.0.113.7"
        let key = settings.rpcKey
        settings.save(to: store)

        let loaded = ApplianceSettings.load(from: store)
        XCTAssertEqual(key, loaded.rpcKey)
        XCTAssertEqual(settings, loaded)

        // And saving what was loaded keeps the same key again.
        loaded.save(to: store)
        XCTAssertEqual(key, ApplianceSettings.load(from: store).rpcKey)
    }

    func testAnAbsentOrUnreadableStoreGivesTheDefaults() {
        // An appliance that refused to start because its settings could not be read would be an
        // appliance that had to be wiped to be used.
        let defaults = ApplianceSettings.load(from: InMemorySettingsStore()).hubSpec
        XCTAssertEqual("go-nomadnet.duckdns.org:4242", defaults)

        let garbage = InMemorySettingsStore(data: Data("not json".utf8))
        XCTAssertEqual(defaults, ApplianceSettings.load(from: garbage).hubSpec)
        XCTAssertFalse(ApplianceSettings.load(from: garbage).rpcKey.isEmpty)
    }

    func testTheStoreProtocolIsAllPersistenceNeeds() {
        let store = InMemorySettingsStore()
        XCTAssertNil(store.load())
        store.save(Data("hello".utf8))
        XCTAssertEqual(Data("hello".utf8), store.load())
    }
}
