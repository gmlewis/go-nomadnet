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
	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-runewidth"
	"github.com/rivo/tview"
)

// NodeInfoData supplies the Local Node Info panel's content. When HasNode is
// false the panel renders the "This instance is not hosting a node" message
// (the only reachable state until node hosting is wired in Phase 5); the
// remaining fields are used by the node-hosting branch (Phase 5).
type NodeInfoData struct {
	HasNode            bool
	Addr               string // node destination hex, no delimiters (RNS.hexrep delimit=False)
	Name               string
	DisablePropagation bool
	LXMFPropAddr       string // prettyhexrep of the lxmf.propagation hash (when !DisablePropagation)

	// Stat-line providers for the hosting branch (Phase 5). Each returns the
	// text shown after its label; nil providers yield the "None"/"Never"
	// fallbacks the Python UpdatingText widgets show before the node reports.
	LastAnnounce   func() string
	StorageStats   func() string
	ActiveLinks    func() string
	TotalConnects  func() string
	TotalPages     func() string
	TotalFiles     func() string
	OnBrowse       func()
	OnResetStats   func()
	OnAnnounce     func()
}

// NodeInfoDisplay is the "Local Node Info" panel that replaces the Local Peer
// Info panel in the Network left pane when the user activates the "Node Info"
// button (Python Network.py:1357-1554). It mirrors Python's NodeInfo: a titled
// LineBox ("Local Node Info") wrapping a Pile. When the instance is not hosting
// a node the Pile is a centered info glyph, the "This instance is not hosting a
// node" message, and a centered "Back" button that swaps the panel back to the
// Local Peer Info (show_peer_info, Network.py:1396-1398).
type NodeInfoDisplay struct {
	app     *App
	widget  *tview.Flex // bordered "Local Node Info"
	backBtn *UrwidButton
	OnBack  func()
	data    NodeInfoData
}

// NewNodeInfoDisplay builds the Local Node Info panel for the given data. Until
// node hosting is wired (Phase 5) only the not-hosting branch is reachable; it
// is the branch pinned by the tests.
func NewNodeInfoDisplay(app *App, data NodeInfoData) *NodeInfoDisplay {
	ni := &NodeInfoDisplay{app: app, data: data}

	ni.backBtn = NewUrwidButton("Back").
		SetSelectedFunc(func() {
			if ni.OnBack != nil {
				ni.OnBack()
			}
		})

	if !data.HasNode {
		// Not-hosting branch (Network.py:1543-1551): a centered info glyph, the
		// "This instance is not hosting a node" message, and a centered Back
		// button. The two Text widgets use urwid.Text(align=CENTER) → ceil-left
		// centering (extra col to the LEFT on odd slack), so centeredText is
		// used. The button uses urwid.Padding(CENTER, PACK) → floor-left
		// centering (extra col to the RIGHT), matched by a Flex row with two
		// equal-proportion spacers flanking the fixed-width button.
		glyph := "-"
		if app != nil && app.Glyphs != nil {
			glyph = app.Glyphs["info"]
		}

		// urwid.Text("\n"+g["info"]) → ["", glyph] (2 rows).
		infoText := newCenteredText(tcell.ColorDefault, "", glyph)
		// urwid.Text("\nThis instance is not hosting a node\n\n") → 4 rows.
		msgText := newCenteredText(tcell.ColorDefault,
			"", "This instance is not hosting a node", "", "")

		btnRow := newCenteredButtonRow(ni.backBtn, urwidButtonNaturalWidth("Back"))

		ni.widget = tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(infoText, 2, 0, false).
			AddItem(msgText, 4, 0, false).
			AddItem(btnRow, 1, 0, true)
		ni.widget.SetBorder(true)
		SetTitledBorder(ni.widget, "Local Node Info")
		return ni
	}

	// Hosting branch (Network.py:1408-1541) — node hosting is not yet wired in
	// the Go port (no app.Node server; Phase 5). Render the Addr/Name header and
	// the Back button so the panel is non-empty if ever reached, with a clear
	// note that the live stat lines are pending. Pinned as a known gap in
	// TODO.md §4.N.
	addrText := tview.NewTextView().SetDynamicColors(false).
		SetText("Addr : " + data.Addr)
	nameText := tview.NewTextView().SetDynamicColors(false).
		SetText("Name : " + data.Name)
	btnRow := newCenteredButtonRow(ni.backBtn, urwidButtonNaturalWidth("Back"))

	ni.widget = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(addrText, 1, 0, false).
		AddItem(nameText, 1, 0, false).
		AddItem(newDividerRow(dividerGlyph(app)), 1, 0, false).
		AddItem(tview.NewTextView().SetDynamicColors(false).
			SetText("Node stats unavailable (hosting not wired)"), 1, 0, false).
		AddItem(btnRow, 1, 0, true)
	ni.widget.SetBorder(true)
	SetTitledBorder(ni.widget, "Local Node Info")
	return ni
}

// Widget returns the bordered "Local Node Info" primitive for layout.
func (ni *NodeInfoDisplay) Widget() *tview.Flex { return ni.widget }

// Height returns the rendered height of the panel: the not-hosting branch is
// 7 content rows + 2 border rows = 9. Callers use this as the tview Flex
// fixedSize so the panel sits PACK-style below the saved-nodes list (matching
// Python's left_pile PACK slot, which resizes to the widget's content).
func (ni *NodeInfoDisplay) Height() int {
	if !ni.data.HasNode {
		return 9
	}
	// Hosting branch height grows with stat lines; until Phase 5 wires them,
	// the placeholder above is 5 content rows + 2 border = 7.
	return 7
}

// urwidButtonNaturalWidth returns the flat render width of a UrwidButton with
// the given label: left bracket + dividechars + label + dividechars + right
// bracket, i.e. 2 + 2*urwidButtonDivideChars + label width = 4 + label width.
// Used to size a fixed-width Flex slot so the button renders flat (not
// stretched) and can be centered.
func urwidButtonNaturalWidth(label string) int {
	return 2 + 2*urwidButtonDivideChars + runewidth.StringWidth(label)
}

// newCenteredButtonRow returns a 1-row Flex that centers a fixed-width button
// horizontally with urwid.Padding(CENTER, PACK) semantics — floor-left
// centering (the extra column goes to the RIGHT on odd slack). tview's Flex
// gives the leftover to the LAST proportional item, so two equal-proportion
// spacers flanking the fixed-width button yield left=floor((w-bw)/2),
// right=ceil((w-bw)/2), exactly matching urwid's calculate_left_right_padding
// for Align.CENTER (urwid/widget/padding.py:553-560).
func newCenteredButtonRow(btn *UrwidButton, naturalWidth int) *tview.Flex {
	return tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(tview.NewBox(), 0, 1, false).
		AddItem(btn, naturalWidth, 0, true).
		AddItem(tview.NewBox(), 0, 1, false)
}

// dividerGlyph returns the divider1 glyph for the app's glyph set, falling back
// to "-" like LocalPeerDisplay.
func dividerGlyph(app *App) string {
	if app != nil && app.Glyphs != nil {
		return app.Glyphs["divider1"]
	}
	return "-"
}