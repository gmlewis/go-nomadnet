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

/// One Reticulum interface the appliance's own transport should dial.
///
/// `host` must be a literal address, never a name. A `CGO_ENABLED=0` Go binary has no
/// resolver of its own, and on an appliance there may be no `/etc/resolv.conf` to fall
/// back to, so a name in this configuration is a value that looks fine and never
/// connects:
///
/// ```
/// dial tcp: lookup <host> on [::1]:53: read udp ...: read: connection refused
/// ```
///
/// The platform *can* resolve, so the appliance resolves every name it is given and
/// writes the literal address here before spawning the transport. `NodeConfigRenderer`
/// refuses to render a host name at all, so a resolver that silently failed cannot
/// produce a configuration that looks correct and never connects.
public struct InterfaceSpec: Equatable, Sendable {
    /// The interface's name, which shows up in Reticulum's own diagnostics.
    public var name: String
    /// The literal address to dial. A name is refused by the renderer.
    public var host: String
    /// The TCP port to dial.
    public var port: Int
    /// Whether the transport dials this interface at startup.
    public var enabled: Bool

    /// Creates one interface specification.
    public init(name: String, host: String, port: Int, enabled: Bool = true) {
        self.name = name
        self.host = host
        self.port = port
        self.enabled = enabled
    }
}

/// The RNode radio the appliance's transport dials.
///
/// It is **enabled by default, whether or not a radio is attached**, and that is
/// deliberate. The appliance is a radio node for everybody who installs it, so it
/// assumes a radio might be there: the interface is rendered switched on from the first
/// start, before any radio has ever been plugged in, so that the simplest possible
/// installation — install the app, tap Start stack, plug a radio in — needs nothing
/// configured by hand.
///
/// A radio that is not there costs nothing and is not an error. The transport logs the
/// interface it cannot open and retries it in the background, the Interfaces page shows
/// it as Disconnected with no traffic, and a radio plugged in later is picked up with
/// nobody touching a file. That steady state is expected and correct.
///
/// `port` is the path the transport opens. It defaults to the serial device a radio on
/// the device's USB port would enumerate as. When the appliance's own bridge has a radio
/// open, the bridge's published path is used instead, which is the slave of a
/// pseudo-terminal and the one serial device a sandboxed application can allocate. That
/// substitution is the whole of how a radio goes from Disconnected to Connected; without
/// it, the default stands and the interface waits.
public struct RNodeSpec: Equatable, Sendable {
    /// The serial path the transport opens.
    public var port: String
    /// The section the radio is rendered under in `[interfaces]`.
    public var name: String
    /// The center frequency in hertz: 915 MHz, the band an RNode is set up in.
    public var frequency: Int
    /// The channel bandwidth in hertz.
    public var bandwidth: Int
    /// The transmit power in dBm.
    public var txpower: Int
    /// The LoRa spreading factor.
    public var spreadingFactor: Int
    /// The LoRa coding rate denominator.
    public var codingRate: Int
    /// Whether the transport dials the radio at startup.
    public var enabled: Bool

    /// The section the radio is rendered under in `[interfaces]`.
    public static let defaultName = "RNode LoRa"
    /// The serial device a radio on the device's USB port enumerates as.
    public static let defaultPort = "/dev/ttyACM0"
    /// 915 MHz, at 125 kHz of bandwidth: the narrow band an RNode is set up in.
    public static let defaultFrequency = 915_000_000
    /// 125 kHz of channel bandwidth.
    public static let defaultBandwidth = 125_000
    /// 17 dBm of transmit power.
    public static let defaultTxPower = 17
    /// Spreading factor 9.
    public static let defaultSpreadingFactor = 9
    /// Coding rate 5.
    public static let defaultCodingRate = 5

    /// Creates one radio specification, defaulting to the shipped LoRa parameters.
    public init(
        port: String = RNodeSpec.defaultPort,
        name: String = RNodeSpec.defaultName,
        frequency: Int = RNodeSpec.defaultFrequency,
        bandwidth: Int = RNodeSpec.defaultBandwidth,
        txpower: Int = RNodeSpec.defaultTxPower,
        spreadingFactor: Int = RNodeSpec.defaultSpreadingFactor,
        codingRate: Int = RNodeSpec.defaultCodingRate,
        enabled: Bool = true
    ) {
        self.port = port
        self.name = name
        self.frequency = frequency
        self.bandwidth = bandwidth
        self.txpower = txpower
        self.spreadingFactor = spreadingFactor
        self.codingRate = codingRate
        self.enabled = enabled
    }
}

/// The transport's configuration, as a value the renderer turns into INI.
///
/// `sharedInstancePort` and `instanceName` are the contract between the appliance and
/// anything that attaches to its transport, so they are part of the rendered
/// configuration rather than a hidden default: the client half must name exactly the
/// same pair, and a silent mismatch produces "no shared instance is running" and
/// nothing else.
public struct NodeConfigSpec: Equatable, Sendable {
    /// The loopback port the appliance's transport listens on for local clients.
    public var sharedInstancePort: Int
    /// The shared instance's name; both sides must agree on it.
    public var instanceName: String
    /// The key the transport's RPC listener authenticates its clients with, as hex.
    ///
    /// It is stated by the appliance rather than derived from an identity, because the
    /// two sides run from different directories and would derive different keys from
    /// them. See `ApplianceSettings.rpcKey`.
    public var rpcKey: String
    /// The hub interfaces the transport dials, already resolved to literal addresses.
    public var interfaces: [InterfaceSpec]
    /// The radio the transport dials, or `nil` for one that is to have none.
    public var rnode: RNodeSpec?
    /// Reticulum's own log level, rendered into `[logging]`.
    public var logLevel: Int
    /// Whether this process owns the transport (`yes`) or attaches to one (`no`).
    public var enableTransport: Bool

    /// The loopback port the appliance's transport listens on for local clients.
    public static let defaultSharedInstancePort = 37428
    /// The shared instance's name; both sides must agree on it.
    public static let defaultInstanceName = "default"

    /// Creates one transport configuration, defaulting to the shipped appliance values.
    public init(
        sharedInstancePort: Int = NodeConfigSpec.defaultSharedInstancePort,
        instanceName: String = NodeConfigSpec.defaultInstanceName,
        rpcKey: String = "",
        interfaces: [InterfaceSpec] = [],
        rnode: RNodeSpec? = RNodeSpec(),
        logLevel: Int = 4,
        enableTransport: Bool = true
    ) {
        self.sharedInstancePort = sharedInstancePort
        self.instanceName = instanceName
        self.rpcKey = rpcKey
        self.interfaces = interfaces
        self.rnode = rnode
        self.logLevel = logLevel
        self.enableTransport = enableTransport
    }
}

/// The bot's configuration, as a value the renderer turns into TOML.
///
/// `hubDestination` is the `rrc.hub` destination hash of the local hub, which the
/// appliance derives from the hub's own identity at start time rather than shipping a
/// name that would have to be resolved.
///
/// `kjvTxtFile` is where the Bible text the `kjv` command reads was installed. An empty
/// value leaves the key out entirely, which is the bot's own way of saying the command
/// is not configured; naming a path that is not there would instead make the bot report
/// the file as unreadable for the life of the install.
public struct BotConfigSpec: Equatable, Sendable {
    /// The FIFO the bot reads GNSS samples from, as a path.
    public var gpsPort: String
    /// The FIFO the bot reads compass samples from, as a path.
    public var compassPort: String
    /// The installed Bible text, or an empty string to disable the command.
    public var kjvTxtFile: String
    /// The name the bot lists the local hub under.
    public var hubName: String
    /// The local hub's `rrc.hub` destination hash, as hex.
    public var hubDestination: String
    /// The rooms the bot joins on the local hub.
    public var rooms: [String]
    /// The nick the bot announces itself under.
    public var nick: String
    /// The nick prefix the bot answers to in a room.
    public var respondTo: String

    /// Creates one bot configuration.
    public init(
        gpsPort: String,
        compassPort: String,
        kjvTxtFile: String = "",
        hubName: String = "gonomadnet local hub",
        hubDestination: String,
        rooms: [String] = ["general"],
        nick: String = "gobot",
        respondTo: String = "gobot"
    ) {
        self.gpsPort = gpsPort
        self.compassPort = compassPort
        self.kjvTxtFile = kjvTxtFile
        self.hubName = hubName
        self.hubDestination = hubDestination
        self.rooms = rooms
        self.nick = nick
        self.respondTo = respondTo
    }
}

/// A configuration the appliance cannot render, and why.
public enum NodeConfigError: Error, Equatable {
    /// An interface named a host instead of a literal address.
    ///
    /// It carries both the interface's name and the offending host so the message can
    /// tell an operator which line to fix.
    case nonLiteralAddress(name: String, host: String)
}

extension NodeConfigError: LocalizedError {
    /// The operator-facing explanation of a refused configuration.
    public var errorDescription: String? {
        switch self {
        case let .nonLiteralAddress(name, host):
            return "interface \(name) names the host \(host), which is not a literal "
                + "address; resolve it before rendering and pass the address, because a "
                + "Go binary on this platform cannot resolve names"
        }
    }
}

/// Renders the configuration files the appliance's daemons are started with.
///
/// The whole surface is static and pure: given a spec, each function returns the exact
/// bytes a configuration file must contain. That is what lets the contract between the
/// appliance and Reticulum be asserted in a unit test with no daemon and no device.
public enum NodeConfigRenderer {
    /// The loopback port the appliance's transport listens on for local clients.
    ///
    /// Termux's attached mode and the client half must name the same port, so it is
    /// pinned here rather than left to a default.
    public static let sharedInstancePort = NodeConfigSpec.defaultSharedInstancePort
    /// The shared instance's name; both sides must agree on it.
    public static let instanceName = NodeConfigSpec.defaultInstanceName
    /// The loopback port the sensor feed is published on.
    ///
    /// It must not collide with Reticulum's own loopback ports, so it is stated beside
    /// them: the shared instance takes 37428 and Reticulum's control channel 37429.
    public static let feedPort = 37430
    /// The interface's name, which shows up in Reticulum's own diagnostics.
    public static let hubInterfaceName = "Home Hub"
    /// The destination name hubs announce under; the client's own default.
    public static let destName = DefaultHubs.destName
    /// A hub destination's length in hex: sixteen bytes.
    public static let hubHashHexLength = DefaultHubs.hubHashHexLength

    /// Renders the transport's configuration.
    ///
    /// `shared_instance_type = tcp` is not a preference: on Linux the default shared
    /// instance is an abstract Unix socket, and two different applications cannot be
    /// assumed to reach each other's abstract sockets because their security categories
    /// differ. A loopback TCP socket needs nothing but the loopback interface, which
    /// removes the question.
    ///
    /// **Nothing in this file is quoted.** Reticulum's configuration is INI-like and its
    /// parser keeps quotation marks as part of the value, so a quoted address is dialled
    /// as the *name* `"203.0.113.7"` — with the quotation marks in it — giving:
    ///
    /// ```
    /// Go TCPClientInterface "Home Hub" connect failed: dial tcp: lookup "203.0.113.7": no such host
    /// ```
    ///
    /// A name with spaces is written bare, as the shipped configuration does, and a
    /// value that genuinely needs a comma or a bracket is not representable here at all —
    /// which is a reason to reject it rather than to quote it.
    ///
    /// - Throws: `NodeConfigError.nonLiteralAddress` when an interface names a host.
    public static func reticulumConfig(_ spec: NodeConfigSpec) throws -> String {
        var out = "# Rendered by the gonomadnet appliance. Do not edit by hand.\n"
        out += "[reticulum]\n"
        out += "  enable_transport = \(spec.enableTransport ? "yes" : "no")\n"
        out += "  share_instance = yes\n"
        out += "  shared_instance_type = tcp\n"
        out += "  shared_instance_port = \(spec.sharedInstancePort)\n"
        out += "  instance_name = \(spec.instanceName)\n"
        out += "  # The appliance owns the shared instance, so it must never be the\n"
        out += "  # one that gives up quietly when something else already holds it.\n"
        out += "  panic_on_interface_error = no\n"
        out += rpcKeyLine(spec)
        out += "\n"
        out += "[logging]\n"
        out += "  loglevel = \(spec.logLevel)\n"
        out += "\n"
        out += try renderInterfaces(spec)
        return out
    }

    /// Renders the client-side configuration the hub and the bot attach through.
    ///
    /// It says `require_shared_instance = yes`, because a client that merely *prefers*
    /// the shared instance tries to become the owner first and only falls back to
    /// attaching when the bind fails. Two clients racing for ownership of the transport's
    /// shared instance is a bug that shows up as one of them mysteriously being the
    /// transport, so the race is removed rather than made unlikely.
    ///
    /// It lists the same interfaces the transport does, and they are the same list for
    /// the same reason: they are what this appliance connects through. The client owns
    /// none of them — the shared instance does — but the client's Interfaces page is a
    /// view of this file, and a file that said "deliberately empty" showed an operator an
    /// appliance with no interfaces at all while its transport was dialling the hub.
    ///
    /// - Throws: `NodeConfigError.nonLiteralAddress` when an interface names a host.
    public static func clientReticulumConfig(_ spec: NodeConfigSpec) throws -> String {
        var out = "# Rendered by the gonomadnet appliance. Do not edit by hand.\n"
        out += "# A client of the appliance's own transport: it owns no interface and it\n"
        out += "# must never take the shared instance, only attach to it.\n"
        out += "[reticulum]\n"
        out += "  enable_transport = no\n"
        out += "  share_instance = yes\n"
        out += "  shared_instance_type = tcp\n"
        out += "  shared_instance_port = \(spec.sharedInstancePort)\n"
        out += "  instance_name = \(spec.instanceName)\n"
        out += "  require_shared_instance = yes\n"
        out += rpcKeyLine(spec)
        out += "\n"
        out += "[logging]\n"
        out += "  loglevel = \(spec.logLevel)\n"
        out += "\n"
        out += try renderInterfaces(spec)
        return out
    }

    /// Renders the bot's configuration.
    ///
    /// The two sensor ports point at FIFOs under the appliance's private directory. A
    /// bot that reads a FIFO has to find its writer already holding the other end, which
    /// the sensor service arranges before anything is spawned; opening the read end of a
    /// FIFO with no writer blocks forever, with no error message at all.
    public static func gorrcbotConfig(_ spec: BotConfigSpec) -> String {
        var out = "# Rendered by the gonomadnet appliance. Do not edit by hand.\n"
        out += "[bot]\n"
        out += "  nick = \(escapeTomlString(spec.nick))\n"
        out += "\n"
        out += "# The live sensor streams the appliance publishes. They are named\n"
        out += "# exactly as the Nomad Network client's [location] section names them.\n"
        out += "gps_port = \(escapeTomlString(spec.gpsPort))\n"
        out += "compass_port = \(escapeTomlString(spec.compassPort))\n"
        if !spec.kjvTxtFile.isEmpty {
            out += "\n"
            out += "# The Bible text the kjv command reads, installed from the bundle's\n"
            out += "# assets on every start because it is far too large to live in a\n"
            out += "# configuration file.\n"
            out += "kjv_txt_file = \(escapeTomlString(spec.kjvTxtFile))\n"
        }
        out += "\n"
        out += "[[hubs]]\n"
        out += "name = \(escapeTomlString(spec.hubName))\n"
        out += "destination = \(escapeTomlString(spec.hubDestination))\n"
        out += "rooms = [\(spec.rooms.map(escapeTomlString).joined(separator: ", "))]\n"
        out += "nick = \(escapeTomlString(spec.nick))\n"
        let respond = spec.rooms
            .map { "\(escapeTomlString($0)) = \(escapeTomlString(spec.respondTo))" }
            .joined(separator: ", ")
        out += "respond_to = { \(respond) }\n"
        return out
    }

    /// Returns the address when it is already a literal, and `nil` when it is a name.
    ///
    /// IPv4 is four dotted decimal octets; IPv6 is anything with at least two colons in
    /// it that is not an IPv4 address, which is generous and deliberate — the point is
    /// to refuse names, and a malformed IPv6 literal is refused by the dialer with a
    /// message of its own.
    public static func literalAddress(_ host: String) -> String? {
        let trimmed = host.trimmingCharacters(in: .whitespaces)
        if trimmed.isEmpty {
            return nil
        }
        if trimmed.filter({ $0 == ":" }).count >= 2 {
            return trimmed.trimmingCharacters(in: CharacterSet(charactersIn: "[]"))
        }
        let octets = trimmed.split(separator: ".", omittingEmptySubsequences: false)
        if octets.count != 4 {
            return nil
        }
        for octet in octets {
            if octet.isEmpty || octet.count > 3 || !octet.allSatisfy(\.isNumber) {
                return nil
            }
            if (Int(octet) ?? 256) > 255 {
                return nil
            }
        }
        return trimmed
    }

    /// Quotes a TOML string, which is what every value in the bot's file is.
    public static func escapeTomlString(_ value: String) -> String {
        let escaped = value
            .replacingOccurrences(of: "\\", with: "\\\\")
            .replacingOccurrences(of: "\"", with: "\\\"")
        return "\"\(escaped)\""
    }

    /// Renders the transport's RPC key, or nothing when the appliance has none.
    ///
    /// Both configuration files carry the same key: the instance's listener
    /// authenticates against it, and every client presents it. Without it the two would
    /// each derive a key from the identity in their own storage directory, which are
    /// different identities here (see `ApplianceSettings.rpcKey`), and every RPC would be
    /// refused.
    private static func rpcKeyLine(_ spec: NodeConfigSpec) -> String {
        let key = spec.rpcKey.trimmingCharacters(in: .whitespacesAndNewlines)
        return key.isEmpty ? "" : "  rpc_key = \(key)\n"
    }

    /// Renders the `[interfaces]` section: the transport's own, and the same one again
    /// for a client.
    ///
    /// Listing them does not make the client dial them: an instance that attaches to a
    /// shared instance starts none of its own (an attached client's interfaces are owned
    /// by the instance it attached to), and the client's `require_shared_instance = yes`
    /// is what keeps it attaching rather than becoming one.
    private static func renderInterfaces(_ spec: NodeConfigSpec) throws -> String {
        var out = "[interfaces]\n"
        if spec.interfaces.isEmpty && spec.rnode == nil {
            out += "  # Deliberately empty: the appliance resolved no interface to "
                + "connect through.\n"
        } else if spec.interfaces.isEmpty {
            out += "  # No hub interface: the appliance resolved none to connect "
                + "through, so\n"
            out += "  # the radio below is the only one it dials.\n"
        }
        if let radio = spec.rnode {
            out += renderRadio(radio)
        }
        for interface in spec.interfaces {
            guard let host = literalAddress(interface.host) else {
                throw NodeConfigError.nonLiteralAddress(
                    name: interface.name, host: interface.host
                )
            }
            out += "  [[\(interface.name)]]\n"
            out += "    type = TCPClientInterface\n"
            out += "    interface_enabled = \(interface.enabled ? "true" : "false")\n"
            out += "    target_host = \(host)\n"
            out += "    target_port = \(interface.port)\n"
        }
        return out
    }

    /// Renders the radio's own section.
    ///
    /// It is an `RNodeInterface`, which opens `RNodeSpec.port` as a serial device and
    /// speaks the radio's own protocol over it: the port is where a radio is — the
    /// appliance's own bridge's published path when it has one open, and the kernel's
    /// serial device otherwise.
    ///
    /// It is rendered **switched on**, whatever `RNodeSpec.port` names and whether or
    /// not anything is at the other end of it. An appliance that hid its radio until one
    /// was plugged in would be an appliance that has to be configured before it can be
    /// used, and this one assumes a radio might be there.
    private static func renderRadio(_ radio: RNodeSpec) -> String {
        var out = "  # The radio on the device's own USB port. It is switched on whether "
        out += "or not a\n"
        out += "  # radio is attached: an interface the transport cannot open is retried "
        out += "in the\n"
        out += "  # background and reported as disconnected, so a radio plugged in later "
        out += "is\n"
        out += "  # picked up without anybody editing this file. When the appliance's own "
        out += "bridge\n"
        out += "  # has a radio open, the port below is the path that bridge published, "
        out += "which is\n"
        out += "  # the only serial device a sandboxed application can allocate; "
        out += "otherwise it is\n"
        out += "  # where the kernel would put one.\n"
        out += "  [[\(radio.name)]]\n"
        out += "    type = RNodeInterface\n"
        out += "    interface_enabled = \(radio.enabled ? "true" : "false")\n"
        out += "    port = \(radio.port)\n"
        out += "    frequency = \(radio.frequency)\n"
        out += "    bandwidth = \(radio.bandwidth)\n"
        out += "    txpower = \(radio.txpower)\n"
        out += "    spreadingfactor = \(radio.spreadingFactor)\n"
        out += "    codingrate = \(radio.codingRate)\n"
        return out
    }
}
