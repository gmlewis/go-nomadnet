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

package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
	"github.com/rivo/tview"
)

// =============================================================================
// Layer 5 — input / event path cost (one consumed event → full redraw)
//
// These replicate the tview event loop's unit of work for a single consumed
// event: dispatch the synthesized event to the widget's MouseHandler, then —
// because the handler returned consumed=true — run drawFrame (Clear+Draw+Show),
// exactly as application.go:494-498 does. This measures the per-event cost the
// user feels: a mouse-move over the Network/Guide pages, a wheel notch on a
// browser/guide page, etc.
// =============================================================================

// center returns a position inside p's current rect (for synthesizing a mouse
// event that lands on p). Call after p has been laid out (after a warm draw).
func center(p tview.Primitive) (int, int) {
	x, y, w, h := p.GetRect()
	return x + w/2, y + h/2
}

// mouseMoveFrame is one iteration of the mouse-move event loop on a widget
// whose MouseHandler lives on root: dispatch a MouseMove at (mx,my), then — if
// consumed — run drawFrame. Returns whether the event was consumed (so callers
// can assert/compare). The HideCursor that Application.SetFocus would fire on a
// real screen is cheap and omitted; the dominant cost is the full redraw.
func mouseMoveFrame(screen tcell.Screen, root tview.Primitive, handler func(tview.MouseAction, *tcell.EventMouse, func(tview.Primitive)) (bool, tview.Primitive), mx, my int, setFocus func(tview.Primitive)) bool {
	ev := tcell.NewEventMouse(mx, my, tcell.ButtonNone, tcell.ModNone)
	consumed, _ := handler(tview.MouseMove, ev, setFocus)
	if consumed {
		drawFrame(screen, root)
	}
	return consumed
}

// mouseScrollFrame is one iteration of the mouse-wheel event loop: dispatch a
// MouseScrollDown at (mx,my), then run drawFrame (wheel handlers always
// consume, so the redraw always fires).
func mouseScrollFrame(screen tcell.Screen, root tview.Primitive, handler func(tview.MouseAction, *tcell.EventMouse, func(tview.Primitive)) (bool, tview.Primitive), mx, my int, setFocus func(tview.Primitive)) {
	ev := tcell.NewEventMouse(mx, my, tcell.ButtonNone, tcell.ModNone)
	handler(tview.MouseScrollDown, ev, setFocus)
	drawFrame(screen, root)
}

// BenchmarkURWIDColumnsMouseMove is the CONFIRMED mouse-move flicker root cause.
// urwidColumns.MouseHandler (tui/urwid-columns.go:452) returns consumed=true and
// calls setFocus for EVERY mouse action — including MouseMove — over a
// focusable column, so every mouse-position event over the Network or Guide
// page triggers a full screen clear + full tree redraw + screen sync. This
// benchmark measures that per-move cost on a mounted Guide page and contrasts
// it with a plain-Flex page (Browser standalone) whose move handler does NOT
// consume — so it redraws nothing. The delta is the flicker cost.
func BenchmarkURWIDColumnsMouseMove(b *testing.B) {
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	gd := NewGuideDisplay(app)
	gd.showMarkupForTest(benchSyntheticMicron(64))
	app.Main.SetDisplay("guide", gd.Widget())
	app.Main.SelectPage("guide")
	setFocus := func(p tview.Primitive) { app.SetFocus(p) }

	screen := newBenchScreen()
	defer screen.Fini()
	root := app.Main.Root()
	root.SetRect(0, 0, benchScreenW, benchScreenH)
	drawFrame(screen, root) // settle layout

	// The urwidColumns handler lives on the Guide widget; route mouse events to
	// it. Move over the right (reader) column at the reader's center.
	cols := gd.Widget()
	mx, my := center(gd.reader)
	handler := cols.MouseHandler()
	mouseMoveFrame(screen, root, handler, mx, my, setFocus) // warm the focus path

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		mouseMoveFrame(screen, root, handler, mx, my, setFocus)
	}
}

// BenchmarkPlainFlexMouseMove is the baseline: a non-urwidColumns page (the
// standalone Browser, a plain tview.Flex) whose content TextView does NOT
// consume MouseMove, so a hover redraws nothing. Contrast with
// BenchmarkURWIDColumnsMouseMove to see the flicker overhead.
func BenchmarkPlainFlexMouseMove(b *testing.B) {
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	bd := NewBrowserDisplay(app)
	markup := benchSyntheticMicron(64)
	lines := micron.RenderToStyledLines(markup, micronTheme(app.Theme))
	text, _ := StyledLinesToTviewText(lines, benchScreenW-2)
	bd.content.SetText(text)
	bd.renderedWidth = benchScreenW - 2
	setFocus := func(p tview.Primitive) { app.SetFocus(p) }

	screen := newBenchScreen()
	defer screen.Fini()
	root := bd.Widget()
	root.SetRect(0, 0, benchScreenW, benchScreenH)
	drawFrame(screen, root)

	mx, my := center(bd.content)
	handler := root.MouseHandler()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		mouseMoveFrame(screen, root, handler, mx, my, setFocus)
	}
}

// BenchmarkGuideWheelScroll measures the per-wheel-notch cost on the Guide
// page: the ScrollBar delegates the wheel to its TextView (lineOffset++), then
// the full MainDisplay tree redraws — including ScrollBar.Draw's per-frame
// wrappedRowCount re-wrap. This is the path the user called "painfully
// sluggish."
func BenchmarkGuideWheelScroll(b *testing.B) {
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	gd := NewGuideDisplay(app)
	gd.showMarkupForTest(benchSyntheticMicron(64))
	app.Main.SetDisplay("guide", gd.Widget())
	app.Main.SelectPage("guide")
	setFocus := func(p tview.Primitive) { app.SetFocus(p) }

	screen := newBenchScreen()
	defer screen.Fini()
	root := app.Main.Root()
	root.SetRect(0, 0, benchScreenW, benchScreenH)
	drawFrame(screen, root)

	// Wheel events go to the ScrollBar's (delegated TextView) handler.
	mx, my := center(gd.reader)
	handler := gd.scroll.MouseHandler()
	mouseScrollFrame(screen, root, handler, mx, my, setFocus) // warm

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		mouseScrollFrame(screen, root, handler, mx, my, setFocus)
	}
}

// BenchmarkBrowserWheelScroll measures the per-wheel-notch cost on a
// standalone Browser page: the browserPageView delegates the wheel to its
// TextView (lineOffset++ via the cached index), then the full browser tree
// redraws. Contrast with the guide number — the guide pays the extra
// ScrollBar re-wrap.
func BenchmarkBrowserWheelScroll(b *testing.B) {
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	bd := NewBrowserDisplay(app)
	markup := benchSyntheticMicron(64)
	lines := micron.RenderToStyledLines(markup, micronTheme(app.Theme))
	text, _ := StyledLinesToTviewText(lines, benchScreenW-2)
	bd.content.SetText(text)
	bd.renderedWidth = benchScreenW - 2
	setFocus := func(p tview.Primitive) { app.SetFocus(p) }

	screen := newBenchScreen()
	defer screen.Fini()
	root := bd.Widget()
	root.SetRect(0, 0, benchScreenW, benchScreenH)
	drawFrame(screen, root)

	mx, my := center(bd.content)
	handler := bd.content.MouseHandler()
	mouseScrollFrame(screen, root, handler, mx, my, setFocus)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		mouseScrollFrame(screen, root, handler, mx, my, setFocus)
	}
}

// BenchmarkGuideWheelScrollDeepOffset scrolls a long guide page deep, then
// measures the per-notch cost while already deep. If tview's resetIndex →
// parseAhead-from-line-0 cost bites on the guide (e.g. on a width change), it
// shows up here as a higher per-notch cost than the top-offset guide number.
func BenchmarkGuideWheelScrollDeepOffset(b *testing.B) {
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	gd := NewGuideDisplay(app)
	gd.showMarkupForTest(benchSyntheticMicron(512))
	app.Main.SetDisplay("guide", gd.Widget())
	app.Main.SelectPage("guide")
	setFocus := func(p tview.Primitive) { app.SetFocus(p) }

	screen := newBenchScreen()
	defer screen.Fini()
	root := app.Main.Root()
	root.SetRect(0, 0, benchScreenW, benchScreenH)
	drawFrame(screen, root)
	// Scroll deep via the reader so the offset is large before timing.
	gd.reader.ScrollTo(800, 0)
	drawFrame(screen, root)

	mx, my := center(gd.reader)
	handler := gd.scroll.MouseHandler()
	mouseScrollFrame(screen, root, handler, mx, my, setFocus)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		mouseScrollFrame(screen, root, handler, mx, my, setFocus)
	}
}
