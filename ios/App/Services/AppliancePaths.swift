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

import ApplianceKit
import Foundation

/// AppliancePaths is every place the appliance keeps something.
///
/// It exists as a value rather than as a pile of computed URLs because the appliance
/// writes to these paths and reads them back, and a test that has to touch the real
/// container is not a test. An appliance under test is given a directory under /tmp and
/// behaves identically.
///
/// Everything is inside the app's own container. An iOS app has no home directory in the
/// Unix sense and no `/etc`, and it is never allowed to write outside its sandbox, so
/// "where does the configuration go" has exactly one answer: here.
public struct AppliancePaths: Sendable {
    /// The container the appliance lives in.
    public let container: URL

    /// Where the Reticulum configuration the transport reads lives.
    ///
    /// The client reads its own Reticulum configuration from a *different* directory
    /// even though the two files name the same shared instance: the transport owns the
    /// instance and the client attaches to it, and running them from one directory would
    /// have the client's own directory hold the transport's identity.
    public let transportRNSDir: URL

    /// Where the client's Reticulum configuration lives.
    public let clientRNSDir: URL

    /// Where the client's own NomadNet configuration and storage live.
    public let clientConfigDir: URL

    /// Where the appliance's own logs go.
    public let logDir: URL

    /// Creates the appliance's paths under a container directory.
    public init(container: URL) {
        self.container = container
        self.transportRNSDir = container.appendingPathComponent("reticulum", isDirectory: true)
        self.clientRNSDir = container.appendingPathComponent("client-reticulum", isDirectory: true)
        self.clientConfigDir = container.appendingPathComponent("nomadnetwork", isDirectory: true)
        self.logDir = container.appendingPathComponent("logs", isDirectory: true)
    }

    /// The appliance's paths inside the app's own container, which is where a real
    /// install lives.
    public static func inContainer() -> AppliancePaths {
        let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
        return AppliancePaths(container: base.appendingPathComponent("gonomadnet", isDirectory: true))
    }

    /// The appliance's paths under a temporary directory, which is where a test lives.
    public static func inTemporaryDirectory() throws -> AppliancePaths {
        // Not FileManager.temporaryDirectory under a test harness on macOS: that path is
        // long enough to push the console's socket past the kernel's limit. /tmp is the
        // short one on both platforms.
        let base = URL(fileURLWithPath: "/tmp", isDirectory: true)
            .appendingPathComponent("gonomadnet-ios-\(ProcessInfo.processInfo.processIdentifier)", isDirectory: true)
        return AppliancePaths(container: base)
    }

    /// The channel store the client reads its RRC hubs from, in the client's own
    /// storage directory, under the name the client itself uses.
    public var hubStore: URL {
        clientConfigDir.appendingPathComponent("storage").appendingPathComponent("rrc_hubs")
    }

    /// Where the hub publishes its own `rrc.hub` destination once it has one.
    ///
    /// The bot's configuration names this address, and the client's channel list is
    /// seeded with it, so a hub that has published is reachable from both halves without
    /// either being told what it is. The name is the hub's identity path with the suffix
    /// gorrcd publishes under, because that is where the Go half writes it: the two agree
    /// on `<identity>.rrc.hub` rather than on a path either of them chose.
    public var hubDestinationFile: URL {
        clientConfigDir.appendingPathComponent("hub_identity.rrc.hub")
    }

    /// Where the bot's own TOML is written, under the client's configuration directory.
    ///
    /// It is here rather than in the bot's home directory because the Go half is told where
    /// it is: the appliance renders it, and the path it renders to is the path it names.
    public var botConfigFile: URL {
        clientConfigDir.appendingPathComponent("gorrcbot.toml")
    }

    /// The Bible text the bot's `kjv` command reads, or an empty string when the bundle has
    /// none.
    ///
    /// It is read where it is installed rather than copied into the container: it is 4.4 MB
    /// of text that nothing writes, and the bundle is already the one place it lives.
    public var kjvTxtFile: String {
        Bundle.main.url(forResource: "kjv", withExtension: "txt")?.path ?? ""
    }

    /// The console's socket.
    ///
    /// It is not beside the appliance's other files, and the reason is a hard limit rather
    /// than a preference: see `ConsoleSocket`.
    public var consoleSocket: String {
        ConsoleSocket.path(temporaryDirectory: FileManager.default.temporaryDirectory)
    }

    /// Creates every directory the appliance writes into.
    ///
    /// It is safe to call on every start: a directory that is already there is exactly
    /// what a second start should find.
    public func ensureDirectories() throws {
        for dir in [container, transportRNSDir, clientRNSDir, clientConfigDir, logDir,
                    hubStore.deletingLastPathComponent()] {
            try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        }
    }
}

/// UserDefaultsSettingsStore persists the appliance's settings.
///
/// `ApplianceSettings` is a value the tests can build and compare; this is the one place
/// it meets the device. It is separate from the settings themselves so that everything
/// decided *from* the settings can be tested without a device at all.
public final class UserDefaultsSettingsStore: SettingsStore, @unchecked Sendable {
    /// The key the appliance's settings are stored under.
    public static let key = "gonomadnet-appliance-settings"

    private let defaults: UserDefaults

    /// Creates a store over a `UserDefaults`, defaulting to the standard one.
    public init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
    }

    public func load() -> Data? {
        defaults.data(forKey: Self.key)
    }

    public func save(_ data: Data) {
        defaults.set(data, forKey: Self.key)
    }
}
