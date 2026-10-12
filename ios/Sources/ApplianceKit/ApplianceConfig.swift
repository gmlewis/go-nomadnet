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

/// The appliance's persisted settings.
///
/// They are a value type rather than a handle onto a platform preferences store, which is
/// what lets the whole of what the appliance remembers be asserted with no user defaults and
/// no device: a round-trip through `Codable` is the whole of the persistence.
public struct ApplianceSettings: Equatable, Codable, Sendable {
    /// The hub to dial, as a host name or literal address.
    ///
    /// The appliance cannot read the client's own configuration — another application's
    /// private data — so the one interface it needs is a setting of its own rather than
    /// something it copies out of the client's file. That is a deliberate duplication of a
    /// single host and port, not of the configuration as a whole: the appliance owns the
    /// transport, and the client owns none.
    public var hubHost: String
    /// The port the hub listens on.
    public var hubPort: Int
    /// Which device axis the heading is measured along.
    ///
    /// The two references degenerate in opposite mountings, so which is right depends on how
    /// the device is held. It persists because a device that lives on a stand should not have
    /// to be told twice.
    public var headingAxis: HeadingAxis
    /// Whether the appliance publishes its position and heading at all.
    public var enableSensors: Bool
    /// Whether the appliance runs its daemon stack, as opposed to acting as a bare client.
    public var enableStack: Bool
    /// The key the transport's RPC listener authenticates its clients with, as lowercase hex.
    ///
    /// Python derives this key from the transport identity in the storage directory the
    /// client runs from, and the ordinary setup is one `~/.reticulum` shared by every process
    /// on the machine. The appliance cannot share one: the client must have a configuration of
    /// its own, or it races the transport for ownership of the shared instance and one of them
    /// quietly becomes the other. With separate directories the two identities differ, and
    /// every RPC a client makes is answered "unauthorized" — an Interfaces page showing an
    /// appliance that is connected as though it were not. So both sides are told the key
    /// instead, which is the supported way to run them apart. It is generated once per install
    /// and preserved across saves: a key that changed on every start would be a transport whose
    /// clients are all rejected after every restart.
    public var rpcKey: String

    /// The home hub this project runs against, published with both an A and an AAAA record.
    public static let defaultHubHost = "go-nomadnet.duckdns.org"
    /// The port the home hub answers on.
    public static let defaultHubPort = 4242
    /// How many bytes the RPC key is, and therefore how long its hex form is.
    public static let rpcKeyBytes = 32

    /// Creates the default settings, minting a fresh RPC key for this install.
    public init() {
        hubHost = Self.defaultHubHost
        hubPort = Self.defaultHubPort
        headingAxis = .screenBack
        enableSensors = true
        enableStack = true
        rpcKey = Self.generateRPCKey()
    }

    /// Mints one RPC key as lowercase hex, which is what the transport's configuration takes.
    public static func generateRPCKey() -> String {
        var generator = SystemRandomNumberGenerator()
        let bytes = (0..<rpcKeyBytes).map { _ in UInt8.random(in: .min ... .max, using: &generator) }
        return bytes.map { String(format: "%02x", $0) }.joined()
    }

    /// The hub as `host:port`, which is how an operator types it.
    public var hubSpec: String {
        "\(hubHost):\(hubPort)"
    }
}

/// Where the appliance's settings are kept.
///
/// It is a protocol rather than a call into a preferences store so the settings can be tested
/// with no user defaults and no device, and so the platform's own store stays in the app
/// target, where the platform frameworks live.
public protocol SettingsStore {
    /// Returns the stored settings' bytes, or `nil` when nothing has been saved.
    func load() -> Data?
    /// Replaces the stored settings with `data`.
    func save(_ data: Data)
}

/// A settings store that keeps its bytes in memory, for tests and for a first run.
public final class InMemorySettingsStore: SettingsStore, @unchecked Sendable {
    /// The bytes most recently saved, or `nil` when nothing has been.
    public private(set) var data: Data?

    /// Creates an empty store, or one seeded with `data`.
    public init(data: Data? = nil) {
        self.data = data
    }

    /// Returns the stored bytes.
    public func load() -> Data? {
        data
    }

    /// Replaces the stored bytes.
    public func save(_ data: Data) {
        self.data = data
    }
}

extension ApplianceSettings {
    /// Loads the settings from `store`, or returns the defaults when nothing valid is there.
    ///
    /// A store that is absent, unreadable, or written by an older shape of this type gives the
    /// defaults rather than an error: an appliance that refused to start because its settings
    /// could not be read would be an appliance that had to be wiped to be used.
    public static func load(from store: SettingsStore) -> ApplianceSettings {
        guard let data = store.load(),
              let settings = try? JSONDecoder().decode(ApplianceSettings.self, from: data)
        else {
            return ApplianceSettings()
        }
        return settings
    }

    /// Saves the settings to `store`.
    ///
    /// The RPC key is part of the encoded value, so it is preserved by this round trip rather
    /// than regenerated: a key that changed here would reject every client after a restart.
    public func save(to store: SettingsStore) {
        guard let data = try? JSONEncoder().encode(self) else { return }
        store.save(data)
    }
}
