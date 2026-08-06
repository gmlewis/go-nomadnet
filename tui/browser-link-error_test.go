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

// TestNotifyLinkErrorKeepsPage pins the parity gap where clicking a malformed
// link (e.g. an https:// URL, which ParseURL rejects as ErrMalformedURL) in the
// Go browser REPLACED the current page with the error text and left the user
// stranded: no history entry had been pushed for the failed link, so Back
// (Ctrl-d) had nothing to return to. Python's Browser.retrieve_url raises
// ValueError("Malformed URL") BEFORE touching status/destination/history, and
// handle_link (or url_dialog) catches it into the FOOTER ("Could not open
// link: ...") — the current page stays visible (Browser.py:300-304, 1142-1150),
// so Back is never needed and works as normal when there is history.
//
// NotifyLinkError mirrors that: it surfaces the error in the footer and leaves
// the rendered page (and its nav state) intact, instead of overwriting the
// content area.
func TestNotifyLinkErrorKeepsPage(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	bd := NewBrowserDisplay(app)
	bd.currentMarkup = navTestPage
	bd.renderPage()
	bd.content.SetRect(0, 0, 80, 6)
	app.SetFocus(bd.content)

	beforeText := bd.content.GetText(true)
	beforeFocus := bd.focusLine
	if beforeFocus < 0 {
		t.Fatalf("prerequisite: page should have a selectable line, focusLine=%d", beforeFocus)
	}

	// A malformed link (https://) fails URL parsing. The error must go to the
	// footer, NOT replace the page.
	bd.NotifyLinkError("Could not open link: malformed URL")

	if got := bd.content.GetText(true); got != beforeText {
		t.Errorf("page content was replaced by the error: got %q, want the rendered page unchanged",
			got)
	}
	if bd.focusLine != beforeFocus {
		t.Errorf("nav state disturbed: focusLine went %d -> %d (page should still be navigable)",
			beforeFocus, bd.focusLine)
	}
	// The footer surfaces the error.
	if got := bd.footerStatus.GetText(true); !strings.Contains(got, "Could not open link") {
		t.Errorf("footer after NotifyLinkError = %q, want it to contain the error message", got)
	}

	// An arrow key must still work on the intact page (no panic, focus moves).
	if res := bd.handleInput(key(tcell.KeyDown, 0)); res != nil {
		t.Errorf("Down not consumed on the intact page after a link error: %v", res)
	}
}

// TestNotifyLinkErrorRestoresLoadingSwap pins the URL-bar variant: LoadURL swaps
// in the "Retrieving" loading body (showLoading). A malformed URL entered there
// must restore the prior page (swap loading back out) and show the footer error,
// not leave "Retrieving" stuck and not replace the page with the error.
func TestNotifyLinkErrorRestoresLoadingSwap(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	bd := NewBrowserDisplay(app)
	bd.currentMarkup = navTestPage
	bd.renderPage()
	bd.content.SetRect(0, 0, 80, 6)

	beforeText := bd.content.GetText(true)

	// Simulate a URL-bar load of a malformed URL: showLoading swaps the page out
	// for the "Retrieving" body, then the fetch's ParseURL step fails.
	bd.showLoading("https://example.com")
	if bd.loading == nil {
		t.Fatal("showLoading did not install the loading body")
	}
	bd.NotifyLinkError("Could not open link: malformed URL")

	if bd.loading != nil {
		t.Fatalf("loading body still showing after NotifyLinkError — the prior page was not restored")
	}
	if got := bd.content.GetText(true); got != beforeText {
		t.Errorf("page content after restoring from loading = %q, want the prior page %q",
			got, beforeText)
	}
	if got := bd.footerStatus.GetText(true); !strings.Contains(got, "Could not open link") {
		t.Errorf("footer after NotifyLinkError = %q, want the error message", got)
	}
}