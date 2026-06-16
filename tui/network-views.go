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
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// KnownNodeInfo displays detailed information about a known node.
// Matches Python's KnownNodeInfo at Network.py:601.
type KnownNodeInfo struct {
	widget tview.Primitive
}

// NewKnownNodeInfo creates a node info panel for the given node entry.
func NewKnownNodeInfo(entry *NodeEntryFull) *KnownNodeInfo {
	dni := &KnownNodeInfo{}

	trustColor := "gray"
	trustStr := "Unknown"
	switch entry.TrustLevel {
	case "trusted":
		trustColor = "green"
		trustStr = "Trusted"
	case "untrusted":
		trustColor = "red"
		trustStr = "Untrusted"
	case "warning":
		trustColor = "yellow"
		trustStr = "Warning"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[::b]Name[-]  : %s\n", entry.DisplayName))
	sb.WriteString(fmt.Sprintf("[::b]Addr[-]  : <[lightblue]%s[-]>\n", entry.SourceHash))
	sb.WriteString(fmt.Sprintf("[::b]Type[-]  : Node Ⓝ\n"))
	sb.WriteString(fmt.Sprintf("[::b]Trust[-] : [%s]%s[-]\n", trustColor, trustStr))
	if entry.PreferredDelivery != "" {
		sb.WriteString(fmt.Sprintf("[::b]Delivery[-]: %s\n", entry.PreferredDelivery))
	}
	if entry.HostsNode {
		sb.WriteString("[::b]Hosts[-] : Yes\n")
	}

	text := tview.NewTextView()
	text.SetDynamicColors(true)
	text.SetTextColor(tcell.NewHexColor(0xbbbbbb))
	text.SetText(sb.String())

	dni.widget = text
	return dni
}

// Widget returns the tview primitive.
func (dni *KnownNodeInfo) Widget() tview.Primitive {
	return dni.widget
}

// NodeInfo displays detailed node statistics.
// Matches Python's NodeInfo at Network.py:1357.
type NodeInfo struct {
	widget tview.Primitive
}

// NewNodeInfo creates a node info panel with stats and buttons.
func NewNodeInfo(hash, name string) *NodeInfo {
	ni := &NodeInfo{}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[::b]Name[-]  : %s\n", name))
	sb.WriteString(fmt.Sprintf("[::b]Addr[-]  : <[lightblue]%s[-]>\n", hash))
	sb.WriteString("[::b]Type[-]  : Nomad Network Node Ⓝ\n")
	sb.WriteString("[gray]No additional stats available[-]\n")

	text := tview.NewTextView()
	text.SetDynamicColors(true)
	text.SetTextColor(tcell.NewHexColor(0xbbbbbb))
	text.SetText(sb.String())

	ni.widget = text
	return ni
}

// Widget returns the tview primitive.
func (ni *NodeInfo) Widget() tview.Primitive {
	return ni.widget
}

// LocalPeer displays information about the local peer.
// Matches Python's LocalPeer at Network.py:1259.
type LocalPeer struct {
	widget tview.Primitive
}

// NewLocalPeer creates a local peer info panel.
func NewLocalPeer(addr, name, lastAnnounce string) *LocalPeer {
	lp := &LocalPeer{}

	var sb strings.Builder
	sb.WriteString("[::b]Local Peer[-]\n\n")
	sb.WriteString(fmt.Sprintf("[::b]LXMF Addr[-] : [lightblue]%s[-]\n", addr))
	sb.WriteString(fmt.Sprintf("[::b]Name[-]      : %s\n", name))
	sb.WriteString(fmt.Sprintf("[::b]Last Announce[-]: %s\n", lastAnnounce))

	text := tview.NewTextView()
	text.SetDynamicColors(true)
	text.SetTextColor(tcell.NewHexColor(0xbbbbbb))
	text.SetText(sb.String())

	lp.widget = text
	return lp
}

// Widget returns the tview primitive.
func (lp *LocalPeer) Widget() tview.Primitive {
	return lp.widget
}

// LXMFPeersView displays a list of LXMF propagation peers.
// Matches Python's LXMFPeers at Network.py:1752.
type LXMFPeersView struct {
	widget tview.Primitive
	list   *tview.List
}

// LXMFPeerEntry holds info about a single propagation peer.
type LXMFPeerEntry struct {
	Hash      string
	Name      string
	Alive     bool
	SyncLimit int
	Pending   int
}

// NewLXMFPeersView creates a propagation peers list view.
func NewLXMFPeersView(peers []LXMFPeerEntry) *LXMFPeersView {
	lv := &LXMFPeersView{}

	title := tview.NewTextView()
	title.SetTextAlign(tview.AlignCenter)
	title.SetDynamicColors(true)
	title.SetTextColor(tcell.NewHexColor(0xdddddd))
	title.SetText("[::b]LXMF Propagation Peers[-]")

	lv.list = tview.NewList()
	lv.list.SetHighlightFullLine(true)
	lv.list.SetSelectedBackgroundColor(tcell.NewHexColor(0x666666))

	for _, p := range peers {
		status := "[green]alive[-]"
		if !p.Alive {
			status = "[red]dead[-]"
		}
		text := fmt.Sprintf("%s %s %s", p.Name, status, ShortHash(p.Hash, 8))
		secondary := fmt.Sprintf("Pending: %d", p.Pending)
		lv.list.AddItem(text, secondary, 0, nil)
	}

	if len(peers) == 0 {
		lv.list.AddItem("[gray]No propagation peers[-]", "", 0, nil)
	}

	layout := tview.NewFlex().SetDirection(tview.FlexRow)
	layout.AddItem(title, 1, 0, false)
	layout.AddItem(lv.list, 0, 1, true)
	layout.SetBorder(true)

	lv.widget = layout
	return lv
}

// Widget returns the tview primitive.
func (lv *LXMFPeersView) Widget() tview.Primitive {
	return lv.widget
}
