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
	"github.com/rivo/tview"
)

// TestBrowserPaneDisconnectClearsLoading pins the regression where C-w on the
// Network page left the "Remote Node" right pane stuck on "Retrieving" forever
// after a connect to a non-responding node. The Network page's C-w was a no-op:
// networkDisplay.OnDisconnect was never wired, so the in-flight fetch was never
// cancelled and the loading body was never swapped out.
//
// BrowserPane.Disconnect mirrors Python Browser.disconnect → update_display
// DISCONECTED (Browser.py:862-888, 549-559): it cancels the underlying
// BrowserDisplay's in-flight fetch + clears history (BrowserDisplay.Disconnect,
// which swaps the loading body out via showContent), then resets the pane to
// the centered "Disconnected ← →" body with the "Remote Node" title.
func TestBrowserPaneDisconnectClearsLoading(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	bp := NewBrowserPane(app)
	bd := bp.BrowserDisplay()

	// A fetch is in flight: LoadURL showed the loading body (no OnRetrieveURL
	// wired, so the page never renders — the loading body stays).
	bp.LoadURL("7b75524d2453f2f5138947daaddb5b77/page/index.mu")
	if bd.loading == nil {
		t.Fatal("loading body not shown after LoadURL (bd.loading is nil)")
	}
	flex, ok := bp.Widget().(*tview.Flex)
	if !ok {
		t.Fatalf("expected *tview.Flex")
	}
	if got := flex.GetTitle(); got != " <7b75524d2453f2f5138947daaddb5b77> " {
		t.Fatalf("title after LoadURL = %q, want the node hash title", got)
	}

	// C-w / Disconnect from the Network page.
	bp.Disconnect()

	// The in-flight fetch is cancelled and the loading body swapped out.
	if bd.loading != nil {
		t.Errorf("after Disconnect, bd.loading is still non-nil — " +
			"the \"Retrieving\" body is never swapped out, so the browser stays stuck")
	}
	// The pane resets to the disconnected view with the "Remote Node" title.
	if got := flex.GetTitle(); got != " Remote Node " {
		t.Errorf("title after Disconnect = %q, want \" Remote Node \"", got)
	}
	// The centered "Disconnected" body is back in the pane (the BrowserDisplay
	// widget that LoadURL swapped in is gone).
	rows := renderPrimitive(t, bp.Widget(), 28, 22)
	if !rowContains(rows, "Disconnected") {
		t.Errorf("disconnected body not shown after Disconnect; rows:\n%s", joinRows(rows))
	}
	if rowContains(rows, "Retrieving") {
		t.Errorf("\"Retrieving\" still visible after Disconnect; rows:\n%s", joinRows(rows))
	}
}

// TestNetworkCtrlWDisconnectsBrowserPane pins the wiring: C-w on the Network
// page (focus on the left list) routes through NetworkDisplay.handleInput →
// OnDisconnect, which must disconnect the Network right-pane browser. The
// wiring layer (cmd/gonomadnet/textui.go) sets OnDisconnect = BrowserPane's
// Disconnect; this test exercises that path end-to-end at the TUI level.
func TestNetworkCtrlWDisconnectsBrowserPane(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	nd := NewNetworkDisplay(app, nil, nil)

	// Wire OnDisconnect exactly as cmd/gonomadnet/textui.go does.
	nd.OnDisconnect = func() {
		if bp := nd.BrowserPane(); bp != nil {
			bp.Disconnect()
		}
	}

	bp := nd.BrowserPane()
	bd := nd.BrowserDisplay()
	bp.LoadURL("7b75524d2453f2f5138947daaddb5b77/page/index.mu")
	if bd.loading == nil {
		t.Fatal("loading body not shown after LoadURL")
	}

	// C-w from the left list (the focus position after connectToNode closes the
	// Announce Info dialog via focusLeftList).
	res := nd.handleInput(tcell.NewEventKey(tcell.KeyCtrlW, 0, tcell.ModNone))
	if res != nil {
		t.Errorf("C-w was not consumed by handleInput")
	}
	if bd.loading != nil {
		t.Errorf("after C-w, bd.loading is still non-nil — OnDisconnect did not " +
			"disconnect the Network browser pane (browser stays stuck on Retrieving)")
	}
	flex, ok := bp.Widget().(*tview.Flex)
	if !ok {
		t.Fatalf("expected *tview.Flex")
	}
	if got := flex.GetTitle(); got != " Remote Node " {
		t.Errorf("title after C-w = %q, want \" Remote Node \"", got)
	}
}

func rowContains(rows []string, s string) bool {
	for _, r := range rows {
		if strings.Contains(r, s) {
			return true
		}
	}
	return false
}

func joinRows(rows []string) string {
	out := ""
	for _, r := range rows {
		out += r + "\n"
	}
	return out
}
