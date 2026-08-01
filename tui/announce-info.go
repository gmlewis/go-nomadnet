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
	"time"

	"github.com/rivo/tview"
)

// AnnounceInfoData holds the directory-resolved fields Python's AnnounceInfo
// computes at view time (Network.py:76-78,96-100,138-144): the trust level +
// its palette style, the simplest display string, and (for nodes) the operator
// display string. The wiring layer resolves these from the app directory; the
// operator string needs RNS identity recall (Phase 5) and is "Unknown" until
// then.
type AnnounceInfoData struct {
	DisplayStr string // directory.simplest_display_str(source_hash)
	TrustStr   string // "Untrusted" / "Unknown" / "Trusted" / "Warning"
	TrustStyle string // palette key: list_untrusted/list_unknown/list_trusted/list_warning
	OpStr      string // node operator display (Phase 5: "Unknown")
}

// trustPaletteHex maps a trust palette key to the hex color tview color tags
// use for the trust string, matching the palette entries in palette.go /
// theme.go (list_trusted #6b2, list_unknown #bbb, list_untrusted #a22,
// list_warning #ba4 in the dark theme).
func trustPaletteHex(style string) string {
	switch style {
	case "list_trusted":
		return "#66bb22"
	case "list_untrusted":
		return "#aa2222"
	case "list_warning":
		return "#bbaa44"
	case "list_unknown":
		return "#bbbbbb"
	default:
		return "#bbaa44"
	}
}

// announceInfoDisplay ports Python's AnnounceInfo widget (Network.py:59-256):
// a TOP-filled Pile of left-aligned Text rows (Time/Addr/Type/Name/Oprtr/
// Trust), divider lines, the announce data block, and a weighted button row,
// shown inside the bordered "Announce Info" slot. The button set depends on
// the announce type: node → Back/Connect/Msg Op/Save, pn → Back/Use as
// default, peer → Back/Converse. Esc (handled by NetworkDisplay.HandleEsc)
// returns to the announce stream.
type announceInfoDisplay struct {
	nd   *NetworkDisplay
	ann  AnnounceEntry
	data AnnounceInfoData
	pile *tview.Flex
}

// newAnnounceInfoDisplay builds the AnnounceInfo Pile for the given announce.
// `data` carries the directory-resolved fields; buttons are wired to the
// NetworkDisplay's in-detail action methods (which pop back to the stream).
func newAnnounceInfoDisplay(nd *NetworkDisplay, ann AnnounceEntry, data AnnounceInfoData) *announceInfoDisplay {
	ai := &announceInfoDisplay{nd: nd, ann: ann, data: data}

	g := nd.glyphs()
	isNode := ann.Type == "node"
	isPN := ann.Type == "pn"

	tsStr := ann.Timestamp.Format("2006-01-02 15:04:05")
	addrStr := "<" + ann.SourceHash + ">"

	typeString := "Peer " + g["peer"]
	if isNode {
		typeString = "Nomad Network Node " + g["node"]
	} else if isPN {
		typeString = "LXMF Propagation Node " + g["sent"]
	}

	displayStr := data.DisplayStr
	if displayStr == "" {
		displayStr = ann.DisplayName
	}
	if displayStr == "" {
		displayStr = addrStr
	}

	// Announce data: strip_modifiers + truncate-to-32-with-" [...]" when the
	// source is not trusted (Network.py:93-100). strip_modifiers removes Micron
	// styling tags; the Go announce app data is already plain text, so we only
	// truncate.
	dataStr := ann.AppData
	if data.TrustStr != "Trusted" && len([]rune(dataStr)) > 32 {
		dataStr = string([]rune(dataStr)[:32]) + " [...]"
	}

	pile := tview.NewFlex().SetDirection(tview.FlexRow)

	// Common header rows (Network.py:236-239 / 227-230).
	pile.AddItem(textRow("Time  : "+tsStr), 1, 0, false)
	pile.AddItem(textRow("Addr  : "+addrStr), 1, 0, false)
	pile.AddItem(textRow("Type  : "+typeString), 1, 0, false)

	if isPN {
		// PN branch (Network.py:226-233): Time, Addr, Type, Divider, buttons.
		pile.AddItem(newDividerRow(g["divider1"]), 1, 0, false)
		pile.AddItem(ai.buttonRow(), 1, 0, true)
	} else {
		// Non-PN branch (Network.py:235-246): Name, [Oprtr for nodes], Trust,
		// Divider, Announce Data, Divider, buttons. Operator is inserted at
		// index 4 (between Name and Trust) for nodes (Network.py:248-250).
		pile.AddItem(textRow("Name  : "+displayStr), 1, 0, false)
		if isNode {
			pile.AddItem(textRow("Oprtr : "+data.OpStr), 1, 0, false)
		}
		pile.AddItem(ai.trustRow(), 1, 0, false)
		pile.AddItem(newDividerRow(g["divider1"]), 1, 0, false)
		pile.AddItem(ai.announceDataRow(dataStr), 2, 0, false)
		pile.AddItem(newDividerRow(g["divider1"]), 1, 0, false)
		pile.AddItem(ai.buttonRow(), 1, 0, true)
	}

	ai.pile = pile
	return ai
}

// Widget returns the Pile primitive to swap into the bordered list slot.
func (ai *announceInfoDisplay) Widget() tview.Primitive { return ai.pile }

// textRow returns a 1-row, left-aligned TextView rendering s in the default
// text style, matching urwid.Text(s, align=LEFT) (Network.py:236-241).
func textRow(s string) *tview.TextView {
	return tview.NewTextView().
		SetDynamicColors(false).
		SetTextAlign(tview.AlignLeft).
		SetText(s)
}

// trustRow returns the "Trust : <trust_str>" row with the trust string colored
// via the trust palette style (Network.py:241: Text(["Trust : ", (style,
// trust_str)])). "Trust : " stays in the default style; only trust_str is
// colored.
func (ai *announceInfoDisplay) trustRow() *tview.TextView {
	hex := trustPaletteHex(ai.data.TrustStyle)
	return tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft).
		SetText("Trust : [" + hex + "]" + ai.data.TrustStr + "[-]")
}

// announceDataRow returns the 2-row "Announce Data: \n<data>" block
// (Network.py:243: Text(["Announce Data: \n", (data_style, data_str)])). The
// label "Announce Data: " is row 0 (default style); the data is row 1. The
// data_style is "" on a normal decode and "list_untrusted" on decode failure
// (Network.py:95-100); a failure is not reachable from the Go port (app data
// is pre-decoded), so the data renders in the default style.
func (ai *announceInfoDisplay) announceDataRow(dataStr string) *tview.TextView {
	return tview.NewTextView().
		SetDynamicColors(false).
		SetTextAlign(tview.AlignLeft).
		SetText("Announce Data: \n" + dataStr)
}

// buttonRow builds the weighted button Columns matching Python's button_columns
// (Network.py:207-222). Node → Back/Connect/Msg Op/Save (weights 0.45/0.1/
// 0.45/0.1/0.45/0.1/0.45); pn → Back/Use as default (0.45/0.1/0.45); peer →
// Back/Converse (0.45/0.1/0.45). The fractional weights are scaled by 20 to
// integers (0.45→9, 0.1→2) which urwid's subtractive distribution maps to the
// same widths ([11,2,11,2,11,2,11] and [23,5,22] at inner 50). Buttons are flat
// urwid.Button "< label >" (NewUrwidButton); the spacer columns are empty
// Boxes. dividechars is 0 (urwid Columns default).
func (ai *announceInfoDisplay) buttonRow() *urwidColumns {
	isNode := ai.ann.Type == "node"
	isPN := ai.ann.Type == "pn"

	back := NewUrwidButton("Back").SetSelectedFunc(func() { ai.nd.showAnnounceStream() })

	if isNode {
		row := newURWIDColumns(0,
			back,
			tview.NewBox(),
			NewUrwidButton("Connect").SetSelectedFunc(func() { ai.nd.connectToNode(ai.ann) }),
			tview.NewBox(),
			NewUrwidButton("Msg Op").SetSelectedFunc(func() { ai.nd.msgOpNode(ai.ann) }),
			tview.NewBox(),
			NewUrwidButton("Save").SetSelectedFunc(func() { ai.nd.saveNode(ai.ann) }),
		).
			SetWeight(0, 9).SetWeight(1, 2).SetWeight(2, 9).
			SetWeight(3, 2).SetWeight(4, 9).SetWeight(5, 2).SetWeight(6, 9)
		return row
	}

	var action *UrwidButton
	if isPN {
		action = NewUrwidButton("Use as default").SetSelectedFunc(func() { ai.nd.useAsPN(ai.ann) })
	} else {
		action = NewUrwidButton("Converse").SetSelectedFunc(func() { ai.nd.converseWith(ai.ann) })
	}
	return newURWIDColumns(0, back, tview.NewBox(), action).
		SetWeight(0, 9).SetWeight(1, 2).SetWeight(2, 9)
}

// formatAnnounceInfoTimestamp is a small helper retained for tests that want
// the Python time-format string used by AnnounceInfo ("2006-01-02 15:04:05" in
// Go form, mirroring "%Y-%m-%d %H:%M:%S").
func formatAnnounceInfoTimestamp(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}
