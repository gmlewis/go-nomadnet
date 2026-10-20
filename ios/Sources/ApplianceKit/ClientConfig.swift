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

/// A template the appliance cannot start from, and why.
public enum ClientConfigError: Error, Equatable {
    /// A section did not carry the keys the appliance has to set.
    ///
    /// It names the missing keys and the section they were looked for in, so the message
    /// can tell an operator which part of the client's default moved.
    case missingKeys([String], section: String)
}

extension ClientConfigError: LocalizedError {
    /// The operator-facing explanation of a refused template.
    public var errorDescription: String? {
        switch self {
        case let .missingKeys(keys, section):
            return "the client's default configuration states no \(keys.joined(separator: ", ")) "
                + "in [\(section)], so the appliance cannot set it"
        }
    }
}

/// The settings the appliance states in the client's own configuration.
///
/// The client is a program with a configuration file of its own, written from the Go
/// project's default on its first run, and that default describes a desktop: a client that
/// serves no pages, a node with no name, and an editor that has to be installed. An
/// appliance is none of those things, so the appliance states the three values before the
/// client has ever run:
///
///   - `enable_node = yes`, because the appliance is a node and its console otherwise
///     answers "This instance is not hosting a node";
///   - `node_name = gonomadnet on iOS`, so that its node can be told apart from every
///     desktop node it appears beside;
///   - `editor = `, empty, because there is no editor program to name. iOS forbids an
///     application from starting a program at all, so anything written here would be a
///     command that can never run, and the client's Config page would fail with a reason
///     that blames a program instead of naming the one that does the job. The appliance's
///     editor is a screen in the app, and the client is told so rather than pointed at a
///     program.
///
/// The file is written from the client's own default rather than from a second copy of it:
/// the template is the `default.conf` the Go client embeds and writes on its first run,
/// taken from the module that owns it. A configuration written from it is therefore
/// complete — an operator who opens the editor sees the documented file they would have
/// got from the client — with three values changed.
public enum ClientConfig {
    /// The value the client's `editor` setting is given, which is nothing.
    ///
    /// An empty editor is the honest answer on a platform that has no editor programs:
    /// the field asks which program to start, and the appliance's answer is that there is
    /// no program — the editing happens in the app.
    public static let editorSetting = ""

    /// The name the appliance's node announces and shows to peers.
    ///
    /// It says which appliance this is rather than what the node is for: the device's node
    /// appears in other people's node lists beside desktop nodes, and "gonomadnet on iOS"
    /// is the one name that tells them apart without a lookup.
    public static let nodeName = "gonomadnet on iOS"

    /// The values of `editor` that name no editor this device will ever have: the client's
    /// own default, the alias Python resolves to a program on the machine, and the empty
    /// value the appliance itself writes.
    ///
    /// A configuration that still says one of these is one nobody has chosen an editor in,
    /// which is what makes it safe to state the appliance's own value over it.
    public static let replaceableEditors = ["nano", "editor", editorSetting]

    /// `template` with the appliance's node settings and its editor.
    ///
    /// Every other line is the client's own, comments included, so the file an operator
    /// reads is the file the client would have written with three values different. Only
    /// the two sections the keys live in are looked at: `enable_node` is not a key in any
    /// other section, and a rewrite that scanned the whole document would change a value
    /// it had not understood the first time another section grew a key of the same name.
    ///
    /// - Throws: `ClientConfigError.missingKeys` when a template no longer carries the
    ///   keys. Refusing is deliberate: the alternative is a file that looks correct and
    ///   hosts nothing.
    public static func settings(template: String, editor: String) throws -> String {
        let served = try rewrite(
            template,
            section: nodeSection,
            requiredKeys: [enableNodeKey, nodeNameKey]
        ) { key, _ in
            switch key {
            case enableNodeKey: return "yes"
            case nodeNameKey: return nodeName
            default: return nil
            }
        }
        return try rewrite(
            served,
            section: textUISection,
            requiredKeys: [editorKey]
        ) { key, _ in
            key == editorKey ? editor : nil
        }
    }

    /// The appliance's own values stated over a file that already exists.
    ///
    /// This is what the appliance writes on every start, and what the in-app editor
    /// saves: the file an operator has been editing is the file the appliance states its
    /// three values in, so an edit anywhere else in it survives and the appliance still
    /// hosts a named node with an editor it can name. Starting from the shipped default
    /// every time would silently discard everything an operator wrote.
    ///
    /// - Throws: `ClientConfigError.missingKeys` when the file no longer carries the
    ///   keys the appliance has to state. An edited file that dropped one is refused with
    ///   its section named, rather than saved into a configuration that hosts nothing.
    public static func applied(to text: String) throws -> String {
        try settings(template: text, editor: editorSetting)
    }

    /// Replaces the values of the keys of one section, and refuses the result if a key of
    /// `requiredKeys` was not in it.
    ///
    /// `value` is given each key of the section and its current value, and returns the
    /// value to write or `nil` to leave the line alone, which is how a rule that applies to
    /// some of a section's keys is expressed. The key and its spacing are the file's: only
    /// the value is written, so a template that indents or spaces its lines is not
    /// reformatted.
    private static func rewrite(
        _ text: String,
        section: String,
        requiredKeys: Set<String>,
        value: (String, String) -> String?
    ) throws -> String {
        var lines = text.components(separatedBy: "\n")
        var inSection = false
        var found: Set<String> = []
        for (index, line) in lines.enumerated() {
            let trimmed = line.trimmingCharacters(in: .whitespaces)
            if trimmed.hasPrefix("["), trimmed.hasSuffix("]") {
                let name = trimmed.dropFirst().dropLast().trimmingCharacters(in: .whitespaces)
                inSection = name == section
                continue
            }
            if !inSection || trimmed.hasPrefix("#") || !trimmed.contains("=") {
                continue
            }
            guard let equals = line.firstIndex(of: "=") else { continue }
            let key = line[..<equals].trimmingCharacters(in: .whitespaces)
            let current = line[line.index(after: equals)...].trimmingCharacters(in: .whitespaces)
            guard let replacement = value(String(key), current) else { continue }
            lines[index] = String(line[..<equals]) + "= " + replacement
            found.insert(String(key))
        }
        let missing = requiredKeys.subtracting(found).sorted()
        if !missing.isEmpty {
            throw ClientConfigError.missingKeys(missing, section: section)
        }
        return lines.joined(separator: "\n")
    }

    /// The section the node's settings live in.
    private static let nodeSection = "node"
    /// The key that turns page serving on.
    private static let enableNodeKey = "enable_node"
    /// The key that names the node in announces.
    private static let nodeNameKey = "node_name"
    /// The section the editor setting lives in.
    private static let textUISection = "textui"
    /// The key naming the editor the Config page opens.
    private static let editorKey = "editor"
}
