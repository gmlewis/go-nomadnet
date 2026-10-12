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

/// ControlsView is the appliance's status and its buttons.
///
/// It is where a reader finds out what the appliance is doing — the transport, the
/// position, the heading, the console — and where they start and stop it. It is
/// deliberately the only place in the app that reports state, so that "what is the
/// appliance doing" has one answer rather than one per screen.
struct ControlsView: View {
    /// What the stack is doing.
    @ObservedObject var model: ApplianceModel

    /// Whether the configuration editor is open.
    @State private var showingConfigEditor = false

    var body: some View {
        Form {
            Section("Stack") {
                statusRow("Transport", value: model.transportStatus, ok: model.state.isRunning)
                statusRow("Console", value: model.consoleStatus, ok: model.consolePath != nil)

                if case .failed(let reason) = model.state {
                    Text(reason)
                        .font(.footnote)
                        .foregroundStyle(.red)
                }

                Button(model.state.isRunning ? "Stop Stack" : "Start Stack") {
                    model.toggleStack()
                }
                .disabled(model.state.isBusy)

                Button("Open gonomadnet") {
                    model.openConsole()
                }
                .disabled(!model.state.isRunning)

                Toggle("Run the stack", isOn: $model.enableStack)
                Text("With this off the appliance is a client and nothing else: it attaches to no transport of its own and hosts no node.")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }

            Section("Sensors") {
                statusRow("Position", value: model.locationStatus, ok: model.hasFix)
                statusRow("Heading", value: model.headingStatus, ok: model.hasHeading)

                Button(model.sensorsRunning ? "Stop Sensors" : "Start Sensors") {
                    model.toggleSensors()
                }

                Toggle("Publish sensors", isOn: $model.enableSensors)
                Text("With this off the appliance publishes no position and no heading at all, and the client's location forms and the bot's \"where\" have nothing to answer from.")
                    .font(.footnote)
                    .foregroundStyle(.secondary)

                Picker("Heading axis", selection: $model.headingAxis) {
                    Text("Screen back").tag(HeadingAxis.screenBack)
                    Text("Screen top").tag(HeadingAxis.screenTop)
                }
                .pickerStyle(.menu)

                Text(model.headingAxis == .screenBack
                     ? "For a device standing up, on a stand or in a keyboard case."
                     : "For a device lying flat on a table.")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }

            Section("Radio") {
                statusRow("Radio", value: model.radioStatus, ok: model.radioPaired)

                if model.radioPaired {
                    Button("Unpair") { model.unpairRadio() }
                } else {
                    Button("Scan for a radio") { model.scanForRadios() }

                    ForEach(model.radiosFound) { found in
                        Button("Pair with \(found.name)") { model.pair(found) }
                    }
                }

                Text("An RNode is reached over Bluetooth. The appliance serves it on a loopback address the transport dials, so the transport is told no more about it than where to find it. A radio is paired by name; it must be switched on, in range, and not connected to another device.")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }

            Section("Hub") {
                LabeledContent("Hub", value: model.settings.hubSpec)
                Button("Edit Configuration") { showingConfigEditor = true }
                Text("The public RRC hub the appliance's transport dials, and the client's own configuration. A host name is refused: a device with no DNS would never connect, and would never say why.")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }

            if !model.log.isEmpty {
                Section("Log") {
                    ForEach(Array(model.log.suffix(20).enumerated()), id: \.offset) { _, line in
                        Text(line)
                            .font(.system(.caption, design: .monospaced))
                            .foregroundStyle(.secondary)
                    }
                }
            }
        }
        .navigationTitle("gonomadnet")
        .sheet(isPresented: $showingConfigEditor) {
            ConfigEditorView(model: model)
        }
    }

    /// statusRow is one line of state, with the dot that says whether it is what it should be.
    private func statusRow(_ title: String, value: String, ok: Bool) -> some View {
        LabeledContent(title) {
            HStack(spacing: 6) {
                Circle()
                    .fill(ok ? Color.green : Color.secondary)
                    .frame(width: 8, height: 8)
                Text(value)
            }
        }
    }
}
