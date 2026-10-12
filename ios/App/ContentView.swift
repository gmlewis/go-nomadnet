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

import SwiftUI
import TerminalKit

/// ContentView is the appliance's one screen.
///
/// On an iPad it is a split view: the controls beside a console on the same screen, which
/// is the shape the appliance wants — the reader watches the status while they use the
/// client, and changes one without leaving the other. On an iPhone there is not room for
/// both, so the controls are the root and the console is pushed over them, which is the
/// same two things in the only order a phone can show them.
struct ContentView: View {
    /// The appliance, as the interface sees it.
    @ObservedObject var model: ApplianceModel

    /// Whether the console is on screen on a phone.
    @State private var showingConsole = false

    var body: some View {
        Group {
            if UIDevice.current.userInterfaceIdiom == .pad {
                NavigationSplitView {
                    ControlsView(model: model)
                } detail: {
                    consolePane
                }
            } else {
                NavigationStack {
                    ControlsView(model: model)
                        .toolbar {
                            Button("Console") { showingConsole = true }
                                .disabled(model.terminal == nil)
                        }
                        .navigationDestination(isPresented: $showingConsole) {
                            consolePane
                        }
                }
            }
        }
        // The appliance's configuration is written before anything is started, which is what
        // makes the Config editor and the client's own configuration file something a reader
        // can look at on a device they have just installed this on. It is idempotent, and it
        // leaves an edited file alone, so being called again is what keeps those edits.
        .onAppear { model.prepare() }
    }

    /// consolePane is the terminal, or what stands in for it before there is one.
    ///
    /// The empty state names the button that fixes it rather than saying "no console",
    /// because a reader who has opened the app for the first time has no way to know that
    /// the stack comes before the console.
    @ViewBuilder
    private var consolePane: some View {
        if let terminal = model.terminal {
            ConsolePane(terminal: terminal)
        } else {
            VStack(spacing: 8) {
                Image(systemName: "terminal")
                    .font(.largeTitle)
                    .foregroundStyle(.secondary)
                Text("No console open")
                    .font(.headline)
                Text(model.state.isRunning
                     ? "Press Open gonomadnet to attach the client."
                     : "Press Start Stack, then Open gonomadnet.")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
            }
            .padding()
        }
    }
}

/// ConsolePane is the terminal itself, with the key strip above the keyboard.
///
/// The strip appears when the software keyboard does and hides when a hardware one is
/// attached: on a Magic Keyboard the strip would be two rows taken from the console for
/// keys the reader already has.
struct ConsolePane: View {
    /// The console's controller, which owns the session and the screen.
    let terminal: TerminalController

    /// The latches the strip and the keyboard share.
    @State private var latch = ModifierLatch()

    /// Whether the software keyboard is up, which is what the strip follows.
    @State private var keyboardHeight: Double = 0

    /// The metrics of the console, which set the strip's height.
    @State private var cellHeight: Double = 20

    var body: some View {
        VStack(spacing: 0) {
            TerminalHostView(terminal: terminal, cellHeight: $cellHeight, onKey: send)
                .ignoresSafeArea(.keyboard, edges: .bottom)

            if keyboardHeight > 0 {
                KeyStripView(latch: $latch, cellHeight: cellHeight) { key in
                    send(key)
                } onLatch: { kind in
                    // A tap on a latch arms it or clears it, and the next key spends it.
                    if latch.isArmed(kind) {
                        latch.disarm(kind)
                    } else {
                        latch.arm(kind)
                    }
                }
            }
        }
        .onReceive(NotificationCenter.default.publisher(for: UIResponder.keyboardWillShowNotification)) { note in
            let frame = note.userInfo?[UIResponder.keyboardFrameEndUserInfoKey] as? CGRect
            keyboardHeight = Double(frame?.height ?? 0)
        }
        .onReceive(NotificationCenter.default.publisher(for: UIResponder.keyboardWillHideNotification)) { _ in
            keyboardHeight = 0
        }
    }

    /// send gives a key to the client, spending the latches.
    ///
    /// It is the one place a key becomes bytes, whatever produced it — a button on the strip,
    /// the software keyboard, or a hardware one — which is what makes the two latches a single
    /// piece of state: the strip draws them, both keyboards spend them, and the strip stops
    /// showing one as lit the moment it is spent.
    private func send(_ key: TerminalKey) {
        let (control, alt) = latch.spend(key)
        guard let bytes = TerminalKeys.bytes(for: key, control: control, alt: alt) else { return }
        terminal.send(bytes)
    }
}

/// TerminalHostView puts the Core Text terminal into SwiftUI.
///
/// A `UIViewRepresentable` rather than a `Canvas` because the client's output arrives in
/// small chunks at its own pace and only the changed cells are worth redrawing, which is
/// something a view can do and a `Canvas` cannot.
struct TerminalHostView: UIViewRepresentable {
    /// The console's controller.
    let terminal: TerminalController

    /// Reports the console's cell height back, which is what sizes the key strip.
    @Binding var cellHeight: Double

    /// What a key from the console's own keyboard is given to, which is the pane.
    ///
    /// The key rather than its bytes: the modifier latches belong to the pane, so that the
    /// button the strip draws as lit and the key that spends it are the same latch.
    let onKey: (TerminalKey) -> Void

    func makeUIView(context: Context) -> TerminalView {
        let view = TerminalView(
            frame: .zero,
            screen: terminal.screen,
            fontSize: UIFont.preferredFont(forTextStyle: .body).pointSize
        )
        view.onKey = onKey
        view.onMouse = { bytes in terminal.send(bytes) }
        // The view reports the grid it can show whenever its own bounds change — a rotation, a
        // split view, the key strip — and the controller decides whether the client has to be
        // told. Nothing in SwiftUI's own update path is a notification that a window has been
        // laid out, which is why the view has to say so itself.
        view.onGridChanged = { [weak terminal] metrics in
            terminal?.resizeIfNeeded(to: metrics)
        }
        terminal.onScreenChanged = { [weak view] in view?.screenChanged() }
        return view
    }

    func updateUIView(_ view: TerminalView, context: Context) {
        // The grid is settled by the view's own layout; this only keeps the key strip's height
        // in step with the cell the terminal is drawn at.
        let metrics = view.relayout()
        cellHeight = Double(metrics.cellHeight)
    }
}
