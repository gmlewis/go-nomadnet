// swift-tools-version: 5.9

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

import PackageDescription

// The appliance's two Swift libraries.
//
// They are declared for macOS as well as iOS on purpose: every part of the
// appliance that can be tested without a device is tested on the machine that
// builds it, and `swift test` is the only gate that has to pass for a change to
// be believed. Only the views in ios/App need a device or a simulator, and they
// are not in this package.
//
// Nothing here depends on a third-party package. The frameworks the appliance
// uses — Foundation, CoreLocation, CoreMotion, CoreBluetooth, SwiftUI — are all
// part of the SDK, and the terminal engine, the key encodings and the colour
// resolution are the appliance's own code, so the wire format it speaks to the
// Go bridge is one this package can assert byte for byte.
let package = Package(
    name: "GonomadnetAppliance",
    platforms: [
        .macOS(.v13),
        .iOS(.v16),
    ],
    products: [
        .library(name: "TerminalKit", targets: ["TerminalKit"]),
        .library(name: "ApplianceKit", targets: ["ApplianceKit"]),
    ],
    targets: [
        // The terminal the NomadNet client draws into: the frame codec it shares
        // with the Go bridge, the escape-sequence parser, the screen model, the
        // palette, the layout maths, the key and mouse encodings, and the touch
        // handling that turns a finger into them.
        .target(name: "TerminalKit"),

        // Everything the appliance decides rather than renders: the
        // configuration files it renders for Reticulum and the client, the
        // channels it seeds, the settings it persists, and the sensor
        // translations that turn a device attitude into a compass heading.
        .target(name: "ApplianceKit"),

        .testTarget(name: "TerminalKitTests", dependencies: ["TerminalKit"]),
        .testTarget(name: "ApplianceKitTests", dependencies: ["ApplianceKit"]),
    ]
)
