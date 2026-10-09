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
 * cell is, [TerminalPalette]'s. What is left — and all this file contains — is the calls
 * into `Canvas` and the touches that come back out of it, neither of which a JVM test can
 * reach. Nothing here may be claimed as working until it has been looked at on the tablet.
 *
 * The view never starts or talks to the client itself. It reports a key press through
 * [onKey] and a size change through [onResize], and is told what happened through [screen]
 * and [outputParsed], so the session's lifetime stays [ConsoleSession]'s business and the
 * drawing stays the UI thread's.
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

        /** DRAG_LINES is how far a finger must move down to scroll a line back. */
        private const val DRAG_LINES: Int = 1

        /** MAX_DELETIONS is the most backspaces one input-method edit can ask for. */
        private const val MAX_DELETIONS: Int = 64

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

    // Control, which a tablet cannot hold down — see [ControlLatch] — and the state of the
    // cursor's blink.
    private val control = ControlLatch()
    private var cursorVisible = true
    private val blink = object : Runnable {
        override fun run() {
            cursorVisible = !cursorVisible
            invalidate()
            postDelayed(this, BLINK_MS)
        }
    }

    // Where a finger went down, in the view's pixels, and whether it went down on the key
    // row — a tap there is a key and a drag there must not scroll the console.
    private var downX = 0f
    private var downY = 0f
    private var downOnKeyRow = false
    private var dragRemainder = 0f
    private val touchSlop = ViewConfiguration.get(context).scaledTouchSlop

    init {
        // The view draws itself rather than being a background to something else, so it has
        // to be told that: a View that reports no background is otherwise skipped entirely.
        setWillNotDraw(false)
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
        val wanted = measured.gridFor(width, height - keyRowHeight(measured))
        scrollOffset = scrollOffset.coerceIn(0, TerminalScroll.maxOffset(screenTotalLines(), wanted.rows))
        if (wanted != grid) {
            grid = wanted
            // The session is told once the grid actually changed, so a rotation is one
            // resize to the client rather than one per layout pass.
            onResize?.invoke(wanted)
        }
        invalidate()
    }

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
        metrics.cellHeight + 2 * keyRowPadding(metrics)

    private fun keyRowPadding(metrics: TerminalMetrics): Int = max(4, metrics.cellHeight / 4)

    /** keyRowTop is the y the key row starts at, or the view's height when there is none. */
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
     * The buttons share the width equally, which is the division [TerminalKeyRow.entryAt]
     * undoes for a tap, so what is drawn and what is hit are the same arithmetic. Control is
     * drawn differently while it is armed: a latch that cannot be seen is one a reader
     * leaves set and then cannot explain the next key press.
     */
    private fun drawKeyRow(canvas: Canvas, metrics: TerminalMetrics) {
        val padding = keyRowPadding(metrics)
        val top = keyRowTop(metrics)
        val rowHeight = height - top
        if (rowHeight <= 0 || width <= 0) return

        textPaint.isFakeBoldText = false
        textPaint.textSkewX = 0f
        textPaint.color = TerminalPalette.DEFAULT_FOREGROUND
        canvas.drawRect(0f, top.toFloat(), width.toFloat(), (top + 1).toFloat(), textPaint)

        val entries = TerminalKeyRow.entries
        val share = width.toFloat() / entries.size
        val textY = top + padding + -textPaint.fontMetrics.ascent
        for ((index, entry) in entries.withIndex()) {
            val left = index * share
            if (entry is KeyRowEntry.Control && control.armed) {
                textPaint.color = TerminalPalette.DEFAULT_FOREGROUND
                canvas.drawRect(left, (top + 1).toFloat(), left + share, height.toFloat(), textPaint)
                textPaint.color = TerminalPalette.DEFAULT_BACKGROUND
            }
            val labelWidth = textPaint.measureText(entry.label)
            canvas.drawText(entry.label, left + (share - labelWidth) / 2f, textY, textPaint)
            textPaint.color = TerminalPalette.DEFAULT_FOREGROUND
        }
    }

    // --------------------------------------------------------------------------- touching

    override fun onTouchEvent(event: MotionEvent): Boolean {
        val metrics = metrics ?: return false
        when (event.actionMasked) {
            MotionEvent.ACTION_DOWN -> {
                downX = event.x
                downY = event.y
                dragRemainder = 0f
                downOnKeyRow = event.y >= keyRowTop(metrics)
            }

            MotionEvent.ACTION_MOVE -> {
                if (downOnKeyRow) return true
                dragRemainder += event.y - downY
                downY = event.y
                val lines = (dragRemainder / metrics.cellHeight).toInt()
                if (lines != 0) {
                    dragRemainder -= lines * metrics.cellHeight
                    scrollBy(-lines * DRAG_LINES)
                }
            }

            MotionEvent.ACTION_UP -> {
                if (downOnKeyRow && event.y >= keyRowTop(metrics) && abs(event.x - downX) < touchSlop) {
                    pressKeyRow(event.x.toInt())
                } else if (!downOnKeyRow && abs(event.y - downY) < touchSlop) {
                    // A tap on the console itself is how the soft keyboard is asked for: a
                    // reader who has to find a menu item to type has been given a terminal
                    // with no way in.
                    requestFocus()
                    showSoftKeyboard()
                }
            }

            MotionEvent.ACTION_SCROLL -> {
                // A wheel or a trackpad, which reports the scroll as a signed count rather
                // than as a distance: up and back into the scrollback is positive. It is
                // rounded, because a wheel that reported a fraction of a step would
                // otherwise be a gesture that scrolled nothing at all.
                val lines = event.getAxisValue(MotionEvent.AXIS_VSCROLL).roundToInt()
                if (lines != 0) scrollBy(lines)
            }
        }
        return true
    }

    /** pressKeyRow is what a tap on the key row sends. */
    private fun pressKeyRow(x: Int) {
        when (val entry = TerminalKeyRow.entryAt(x, width)) {
            is KeyRowEntry.Control -> {
                control.toggle()
                invalidate()
            }

            is KeyRowEntry.Press -> send(entry.key)
        }
    }

    /**
     * send hands one key to the session, after the control latch has had its say.
     *
     * Every key comes through here — the row's buttons, the soft keyboard, and a hardware
     * keyboard — because the latch is the console's and not the row's: a person arms Control
     * with a tap and then types a letter wherever they like.
     */
    private fun send(key: TerminalKey) {
        val wasArmed = control.armed
        val sent = control.spend(key)
        onKey?.invoke(sent)
        // The Control button is drawn differently while it is armed, so the row is redrawn
        // when the latch changes — and only then, since a keystroke redraws the console
        // through the client's own output.
        if (wasArmed != control.armed) {
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

        val character = event.unicodeChar
        if (character == 0) return null
        val text = String(Character.toChars(character))
        // A hardware control key is reported as a flag rather than as a code of its own, and
        // ctrl-c has to reach the client as one byte: this is the only path that can send it
        // from a keyboard, and the key row's latch is the only one from a touchscreen.
        if (event.isCtrlPressed && text.length == 1) {
            val ctrl = TerminalKeyRow.withControl(armed = true, key = TerminalKey.Rune(text))
            if (ctrl != TerminalKey.Rune(text)) return ctrl
        }
        return TerminalKey.Rune(text)
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
