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
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns"
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
	entries []LXMFPeerEntry // current peer rows, parallel to the list order

	// OnUnpeer is invoked on `ctrl x` for the selected peer, with that peer's
	// destination hash. Mirrors Python LXMFPeers.delete_selected_entry
	// (Network.py:1800-1806): the wiring layer calls message_router.unpeer,
	// then reinit_lxmf_peers + show_peers. nil ⇒ C-x is a no-op.
	OnUnpeer func(destinationHash []byte)
	// OnSync is invoked on `ctrl r` for the selected peer, with that peer's
	// destination hash. Mirrors Python LXMFPeers.sync_selected_entry
	// (Network.py:1808-1834): the wiring layer checks the sync grace window,
	// triggers peer.sync(), and shows the "delivery sync requested" dialog.
	// nil ⇒ C-r is a no-op.
	OnSync func(destinationHash []byte)
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
	return fmt.Sprintf("LXMF Propagation Peers (%v)", p.count)
}

// SetPeers replaces the peer list. With no peers the no-content branch is
// shown; otherwise a selectable list of peer rows renders. Each entry's
// DisplayText (the multi-line peer_info_str from FormatLXMFPeerEntry) is the
// list row's main text; DestinationHash is stashed so the C-x/C-r keybindings
// can resolve the selected peer. Matches Python's rebuild_widget_list /
// reinit_lxmf_peers (Network.py:1843-1862, 1717). The caller is responsible for
// sorting by (pn_trust_level, sync_transfer_rate) descending (Network.py:1866)
// before calling; the display preserves the given order.
func (p *LXMFPeersDisplay) SetPeers(entries []LXMFPeerEntry) {
	p.entries = entries
	p.count = len(entries)
	if p.count == 0 {
		p.setNoContent()
		return
	}
	list := tview.NewList()
	list.SetHighlightFullLine(true)
	ApplyListFocusStyle(list, p.app.Theme)
	for _, e := range entries {
		label := e.DisplayText
		if label == "" {
			// Legacy/simple fallback for callers that only populate
			// Hash/Name (e.g. unit tests). Matches the prior placeholder.
			label = e.Name
			if label == "" {
				label = e.Hash
			}
		}
		list.AddItem(label, "", 0, nil)
	}
	list.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		return p.handlePeerKey(list, event)
	})
	p.content = list
}

// handlePeerKey dispatches the LXMFPeers keybindings (Python LXMFPeers.keypress,
// Network.py:1793-1798): ctrl x unpeers the selected entry, ctrl r requests a
// delivery sync for it. Both consume the event (return nil) when acted on;
// other keys pass through unchanged.
func (p *LXMFPeersDisplay) handlePeerKey(list *tview.List, event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyCtrlX:
		if h := p.selectedHash(list); h != nil && p.OnUnpeer != nil {
			p.OnUnpeer(h)
		}
		return nil
	case tcell.KeyCtrlR:
		if h := p.selectedHash(list); h != nil && p.OnSync != nil {
			p.OnSync(h)
		}
		return nil
	}
	return event
}

// selectedHash returns the destination hash of the currently selected peer row,
// or nil if none is selected. Mirrors Python's ilb.get_selected_item()
// .original_widget.destination_hash (Network.py:1802,1820).
func (p *LXMFPeersDisplay) selectedHash(list *tview.List) []byte {
	if list == nil {
		return nil
	}
	idx := list.GetCurrentItem()
	if idx < 0 || idx >= len(p.entries) {
		return nil
	}
	return p.entries[idx].DestinationHash
}

// PeerEntryData is the resolved, render-ready input to FormatLXMFPeerEntry. The
// fields mirror the intermediate strings Python's LXMFPeerEntry.__init__
// (Network.py:1875-1928) computes before concatenating peer_info_str, so the
// pure formatter is testable without an RNS transport or directory. The
// per-field string formatting (prettysize, prettyspeed, "%v"/"Unknown") is done
// by BuildPeerEntryData; FormatLXMFPeerEntry only does the final join + the
// time-injected pretty_date.
type PeerEntryData struct {
	Sym           string // g["sent"] glyph prefixing the first line
	DisplayStr    string // display name + "\n  " + hex, or just hex
	AliveStr      string // "Available" / "Unresponsive" / "Unknown"
	LastHeard     time.Time
	SyncLimit     string // prettysize(sync_limit*1000) or "Unknown"
	TxferLimit    string // prettysize(transfer_limit*1000) or "No"
	STR           string // prettyspeed(sync_transfer_rate)
	LER           string // prettyspeed(link_establishment_rate)
	StampCost     string // "%v" or "Unknown"
	StampFlex     string // " (flex N)" or ""
	Unhandled     int    // unhandled_message_count
	AcceptancePct string // Python round(rate*100,2) formatted as f"{ar}"
}

// FormatLXMFPeerEntry renders the multi-line peer_info_str for an LXMFPeerEntry
// (Network.py:1919-1924), using `now` for the pretty_date "last heard" phrase.
// The template is:
//
//	sym+" "+display_str
//	"  "+alive_string+", last heard "+pretty_date(int(last_heard))
//	"  "+sync_limit+" sync limit, "+txfer_limit+" msg limit"
//	"  "+STR+" STR, "+LER+" LER"
//	"  Propagation cost "+sct+scf
//	"  "+unhandled+" unhandled LXMs, "+ar+"% AR"
func FormatLXMFPeerEntry(d PeerEntryData, now time.Time) string {
	var b strings.Builder
	b.WriteString(d.Sym)
	b.WriteString(" ")
	b.WriteString(d.DisplayStr)
	b.WriteString("\n  ")
	b.WriteString(d.AliveStr)
	b.WriteString(", last heard ")
	b.WriteString(prettyDateAt(d.LastHeard, now))
	b.WriteString("\n  ")
	b.WriteString(d.SyncLimit)
	b.WriteString(" sync limit, ")
	b.WriteString(d.TxferLimit)
	b.WriteString(" msg limit")
	b.WriteString("\n  ")
	b.WriteString(d.STR)
	b.WriteString(" STR, ")
	b.WriteString(d.LER)
	b.WriteString(" LER")
	b.WriteString("\n  Propagation cost ")
	b.WriteString(d.StampCost)
	b.WriteString(d.StampFlex)
	b.WriteString("\n  ")
	b.WriteString(strconv.Itoa(d.Unhandled))
	b.WriteString(" unhandled LXMs, ")
	b.WriteString(d.AcceptancePct)
	b.WriteString("% AR")
	return b.String()
}

// formatAcceptancePct mirrors Python `ar = round(peer.acceptance_rate*100, 2)`
// followed by the f-string `f"{ar}"` (Network.py:1917,1924). Python's str(float)
// always renders at least one decimal digit ("100.0", "0.0", "87.5", "33.33"),
// so after Go's shortest-representation format we append ".0" when the result
// has no decimal point. Rounding uses math.Round (half-away-from-zero); this
// matches Python's round() for the non-exact-half values acceptance ratios
// produce in practice.
func formatAcceptancePct(rate float64) string {
	pct := rate * 100
	rounded := float64(int64(pct*100+0.5)) / 100 // round to 2 decimals, half-up
	if pct < 0 {
		rounded = -float64(int64(-pct*100+0.5)) / 100
	}
	s := strconv.FormatFloat(rounded, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}

// BuildPeerEntryData gathers the render-ready PeerEntryData for an lxmf.Peer,
// mirroring Python LXMFPeerEntry.__init__ (Network.py:1875-1918). displayStr is
// the resolved display string (the caller resolves the alleged display name,
// since it touches app.directory + RNS.Identity.recall); sym is the active glyph
// set's "sent" glyph. Per-row trust background coloring is applied by the
// caller via tview tags (Python's list_trusted/list_normal/list_unresponsive
// AttrMap styles) and is not part of the text payload.
func BuildPeerEntryData(peer *lxmf.Peer, displayStr string, sym string) PeerEntryData {
	d := PeerEntryData{
		Sym:        sym,
		DisplayStr: displayStr,
	}
	if peer == nil {
		d.AliveStr = "Unknown"
		d.SyncLimit = "Unknown"
		d.TxferLimit = "No"
		d.STR = PrettySpeed(0)
		d.LER = PrettySpeed(0)
		d.StampCost = "Unknown"
		d.AcceptancePct = formatAcceptancePct(0)
		return d
	}
	d.LastHeard = time.Unix(int64(peer.LastHeard()), 0)
	if peer.Alive() {
		d.AliveStr = "Available"
	} else {
		// Go's lxmf.Peer always tracks alive (it has no "no alive attr" state),
		// so the Python "Unknown" branch (hasattr(peer,"alive") == False) never
		// occurs here; a not-alive peer is "Unresponsive".
		d.AliveStr = "Unresponsive"
	}
	if tl := peer.PropagationSyncLimit(); tl != nil {
		d.SyncLimit = Prettysize(float64(*tl) * 1000)
	} else {
		d.SyncLimit = "Unknown"
	}
	if tl := peer.PropagationTransferLimit(); tl != nil && *tl != 0 {
		d.TxferLimit = Prettysize(*tl * 1000)
	} else {
		d.TxferLimit = "No"
	}
	d.STR = PrettySpeed(peer.SyncTransferRate())
	d.LER = PrettySpeed(peer.LinkEstablishmentRate())
	if sc := peer.PropagationStampCost(); sc != nil {
		d.StampCost = strconv.Itoa(*sc)
	} else {
		d.StampCost = "Unknown"
	}
	if scf := peer.PropagationStampCostFlexibility(); scf != nil {
		d.StampFlex = fmt.Sprintf(" (flex %v)", *scf)
	}
	d.Unhandled = peer.UnhandledMessageCount()
	d.AcceptancePct = formatAcceptancePct(peer.AcceptanceRate())
	return d
}

// ResolvePeerDisplayStr computes the display string for a peer's destination
// hash, mirroring Python LXMFPeerEntry.__init__ (Network.py:1882-1889): derive
// the nomadnetwork.node hash from the recalled identity, look up the alleged
// display name, and on a hit render `display_name + "\n  " + prettyhexrep`;
// otherwise just `prettyhexrep`. identity is the recalled identity for the peer
// (peer.Identity(), or rns.RecallIdentity(ts, destHash)); nil ⇒ hex only.
// displayName returns the alleged display name for a node hash (or "").
func ResolvePeerDisplayStr(destHash []byte, identity *rns.Identity, displayName func([]byte) string) string {
	hex := rns.PrettyHexRep(destHash)
	if identity == nil {
		return hex
	}
	nodeHash := rns.CalculateHash(identity, "nomadnetwork", "node")
	if displayName != nil {
		if name := displayName(nodeHash); name != "" {
			return name + "\n  " + hex
		}
	}
	return hex
}
