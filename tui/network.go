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
// Matches Python's NetworkDisplay with KnownNodes, AnnounceStream,
// and toggle between them via ctrl-l.
type NetworkDisplay struct {
	app          *tview.Application
	widget       *tview.Flex
	leftPanel    *tview.Flex
	announces    *tview.List
	nodes        *tview.List
	detail       *tview.TextView
	showingNodes bool
	inInfoView   bool
	displayMode  DisplayMode
	announceData []AnnounceEntry
	onNavigate   func(url string)

	// Keyboard shortcut callbacks (Python: NetworkDisplay.keypress)
	OnToggleFullscreen func()
	OnToggleList       func()
	OnEditNode         func()
	OnShowPeers        func()
	OnDisconnect       func()
	OnURLDialog        func()
	OnSaveNode         func()
	OnDeleteSelected   func()
}

// NewNetworkDisplay creates a new network display matching Python's layout.
func NewNetworkDisplay(app *tview.Application, announces []AnnounceEntry, nodes []NodeEntry) *NetworkDisplay {
	nd := &NetworkDisplay{app: app, showingNodes: false}

	// Title
	title := tview.NewTextView()
	title.SetTextAlign(tview.AlignCenter)
	title.SetDynamicColors(true)
	title.SetTextColor(tcell.NewHexColor(0xdddddd))
	title.SetText("[::b]Network[-]")

	// Announces list
	nd.announces = tview.NewList()
	nd.announces.SetHighlightFullLine(true)
	nd.announces.SetSelectedBackgroundColor(tcell.NewHexColor(0x666666))

	for _, ann := range announces {
		nd.addAnnounceEntry(ann)
	}

	// Nodes list
	nd.nodes = tview.NewList()
	nd.nodes.SetHighlightFullLine(true)
	nd.nodes.SetSelectedBackgroundColor(tcell.NewHexColor(0x666666))

	for _, node := range nodes {
		nd.addNodeEntry(node)
	}

	// Detail view
	nd.detail = tview.NewTextView()
	nd.detail.SetDynamicColors(true)
	nd.detail.SetScrollable(true)
	nd.detail.SetTextColor(tcell.NewHexColor(0xbbbbbb))
	nd.detail.SetText("[gray]Select an announce or node to view details[-]")

	// Left panel: announces by default, nodes hidden
	nd.leftPanel = tview.NewFlex().SetDirection(tview.FlexRow)
	nd.leftPanel.AddItem(nd.announces, 0, 1, true)

	// Content: left panel + detail
	content := tview.NewFlex().SetDirection(tview.FlexColumn)
	content.AddItem(nd.leftPanel, 0, 1, true)
	content.AddItem(nd.detail, 0, 2, false)

	nd.widget = tview.NewFlex().SetDirection(tview.FlexRow)
	nd.widget.SetBorder(true)
	nd.widget.AddItem(title, 2, 0, false)
	nd.widget.AddItem(content, 0, 1, true)

	// Set up list callbacks
	nd.announces.SetSelectedFunc(func(i int, mainText, secondaryText string, shortcut rune) {
		nd.showAnnounceDetail(i)
	})
	nd.nodes.SetSelectedFunc(func(i int, mainText, secondaryText string, shortcut rune) {
		nd.showNodeDetail(i)
	})

	return nd
}

// SetNavigateCallback sets the callback invoked when the user wants
// to open a URL in the browser (e.g., Connect on a node).
func (nd *NetworkDisplay) SetNavigateCallback(fn func(url string)) {
	nd.onNavigate = fn
}

// addAnnounceEntry adds a single announce to the list.
func (nd *NetworkDisplay) addAnnounceEntry(ann AnnounceEntry) {
	text := FormatAnnounceFull(ann, false)
	secondary := fmt.Sprintf("%s — %s", ann.Timestamp.Format("15:04:05"), truncateStr(ann.AppData, 30))
	nd.announces.AddItem(text, secondary, 0, nil)
}

// addNodeEntry adds a single node to the list.
func (nd *NetworkDisplay) addNodeEntry(node NodeEntry) {
	trustIcon := "○"
	if node.TrustLevel == "trusted" {
		trustIcon = "●"
	} else if node.TrustLevel == "untrusted" {
		trustIcon = "×"
	}
	text := fmt.Sprintf("%s %s", trustIcon, node.DisplayName)
	secondary := fmt.Sprintf("Delivery: %s", node.Delivery)
	nd.nodes.AddItem(text, secondary, 0, nil)
}

// showAnnounceDetail replaces the left panel with an AnnounceInfo view
// matching Python's AnnounceInfo widget. Shows Time, Addr, Type, Name,
// Trust, Operator (for nodes), Announce Data, and action buttons.
func (nd *NetworkDisplay) showAnnounceDetail(i int) {
	if i < 0 || i >= len(nd.announceData) {
		return
	}
	ann := nd.announceData[i]

	isNode := ann.Type == "node"
	isPN := ann.Type == "pn"

	// Build info text
	var sb strings.Builder
	tsStr := ann.Timestamp.Format("2006-01-02 15:04:05")

	typeStr := "Peer Ⓟ"
	if isNode {
		typeStr = "Nomad Network Node Ⓝ"
	} else if isPN {
		typeStr = "LXMF Propagation Node ↑"
	}

	addrStr := "<" + ann.SourceHash + ">"
	displayStr := ann.DisplayName
	if displayStr == "" {
		displayStr = ann.SourceHash
	}

	sb.WriteString(fmt.Sprintf("[::b]Time  :[-]  %s\n", tsStr))
	sb.WriteString(fmt.Sprintf("[::b]Addr  :[-]  [lightblue]%s[-]\n", addrStr))
	sb.WriteString(fmt.Sprintf("[::b]Type  :[-]  %s\n", typeStr))
	sb.WriteString(fmt.Sprintf("[::b]Name  :[-]  %s\n", displayStr))

	if isNode {
		sb.WriteString(fmt.Sprintf("[::b]Trust :[-]  %s\n", ann.TrustLevel))
	}

	if ann.AppData != "" {
		dataStr := truncateStr(ann.AppData, 120)
		sb.WriteString(fmt.Sprintf("\n[::b]Announce Data:[-]\n%s\n", dataStr))
	}

	infoText := tview.NewTextView().
		SetDynamicColors(true).
		SetTextColor(tcell.NewHexColor(0xbbbbbb)).
		SetText(sb.String())

	// Build buttons matching Python's layout
	var buttons *tview.Flex
	if isNode {
		buttons = tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(nd.makeButton("[Back]", func() { nd.showAnnounceStream() }), 8, 1, false).
			AddItem(tview.NewTextView().SetText(" "), 1, 0, false).
			AddItem(nd.makeButton("[Connect]", func() { nd.connectToNode(ann) }), 12, 1, false).
			AddItem(tview.NewTextView().SetText(" "), 1, 0, false).
			AddItem(nd.makeButton("[Msg Op]", func() { nd.msgOpNode(ann) }), 11, 1, false).
			AddItem(tview.NewTextView().SetText(" "), 1, 0, false).
			AddItem(nd.makeButton("[Save]", func() { nd.saveNode(ann) }), 9, 1, false)
	} else if isPN {
		buttons = tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(nd.makeButton("[Back]", func() { nd.showAnnounceStream() }), 8, 1, false).
			AddItem(tview.NewTextView().SetText(" "), 1, 0, false).
			AddItem(nd.makeButton("[Use as default]", func() { nd.useAsPN(ann) }), 20, 1, false)
	} else {
		buttons = tview.NewFlex().SetDirection(tview.FlexColumn).
			AddItem(nd.makeButton("[Back]", func() { nd.showAnnounceStream() }), 8, 1, false).
			AddItem(tview.NewTextView().SetText(" "), 1, 0, false).
			AddItem(nd.makeButton("[Converse]", func() { nd.converseWith(ann) }), 14, 1, false)
	}

	// Layout: info + divider + buttons
	infoView := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(infoText, 0, 1, false).
		AddItem(buttons, 1, 0, false)
	infoView.SetBorder(true)
	infoView.SetBorderColor(tcell.NewHexColor(0x888888))

	// Swap into left panel
	nd.leftPanel.RemoveItem(nd.announces)
	nd.leftPanel.RemoveItem(nd.nodes)
	nd.leftPanel.AddItem(infoView, 0, 1, true)
	nd.inInfoView = true

	// Show announce detail in the right panel
	nd.detail.SetText(formatAnnounce(ann))
}

// showNodeDetail is an alias — nodes are selected from the nodes list.
func (nd *NetworkDisplay) showNodeDetail(i int) {
	nd.showAnnounceDetail(i)
}

// showAnnounceStream restores the left panel to the announce/node list.
func (nd *NetworkDisplay) showAnnounceStream() {
	if !nd.inInfoView {
		return
	}

	// Remove info view from left panel
	nd.leftPanel.Clear()

	// Restore the appropriate list
	if nd.showingNodes {
		nd.leftPanel.AddItem(nd.nodes, 0, 1, true)
	} else {
		nd.leftPanel.AddItem(nd.announces, 0, 1, true)
	}
	nd.inInfoView = false
}

// HandleEsc returns from AnnounceInfo to the list. Returns true if the
// event was consumed.
func (nd *NetworkDisplay) HandleEsc() bool {
	if nd.inInfoView {
		nd.showAnnounceStream()
		return true
	}
	return false
}

// connectToNode opens the browser to the node's page URL.
// Matches Python's connect(sender): self.parent.browser.retrieve_url(...)
func (nd *NetworkDisplay) connectToNode(ann AnnounceEntry) {
	nd.showAnnounceStream()
	if nd.onNavigate != nil {
		nd.onNavigate(ann.SourceHash)
	}
}

// msgOpNode starts a conversation with the node's operator.
// Matches Python's msg_op(sender).
func (nd *NetworkDisplay) msgOpNode(ann AnnounceEntry) {
	nd.showAnnounceStream()
	// TODO: Open conversation with the node's operator identity.
	// Requires resolving the operator's LXMF delivery address from
	// the node's source hash via RNS.Identity.
}

// saveNode saves the node to the directory.
// Matches Python's save_node(sender).
func (nd *NetworkDisplay) saveNode(ann AnnounceEntry) {
	nd.showAnnounceStream()
	// TODO: Save to directory via app.directory.remember().
}

// useAsPN sets the announce's source as the default propagation node.
// Matches Python's use_pn(sender).
func (nd *NetworkDisplay) useAsPN(ann AnnounceEntry) {
	nd.showAnnounceStream()
	// TODO: Set via app.set_user_selected_propagation_node().
}

// converseWith starts a conversation with a peer.
// Matches Python's converse(sender).
func (nd *NetworkDisplay) converseWith(ann AnnounceEntry) {
	nd.showAnnounceStream()
	// TODO: Open conversation with the peer.
}

// makeButton creates a tview.Button styled for the AnnounceInfo view.
func (nd *NetworkDisplay) makeButton(label string, action func()) *tview.Button {
	btn := tview.NewButton(label)
	btn.SetBackgroundColor(tcell.NewHexColor(0x444444))
	btn.SetLabelColor(tcell.NewHexColor(0xdddddd))
	btn.SetSelectedFunc(action)
	return btn
}

// Widget returns the tview primitive for this display.
func (nd *NetworkDisplay) Widget() tview.Primitive {
	return nd.widget
}

// UpdateAnnounces replaces the announce list with new data,
// preserving the current scroll position.
func (nd *NetworkDisplay) UpdateAnnounces(announces []AnnounceEntry) {
	nd.announceData = announces

	// Save current position before clearing.
	currentItem := nd.announces.GetCurrentItem()

	nd.announces.Clear()
	for _, ann := range announces {
		nd.addAnnounceEntryWithMode(ann)
	}

	// Restore position, clamping to valid range.
	newCount := nd.announces.GetItemCount()
	if newCount > 0 {
		if currentItem >= newCount {
			currentItem = newCount - 1
		}
		nd.announces.SetCurrentItem(currentItem)
	}

	if nd.app != nil {
		nd.app.QueueUpdateDraw(func() {})
	}
}

// toggleList switches between announces and nodes views.
func (nd *NetworkDisplay) toggleList() {
	if nd.inInfoView {
		nd.showAnnounceStream()
	}

	nd.leftPanel.RemoveItem(nd.announces)
	nd.leftPanel.RemoveItem(nd.nodes)

	if nd.showingNodes {
		nd.leftPanel.AddItem(nd.announces, 0, 1, true)
		nd.showingNodes = false
	} else {
		nd.leftPanel.AddItem(nd.nodes, 0, 1, true)
		nd.showingNodes = true
	}
}

// ToggleDisplayMode toggles between showing display names and
// destination hashes in the announce stream.
func (nd *NetworkDisplay) ToggleDisplayMode() {
	nd.displayMode = ToggleDisplayMode(nd.displayMode)
	nd.rebuildAnnounceList()
}

// rebuildAnnounceList repopulates the announce list using the
// current display mode.
func (nd *NetworkDisplay) rebuildAnnounceList() {
	nd.announces.Clear()
	for _, ann := range nd.announceData {
		nd.addAnnounceEntryWithMode(ann)
	}
	if nd.app != nil {
		nd.app.QueueUpdateDraw(func() {})
	}
}

// addAnnounceEntryWithMode adds a single announce using the current display mode.
func (nd *NetworkDisplay) addAnnounceEntryWithMode(ann AnnounceEntry) {
	text := FormatAnnounceFull(ann, nd.displayMode == DisplayHash)
	secondary := fmt.Sprintf("%s — %s", ann.Timestamp.Format("15:04:05"), truncateStr(ann.AppData, 30))
	nd.announces.AddItem(text, secondary, 0, nil)
}

// GetDisplayMode returns the current display mode.
func (nd *NetworkDisplay) GetDisplayMode() DisplayMode {
	return nd.displayMode
}

// DisplayModeLabel returns the toggle button label for the current mode.
func (nd *NetworkDisplay) DisplayModeLabel() string {
	if nd.displayMode == DisplayName {
		return "Show: Name"
	}
	return "Show: Dest"
}

// truncateStr truncates a string to maxLen characters.
func truncateStr(s string, maxLen int) string {
	if len([]rune(s)) <= maxLen {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxLen-3]) + "..."
}

// DisplayMode controls whether the announce stream shows names or hashes.
type DisplayMode int

const (
	// DisplayName shows the peer's display name.
	DisplayName DisplayMode = iota
	// DisplayHash shows the raw destination hex hash.
	DisplayHash
)

// ToggleDisplayMode returns the alternate display mode.
func ToggleDisplayMode(mode DisplayMode) DisplayMode {
	if mode == DisplayName {
		return DisplayHash
	}
	return DisplayName
}

// FormatAnnounceEntry returns the display text for an announce entry
// based on the given display mode. When showHash is true, the raw
// source hash is shown instead of the display name.
func FormatAnnounceEntry(ann AnnounceEntry, showHash bool) string {
	if showHash {
		return ann.SourceHash
	}
	if ann.DisplayName != "" {
		return ann.DisplayName
	}
	return ann.SourceHash
}

// FormatAnnounceFull returns the full announce line with type icon and
// display text. Used for the announce stream list items.
func FormatAnnounceFull(ann AnnounceEntry, showHash bool) string {
	typeIcon := "○"
	switch ann.Type {
	case "node":
		typeIcon = "Ⓝ"
	case "pn":
		typeIcon = "↑"
	case "peer":
		typeIcon = "Ⓟ"
	}
	return typeIcon + " " + FormatAnnounceEntry(ann, showHash)
}

// formatAnnounce formats an announce for the detail panel.
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
