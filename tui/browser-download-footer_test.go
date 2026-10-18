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

	"github.com/gmlewis/tcell/v2"
)

// TestBrowserStatusTextSavedFile pins the footer line a completed /file/ download
// shows: Python's status_text returns "Saved "+saved_file_name+stats_string
// (Browser.py:1797-1798), where the stats use saved_file_size as the response
// size (:1761). Before the download path was wired the saved-file branch was
// unreachable — the only caller passed "" — so a download reported "Done ..."
// like a page fetch.
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
