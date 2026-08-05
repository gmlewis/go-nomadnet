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

	"github.com/rivo/tview"
)

// loadingRows renders the BrowserDisplay layout while a fetch is in flight and
// returns the rendered cell rows.
func loadingRows(t *testing.T, bd *BrowserDisplay, w, h int) []string {
	t.Helper()
	bd.layout.SetRect(0, 0, w, h)
	return renderPrimitive(t, bd.layout, w, h)
}

// TestBrowserLoadingShowsCenteredRetrieving pins the fix for the user report
// that nomadnet shows "Retrieving" and "[<url>]" vertically+horizontally
// centered in the browser while loading, but gonomadnet did not. Python's
// Browser.update_display REQUEST_SENT branch (Browser.py:593-598) sets the body
// to Filler(Text("Retrieving\n["+current_url()+"]", CENTER), MIDDLE). The Go
// port now swaps a MIDDLE-centered "Retrieving\n[<url>]" body into the layout
// in place of the page content during displayURL.
func TestBrowserLoadingShowsCenteredRetrieving(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	bd := NewBrowserDisplay(app)

	// Drive a load without a wired OnRetrieveURL so the page never renders —
	// the loading body stays on screen.
	bd.LoadURL("a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6/page/index.mu")

	if bd.loading == nil {
		t.Fatal("loading body not shown after LoadURL (bd.loading is nil)")
	}
	if bd.content == nil {
		t.Fatal("content nil")
	}

	rows := loadingRows(t, bd, 60, 16)

	// Find the "Retrieving" row and the "[<url>]" row. They must be present and
	// horizontally centered (equal-or-within-one left padding).
	retRow := -1
	urlRow := -1
	for i, r := range rows {
		if strings.Contains(r, "Retrieving") {
			retRow = i
		}
		if strings.Contains(r, "[a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6/page/index.mu]") {
			urlRow = i
		}
	}
	if retRow < 0 {
		t.Fatalf("Retrieving line not found in render:\n%s", strings.Join(rows, "\n"))
	}
	if urlRow < 0 {
		t.Fatalf("[url] line not found in render:\n%s", strings.Join(rows, "\n"))
	}
	if urlRow != retRow+1 {
		t.Errorf("Retrieving at row %d, [url] at row %d; want [url] immediately below", retRow, urlRow)
	}

	// Vertically centered: the "Retrieving" row sits within one row of the
	// vertical middle of the body region. The layout is header (url + divider
	// = 2 rows) · body · footer (divider + status = 2 rows). Body height =
	// 16-4 = 12; two lines centered ⇒ top at ceil((12-2)/2) = 5 rows into the
	// body ⇒ screen row 2+5 = 7.
	if retRow < 5 || retRow > 9 {
		t.Errorf("Retrieving at row %d, want roughly vertically centered (5..9) in a 16-row pane with 2 header + 2 footer rows", retRow)
	}

	// Horizontally centered: the URL line's left padding inside the body area
	// ≈ (innerWidth - len) / 2. The layout has NO border (the enclosing
	// BrowserPane LineBox is the caller's responsibility), so the inner content
	// starts at col 0.
	firstCol := -1
	for x := 0; x < 60; x++ {
		if cellAt(rows, x, urlRow) != " " {
			firstCol = x
			break
		}
	}
	if firstCol < 0 {
		t.Fatalf("[url] line not found in cells at row %d", urlRow)
	}
	leftPad := firstCol // inner content begins at column 0 (no border)
	const innerWidth = 60
	wantPad := (innerWidth - tview.TaggedStringWidth("[a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6/page/index.mu]")) / 2
	if abs(leftPad-wantPad) > 1 {
		t.Errorf("[url] left padding = %d, want ~%d (centered)", leftPad, wantPad)
	}

	// The URL header above still shows the URL (Python keeps the control-widget
	// header during loading).
	if got := bd.URLDisplayText(); !strings.Contains(got, "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6/page/index.mu") {
		t.Errorf("URL display = %q during loading, want it to show the requested URL", got)
	}
}

// TestBrowserLoadingRestoresContentOnRender pins that renderPage swaps the
// page content back in, removing the centered loading body once the page
// arrives.
func TestBrowserLoadingRestoresContentOnRender(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	bd := NewBrowserDisplay(app)

	bd.LoadURL("a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6/page/index.mu")
	if bd.loading == nil {
		t.Fatal("loading body not shown")
	}

	bd.RenderPage(">Welcome Node\nsome body text")

	if bd.loading != nil {
		t.Error("loading body still set after RenderPage; renderPage must restore content")
	}

	// The page content is back and shows the rendered page, not the loading
	// text.
	text := bd.content.GetText(true)
	if !strings.Contains(text, "Welcome Node") {
		t.Errorf("content after render = %q, want the rendered page", text)
	}
}

// abs is provided by cursor-coords.go in this package.
