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
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// AnnounceEntry holds a single announce for display.
type AnnounceEntry struct {
	Timestamp   time.Time
	SourceHash  string
	AppData     string
	Type        string // "node", "peer", "pn"
	TrustLevel  string
	DisplayName string
}

// NodeEntry holds a known node/peer for display.
type NodeEntry struct {
	SourceHash  string
	DisplayName string
	TrustLevel  string
	HostsNode   bool
	Delivery    string
}

// NetworkDisplay shows the announce stream and known nodes.
type NetworkDisplay struct {
	app       *tview.Application
	widget    tview.Primitive
	announces *tview.List
	nodes     *tview.List
	detail    *tview.TextView
}

// NewNetworkDisplay creates a new network display.
func NewNetworkDisplay(app *tview.Application, announces []AnnounceEntry, nodes []NodeEntry) *NetworkDisplay {
	nd := &NetworkDisplay{app: app}

	// Title
	title := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true).
		SetTextColor(tcell.NewHexColor(0xdddddd)).
		SetText("[::b]Network[-]")

	// Announces list
	nd.announces = tview.NewList().
		SetHighlightFullLine(true).
		SetSelectedBackgroundColor(tcell.NewHexColor(0x666666))

	for _, ann := range announces {
		typeIcon := "○"
		switch ann.Type {
		case "node":
			typeIcon = "Ⓝ"
		case "pn":
			typeIcon = "↑"
		case "peer":
			typeIcon = "Ⓟ"
		}
		text := fmt.Sprintf("%s %s", typeIcon, ann.DisplayName)
		secondary := fmt.Sprintf("%s — %s", ann.Timestamp.Format("15:04:05"), truncateStr(ann.AppData, 30))
		nd.announces.AddItem(text, secondary, 0, nil)
	}

	// Nodes list
	nd.nodes = tview.NewList().
		SetHighlightFullLine(true).
		SetSelectedBackgroundColor(tcell.NewHexColor(0x666666))

	for _, node := range nodes {
		trustIcon := "○"
		if node.TrustLevel == "trusted" {
			trustIcon = "●"
		}
		text := fmt.Sprintf("%s %s", trustIcon, node.DisplayName)
		secondary := fmt.Sprintf("Delivery: %s", node.Delivery)
		nd.nodes.AddItem(text, secondary, 0, nil)
	}

	// Detail view
	nd.detail = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetTextColor(tcell.NewHexColor(0xbbbbbb)).
		SetText("[gray]Select an announce or node to view details[-]")

	// Tabs for announces/nodes
	tabBar := tview.NewTextView().
		SetTextAlign(tview.AlignCenter).
		SetDynamicColors(true).
		SetTextColor(tcell.NewHexColor(0xdddddd)).
		SetText("[yellow]1[-] Announces  [yellow]2[-] Nodes")

	// Layout: lists on left, detail on right
	leftPanel := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tabBar, 1, 0, false).
		AddItem(nd.announces, 0, 1, true)

	content := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(leftPanel, 0, 1, true).
		AddItem(nd.detail, 0, 2, false)

	nd.widget = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(title, 2, 0, false).
		AddItem(content, 0, 1, true)

	return nd
}

// Widget returns the tview primitive for this display.
func (nd *NetworkDisplay) Widget() tview.Primitive {
	return nd.widget
}

// UpdateAnnounces replaces the announce list with new data.
func (nd *NetworkDisplay) UpdateAnnounces(announces []AnnounceEntry) {
	nd.announces.Clear()
	for _, ann := range announces {
		typeIcon := "○"
		switch ann.Type {
		case "node":
			typeIcon = "Ⓝ"
		case "pn":
			typeIcon = "↑"
		case "peer":
			typeIcon = "Ⓟ"
		}
		text := fmt.Sprintf("%s %s", typeIcon, ann.DisplayName)
		secondary := fmt.Sprintf("%s — %s", ann.Timestamp.Format("15:04:05"), truncateStr(ann.AppData, 30))
		nd.announces.AddItem(text, secondary, 0, nil)
	}
}

// truncateStr truncates a string to maxLen characters.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// formatAnnounce formats an announce for display.
func formatAnnounce(ann AnnounceEntry) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("[::b]%s[-]\n", ann.DisplayName))
	sb.WriteString(fmt.Sprintf("Type: %s\n", ann.Type))
	sb.WriteString(fmt.Sprintf("Trust: %s\n", ann.TrustLevel))
	sb.WriteString(fmt.Sprintf("Hash: %s\n", ann.SourceHash))
	sb.WriteString(fmt.Sprintf("Time: %s\n", ann.Timestamp.Format("2006-01-02 15:04:05")))
	sb.WriteString(fmt.Sprintf("Data: %s\n", ann.AppData))
	return sb.String()
}
