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

import CoreText
import TerminalKit
import UIKit

/// TerminalView draws the NomadNet client's terminal and turns fingers into keystrokes.
///
/// It is a `UIView` rather than a SwiftUI `Canvas` for one reason: the client's output
/// arrives at its own pace, in chunks that are often a single cell, and a view that
/// redraws only what changed is the difference between a console that keeps up and one
/// that does not. Core Text draws a row of monospace glyphs in one pass, which is what
/// makes a full redraw affordable in the first place.
///
/// It owns no client state. The screen, the parser and the session belong to
/// `TerminalController`; this view draws the screen and reports what the reader did to
/// it, so every decision about what the terminal means is testable without a device.
public final class TerminalView: UIView, UIKeyInput {
    /// The terminal's screen model, which the parser fills.
    public var screen: TerminalScreen

    /// The palette the screen is drawn in.
    public var palette: TerminalPalette = .shipped

    /// Called when the reader taps a cell, with the mouse report the client expects.
    public var onMouse: (([UInt8]) -> Void)?

    /// Called when the reader types, with the key the client expects.
    ///
    /// The key and not its bytes, because the modifier latches are the *pane's*: a `Ctrl` the
    /// reader tapped on the strip is shown as a lit button there, and the key that spends it
    /// has to spend the same one the strip is showing. A view with its own copy of the latch
    /// spent that instead — so a `Ctrl` then a letter left `Ctrl` lit on the strip, which is a
    /// latch a reader cannot tell is spent.
    public var onKey: ((TerminalKey) -> Void)?

    /// Called when the console's own scrollback moves, with the number of rows the view is
    /// now scrolled back from the live output. Zero means the view is at the newest line.
    public var onScrollback: ((Int) -> Void)?

    /// How many rows the reader has scrolled back from the live output.
    ///
    /// It is reset to zero by any new output, because a console that scrolls away from
    /// what just arrived is a console the reader cannot read.
    public private(set) var scrollOffset = 0

    /// The metrics the grid is laid out with, derived from the font.
    private var metrics: TerminalMetrics

    /// The gesture in flight, if any.
    private var gesture: TouchGesture

    /// The row the strip occupies at the top of the view, if the strip is showing.
    ///
    /// The strip is drawn by `KeyStripView`, which is a sibling; this view only has to
    /// know how much of its own top is covered, so that a touch there is not turned into
    /// a click on a row of the terminal that is not visible.
    public var coveredTop: CGFloat = 0

    private let font: UIFont
    private var blinkOn = true
    private var blinkTimer: Timer?

    /// The font the console is drawn in: the Nerd Font the appliance ships, which is what
    /// makes the client's glyph set render as intended rather than as boxes.
    ///
    /// A font that failed to register falls back to the system monospace, because a
    /// console in the wrong face is still a console.
    public static func consoleFont(size: CGFloat) -> UIFont {
        UIFont(name: "AtkinsonHyperlegibleMono-Regular", size: size)
            ?? UIFont(name: "AtkynsonMonoNerdFontMono-Regular", size: size)
            ?? UIFont.monospacedSystemFont(ofSize: size, weight: .regular)
    }

    /// Creates a terminal view for a screen and a font size.
    public init(frame: CGRect, screen: TerminalScreen, fontSize: CGFloat = 13) {
        self.screen = screen
        self.font = Self.consoleFont(size: fontSize)
        let cellWidth = ("M" as NSString).size(withAttributes: [.font: self.font]).width.rounded(.up)
        let cellHeight = self.font.lineHeight.rounded(.up)
        self.metrics = TerminalMetrics(
            cellWidth: max(1, Int(cellWidth)),
            cellHeight: max(1, Int(cellHeight)),
            cols: screen.cols,
            rows: screen.rows
        )
        self.gesture = TouchGesture(slop: 16, cellHeight: Double(max(1, Int(cellHeight))))
        super.init(frame: frame)
        backgroundColor = Self.color(TerminalPalette.defaultBackground)
        isOpaque = true
        isMultipleTouchEnabled = false
        // The console is a terminal, not a document: it has no scroll view of its own,
        // so every touch that is not the software keyboard's belongs to it.
        isUserInteractionEnabled = true
        // The device acceptance test drives this view and cannot name it by its content:
        // what it draws is cells, not text. The identifier is what lets that test tap the
        // console rather than guess at a coordinate, and the view has to be an
        // accessibility element for it to be findable at all — an identifier on a view
        // that is not one is an identifier nothing queries.
        isAccessibilityElement = true
        accessibilityIdentifier = "gonomadnet-console"
        // What it is, for anyone who reaches it without being able to see it: the cells
        // are drawn, not laid out as text, so there is nothing inside to read.
        accessibilityLabel = "gonomadnet console"
        accessibilityTraits = .updatesFrequently
        startBlinking()
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) { fatalError("the terminal is created in code") }

    deinit { blinkTimer?.invalidate() }

    // MARK: - Geometry

    /// relayout recalculates the grid for the view's current size.
    ///
    /// It is called when the view is laid out and when the key strip appears or
    /// disappears, because the strip takes rows from the terminal and the client has to
    /// be told: a client that is not told reflows its own content for a window it does
    /// not have.
    /// Called when the grid this view can show has changed, with the grid it can show now.
    ///
    /// The view cannot tell the client its window has changed — that belongs to the controller,
    /// which is the only thing that knows what the client currently believes — but the view is
    /// the only thing that knows when its own bounds have changed, so it reports rather than
    /// waits to be asked. Waiting to be asked is what the appliance did: SwiftUI calls a
    /// representable's `updateUIView` when its inputs change, and a window that has just been
    /// laid out is not one of those inputs, so the grid was computed once — often while the
    /// bounds were still empty — and never again. The client drew 80 columns into a 40-column
    /// pane for the whole session, with the last rows of the screen black, and no rotation could
    /// fix it because nothing was ever sent.
    public var onGridChanged: ((TerminalMetrics) -> Void)?

    /// relayout recomputes the grid this view can show for its current bounds, and returns it.
    ///
    /// It deliberately does **not** resize the screen. The screen is the client's window, and
    /// only the controller may change it: a view that resizes it has already answered the
    /// question the caller is about to ask — "has the client been told its new size?" — and the
    /// answer is always yes. The caller compares what this returns against the size the client
    /// was last told, and the controller is the one that acts on it. See `TerminalSizing`.
    @discardableResult
    public func relayout() -> TerminalMetrics {
        let available = bounds.height - coveredTop
        guard let new = TerminalMetrics.metrics(
            bounds: (width: Double(bounds.width), height: Double(available)),
            cellWidth: Double(metrics.cellWidth),
            cellHeight: Double(metrics.cellHeight),
            keyStripHeight: 0
        ) else { return metrics }

        guard new.cols != metrics.cols || new.rows != metrics.rows else { return metrics }
        metrics = new
        setNeedsDisplay()
        onGridChanged?(metrics)
        return metrics
    }

    /// layoutSubviews is where a window that has changed size is noticed.
    ///
    /// The layout the appliance was given arrives here and nowhere else: a rotation, a split
    /// view being dragged, or the key strip appearing all change this view's bounds, and UIKit
    /// is what calls this when they do.
    public override func layoutSubviews() {
        super.layoutSubviews()
        relayout()
    }

    // MARK: - Drawing

    public override func draw(_ rect: CGRect) {
        guard let context = UIGraphicsGetCurrentContext() else { return }

        context.setFillColor(Self.color(palette.background).cgColor)
        context.fill(rect)
        let firstRow = visibleFirstRow()
        for row in 0..<metrics.rows {
            let sourceRow = firstRow + row
            let y = coveredTop + CGFloat(row * metrics.cellHeight)
            draw(cells: cells(forSourceRow: sourceRow), at: y, in: context)
        }
        drawCursor(in: context)
    }

    /// visibleFirstRow is the first screen or scrollback row the view shows.
    ///
    /// A view scrolled back shows history above the live screen; at rest it shows the
    /// screen itself, and the most recent output is on the last row.
    private func visibleFirstRow() -> Int {
        if scrollOffset > 0 {
            return screen.scrollbackCount - scrollOffset
        }
        return screen.scrollbackCount
    }

    /// cells is the row of cells at a given index into scrollback-then-screen.
    ///
    /// Scrollback is addressed before the live screen because that is what the reader
    /// scrolls through: the history runs from the oldest line the appliance still holds
    /// to the newest line the client has drawn.
    private func cells(forSourceRow source: Int) -> [TerminalCell] {
        if source < screen.scrollbackCount {
            return screen.scrollbackLine(source)
        }
        let row = source - screen.scrollbackCount
        guard row < screen.rows else { return [] }
        return (0..<screen.cols).map { screen.cell(col: $0, row: row) }
    }

    /// draw renders one row of cells, its backgrounds first and its glyphs after.
    private func draw(cells: [TerminalCell], at y: CGFloat, in context: CGContext) {
        guard !cells.isEmpty else { return }

        let cellHeight = CGFloat(metrics.cellHeight)
        let cellWidth = CGFloat(metrics.cellWidth)

        // Backgrounds first, so a run of them never paints over a neighbor's glyph.
        for (col, cell) in cells.enumerated() where !cell.isSpacer {
            let (foregroundARGB, backgroundARGB) = palette.colors(of: cell.pen)
            guard backgroundARGB != palette.background else { continue }
            let run = runLength(of: cell, from: col, in: cells)
            context.setFillColor(Self.color(backgroundARGB).cgColor)
            context.fill(CGRect(
                x: CGFloat(col) * cellWidth,
                y: y,
                width: CGFloat(run) * cellWidth,
                height: cellHeight
            ))
            _ = foregroundARGB
        }

        // Then one attributed string per pen run, which is what keeps a row a handful of
        // Core Text calls rather than one per cell.
        var col = 0
        while col < cells.count {
            let cell = cells[col]
            if cell.isSpacer {
                col += 1
                continue
            }
            let run = runLength(of: cell, from: col, in: cells)
            let text = NSMutableAttributedString()
            for i in col..<(col + run) {
                let c = cells[i]
                text.append(NSAttributedString(string: c.glyph, attributes: attributes(for: c.pen)))
            }

            let line = CTLineCreateWithAttributedString(text)
            draw(line, in: context, cellLeft: CGFloat(col) * cellWidth, cellTop: y)

            if cell.pen.underline {
                let (foregroundARGB, _) = palette.colors(of: cell.pen)
                context.setFillColor(Self.color(foregroundARGB).cgColor)
                let thickness = max(1, cellHeight / 12)
                context.fill(CGRect(
                    x: CGFloat(col) * cellWidth,
                    y: y + font.ascender + thickness,
                    width: CGFloat(run) * cellWidth,
                    height: thickness
                ))
            }
            col += run
        }
    }

    /// draw renders one line of cells, standing up.
    ///
    /// The two coordinate systems disagree and this is where they are reconciled. UIKit
    /// hands a view a context whose y axis points down; Core Text draws glyphs with y
    /// pointing up. A line drawn with the identity text matrix is therefore mirrored — the
    /// appliance's whole client rendered upside down on the device, which is what a
    /// screenshot of the running appliance showed. Negating the text matrix's y is what
    /// makes the glyphs and the context agree, and once they do the baseline has to be
    /// named rather than inferred from the cell's floor: it sits one ascent below the cell's
    /// top, which is where a terminal puts it and what keeps a row's text inside its row.
    ///
    /// The position is in the context's own space, so it is not affected by the text matrix.
    private func draw(_ line: CTLine, in context: CGContext, cellLeft: CGFloat, cellTop: CGFloat) {
        context.textMatrix = CGAffineTransform(scaleX: 1, y: -1)
        context.textPosition = CGPoint(x: cellLeft, y: cellTop + font.ascender)
        CTLineDraw(line, context)
    }

    /// runLength is how many cells from an index share the cell's pen and glyph width,
    /// which is what a single Core Text call can draw.
    private func runLength(of cell: TerminalCell, from col: Int, in cells: [TerminalCell]) -> Int {
        var run = 0
        var index = col
        var width = 0
        while index < cells.count, width < cell.width {
            let next = cells[index]
            if next.isSpacer {
                width += 1
                index += 1
                run += 1
                continue
            }
            if next.pen != cell.pen || next.width != cell.width { break }
            width += next.width
            index += 1
            run += 1
        }
        return max(1, run)
    }

    /// attributes is how one cell's pen becomes text attributes.
    private func attributes(for pen: TerminalPen) -> [NSAttributedString.Key: Any] {
        let (foregroundARGB, _) = palette.colors(of: pen)
        var font = self.font
        var traits: UIFontDescriptor.SymbolicTraits = []
        if pen.bold { traits.insert(.traitBold) }
        if pen.italic { traits.insert(.traitItalic) }
        if !traits.isEmpty, let descriptor = font.fontDescriptor.withSymbolicTraits(traits) {
            font = UIFont(descriptor: descriptor, size: self.font.pointSize)
        }
        return [
            .font: font,
            .foregroundColor: Self.color(foregroundARGB),
        ]
    }

    /// drawCursor draws the cursor as a block in reverse video, which is how the client's
    /// own terminals draw it and therefore what a reader recognizes.
    private func drawCursor(in context: CGContext) {
        guard screen.cursorVisible, blinkOn, scrollOffset == 0 else { return }
        guard screen.cursorY < metrics.rows, screen.cursorX < metrics.cols else { return }

        let cellWidth = CGFloat(metrics.cellWidth)
        let cellHeight = CGFloat(metrics.cellHeight)
        let rect = CGRect(
            x: CGFloat(screen.cursorX) * cellWidth,
            y: coveredTop + CGFloat(screen.cursorY) * cellHeight,
            width: cellWidth,
            height: cellHeight
        )
        context.setFillColor(Self.color(palette.foreground).cgColor)
        context.fill(rect)

        let cell = screen.cell(col: screen.cursorX, row: screen.cursorY)
        let text = NSAttributedString(string: cell.glyph, attributes: [
            .font: font,
            .foregroundColor: Self.color(palette.background),
        ])
        let line = CTLineCreateWithAttributedString(text)
        draw(line, in: context, cellLeft: rect.minX, cellTop: rect.minY)
    }

    /// startBlinking alternates the cursor, which is what tells a reader where their
    /// typing will land when the terminal is otherwise still.
    private func startBlinking() {
        blinkTimer?.invalidate()
        blinkTimer = Timer.scheduledTimer(withTimeInterval: 0.5, repeats: true) { [weak self] _ in
            Task { @MainActor in
                guard let self else { return }
                self.blinkOn.toggle()
                self.setNeedsDisplay()
            }
        }
    }

    // MARK: - Output

    /// screenChanged is called when the client has drawn something.
    ///
    /// Any new output returns the view to the live end: a console that stays scrolled
    /// back while the client is producing output is a console showing the reader
    /// something that is no longer true.
    public func screenChanged() {
        if scrollOffset != 0 {
            scrollOffset = 0
            onScrollback?(0)
        }
        setNeedsDisplay()
    }

    // MARK: - Touch

    public override func touchesBegan(_ touches: Set<UITouch>, with event: UIEvent?) {
        guard let touch = touches.first else { return }
        let point = touch.location(in: self)
        if point.y < coveredTop { return }
        gesture.down(x: Double(point.x), y: Double(point.y - coveredTop))
    }

    public override func touchesMoved(_ touches: Set<UITouch>, with event: UIEvent?) {
        guard let touch = touches.first else { return }
        let point = touch.location(in: self)
        let lines = gesture.move(x: Double(point.x), y: Double(point.y - coveredTop))
        guard lines != 0 else { return }
        wheel(lines, x: Double(point.x), y: Double(point.y - coveredTop))
    }

    public override func touchesEnded(_ touches: Set<UITouch>, with event: UIEvent?) {
        switch gesture.up() {
        case .tap(let x, let y):
            tap(x: x, y: y)
        case .none:
            break
        }
    }

    public override func touchesCancelled(_ touches: Set<UITouch>, with event: UIEvent?) {
        _ = gesture.up()
    }

    /// wheel reports a drag as mouse wheel notches at the cell the finger is on, or scrolls the
    /// console's own history when the client is not listening for a mouse.
    ///
    /// **The cell is the finger's, not the cursor's**, and that is the whole of whether a drag
    /// does anything. A client routes a wheel report to whatever it has under the cell, which
    /// is how a reader scrolls the panel they are dragging — a page, a room's messages, a
    /// guide. Reported at the cursor instead, the wheel arrives at whichever panel holds the
    /// focus, which on the NomadNet client is the menu bar: it scrolls nothing at all, and a
    /// drag anywhere on the screen looks like a console that cannot be dragged.
    ///
    /// A console scrolled away from the live output is a console the reader has taken
    /// over, so the client is not told about the drag at all: sending it both a wheel
    /// report and a scroll would move the client's own view and this one.
    private func wheel(_ lines: Int, x: Double, y: Double) {
        if screen.mouseReporting, scrollOffset == 0 {
            let button: TerminalMouseButton = lines > 0 ? .wheelDown : .wheelUp
            let notches = min(abs(lines), TouchGesture.maxWheelNotches)
            let (col, row) = metrics.pixelToCell(x: x, y: y)
            for _ in 0..<notches {
                onMouse?(TerminalMouse.sgr(button: button, col: col, row: row, pressed: true))
            }
            return
        }

        // Positive is a finger moving up the screen, which moves the window towards the
        // newest line; the lines above it are reached by dragging down.
        let wanted = scrollOffset - lines
        let clamped = max(0, min(screen.scrollbackCount, wanted))
        guard clamped != scrollOffset else { return }
        scrollOffset = clamped
        onScrollback?(clamped)
        setNeedsDisplay()
    }

    /// tap clicks the cell the reader touched, which is how a link, a menu entry or a
    /// button is activated.
    ///
    /// The client makes a click out of a press *and* a release in the same cell, so both
    /// are sent; a console that is not listening for a mouse is not sent either.
    private func tap(x: Double, y: Double) {
        // The keyboard is taken whatever the console's own state is: a tap is a reader asking to
        // type into the console they are looking at, and one who has dragged the window back
        // into the scrollback — or whose client is not listening for a mouse — has no other way
        // to be typed into at all.
        becomeFirstResponder()
        guard screen.mouseReporting, scrollOffset == 0 else { return }
        let (col, row) = metrics.pixelToCell(x: x, y: y)
        onMouse?(TerminalMouse.sgr(button: .left, col: col, row: row, pressed: true))
        onMouse?(TerminalMouse.sgr(button: .left, col: col, row: row, pressed: false))
    }

    // MARK: - Keyboard

    /// didMoveToWindow takes the keyboard for the console as soon as there is a window to take
    /// it in.
    ///
    /// A key goes to the first responder. A hardware keyboard's keys arrive there, and the
    /// software keyboard is served to it; a console that is drawn but is not the first responder
    /// therefore receives nothing at all. The client's interface is reached with keys — its
    /// lists, its buttons, its menu, and the shortcuts on its last row — so a console that is not
    /// the first responder is the client's whole interface going quiet, with nothing on the
    /// screen to say why. The pane is the console, and the console is what is typed into, so it
    /// takes the keyboard the moment it has a window.
    ///
    /// With a hardware keyboard attached the system presents no software keyboard for it, so the
    /// pane looks exactly as it did and the keys work; without one the keyboard comes up at once,
    /// and the key strip that accompanies it, which is the state a tap on the console reaches.
    public override func didMoveToWindow() {
        super.didMoveToWindow()
        if window != nil, !isFirstResponder {
            becomeFirstResponder()
        }
    }

    /// Sends a key the console's own keyboard produced, which is the pane's to spend.
    ///
    /// The latches are the pane's, and it is the pane that applies them, because the next key
    /// may come from anywhere: a reader who taps `Ctrl` and then presses `c` on a Magic Keyboard
    /// means the same thing as one who taps `Ctrl` and then `c` on the strip, and the strip has
    /// to stop showing `Ctrl` as lit either way.
    public func send(key: TerminalKey) {
        onKey?(key)
    }

    public override var canBecomeFirstResponder: Bool { true }

    public var hasText: Bool { false }

    public func insertText(_ text: String) {
        for character in text {
            send(key: .rune(String(character)))
        }
    }

    public func deleteBackward() {
        send(key: .backspace)
    }

    public override func pressesBegan(_ presses: Set<UIPress>, with event: UIPressesEvent?) {
        var handled = false
        for press in presses {
            guard let key = press.key, let mapped = Self.key(for: key) else { continue }
            send(key: mapped)
            handled = true
        }
        if !handled { super.pressesBegan(presses, with: event) }
    }

    /// key maps a hardware key to the terminal's own key.
    ///
    /// A key with no terminal meaning is left unmapped and reaches the rest of the app:
    /// a Magic Keyboard's volume keys and the hardware Escape are not the terminal's to
    /// swallow.
    static func key(for key: UIKey) -> TerminalKey? {
        switch key.keyCode {
        case .keyboardUpArrow: return .up
        case .keyboardDownArrow: return .down
        case .keyboardLeftArrow: return .left
        case .keyboardRightArrow: return .right
        case .keyboardEscape: return .escape
        case .keyboardTab: return .tab
        case .keyboardHome: return .home
        case .keyboardEnd: return .end
        case .keyboardPageUp: return .pageUp
        case .keyboardPageDown: return .pageDown
        case .keyboardDeleteForward: return .delete
        case .keyboardDeleteOrBackspace: return .backspace
        case .keyboardReturnOrEnter: return .enter
        default: break
        }
        guard !key.characters.isEmpty else { return nil }
        return .rune(key.characters)
    }

    // MARK: - Color

    /// color turns a packed ARGB value into the `UIColor` the drawing system wants.
    static func color(_ argb: UInt32) -> UIColor {
        UIColor(
            red: CGFloat((argb >> 16) & 0xFF) / 255,
            green: CGFloat((argb >> 8) & 0xFF) / 255,
            blue: CGFloat(argb & 0xFF) / 255,
            alpha: CGFloat((argb >> 24) & 0xFF) / 255
        )
    }
}
