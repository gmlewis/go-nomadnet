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

/// How big one character is, and what fits in a rectangle of points.
///
/// The appliance has to tell the client what terminal it is on: the client lays itself out
/// for the columns and rows it is given, and a terminal that is one column wider than the
/// screen is a line of text with its last character cut off. So the size comes from the font
/// the view actually draws with and the rectangle it actually has, rather than from a guess,
/// and the fit rounds down: half a character is not a character.
///
/// The metrics are a value so that a view can hold one, and so that what it draws and what it
/// tells the client are the same arithmetic rather than two of it.
public struct TerminalMetrics: Equatable, Sendable {

    /// How wide one character is, in points.
    public let cellWidth: Int

    /// How tall one character is, in points.
    public let cellHeight: Int

    /// How many whole columns fit in the rectangle.
    public let cols: Int

    /// How many whole rows fit in the rectangle.
    public let rows: Int

    /// How wide the display the console is drawn on is, in the unit the app measured it in.
    ///
    /// It is the display's size and not the console rectangle's, because it is what tells a
    /// console standing up from one lying down: its own interface changes with the shape of
    /// its terminal, and a cell is about twice as tall as it is wide, so a console in
    /// portrait holds more columns than rows. The display is what the device's rotation
    /// turns, so a rotation is a change in this size whether or not the on-screen keyboard
    /// is up — a console rectangle shrinks by the keyboard's height and would say the
    /// console is lying down while the device is standing up.
    public let pixelWidth: Int

    /// How tall the display the console is drawn on is, in the same unit.
    public let pixelHeight: Int

    /// Metrics for a cell of `cellWidth` by `cellHeight` points and a `cols` by `rows` grid.
    ///
    /// A cell of no points is not a small font, it is a font that has not been measured, and
    /// every division over these would be by zero; the invariant is therefore kept by
    /// clamping rather than by trusting the caller.
    ///
    /// The display size defaults to nothing measured, which is what a caller that only knows
    /// a grid has to report; a view attaches the display it is drawn on with
    /// ``onDisplay(width:height:)``.
    public init(
        cellWidth: Int,
        cellHeight: Int,
        cols: Int,
        rows: Int,
        pixelWidth: Int = 0,
        pixelHeight: Int = 0
    ) {
        self.cellWidth = max(1, cellWidth)
        self.cellHeight = max(1, cellHeight)
        self.cols = max(1, cols)
        self.rows = max(1, rows)
        self.pixelWidth = max(0, pixelWidth)
        self.pixelHeight = max(0, pixelHeight)
    }

    /// onDisplay returns these metrics as they are on a display of this pixel size.
    ///
    /// The grid is unchanged: the cells are the console's, and this is the display they are
    /// drawn on, which is what the client reads to know which way up its terminal is.
    public func onDisplay(width: Int, height: Int) -> TerminalMetrics {
        TerminalMetrics(
            cellWidth: cellWidth,
            cellHeight: cellHeight,
            cols: cols,
            rows: rows,
            pixelWidth: width,
            pixelHeight: height)
    }

    /// The grid a rectangle can hold, given the cell the view draws with and the strip of
    /// keys drawn over its bottom edge.
    ///
    /// It is nil when there is nothing to measure: a cell of no points (a font that has not
    /// been laid out yet), a rectangle with no width, or one whose height is entirely taken
    /// by the key strip. Anything else is a terminal, and the view has to have one: a console
    /// with no rows is not a console it can draw or a size it can tell the client.
    public static func metrics(
        bounds: (width: Double, height: Double),
        cellWidth: Double,
        cellHeight: Double,
        keyStripHeight: Double
    ) -> TerminalMetrics? {
        guard bounds.width.isFinite, bounds.height.isFinite,
              cellWidth.isFinite, cellHeight.isFinite,
              cellWidth > 0, cellHeight > 0, bounds.width > 0 else { return nil }
        let stripped = keyStripHeight.isFinite ? max(0, keyStripHeight) : 0
        let available = bounds.height - stripped
        guard available > 0 else { return nil }

        let width = max(1, Int(cellWidth.rounded(.down)))
        let height = max(1, Int(cellHeight.rounded(.down)))
        return TerminalMetrics(
            cellWidth: width,
            cellHeight: height,
            cols: max(1, Int(bounds.width.rounded(.down)) / width),
            rows: max(1, Int(available.rounded(.down)) / height)
        )
    }

    /// The cell a point is in, clamped into the grid.
    ///
    /// A point outside the view is clamped rather than refused: a finger on the edge of the
    /// screen is a finger on a cell.
    public func pixelToCell(x: Double, y: Double) -> (col: Int, row: Int) {
        (
            col: Self.index(of: x, cell: cellWidth, count: cols),
            row: Self.index(of: y, cell: cellHeight, count: rows)
        )
    }

    /// Where a cell starts, from the view's top-left corner.
    public func cellToPixel(col: Int, row: Int) -> (x: Double, y: Double) {
        (x: Double(col * cellWidth), y: Double(row * cellHeight))
    }

    // MARK: - The key strip

    /// The strip's top row: escape, the two characters a path is spelled with, and
    /// navigation.
    public static let rowOne: [KeyStripEntry] = [
        .key(.escape),
        .key(.rune("/")),
        .key(.rune("-")),
        .key(.home),
        .key(.up),
        .key(.end),
        .key(.pageUp),
    ]

    /// The strip's bottom row: TAB, the two latches and the rest of the navigation.
    public static let rowTwo: [KeyStripEntry] = [
        .key(.tab),
        .control,
        .alt,
        .key(.left),
        .key(.down),
        .key(.right),
        .key(.pageDown),
    ]

    /// How tall one key-strip row is, from one client cell's height.
    ///
    /// The strip's rows are ``fontScale`` times a client cell so a key is a finger-sized
    /// target rather than a text-sized one; the view sizes its key font to the same scale, so
    /// a label and the row it sits in grow together.
    public static func rowHeight(forCellHeight cellHeight: Double) -> Double {
        guard cellHeight.isFinite else { return (fontScale).rounded() }
        return (max(1, cellHeight) * fontScale).rounded()
    }

    /// The space above and below the strip's rows, from one row's height.
    ///
    /// It is a fraction of the row so that the strip keeps its proportions with the font size,
    /// and never less than a few points: a button with no margin is a button whose label
    /// touches the row above it. The division is the whole-point one the reference does, so a
    /// thirty-point row has seven points above it and not seven and a half.
    public static func stripPadding(rowHeight: Double) -> Double {
        guard rowHeight.isFinite, rowHeight > 0 else { return Double(minimumPadding) }
        return Double(max(minimumPadding, Int(rowHeight) / 4))
    }

    /// How much of the bottom edge the strip's rows take, given one row's height.
    ///
    /// The rows are the client's own: what the strip takes from the bottom of the screen is
    /// what the client does not get, which is why showing and hiding it is a resize rather
    /// than a redraw.
    public static func stripHeight(rowHeight: Double) -> Double {
        stripHeight(rowHeight: rowHeight, rowCount: rowCount)
    }

    /// How much of the bottom edge `rowCount` rows take, given one row's height.
    ///
    /// A strip that is not shown takes nothing at all, which is how a console with the soft
    /// keyboard hidden gets the whole screen.
    public static func stripHeight(rowHeight: Double, rowCount: Int) -> Double {
        guard rowCount > 0 else { return 0 }
        let row = rowHeight.isFinite ? max(1, rowHeight) : 1
        return Double(rowCount) * row + 2 * stripPadding(rowHeight: rowHeight)
    }

    /// The button a tap at (`x`, `y`) landed on, in the view's own points.
    ///
    /// `bounds` is the strip's own rectangle — its width, and the height the rows are drawn
    /// in — and the height is divided equally among the rows while each row's buttons take
    /// equal shares of its width. A view that draws the strip itself has the row height and
    /// the strip's top, and should use the form that takes them instead, which divides by
    /// exactly what was drawn.
    ///
    /// A row or a column a tap cannot be outside of is clamped rather than rejected, because a
    /// finger on an edge is a finger on a button, and a strip with no width yet — a view
    /// between construction and its first layout — is its first button rather than a division
    /// by zero. A strip with no buttons at all is nothing a finger can land on.
    public static func entry(
        at x: Double,
        y: Double,
        bounds: (width: Double, height: Double),
        rows: [[KeyStripEntry]]
    ) -> KeyStripEntry? {
        guard !rows.isEmpty else { return nil }
        let band = bounds.height.isFinite && bounds.height > 0
            ? bounds.height / Double(rows.count)
            : 0
        return entry(at: x, y: y, width: bounds.width, stripTop: 0, rowHeight: band, rows: rows)
    }

    /// The button a tap at (`x`, `y`) landed on, divided by the geometry the strip is drawn
    /// with.
    ///
    /// `stripTop` is where the strip begins and `rowHeight` how tall one of its rows is, both
    /// of which are the view's arithmetic: the rows are drawn in those bands, so a tap is
    /// divided by them as well. A row height of zero is a strip that has not been laid out,
    /// and every tap there is a tap on its first row rather than a division by zero.
    public static func entry(
        at x: Double,
        y: Double,
        width: Double,
        stripTop: Double,
        rowHeight: Double,
        rows: [[KeyStripEntry]]
    ) -> KeyStripEntry? {
        guard !rows.isEmpty else { return nil }
        let row: Int
        if rowHeight > 0, rowHeight.isFinite, y.isFinite, stripTop.isFinite {
            row = clamp(Int(((y - stripTop) / rowHeight).rounded(.down)), 0, rows.count - 1)
        } else {
            row = 0
        }
        let entries = rows[row]
        guard let first = entries.first else { return nil }
        guard width > 0, width.isFinite, x.isFinite else { return first }
        let index = clamp(Int((x * Double(entries.count) / width).rounded(.down)),
                          0, entries.count - 1)
        return entries[index]
    }

    /// How much larger a key-strip row, and its labels, is than a client cell.
    ///
    /// The strip's keys are touched, not read, and a target as small as one terminal cell is
    /// genuinely hard to hit on a tablet — so the row is one and a half cells tall and the
    /// labels are one and a half cells' font. (Twice was tried on hardware and is too big.)
    private static let fontScale = 1.5

    /// How many rows of keys the strip has, which is what ``stripHeight(rowHeight:)`` needs.
    private static let rowCount = 2

    /// The least space above and below the strip's rows, in points.
    private static let minimumPadding = 4

    /// The whole cell a coordinate falls in, clamped into `count` cells.
    private static func index(of value: Double, cell: Int, count: Int) -> Int {
        guard value.isFinite else { return 0 }
        return clamp(Int((value / Double(cell)).rounded(.down)), 0, count - 1)
    }

    /// Keeps a value between two bounds, with the lower one winning an impossible pair.
    private static func clamp(_ value: Int, _ low: Int, _ high: Int) -> Int {
        if value < low { return low }
        if value > high { return high }
        return value
    }
}

/// One button on the on-screen key strip.
///
/// A tablet's keyboard has no escape and no arrows, and the client cannot be driven without
/// them: escape backs out of a dialog, tab moves between the regions of a window, and the
/// arrows walk a menu. The strip carries the same keys the Termux extra-keys row does, so a
/// person who has driven this client under Termux has the same buttons here.
///
/// A button is what it sends and not what it says: the label is the view's, because a strip
/// that carried its own would be a second place for the two to disagree, and the arrows a page
/// is walked with are drawn as the glyphs the console's own font has.
public enum KeyStripEntry: Equatable, Sendable {

    /// A key that is sent when the button is tapped.
    case key(TerminalKey)

    /// The control latch, which is armed by a tap and spent by the next character.
    ///
    /// A tablet cannot hold one key and press another, so control is a switch rather than a
    /// key: that is how ctrl-c reaches the client at all.
    case control

    /// The alt (meta) latch, which is armed by a tap and spent by the next key.
    ///
    /// A terminal sends an alt combination as the key's own bytes with an escape in front of
    /// them, so this is a modifier the strip arms and spends rather than a key it presses.
    case alt
}
