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

/// The settings the appliance states in the client's own configuration.
///
/// These tests read the real `default.conf` out of the Go module rather than a fixture, so a
/// template that stops naming the keys fails here instead of on a device: the template is part
/// of the contract, and a check against a copy kept beside this file would be a check on the
/// copy.
final class ClientConfigTests: XCTestCase {
    /// The editor the appliance states, which is no program at all: iOS forbids an
    /// application from starting one, so the field is left empty rather than pointed at a
    /// command that can never run.
    private let editor = ClientConfig.editorSetting

    /// The client's default configuration, in the Go module that owns it.
    private var templateURL: URL {
        TestPaths.repoRoot
            .appendingPathComponent("nomadnet")
            .appendingPathComponent("config")
            .appendingPathComponent("testdata")
            .appendingPathComponent("default.conf")
    }

    /// The client's default configuration, as the app will carry it.
    private func template() throws -> String {
        try String(contentsOf: templateURL, encoding: .utf8)
    }

    func testTheTemplateTheApplianceReadsIsTheClientsOwnDefault() throws {
        XCTAssertTrue(
            FileManager.default.fileExists(atPath: templateURL.path),
            "the client's default configuration is not there: \(templateURL.path)"
        )
        let text = try template()
        XCTAssertTrue(text.contains("editor = nano"))
        XCTAssertTrue(text.contains("enable_node = no"))
        XCTAssertTrue(text.contains("node_name = None"))
    }

    func testTheClientsOwnDefaultIsMadeToHostANamedNodeWithAnEditor() throws {
        let patched = try ClientConfig.settings(template: try template(), editor: editor)

        XCTAssertTrue(patched.contains("enable_node = yes"), patched)
        XCTAssertTrue(patched.contains("node_name = \(ClientConfig.nodeName)"), patched)
        XCTAssertTrue(patched.contains("editor = \(editor)"), patched)
        for unset in ["enable_node = no", "node_name = None", "editor = nano"] {
            XCTAssertFalse(patched.contains(unset), "the default still says \"\(unset)\"")
        }
    }

    func testExactlyThreeLinesDifferAndTheRestIsByteIdentical() throws {
        let original = try template()
        let patched = try ClientConfig.settings(template: original, editor: editor)

        let before = original.components(separatedBy: "\n")
        let after = patched.components(separatedBy: "\n")
        XCTAssertEqual(
            before.count, after.count,
            "lines were added or dropped, so an operator's file is not the client's"
        )

        let changed = zip(before, after).filter { $0 != $1 }
        XCTAssertEqual(
            ["editor = nano", "enable_node = no", "node_name = None"],
            changed.map { $0.0 },
            "the rewrite changed more than the three settings"
        )

        // Everything the rewrite did not name is byte-identical: putting the three original
        // lines back reconstructs the client's own default exactly.
        var restored = after
        for (index, pair) in zip(before, after).enumerated() where pair.0 != pair.1 {
            restored[index] = pair.0
        }
        XCTAssertEqual(original, restored.joined(separator: "\n"))
        XCTAssertTrue(patched.contains("# Whether to enable node hosting"))
        XCTAssertTrue(patched.contains("colormode = 24bit"))
    }

    func testAKeyInAnotherSectionIsNotTouched() throws {
        // The keys are read out of their own sections and nowhere else, so a rewrite that
        // scanned the whole document would be changing values nobody asked it to change the
        // first time another section grew a key of the same name.
        let elsewhere = [
            "[client]",
            "enable_node = no",
            "node_name = None",
            "",
            "[textui]",
            "editor = nano",
            "",
            "[node]",
            "enable_node = no",
            "node_name = None",
        ].joined(separator: "\n") + "\n"

        let expected = [
            "[client]",
            "enable_node = no",
            "node_name = None",
            "",
            "[textui]",
            "editor = \(editor)",
            "",
            "[node]",
            "enable_node = yes",
            "node_name = \(ClientConfig.nodeName)",
        ].joined(separator: "\n") + "\n"
        XCTAssertEqual(expected, try ClientConfig.settings(template: elsewhere, editor: editor))
    }

    func testTheShapeOfTheTemplateIsTheTemplates() throws {
        let indented = [
            "[textui]",
            "\teditor = nano",
            "",
            "[node]",
            "  enable_node = no",
            "  node_name = None",
        ].joined(separator: "\n") + "\n"

        let expected = [
            "[textui]",
            "\teditor = \(editor)",
            "",
            "[node]",
            "  enable_node = yes",
            "  node_name = \(ClientConfig.nodeName)",
        ].joined(separator: "\n") + "\n"
        XCTAssertEqual(expected, try ClientConfig.settings(template: indented, editor: editor))
    }

    func testATemplateThatNoLongerNamesTheKeysIsRefused() {
        let renamed = "[node]\nenable_node = no\nnode_naming = None\n"
        XCTAssertThrowsError(try ClientConfig.settings(template: renamed, editor: editor)) { error in
            let message = (error as? LocalizedError)?.errorDescription ?? "\(error)"
            XCTAssertTrue(message.contains("node_name"), message)
        }
        XCTAssertThrowsError(
            try ClientConfig.settings(template: "[node]\nnode_name = None\n", editor: editor)
        )
        XCTAssertThrowsError(
            try ClientConfig.settings(
                template: "[node]\nenable_node = no\nnode_name = None\n",
                editor: editor
            )
        )
    }

    func testTheNamesTheApplianceStatesAreTheApplianceOnes() {
        XCTAssertEqual("gonomadnet on iOS", ClientConfig.nodeName)
        XCTAssertEqual(["nano", "editor", ""], ClientConfig.replaceableEditors)
    }

    /// The editor the appliance states names no program.
    ///
    /// This is the acceptance for configuration editing on a platform with no processes to
    /// start: a command written here is a command that can never run, and the client's
    /// Config page would fail blaming a program that does not exist instead of naming the
    /// screen in the app that does the job.
    func testTheClientsEditorNamesNoProgram() {
        XCTAssertEqual("", ClientConfig.editorSetting)
        XCTAssertTrue(
            ClientConfig.replaceableEditors.contains(ClientConfig.editorSetting),
            "the appliance's own value is not one an operator's choice may replace"
        )
    }

    /// An operator's own edits to the client's configuration survive the appliance
    /// stating its values over them.
    ///
    /// The alternative — writing the shipped default on every start — is an editor whose
    /// work is discarded by the next Start Stack, which is worse than no editor.
    func testAnOperatorsEditsSurviveTheApplianceStatingItsOwnValues() throws {
        let edited = """
            [client]
            nick = Field Station

            [textui]
            theme = light
            editor = nano

            [node]
            enable_node = no
            node_name = None

            """

        let applied = try ClientConfig.applied(to: edited)

        XCTAssertTrue(applied.contains("nick = Field Station"), applied)
        XCTAssertTrue(applied.contains("theme = light"), applied)
        XCTAssertTrue(applied.contains("enable_node = yes"), applied)
        XCTAssertTrue(applied.contains("node_name = \(ClientConfig.nodeName)"), applied)
        XCTAssertTrue(applied.contains("editor = \(ClientConfig.editorSetting)"), applied)
    }

    /// An edited file that dropped a key the appliance has to state is refused, and the
    /// refusal names the section it was looked for in.
    func testAnEditedFileThatDroppedAKeyIsRefused() {
        let dropped = "[node]\nenable_node = yes\n"

        XCTAssertThrowsError(try ClientConfig.applied(to: dropped)) { error in
            guard case let ClientConfigError.missingKeys(keys, section) = error else {
                return XCTFail("the refusal was \(error), want missing keys")
            }
            XCTAssertEqual(keys, ["node_name"])
            XCTAssertEqual(section, "node")
        }
    }
}
