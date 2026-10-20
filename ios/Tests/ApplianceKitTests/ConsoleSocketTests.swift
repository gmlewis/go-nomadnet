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

/// The tests for where the console's socket is bound.
///
/// A data container as iOS names one, with the identifier every install gets. Nothing
/// here reads the file system: the socket's path is a function of the directory the
/// appliance is handed, which is what makes an iOS-only mistake testable on a Mac.
private let container = URL(fileURLWithPath:
    "/var/mobile/Containers/Data/Application/5B1D0A9C-7E4F-4A2B-9C3D-0E1F2A3B4C5D")

final class ConsoleSocketTests: XCTestCase {
    /// The socket in an app's own container has to fit the kernel's `sun_path`.
    ///
    /// The container's identifier spends most of the 103 bytes, so the socket goes in the
    /// app's temporary directory, which is the shortest one an iOS app has.
    func testTheConsoleSocketInAnAppsContainerFitsTheKernelsLimit() {
        let path = ConsoleSocket.path(temporaryDirectory: container.appendingPathComponent("tmp"))

        XCTAssertTrue(path.hasSuffix("/gm.sock"), "the socket's name is its own, got \(path)")
        XCTAssertLessThanOrEqual(path.utf8.count, ConsoleSocket.maxPath,
                                "the kernel would refuse \(path), which is \(path.utf8.count) bytes")
    }

    /// The configuration directory is the one that does not fit, which is why the socket is
    /// not there.
    func testTheDirectoryTheConfigurationLivesInIsTooLongForTheSocket() {
        let beside = container
            .appendingPathComponent("Library/Application Support/gonomadnet")
            .appendingPathComponent(ConsoleSocket.name)
            .path

        XCTAssertGreaterThan(beside.utf8.count, ConsoleSocket.maxPath,
                             "\(beside) is \(beside.utf8.count) bytes and was expected to be too long")
    }

    /// The appliance is handed the temporary directory rather than choosing one, so the
    /// path it produces is the directory it was given and nothing else.
    func testTheSocketPathIsTheDirectoryItWasGiven() {
        for directory in ["/tmp", "/var/mobile/Containers/Data/Application/abc/tmp"] {
            let base = URL(fileURLWithPath: directory)
            XCTAssertEqual(ConsoleSocket.path(temporaryDirectory: base),
                           base.appendingPathComponent(ConsoleSocket.name).path)
        }
    }
}
