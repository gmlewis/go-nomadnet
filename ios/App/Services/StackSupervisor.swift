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

/// StackSupervisor is the appliance's own supervisor: it renders the configuration
/// the Go half reads, brings the stack up and down, and keeps the console's place.
///
/// On Android this job belongs to a Kotlin `StackSupervisor` that watches eight child
/// processes and restarts the ones that die. There are no children here, so there is
/// nothing to watch: what is left is the part that was always the interesting one —
/// writing the files the subsystems read, in the order they read them, and bringing
/// them up in an order that lets each one depend on the last.
///
/// Everything it decides is a pure function in `ApplianceKit`, tested on the machine
/// that builds the appliance. What lives here is the file writing and the calling,
/// which is the part a device has to do.
@MainActor
public final class StackSupervisor {
    /// What the appliance is doing, as the controls show it.
    public enum State: Equatable {
        case stopped
        case starting
        case running
        case stopping
        case failed(String)

        /// Whether the stack is up.
        public var isRunning: Bool { self == .running }

        /// Whether a start or stop is in flight.
        public var isBusy: Bool { self == .starting || self == .stopping }
    }

    /// The appliance's state, published so the controls can show it.
    public private(set) var state: State = .stopped {
        didSet { onStateChange?(state) }
    }

    /// onStateChange is called on the main actor whenever the state changes.
    public var onStateChange: ((State) -> Void)?

    /// onLog is called on the main actor with a line worth showing the reader.
    public var onLog: ((String) -> Void)?

    /// The appliance's own settings, which its configuration is rendered from.
    public private(set) var settings = ApplianceSettings()

    /// The console socket the app dials, once a console has been started.
    public private(set) var consolePath: String?

    /// radioInterface is the radio the transport is rendered for, or nil for none.
    ///
    /// It is asked at every start rather than remembered, because pairing and unpairing
    /// happen while the appliance is running: a radio paired after the configuration was
    /// written is one the next start renders, and one that has gone away is one the next
    /// start leaves out.
    public var radioInterface: () -> RNodeSpec? = { nil }

    private let paths: AppliancePaths
    private let settingsStore: SettingsStore

    public init(paths: AppliancePaths = .inContainer(), settingsStore: SettingsStore = UserDefaultsSettingsStore()) {
        self.paths = paths
        self.settingsStore = settingsStore
        self.settings = ApplianceSettings.load(from: settingsStore)
    }

    /// loadSettings re-reads the persisted settings, after the controls have saved them.
    public func reloadSettings() {
        settings = ApplianceSettings.load(from: settingsStore)
    }

    /// start brings the appliance up.
    ///
    /// The order is the appliance. The configuration is written first, because a
    /// subsystem that starts against a file that is not there yet reads a default that
    /// belongs to nothing. The channels are seeded before the client can read them. The
    /// transport is up before anything attaches to its shared instance, and the sensor
    /// feed is up inside the Go half before the bot reads it.
    public func start() {
        guard !state.isBusy, !state.isRunning else { return }
        state = .starting

        do {
            try prepareFiles()
            try ApplianceBridge.start(
                configDir: paths.clientConfigDir.path,
                rnsConfigDir: paths.transportRNSDir.path,
                feedPort: Int32(NodeConfigRenderer.feedPort)
            )
            state = .running
            log("the appliance is running; the transport is \(settings.hubSpec)")
        } catch {
            state = .failed(message(for: error))
            log("the appliance did not start: \(message(for: error))")
        }
    }

    /// prepare writes everything the appliance reads, without starting anything.
    ///
    /// This is what the first launch does, and it is a separate step because the
    /// configuration has to be a fact before the appliance is a running process: the Config
    /// editor has something to show, a reader who opens the client's configuration finds the
    /// file the appliance will use rather than the client's own untouched default, and
    /// "the appliance is set up" is something that can be checked rather than hoped for.
    ///
    /// It writes the same files start does, and leaves them alone when they are already
    /// byte for byte what it would write, so calling it on every launch is what keeps an
    /// operator's own edits.
    public func prepare() {
        do {
            try prepareFiles()
            log("the configuration is written; nothing is started")
        } catch {
            // A preparation that failed is not a reason to refuse to run: start writes the
            // same files again and reports the failure the reader can act on.
            log("the configuration was not written: \(message(for: error))")
        }
    }

    /// prepareFiles creates the directories and writes every file the appliance reads.
    private func prepareFiles() throws {
        try paths.ensureDirectories()
        try writeConfigurations()
        _ = DefaultHubs.seed(store: paths.hubStore, localHub: nil)
    }

    /// stop takes the appliance down and releases everything it holds.
    public func stop() {
        guard state.isRunning || state == .failed("") else { return }
        state = .stopping
        consolePath = nil
        do {
            try ApplianceBridge.stop()
        } catch {
            log("the appliance did not stop cleanly: \(message(for: error))")
        }
        state = .stopped
        log("the appliance is stopped")
    }

    /// startConsole attaches the NomadNet client to a console and returns the socket
    /// the app dials.
    ///
    /// The socket name is as short as it can be, and it is checked rather than assumed:
    /// the kernel's `sun_path` holds 104 bytes, and an iOS app's own temporary
    /// directory already spends most of that before the name.
    public func startConsole(cols: UInt16, rows: UInt16) throws -> String {
        guard state.isRunning else {
            throw StackError.consoleWithoutStack
        }

        let name = "gm.sock"
        let path = paths.consoleSocket(name)
        guard path.utf8.count <= Self.maxSocketPath else {
            throw StackError.socketPathTooLong(path: path, limit: Self.maxSocketPath)
        }

        try ApplianceBridge.startConsole(socketPath: path, cols: cols, rows: rows)
        consolePath = path
        log("the console is listening on \(path)")
        return path
    }

    /// hubDestination is the appliance's own hub address, once the hub has published one.
    ///
    /// A hub that has not published yet is not an address: the client's channel list is
    /// seeded with the two public channels and the local one is added when it exists,
    /// rather than being invented.
    public func localHubDestination() -> String? {
        guard let data = try? Data(contentsOf: paths.hubDestinationFile) else { return nil }
        let hex = String(decoding: data, as: UTF8.self).trimmingCharacters(in: .whitespacesAndNewlines)
        return hex.isEmpty ? nil : hex
    }

    // MARK: - Configuration

    /// writeConfigurations renders every file the Go half reads.
    ///
    /// A file that is already there is left exactly as it is when it is byte for byte
    /// what the appliance would write, which is what keeps an operator's own edits from
    /// being overwritten by the next launch.
    private func writeConfigurations() throws {
        // A paired radio is rendered as the interface the transport dials it through: the
        // loopback address the radio bridge serves, which is the transport's own `tcp://`
        // RNode mode. With no radio paired there is no interface at all, because one enabled
        // against an address nothing is listening on would only report itself disconnected
        // for the life of the install.
        let radio = radioInterface()

        // Both Reticulum configurations come from the one rendering the Config editor
        // also uses, so an edit the editor accepted is the edit the stack starts with:
        // there is no second path from settings to bytes for the two to drift along.
        let rendered = try SettingsEdit.render(
            hubHost: settings.hubHost,
            hubPort: settings.hubPort,
            rpcKey: settings.rpcKey,
            rnode: radio
        )
        try write(rendered.transport, to: paths.transportRNSDir.appendingPathComponent("config"))
        try write(rendered.client, to: paths.clientRNSDir.appendingPathComponent("config"))

        // A hub that did not resolve is not a failure to start: it is an appliance with no
        // hub to dial, which is what Android renders in the same case. Saying so is what
        // stops a reader looking for a transport that is up with nothing to connect through.
        if let problem = rendered.hubProblem {
            log(problem)
        } else if let address = rendered.hubAddress {
            log("the hub \(settings.hubHost) resolved to \(address)")
        }

        // The client's own configuration is the client's default with three values
        // changed, and the template is the one the Go module ships the first time. What
        // is there afterwards is the file itself: an operator who edited it keeps every
        // byte the appliance does not own, so editing is not undone by the next start.
        let client = try ClientConfig.applied(to: clientConfigText())
        try write(client, to: clientConfigFile)
    }

    // MARK: - The reader's edits

    /// updateSettings saves a change to the settings the configuration is rendered from.
    ///
    /// The change is saved as it is made, because the settings are what the next start
    /// renders from: an edit that was only a screen's is an edit the transport never sees.
    public func updateSettings(_ change: (inout ApplianceSettings) -> Void) {
        var updated = settings
        change(&updated)
        updated.save(to: settingsStore)
        settings = updated
    }

    /// The client's own configuration file.
    public var clientConfigFile: URL {
        paths.clientConfigDir.appendingPathComponent("config")
    }

    /// clientConfigText is the client's configuration as the editor reads it: the file
    /// that is there, or the shipped default before the first start has written one.
    public func clientConfigText() throws -> String {
        if let existing = try? String(contentsOf: clientConfigFile, encoding: .utf8) {
            return existing
        }
        return try clientTemplate()
    }

    /// saveClientConfig writes an edited client configuration.
    ///
    /// The appliance's own values are stated over the operator's text, so an edit
    /// anywhere else in the file survives and the appliance still hosts a named node with
    /// an editor it can name. What is written is what the next Start Stack starts with —
    /// the editor's Save and the stack's start agree because they call the same thing.
    ///
    /// - Throws: `ClientConfigError.missingKeys` when the edit dropped a key the
    ///   appliance has to state, which is reported rather than saved.
    public func saveClientConfig(_ text: String) throws {
        try write(try ClientConfig.applied(to: text), to: clientConfigFile)
    }

    /// clientTemplate is the client's default configuration, which the Go module owns
    /// and the build stages into the app's bundle.
    private func clientTemplate() throws -> String {
        guard let url = Bundle.main.url(forResource: "default", withExtension: "conf"),
              let template = try? String(contentsOf: url, encoding: .utf8)
        else {
            throw StackError.missingClientTemplate
        }
        return template
    }

    /// write writes a rendered file, leaving an identical one alone.
    private func write(_ text: String, to url: URL) throws {
        let data = Data(text.utf8)
        if let existing = try? Data(contentsOf: url), existing == data {
            return
        }
        try FileManager.default.createDirectory(at: url.deletingLastPathComponent(), withIntermediateDirectories: true)
        try data.write(to: url, options: .atomic)
    }

    // MARK: - Helpers

    private func message(for error: Error) -> String {
        if let bridgeError = error as? ApplianceBridge.ApplianceError {
            return bridgeError.description
        }
        return "\(error)"
    }

    private func log(_ line: String) {
        onLog?(line)
    }

    /// maxSocketPath is all the kernel's `sun_path` holds, its terminating NUL included.
    static let maxSocketPath = 103
}

/// StackError is why the appliance could not be brought up or attached to.
public enum StackError: Error, CustomStringConvertible {
    /// A console was asked for while the stack was not running.
    ///
    /// The client attaches to the transport's shared instance, and a client attached to
    /// nothing renders an empty interface with no error anywhere.
    case consoleWithoutStack

    /// The console's socket path does not fit the kernel's `sun_path`.
    case socketPathTooLong(path: String, limit: Int)

    /// The client's default configuration is not in the app bundle.
    case missingClientTemplate

    public var description: String {
        switch self {
        case .consoleWithoutStack:
            return "start the stack before opening the console: the client attaches to the appliance's own transport"
        case .socketPathTooLong(let path, let limit):
            return "the console socket path is \(path.utf8.count) bytes: \(path)\nThe kernel allows \(limit)."
        case .missingClientTemplate:
            return "the client's default configuration is missing from the app bundle"
        }
    }
}
