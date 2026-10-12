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

/// ConsoleSocket is where the appliance binds the console's socket.
///
/// The appliance and the app are one process, so the socket is not a way to reach
/// anything: it is the console's connection, and the app dials the path the appliance
/// bound. That makes the path a decision rather than a detail, because a Unix socket path
/// has a hard limit and an iOS app's directories are long.
///
/// It is deliberately not with the rest of the appliance's files. `AppliancePaths`
/// puts everything under `Library/Application Support/gonomadnet`, which on a device
/// runs to 113 bytes before a name is added — the kernel binds 103 — so a console kept
/// there could never open at all: the bridge refuses the path, no socket is bound, and
/// the interface reports only that no console is open. The app's own temporary directory
/// is the shortest one an iOS app has, and the socket is not a file anybody keeps, so
/// nothing is lost by putting it there.
public enum ConsoleSocket {
    /// maxPath is the longest socket path the kernel will bind: `sun_path` holds 104
    /// bytes including the NUL that ends it.
    public static let maxPath = 103

    /// name is the socket's own name, at the end of the path. It is short because on iOS
    /// everything before it is spent on the container and on `tmp/`.
    public static let name = "gm.sock"

    /// path is where the console's socket is bound, given the temporary directory of the
    /// app it belongs to.
    ///
    /// The caller checks the result against `maxPath` rather than this function doing it:
    /// a path that does not fit is a failure the reader has to be told about, and a
    /// function that quietly returned a shorter one would leave the app dialling a path
    /// the appliance never bound.
    public static func path(temporaryDirectory: URL) -> String {
        temporaryDirectory.appendingPathComponent(name).path
    }
}
