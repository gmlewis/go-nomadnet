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

// kndRow builds a KnownNodeInfo display for the given data and renders it at
// w×h, returning the rows with trailing pad spaces stripped.
func kndRow(t *testing.T, nd *NetworkDisplay, hash string, data KnownNodeInfoData, w, h int) []string {
	t.Helper()
	k := newKnownNodeInfoDisplay(nd, hash, data)
	rows := renderPrimitive(t, k.Widget(), w, h)
	for i := range rows {
		rows[i] = strings.TrimRight(rows[i], " ")
	}
	return rows
}

// kndKey sends a key event to the KnownNodeInfo pile's input handler.
func kndKey(k *knownNodeInfoDisplay, key tcell.Key) {
	k.pile.InputHandler()(tcell.NewEventKey(key, 0, tcell.ModNone), func(tview.Primitive) {})
}

// TestKnownNodeInfoStructure pins the KnownNodeInfo pile layout against Python's
// KnownNodeInfo (Network.py:593-810): a TOP-filled Pile whose rows are, in
// order, Type / Name(edit) / Node Addr / Operator / Distance / Sort Rank(edit)
// — Operator and Distance inserted at indices 3 and 4 (Network.py:789,797) —
// then a divider, the centered LXMF PN-address line, another divider, the two
// CheckBoxes (default PN / identify on connect), a divider, the three trust
// RadioButtons (Untrusted / Unknown / Trusted), a final divider, and the
// weighted Back/Connect/Msg Op/Save button row. Labels align the colon at
// column 10 ("Type      : ", "Name      : ", "Node Addr : ", "Operator  : ",
// "Distance  : ", "Sort Rank : "). Rendered tall enough that no top-trim occurs.
func TestKnownNodeInfoStructure(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	nd := NewNetworkDisplay(app, nil, nil)

	hash := strings.Repeat("a", 32)
	data := KnownNodeInfoData{
		DisplayStr:        "Test Node",
		SortStr:           "None",
		TrustLevel:        "unknown",
		OpStr:             "Unknown",
		HopsStr:           "Unknown",
		LXMFAddrStr:       "No associated Propagation Node known",
		UseAsPN:           false,
		IdentifyOnConnect: true,
	}

	rows := kndRow(t, nd, hash, data, 80, 40)

	// Row order (17 rows, no trim):
	//  0 Type       1 Name(edit)   2 Node Addr   3 Operator   4 Distance
	//  5 Sort Rank  6 divider      7 LXMF(center) 8 divider
	//  9 PN check   10 id check    11 divider
	// 12 Untrusted  13 Unknown     14 Trusted     15 divider   16 buttons
	wantPrefix := []string{
		"Type      : ",
		"Name      : ",
		"Node Addr : ",
		"Operator  : ",
		"Distance  : ",
		"Sort Rank : ",
		"", // divider
		"", // LXMF (centered, no left-prefix)
		"", // divider
		"", // checkbox (label after the box, no fixed colon prefix)
		"",
		"",
		"", // radios
		"",
		"",
		"", // divider
		"", // buttons
	}
	for i, p := range wantPrefix {
		if p != "" && !strings.HasPrefix(rows[i], p) {
			t.Errorf("row %v = %q, want prefix %q", i, rows[i], p)
		}
	}

	// Exact static-Text rows.
	if want := "Type      : Nomad Network Node Ⓝ"; rows[0] != want {
		t.Errorf("row 0 = %q, want %q", rows[0], want)
	}
	if want := "Node Addr : <" + hash + ">"; rows[2] != want {
		t.Errorf("row 2 = %q, want %q", rows[2], want)
	}
	if want := "Operator  : Unknown"; rows[3] != want {
		t.Errorf("row 3 = %q, want %q (Operator inserted at index 3)", rows[3], want)
	}
	if want := "Distance  : Unknown"; rows[4] != want {
		t.Errorf("row 4 = %q, want %q (Distance inserted at index 4)", rows[4], want)
	}

	// ReadlineEdit rows show the label then the field text.
	if !strings.HasPrefix(rows[1], "Name      : ") || !strings.Contains(rows[1], "Test Node") {
		t.Errorf("row 1 (Name edit) = %q, want label + \"Test Node\"", rows[1])
	}
	if !strings.HasPrefix(rows[5], "Sort Rank : ") || !strings.Contains(rows[5], "None") {
		t.Errorf("row 5 (Sort Rank edit) = %q, want label + \"None\"", rows[5])
	}

	// Dividers are full-width divider1 (┄) lines.
	divider := strings.Repeat("┄", 80)
	if rows[6] != divider {
		t.Errorf("row 6 (divider) = %q, want %q", rows[6], divider)
	}
	if rows[8] != divider {
		t.Errorf("row 8 (divider) = %q, want %q", rows[8], divider)
	}
	if rows[11] != divider {
		t.Errorf("row 11 (divider) = %q, want %q", rows[11], divider)
	}
	if rows[15] != divider {
		t.Errorf("row 15 (divider) = %q, want %q", rows[15], divider)
	}

	// LXMF PN-address line is centered (leading pad spaces before the text).
	if got := strings.TrimSpace(rows[7]); got != "No associated Propagation Node known" {
		t.Errorf("row 7 (LXMF) = %q, want centered PN-address line", rows[7])
	}
	// Centered ⇒ there must be leading padding.
	if !strings.HasPrefix(rows[7], " ") {
		t.Errorf("row 7 (LXMF) = %q, want leading spaces (centered)", rows[7])
	}

	// CheckBoxes: the label text appears on the row (tview renders box + label).
	if !strings.Contains(rows[9], "Use as default propagation node") {
		t.Errorf("row 9 (PN checkbox) = %q, want label present", rows[9])
	}
	if !strings.Contains(rows[10], "Identify when connecting") {
		t.Errorf("row 10 (id checkbox) = %q, want label present", rows[10])
	}

	// Trust radios: TrustLevel=="unknown" preselects Unknown ("(X)"), the
	// other two unchecked ("( )").
	if want := "( ) Untrusted"; rows[12] != want {
		t.Errorf("row 12 (Untrusted radio) = %q, want %q", rows[12], want)
	}
	if want := "(X) Unknown"; rows[13] != want {
		t.Errorf("row 13 (Unknown radio) = %q, want %q (preselected)", rows[13], want)
	}
	if want := "( ) Trusted"; rows[14] != want {
		t.Errorf("row 14 (Trusted radio) = %q, want %q", rows[14], want)
	}

	// Button row: Back/Connect/Msg Op/Save with weights [10,1,10,1,10,1,10]
	// scaled over 80 columns. Each button is "< label   …   >". The four labels
	// must all appear on row 16.
	btnRow := rows[16]
	for _, label := range []string{"Back", "Connect", "Msg Op", "Save"} {
		if !strings.Contains(btnRow, label) {
			t.Errorf("button row %q missing label %q", btnRow, label)
		}
	}
}

// TestKnownNodeInfoTrustPreselect verifies the radio preselection matches the
// incoming TrustLevel for each of the three values that map to a radio.
func TestKnownNodeInfoTrustPreselect(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	nd := NewNetworkDisplay(app, nil, nil)

	cases := []struct {
		trust string
		want  string // the radio label that should be "(X)"
	}{
		{"untrusted", "Untrusted"},
		{"unknown", "Unknown"},
		{"trusted", "Trusted"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.trust, func(t *testing.T) {
			t.Parallel()
			data := KnownNodeInfoData{
				DisplayStr:  "N",
				SortStr:     "None",
				TrustLevel:  c.trust,
				OpStr:       "Unknown",
				HopsStr:     "Unknown",
				LXMFAddrStr: "none",
			}
			k := newKnownNodeInfoDisplay(nd, strings.Repeat("b", 32), data)
			radios := map[string]*RadioButton{
				"Untrusted": k.rUntrusted, "Unknown": k.rUnknown, "Trusted": k.rTrusted,
			}
			gotChecked := 0
			for label, rb := range radios {
				if rb.Checked() {
					gotChecked++
					if label != c.want {
						t.Errorf("trust %q: %s checked, want %s", c.trust, label, c.want)
					}
				}
			}
			if gotChecked != 1 {
				t.Errorf("trust %q: %v radios checked, want exactly 1", c.trust, gotChecked)
			}
		})
	}
}

// TestKnownNodeInfoTrustWarningPreselectsNone verifies that a "warning" trust
// level leaves no radio checked (Python KnownNodeInfo only preselects for
// untrusted/unknown/trusted; warning falls through with none set).
func TestKnownNodeInfoTrustWarningPreselectsNone(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	nd := NewNetworkDisplay(app, nil, nil)
	data := KnownNodeInfoData{DisplayStr: "N", SortStr: "None", TrustLevel: "warning"}
	k := newKnownNodeInfoDisplay(nd, strings.Repeat("c", 32), data)
	for _, rb := range []*RadioButton{k.rUntrusted, k.rUnknown, k.rTrusted} {
		if rb.Checked() {
			t.Errorf("warning trust: %q checked, want none checked", rb.label)
		}
	}
}

// TestKnownNodeInfoFormData verifies FormData collects the edited name, sort
// rank, the radio-selected trust level, and the two checkbox states. Trust
// defaults to "untrusted"; Unknown and Trusted override (matching Python's
// if/elif chain in save_node, Network.py:755-785).
func TestKnownNodeInfoFormData(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	nd := NewNetworkDisplay(app, nil, nil)

	data := KnownNodeInfoData{
		DisplayStr:        "Old Name",
		SortStr:           "5",
		TrustLevel:        "untrusted",
		UseAsPN:           false,
		IdentifyOnConnect: false,
	}
	k := newKnownNodeInfoDisplay(nd, strings.Repeat("d", 32), data)

	// Edit the fields.
	k.nameEdit.SetText("New Name")
	k.sortEdit.SetText("12")
	k.pnCheck.SetChecked(true)
	k.idCheck.SetChecked(true)
	// Select Trusted (defaults to untrusted).
	k.rTrusted.SetChecked(true)

	fd := k.FormData()
	if fd.Name != "New Name" {
		t.Errorf("FormData.Name = %q, want %q", fd.Name, "New Name")
	}
	if fd.SortRank != "12" {
		t.Errorf("FormData.SortRank = %q, want %q", fd.SortRank, "12")
	}
	if fd.TrustLevel != "trusted" {
		t.Errorf("FormData.TrustLevel = %q, want %q", fd.TrustLevel, "trusted")
	}
	if !fd.UseAsPN {
		t.Error("FormData.UseAsPN = false, want true")
	}
	if !fd.IdentifyOnConnect {
		t.Error("FormData.IdentifyOnConnect = false, want true")
	}
}

// TestKnownNodeInfoFormDataTrustDefault verifies that with no radio checked the
// trust level defaults to "untrusted" (Python save_node default).
func TestKnownNodeInfoFormDataTrustDefault(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	nd := NewNetworkDisplay(app, nil, nil)
	data := KnownNodeInfoData{DisplayStr: "N", SortStr: "None", TrustLevel: "warning"}
	k := newKnownNodeInfoDisplay(nd, strings.Repeat("e", 32), data)
	fd := k.FormData()
	if fd.TrustLevel != "untrusted" {
		t.Errorf("FormData.TrustLevel = %q, want %q (default)", fd.TrustLevel, "untrusted")
	}
}

// TestKnownNodeInfoFocusOnButtonRow verifies the pile focus starts on the last
// selectable item (the button row), matching Python's pile.focus_position =
// len(pile_widgets)-1 (Network.py:799).
func TestKnownNodeInfoFocusOnButtonRow(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	nd := NewNetworkDisplay(app, nil, nil)
	data := KnownNodeInfoData{DisplayStr: "N", SortStr: "None", TrustLevel: "unknown"}
	k := newKnownNodeInfoDisplay(nd, strings.Repeat("f", 32), data)
	want := len(k.pile.selectable) - 1
	if got := k.pile.FocusIndex(); got != want {
		t.Errorf("pile focusIndex = %v, want %v (last = button row)", got, want)
	}
}

// TestKnownNodeInfoEscReturnsToNodes verifies the Esc handler pops the left pane
// back to the saved-nodes list (showKnownNodes), clearing inInfoView.
func TestKnownNodeInfoEscReturnsToNodes(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	app.Glyphs = GetGlyphSet(GlyphUnicode)
	nd := NewNetworkDisplay(app, nil, nil)

	// Enter the KnownNodeInfo view the way the app does, so inInfoView is set.
	hash := strings.Repeat("g", 32)
	nd.ShowKnownNodeInfo(hash)
	if !nd.inInfoView {
		t.Fatal("ShowKnownNodeInfo did not set inInfoView")
	}

	// The swapped-in left pane is the KnownNodeInfo pile; send it Esc. The pile
	// was built with SetEscHandler → showKnownNodes, but the swapped primitive
	// is a fresh tview.Box wrapper from setLeftList, so drive the display's own
	// pile instead. Recover the display via the esc handler contract: the
	// KnownNodeInfo display's pile intercepts Esc and calls showKnownNodes.
	// Rebuild the display to get a handle and fire Esc.
	k := newKnownNodeInfoDisplay(nd, hash, KnownNodeInfoData{DisplayStr: "N", SortStr: "None", TrustLevel: "unknown"})
	kndKey(k, tcell.KeyEscape)

	if nd.inInfoView {
		t.Error("after Esc inInfoView = true, want false (showKnownNodes)")
	}
	if !nd.showingNodes {
		t.Error("after Esc showingNodes = false, want true")
	}
}
