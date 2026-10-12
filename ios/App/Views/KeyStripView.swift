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

/// KeyStripView is the two rows of keys a touch keyboard does not have.
///
/// A touch keyboard has letters and little else, and the client's interface is driven by
/// `ESC`, `TAB`, the arrows, `HOME`, `END`, `PGUP` and `PGDN`. Without them a reader on a
/// tablet can read the interface and not use it. The strip is what an Apple keyboard
/// gives for free and this gives to everybody else — and it hides itself the moment a
/// real keyboard is attached, because then it is rows taken from the console for nothing.
///
/// The keys and their order are the appliance's own (`TerminalKit`'s key strip), so the
/// strip a reader taps and the strip the Android appliance draws cannot drift apart. What
/// is here is only the drawing.
struct KeyStripView: View {
    /// Which keys are latched, so the strip can show it.
    @Binding var latch: ModifierLatch

    /// The metrics of the console the strip sits above, which is what sets its height.
    let cellHeight: Double

    /// Called when a key is tapped.
    let onKey: (TerminalKey) -> Void

    /// Called when a latch is tapped.
    let onLatch: (ModifierLatch.Kind) -> Void

    private var rowHeight: Double { TerminalMetrics.rowHeight(forCellHeight: cellHeight) }

    var body: some View {
        VStack(spacing: 0) {
            row(TerminalMetrics.rowOne)
            row(TerminalMetrics.rowTwo)
        }
        .padding(.vertical, TerminalMetrics.stripPadding(rowHeight: rowHeight))
        .frame(height: TerminalMetrics.stripHeight(rowHeight: rowHeight))
        .background(keyBackground)
    }

    /// row draws one row of keys, each taking an equal share of the width.
    private func row(_ entries: [KeyStripEntry]) -> some View {
        HStack(spacing: 0) {
            ForEach(Array(entries.enumerated()), id: \.offset) { _, entry in
                key(entry)
                    .frame(maxWidth: .infinity)
                    .frame(height: rowHeight)
            }
        }
    }

    /// key draws one entry, which is either a key or one of the two latches.
    @ViewBuilder
    private func key(_ entry: KeyStripEntry) -> some View {
        switch entry {
        case .key(let terminalKey):
            Button {
                onKey(terminalKey)
            } label: {
                label(Self.title(for: terminalKey), armed: false)
            }
            .buttonStyle(.plain)

        case .control:
            latchKey(.control, title: "Ctrl")
        case .alt:
            latchKey(.alt, title: "Alt")
        }
    }

    /// latchKey draws one of the two modifiers, showing whether it is armed.
    ///
    /// A latch a reader cannot see the state of is a latch they will leave on by accident,
    /// so the armed state is shown as a filled background rather than a color: it has to
    /// be visible in both the light and the dark appearance.
    private func latchKey(_ kind: ModifierLatch.Kind, title: String) -> some View {
        Button {
            onLatch(kind)
        } label: {
            label(title, armed: latch.isArmed(kind))
        }
        .buttonStyle(.plain)
    }

    private func label(_ text: String, armed: Bool) -> some View {
        Text(text)
            .font(.system(size: rowHeight * 0.45, weight: .medium, design: .monospaced))
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .background(armed ? Color.accentColor.opacity(0.55) : Color.clear)
            .contentShape(Rectangle())
    }

    private var keyBackground: some View {
        // The strip is chrome, not content: a flat surface that reads as separate from the
        // console without competing with the client's own colors.
        Rectangle().fill(Color(white: 0.12))
    }

    /// title is the word drawn on a key, which is the key's own name rather than its
    /// encoding: `PGUP` is what a keyboard has printed on it.
    static func title(for key: TerminalKey) -> String {
        switch key {
        case .escape: return "ESC"
        case .tab: return "TAB"
        case .up: return "↑"
        case .down: return "↓"
        case .left: return "←"
        case .right: return "→"
        case .home: return "HOME"
        case .end: return "END"
        case .pageUp: return "PGUP"
        case .pageDown: return "PGDN"
        case .rune(let text): return text
        case .enter: return "⏎"
        case .backspace: return "⌫"
        case .delete: return "DEL"
        case .insert: return "INS"
        case .ctrl(let character): return "^\(character)"
        }
    }
}
