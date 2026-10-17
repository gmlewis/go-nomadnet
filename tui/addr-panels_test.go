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
	"time"

	"github.com/gmlewis/tcell/v2"
	"github.com/gmlewis/tview"
)

// addrRowIn returns the first row in the primitive tree whose text carries an
// address region tag — the row a click on an address resolves against.
func addrRowIn(p tview.Primitive) *tview.TextView {
	switch v := p.(type) {
	case *tview.TextView:
		if strings.Contains(v.GetText(false), `["`+addrRegionPrefix) {
			return v
		}
	case *pileFiller:
		for _, it := range v.items {
			if row := addrRowIn(it.widget); row != nil {
				return row
			}
		}
	case *urwidColumns:
		for _, child := range v.children {
			if row := addrRowIn(child); row != nil {
				return row
			}
		}
	case *tview.Flex:
		for i := range v.GetItemCount() {
			if row := addrRowIn(v.GetItem(i)); row != nil {
				return row
			}
		}
	}
	return nil
}

// centerAddrLineIn returns the first centered ulwid text row in the tree that
// has an address click hook installed.
func centerAddrLineIn(p tview.Primitive) *urwidCenterText {
	switch v := p.(type) {
	case *urwidCenterText:
		if v.GetMouseCapture() != nil {
			return v
		}
	case *pileFiller:
		for _, it := range v.items {
			if line := centerAddrLineIn(it.widget); line != nil {
				return line
			}
		}
	case *urwidColumns:
		for _, child := range v.children {
			if line := centerAddrLineIn(child); line != nil {
				return line
			}
		}
	case *tview.Flex:
		for i := range v.GetItemCount() {
			if line := centerAddrLineIn(v.GetItem(i)); line != nil {
				return line
			}
		}
	}
	return nil
}

// drawPanel lays a panel out at w×h and draws it, so every row has its rect and
// tview has resolved the address regions.
func drawPanel(t *testing.T, p tview.Primitive, w, h int) {
	t.Helper()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	t.Cleanup(screen.Fini)
	screen.SetSize(w, h)
	p.SetRect(0, 0, w, h)
	p.Draw(screen)
}

// clickAddrInPanel clicks the address of the panel's clickable address row and
// returns the address it copied. The event goes to the PANEL's handler, not the
// row's, so the whole container chain a real click travels — Flex → pileFiller →
// row — is exercised, exactly as tview routes it from the root.
func clickAddrInPanel(t *testing.T, app *App, panel tview.Primitive, w, h int) string {
	t.Helper()
	drawPanel(t, panel, w, h)
	row := addrRowIn(panel)
	if row == nil {
		t.Fatal("no address row found in the panel")
	}
	addr, start, _, ok := addrSpan(row.GetText(true))
	if !ok {
		t.Fatalf("row %q carries no address", row.GetText(true))
	}
	x, y, _, _ := row.GetRect()
	clickPrimitive(app, panel, x+start, y)
	return addr
}

// TestLocalPeerPanelAddrRowIsClickable drives the Local Peer Info panel end to
// end: a click on the LXMF address row copies the bare address and opens the QR
// modal, without any knowledge of the row's internals — the test finds the row
// the way a user's click does.
func TestLocalPeerPanelAddrRowIsClickable(t *testing.T) {
	t.Parallel()
	app, fake, _ := addrTestApp(t)

	lp := NewLocalPeerDisplay(app, "<"+testAddr+">", "<deadbeef>", "Alice", time.Time{})
	got := clickAddrInPanel(t, app, lp.Widget(), networkListWidth, lp.Height())

	if got != testAddr {
		t.Fatalf("clicked address = %q, want %q", got, testAddr)
	}
	if len(fake.texts) != 1 || fake.texts[0] != testAddr {
		t.Errorf("clipboard writes = %q, want exactly [%q]", fake.texts, testAddr)
	}
	if n := len(app.Dialogs.stack); n != 1 {
		t.Errorf("open dialogs = %v, want 1", n)
	}
}

// TestLocalPeerPanelLeavesTruncatedHashAlone pins that the identity row's
// placeholder hash — not a full 16-byte address — is left inert, so a panel can
// show a short hash without pretending it can be sent anywhere.
func TestLocalPeerPanelLeavesTruncatedHashAlone(t *testing.T) {
	t.Parallel()
	app := newTestApp()

	lp := NewLocalPeerDisplay(app, "<deadbeef>", "<cafef00d>", "Alice", time.Time{})
	drawPanel(t, lp.Widget(), networkListWidth, lp.Height())

	if row := addrRowIn(lp.Widget()); row != nil {
		t.Errorf("row %q was made clickable, want no address row", row.GetText(true))
	}
}

// TestNodeInfoPanelAddrRowIsClickable drives the Local Node Info panel: the
// node's own address row is clickable, and the centered LXMF propagation line
// carries the click hook (its geometry is pinned separately, in
// addr-click_test.go).
func TestNodeInfoPanelAddrRowIsClickable(t *testing.T) {
	t.Parallel()
	app, fake, _ := addrTestApp(t)
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	app.GlyphSet = GlyphUnicode

	data := nodeInfoHostingData()
	data.Addr = testAddr
	data.LXMFPropAddr = "<" + testAddr + ">"
	ni := NewNodeInfoDisplay(app, data)

	got := clickAddrInPanel(t, app, ni.Widget(), networkListWidth, ni.Height())
	if got != testAddr {
		t.Fatalf("clicked address = %q, want %q", got, testAddr)
	}
	if len(fake.texts) != 1 || fake.texts[0] != testAddr {
		t.Errorf("clipboard writes = %q, want exactly [%q]", fake.texts, testAddr)
	}
	if n := len(app.Dialogs.stack); n != 1 {
		t.Errorf("open dialogs = %v, want 1", n)
	}

	if line := centerAddrLineIn(ni.Widget()); line == nil {
		t.Error("the LXMF propagation address line has no click hook")
	}
}
