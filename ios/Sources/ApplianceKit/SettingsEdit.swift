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

/// What the appliance writes for a given set of settings.
///
/// The appliance renders both Reticulum configurations from its settings, and it
/// re-renders them every time the stack starts — so an edit to the hub is not an edit
/// to a file, it is an edit to the values the file is rendered from. This is that
/// rendering, in one place, so that the screen a reader types into and the stack that
/// starts the transport cannot disagree about what an edit means, and so that "the edit
/// was accepted" and "the rendered config names the new host" are the same question
/// asked twice.
///
/// A hub named by a host name is resolved here, before the renderer sees it: the renderer
/// refuses names, and the daemon that reads its output has no resolver. Everything else is
/// the renderer's own validation, so a configuration this refuses is one the stack would
/// refuse, and the reader is told before the appliance is taken down rather than after.
public enum SettingsEdit {
    /// The two Reticulum configurations an edited hub produces, and what became of the hub.
    public struct Rendered: Equatable, Sendable {
        /// The transport's own configuration: the file the transport is started with.
        public var transport: String
        /// The configuration the hub and the bot attach through.
        public var client: String
        /// The address the hub resolved to, when it resolved to one.
        ///
        /// It is what the reader is shown, because a hub set to a name is a hub whose
        /// address the appliance chose: showing the name back would leave the one fact the
        /// resolution exists for invisible.
        public var hubAddress: String?
        /// Why no hub interface was rendered, when the hub did not resolve.
        ///
        /// It is not an error. An appliance whose hub resolved to nothing still runs, still
        /// serves its own pages and still dials a radio, and saying so is what stops a
        /// reader looking for a transport that is up with nothing to connect through.
        public var hubProblem: String?
    }

    /// The two Reticulum configurations a hub produces.
    ///
    /// The transport owns the shared instance and the client attaches to it, so both
    /// files name the same hub interface and only the first enables the transport. The hub's
    /// host is resolved before anything is rendered: the renderer refuses a name, and the
    /// half that reads the file cannot resolve one.
    ///
    /// - Parameters:
    ///   - hubHost: the hub's address, or a name for the appliance to resolve.
    ///   - hubPort: the hub's port; zero or less dials no hub at all.
    ///   - rpcKey: the key the transport's RPC listener authenticates its clients with.
    ///   - rnode: the radio the transport dials, or `nil` for none.
    ///   - lookup: how a name is resolved, injected so this needs no network to test.
    ///
    /// - Throws: `NodeConfigError.nonLiteralAddress` when a name reaches the renderer, or
    ///   whatever else the renderer refuses.
    public static func render(
        hubHost: String,
        hubPort: Int,
        rpcKey: String,
        rnode: RNodeSpec? = nil,
        sharedInstancePort: Int = NodeConfigRenderer.sharedInstancePort,
        instanceName: String = NodeConfigRenderer.instanceName,
        lookup: HubResolver.Lookup = HubResolver.systemLookup
    ) throws -> Rendered {
        // The hub is resolved before anything is rendered. A name that resolves to nothing
        // is not a failure of the appliance: it is an appliance with no hub interface, which
        // is what the Android appliance renders in the same case, and the reason travels back
        // to whoever asked so it can be said out loud rather than looked for.
        var interfaces: [InterfaceSpec] = []
        var hubAddress: String?
        var hubProblem: String?
        if hubPort > 0 {
            if let address = HubResolver.resolve(hubHost, lookup: lookup) {
                hubAddress = address
                interfaces = [InterfaceSpec(
                    name: NodeConfigRenderer.hubInterfaceName,
                    host: address,
                    port: hubPort
                )]
            } else {
                hubProblem = "the hub \(hubHost) did not resolve to an address, so the "
                    + "transport is rendered with no hub interface"
            }
        }

        let spec = NodeConfigSpec(
            sharedInstancePort: sharedInstancePort,
            instanceName: instanceName,
            rpcKey: rpcKey,
            interfaces: interfaces,
            rnode: rnode
        )
        return Rendered(
            transport: try NodeConfigRenderer.reticulumConfig(spec),
            client: try NodeConfigRenderer.clientReticulumConfig(spec),
            hubAddress: hubAddress,
            hubProblem: hubProblem
        )
    }
}
