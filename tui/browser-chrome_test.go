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
)

// TestSizeStrGolden pins the Go port of Python Browser.size_str
// (Browser.py:1817-1834) against golden values captured from the installed
// Python nomadnet. size_str uses base-1000 units with NO space between the
// number and the unit ("815B", "1.02KB"); the 'b' suffix converts to bits
// first ("6.52Kb"). Distinct from Prettysize (space + RNS.prettysize).
func TestSizeStrGolden(t *testing.T) {
	t.Parallel()
	cases := []struct {
		num    float64
		suffix string
		want   string
	}{
		{0, "B", "0B"},
		{1, "B", "1B"},
		{815, "B", "815B"},
		{999, "B", "999B"},
		{1000, "B", "1.00KB"},
		{1023, "B", "1.02KB"},
		{1024, "B", "1.02KB"},
		{1500, "B", "1.50KB"},
		{1048576, "B", "1.05MB"},
		{1234567, "B", "1.23MB"},
		{815, "b", "6.52Kb"},
		{1058, "b", "8.46Kb"},
		{8480, "b", "67.84Kb"},
		{1234567, "b", "9.88Mb"},
		{6520, "b", "52.16Kb"},
	}
	for _, c := range cases {
		t.Run(t.Name(), func(t *testing.T) {
			if got := sizeStr(c.num, c.suffix); got != c.want {
				t.Errorf("sizeStr(%v, %q) = %q, want %q", c.num, c.suffix, got, c.want)
			}
		})
	}
}

// TestBrowserStatusTextDone pins the DONE footer status line, mirroring Python
// Browser.status_text (Browser.py:1756-1803): "Done" + "  ▤ <size>   ↓<size>
// in <t>s   ◷ <speed>b/s". Captured from the Python source (glyphs: page "▤ ",
// arrow_d "↓", speed "◷ ").
func TestBrowserStatusTextDone(t *testing.T) {
	t.Parallel()
	g := GetGlyphSet(GlyphUnicode)
	got := browserStatusText(g, browserDone, 815, 815, 0.77, true, false, "")
	want := "Done  " + g["page"] + "815B   " + g["arrow_d"] + "815B in 0.77s   " + g["speed"] + sizeStr(815/0.77, "b") + "/s"
	if got != want {
		t.Errorf("statusText(Done) = %q\n                 want %q", got, want)
	}
}

// TestBrowserStatusTextCached pins the cache-hit footer. Python status_text:
// for a DONE cached load with no transfer size, stats_string = " (cached)" and
// the DONE branch returns "Done"+stats_string = "Done (cached)".
func TestBrowserStatusTextCached(t *testing.T) {
	t.Parallel()
	g := GetGlyphSet(GlyphUnicode)
	if got := browserStatusText(g, browserDone, 0, 0, 0, false, true, ""); got != "Done (cached)" {
		t.Errorf("cached status = %q, want %q", got, "Done (cached)")
	}
}

// TestBrowserStatusTextInFlight pins the in-flight status labels.
func TestBrowserStatusTextInFlight(t *testing.T) {
	t.Parallel()
	g := GetGlyphSet(GlyphUnicode)
	cases := []struct {
		status browserStatus
		want   string
	}{
		{browserRequestSent, "Request sent, awaiting response..."},
		{browserRequesting, "Sending request..."},
		{browserEstablishingLink, "Establishing link..."},
		{browserNoPath, "No path to destination known"},
		{browserDisconnected, "Disconnected"},
	}
	for _, c := range cases {
		if got := browserStatusText(g, c.status, 0, 0, 0, false, false, ""); got != c.want {
			t.Errorf("status(%v) = %q, want %q", c.status, got, c.want)
		}
	}
}

// TestBrowserDisplayNoInnerBorder pins R-NET-BROWSER-STRUCTURE: the
// BrowserDisplay layout renders NO border and NO "Browser" title — Python has
// a single LineBox(BrowserFrame(...), title=<hash>) and the enclosing LineBox
// is the BrowserPane's responsibility, so the display itself must not add a
// nested bordered box.
func TestBrowserDisplayNoInnerBorder(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	bd := NewBrowserDisplay(app)

	rows := renderPrimitive(t, bd.layout, 60, 16)
	for i, r := range rows {
		if strings.Contains(r, "Browser") {
			t.Errorf("row %d contains a \"Browser\" title; the BrowserPane owns the title — drop the inner box (R-NET-BROWSER-STRUCTURE): %q", i, r)
		}
		// A bordered box draws corner/frame glyphs at the edges.
		if strings.Contains(r, "┌") || strings.Contains(r, "┐") || strings.Contains(r, "└") || strings.Contains(r, "┘") {
			t.Errorf("row %d has a border frame glyph; the BrowserPane owns the border (R-NET-BROWSER-STRUCTURE): %q", i, r)
		}
	}
}

// TestBrowserDisplayNoNavBar pins R-NET-BROWSER-NAVBAR: no spurious top nav bar
// "Enter Load  Ctrl-L Back  Ctrl-R Forward  Esc URL bar". Python has no top nav
// bar; controls live in the footer. The layout's first rows must be the URL
// header + divider, never the nav-bar text.
func TestBrowserDisplayNoNavBar(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	bd := NewBrowserDisplay(app)

	// Render the layout and scan every row for the nav-bar signature.
	rows := renderPrimitive(t, bd.layout, 60, 16)
	for i, r := range rows {
		if strings.Contains(r, "Enter Load") || strings.Contains(r, "Ctrl-L Back") || strings.Contains(r, "URL bar") {
			t.Errorf("row %d contains nav-bar text %q; Python has no top nav bar (R-NET-BROWSER-NAVBAR)", i, r)
		}
	}
}

// TestBrowserDisplayURLHeader pins R-NET-BROWSER-URLBAR: the URL header is a
// non-editable Text showing g["node"]+" "+<url> ("Ⓝ <hash>:/path"), NOT an
// editable "URL: <hash>" field. The urlHeader is a TextView (not ReadlineEdit)
// and shows the node glyph + the canonical URL after a load.
func TestBrowserDisplayURLHeader(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	bd := NewBrowserDisplay(app)

	// urlHeader is a TextView (non-editable), not a ReadlineEdit field.
	if bd.urlHeader == nil {
		t.Fatal("urlHeader is nil")
	}

	// A 32-hex hash URL loads as "<hash>:/page/index.mu"; the header shows the
	// node glyph + " " + that canonical URL.
	hash := "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6"
	bd.LoadURL(hash)
	want := app.Glyphs["node"] + " " + hash + ":/page/index.mu"
	if got := bd.urlHeader.GetText(true); got != want {
		t.Errorf("urlHeader = %q, want %q (R-NET-BROWSER-URLBAR)", got, want)
	}
	if bd.URLDisplayText() != hash+":/page/index.mu" {
		t.Errorf("URLDisplayText = %q, want %q", bd.URLDisplayText(), hash+":/page/index.mu")
	}
}

// TestBrowserDisplayChromeDividers pins R-NET-BROWSER-DIVIDER: the header (below
// the URL row) and the footer (above the status row) each render a full-width
// "┄┄┄" divider (urwid.Divider(self.g["divider1"])).
func TestBrowserDisplayChromeDividers(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	bd := NewBrowserDisplay(app)
	bd.LoadURL("a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6")

	rows := renderPrimitive(t, bd.layout, 40, 16)
	div := app.Glyphs["divider1"] // "┄"
	// Header divider is row 1 (row 0 = URL header); it must be a full-width run
	// of the divider glyph.
	if !strings.HasPrefix(strings.TrimRight(rows[1], " "), strings.Repeat(div, 5)) {
		t.Errorf("header divider row 1 = %q, want a full-width run of %q (R-NET-BROWSER-DIVIDER)", rows[1], div)
	}
	// Footer divider is the row above the last (footer status). Find it: the
	// last divider run above the bottom row.
	foundFooterDiv := false
	for i := len(rows) - 2; i >= 2; i-- {
		if strings.HasPrefix(strings.TrimRight(rows[i], " "), strings.Repeat(div, 5)) {
			foundFooterDiv = true
			break
		}
	}
	if !foundFooterDiv {
		t.Errorf("no footer divider row found (R-NET-BROWSER-DIVIDER):\n%s", strings.Join(rows, "\n"))
	}
}

// TestBrowserDisplayFooterTransferStats pins R-NET-BROWSER-FOOTER: after a
// fetch with transfer stats, the footer shows the "Done  ▤ <size>   ↓<size> in
// <t>s   ◷ <speed>b/s" line (Python make_status_widget, Browser.py:497-501).
func TestBrowserDisplayFooterTransferStats(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	bd := NewBrowserDisplay(app)

	bd.LoadURL("a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6")
	bd.SetTransferStats(815, 815, 0.77, false)
	bd.RenderPage(">Welcome\nbody")

	g := app.Glyphs
	want := "Done  " + g["page"] + "815B   " + g["arrow_d"] + "815B in 0.77s   " + g["speed"] + sizeStr(815/0.77, "b") + "/s"
	if got := bd.footerStatus.GetText(true); got != want {
		t.Errorf("footer status = %q\n           want %q (R-NET-BROWSER-FOOTER)", got, want)
	}
}

// TestBrowserDisplayLinkPeekFooter pins R-NET-BROWSER-LINKPEEK: marked_link
// shows "Link to <target>" in the browser FOOTER (not the global shortcut bar),
// and clearing it restores the transfer-status widget.
func TestBrowserDisplayLinkPeekFooter(t *testing.T) {
	t.Parallel()
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	bd := NewBrowserDisplay(app)
	bd.LoadURL("a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6")
	bd.SetTransferStats(815, 815, 0.77, false)
	bd.RenderPage(">Welcome\nbody")

	// Peeking a link replaces the footer with "Link to <target>".
	bd.MarkedLink("8b0b7663abcdef0123456789abcdef01/page/index.mu", "")
	want := "Link to 8b0b7663abcdef0123456789abcdef01/page/index.mu"
	if got := bd.footerStatus.GetText(true); got != want {
		t.Errorf("link peek footer = %q, want %q (R-NET-BROWSER-LINKPEEK)", got, want)
	}

	// Clearing the peek restores the transfer-status widget.
	bd.MarkedLink("", "")
	g := app.Glyphs
	wantStats := "Done  " + g["page"] + "815B   " + g["arrow_d"] + "815B in 0.77s   " + g["speed"] + sizeStr(815/0.77, "b") + "/s"
	if got := bd.footerStatus.GetText(true); got != wantStats {
		t.Errorf("footer after clear = %q, want stats %q (R-NET-BROWSER-LINKPEEK)", got, wantStats)
	}
}
