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

/// TerminalSize is a terminal's size in cells: what the client is told its window is.
///
/// It is a type of its own rather than a pair of integers because it is the one thing the two
/// halves of the console have to agree about, and because confusing it with the size of the
/// screen the app draws is the mistake this file exists to prevent.
public struct TerminalSize: Equatable, Sendable {
    /// How many columns the client has.
    public let cols: Int
    /// How many rows the client has.
    public let rows: Int

    /// Creates a size, which is never smaller than one cell in either direction.
    public init(cols: Int, rows: Int) {
        self.cols = max(1, cols)
        self.rows = max(1, rows)
    }
}

/// TerminalSizing decides when the client has to be told that its window has changed.
///
/// The appliance asks this question on every layout, and for a long time it asked it of the
/// wrong thing. It compared the grid the view could show against `screen.cols`, which is the
/// grid the app is *drawing* — and the view resizes that screen itself the moment the layout
/// changes, so the two were equal by construction and the answer was always "no change". No
/// RESIZE frame was ever sent. The appliance started its client at 80 by 24, the pane could
/// show 75 by 37, and the client spent the whole session drawing an 80-column interface into a
/// 75-column window: clipped on the right, with thirteen rows of the screen left black.
///
/// The question is only answerable against what the *client* believes, which is the size the
/// last frame carried. That is `TerminalSize`, and it is kept by the thing that sends the
/// frames rather than read back off the screen they are drawn into.
public enum TerminalSizing {
    /// resize returns the size to tell the client, or `nil` when it already has that size.
    ///
    /// - Parameters:
    ///   - metrics: the grid the view can show for its current bounds.
    ///   - told: the size the client was last told, which is what it is drawing for now.
    /// - Returns: the size to send, or `nil` when there is nothing to send.
    public static func resize(fitting metrics: TerminalMetrics, told: TerminalSize) -> TerminalSize? {
        guard metrics.cols != told.cols || metrics.rows != told.rows else { return nil }
        return TerminalSize(cols: metrics.cols, rows: metrics.rows)
    }
}
