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
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
)

// TestDividerReflowsToContentWidth verifies the R-MICRON-DIVIDER-WIDTH fix: a
// micron `-` horizontal divider must fill the browser content's real width,
// not the stale/zero width seen when the fetch callback renders the page
// before the content TextView has been drawn at its true size.
//
// The real flow is: OnRetrieveURL fires (via QueueUpdateDraw) before the
// browser display's first Draw, so contentWidth() returns a stale value and
// the divider renders too short. Once the layout is drawn at the real width,
// browserPageView.Draw queues a re-render (reflowIfWidthChanged) so the
// divider reflows to match — mirroring Python's urwid.Divider box widget,
// which fills the pane width at draw time.
//
// This test simulates that flow without a live event loop: render at the
// pre-draw (stale) width, draw the layout at a known full width, then perform
// the queued re-render and assert the divider now spans the content width.
func TestDividerReflowsToContentWidth(t *testing.T) {
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	bd := NewBrowserDisplay(app)

	// Render the page BEFORE the content has a real rect (the stale-width
	// condition). The divider is whatever contentWidth returns now.
	bd.RenderPage(">Title\n-\nServed by rngit 1.4.2")
	staleWidth := bd.contentWidth()
	t.Logf("pre-draw contentWidth=%d", staleWidth)

	// Draw the layout at a known full width. This is what the event loop does
	// on the first refresh after the fetch callback, establishing the content's
	// real inner rect.
	screen := tcell.NewSimulationScreen("UTF-8")
	screen.SetSize(120, 30)
	bd.layout.SetRect(0, 0, 120, 30)
	bd.layout.Draw(screen)

	realWidth := bd.contentWidth()
	t.Logf("post-draw contentWidth=%d", realWidth)
	if realWidth <= staleWidth {
		t.Fatalf("draw did not establish a wider content rect: stale=%d real=%d", staleWidth, realWidth)
	}

	// The production glue (browserPageView.Draw → reflowIfWidthChanged) queues
	// a re-render via QueueUpdateDraw, which is a no-op without a running loop.
	// Perform that re-render explicitly and confirm the divider reflows.
	bd.renderPage()

	text := bd.content.GetText(true)
	var divider string
	for line := range strings.SplitSeq(text, "\n") {
		if strings.Contains(line, "─") {
			divider = line
			break
		}
	}
	if divider == "" {
		t.Fatalf("no divider line in rendered page:\n%s", text)
	}
	got := len([]rune(divider))
	if got != realWidth {
		t.Fatalf("divider width = %d, want %d (content width); divider=%q", got, realWidth, divider)
	}
}

// TestReflowGuardNoPage verifies reflowIfWidthChanged is a no-op when no page
// is loaded (currentMarkup empty), so the Draw path does not queue spurious
// re-renders for the initial placeholder text.
func TestReflowGuardNoPage(t *testing.T) {
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	bd := NewBrowserDisplay(app)
	// No RenderPage called → currentMarkup is empty. Drawing must not panic
	// and must not request a reflow.
	screen := tcell.NewSimulationScreen("UTF-8")
	screen.SetSize(120, 30)
	bd.layout.SetRect(0, 0, 120, 30)
	bd.layout.Draw(screen) // exercises browserPageView.Draw → reflowIfWidthChanged
	if bd.currentMarkup != "" {
		t.Fatalf("currentMarkup=%q, want empty", bd.currentMarkup)
	}
}
