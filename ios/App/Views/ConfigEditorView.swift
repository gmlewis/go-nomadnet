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

import ApplianceKit
import SwiftUI

/// ConfigEditorView is the appliance's editor for its own configuration.
///
/// The appliance's configuration is two files with two different owners, so the editor
/// treats them differently, which is the only honest way to present them:
///
///   - **Reticulum's configuration is rendered.** The appliance writes it from its own
///     settings every time the stack starts, so editing its text by hand would be an
///     edit the next Start Stack silently discards. The editor therefore edits the
///     *values* the renderer takes — the hub's address and port — and shows the exact
///     text they produce. What a reader sees after an accepted edit is the file the
///     transport will be started with, because it is the same rendering.
///   - **The client's configuration is the client's own file.** Nothing rewrites it but
///     the appliance's three values, which are stated over whatever is there, so
///     everything else a reader writes survives. It is edited as text.
///
/// Validation is the renderers' own in both cases: a hub host that is a name is refused
/// with `NodeConfigError.nonLiteralAddress`'s reason, and client configuration that has
/// lost a key the appliance has to state is refused with `ClientConfigError`'s. Neither
/// is re-implemented here, because a second set of rules is a second set of rules to be
/// wrong.
struct ConfigEditorView: View {
    /// The appliance, as the interface sees it.
    @ObservedObject var model: ApplianceModel

    /// Which file is being edited.
    private enum Pane: String, CaseIterable, Identifiable {
        case reticulum = "Reticulum"
        case client = "Client"
        var id: String { rawValue }
    }

    @Environment(\.dismiss) private var dismiss
    @State private var pane: Pane = .reticulum

    /// The hub's address and port as the editor has them, before they are accepted.
    @State private var hubHost = ""
    @State private var hubPort = ""

    /// The client's configuration as the editor has it.
    @State private var clientText = ""

    /// Why the last edit was refused, or nil when nothing has been.
    @State private var refusal: String?

    /// Whether a saved edit is waiting for the stack to be restarted to take effect.
    @State private var needsRestart = false

    var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                Picker("Configuration", selection: $pane) {
                    ForEach(Pane.allCases) { Text($0.rawValue).tag($0) }
                }
                .pickerStyle(.segmented)
                .padding([.horizontal, .top])

                switch pane {
                case .reticulum: reticulumPane
                case .client: clientPane
                }

                if let refusal {
                    Text(refusal)
                        .font(.footnote)
                        .foregroundStyle(.red)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding([.horizontal, .bottom])
                }
            }
            .navigationTitle("Configuration")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Done") { dismiss() }
                }
            }
        }
        .onAppear(perform: load)
    }

    // MARK: - Reticulum

    /// reticulumPane edits the values the transport's configuration is rendered from,
    /// and shows the file they produce.
    private var reticulumPane: some View {
        Form {
            Section("Hub") {
                TextField("Address or name", text: $hubHost)
                    .autocorrectionDisabled()
                    .textInputAutocapitalization(.never)
                    .keyboardType(.URL)
                TextField("Port", text: $hubPort)
                    .keyboardType(.numberPad)

                Text("A name is resolved here, while the appliance can still resolve one, and the address it resolves to is what the transport is given. A name that resolves to nothing leaves the appliance with no hub to dial, and says so.")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }

            Section("Saves") {
                Button("Apply") { applyHub() }
                    .disabled(hubHost.isEmpty || Int(hubPort) == nil)
                if needsRestart {
                    Button("Restart Stack") {
                        model.restartStack()
                        needsRestart = false
                    }
                }
            }

            if let hubProblem {
                Section("Hub") {
                    Text(hubProblem)
                        .font(.footnote)
                        .foregroundStyle(.orange)
                }
            }

            Section("Reticulum's configuration") {
                Text("This file is written from the settings above every time the stack starts, so it is edited by changing them, not by hand.")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                Text(renderedTransport)
                    .font(.system(.caption2, design: .monospaced))
                    .textSelection(.enabled)
            }
        }
    }

    /// renderedTransport is the transport's configuration as the settings produce it, or
    /// the renderer's reason it produces none.
    private var renderedTransport: String {
        switch model.renderedConfiguration() {
        case .success(let rendered):
            var text = ""
            if let address = rendered.hubAddress {
                text += "# The hub resolved to \(address)\n"
            }
            return text + rendered.transport
        case .failure(let error):
            return "\((error as? LocalizedError)?.errorDescription ?? "\(error)")"
        }
    }

    /// hubProblem is what the last accepted edit could not do, or nil.
    @State private var hubProblem: String?

    /// applyHub offers the edited hub to the renderer and the appliance.
    private func applyHub() {
        refusal = nil
        hubProblem = nil
        guard let port = Int(hubPort) else {
            refusal = "\"\(hubPort)\" is not a port"
            return
        }
        do {
            hubProblem = try model.setHub(host: hubHost, port: port)
            needsRestart = model.state.isRunning
        } catch {
            // The renderer's own words: a configuration it will not write is refused by the
            // code that would have written it, so the reason is the one the renderer gives.
            refusal = (error as? LocalizedError)?.errorDescription ?? "\(error)"
        }
    }

    // MARK: - Client

    /// clientPane edits the client's own configuration file as text.
    private var clientPane: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("The client's own configuration. The appliance states three values in it — the node is enabled and named, and the editor is left empty because iOS has no editor program to name. Every other line is yours, and survives the next start.")
                .font(.footnote)
                .foregroundStyle(.secondary)
                .padding(.horizontal)

            TextEditor(text: $clientText)
                .font(.system(.caption, design: .monospaced))
                .autocorrectionDisabled()
                .textInputAutocapitalization(.never)
                .border(Color.secondary.opacity(0.3))

            HStack {
                Spacer()
                Button("Save") { saveClient() }
            }
            .padding(.horizontal)
        }
        .padding(.bottom)
    }

    /// saveClient writes the edited client configuration, and shows back what the
    /// appliance wrote.
    private func saveClient() {
        refusal = nil
        do {
            clientText = try model.saveClientConfig(clientText)
        } catch {
            refusal = (error as? LocalizedError)?.errorDescription ?? "\(error)"
        }
    }

    // MARK: - Loading

    /// load fills the editor from the appliance's settings and the client's file.
    private func load() {
        hubHost = model.settings.hubHost
        hubPort = "\(model.settings.hubPort)"
        do {
            clientText = try model.clientConfigText()
        } catch {
            refusal = (error as? LocalizedError)?.errorDescription ?? "\(error)"
        }
    }
}
