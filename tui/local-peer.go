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

	"github.com/gmlewis/tcell/v2"
	"github.com/gmlewis/tview"
)

// LocalPeerDisplay is the "Local Peer Info" panel shown PACKed below the saved
// nodes list in the Network left pane. It mirrors Python's LocalPeer
// (Network.py:1259-1355): a titled LineBox wrapping a Pile of the LXMF address,
// identity hash, a name ReadlineEdit, a divider, the last-announce time, an
// "Announce Now" button, another divider, and a Save | Node Info button row.
type LocalPeerDisplay struct {
	app         *App
	widget      *tview.Flex // bordered "Local Peer Info" FlexRow wrapping pile
	pile        *pileFiller // the panel's urwid Pile (UP/DOWN walk its rows)
	lxmfAddr    *tview.TextView
	identity    *tview.TextView
	nameEdit    *ReadlineEdit
	announce    *tview.TextView
	announceBtn *UrwidButton
	saveBtn     *UrwidButton
	nodeInfoBtn *UrwidButton
	// buttons is the Save | Node Info Columns (urwid Columns: LEFT/RIGHT move
	// focus_column between the two buttons).
	buttons *urwidColumns

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
	// Python's LocalPeer e_name is a bare ReadlineEdit (no AttrMap,
	// Network.py:1273), so urwid renders it with NO background. tview's
	// InputField otherwise paints the text area with the contrast background
	// (blue), mismatching the original. Force the terminal-default background.
	lp.nameEdit.SetFieldBackgroundColor(tcell.ColorDefault)

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
	// urwidColumns (not a tview Flex) is required so LEFT/RIGHT move
	// focus_column between Save and Node Info, as urwid's Columns.keypress does.
	lp.buttons = newURWIDColumns(0, lp.saveBtn, tview.NewBox(), lp.nodeInfoBtn).
		SetFixedWidth(0, 23).
		SetFixedWidth(1, 5).
		SetFixedWidth(2, 22)

	glyph := "-"
	if app != nil && app.Glyphs != nil {
		glyph = app.Glyphs["divider1"]
	}

	// The panel body is an urwid Pile of fixed-height flow widgets, mirroring
	// Python's LocalPeer.display_widget Pile (Network.py:1305-1353). Its
	// focus_position defaults to the first selectable child — the Name edit —
	// and UP/DOWN walk Name → Announce Now → the Save | Node Info Columns, so
	// the Node Info button is reachable from the keyboard exactly as in the
	// original. The bordered LineBox wrapper stays outside the Pile (Python
	// wraps it in urwid.LineBox(..., title="Local Peer Info")).
	lp.pile = newPileFiller()
	lp.pile.AddItem(lp.lxmfAddr, 1, false)
	lp.pile.AddItem(lp.identity, 1, false)
	lp.pile.AddItem(lp.nameEdit, 1, true)
	lp.pile.AddItem(newDividerRow(glyph), 1, false)
	lp.pile.AddItem(lp.announce, 1, false)
	lp.pile.AddItem(lp.announceBtn, 1, true)
	lp.pile.AddItem(newDividerRow(glyph), 1, false)
	lp.pile.AddItem(lp.buttons, 1, true)

	lp.widget = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(lp.pile, 0, 1, true)
	lp.widget.SetBorder(true)
	SetTitledBorder(lp.widget, "Local Peer Info")

	return lp
}

// Widget returns the bordered "Local Peer Info" primitive for layout.
func (lp *LocalPeerDisplay) Widget() *tview.Flex { return lp.widget }

// Pile returns the panel's inner urwid Pile, so the Network left pane can wire
// its Up-escape (Up at the panel's first selectable row hands focus back to the
// list, urwid Pile.keypress bubbling the key out to left_pile.contents[0]).
func (lp *LocalPeerDisplay) Pile() *pileFiller { return lp.pile }

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

// The focus probes below report which of the panel's selectable rows currently
// holds focus. They are the read counterparts of the panel's urwid Pile focus
// positions (Network.py:1305-1353): the Name ReadlineEdit, the Announce Now
// button, and the Save | Node Info Columns, whose own focus_position selects
// Save or Node Info.

// NameEditHasFocus reports whether the panel's Name edit row holds focus.
func (lp *LocalPeerDisplay) NameEditHasFocus() bool { return lp.nameEdit.HasFocus() }

// AnnounceButtonHasFocus reports whether the Announce Now button holds focus.
func (lp *LocalPeerDisplay) AnnounceButtonHasFocus() bool { return lp.announceBtn.HasFocus() }

// SaveButtonHasFocus reports whether the Save button holds focus.
func (lp *LocalPeerDisplay) SaveButtonHasFocus() bool { return lp.saveBtn.HasFocus() }

// NodeInfoButtonHasFocus reports whether the Node Info button holds focus.
func (lp *LocalPeerDisplay) NodeInfoButtonHasFocus() bool { return lp.nodeInfoBtn.HasFocus() }

// ConsumesLeftRight reports whether the focused panel row handles Left/Right
// itself: the Name ReadlineEdit (a urwid Edit consumes them to move the cursor)
// and the Save | Node Info Columns (urwid Columns.keypress moves focus_column on
// LEFT/RIGHT). Every other row returns the keys unhandled, so the enclosing
// Network Columns then moves pane focus to the browser
// (urwid/widget/pile.py Pile.keypress + columns.py Columns.keypress).
func (lp *LocalPeerDisplay) ConsumesLeftRight() bool {
	return lp.nameEdit.HasFocus() || lp.buttons.HasFocus()
}

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
