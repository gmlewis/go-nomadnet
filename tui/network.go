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
	TimestampF  float64 // same instant as Timestamp, as the float64 seconds the directory stores
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
	app           *App
	widget        *tview.Flex
	leftPanel     *tview.Flex
	announces       *tview.List
	announcesList   *IndicativeListBox
	nodes           *tview.List
	nodesList       *IndicativeListBox
	nodeEmptyState  *tview.TextView
	detail          *tview.TextView
	showingNodes  bool
	inInfoView    bool
	displayMode   DisplayMode
	SanitizeNames bool
	announceData  []AnnounceEntry
	nodeData      []NodeEntry
	onNavigate    func(url string)

	// Keyboard shortcut callbacks (Python: NetworkDisplay.keypress)
	OnToggleFullscreen func()
	OnToggleList       func()
	OnEditNode         func()
	OnShowPeers        func()
	OnDisconnect       func()
	OnURLDialog        func()
	OnSaveNode         func()
	OnDeleteSelected   func()

	// In-detail action callbacks (Python AnnounceInfo buttons). Each is no-arg;
	// the wiring layer resolves the target via SelectedAnnounce/SelectedNode.
	OnMsgOp    func() // [Msg Op] — message the node operator
	OnUseAsPN  func() // [Use as default] — set default propagation node
	OnConverse func() // [Converse] — open a conversation with the peer
}

// NewNetworkDisplay creates a new network display matching Python's layout.
func NewNetworkDisplay(app *App, announces []AnnounceEntry, nodes []NodeEntry) *NetworkDisplay {
	// Python defaults list_display=1 (KnownNodes shown first, Network.py:1638),
	// so the left pane opens on "Saved Nodes", not the announce stream.
	nd := &NetworkDisplay{app: app, showingNodes: true}

	// Announces list. Python's AnnounceStreamEntry is a single-row ListEntry
	// (urwid.Text), so disable tview's secondary-text row (blank line under
	// every item) — the row text already carries the whole entry.
	nd.announces = tview.NewList()
	nd.announces.SetHighlightFullLine(true)
	nd.announces.ShowSecondaryText(false)
	ApplyListFocusStyle(nd.announces, app.Theme)

	nd.announceData = announces
	for _, ann := range announces {
		nd.addAnnounceEntry(ann)
	}

	// Nodes list. Same single-row ListEntry basis as the announce stream.
	nd.nodes = tview.NewList()
	nd.nodes.SetHighlightFullLine(true)
	nd.nodes.ShowSecondaryText(false)
	ApplyListFocusStyle(nd.nodes, app.Theme)

	nd.nodeData = nodes
	for _, node := range nodes {
		nd.addNodeEntry(node)
	}

	// Wrap the lists with IndicativeListBox end-indicator bars (───/▲/▼),
	// matching the original IndicativeListBox. The bare Lists remain the
	// focus/manipulation targets; the wrappers are the layout children.
	nd.announcesList = NewIndicativeListBox(nd.announces)
	nd.nodesList = NewIndicativeListBox(nd.nodes)

	// KnownNodes empty-state (Network.py:833-882): when no nodes are saved the
	// "Saved Nodes" LineBox shows a centered warning-colored info glyph followed
	// by "Currently, no nodes are saved\n\nCtrl+L to view the announce stream",
	// in a TOP-filled Filler. The whole message uses the warning_text palette
	// color (#ba4 → #bbaa44). Shown in place of the nodes list when it is empty.
	nd.nodeEmptyState = tview.NewTextView()
	nd.nodeEmptyState.SetTextAlign(tview.AlignCenter)
	nd.nodeEmptyState.SetDynamicColors(false)
	nd.nodeEmptyState.SetTextColor(GetThemeColors(app.Theme)["warning_text"])
	nd.nodeEmptyState.SetText(nd.glyphs()["info"] + "\n\nCurrently, no nodes are saved\n\nCtrl+L to view the announce stream\n\n")

	// Detail view
	nd.detail = tview.NewTextView()
	nd.detail.SetDynamicColors(true)
	nd.detail.SetScrollable(true)
	nd.detail.SetTextColor(tcell.NewHexColor(0xbbbbbb))
	nd.detail.SetText("[gray]Select an announce or node to view details[-]")
	// Python's right pane is the Browser display (a LineBox); the detail pane
	// carries its own border so the two panes render with separate borders and
	// no outer box around the page.
	nd.detail.SetBorder(true)

	// Left panel: Saved Nodes by default (Python list_display=1). Python's left
	// sub-widgets each carry their own titled LineBox — KnownNodes is titled
	// "Saved Nodes" (Network.py:867), AnnounceStream is titled "Announce Stream"
	// (Network.py:446). The panel border+title reflects the active list mode and
	// is updated on toggle. When the saved-nodes list is empty the empty-state
	// message is shown in its place.
	nd.leftPanel = tview.NewFlex().SetDirection(tview.FlexRow)
	nd.leftPanel.SetBorder(true)
	SetTitledBorder(nd.leftPanel, "Saved Nodes")
	nd.leftPanel.AddItem(nd.nodesView(), 0, 1, true)

	// Content: left panel + detail. Python: self.widget = self.columns
	// (Network.py:1666) — NO outer LineBox or title around the page; the two
	// columns sit directly in the body.
	nd.widget = tview.NewFlex().SetDirection(tview.FlexColumn)
	nd.widget.SetInputCapture(nd.handleInput)
	nd.widget.AddItem(nd.leftPanel, 52, 0, true)
	nd.widget.AddItem(nd.detail, 0, 1, false)

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

// handleInput processes keyboard shortcuts for the network display.
// Matches Python's NetworkDisplay.keypress() at Network.py:1600.
func (nd *NetworkDisplay) handleInput(event *tcell.EventKey) *tcell.EventKey {
	// When AnnounceInfo is open, only Esc is handled (via MainDisplay.onEsc).
	if nd.inInfoView {
		return event
	}

	switch event.Key() {
	case tcell.KeyCtrlL:
		nd.toggleList()
		return nil
	case tcell.KeyCtrlG:
		if nd.OnToggleFullscreen != nil {
			nd.OnToggleFullscreen()
		}
		return nil
	case tcell.KeyCtrlE:
		if nd.OnEditNode != nil {
			nd.OnEditNode()
		}
		return nil
	case tcell.KeyCtrlP:
		if nd.OnShowPeers != nil {
			nd.OnShowPeers()
		}
		return nil
	case tcell.KeyCtrlW:
		if nd.OnDisconnect != nil {
			nd.OnDisconnect()
		}
		return nil
	case tcell.KeyCtrlU:
		if nd.OnURLDialog != nil {
			nd.OnURLDialog()
		}
		return nil
	case tcell.KeyCtrlS:
		if nd.OnSaveNode != nil {
			nd.OnSaveNode()
		}
		return nil
	case tcell.KeyCtrlX:
		if nd.OnDeleteSelected != nil {
			nd.OnDeleteSelected()
		}
		return nil
	}

	return event
}

// addAnnounceEntry adds a single announce to the list.
func (nd *NetworkDisplay) addAnnounceEntry(ann AnnounceEntry) {
	nd.addAnnounceEntryWithMode(ann)
}

// addNodeEntry adds a single node to the list. The row text mirrors Python's
// NodeEntry (Network.py:984-1015): "{node_glyph} {display_str}" with no
// secondary text. Per-item trust coloring (list_trusted/list_untrusted/...) is
// applied via the palette/theme, not tview color tags, matching the
// AnnounceStreamEntry wiring.
func (nd *NetworkDisplay) addNodeEntry(node NodeEntry) {
	text := FormatNodeEntryRow(node, nd.glyphs())
	nd.nodes.AddItem(text, "", 0, nil)
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

	// Layout: info + divider + buttons. The info view uses the left panel's
	// own border (no separate border → no nested borders); the panel title
	// switches to "Announce Info" while the detail is shown.
	infoView := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(infoText, 0, 1, false).
		AddItem(buttons, 1, 0, false)

	// Swap into left panel
	nd.leftPanel.RemoveItem(nd.announcesList)
	nd.leftPanel.RemoveItem(nd.nodesList)
	SetTitledBorder(nd.leftPanel, "Announce Info")
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
		nd.leftPanel.AddItem(nd.nodesView(), 0, 1, true)
		SetTitledBorder(nd.leftPanel, "Saved Nodes")
	} else {
		nd.leftPanel.AddItem(nd.announcesList, 0, 1, true)
		SetTitledBorder(nd.leftPanel, "Announce Stream")
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
	if nd.OnMsgOp != nil {
		nd.OnMsgOp()
	}
}

// saveNode saves the node to the directory.
// Matches Python's save_node(sender).
func (nd *NetworkDisplay) saveNode(ann AnnounceEntry) {
	nd.showAnnounceStream()
	if nd.OnSaveNode != nil {
		nd.OnSaveNode()
	}
}

// useAsPN sets the announce's source as the default propagation node.
// Matches Python's use_pn(sender).
func (nd *NetworkDisplay) useAsPN(ann AnnounceEntry) {
	nd.showAnnounceStream()
	if nd.OnUseAsPN != nil {
		nd.OnUseAsPN()
	}
}

// converseWith starts a conversation with a peer.
// Matches Python's converse(sender).
func (nd *NetworkDisplay) converseWith(ann AnnounceEntry) {
	nd.showAnnounceStream()
	if nd.OnConverse != nil {
		nd.OnConverse()
	}
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

// SelectedAnnounce returns the announce currently selected in the announce
// stream list, or ok=false if the stream is empty/not showing.
func (nd *NetworkDisplay) SelectedAnnounce() (AnnounceEntry, bool) {
	if nd.showingNodes || len(nd.announceData) == 0 {
		return AnnounceEntry{}, false
	}
	idx := nd.announces.GetCurrentItem()
	if idx < 0 || idx >= len(nd.announceData) {
		return AnnounceEntry{}, false
	}
	return nd.announceData[idx], true
}

// SelectedNode returns the node currently selected in the saved-nodes list, or
// ok=false if the nodes view is not active or empty.
func (nd *NetworkDisplay) SelectedNode() (NodeEntry, bool) {
	if !nd.showingNodes || len(nd.nodeData) == 0 {
		return NodeEntry{}, false
	}
	idx := nd.nodes.GetCurrentItem()
	if idx < 0 || idx >= len(nd.nodeData) {
		return NodeEntry{}, false
	}
	return nd.nodeData[idx], true
}

// UpdateNodes replaces the saved-nodes list with the given entries.
func (nd *NetworkDisplay) UpdateNodes(nodes []NodeEntry) {
	nd.nodeData = nodes
	current := nd.nodes.GetCurrentItem()
	nd.nodes.Clear()
	for _, node := range nodes {
		nd.addNodeEntry(node)
	}
	if n := nd.nodes.GetItemCount(); n > 0 {
		if current >= n {
			current = n - 1
		}
		nd.nodes.SetCurrentItem(current)
	}
	// If the saved-nodes view is currently shown, swap the empty-state message
	// in/out so the pane reflects the new node count without a manual toggle.
	nd.refreshNodesView()
	if nd.app != nil {
		nd.app.QueueUpdateDraw(func() {})
	}
}

// refreshNodesView swaps the empty-state message in/out of the left pane when
// the saved-nodes mode is active, so the pane matches the current node count
// without a manual toggle. It is a no-op unless the saved-nodes list is the
// current view. Split out of UpdateNodes so tests can drive the swap without
// QueueUpdateDraw (which blocks forever with no event loop running).
func (nd *NetworkDisplay) refreshNodesView() {
	if !nd.showingNodes || nd.inInfoView {
		return
	}
	nd.leftPanel.RemoveItem(nd.nodesList)
	nd.leftPanel.RemoveItem(nd.nodeEmptyState)
	nd.leftPanel.AddItem(nd.nodesView(), 0, 1, true)
}

// ShowingNodes reports whether the saved-nodes list is currently displayed
// (vs. the announce stream). Used by wiring to pick the right delete action.
func (nd *NetworkDisplay) ShowingNodes() bool { return nd.showingNodes }

// nodesView returns the left-pane widget for the saved-nodes mode: the
// IndicativeListBox when nodes exist, or the centered empty-state message when
// none are saved (Python KnownNodes empty-state, Network.py:833-882).
func (nd *NetworkDisplay) nodesView() tview.Primitive {
	if nd.nodes.GetItemCount() == 0 {
		return nd.nodeEmptyState
	}
	return nd.nodesList
}

// toggleList switches between announces and nodes views.
func (nd *NetworkDisplay) toggleList() {
	if nd.inInfoView {
		nd.showAnnounceStream()
	}

	nd.leftPanel.RemoveItem(nd.announcesList)
	nd.leftPanel.RemoveItem(nd.nodesList)
	nd.leftPanel.RemoveItem(nd.nodeEmptyState)

	if nd.showingNodes {
		nd.leftPanel.AddItem(nd.announcesList, 0, 1, true)
		nd.showingNodes = false
		SetTitledBorder(nd.leftPanel, "Announce Stream")
	} else {
		nd.leftPanel.AddItem(nd.nodesView(), 0, 1, true)
		nd.showingNodes = true
		SetTitledBorder(nd.leftPanel, "Saved Nodes")
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
	text := FormatAnnounceStreamRow(ann, time.Now(), nd.displayMode == DisplayHash, nd.SanitizeNames, nd.glyphs())
	nd.announces.AddItem(text, "", 0, nil)
}

// glyphs returns the glyph set for this display, falling back to unicode.
func (nd *NetworkDisplay) glyphs() GlyphSet {
	if nd.app != nil && nd.app.Glyphs != nil {
		return nd.app.Glyphs
	}
	return glyphsUnicode
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

// ShowLocalPeerDialog shows the local peer information panel.
// Matches Python's LocalPeer at Network.py:1259-1350.
func (nd *NetworkDisplay) ShowLocalPeerDialog(lxmfAddr, identityHash, name string, lastAnnounce string) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(" LXMF Addr : %s\n", lxmfAddr))
	sb.WriteString(fmt.Sprintf(" Identity  : %s\n", identityHash))
	sb.WriteString(fmt.Sprintf(" Name      : %s\n", name))
	if lastAnnounce != "" {
		sb.WriteString(fmt.Sprintf(" Last Announce: %s\n", lastAnnounce))
	}

	buttons := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(tview.NewButton("Save").SetSelectedFunc(func() {
			// Save name
		}), 0, 1, true).
		AddItem(tview.NewButton("Announce Now").SetSelectedFunc(func() {
			// Trigger announce
		}), 0, 1, false)

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewTextView().
			SetDynamicColors(true).
			SetTextColor(tcell.NewHexColor(0xdddddd)).
			SetText(sb.String()), 0, 1, false).
		AddItem(buttons, 1, 0, false)

	nd.app.Dialogs.ShowDialog("Local Peer", layout, 50, 10, nil)
}

// ShowLXMFPeersDialog shows the LXMF peers list panel.
// Matches Python's LXMFPeers at Network.py:1752-1860.
func (nd *NetworkDisplay) ShowLXMFPeersDialog(peers []LXMFPeerEntry) {
	list := tview.NewList()
	list.SetHighlightFullLine(true)
	ApplyListFocusStyle(list, nd.app.Theme)

	for _, peer := range peers {
		status := "[green]alive[-]"
		if !peer.Alive {
			status = "[red]dead[-]"
		}
		text := fmt.Sprintf("%s %s %s", peer.Name, status, truncateStr(peer.Hash, 8))
		secondary := fmt.Sprintf("Pending: %d", peer.Pending)
		list.AddItem(text, secondary, 0, nil)
	}

	if len(peers) == 0 {
		list.AddItem("[gray]No propagation peers[-]", "", 0, nil)
	}

	buttons := tview.NewFlex().SetDirection(tview.FlexColumn).
		AddItem(tview.NewButton("Delete").SetSelectedFunc(func() {
			// Delete selected peer
		}), 0, 1, false).
		AddItem(tview.NewButton("Sync").SetSelectedFunc(func() {
			// Sync selected peer
		}), 0, 1, false).
		AddItem(tview.NewButton("Close").SetSelectedFunc(func() {}), 0, 1, false)

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(list, 0, 1, true).
		AddItem(buttons, 1, 0, false)

	nd.app.Dialogs.ShowDialog("LXMF Peers", layout, 50, 12, nil)
}
