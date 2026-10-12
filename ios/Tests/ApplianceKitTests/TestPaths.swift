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

/// Locations the ApplianceKit tests need that are not inside this package.
///
/// The client's own default configuration and the appliance's channel-store fixtures live in
/// the same repository, and the tests read those files rather than copies kept beside them: a
/// fixture would be a second default that drifts from the first. They are located from the
/// test file's own path, so they resolve however the package is checked out.
enum TestPaths {
    /// The repository root, four directories above this file.
    ///
    /// `ios/Tests/ApplianceKitTests/TestPaths.swift` climbs to `ApplianceKitTests`, then
    /// `Tests`, then `ios`, then the root.
    static var repoRoot: URL {
        URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
    }

    /// A fresh, empty directory for one test.
    ///
    /// It is made under `/tmp` rather than through the platform's temporary directory because
    /// that path is short and predictable, which keeps anything the file system does with it
    /// from hitting a length limit, and it is removed when the test's process ends.
    static func makeTempDirectory() -> URL {
        let url = URL(fileURLWithPath: "/tmp")
            .appendingPathComponent("appliancekit-\(UUID().uuidString)")
        try? FileManager.default.createDirectory(at: url, withIntermediateDirectories: true)
        return url
    }
}
