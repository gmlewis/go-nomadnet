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

import Foundation

/// One channel as the client's own store describes it.
///
/// The client keeps its channels in `rrc_hubs`, where each entry is a hub destination,
/// the name to show it under, and the rooms to join. The appliance writes that store
/// before the client ever runs, so a freshly installed appliance arrives with channels
/// to open rather than an empty Channels page and nothing to connect to.
public struct DefaultHub: Equatable, Sendable {
    /// The name the channel is listed under.
    public var name: String
    /// The hub's truncated destination hash, as hex: sixteen bytes, thirty-two digits.
    public var destination: String
    /// The rooms the client joins on this hub.
    public var rooms: [String]

    /// Creates one channel entry.
    public init(name: String, destination: String, rooms: [String]) {
        self.name = name
        self.destination = destination
        self.rooms = rooms
    }
}

/// The channels a freshly installed appliance already knows about.
///
/// Three channels, because they are the three an appliance can actually reach:
///
///  * the community hub, which carries the wider RRC network;
///  * this project's public hub;
///  * the hub running **inside this appliance**, which is where its own bot answers, and
///    whose destination is different on every install because it is derived from an
///    identity generated on the device. It is written only when that identity has
///    published its destination, which the hub does a moment after it starts.
///
/// The store is the client's own file in the client's own format — CBOR, the one the Go
/// client's `rrc` manager saves — so this encoder is a second implementation of that
/// format. A store the client cannot read is one it silently ignores, leaving the
/// appliance with no channels at all and no error anywhere to say so.
public enum DefaultHubs {
    /// The hub the wider Reticulum community uses.
    public static let rnsCommunityName = "RNS Community"
    /// Its RRC destination.
    public static let rnsCommunityDestination = "28c7c1a68c735693aa8e6b8193ed44b2"

    /// This project's public hub.
    public static let publicHubName = "gonomadnet Public Hub"
    /// Its RRC destination.
    public static let publicHubDestination = "a012129c10205c0b9441fcd2b755b2a7"

    /// The name the local hub is listed under, which is the name it announces.
    public static let localHubName = "appliance-hub"

    /// The room every one of these hubs carries.
    public static let defaultRoom = "general"

    /// The destination name hubs announce under; the client's own default.
    public static let destName = "rrc.hub"

    /// A hub destination's length in hex: sixteen bytes.
    public static let hubHashHexLength = 32

    /// The channels to seed, which are the two public ones and, when it is known, the hub
    /// inside the appliance.
    ///
    /// `localHub` is that hub's destination, or `nil` when it has not published one yet —
    /// an appliance whose stack has never been started has no local hub to list, and
    /// inventing a destination for it would be inventing one that cannot exist.
    public static func hubs(localHub: String?) -> [DefaultHub] {
        var out = [
            DefaultHub(
                name: rnsCommunityName,
                destination: rnsCommunityDestination,
                rooms: [defaultRoom]
            ),
            DefaultHub(
                name: publicHubName,
                destination: publicHubDestination,
                rooms: [defaultRoom]
            ),
        ]
        if let hex = localHubHex(localHub) {
            out.append(DefaultHub(name: localHubName, destination: hex, rooms: [defaultRoom]))
        }
        return out
    }

    /// Encodes the whole store: a map with one `hubs` key holding the list.
    ///
    /// `localHub`, when it is a valid destination and no entry of `hubs` already carries
    /// the local hub's name, is appended as the appliance's own hub. That lets a caller
    /// hold the two public channels as a value and hand in the local destination as it
    /// becomes known, which is exactly how the appliance seeds them across two console
    /// opens.
    ///
    /// The entries' keys are written in alphabetical order, which is the order the client
    /// writes them, so the bytes are comparable with a store the client has saved. A Go
    /// test decodes these same bytes with the client's own reader, which is the
    /// cross-language proof that the client loads what this seeds.
    public static func encode(_ hubs: [DefaultHub], localHub: String? = nil) -> [UInt8] {
        var all = hubs
        if let hex = localHubHex(localHub), !all.contains(where: { $0.name == localHubName }) {
            all.append(DefaultHub(name: localHubName, destination: hex, rooms: [defaultRoom]))
        }
        var out: [UInt8] = []
        writeHead(&out, major: majorMap, value: 1)
        writeText(&out, "hubs")
        writeHead(&out, major: majorArray, value: all.count)
        for hub in all {
            writeHub(&out, hub)
        }
        return out
    }

    /// Reports whether a store should be written over.
    ///
    /// A store that is absent has never been written, and one that is still byte-identical
    /// to the appliance's own seed is one nobody has edited — that is the record that this
    /// has been done, and both may be written or completed. Anything else is the operator's,
    /// or the client's own rewrite, and is left exactly alone: a channel the operator removes
    /// stays removed, and a store the client has saved is never second-guessed.
    ///
    /// `seeded` is the appliance's own stable seed — the two-hub store it writes before its
    /// local hub has a destination — so an untouched two-hub store is recognized even when
    /// the caller is about to complete it into a three-hub one.
    public static func shouldSeed(existing: [UInt8]?, seeded: [UInt8]) -> Bool {
        guard let existing else { return true }
        return existing == seeded
    }

    /// Writes the store to a file, and reports whether it wrote.
    ///
    /// The write is guarded by `shouldSeed` against the appliance's own two-hub seed, so an
    /// operator's edits survive; the file that lands is the most complete store the caller can
    /// produce at that moment. A directory that is not there yet is created, because the
    /// console is opened on a fresh install before the client has ever made its own.
    @discardableResult
    public static func seed(store: URL, localHub: String?) -> Bool {
        let existing = try? Data(contentsOf: store).map { $0 }
        if FileManager.default.fileExists(atPath: store.path), existing == nil {
            // A path that exists but is not a readable regular file is not a store, and
            // not ours to replace.
            return false
        }
        let ownSeed = encode(hubs(localHub: nil))
        guard shouldSeed(existing: existing, seeded: ownSeed) else { return false }
        let wanted = encode(hubs(localHub: localHub))
        if existing == wanted {
            return false
        }
        do {
            try FileManager.default.createDirectory(
                at: store.deletingLastPathComponent(),
                withIntermediateDirectories: true
            )
            try Data(wanted).write(to: store, options: .atomic)
        } catch {
            return false
        }
        return true
    }

    /// Decodes a destination, which is sixteen bytes — the client stores a hub's truncated
    /// destination hash, not the thirty-two byte one — or `nil` when the text is not exactly
    /// that much hex.
    public static func hexBytes(_ hex: String) -> [UInt8]? {
        guard hex.count == hubHashHexLength,
              hex.allSatisfy({ $0.isHexDigit && !$0.isUppercase })
        else { return nil }
        var out: [UInt8] = []
        out.reserveCapacity(hex.count / 2)
        var index = hex.startIndex
        while index < hex.endIndex {
            let next = hex.index(index, offsetBy: 2)
            guard let byte = UInt8(hex[index..<next], radix: 16) else { return nil }
            out.append(byte)
            index = next
        }
        return out
    }

    /// Returns a destination as lowercase hex, or `nil` when it is not one.
    private static func localHubHex(_ localHub: String?) -> String? {
        guard let localHub else { return nil }
        let hex = localHub.trimmingCharacters(in: .whitespaces).lowercased()
        return hexBytes(hex) == nil ? nil : hex
    }

    /// Writes one hub entry, with its keys in the order the client writes them, which is
    /// alphabetical.
    ///
    /// The three auto flags are on, which is a departure from the client's own default of off
    /// and is the whole point of seeding them: an appliance is expected to reconnect to its
    /// channels by itself, and to be showing who is in a room when somebody looks at it. They
    /// are the operator's to change afterwards, from the channel list.
    private static func writeHub(_ out: inout [UInt8], _ hub: DefaultHub) {
        let destination = hexBytes(hub.destination) ?? []
        writeHead(&out, major: majorMap, value: 8)
        writeText(&out, "auto_list")
        writeBool(&out, true)
        writeText(&out, "auto_reconnect")
        writeBool(&out, true)
        writeText(&out, "auto_who")
        writeBool(&out, true)
        writeText(&out, "dest_name")
        writeText(&out, destName)
        writeText(&out, "hash")
        writeBytes(&out, destination)
        writeText(&out, "name")
        writeText(&out, hub.name)
        writeText(&out, "parted_rooms")
        writeHead(&out, major: majorArray, value: 0)
        writeText(&out, "rooms")
        writeHead(&out, major: majorArray, value: hub.rooms.count)
        for room in hub.rooms {
            writeText(&out, room)
        }
    }

    /// CBOR's head byte: the major type and, for values under 24, the value itself;
    /// otherwise a length marker followed by the value in the fewest whole bytes that hold
    /// it.
    private static func writeHead(_ out: inout [UInt8], major: Int, value: Int) {
        let prefix = UInt8(major << 5)
        switch value {
        case ..<24:
            out.append(prefix | UInt8(value))
        case ..<0x100:
            out.append(prefix | 24)
            out.append(UInt8(value))
        case ..<0x1_0000:
            out.append(prefix | 25)
            out.append(UInt8(value >> 8))
            out.append(UInt8(value & 0xff))
        default:
            out.append(prefix | 26)
            out.append(UInt8((value >> 24) & 0xff))
            out.append(UInt8((value >> 16) & 0xff))
            out.append(UInt8((value >> 8) & 0xff))
            out.append(UInt8(value & 0xff))
        }
    }

    /// Writes a text string. Its bytes are UTF-8, and its length is counted in those bytes
    /// rather than in characters: a room name outside ASCII would otherwise be given a length
    /// the reader would walk past, which turns the rest of the store into noise.
    private static func writeText(_ out: inout [UInt8], _ text: String) {
        let bytes = Array(text.utf8)
        writeHead(&out, major: majorText, value: bytes.count)
        out.append(contentsOf: bytes)
    }

    /// Writes a byte string, whose length is likewise in bytes.
    private static func writeBytes(_ out: inout [UInt8], _ bytes: [UInt8]) {
        writeHead(&out, major: majorBytes, value: bytes.count)
        out.append(contentsOf: bytes)
    }

    /// Writes a CBOR boolean, which has dedicated head bytes rather than a major type.
    private static func writeBool(_ out: inout [UInt8], _ value: Bool) {
        out.append(value ? boolTrue : boolFalse)
    }

    private static let majorBytes = 2
    private static let majorText = 3
    private static let majorArray = 4
    private static let majorMap = 5
    private static let boolFalse: UInt8 = 0xf4
    private static let boolTrue: UInt8 = 0xf5
}
