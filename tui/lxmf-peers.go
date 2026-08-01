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
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// LXMFPeersDisplay is the left-pane "LXMF Propagation Peers" list. Matches
// Python's LXMFPeers (Network.py:1752): a bordered IndicativeListBox of peered
// LXMF nodes, or — when none are peered — a top-filled, centered warning-text
// block ("ℹ / Currently, no LXMF nodes are peered"). The border+title are
// supplied by the network left-pane slot (setLeftList), so this widget renders
// only the inner content, like AnnounceInfo/KnownNodes do.
type LXMFPeersDisplay struct {
	app     *App
	count   int
	content tview.Primitive // noContent (centeredText) or a peers list
}

// NewLXMFPeersDisplay creates a peers display with no peered nodes (the
// no-content branch), matching LXMFPeers.__init__ with an empty peer_list
// (Network.py:1779-1788).
func NewLXMFPeersDisplay(app *App) *LXMFPeersDisplay {
	p := &LXMFPeersDisplay{app: app}
	p.setNoContent()
	return p
}

// setNoContent builds the empty-state block: a top-filled, ceil-left-centered
// warning-text info glyph, a blank, and "Currently, no LXMF nodes are peered"
// (two trailing blanks), matching Python's LXMFPeers no-content Pile
// (Network.py:1782-1786): Text(info+"\n") + SelectText(msg+"\n\n").
func (p *LXMFPeersDisplay) setNoContent() {
	color := GetThemeColors(p.app.Theme)["warning_text"]
	if color == tcell.ColorDefault {
		color = tcell.NewHexColor(0xbbaa44)
	}
	glyph := ""
	if g := p.app.Glyphs; g != nil {
		glyph = g["info"]
	}
	p.content = newCenteredText(color,
		glyph, "",
		"Currently, no LXMF nodes are peered", "", "",
	)
}

// Widget returns the inner content primitive (no border; the left-pane slot
// provides the border+title).
func (p *LXMFPeersDisplay) Widget() tview.Primitive { return p.content }

// Count returns the number of peered LXMF nodes.
func (p *LXMFPeersDisplay) Count() int { return p.count }

// Title returns the LineBox title for this list, "LXMF Propagation Peers (N)",
// matching Python's LXMFPeers title (Network.py:1788).
func (p *LXMFPeersDisplay) Title() string {
	return fmt.Sprintf("LXMF Propagation Peers (%d)", p.count)
}

// SetPeers replaces the peer list. With no peers (or until Phase 5 wires the
// LXMF message router) the no-content branch is shown; otherwise a selectable
// list of LXMFPeerEntry rows renders (Phase 5). Matches Python's
// rebuild_widget_list / reinit_lxmf_peers (Network.py:1843-1862, 1717).
func (p *LXMFPeersDisplay) SetPeers(entries []LXMFPeerEntry) {
	p.count = len(entries)
	if p.count == 0 {
		p.setNoContent()
		return
	}
	// Phase 5: build a selectable IndicativeListBox of LXMFPeerEntry rows
	// sorted by (pn_trust_level, sync_transfer_rate) descending
	// (Network.py:1863-1869). Until the message router is wired this branch
	// is unreachable; keep the no-content fallback robust.
	list := tview.NewList()
	list.SetHighlightFullLine(true)
	ApplyListFocusStyle(list, p.app.Theme)
	for _, e := range entries {
		label := e.Name
		if label == "" {
			label = e.Hash
		}
		list.AddItem(label, "", 0, nil)
	}
	p.content = list
}
