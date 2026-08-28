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
	"github.com/rivo/tview"
)

// TestEmptyPlaceholderOffFocusStyle pins the 2026-08-28 finding: Python paints
// the Conversations empty-state placeholder row with the list_off_focus
// palette (fg #111→#000000, bg #777→#878787) across the full inner width, in
// every focus state (live captures pyaconv_100x28_00/01/02 row 4 all show
// fg 0,0,0 bg 135,135,135). The Go port previously rendered the placeholder
// with the terminal default (no highlight).
func TestEmptyPlaceholderOffFocusStyle(t *testing.T) {
	t.Parallel()

	colors := GetThemeColors(ThemeDark)
	fg, bg := colors["list_off_focus_fg"], colors["list_off_focus_bg"]

	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)

	// Standalone ILB styled exactly as the Conversations display will wire it.
	list := tview.NewList()
	ilb := NewIndicativeListBox(list)
	ilb.SetEmptyText("No trusted conversations")
	ilb.SetEmptyStyle(fg, bg)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(50, 6)
	ilb.SetRect(0, 0, 50, 6)
	ilb.Draw(screen)

	// The placeholder row is the FIRST row of the list area (between the
	// indicator bars) — screen row 1. Every cell of the row must carry the
	// list_off_focus style (urwid's AttrMap fills the full canvas width).
	for x := 0; x < 50; x++ {
		c, _, style, _ := screen.GetContent(x, 1)
		if c == ' ' && (x < 13 || x > 37) {
			// Padding cells outside the text still carry the background.
		}
		f, bgs, _ := style.Decompose()
		_ = f
		if bgs != bg {
			t.Fatalf("placeholder row cell (%v,1) bg = %v, want %v (list_off_focus)", x, bgs, bg)
		}
	}
	// The text itself carries the foreground color.
	c, _, style, _ := screen.GetContent(13, 1) // (50-24)/2 = 13 → 'N'
	if c != 'N' {
		t.Fatalf("placeholder text cell (13,1) = %q, want 'N'", string(c))
	}
	if f, _, _ := style.Decompose(); f != fg {
		t.Errorf("placeholder text fg = %v, want %v", f, fg)
	}

	// Unfocused: the same style persists (the live captures show it in every
	// focus state).
	ilb.Blur()
	screen.Clear()
	ilb.Draw(screen)
	_, _, style, _ = screen.GetContent(14, 1)
	if _, bgs, _ := style.Decompose(); bgs != bg {
		t.Errorf("unfocused placeholder bg = %v, want %v (live shows it in every focus state)", bgs, bg)
	}
}

// TestEmptyStyleNotAppliedToEmptyWidget pins the boundary: the Channels empty
// state uses SetEmptyWidget (its own styled primitive) and must be untouched
// by the new empty-text styling.
func TestEmptyStyleNotAppliedToEmptyWidget(t *testing.T) {
	t.Parallel()

	colors := GetThemeColors(ThemeDark)
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)

	list := tview.NewList()
	ilb := NewIndicativeListBox(list)
	ilb.SetEmptyStyle(colors["list_off_focus_fg"], colors["list_off_focus_bg"])
	// A custom empty widget (like the Channels noHubsText) keeps its own style.
	w := tview.NewTextView().SetText("No hubs yet. Press Ctrl-N to add one.")
	ilb.SetEmptyWidget(w)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(50, 6)
	ilb.SetRect(0, 0, 50, 6)
	ilb.Draw(screen)

	// Whatever the widget draws, the ILB must not paint a background over the
	// row — the custom widget's own TextView carries the default bg here.
	_, _, style, _ := screen.GetContent(2, 1)
	if _, bgs, _ := style.Decompose(); bgs == colors["list_off_focus_bg"] {
		t.Error("SetEmptyStyle must not affect the SetEmptyWidget path")
	}
}
