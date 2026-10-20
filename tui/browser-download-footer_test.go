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

	"github.com/gmlewis/go-nomadnet/nomadnet/micron"
	"github.com/gmlewis/tcell/v2"
)

// TestBrowserStatusTextSavedFile pins the footer line a completed /file/ download
// shows: Python's status_text returns "Saved "+saved_file_name+stats_string
// (Browser.py:1797-1798), where the stats use saved_file_size as the response
// size (:1761). Without a saved name the same numbers produce the "Done ..."
// line a page fetch shows.
func TestBrowserStatusTextSavedFile(t *testing.T) {
	t.Parallel()

	g := GetGlyphSet(GlyphUnicode)
	if g == nil {
		t.Fatal("no unicode glyph set")
	}

	got := browserStatusText(g, browserDone, 1234, 1234, 2.5, true, false, "report.pdf")

	if !strings.HasPrefix(got, "Saved report.pdf") {
		t.Errorf("status text = %q, want it to start with %q", got, "Saved report.pdf")
	}
	// The size shown is the SAVED file's size, and the transfer/time/speed
	// segments follow exactly as they do for a page.
	for _, want := range []string{sizeStr(1234, "B"), sizeStr(1234, "B") + " in 2.50", "/s"} {
		if !strings.Contains(got, want) {
			t.Errorf("status text %q is missing %q", got, want)
		}
	}

	// Without a saved name the same numbers produce the "Done" line.
	plain := browserStatusText(g, browserDone, 1234, 1234, 2.5, true, false, "")
	if !strings.HasPrefix(plain, "Done") {
		t.Errorf("status text without a saved file = %q, want it to start with %q", plain, "Done")
	}
}

// TestBrowserShowsDownloadAndClearsItOnPageRender pins the display half of the
// download path: ShowDownload puts "Saved <name>" in the footer with the file's
// size, and the next page render clears it again, because Python resets
// saved_file_name on every page load (Browser.py:1278,1357,1431) and the status
// text then returns to "Done ...".
func TestBrowserShowsDownloadAndClearsItOnPageRender(t *testing.T) {
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	app.SetScreen(screen)
	screen.SetSize(100, 30)

	bd := NewBrowserDisplay(app)

	bd.ShowDownload("notes.txt", 4096, 1.25)
	if footer := bd.footerStatus.GetText(true); !strings.HasPrefix(footer, "Saved notes.txt") {
		t.Errorf("footer after a download = %q, want it to start with %q", footer, "Saved notes.txt")
	}
	if !strings.Contains(bd.footerStatus.GetText(true), sizeStr(4096, "B")) {
		t.Errorf("footer after a download = %q, want the saved size %q", bd.footerStatus.GetText(true), sizeStr(4096, "B"))
	}

	bd.RenderPage(">>A page\n")
	if footer := bd.footerStatus.GetText(true); strings.Contains(footer, "Saved ") {
		t.Errorf("footer after a page render = %q, want the download line cleared", footer)
	}
}

// TestPageFgDirectiveColorsBodyText pins `#!fg=`: the directive is the parse state's
// starting color, so every plain run takes it. Python feeds the page colors into
// markup_to_attrmaps as fg_color/bg_color (Browser.py:1269,1348,1513 →
// MicronParser.py:104-108,41-45,63-64). Setting them only as the TextView default
// is not enough, because every micron span carries an explicit color and would
// override it.
func TestPageFgDirectiveColorsBodyText(t *testing.T) {
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(screen.Fini)
	app.SetScreen(screen)
	screen.SetSize(80, 24)

	bd := NewBrowserDisplay(app)

	// The directive must be terminated by a newline to be recognised, and it applies to
	// the whole page.
	bd.RenderPage("#!fg=f00\n>>A Page\n\nplain body text\n")
	plain := firstPlainSpan(t, bd)
	if plain.FG != "#ff0000" {
		t.Errorf("plain run FG = %v, want the page's #!fg= color #ff0000", plain.FG)
	}

	// Without the directive the theme's plain color is used, so the assertion above is
	// about the directive rather than a constant.
	bd.RenderPage(">>A Page\n\nplain body text\n")
	plain = firstPlainSpan(t, bd)
	if plain.FG != "#dddddd" {
		t.Errorf("plain run FG = %v without a directive, want the theme plain #dddddd", plain.FG)
	}
}

// firstPlainSpan returns the first span of the rendered body that carries text and no
// heading styling — the run the page's #!fg= applies to.
func firstPlainSpan(t *testing.T, bd *BrowserDisplay) micron.StyledSpan {
	t.Helper()
	for _, line := range bd.currentLines {
		if line.HeadingLevel > 0 || line.Divider {
			continue
		}
		for _, span := range line.Spans {
			if strings.TrimSpace(span.Text) != "" && span.Link == nil {
				return span
			}
		}
	}
	t.Fatal("no plain span found in the rendered page")
	return micron.StyledSpan{}
}

// TestBrowserStatusLabelsMatchPython pins the in-flight footer labels against the
// reference. Python's Browser.status_text returns one fixed string per lifecycle state
// (SOT: ui/textui/Browser.py:1756-1803, nomadnet 1.2.8); they are user-visible, so the
// wording has to match rather than merely exist. Each fragment was checked verbatim
// against the installed reference.
//
// The transitions are wired: fetchBytes reports each Browser status transition and
// cmd/gonomadnet/textui.go feeds them to BrowserDisplay.SetBrowserFetchStage. This test
// pins the words, and TestBrowserFetchStageReachesTheFooter pins the stage mapping.
func TestBrowserStatusLabelsMatchPython(t *testing.T) {
	t.Parallel()

	g := GetGlyphSet(GlyphUnicode)
	if g == nil {
		t.Fatal("no unicode glyph set")
	}

	tests := []struct {
		status browserStatus
		want   string
	}{
		{browserNoPath, "No path to destination known"},
		{browserPathRequested, "Path requested, waiting for path..."},
		{browserEstablishingLink, "Establishing link..."},
		{browserLinkTimeout, "Link establishment timed out"},
		{browserLinkEstablished, "Link established"},
		{browserRequesting, "Sending request..."},
		{browserRequestSent, "Request sent, awaiting response..."},
		{browserRequestFailed, "Request failed"},
		{browserRequestTimeout, "Request timed out"},
		{browserReceivingResponse, "Receiving response..."},
		{browserDisconnected, "Disconnected"},
	}

	for _, tc := range tests {
		if got := browserStatusText(g, tc.status, 0, 0, 0, false, false, ""); got != tc.want {
			t.Errorf("status %v label = %q, want %q", tc.status, got, tc.want)
		}
	}
}

// TestBrowserStatusReachesTheFooter pins the display half: a status set on the display is
// what the footer renders, so the user-visible footer carries the status the fetch path
// reports.
func TestBrowserStatusReachesTheFooter(t *testing.T) {
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(screen.Fini)
	app.SetScreen(screen)
	screen.SetSize(100, 30)

	bd := NewBrowserDisplay(app)

	bd.SetBrowserStatus(int(browserNoPath))
	if got := bd.FooterText(); got != "No path to destination known" {
		t.Errorf("footer after SetBrowserStatus(no path) = %q, want %q", got, "No path to destination known")
	}
	bd.SetBrowserStatus(int(browserRequestSent))
	if got := bd.FooterText(); got != "Request sent, awaiting response..." {
		t.Errorf("footer after SetBrowserStatus(request sent) = %q, want %q", got, "Request sent, awaiting response...")
	}
}
