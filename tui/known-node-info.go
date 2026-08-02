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
	"github.com/rivo/tview"
)

// KnownNodeInfoData holds the directory-resolved fields Python's KnownNodeInfo
// computes at view time (Network.py:612-740): the display name, sort rank, trust
// level (+ display string + palette style), operator display, hop distance, the
// LXMF propagation-node address line, and the two checkbox preselections (use as
// default PN, identify on connect). The RNS-dependent fields (operator string
// via Identity.recall, hop count via Transport.hops_to, the PN address hash, the
// current user-selected PN) are stubs until Phase 5; identify-on-connect comes
// from the directory entry and is wired now.
type KnownNodeInfoData struct {
	DisplayStr        string // directory display name (or "<hex>")
	SortStr           string // "None" or str(sort_rank)
	TrustLevel        string // "untrusted"/"unknown"/"trusted"/"warning" (radio preselect)
	OpStr             string // node operator display (Phase 5: "Unknown")
	HopsStr           string // "N hop(s)" or "Unknown" (Phase 5: "Unknown")
	LXMFAddrStr       string // centered PN-address line (Phase 5 stub: "No associated …")
	UseAsPN           bool   // preselected "Use as default propagation node" (Phase 5: false)
	IdentifyOnConnect bool   // directory.should_identify_on_connect(source_hash)
}

// KnownNodeInfoFormData is the edited form state collected when the user presses
// Save (Python save_node, Network.py:755-785): the edited name, edited sort
// rank, the trust level selected by the radio group, and the two checkbox
// states. The wiring layer writes these to the directory.
type KnownNodeInfoFormData struct {
	Name              string
	SortRank          string
	TrustLevel        string
	UseAsPN           bool
	UseAsPNChanged    bool // true iff the user toggled the PN checkbox (Python pn_changed)
	IdentifyOnConnect bool
}

// knownNodeInfoDisplay ports Python's KnownNodeInfo widget (Network.py:593-810):
// an editable form shown in the bordered "Node Info" left-pane slot when the
// user presses Ctrl-E on a saved node. A TOP-filled Pile of left-aligned Text
// rows (Type / Node Addr / Operator / Distance), two ReadlineEdits (Name / Sort
// Rank), a centered LXMF-PN-address line, two CheckBoxes (default PN / identify
// on connect), a RadioButton trust group (Untrusted / Unknown / Trusted), and a
// weighted Back/Connect/Msg Op/Save button row. Labels align the colon at
// column 10 ("Type      : ", "Name      : ", "Node Addr : ", "Operator  : ",
// "Distance  : ", "Sort Rank : "). Operator + Distance are inserted at indices 3
// and 4 (Network.py:789,797). The Pile focus is the button row
// (pile.focus_position = len-1), with the Back button focused within it.
//
// The Pile is wrapped in a pileFiller (urwid.Filler(valign=TOP, height=PACK)) so
// overflow is top-trimmed to keep the focused item visible, and Tab/Up/Down
// cycle focus among the selectable fields (ReadlineEdits, CheckBoxes,
// RadioButtons, button row). Esc returns to the saved-nodes list.
type knownNodeInfoDisplay struct {
	nd         *NetworkDisplay
	nodeHash   string
	data       KnownNodeInfoData
	nameEdit   *ReadlineEdit
	sortEdit   *ReadlineEdit
	pnCheck    *tview.Checkbox
	idCheck    *tview.Checkbox
	rUntrusted *RadioButton
	rUnknown   *RadioButton
	rTrusted   *RadioButton
	pile       *pileFiller
	pnChanged  bool // set when the PN checkbox is toggled (Python pn_changed)
}

// newKnownNodeInfoDisplay builds the KnownNodeInfo form for the given node hash.
// `data` carries the directory-resolved fields; buttons are wired to the
// NetworkDisplay's known-node action handlers (which pop back to the list).
func newKnownNodeInfoDisplay(nd *NetworkDisplay, nodeHash string, data KnownNodeInfoData) *knownNodeInfoDisplay {
	k := &knownNodeInfoDisplay{nd: nd, nodeHash: nodeHash, data: data}

	g := nd.glyphs()
	addrStr := "<" + nodeHash + ">"
	typeString := "Nomad Network Node " + g["node"]

	// ReadlineEdits with the kill ring (mirrors local-peer.go:68-71).
	var kr *killRing
	if nd.app != nil && nd.app.killRing != nil {
		kr = nd.app.killRing
	} else {
		kr = &killRing{}
	}
	k.nameEdit = NewReadlineEdit(kr, "Name      : ", "")
	k.nameEdit.SetText(data.DisplayStr)
	k.sortEdit = NewReadlineEdit(kr, "Sort Rank : ", "")
	k.sortEdit.SetText(data.SortStr)

	// Trust radio group (Network.py:660-663). Warning leaves none preselected.
	group := &DialogRadioGroup{}
	k.rUntrusted = NewRadioButton(group, "Untrusted", data.TrustLevel == "untrusted", false)
	k.rUnknown = NewRadioButton(group, "Unknown", data.TrustLevel == "unknown", false)
	k.rTrusted = NewRadioButton(group, "Trusted", data.TrustLevel == "trusted", false)

	// CheckBoxes (Network.py:727-728). The PN checkbox records whether the user
	// toggled it (pn_changed, Network.py:720-721) so Save only acts on it when
	// the user actually changed it, matching Python.
	k.pnCheck = tview.NewCheckbox().
		SetLabel("Use as default propagation node").
		SetChecked(data.UseAsPN).
		SetChangedFunc(func(checked bool) { k.pnChanged = true })
	k.idCheck = tview.NewCheckbox().
		SetLabel("Identify when connecting").
		SetChecked(data.IdentifyOnConnect)

	// LXMF PN address line (centered, Network.py:643-647).
	lxmfRow := tview.NewTextView().
		SetDynamicColors(false).
		SetTextAlign(tview.AlignCenter).
		SetText(data.LXMFAddrStr)

	pile := newPileFiller()

	// Base pile_widgets (Network.py:766-782) then insert Operator at index 3
	// and Distance at index 4 (Network.py:789,797).
	pile.AddItem(textRow("Type      : "+typeString), 1, false)
	pile.AddItem(k.nameEdit, 1, true)
	pile.AddItem(textRow("Node Addr : "+addrStr), 1, false)
	pile.AddItem(textRow("Operator  : "+data.OpStr), 1, false)   // inserted at index 3
	pile.AddItem(textRow("Distance  : "+data.HopsStr), 1, false) // inserted at index 4
	pile.AddItem(k.sortEdit, 1, true)
	pile.AddItem(newDividerRow(g["divider1"]), 1, false)
	pile.AddItem(lxmfRow, 1, false)
	pile.AddItem(newDividerRow(g["divider1"]), 1, false)
	pile.AddItem(k.pnCheck, 1, true)
	pile.AddItem(k.idCheck, 1, true)
	pile.AddItem(newDividerRow(g["divider1"]), 1, false)
	pile.AddItem(k.rUntrusted, 1, true)
	pile.AddItem(k.rUnknown, 1, true)
	pile.AddItem(k.rTrusted, 1, true)
	pile.AddItem(newDividerRow(g["divider1"]), 1, false)
	pile.AddItem(k.buttonRow(), 1, true)

	// Focus the button row (Python pile.focus_position = len-1).
	pile.SetFocusIndexLast()
	pile.SetEscHandler(func() { k.nd.showKnownNodes() })

	k.pile = pile
	return k
}

// SetFocusIndexLast focuses the last selectable item (the button row).
func (p *pileFiller) SetFocusIndexLast() {
	p.SetFocusIndex(len(p.selectable) - 1)
}

// Widget returns the Pile primitive to swap into the bordered list slot.
func (k *knownNodeInfoDisplay) Widget() tview.Primitive { return k.pile }

// FormData collects the edited form state for the Save action (Python save_node,
// Network.py:755-785). Trust defaults to Untrusted; Unknown and Trusted override
// when their radios are checked (matching Python's if/elif chain).
func (k *knownNodeInfoDisplay) FormData() KnownNodeInfoFormData {
	trust := "untrusted"
	if k.rUnknown.Checked() {
		trust = "unknown"
	}
	if k.rTrusted.Checked() {
		trust = "trusted"
	}
	return KnownNodeInfoFormData{
		Name:              k.nameEdit.GetText(),
		SortRank:          k.sortEdit.GetText(),
		TrustLevel:        trust,
		UseAsPN:           k.pnCheck.IsChecked(),
		UseAsPNChanged:    k.pnChanged,
		IdentifyOnConnect: k.idCheck.IsChecked(),
	}
}

// buttonRow builds the weighted Back/Connect/Msg Op/Save button Columns
// (Network.py:742-746): weights 0.2/0.02/0.2/0.02/0.2/0.02/0.2, scaled by 50 to
// ints [10,1,10,1,10,1,10]. Buttons are flat urwid.Button "< label >".
func (k *knownNodeInfoDisplay) buttonRow() *urwidColumns {
	back := NewUrwidButton("Back").SetSelectedFunc(func() { k.nd.showKnownNodes() })
	row := newURWIDColumns(0,
		back,
		tview.NewBox(),
		NewUrwidButton("Connect").SetSelectedFunc(func() { k.nd.connectKnownNode(k.nodeHash) }),
		tview.NewBox(),
		NewUrwidButton("Msg Op").SetSelectedFunc(func() { k.nd.msgOpKnownNode(k.nodeHash) }),
		tview.NewBox(),
		NewUrwidButton("Save").SetSelectedFunc(func() { k.nd.saveKnownNode(k) }),
	).
		SetWeight(0, 10).SetWeight(1, 1).SetWeight(2, 10).
		SetWeight(3, 1).SetWeight(4, 10).SetWeight(5, 1).SetWeight(6, 10)
	return row
}
