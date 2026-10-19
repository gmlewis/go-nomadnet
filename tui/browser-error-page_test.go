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

func newErrorPageTestDisplay(t *testing.T) *BrowserDisplay {
	t.Helper()
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(screen.Fini)
	app.SetScreen(screen)
	screen.SetSize(100, 30)
	return NewBrowserDisplay(app)
}

// TestUnknownAnchorReportsInFooter pins Python Browser._jump_to_anchor
// (Browser.py:331-336): an anchor a page does not declare puts
// "Unknown anchor: #<name>" in the footer rather than doing nothing.
func TestUnknownAnchorReportsInFooter(t *testing.T) {
	t.Parallel()

	bd := newErrorPageTestDisplay(t)
	bd.RenderPage(">Section\n\nbody text\n")

	bd.JumpToAnchor("missing")
	if got, want := bd.FooterText(), "Unknown anchor: #missing"; got != want {
		t.Errorf("footer after an unknown anchor = %q, want %q", got, want)
	}
}

// TestRequestFailedPagePresentation pins Python
// Browser.get_request_failed_widget (Browser.py:530-547): the body becomes a
// centred "!" line followed by the status text, not a bare red message.
func TestRequestFailedPagePresentation(t *testing.T) {
	t.Parallel()

	bd := newErrorPageTestDisplay(t)
	bd.ShowRequestFailed("Request timed out")

	got := bd.content.GetText(true)
	if !strings.HasPrefix(got, "!") {
		t.Errorf("request-failed body = %q, want it to start with Python's %q marker", got, "!")
	}
	if !strings.Contains(got, "Request timed out") {
		t.Errorf("request-failed body = %q, want the status text", got)
	}

	// A later page render replaces the error body (and its alignment reset is
	// applied in renderPage; tview exposes no alignment getter to assert).
	bd.RenderPage(">Page\n\nbody\n")
	if got := bd.content.GetText(true); strings.HasPrefix(got, "!") {
		t.Errorf("page render kept the error body: %q", got)
	}
}
