// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package com.gmlewis.gonomadnet

import android.content.Context
import android.graphics.Canvas
import android.graphics.Paint
import android.graphics.Typeface
import android.os.Build
import android.text.InputType
import android.util.AttributeSet
import android.util.TypedValue
import android.view.KeyEvent
import android.view.MotionEvent
import android.view.View
import android.view.ViewConfiguration
import android.view.inputmethod.BaseInputConnection
import android.view.inputmethod.EditorInfo
import android.view.inputmethod.InputConnection
import android.view.inputmethod.InputMethodManager
import kotlin.math.abs
import kotlin.math.ceil
import kotlin.math.max
import kotlin.math.roundToInt

/**
 * The console, drawn a cell at a time.
 *
 * The appliance runs the client on a pseudo-terminal and is therefore the terminal it draws
 * through, and this is the drawing: a monospace grid of the screen [TerminalScreen] holds,
 * scrolled back through the lines it has kept, with the keys a tablet has no way to press
 * along the bottom edge.
 *
 * Everything here is a thin skin over the tested parts. What to draw is
 * [TerminalScreen]'s; how many columns and rows the view holds, where a cell lands, which
 * line the window shows and what the key row offers are [TerminalLayout]'s; what colour a
 * cell is, [TerminalPalette]'s; what a finger meant when it touched the console,
 * [TouchGesture]'s; and how a tap is spelled for the client, [TerminalMouse]'s. What is left
 * — and all this file contains — is the calls into `Canvas` and the touches that come back
 * out of it, neither of which a JVM test can reach. Nothing here may be claimed as working
 * until it has been looked at on the tablet.
 *
 * The view never starts or talks to the client itself. It reports a key press through
 * [onKey], a mouse event through [onMouse] and a size change through [onResize], and is told
 * what happened through [screen] and [outputParsed], so the session's lifetime stays
 * [ConsoleSession]'s business and the drawing stays the UI thread's.
 */
class TerminalView(context: Context, attrs: AttributeSet? = null) : View(context, attrs) {

    companion object {
        /**
         * The font the console draws with, as it is named in the APK's assets.
         *
         * The client's interface is drawn with Nerd Font glyphs — its menu bar, its list
         * markers, its arrows — and the device's own monospace font has no code point for
         * any of them, so drawing with it would turn a working interface into rows of empty
         * boxes. This font is what the client's own screenshots are made of, and it travels
         * in the APK for exactly this reason.
         */
        const val CONSOLE_FONT = "AtkynsonMonoNerdFontMono-Regular.otf"

        /**
         * The character height, in density-independent pixels.
         *
         * Deliberately not scaled by the system font size. This is a terminal, and the
         * interface drawn into it is the client's, laid out for the columns it is told
         * about: a font scale that halved the columns would reflow someone else's windows
         * rather than make them easier to read.
         */
        private const val TEXT_DIP: Float = 13f

        /** BLINK_MS is how long the cursor rests in each of its two states. */
        private const val BLINK_MS: Long = 500L

        /** MAX_DELETIONS is the most backspaces one input-method edit can ask for. */
        private const val MAX_DELETIONS: Int = 64

        /**
         * MAX_WHEEL_NOTCHES is the most wheel reports one movement is sent as.
         *
         * A drag is delivered as a whole notch of wheel per cell it crossed, and one movement
         * of a fast finger can cross the whole screen: each notch is a write on the console
         * socket, so the burst is bounded rather than proportional to the screen.
         */
        private const val MAX_WHEEL_NOTCHES: Int = 48

        /**
         * consoleTypeface is the bundled console font, or the system's monospace font.
         *
         * A missing or unreadable asset is not worth taking the appliance down for: a
         * console drawn in the system font is a console with boxes where its icons are,
         * which is a great deal better than a screen that throws on the way up. The
         * fallback is also what lets a plain JVM test construct the view at all.
         */
        private fun consoleTypeface(context: Context): Typeface =
            runCatching { Typeface.createFromAsset(context.assets, CONSOLE_FONT) }.getOrNull()
                ?: Typeface.MONOSPACE
    }


    /** The screen to draw. Setting it starts a fresh look at the newest line. */
    var screen: TerminalScreen? = null
        set(value) {
            field = value
            scrollOffset = 0
            lastScrolledOff = value?.linesScrolledOff ?: 0L
            invalidate()
        }

    /** onKey reports a key press, which the session turns into what the client reads. */
    var onKey: ((TerminalKey) -> Unit)? = null

    /**
     * onMouse reports a mouse event as the bytes it is sent as, which the session writes to
     * the client's terminal.
     *
     * It is bytes rather than a report of its own because the spelling is the terminal's —
     * see [TerminalMouse] — and the decision to send one at all is the view's, which is what
     * knows whether the client asked to be told about the mouse.
     */
    var onMouse: ((ByteArray) -> Unit)? = null

    /**
     * onResize reports the size the view can hold, which the client has to be told.
     *
     * It fires when the grid changes, which is what a rotation is: the client lays itself
     * out for the columns and rows it is given, and a client that is not told is a
     * reflowed window with its last columns cut off.
     */
    var onResize: ((TerminalGrid) -> Unit)? = null

    // What one character measures, and the grid that fits in the view. Both are worked out
    // in onSizeChanged, because until the view has been laid out there is no rectangle to
    // fit anything into.
    private val textPaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        typeface = consoleTypeface(context)
        textSize = TypedValue.applyDimension(
            TypedValue.COMPLEX_UNIT_DIP,
            TEXT_DIP,
            resources.displayMetrics,
        )
    }
    private var metrics: TerminalMetrics? = null
    private var grid: TerminalGrid = TerminalGrid(cols = 80, rows = 24)

    // How far the window has been dragged back from the newest line, and how much output
    // had arrived when it was last looked at — the two together are what keeps the text
    // under a reader's eyes from moving while they read it.
    private var scrollOffset = 0
    private var lastScrolledOff = 0L

    // Control and Alt, which a tablet cannot hold down — see [ModifierLatch] — and the
    // state of the cursor's blink.
    private val modifiers = ModifierLatch()
    private var cursorVisible = true

    /**
     * keysVisible is whether the on-screen keys are shown, which follows the soft keyboard.
     *
     * The strip is the soft keyboard's companion: it comes up with the keyboard and goes with
     * it, so a reader who is not typing has the client's whole screen. The keyboard is not the
     * view's business — the page's window insets are what know about it — so this is set from
     * outside, and setting it is a resize: the rows it takes are the client's rows.
     */
    var keysVisible: Boolean = false
        set(value) {
            if (field == value) return
            field = value
            updateGrid()
            invalidate()
        }

    /**
     * collapsed is whether the reader has folded the key strip away.
     *
     * The strip costs rows, and the rows are the client's: folding it hands them back, which
     * is why it is a resize and not just a redraw.
     */
    private var stripCollapsed = false
    private val blink = object : Runnable {
        override fun run() {
            cursorVisible = !cursorVisible
            invalidate()
            postDelayed(this, BLINK_MS)
        }
    }

    // The finger on the console, which remembers where it went down and whether it has
    // travelled far enough since to have been a scroll rather than a tap.
    private val touch = TouchGesture(ViewConfiguration.get(context).scaledTouchSlop.toFloat())

    init {
        // The view draws itself rather than being a background to something else, so it has
        // to be told that: a View that reports no background is otherwise skipped entirely.
        setWillNotDraw(false)
        // It also carries its own background, in the console's own colour. A view with no
        // background shows whatever is behind it until its first frame, which for a terminal
        // is a flash of the window's colour on every rotation.
        setBackgroundColor(TerminalPalette.DEFAULT_BACKGROUND)
        // And the framework paints a translucent highlight over a focused view that has no
        // background of its own — on top of everything the view draws, so the whole console
        // is washed toward white by about a sixth. The console is focused only to be typed
        // into, and it is never a thing to be highlighted: what it shows is the client's
        // screen, and a screen that changes colour because the tablet was driven by a key
        // rather than a finger is a screen that is lying about what it is drawing.
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            setDefaultFocusHighlightEnabled(false)
        }
        isFocusable = true
        isFocusableInTouchMode = true
    }

    // ------------------------------------------------------------------ the session's side

    /**
     * outputParsed is what the session calls after the client's bytes have reached [screen].
     *
     * A reader at the bottom stays at the bottom; a reader up in the scrollback keeps the
     * lines they are reading, which means the window's offset grows by exactly as much as
     * arrived. The count comes from the screen's monotonic total rather than from the
     * scrollback's size, which stops growing once the scrollback is full.
     *
     * The pump runs on a thread of its own, so the work is handed to the UI thread.
     */
    fun outputParsed() {
        post { reanchor() }
    }

    private fun reanchor() {
        val screen = screen ?: return
        val added = (screen.linesScrolledOff - lastScrolledOff).coerceIn(0L, Int.MAX_VALUE.toLong()).toInt()
        lastScrolledOff = screen.linesScrolledOff
        scrollOffset = TerminalScroll.offsetAfterLines(scrollOffset, added, maxScrollOffset())
        invalidate()
    }

    /** maxScrollOffset is how far back the console can be dragged. */
    private fun maxScrollOffset(): Int {
        val screen = screen ?: return 0
        return TerminalScroll.maxOffset(screen.historySize + screen.rows, visibleRows())
    }

    private fun visibleRows(): Int = grid.rows

    // -------------------------------------------------------------------------- measuring

    override fun onSizeChanged(width: Int, height: Int, oldWidth: Int, oldHeight: Int) {
        super.onSizeChanged(width, height, oldWidth, oldHeight)
        val measured = measureCell(width, height)
        metrics = measured
        if (measured == null) {
            // A font that measures to nothing is not a terminal, and every later division
            // would be by zero. Nothing is drawn until a size arrives that can hold a cell.
            return
        }
        updateGrid()
        invalidate()
    }

    /**
     * updateGrid is the terminal size the view now has room for, reported when it changes.
     *
     * It is called for a new rectangle — a rotation, or the keyboard arriving — and for a
     * strip that has been folded away, because folding it gives its rows back to the client.
     * The session is told once the grid actually changed, so a rotation is one resize to the
     * client rather than one per layout pass.
     */
    private fun updateGrid() {
        val measured = metrics ?: return
        if (width <= 0 || height <= 0) return
        val wanted = measured.gridFor(width, height - keyRowHeight(measured))
        scrollOffset = scrollOffset.coerceIn(0, TerminalScroll.maxOffset(screenTotalLines(), wanted.rows))
        if (wanted != grid) {
            grid = wanted
            onResize?.invoke(wanted)
        }
    }

    /**
     * stripRows is the key strip as it stands: two rows of keys, one folded row, or none.
     *
     * None is the ordinary state. The keys are there for typing, so they are there when the
     * keyboard is: a console with no keyboard is a client with the whole screen.
     */
    private fun stripRows(): List<List<KeyRowEntry>> =
        if (!keysVisible) emptyList() else TerminalKeyRow.rows(stripCollapsed)

    /**
     * measureCell is what one character of the console measures.
     *
     * The width is rounded up: half a pixel more than the font's advance is a gap, and half
     * a pixel less is two characters drawn on top of each other.
     */
    private fun measureCell(width: Int, height: Int): TerminalMetrics? {
        if (width <= 0 || height <= 0) return null
        val cellWidth = ceil(textPaint.measureText("M")).toInt()
        val cellHeight = ceil(textPaint.fontSpacing).toInt()
        if (cellWidth <= 0 || cellHeight <= 0) return null
        return TerminalMetrics(cellWidth, cellHeight)
    }

    private fun screenTotalLines(): Int = screen?.let { it.historySize + it.rows } ?: 0

    /** keyRowHeight is how much of the bottom edge the on-screen keys take. */
    private fun keyRowHeight(metrics: TerminalMetrics): Int =
        TerminalKeyRow.height(stripRows(), metrics.cellHeight)

    private fun keyRowPadding(metrics: TerminalMetrics): Int = TerminalKeyRow.padding(metrics.cellHeight)

    /** keyRowTop is the y the key strip starts at, or the view's height when there is none. */
    private fun keyRowTop(metrics: TerminalMetrics): Int =
        (height - keyRowHeight(metrics)).coerceAtLeast(0)

    // --------------------------------------------------------------------------- drawing

    override fun onDraw(canvas: Canvas) {
        super.onDraw(canvas)
        canvas.drawColor(TerminalPalette.DEFAULT_BACKGROUND)

        val screen = screen ?: return
        val metrics = metrics ?: return
        val font = textPaint.fontMetrics

        val first = TerminalScroll.firstVisibleLine(
            totalLines = screen.historySize + screen.rows,
            visibleRows = visibleRows(),
            offset = scrollOffset,
        )
        for (row in 0 until visibleRows()) {
            val line = first + row
            if (line >= screen.historySize + screen.rows) break
            val top = metrics.pixelY(row)
            for (col in 0 until minOf(grid.cols, screen.cols)) {
                drawCell(canvas, metrics, font, cellOf(screen, line, col), col, top)
            }
        }

        drawCursor(canvas, screen, metrics, font, first)
        drawKeyRow(canvas, metrics)
    }

    /** cellOf is one cell of the console: a kept line above, the screen itself below. */
    private fun cellOf(screen: TerminalScreen, line: Int, col: Int): TerminalCell =
        if (line < screen.historySize) screen.historyCell(col, line) else screen.cellAt(col, line - screen.historySize)

    private fun drawCell(
        canvas: Canvas,
        metrics: TerminalMetrics,
        font: Paint.FontMetrics,
        cell: TerminalCell,
        col: Int,
        top: Int,
    ) {
        // The cursor is drawn over the cell it rests on, so the cell is drawn normally and
        // the cursor is what covers it.
        val left = metrics.pixelX(col)
        val colors = TerminalPalette.colorsOf(cell.pen)

        // The background is filled only where it is not the console's own: a colour per
        // cell over the whole screen is three thousand rectangles to say what the cleared
        // canvas already said.
        if (colors.background != TerminalPalette.DEFAULT_BACKGROUND) {
            textPaint.color = colors.background
            canvas.drawRect(
                left.toFloat(),
                top.toFloat(),
                (left + metrics.cellWidth).toFloat(),
                (top + metrics.cellHeight).toFloat(),
                textPaint,
            )
        }

        // A wide glyph's spare column carries nothing: the glyph itself is drawn whole from
        // the column before it, and drawing the empty string there would only be a no-op.
        if (cell.width == 0 || cell.glyph.isEmpty()) return

        textPaint.color = colors.foreground
        // Bold and italic are drawn as a real font would not be available in every
        // monospace family, so they are synthesized the way a terminal synthesizes them:
        // a faked weight and a skewed glyph, which never change the cell's width.
        textPaint.isFakeBoldText = cell.pen.bold
        textPaint.textSkewX = if (cell.pen.italic) -0.25f else 0f
        canvas.drawText(cell.glyph, left.toFloat(), top + -font.ascent, textPaint)
        if (cell.pen.underline) {
            val y = top + metrics.cellHeight - max(1, metrics.cellHeight / 12)
            canvas.drawRect(left.toFloat(), y.toFloat(), (left + metrics.cellWidth).toFloat(), (y + 1).toFloat(), textPaint)
        }
        // The attributes belong to this cell and are set on a paint that outlives it.
        textPaint.isFakeBoldText = false
        textPaint.textSkewX = 0f
    }

    /**
     * drawCursor draws the block the client's cursor rests on.
     *
     * A block rather than an outline, drawn in the two colours a reverse-video cell is
     * drawn in, which is what makes it visible over both dark and light text. It is left
     * out while the client has hidden its cursor, and while it is scrolled out of view:
     * a reader up in the scrollback is looking at history the cursor is not on.
     */
    private fun drawCursor(
        canvas: Canvas,
        screen: TerminalScreen,
        metrics: TerminalMetrics,
        font: Paint.FontMetrics,
        first: Int,
    ) {
        if (!cursorVisible || !screen.cursorVisible) return
        val row = screen.historySize + screen.cursorY - first
        if (row < 0 || row >= visibleRows() || screen.cursorX >= minOf(grid.cols, screen.cols)) return

        val cell = cellOf(screen, first + row, screen.cursorX)
        val colors = TerminalPalette.colorsOf(cell.pen.copy(reverse = !cell.pen.reverse))
        val left = metrics.pixelX(screen.cursorX)
        val top = metrics.pixelY(row)

        textPaint.isFakeBoldText = false
        textPaint.textSkewX = 0f
        textPaint.color = colors.background
        canvas.drawRect(
            left.toFloat(),
            top.toFloat(),
            (left + metrics.cellWidth).toFloat(),
            (top + metrics.cellHeight).toFloat(),
            textPaint,
        )
        if (cell.glyph.isNotEmpty() && cell.width != 0) {
            textPaint.color = colors.foreground
            canvas.drawText(cell.glyph, left.toFloat(), top + -font.ascent, textPaint)
        }
    }

    /**
     * drawKeyRow draws the keys along the bottom edge.
     *
     * The buttons share each row's width equally, which is the division
     * [TerminalKeyRow.entryAt] undoes for a tap, so what is drawn and what is hit are the
     * same arithmetic. A modifier is drawn differently while it is armed: a latch that
     * cannot be seen is one a reader leaves set and then cannot explain the next key press.
     */
    private fun drawKeyRow(canvas: Canvas, metrics: TerminalMetrics) {
        val rows = stripRows()
        // No keys and no rule over the client's last row: a console with nothing to type into
        // is the client's whole screen, and a line across it is a line the client did not draw.
        if (rows.isEmpty()) return
        val padding = keyRowPadding(metrics)
        val top = keyRowTop(metrics)
        if (height - top <= 0 || width <= 0) return

        textPaint.isFakeBoldText = false
        textPaint.textSkewX = 0f
        textPaint.color = TerminalPalette.DEFAULT_FOREGROUND
        canvas.drawRect(0f, top.toFloat(), width.toFloat(), (top + 1).toFloat(), textPaint)

        for ((rowIndex, entries) in rows.withIndex()) {
            val rowTop = top + padding + rowIndex * metrics.cellHeight
            val textY = rowTop + -textPaint.fontMetrics.ascent
            val share = width.toFloat() / entries.size
            for ((index, entry) in entries.withIndex()) {
                val left = index * share
                if (modifierOf(entry)) {
                    textPaint.color = TerminalPalette.DEFAULT_FOREGROUND
                    canvas.drawRect(
                        left,
                        rowTop.toFloat(),
                        left + share,
                        (rowTop + metrics.cellHeight).toFloat(),
                        textPaint,
                    )
                    textPaint.color = TerminalPalette.DEFAULT_BACKGROUND
                }
                val labelWidth = textPaint.measureText(entry.label)
                canvas.drawText(entry.label, left + (share - labelWidth) / 2f, textY, textPaint)
                textPaint.color = TerminalPalette.DEFAULT_FOREGROUND
            }
        }
    }

    /** modifierOf is whether a button is a modifier whose latch is currently armed. */
    private fun modifierOf(entry: KeyRowEntry): Boolean = when (entry) {
        is KeyRowEntry.Control -> modifiers.ctrl
        is KeyRowEntry.Alt -> modifiers.alt
        else -> false
    }

    // --------------------------------------------------------------------------- touching

    override fun onTouchEvent(event: MotionEvent): Boolean {
        val metrics = metrics ?: return false
        when (event.actionMasked) {
            MotionEvent.ACTION_DOWN -> touch.down(event.x, event.y, keyRowTop(metrics))

            MotionEvent.ACTION_MOVE -> scrollOrWheel(event.x, event.y, touch.move(event.x, event.y, metrics.cellHeight))

            MotionEvent.ACTION_UP -> when (val outcome = touch.up(event.x, event.y)) {
                is TouchOutcome.KeyRow -> pressKeyRow(outcome.x, outcome.y)
                is TouchOutcome.Console -> tapConsole(outcome.x, outcome.y)
                TouchOutcome.None -> Unit
            }

            MotionEvent.ACTION_SCROLL -> {
                // A wheel or a trackpad, which reports the scroll as a signed count rather
                // than as a distance: up and back into the scrollback is positive. It is
                // rounded, because a wheel that reported a fraction of a step would
                // otherwise be a gesture that scrolled nothing at all.
                val lines = event.getAxisValue(MotionEvent.AXIS_VSCROLL).roundToInt()
                if (lines != 0) scrollOrWheel(event.x, event.y, lines)
            }
        }
        return true
    }

    /**
     * tapConsole is what a tap on the console itself sends, and what it asks for.
     *
     * The soft keyboard is asked for, because the console is a thing to be typed into and a
     * reader who has to find a menu item to type has been given a terminal with no way in.
     *
     * The client is told about the tap as well, as the mouse report a real terminal would
     * have sent it — for the same reason a real terminal sends one: without it, the
     * interface the client draws is a screen that can be read and not touched, and every
     * entry in its lists, every button and every menu is unreachable. It is sent only when
     * the client has asked to be told about the mouse (see [TerminalScreen.mouseReporting]):
     * a report it never asked for is a click it would act on at a cell nobody touched.
     *
     * Nor is one sent while the window is back in the scrollback. What the client draws is
     * the screen at the newest line, and a click on a line above it would land on whatever
     * the client has at that row rather than on the line the reader is looking at.
     */
    private fun tapConsole(x: Int, y: Int) {
        val screen = screen
        val metrics = metrics
        if (screen != null && metrics != null && screen.mouseReporting && scrollOffset == 0) {
            val report = MouseReport(
                button = MouseButton.LEFT,
                col = (x / metrics.cellWidth).coerceIn(0, screen.cols - 1),
                row = (y / metrics.cellHeight).coerceIn(0, screen.rows - 1),
                down = true,
            )
            // A press and a release in the same cell, which together are what the client
            // makes a click out of: a press alone moves its focus and activates nothing.
            onMouse?.invoke(TerminalMouse.encode(report, screen.mouseSgr))
            onMouse?.invoke(TerminalMouse.encode(report.copy(down = false), screen.mouseSgr))
        }
        requestFocus()
        showSoftKeyboard()
    }

    /** pressKeyRow is what a tap on the key strip sends, or which button it works. */
    private fun pressKeyRow(x: Int, y: Int) {
        val metrics = metrics ?: return
        val rows = stripRows()
        // The keyboard can go away between the finger going down and coming up, which takes
        // the keys with it: there is then no button under the finger to have pressed.
        if (rows.isEmpty()) return
        val entry = TerminalKeyRow.entryAt(
            x = x,
            y = y,
            width = width,
            stripTop = keyRowTop(metrics),
            rowHeight = metrics.cellHeight,
            rows = rows,
        )
        when (entry) {
            is KeyRowEntry.Control -> {
                modifiers.toggleCtrl()
                invalidate()
            }

            is KeyRowEntry.Alt -> {
                modifiers.toggleAlt()
                invalidate()
            }

            is KeyRowEntry.Toggle -> foldStrip(!stripCollapsed)
            is KeyRowEntry.Press -> send(entry.key)
        }
    }

    /**
     * foldStrip folds the key strip away, or brings it back, and hands the rows over with it.
     *
     * The strip is drawn over the terminal, so the rows it costs are rows the client does not
     * have: folding it is a resize the client is told about, and unfolding it is another.
     */
    private fun foldStrip(collapsed: Boolean) {
        stripCollapsed = collapsed
        updateGrid()
        invalidate()
    }

    /**
     * send hands one key to the session, after the modifier latches have had their say.
     *
     * Every key comes through here — the strip's buttons, the soft keyboard, and a hardware
     * keyboard — because the latches are the console's and not the strip's: a person arms
     * Control with a tap and then types a letter wherever they like.
     */
    private fun send(key: TerminalKey) {
        val wasArmed = modifiers.ctrl || modifiers.alt
        onKey?.invoke(modifiers.spend(key))
        // A modifier button is drawn differently while it is armed, so the strip is redrawn
        // when a latch changes — and only then, since a keystroke redraws the console
        // through the client's own output.
        if (wasArmed != (modifiers.ctrl || modifiers.alt)) {
            invalidate()
        }
    }

    /** scrollBy moves the window by [lines] lines, negative being towards the newest. */
    private fun scrollBy(lines: Int) {
        val wanted = (scrollOffset + lines).coerceIn(0, maxScrollOffset())
        if (wanted == scrollOffset) return
        scrollOffset = wanted
        invalidate()
    }

    /**
     * scrollOrWheel is what a drag, or a wheel, over the console asks for.
     *
     * A gesture across the client's interface is a scroll, and whose scroll it is depends on
     * what the client can do with one. A client that has asked to be told about the mouse is
     * given wheel reports at the cell the finger is on, so what scrolls is whatever it has
     * there — a guide, a page, a room's messages — which is the only scrolling a full-screen
     * client can be given from outside itself. A client that has asked for nothing is
     * scrolled by the console instead, through the lines it kept above the screen.
     *
     * [lines] is [TouchGesture.move]'s own: positive is a finger moving up the screen, which
     * asks for what is below the window — the next lines of a document and the older lines of
     * a console are the same gesture, so the wheel turns down and the window moves towards
     * the newest.
     */
    private fun scrollOrWheel(x: Float, y: Float, lines: Int) {
        if (lines == 0) return
        val screen = screen
        val metrics = metrics
        if (screen != null && metrics != null && screen.mouseReporting && scrollOffset == 0) {
            val button = if (lines > 0) MouseButton.WHEEL_DOWN else MouseButton.WHEEL_UP
            val report = MouseReport(
                button = button,
                col = (x / metrics.cellWidth).toInt().coerceIn(0, screen.cols - 1),
                row = (y / metrics.cellHeight).toInt().coerceIn(0, screen.rows - 1),
                // The wheel has no press and no release: one report is one notch.
                down = true,
            )
            repeat(abs(lines).coerceAtMost(MAX_WHEEL_NOTCHES)) {
                onMouse?.invoke(TerminalMouse.encode(report, screen.mouseSgr))
            }
            return
        }
        scrollBy(-lines)
    }

    // ------------------------------------------------------------------------ the keyboard

    override fun onCheckIsTextEditor(): Boolean = true

    override fun onCreateInputConnection(outAttrs: EditorInfo): InputConnection {
        outAttrs.inputType = InputType.TYPE_NULL
        outAttrs.imeOptions = EditorInfo.IME_ACTION_NONE or EditorInfo.IME_FLAG_NO_FULLSCREEN
        // A connection with no editable text behind it: the console holds no text of its
        // own to edit, every character that arrives goes to the client as a key press.
        return object : BaseInputConnection(this@TerminalView, false) {
            override fun commitText(text: CharSequence, newCursorPosition: Int): Boolean {
                if (text.isNotEmpty()) send(TerminalKey.Rune(text.toString()))
                return true
            }

            override fun deleteSurroundingText(beforeLength: Int, afterLength: Int): Boolean {
                // There is nothing here to delete, so every character the input method
                // wanted removed is a backspace the client has to receive instead. The
                // count is bounded because it arrives from the input method, and a broken
                // one asking for a million deletions must not become a million writes.
                repeat(beforeLength.coerceIn(0, MAX_DELETIONS)) {
                    send(TerminalKey.Special(SpecialKey.BACKSPACE))
                }
                return true
            }

            override fun sendKeyEvent(event: KeyEvent): Boolean = press(event) || super.sendKeyEvent(event)
        }
    }

    override fun onKeyDown(keyCode: Int, event: KeyEvent): Boolean = press(event) || super.onKeyDown(keyCode, event)

    /** press sends a hardware or input-method key press, and reports whether it was one. */
    private fun press(event: KeyEvent): Boolean {
        if (event.action != KeyEvent.ACTION_DOWN) return false
        val key = keyFor(event) ?: return false
        send(key)
        return true
    }

    /** keyFor is the key press an Android key event stands for, if it stands for one. */
    private fun keyFor(event: KeyEvent): TerminalKey? {
        val special = when (event.keyCode) {
            KeyEvent.KEYCODE_ENTER, KeyEvent.KEYCODE_NUMPAD_ENTER -> SpecialKey.ENTER
            KeyEvent.KEYCODE_DEL -> SpecialKey.BACKSPACE
            KeyEvent.KEYCODE_TAB -> SpecialKey.TAB
            KeyEvent.KEYCODE_ESCAPE -> SpecialKey.ESCAPE
            KeyEvent.KEYCODE_DPAD_UP -> SpecialKey.UP
            KeyEvent.KEYCODE_DPAD_DOWN -> SpecialKey.DOWN
            KeyEvent.KEYCODE_DPAD_LEFT -> SpecialKey.LEFT
            KeyEvent.KEYCODE_DPAD_RIGHT -> SpecialKey.RIGHT
            KeyEvent.KEYCODE_MOVE_HOME -> SpecialKey.HOME
            KeyEvent.KEYCODE_MOVE_END -> SpecialKey.END
            KeyEvent.KEYCODE_PAGE_UP -> SpecialKey.PAGE_UP
            KeyEvent.KEYCODE_PAGE_DOWN -> SpecialKey.PAGE_DOWN
            KeyEvent.KEYCODE_FORWARD_DEL -> SpecialKey.DELETE
            else -> null
        }
        if (special != null) return TerminalKey.Special(special)

        // A control key is a flag on the letter it was held with rather than a character of
        // its own, and on a device whose key map gives letters no control code — which is
        // the one on this appliance's tablet — ctrl-d arrives with no character at all. The
        // letter therefore comes from the key code: a key handler that read the character
        // would drop every control combination there is, which is every key a terminal
        // cannot do without.
        if (event.isCtrlPressed) {
            TerminalKeyRow.ctrlLetter(event.keyCode - KeyEvent.KEYCODE_A)?.let { letter ->
                return if (event.isAltPressed) TerminalKey.Alt(letter) else letter
            }
        }

        val character = event.unicodeChar
        if (character == 0) return null
        val text = String(Character.toChars(character))
        // Alt is a flag as well, and a prefix on whatever the key sends.
        val typed = TerminalKeyRow.withControl(event.isCtrlPressed, TerminalKey.Rune(text))
        return if (event.isAltPressed) TerminalKey.Alt(typed) else typed
    }

    private fun showSoftKeyboard() {
        val manager = context.getSystemService(Context.INPUT_METHOD_SERVICE) as? InputMethodManager ?: return
        manager.showSoftInput(this, InputMethodManager.SHOW_IMPLICIT)
    }

    // ------------------------------------------------------------------------ the blink

    override fun onAttachedToWindow() {
        super.onAttachedToWindow()
        cursorVisible = true
        postDelayed(blink, BLINK_MS)
    }

    override fun onDetachedFromWindow() {
        // The blink is a message posted to this view every half second, and it arms the next
        // one as it runs. Left alone when the view is gone it would keep a detached view
        // alive for as long as the process lives, which on an appliance that is never
        // closed is for as long as the tablet is on.
        removeCallbacks(blink)
        super.onDetachedFromWindow()
    }
}
