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

/// The channels an appliance comes with, and the file it writes them into.
///
/// The store is the client's own file in the client's own format, so this encoder is the second
/// implementation of a format that already has one. The golden files are what pin the two
/// together: this test asserts the encoder produces exactly those bytes, and a Go test in the
/// same repository decodes those same bytes with the client's own reader. A store the client
/// cannot read is reported as an error at startup and otherwise ignored, so the appliance would
/// show an empty Channels page with nothing anywhere saying why.
final class DefaultHubsTests: XCTestCase {
    /// The local hub's destination the fixtures were written for.
    private let localDestination = "00112233445566778899aabbccddeeff"

    private var resourcesURL: URL {
        TestPaths.repoRoot
            .appendingPathComponent("android")
            .appendingPathComponent("app")
            .appendingPathComponent("src")
            .appendingPathComponent("test")
            .appendingPathComponent("resources")
    }

    /// The store as the client's own encoder writes it, with all three channels.
    private func fixture() throws -> [UInt8] {
        try Array(Data(contentsOf: resourcesURL.appendingPathComponent("default_hubs.cbor")))
    }

    /// The seed the appliance writes before the local hub has a destination.
    private func twoHubFixture() throws -> [UInt8] {
        try Array(Data(contentsOf: resourcesURL.appendingPathComponent("default_hubs_no_local.cbor")))
    }

    /// The three channels the fixture was written for.
    private var expected: [DefaultHub] {
        [
            DefaultHub(name: "RNS Community", destination: "28c7c1a68c735693aa8e6b8193ed44b2", rooms: ["general"]),
            DefaultHub(name: "gonomadnet Public Hub", destination: "a012129c10205c0b9441fcd2b755b2a7", rooms: ["general"]),
            DefaultHub(name: "appliance-hub", destination: "00112233445566778899aabbccddeeff", rooms: ["general"]),
        ]
    }

    func testTheEncoderWritesExactlyWhatTheClientsOwnReaderExpects() throws {
        XCTAssertEqual(try fixture(), DefaultHubs.encode(expected))
    }

    func testTheEncoderCompletesTheLocalHubFromItsDestination() throws {
        // The signature the appliance uses: the two public channels as a value, and the local
        // hub appended once its identity has published a destination.
        let base = DefaultHubs.hubs(localHub: nil)
        XCTAssertEqual(try fixture(), DefaultHubs.encode(base, localHub: localDestination))
    }

    func testTheTwoHubSeedIsAStoreTheClientCanRead() throws {
        XCTAssertEqual(try twoHubFixture(), DefaultHubs.encode(DefaultHubs.hubs(localHub: nil)))
    }

    func testAnApplianceWithALocalHubComesWithThreeChannels() {
        let hubs = DefaultHubs.hubs(localHub: localDestination)
        XCTAssertEqual(3, hubs.count)
        XCTAssertEqual(
            [DefaultHubs.rnsCommunityName, DefaultHubs.publicHubName, DefaultHubs.localHubName],
            hubs.map(\.name)
        )
        for hub in hubs {
            XCTAssertEqual(["general"], hub.rooms)
        }
    }

    func testALocalHubThatHasNotPublishedADestinationIsNotInvented() {
        // The local hub's destination is derived from an identity generated on the device, so
        // an appliance whose stack has never run has nothing to list it under. Writing a
        // destination nobody can reach would be a channel that can never connect.
        XCTAssertEqual(2, DefaultHubs.hubs(localHub: nil).count)
        XCTAssertEqual(2, DefaultHubs.hubs(localHub: "").count)
        XCTAssertEqual(2, DefaultHubs.hubs(localHub: "   ").count)
        XCTAssertEqual(2, DefaultHubs.hubs(localHub: "not-a-hash").count)
    }

    func testTheSeedWritesOnceAndNeverAgain() throws {
        let store = TestPaths.makeTempDirectory().appendingPathComponent("rrc_hubs")

        XCTAssertTrue(DefaultHubs.seed(store: store, localHub: localDestination))
        XCTAssertEqual(try fixture(), try Array(Data(contentsOf: store)))

        // The file belongs to the client from the moment it runs: it holds whatever the
        // operator has added or removed, and an appliance that rewrote it on every start would
        // undo that.
        try Data("the operator's own store".utf8).write(to: store)
        XCTAssertFalse(DefaultHubs.seed(store: store, localHub: nil))
        XCTAssertEqual("the operator's own store", try String(contentsOf: store, encoding: .utf8))
    }

    func testAStoreThatIsStillTheApplianceOwnSeedIsCompletedWhenTheLocalHubAppears() throws {
        // The first console open happens before the stack has ever run, so the appliance writes
        // the two public hubs and no local one. The bytes are still its own — nobody has edited
        // them — so the next open, by which time the local hub has published a destination, may
        // complete them.
        let store = TestPaths.makeTempDirectory().appendingPathComponent("rrc_hubs")
        XCTAssertTrue(DefaultHubs.seed(store: store, localHub: nil))
        XCTAssertEqual(try twoHubFixture(), try Array(Data(contentsOf: store)))

        XCTAssertTrue(DefaultHubs.seed(store: store, localHub: localDestination))
        XCTAssertEqual(try fixture(), try Array(Data(contentsOf: store)))

        XCTAssertFalse(DefaultHubs.seed(store: store, localHub: localDestination))
        XCTAssertEqual(try fixture(), try Array(Data(contentsOf: store)))
    }

    func testAStoreAnyoneElseHasChangedIsLeftExactlyAsItIs() throws {
        // Anything that is not byte-for-byte the appliance's own seed is somebody's decision:
        // the client's, once it has saved, or the operator's, once they have added or removed a
        // channel. The byte rule exists precisely so removing `appliance-hub` sticks.
        let others: [String: [UInt8]] = [
            "an operator-edited store": Array("the operator's own store".utf8),
            "a two-hub store an operator built": DefaultHubs.encode([
                DefaultHub(name: "RNS Community", destination: DefaultHubs.rnsCommunityDestination, rooms: ["general"]),
                DefaultHub(name: "My Hub", destination: "ffeeddccbbaa99887766554433221100", rooms: ["general"]),
            ]),
            "a truncated store": Array(try fixture().prefix(try fixture().count / 2)),
            "an empty store": [],
        ]
        for (what, bytes) in others {
            let store = TestPaths.makeTempDirectory().appendingPathComponent("rrc_hubs")
            try Data(bytes).write(to: store)
            XCTAssertFalse(DefaultHubs.seed(store: store, localHub: localDestination), what)
            XCTAssertEqual(bytes, try Array(Data(contentsOf: store)), what)
        }
    }

    func testShouldSeedOnlyForAnAbsentOrUntouchedStore() {
        let seed = DefaultHubs.encode(DefaultHubs.hubs(localHub: nil))
        XCTAssertTrue(DefaultHubs.shouldSeed(existing: nil, seeded: seed))
        XCTAssertTrue(DefaultHubs.shouldSeed(existing: seed, seeded: seed))
        XCTAssertFalse(DefaultHubs.shouldSeed(existing: [], seeded: seed))
        XCTAssertFalse(
            DefaultHubs.shouldSeed(existing: Array("someone else's".utf8), seeded: seed)
        )
    }

    func testAStoreWrittenIntoADirectoryThatIsNotThereYetIsStillWritten() {
        // The console is opened on a fresh install before the client has ever created its own
        // storage directory.
        let store = TestPaths.makeTempDirectory()
            .appendingPathComponent("storage/rrc_hubs")
        XCTAssertTrue(DefaultHubs.seed(store: store, localHub: nil))
        XCTAssertTrue(FileManager.default.fileExists(atPath: store.path))
    }

    func testATextLengthIsCountedInBytesAndNotInCharacters() {
        // A room or a hub name outside ASCII would otherwise be given a length the reader walks
        // past, turning the rest of the store into noise.
        let name = "ééé"
        let encoded = DefaultHubs.encode([
            DefaultHub(name: name, destination: localDestination, rooms: ["general"]),
        ])
        let needle = Array(name.utf8)
        guard let at = encoded.firstIndex(ofBytes: needle) else {
            return XCTFail("the name is not in the store at all")
        }
        XCTAssertEqual(needle.count, Int(encoded[at - 1] & 0x1f))
    }

    func testOnlySixteenBytesOfHexIsADestination() {
        XCTAssertNil(DefaultHubs.hexBytes("00112233445566778899aabbccddee"))
        XCTAssertNil(DefaultHubs.hexBytes("00112233445566778899aabbccddeeff00"))
        XCTAssertNil(DefaultHubs.hexBytes("00112233445566778899aabbccddeefg"))
        XCTAssertNil(DefaultHubs.hexBytes("00112233445566778899AABBCCDDEEFF"))
        XCTAssertEqual(16, DefaultHubs.hexBytes("00112233445566778899aabbccddeeff")?.count)
        XCTAssertEqual(
            [0x28, 0xc7, 0xc1, 0xa6],
            Array(DefaultHubs.hexBytes("28c7c1a68c735693aa8e6b8193ed44b2")?.prefix(4) ?? [])
        )
    }

    func testTheDefaultHubNamesAreTheApplianceOnes() {
        XCTAssertEqual("RNS Community", DefaultHubs.rnsCommunityName)
        XCTAssertEqual("28c7c1a68c735693aa8e6b8193ed44b2", DefaultHubs.rnsCommunityDestination)
        XCTAssertEqual("gonomadnet Public Hub", DefaultHubs.publicHubName)
        XCTAssertEqual("a012129c10205c0b9441fcd2b755b2a7", DefaultHubs.publicHubDestination)
        XCTAssertEqual("appliance-hub", DefaultHubs.localHubName)
        XCTAssertEqual(["general"], [DefaultHubs.defaultRoom])
        XCTAssertEqual("rrc.hub", DefaultHubs.destName)
    }
}

extension Array where Element == UInt8 {
    /// The offset of `needle` in this array, or `nil` when it does not occur.
    func firstIndex(ofBytes needle: [UInt8]) -> Int? {
        guard !needle.isEmpty, needle.count <= count else { return nil }
        for start in 0...(count - needle.count) where Array(self[start..<(start + needle.count)]) == needle {
            return start
        }
        return nil
    }
}
