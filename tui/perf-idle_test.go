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
)

// =============================================================================
// Layer 6 — background / idle refresh cost
//
// The app has several background goroutines that marshal work onto the event
// loop via App.QueueUpdateDraw(f) (cmd/gonomadnet/textui.go): a 1 s interface
// ticker, a 30 s sync-status refresh, and a 2 s unread-conversation blink
// (MainDisplay.startUnreadBlink). tview's QueueUpdateDraw runs f and THEN calls
// Draw() unconditionally, so EVERY tick triggers a full screen redraw even when
// the underlying state did not change — the "CPU spent when nothing should be
// happening" architectural concern.
//
// The 1 s / 30 s tickers gather data from the live RNS node (interfaces,
// sync state), so their data-gathering cost is coupled to package main and the
// running transport; it is captured by the live pprof harness (Layer 7), not by
// a headless go-test benchmark. Their REDRAW cost equals the full-page Draw
// numbers already measured in Layer 4. What IS measurable headlessly is the
// unread blink's per-tick work + the unconditional redraw it triggers.
// =============================================================================

// BenchmarkUnreadBlinkTick measures the full per-2 s idle cost: the blink's
// updateUnreadIndicator() (a no-op when the indicator is unchanged, as it is on
// steady state with no unread probe set) PLUS the full screen redraw that
// QueueUpdateDraw fires afterward regardless. This is the floor of idle CPU:
// every 2 s the whole screen clears and redraws even though nothing changed.
func BenchmarkUnreadBlinkTick(b *testing.B) {
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	gd := NewGuideDisplay(app)
	gd.showMarkupForTest(benchSyntheticMicron(64))
	app.Main.SetDisplay("guide", gd.Widget())
	app.Main.SelectPage("guide")

	screen := newBenchScreen()
	defer screen.Fini()
	root := app.Main.Root()
	root.SetRect(0, 0, benchScreenW, benchScreenH)
	drawFrame(screen, root)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		// The blink's queued work (cheap when unchanged) ...
		app.Main.updateUnreadIndicator()
		// ... followed by the unconditional redraw QueueUpdateDraw always fires.
		drawFrame(screen, root)
	}
}

// BenchmarkUpdateUnreadIndicator isolates the indicator check itself (no
// redraw). Contrast with BenchmarkUnreadBlinkTick: the indicator check is
// negligible; the cost the user pays is the redraw, not the probe.
func BenchmarkUpdateUnreadIndicator(b *testing.B) {
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	b.ReportAllocs()
	for b.Loop() {
		app.Main.updateUnreadIndicator()
	}
}

// BenchmarkRedrawMenuBar measures the menu-bar repaint (the work the blink does
// only when the indicator actually changes). One row of text — cheap relative
// to the full-tree redraw, but it runs on every change.
func BenchmarkRedrawMenuBar(b *testing.B) {
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	gd := NewGuideDisplay(app)
	app.Main.SetDisplay("guide", gd.Widget())
	app.Main.SelectPage("guide")

	screen := newBenchScreen()
	defer screen.Fini()
	root := app.Main.Root()
	root.SetRect(0, 0, benchScreenW, benchScreenH)
	drawFrame(screen, root)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		app.Main.redrawMenuBar()
	}
}
