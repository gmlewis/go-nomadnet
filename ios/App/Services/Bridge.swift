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

/// The appliance's Go half, as Swift sees it.
///
/// The functions behind this are declared in `ios/Sources/GonomadnetBridge/include/bridge.h`
/// and implemented by the static archive the build script produces from
/// `./cmd/gonomadnet`. The header is the project's bridging header, so the C entry
/// points are visible directly; this type exists to give them names Swift can read,
/// to turn their status codes into errors, and to make the one ownership rule they
/// have impossible to get wrong.
///
/// The rule is about the reason string: `GonomadnetLastError` returns memory Go owns,
/// and it is released by `GonomadnetFreeError` rather than by Swift. Wrapping that in
/// one place is the only way to be sure every call site does it.
public enum ApplianceBridge {
    /// A status the Go half reports. The numbers are the C contract.
    public enum Status: Int32 {
        case ok = 0
        case alreadyRunning = 1
        case notRunning = 2
        case badArgument = 3
        case failed = 4
        case shutDown = 5

        /// Whether the status means the call did what it says.
        ///
        /// `alreadyRunning` is a success and not a failure: the app's launch path and
        /// its start button can both ask, and the second ask is answered by the first.
        public var isSuccess: Bool { self == .ok || self == .alreadyRunning }
    }

    /// ApplianceError is why a call to the Go half failed.
    public struct ApplianceError: Error, CustomStringConvertible {
        /// The status the call returned.
        public let status: Status

        /// The reason the Go half recorded, when it recorded one.
        public let reason: String

        public var description: String {
            reason.isEmpty ? "the appliance reported status \(status.rawValue)" : reason
        }
    }

    /// version is the Go NomadNet version the appliance runs.
    public static var version: String {
        guard let raw = GonomadnetVersion() else { return "" }
        defer { GonomadnetFreeError(raw) }
        return String(cString: raw)
    }

    /// start brings the appliance up: the Reticulum transport, the sensor feed, the
    /// hub and the bot, in that order.
    ///
    /// - Parameters:
    ///   - configDir: the client's own configuration directory.
    ///   - rnsConfigDir: the Reticulum configuration directory the appliance rendered.
    ///   - feedPort: the loopback port the sensor feed binds; 0 means the pinned one.
    /// - Throws: `ApplianceError` when the appliance could not be brought up.
    public static func start(configDir: String, rnsConfigDir: String, feedPort: Int32 = 0) throws {
        let status = Status(rawValue: GonomadnetStart(configDir, rnsConfigDir, feedPort)) ?? .failed
        try check(status, "starting the appliance")
    }

    /// stop shuts the appliance down and releases everything it holds.
    ///
    /// It is safe to call more than once, and safe to call on an appliance that never
    /// started, which is what an app being torn down needs.
    public static func stop() throws {
        let status = Status(rawValue: GonomadnetStop()) ?? .failed
        // A stop with nothing to stop is not a failure: the app is being closed either
        // way, and there is nothing left to report to anybody.
        if status == .notRunning || status == .alreadyRunning { return }
        try check(status, "stopping the appliance")
    }

    /// startConsole listens for the app to attach and runs the NomadNet client on it.
    ///
    /// - Parameters:
    ///   - socketPath: the Unix socket the app dials. Keep it short: the kernel's
    ///     `sun_path` holds 104 bytes, and iOS's own temporary directory is long
    ///     enough for a careless name to reach that limit.
    ///   - cols: the terminal's initial width in columns.
    ///   - rows: the terminal's initial height in rows.
    public static func startConsole(socketPath: String, cols: UInt16, rows: UInt16) throws {
        let status = Status(rawValue: GonomadnetStartConsole(socketPath, cols, rows)) ?? .failed
        try check(status, "starting the console")
    }

    /// feedLocation publishes one GNSS fix to the appliance's sensor stream.
    ///
    /// A measurement the receiver did not take is passed as `nil` and published as an
    /// empty NMEA field, so a height nobody measured is never reported as standing at
    /// sea level.
    public static func feedLocation(
        latitude: Double,
        longitude: Double,
        altitude: Double?,
        horizontalAccuracy: Double?,
        speedKnots: Double?,
        courseDegrees: Double?,
        timestamp: Date
    ) throws {
        let status = Status(rawValue: GonomadnetFeedLocation(
            latitude,
            longitude,
            altitude ?? .nan,
            horizontalAccuracy ?? .nan,
            speedKnots ?? .nan,
            courseDegrees ?? .nan,
            timestamp.timeIntervalSince1970
        )) ?? .failed
        try check(status, "publishing a position")
    }

    /// feedHeading publishes one orientation reading.
    ///
    /// A heading the device could not measure is passed as `nil` and publishes
    /// nothing: zero degrees is due north, and the client cannot tell the two apart.
    /// The true heading wins when it is known, because the client holds one heading
    /// and the true one is the one a person reads off a map.
    public static func feedHeading(
        magnetic: Double?,
        trueHeading: Double?,
        yaw: Double,
        pitch: Double,
        roll: Double,
        isBackAxis: Bool
    ) throws {
        let status = Status(rawValue: GonomadnetFeedHeading(
            magnetic ?? .nan,
            trueHeading ?? .nan,
            yaw,
            pitch,
            roll,
            isBackAxis ? 1 : 0
        )) ?? .failed
        try check(status, "publishing a heading")
    }

    /// check turns a status into a thrown error, reading the reason the Go half left.
    private static func check(_ status: Status, _ what: String) throws {
        if status.isSuccess { return }

        var reason = ""
        if let raw = GonomadnetLastError() {
            defer { GonomadnetFreeError(raw) }
            reason = String(cString: raw)
        }
        if reason.isEmpty {
            reason = "\(what) failed with status \(status.rawValue)"
        }
        throw ApplianceError(status: status, reason: reason)
    }
}
