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
// You should have received a copy of the GNU General Public License along
// with this program. If not, see <https://www.gnu.org/licenses/>.

import Foundation

/// Turns a hub's host into an address the transport can actually dial.
///
/// Reticulum's configuration on this appliance carries addresses and never names, because
/// the half that reads it cannot resolve one: a name written into the file is dialled as a
/// name by a Go binary with no resolver behind it, and the result is an interface that
/// reports itself disconnected forever with no reason anywhere to find. So the appliance
/// resolves the name *here*, while the name is still a name, and states the address.
///
/// This is the same step the Android appliance takes in Java, and it is here for the same
/// reason: the platform's own resolver is available to the app and not to the daemon.
///
/// The lookup is injected so the whole policy — a literal is passed through, an IPv6
/// literal is unwrapped, the first usable address wins, a name that resolves to nothing is
/// said to have resolved to nothing — is a unit test with no network in it.
public enum HubResolver {
    /// A host lookup, as the system's resolver is.
    ///
    /// It returns the addresses a host resolves to, in the order the resolver gave them, and
    /// an empty list for a name that resolved to nothing.
    public typealias Lookup = (String) -> [String]

    /// resolve returns the address to write for `host`, or nil when there is none.
    ///
    /// A literal address is its own answer and asks nobody: it is what a reader who has been
    /// burned by a name will type, and a device with no DNS at all is one where it is the
    /// only thing that can work.
    public static func resolve(_ host: String, lookup: Lookup = systemLookup) -> String? {
        let trimmed = host.trimmingCharacters(in: .whitespaces)
        if trimmed.isEmpty {
            return nil
        }
        if let literal = NodeConfigRenderer.literalAddress(trimmed) {
            return literal
        }
        // A resolver that answers with another name has not answered: the renderer refuses
        // names, and a second name would be refused in its turn.
        for address in lookup(trimmed) {
            if let literal = NodeConfigRenderer.literalAddress(address) {
                return literal
            }
        }
        return nil
    }

    /// systemLookup asks the system's own resolver.
    ///
    /// It is the one part of this that cannot be tested without a network, which is why it is
    /// the only part: everything that decides what to do with an answer is above it, and the
    /// answers it gives are addresses, numeric and unambiguous.
    public static func systemLookup(_ host: String) -> [String] {
        var hints = addrinfo()
        hints.ai_family = AF_UNSPEC
        hints.ai_socktype = SOCK_STREAM
        hints.ai_flags = AI_ADDRCONFIG

        var result: UnsafeMutablePointer<addrinfo>?
        guard getaddrinfo(host, nil, &hints, &result) == 0, let first = result else {
            return []
        }
        defer { freeaddrinfo(first) }

        var addresses: [String] = []
        var node: UnsafeMutablePointer<addrinfo>? = first
        while let current = node {
            if let socketAddress = current.pointee.ai_addr {
                var buffer = [CChar](repeating: 0, count: Int(NI_MAXHOST))
                let length = socklen_t(buffer.count)
                if getnameinfo(socketAddress, current.pointee.ai_addrlen,
                               &buffer, length, nil, 0, NI_NUMERICHOST) == 0 {
                    addresses.append(String(cString: buffer))
                }
            }
            node = current.pointee.ai_next
        }
        return addresses
    }
}
