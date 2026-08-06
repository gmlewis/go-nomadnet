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

import "testing"

// TestBrowserLoadingSwapOnFailure pins the regression where the browser gets
// STUCK on the "Retrieving" loading body after a fetch fails (timeout/error) or
// the user disconnects (C-w) while a request is in flight.
//
// displayURL/showLoading removes bd.content from bd.body and inserts the
// centered "Retrieving\n[<url>]" loading primitive (bd.loading) in its place.
// Only renderPage (success) swapped it back via showContent. The fetch-failure
// path (OnBrowserError → SetContent) and Disconnect set text on bd.content but
// never called showContent, so bd.loading stayed in the body and the
// error/"Disconnected." text on bd.content was invisible — the browser appeared
// permanently "Retrieving" with no timeout ever surfacing to the user, even
// though the underlying fetch had already returned.
func TestBrowserLoadingSwapOnFailure(t *testing.T) {
	t.Parallel()
	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	bd := NewBrowserDisplay(app)

	// A fetch is in flight: displayURL showed the loading body.
	bd.showLoading("dead.node")
	if bd.loading == nil {
		t.Fatal("showLoading did not install the loading body")
	}

	// The fetch fails (timeout / no path / link error). SetContent is the
	// app-layer failure callback (cmd/gonomadnet/textui.go OnBrowserError).
	bd.SetContent("[red]timed out[-]")
	if bd.loading != nil {
		t.Errorf("after SetContent on a failed fetch, bd.loading is still non-nil — " +
			"the \"Retrieving\" body is never swapped out, so the browser stays stuck")
	}
	if got := bd.content.GetText(true); got != "timed out" {
		t.Errorf("bd.content text after SetContent = %q, want the error text", got)
	}

	// A second in-flight fetch, then the user disconnects (C-w).
	bd.showLoading("dead.node:2")
	if bd.loading == nil {
		t.Fatal("showLoading did not reinstall the loading body")
	}
	bd.Disconnect()
	if bd.loading != nil {
		t.Errorf("after Disconnect while loading, bd.loading is still non-nil — " +
			"the \"Retrieving\" body is never swapped out, so the browser stays stuck " +
			"after C-w on a non-responding node")
	}
	if got := bd.content.GetText(true); got != "Disconnected." {
		t.Errorf("bd.content text after Disconnect = %q, want \"Disconnected.\"", got)
	}
}
