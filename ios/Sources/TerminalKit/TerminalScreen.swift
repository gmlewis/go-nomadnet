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

/// The emulated terminal's screen: its grid, its cursor and its scroll region.
///
/// The appliance runs the client on a pseudo-terminal, so what arrives over the console
/// socket is the byte stream a terminal emulator would receive, and the app is the
/// emulator. This class is the part of it that holds state — cells, cursor, scroll region,
/// alternate screen — while ``TerminalParser`` is the part that reads the escape sequences
/// and drives it.
///
/// It is a port of the Go client's `tui/vterm.go`, deliberately function for function:
/// that file is what the same client draws through on a desktop terminal, and two
/// emulators that disagree are not an error anyone sees, they are a screen with the wrong
/// characters on it. The Go file's vectors are asserted here against the same behavior
/// rather than retyped, and `TerminalScreenTests` keeps each one readable beside its
/// original.
///
/// One thing here has no counterpart in `vterm.go`, and cannot have: a scrollback. On a
/// desktop the client draws through a terminal emulator that owns the lines which leave
/// the window, and the Go file is only ever the screen. In the appliance this emulator
/// *is* the terminal, so the lines that scroll off the top are kept here — see
/// ``scrollbackLine(_:)`` — and the view is what a reader drags back through them.
///
/// The screen is not thread-safe, and does not pretend to be: the parser writes it from
/// the queue that reads the console socket, and the view reads it when it draws, so the
/// two have to be serialized by whatever owns them.
public final class TerminalScreen {

    /// How many lines of scrollback a screen keeps unless its owner says otherwise.
    ///
    /// A tablet runs for weeks, so this is a bound rather than a policy: the oldest lines
    /// go as the newest arrive, and the memory a session can hold does not depend on how
    /// long it has been running.
    public static let defaultMaxScrollback = 500

    /// The screen's width, in columns.
    public private(set) var cols: Int

    /// The screen's height, in rows.
    public private(set) var rows: Int

    /// How many lines of scrollback this screen keeps.
    public let maxScrollback: Int

    /// The cursor's column, 0-based.
    public private(set) var cursorX = 0

    /// The cursor's row, 0-based.
    public private(set) var cursorY = 0

    /// Whether the child has asked for a visible cursor (CSI ?25h / ?25l).
    public private(set) var cursorVisible = true

    /// Whether the child wants mouse events forwarded (CSI ?1000h / ?1002h / ?1003h).
    public private(set) var mouseReporting = false

    /// Whether those events use the SGR encoding the modern clients ask for (CSI ?1006h).
    public private(set) var mouseSGR = false

    /// Whether the alternate screen is the one being drawn on (CSI ?1049h / ?1049l).
    public private(set) var isAltScreen = false

    /// The color and attributes a character written now would be drawn with.
    public private(set) var pen = TerminalPen.plain

    /// The first row the screen scrolls, 0-based.
    public private(set) var scrollTop = 0

    /// The last row the screen scrolls, 0-based and inclusive.
    public private(set) var scrollBottom: Int

    /// How many lines have left the top of the screen over this screen's whole life,
    /// whether or not the scrollback still holds them.
    ///
    /// It is what a view re-anchors a reader by. The scrollback is bounded, so
    /// ``scrollbackCount`` stops growing once the buffer is full and cannot say how much
    /// output has arrived since; this keeps counting, and the difference between two
    /// readings is exactly the number of lines that have appeared below a reader parked in
    /// the scrollback.
    ///
    /// Only the console's own lines count. A scroll inside a region and a full-screen
    /// client's repaint are the client drawing over itself, and they add nothing to the
    /// console for a reader to have been pushed down by.
    public private(set) var linesScrolledOff = 0

    /// The grid, one array of cells per row.
    private var grid: [[TerminalCell]]

    /// The lines that have left the top of the screen, oldest first.
    private var scrollback: [[TerminalCell]] = []

    /// Whether the cursor rests on the last column waiting for the next rune to wrap.
    ///
    /// A terminal wraps when the next character arrives, not when the last one is written,
    /// which is what lets a program overtype the last column of a full line.
    private var wrapPending = false

    /// The primary screen, kept while the alternate screen is in use.
    private var savedGrid: [[TerminalCell]]?

    /// The cursor the alternate screen's switch remembers, with the screen it belongs to.
    private var savedCursorX = 0
    private var savedCursorY = 0

    /// The cursor the child saves for itself with DECSC/DECRC or CSI s / CSI u.
    ///
    /// It is held apart from the alternate screen's own save, which the client's startup
    /// makes for itself (CSI ?1049h): a child that borrows the cursor to draw with must not
    /// disturb the cursor the screen switch will restore.
    private var decSavedX = 0
    private var decSavedY = 0
    private var decSavedPen = TerminalPen.plain

    /// A screen of `cols` by `rows` cells, all blank, with the cursor at the origin.
    ///
    /// A size that is not a screen — a surface that has not been laid out yet measures as
    /// zero — is clamped to one cell rather than refused, because a screen with no columns
    /// is not a terminal and every division below would be by zero.
    public init(cols: Int, rows: Int, maxScrollback: Int = TerminalScreen.defaultMaxScrollback) {
        let width = max(1, cols)
        let height = max(1, rows)
        self.cols = width
        self.rows = height
        self.maxScrollback = max(0, maxScrollback)
        self.scrollBottom = height - 1
        self.grid = Self.blankGrid(cols: width, rows: height, pen: .plain)
    }

    // MARK: - Reading the screen

    /// One cell of the screen, glyph and pen together, for the renderer to draw.
    ///
    /// A cell outside the screen is blank rather than an error: a caller that is drawing a
    /// partly-visible screen asks for the cells its rectangle covers, and a finger can be
    /// past the edge of a view.
    public func cell(col: Int, row: Int) -> TerminalCell {
        guard row >= 0, row < rows, col >= 0, col < cols else { return .blank }
        return grid[row][col]
    }

    /// One row's characters, the wide glyphs' spare columns left out.
    public func rowText(_ row: Int) -> String {
        guard row >= 0, row < rows else { return String(repeating: " ", count: cols) }
        return Self.text(of: grid[row])
    }

    /// The whole screen, one row per line.
    public func text() -> String {
        (0..<rows).map { Self.text(of: grid[$0]) }.joined(separator: "\n")
    }

    /// How many lines have left the top of the screen and been kept.
    public var scrollbackCount: Int { scrollback.count }

    /// One kept line's cells, oldest first: index 0 is the oldest line there is.
    public func scrollbackLine(_ index: Int) -> [TerminalCell] {
        guard index >= 0, index < scrollback.count else {
            return Array(repeating: .blank, count: cols)
        }
        return scrollback[index]
    }

    // MARK: - The pen

    /// Changes the pen a character written now would be drawn with.
    public func setPen(_ pen: TerminalPen) {
        self.pen = pen
    }

    /// Applies an SGR sequence's parameters — already split on their semicolons — to the
    /// pen, which is where the pen lives.
    ///
    /// A terminal applies an SGR sequence to the pen it is holding rather than from nothing,
    /// which is what lets a program set a color once and change only the attributes after
    /// it.
    public func applySgr(_ params: [String]) {
        pen = Self.pen(params, appliedTo: pen)
    }

    // MARK: - Modes

    /// Shows or hides the cursor (CSI ?25h / ?25l).
    public func setCursorVisible(_ on: Bool) {
        cursorVisible = on
    }

    /// Turns the forwarding of mouse events on or off (CSI ?1000h and its relatives).
    public func setMouseReporting(_ on: Bool) {
        mouseReporting = on
    }

    /// Turns the SGR mouse encoding on or off (CSI ?1006h): the modern encoding, which
    /// carries coordinates past the 223 columns the X10 encoding can spell.
    public func setMouseSGR(_ on: Bool) {
        mouseSGR = on
    }

    // MARK: - The cursor

    /// Puts the cursor at a cell and clamps it into the screen.
    ///
    /// Every cursor-addressing escape sequence ends here, so the move also cancels a pending
    /// wrap: a terminal that is about to place the cursor has no wrap to defer.
    public func setCursor(x: Int, y: Int) {
        wrapPending = false
        cursorX = x
        cursorY = y
        clampCursor()
    }

    /// Puts the cursor at a cell and clamps it into the screen; the same placement
    /// ``setCursor(x:y:)`` makes, under the name the parser's sequences read as.
    public func moveCursor(x: Int, y: Int) {
        setCursor(x: x, y: y)
    }

    /// Returns the cursor to the start of its row (CR).
    public func carriageReturn() {
        cursorX = 0
        wrapPending = false
    }

    /// Drops a wrap the cursor is deferring without moving it.
    ///
    /// Every escape sequence and every control byte arrives with the terminal about to do
    /// something of its own, and each of them ends the deferral: `vterm.go` clears its
    /// `wrapPending` in `dispatchCSI` and beside every control byte, and an emulator that
    /// did not would wrap where the Go one writes in place.
    public func cancelWrap() {
        wrapPending = false
    }

    /// Moves the cursor one column left, no further than the first (BS).
    public func backspace() {
        if cursorX > 0 {
            cursorX -= 1
        }
        wrapPending = false
    }

    /// Advances the cursor to the next multiple of eight columns (HT).
    public func tab() {
        cursorX = ((cursorX / 8) + 1) * 8
        if cursorX >= cols {
            cursorX = cols - 1
        }
        wrapPending = false
    }

    /// Remembers the cursor and the pen for ``restoreCursor()`` (DECSC, CSI s).
    public func saveCursor() {
        decSavedX = cursorX
        decSavedY = cursorY
        decSavedPen = pen
    }

    /// Returns to the cursor and pen ``saveCursor()`` remembered (DECRC, CSI u).
    public func restoreCursor() {
        setCursor(x: decSavedX, y: decSavedY)
        pen = decSavedPen
    }

    // MARK: - Writing

    /// Writes one printable character at the cursor and advances it by the columns it
    /// occupies.
    ///
    /// `width` is 2 for the double-width characters a CJK page or an emoji is made of. Such a
    /// glyph is held whole: a lead cell that carries it and a spare cell after it that carries
    /// nothing, which is what keeps a row's columns countable. A glyph that would not fit at
    /// the end of a row moves whole to the next one rather than being split across the margin.
    public func putGlyph(_ glyph: String, width: Int) {
        if wrapPending {
            cursorX = 0
            lineFeed()
            wrapPending = false
        }
        guard cursorY >= 0, cursorY < rows else { return }
        let columns = max(1, min(width, cols))
        if cursorX + columns > cols {
            cursorX = 0
            lineFeed()
        }
        guard cursorY >= 0, cursorY < rows, cursorX >= 0, cursorX < cols else { return }
        grid[cursorY][cursorX] = TerminalCell(glyph: glyph, pen: pen, width: columns)
        for i in 1..<columns where cursorX + i < cols {
            grid[cursorY][cursorX + i] = TerminalCell(glyph: "", pen: pen, width: 0)
        }
        cursorX += columns
        if cursorX >= cols {
            wrapPending = true
            cursorX = cols - 1 // the cursor rests on the last column until the next rune
        }
    }

    /// Moves the cursor down one row, scrolling the region at its bottom.
    public func lineFeed() {
        if cursorY == scrollBottom {
            scrollUp(1)
            return
        }
        if cursorY < rows - 1 {
            cursorY += 1
        }
    }

    /// Moves the cursor up one row, scrolling the region at its top (RI).
    public func reverseLineFeed() {
        if cursorY == scrollTop {
            scrollDown(1)
            return
        }
        if cursorY > 0 {
            cursorY -= 1
        }
    }

    /// Moves the scroll region's rows up and blanks the row it leaves behind.
    public func scrollUp(_ count: Int) {
        guard count > 0 else { return }
        for _ in 0..<count {
            // A row leaving the top of the whole screen is a line of the console that has
            // gone by, and is kept. A scroll inside a region below the top is the client
            // drawing in part of the screen, and the lines it moves did not leave anything.
            if scrollTop == 0 && !isAltScreen {
                keepInScrollback(grid[0])
            }
            if scrollTop < scrollBottom {
                for y in scrollTop..<scrollBottom {
                    grid[y] = grid[y + 1]
                }
            }
            grid[scrollBottom] = blankRow()
        }
    }

    /// Moves the scroll region's rows down and blanks the row it leaves behind.
    public func scrollDown(_ count: Int) {
        guard count > 0 else { return }
        for _ in 0..<count {
            if scrollBottom > scrollTop {
                for y in stride(from: scrollBottom, to: scrollTop, by: -1) {
                    grid[y] = grid[y - 1]
                }
            }
            grid[scrollTop] = blankRow()
        }
    }

    /// Inserts blank rows at the cursor, pushing the region's rows below it down and off the
    /// bottom (CSI L). Outside the scroll region it does nothing.
    public func insertLines(_ count: Int) {
        guard count > 0, cursorY >= scrollTop, cursorY <= scrollBottom else { return }
        for _ in 0..<count {
            if cursorY < scrollBottom {
                for y in stride(from: scrollBottom, to: cursorY, by: -1) {
                    grid[y] = grid[y - 1]
                }
            }
            grid[cursorY] = blankRow()
        }
    }

    /// Deletes rows at the cursor, pulling the region's rows below it up (CSI M).
    public func deleteLines(_ count: Int) {
        guard count > 0, cursorY >= scrollTop, cursorY <= scrollBottom else { return }
        for _ in 0..<count {
            if cursorY < scrollBottom {
                for y in cursorY..<scrollBottom {
                    grid[y] = grid[y + 1]
                }
            }
            grid[scrollBottom] = blankRow()
        }
    }

    // MARK: - Erasing

    /// Erases part of the cursor's row: 0 to its end, 1 to its start, 2 all of it.
    public func eraseLine(mode: Int) {
        guard cursorY >= 0, cursorY < rows else { return }
        let blank = blankCell()
        let row = cursorY
        switch mode {
        case 0:
            if cursorX < cols {
                for x in max(0, cursorX)..<cols { grid[row][x] = blank }
            }
        case 1:
            if cursorX >= 0 {
                for x in 0...min(cursorX, cols - 1) { grid[row][x] = blank }
            }
        case 2:
            for x in 0..<cols { grid[row][x] = blank }
        default:
            break
        }
    }

    /// Erases part of the screen: 0 below the cursor, 1 above it, 2 or 3 all of it.
    public func eraseDisplay(mode: Int) {
        let blank = blankCell()
        switch mode {
        case 0:
            guard cursorY >= 0, cursorY < rows else { return }
            if cursorX < cols {
                for x in max(0, cursorX)..<cols { grid[cursorY][x] = blank }
            }
            for y in cursorY + 1..<rows {
                for x in 0..<cols { grid[y][x] = blank }
            }
        case 1:
            for y in 0..<min(cursorY, rows) {
                for x in 0..<cols { grid[y][x] = blank }
            }
            if cursorY >= 0, cursorY < rows, cursorX >= 0 {
                for x in 0...min(cursorX, cols - 1) { grid[cursorY][x] = blank }
            }
        case 2, 3:
            clearScreen()
        default:
            break
        }
    }

    /// Blanks the columns from the cursor to the right (CSI X).
    public func eraseChars(count: Int) {
        guard cursorY >= 0, cursorY < rows, count > 0 else { return }
        let blank = blankCell()
        // The row's end stops the erase: a count past it is the client erasing to the margin
        // and not a walk of its own, so the columns it does not have are not visited at all.
        var x = max(0, cursorX)
        var remaining = count
        while remaining > 0 && x < cols {
            grid[cursorY][x] = blank
            x += 1
            remaining -= 1
        }
    }

    /// Deletes columns at the cursor, pulling the rest of the row left (CSI P).
    public func deleteChars(count: Int) {
        guard cursorY >= 0, cursorY < rows, count > 0 else { return }
        let blank = blankCell()
        let row = cursorY
        let end = cols - count
        if end > cursorX {
            for x in cursorX..<end { grid[row][x] = grid[row][x + count] }
        }
        for x in max(end, 0)..<cols where x >= cursorX {
            grid[row][x] = blank
        }
    }

    /// Inserts blank columns at the cursor, pushing the row's right along (CSI @).
    public func insertChars(count: Int) {
        guard cursorY >= 0, cursorY < rows, count > 0 else { return }
        let blank = blankCell()
        let row = cursorY
        if cursorX + count <= cols - 1 {
            for x in stride(from: cols - 1, through: cursorX + count, by: -1) {
                grid[row][x] = grid[row][x - count]
            }
        }
        for x in cursorX..<min(cursorX + count, cols) where x >= 0 {
            grid[row][x] = blank
        }
    }

    // MARK: - The scroll region and the screens

    /// Sets the rows the screen scrolls, 0-based and inclusive, exactly as `vterm.go` sets
    /// them from CSI r: the rows are clamped to the screen, a region with no height is
    /// refused in favor of the whole screen, and the cursor is homed with it.
    public func setScrollRegion(top: Int, bottom: Int) {
        let clampedTop = max(top, 0)
        let clampedBottom = min(bottom, rows - 1)
        if clampedTop < clampedBottom {
            scrollTop = clampedTop
            scrollBottom = clampedBottom
        } else {
            scrollTop = 0
            scrollBottom = rows - 1
        }
        setCursor(x: 0, y: 0)
    }

    /// Switches between the primary and alternate screens.
    ///
    /// Entering saves the primary screen and its cursor and clears a blank alternate one;
    /// leaving restores them. `saveCursor` is false for the legacy switches (?47h and
    /// ?1047h), which restore the grid but leave the cursor where the alternate screen left
    /// it. A switch to the screen already in use does nothing, so entering the alternate
    /// screen twice does not overwrite the screen saved the first time.
    public func setAltScreen(_ on: Bool, saveCursor: Bool) {
        guard on != isAltScreen else { return }
        if on {
            savedGrid = grid
            savedCursorX = cursorX
            savedCursorY = cursorY
            clearScreen()
            cursorX = 0
            cursorY = 0
            wrapPending = false
        } else {
            if let saved = savedGrid {
                grid = saved
                savedGrid = nil
                if saveCursor {
                    cursorX = savedCursorX
                    cursorY = savedCursorY
                }
            } else {
                clearScreen()
                cursorX = 0
                cursorY = 0
            }
            wrapPending = false
        }
        isAltScreen = on
    }

    // MARK: - Size

    /// Changes the screen's size, keeping the upper-left cells that still fit and blanking
    /// the ones the new size adds.
    ///
    /// A size that is not a screen is ignored rather than applied, as `vterm.go` ignores it:
    /// the child is told the size the surface actually has, so a zero can only come from a
    /// surface that has not been laid out yet.
    public func resize(cols newCols: Int, rows newRows: Int) {
        guard newCols > 0, newRows > 0 else { return }
        grid = Self.resized(grid, cols: newCols, rows: newRows)
        cols = newCols
        rows = newRows
        scrollTop = 0
        scrollBottom = rows - 1
        clampCursor()
        // A saved primary screen is resized with the screen it will be restored onto, so that
        // leaving the alternate screen after a resize still fits.
        if let saved = savedGrid {
            savedGrid = Self.resized(saved, cols: newCols, rows: newRows)
        }
    }

    /// Returns the screen to the state a fresh one of the same size has.
    ///
    /// It is what a session that starts over needs: the grid, the scrollback and the count
    /// of lines that have left it all belong to the client that has gone, and leaving any of
    /// them behind would put the previous session's output under the new one's first frame.
    public func reset() {
        grid = Self.blankGrid(cols: cols, rows: rows, pen: .plain)
        scrollback.removeAll()
        linesScrolledOff = 0
        pen = .plain
        cursorX = 0
        cursorY = 0
        cursorVisible = true
        mouseReporting = false
        mouseSGR = false
        isAltScreen = false
        savedGrid = nil
        savedCursorX = 0
        savedCursorY = 0
        decSavedX = 0
        decSavedY = 0
        decSavedPen = .plain
        scrollTop = 0
        scrollBottom = rows - 1
        wrapPending = false
    }

    // MARK: - The grid

    /// Keeps a row that has left the top of the screen.
    ///
    /// It is called before the row shuffle, and that is what makes keeping the array itself
    /// safe: the shuffle only ever moves a row up and drops the top one, and the arrays are
    /// values, so the row handed over here is one no part of the grid holds any more and
    /// nothing will write to again.
    private func keepInScrollback(_ row: [TerminalCell]) {
        scrollback.append(row)
        linesScrolledOff += 1
        if scrollback.count > maxScrollback {
            scrollback.removeFirst(scrollback.count - maxScrollback)
        }
    }

    /// Blanks every cell of the screen the way `vterm.go`'s clearScreen does, in the pen the
    /// child is drawing with.
    private func clearScreen() {
        for y in 0..<rows {
            for x in 0..<cols {
                grid[y][x] = blankCell()
            }
        }
    }

    /// A space drawn with the pen in use: what an erase and a scroll leave.
    private func blankCell() -> TerminalCell {
        TerminalCell(glyph: " ", pen: pen, width: 1)
    }

    /// A fresh, blank row: what a scroll leaves behind and what a resize adds.
    private func blankRow() -> [TerminalCell] {
        Array(repeating: blankCell(), count: cols)
    }

    /// Keeps the cursor inside the screen.
    private func clampCursor() {
        if cursorX < 0 { cursorX = 0 }
        if cursorY < 0 { cursorY = 0 }
        if cursorX >= cols { cursorX = cols - 1 }
        if cursorY >= rows { cursorY = rows - 1 }
    }

    /// A grid of spaces: the state of a fresh screen and of every new row.
    private static func blankGrid(cols: Int, rows: Int, pen: TerminalPen) -> [[TerminalCell]] {
        let cell = TerminalCell(glyph: " ", pen: pen, width: 1)
        return Array(repeating: Array(repeating: cell, count: cols), count: rows)
    }

    /// Rebuilds a grid at a new size, preserving the upper-left overlap.
    ///
    /// A wide glyph whose spare column falls outside the new width is replaced by a space:
    /// half of a glyph is not a glyph, and a cell that claims a column it does not have would
    /// put every column after it out by one.
    private static func resized(_ grid: [[TerminalCell]], cols: Int, rows: Int) -> [[TerminalCell]] {
        var next = blankGrid(cols: cols, rows: rows, pen: .plain)
        for y in 0..<min(grid.count, rows) {
            for x in 0..<min(grid[y].count, cols) {
                let cell = grid[y][x]
                let split = cell.width > 1 && x + cell.width > cols
                next[y][x] = split ? .blank : cell
            }
        }
        return next
    }

    /// One row's characters, the spare columns of its wide glyphs left out.
    private static func text(of row: [TerminalCell]) -> String {
        row.reduce(into: "") { $0 += $1.glyph }
    }

    /// The pen an SGR sequence's parameters leave behind, applied to the pen in use.
    private static func pen(_ params: [String], appliedTo current: TerminalPen) -> TerminalPen {
        // A bare CSI m, or one with no parameters at all, is a reset.
        if params.isEmpty || (params.count == 1 && params[0].isEmpty) {
            return .plain
        }

        var foreground = current.foreground
        var background = current.background
        var bold = current.bold
        var italic = current.italic
        var underline = current.underline
        var blink = current.blink
        var reverse = current.reverse

        var i = 0
        while i < params.count {
            guard let n = csiIntOrNull(params[i]) else {
                i += 1
                continue
            }
            switch n {
            case 0: // the reset: no color and no attributes
                foreground = .default
                background = .default
                bold = false
                italic = false
                underline = false
                blink = false
                reverse = false
            case 1: bold = true
            case 2: bold = false
            case 3: italic = true
            case 4: underline = true
            case 5: blink = true
            case 7: reverse = true
            case 22: bold = false
            case 23: italic = false
            case 24: underline = false
            case 25: blink = false
            case 27: reverse = false
            case 30...37: foreground = .palette(n - 30)
            case 38, 48:
                if let parsed = sgrColor(params, at: i) {
                    if n == 38 { foreground = parsed.color } else { background = parsed.color }
                    i += parsed.consumed
                }
            case 39: foreground = .default
            case 40...47: background = .palette(n - 40)
            case 49: background = .default
            case 90...97: foreground = .palette(n - 90 + 8)
            case 100...107: background = .palette(n - 100 + 8)
            default: break
            }
            i += 1
        }

        return TerminalPen(
            foreground: foreground,
            background: background,
            bold: bold,
            italic: italic,
            underline: underline,
            blink: blink,
            reverse: reverse
        )
    }
}

/// A color an SGR sequence named, and how many parameters spelling it took.
private struct SgrColor {
    let color: TerminalColor
    let consumed: Int
}

/// Reads the color an SGR 38 or 48 introduces: `5;n` for a palette color, `2;r;g;b` for a
/// 24-bit one. It returns nil when the parameters do not spell a color, which leaves the
/// pen as it was rather than guessing.
private func sgrColor(_ params: [String], at i: Int) -> SgrColor? {
    guard i + 1 < params.count else { return nil }
    switch csiIntOrNull(params[i + 1]) {
    case 5:
        guard i + 2 < params.count, let index = csiIntOrNull(params[i + 2]) else { return nil }
        return SgrColor(color: .palette(index), consumed: 2)
    case 2:
        guard i + 4 < params.count else { return nil }
        let r = min(max(csiIntOrNull(params[i + 2]) ?? 0, 0), 255)
        let g = min(max(csiIntOrNull(params[i + 3]) ?? 0, 0), 255)
        let b = min(max(csiIntOrNull(params[i + 4]) ?? 0, 0), 255)
        return SgrColor(color: .rgb(r: r, g: g, b: b), consumed: 4)
    default:
        return nil
    }
}
