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
)

// TestD2CtrlUDelegatesToBrowserDialog pins D2: C-u must open the browser's
// URL dialog PRE-FILLED with the current page URL. Python routes "ctrl u" to
// browser.url_dialog() from BOTH the NetworkLeftPile (Network.py:1607-1608)
// and the BrowserFrame (Browser.py:30-33); url_dialog pre-fills the edit with
// self.current_url() (Browser.py:1136) — so with a page loaded the field shows
// "URL : <hash>:/page/index.mu". The Go mainCols input capture used to
// intercept C-u first and fire the display-level handler, which always opened
// the dialog with an EMPTY field.
func TestD2CtrlUDelegatesToBrowserDialog(t *testing.T) {
	t.Parallel()

	app := NewApp(ThemeDark, GlyphUnicode, ColorModeTrue)
	nd := NewNetworkDisplay(app, nil, nil)
	app.Main.SetDisplay("network", nd.Widget())
	app.SetRoot()

	bd := nd.BrowserDisplay()
	if bd == nil {
		t.Fatal("network display has no browser display")
	}

	// Simulate a connected page: the browser history holds the current URL.
	url := "d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2d2:/page/index.mu"
	bd.LoadURL(url)
	if got := bd.CurrentURL(); got != url {
		t.Fatalf("CurrentURL = %q, want %q (browser must be connected)", got, url)
	}

	firedBrowser, firedDisplay := false, false
	bd.OnURLDialog = func() { firedBrowser = true }
	nd.OnURLDialog = func() { firedDisplay = true }

	// C-u through the network display's input capture (the runtime dispatch
	// reaches nd.handleInput first).
	nd.handleInput(tcell.NewEventKey(tcell.KeyCtrlU, 0, tcell.ModNone))

	if !firedBrowser {
		t.Error("C-u did not delegate to the browser frame's URL dialog (pre-filled variant)")
	}
	if firedDisplay {
		t.Error("C-u fired the display-level (empty pre-fill) dialog instead")
	}

	// With no page connected the browser dialog still fires (empty pre-fill,
	// matching Python current_url() == "").
	bd2 := nd.BrowserDisplay()
	bd2.LoadURL("")
	firedBrowser = false
	nd.handleInput(tcell.NewEventKey(tcell.KeyCtrlU, 0, tcell.ModNone))
	if !firedBrowser {
		t.Error("C-u with no connected page must still open the browser URL dialog (empty pre-fill)")
	}
}
