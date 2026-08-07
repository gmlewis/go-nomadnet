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
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// localPeerLabelWidth is the column width of the "LXMF Addr : ", "Identity  : "
// and "Name      : " labels so the colons line up, matching Python's LocalPeer
// (Network.py:1278-1282).
const localPeerLabelWidth = 12

// LocalPeerDisplay is the "Local Peer Info" panel shown PACKed below the saved
// nodes list in the Network left pane. It mirrors Python's LocalPeer
// (Network.py:1259-1355): a titled LineBox wrapping a Pile of the LXMF address,
// identity hash, a name ReadlineEdit, a divider, the last-announce time, an
// "Announce Now" button, another divider, and a Save | Node Info button row.
type LocalPeerDisplay struct {
	app         *App
	widget      *tview.Flex // bordered "Local Peer Info" FlexRow
	lxmfAddr    *tview.TextView
	identity    *tview.TextView
	nameEdit    *ReadlineEdit
	announce    *tview.TextView
	announceBtn *UrwidButton
	saveBtn     *UrwidButton
	nodeInfoBtn *UrwidButton

	// Callbacks fired by the buttons; the wiring layer connects these to the
	// app (set_display_name / announce_now / show NodeInfo panel).
	OnSave     func(name string)
	OnAnnounce func()
	OnNodeInfo func()
}

// NewLocalPeerDisplay builds the Local Peer Info panel. lxmfAddr and
// identityHash are already prettyhexrep-formatted ("<hex>"); name is the
// current display name (may be ""); lastAnnounce is the last announce time
// (zero time → "Never").
func NewLocalPeerDisplay(app *App, lxmfAddr, identityHash, name string, lastAnnounce time.Time) *LocalPeerDisplay {
	lp := &LocalPeerDisplay{app: app}

	lp.lxmfAddr = tview.NewTextView().
		SetDynamicColors(false).
		SetText("LXMF Addr : " + lxmfAddr)
	lp.identity = tview.NewTextView().
		SetDynamicColors(false).
		SetText("Identity  : " + identityHash)

	if app != nil && app.killRing != nil {
		lp.nameEdit = NewReadlineEdit(app.killRing, "Name      : ", "")
	} else {
		lp.nameEdit = NewReadlineEdit(&killRing{}, "Name      : ", "")
	}
	lp.nameEdit.SetText(name)

	lp.announce = tview.NewTextView().
		SetDynamicColors(false).
		SetText(lp.announceLine(lastAnnounce))

	lp.announceBtn = NewUrwidButton("Announce Now").
		SetSelectedFunc(func() {
			if lp.OnAnnounce != nil {
				lp.OnAnnounce()
			}
		})

	lp.saveBtn = NewUrwidButton("Save").
		SetSelectedFunc(func() {
			if lp.OnSave != nil {
				lp.OnSave(lp.nameEdit.GetText())
			}
		})
	lp.nodeInfoBtn = NewUrwidButton("Node Info").
		SetSelectedFunc(func() {
			if lp.OnNodeInfo != nil {
				lp.OnNodeInfo()
			}
		})

	// Button row: Save (0.45) | spacer (0.10) | Node Info (0.45), matching
	// Python's Columns weights (Network.py:1343-1347). The Local Peer Info
	// panel lives in the 52-wide Network left pane (inner 50), so the columns
	// are fixed at the urwid-computed sizes: 0.45*50=22.5 floors to 22 each,
	// 0.1*50=5, and urwid gives the 1-col leftover to the FIRST column →
	// Save=23, spacer=5, Node Info=22. tview's proportional Flex gives the
	// leftover to the LAST column instead, so fixed sizes are used for parity.
	buttons := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(lp.saveBtn, 23, 0, false).
		AddItem(tview.NewBox(), 5, 0, false).
		AddItem(lp.nodeInfoBtn, 22, 0, false)

	glyph := "-"
	if app != nil && app.Glyphs != nil {
		glyph = app.Glyphs["divider1"]
	}

	lp.widget = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(lp.lxmfAddr, 1, 0, false).
		AddItem(lp.identity, 1, 0, false).
		AddItem(lp.nameEdit, 1, 0, true).
		AddItem(newDividerRow(glyph), 1, 0, false).
		AddItem(lp.announce, 1, 0, false).
		AddItem(lp.announceBtn, 1, 0, false).
		AddItem(newDividerRow(glyph), 1, 0, false).
		AddItem(buttons, 1, 0, false)
	lp.widget.SetBorder(true)
	SetTitledBorder(lp.widget, "Local Peer Info")

	return lp
}

// Widget returns the bordered "Local Peer Info" primitive for layout.
func (lp *LocalPeerDisplay) Widget() *tview.Flex { return lp.widget }

// Height returns the fixed rendered height of the panel: 8 content rows plus 2
// border rows = 10. Callers use this as the tview Flex fixedSize so the panel
// sits PACK-style below the saved-nodes list.
func (lp *LocalPeerDisplay) Height() int { return 10 }

// SetLastAnnounce refreshes the "Announced : …" line.
func (lp *LocalPeerDisplay) SetLastAnnounce(t time.Time) {
	lp.announce.SetText(lp.announceLine(t))
}

// SetName seeds the name edit field with the current display name. It is called
// ONCE by the wiring layer when the Network display is first wired (from
// app.GetDisplayName(), which reads peer_settings loaded synchronously during
// Init), mirroring Python's LocalPeer.__init__ which sets e_name.edit_text once
// at construction (Network.py:1271) and never re-sets it. The name is
// deliberately NOT part of SetData: incoming announces/messages fire
// UIChangeCallback → SetData on every event, so including the name there would
// overwrite the user's in-progress typing and jump the cursor to the end. The
// name is only ever re-seeded across a full app restart (when LocalPeer is
// reconstructed from the saved peer_settings display name).
func (lp *LocalPeerDisplay) SetName(name string) {
	lp.nameEdit.SetText(name)
}

// SetData refreshes the read-only Local Peer Info fields: the LXMF address,
// identity hash (prettyhexrep-formatted "<hex>"), and the last-announce time.
// It intentionally does NOT touch the name edit — see SetName. This mirrors
// Python's LocalPeer, which never re-sets e_name.edit_text after construction
// (Network.py:1271); only the announce time and identity fields update on a
// refresh.
func (lp *LocalPeerDisplay) SetData(lxmfAddr, identityHash string, lastAnnounce time.Time) {
	lp.lxmfAddr.SetText("LXMF Addr : " + lxmfAddr)
	lp.identity.SetText("Identity  : " + identityHash)
	lp.announce.SetText(lp.announceLine(lastAnnounce))
}

// Name returns the current text of the name edit field.
func (lp *LocalPeerDisplay) Name() string { return lp.nameEdit.GetText() }

// announceLine formats the "Announced : <pretty>" line: "Never" when no
// announce has been recorded (zero time), otherwise PrettyDate of the stamp —
// matching Python's AnnounceTime.update_time (Network.py:1036-1040).
func (lp *LocalPeerDisplay) announceLine(t time.Time) string {
	if t.IsZero() {
		return "Announced : Never"
	}
	return "Announced : " + PrettyDate(t)
}

// newDividerRow returns a 1-row primitive that fills its width with the given
// divider glyph, matching urwid.Divider(glyph) (Network.py:1283,1331). The
// glyph is drawn in the default text style across the full inner width.
func newDividerRow(glyph string) tview.Primitive {
	box := tview.NewBox()
	box.SetDrawFunc(func(screen tcell.Screen, x, y, w, h int) (int, int, int, int) {
		if w > 0 && h > 0 {
			tview.Print(screen, strings.Repeat(glyph, w), x, y, w, tview.AlignLeft, tcell.ColorDefault)
		}
		return x, y, w, h
	})
	return box
}
