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
	"os"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// debugNetLog is a TEMPORARY diagnostic appended to /tmp/peek-debug.log tracing
// every key the network display sees + the mainCols focus index and which pane
// has focus. Remove after diagnosing the test-gonomadnet-input-box focus failure.
func debugNetLog(nd *NetworkDisplay, event *tcell.EventKey) {
	f, err := os.OpenFile("/tmp/peek-debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	key := event.Name()
	fi := -1
	if nd.mainCols != nil {
		fi = nd.mainCols.FocusIndex()
	}
	browser := "no"
	if nd.browser != nil && nd.browser.Widget().HasFocus() {
		browser = "yes"
	}
	left := "no"
	if nd.leftPanel.HasFocus() {
		left = "yes"
	}
	fmt.Fprintf(f, "NET key=%s focusIdx=%d browserHasFocus=%s leftHasFocus=%s\n", key, fi, browser, left)
}

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
	app            *App
	widget         tview.Primitive
	leftPanel      *tview.Flex
	announces      *tview.List
	announcesList  *IndicativeListBox
	announceStream *announceStreamDisplay
	nodes          *tview.List
	nodesList      *IndicativeListBox
	nodeEmptyState *centeredText
	listBox        *tview.Flex // bordered, titled list slot (Saved Nodes/Announce Stream/Announce Info/…)
	localPeer      *LocalPeerDisplay
	nodeInfo       *NodeInfoDisplay
	lxmfPeers      *LXMFPeersDisplay
	browser        *BrowserPane
	mainCols       *urwidColumns
	showingNodes   bool
	showingPeers   bool
	inInfoView     bool
	displayMode    DisplayMode
	// listWidth is the normal (non-fullscreen) fixed width of the left list
	// pane (Python NetworkDisplay.given_list_width = 52, Network.py:1622).
	// fullscreen is true while Ctrl-G has collapsed the left pane to width 0 so
	// the in-page browser fills the page. See ToggleFullscreen.
	listWidth     int
	fullscreen    bool
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

	// LXMF peers list callbacks (Python LXMFPeers.keypress, Network.py:1793-
	// 1798). OnLXMFPeerUnpeer is ctrl-x; OnLXMFPeerSync is ctrl-r. Each receives
	// the selected peer's destination hash. The wiring layer calls
	// message_router.unpeer / peer.sync + reinit/show_peers.
	OnLXMFPeerUnpeer func(destinationHash []byte)
	OnLXMFPeerSync   func(destinationHash []byte)

	// In-detail action callbacks (Python AnnounceInfo buttons). Each is no-arg;
	// the wiring layer resolves the target via SelectedAnnounce/SelectedNode.
	OnMsgOp    func() // [Msg Op] — message the node operator
	OnUseAsPN  func() // [Use as default] — set default propagation node
	OnConverse func() // [Converse] — open a conversation with the peer

	// OnResolveAnnounceInfo resolves the directory-backed fields an AnnounceInfo
	// view needs at view time (Python AnnounceInfo __init__: trust_level,
	// simplest_display_str, op_str). Returns ok=false when no resolver is wired
	// (the view then falls back to the AnnounceEntry's own fields).
	OnResolveAnnounceInfo func(ann AnnounceEntry) (AnnounceInfoData, bool)

	// OnResolveKnownNodeInfo resolves the directory-backed fields a KnownNodeInfo
	// form needs at view time (Python KnownNodeInfo __init__: display_str,
	// sort_rank, trust_level, op_str, hops, pn address, identify_on_connect).
	// Returns ok=false when no resolver is wired (the form falls back to the
	// node hash + Unknown defaults).
	OnResolveKnownNodeInfo func(nodeHash string) (KnownNodeInfoData, bool)

	// OnKnownNodeSave writes the edited KnownNodeInfo form state to the directory
	// (Python save_node, Network.py:755-785): name, sort rank, trust, default-PN,
	// identify-on-connect. The display collects the form data and passes it here.
	OnKnownNodeSave func(nodeHash string, data KnownNodeInfoFormData)
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

	// AnnounceStream Pile (tab bar + search/display-toggle + list), mirroring
	// Python's AnnounceStream (Network.py:394-551). Built after the
	// IndicativeListBox so it can wrap it; its update() populates the announce
	// list from announceData (counting/filtering by type + search text).
	nd.announceStream = newAnnounceStreamDisplay(nd)
	nd.nodesList = NewIndicativeListBox(nd.nodes)

	// KnownNodes empty-state (Network.py:833-882): when no nodes are saved the
	// "Saved Nodes" LineBox shows a centered warning-colored info glyph followed
	// by "Currently, no nodes are saved\n\nCtrl+L to view the announce stream",
	// in a TOP-filled Filler. The whole message uses the warning_text palette
	// color (#ba4 → #bbaa44). Shown in place of the nodes list when it is empty.
	// KnownNodes empty-state (Network.py:833-882): when no nodes are saved the
	// "Saved Nodes" LineBox shows a centered warning-colored info glyph followed
	// by "Currently, no nodes are saved\n\nCtrl+L to view the announce stream",
	// in a TOP-filled Filler. The whole message uses the warning_text palette
	// color (#ba4 → #bbaa44). Shown in place of the nodes list when it is empty.
	// centeredText ceil-left-centers each line to match urwid (tview's
	// AlignCenter floors, landing 1 col right of the original on odd slack).
	nd.nodeEmptyState = newCenteredText(
		GetThemeColors(app.Theme)["warning_text"],
		nd.glyphs()["info"], "",
		"Currently, no nodes are saved", "",
		"Ctrl+L to view the announce stream", "", "",
	)

	// Right pane: the "Remote Node" browser (Python self.browser.display_widget,
	// Browser.py:486). Boot state is the disconnected view — a bordered "Remote
	// Node" LineBox with a centered "Disconnected / ←  →" (browser_inactive
	// #444). URL fetching / page rendering arrive in Phase 5 (the RNS link);
	// until then this matches Python's boot appearance. The pane carries its
	// own border so the two panes render with separate borders and no outer box
	// around the page.
	nd.browser = NewBrowserPane(app)

	// Left panel: Saved Nodes by default (Python list_display=1). Python's left
	// sub-widgets each carry their own titled LineBox — KnownNodes is titled
	// "Saved Nodes" (Network.py:867), AnnounceStream is titled "Announce Stream"
	// (Network.py:446). The panel border+title reflects the active list mode and
	// is updated on toggle. When the saved-nodes list is empty the empty-state
	// message is shown in its place.
	// Local Peer Info panel, PACKed below the list in the left pane
	// (Network.py:1641-1644: left_pile = [(WEIGHT 1, known_nodes), (PACK,
	// local_peer)]). Created with empty data; the wiring layer fills it via
	// UpdateLocalPeer once the app's identity/LXMF destination are ready.
	nd.localPeer = NewLocalPeerDisplay(app, "", "", "", time.Time{})

	// LXMF Propagation Peers list (Python LXMFPeers, Network.py:1752). Built
	// with no peered nodes (the no-content branch) until the wiring layer
	// populates it via UpdateLXMFPeers in Phase 5 (the LXMF message router is
	// not wired yet). C-p swaps it into the left-pane list slot.
	nd.lxmfPeers = NewLXMFPeersDisplay(app)
	// Forward the wiring-layer ctrl-x/ctrl-r callbacks to the peers display.
	// These are read live each keypress, so wiring can set them after NewNetworkDisplay.
	nd.lxmfPeers.OnUnpeer = func(h []byte) {
		if nd.OnLXMFPeerUnpeer != nil {
			nd.OnLXMFPeerUnpeer(h)
		}
	}
	nd.lxmfPeers.OnSync = func(h []byte) {
		if nd.OnLXMFPeerSync != nil {
			nd.OnLXMFPeerSync(h)
		}
	}

	// The left pane is a PILE of two separately-bordered LineBoxes — the
	// mode-titled list (Saved Nodes/Announce Stream/Announce Info/…) and the
	// Local Peer Info panel — with NO outer border around the pile, matching
	// Python's NetworkLeftPile (Network.py:1641, 867, 446, 256). listBox carries
	// the list slot's border+title; setLeftList swaps its content and title.
	nd.listBox = tview.NewFlex().SetDirection(tview.FlexRow)
	nd.listBox.SetBorder(true)
	nd.setLeftList(nd.nodesView(), "Saved Nodes")

	nd.leftPanel = tview.NewFlex().SetDirection(tview.FlexRow)
	nd.leftPanel.AddItem(nd.listBox, 0, 1, true)
	nd.leftPanel.AddItem(nd.localPeer.Widget(), nd.localPeer.Height(), 0, false)

	// Content: left panel + detail. Python: self.widget = self.columns
	// (Network.py:1666) — NO outer LineBox or title around the page; the two
	// columns sit directly in the body.
	mainCols := newURWIDColumns(0, nd.leftPanel, nd.browser.Widget()).
		SetFixedWidth(0, 52).
		SetWeight(1, 1)
	nd.listWidth = 52
	mainCols.SetInputCapture(nd.handleInput)
	// The right pane (browser) is self-managing: it owns Left/Right for its
	// part-cursor model + Left-at-start focus release, so the outer Columns
	// forwards Left/Right to it instead of pane-wrapping (Python keeps the
	// browser body's keypress, which never bubbles Left/Right to the Columns).
	mainCols.SetSelfManaging(1, true)
	nd.mainCols = mainCols
	nd.widget = mainCols

	// Set up list callbacks. The announce list shows only the active tab's
	// filtered entries, so resolve the selected entry through the AnnounceStream
	// (its entries slice maps 1:1 to list rows).
	nd.announces.SetSelectedFunc(func(i int, mainText, secondaryText string, shortcut rune) {
		ann, ok := nd.announceStream.entryAt(i)
		if !ok {
			return
		}
		nd.showAnnounceDetailFor(ann)
	})
	// Saved-nodes list: Enter is a no-op (Python KnownNodes.node_list_selection
	// is `pass`, Network.py:888). The editable KnownNodeInfo form is opened by
	// Ctrl-E (NetworkLeftPile.keypress → selected_node_info, Network.py:1603),
	// wired via OnEditNode → ShowKnownNodeInfo in the wiring layer.

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
	debugNetLog(nd, event)
	// When AnnounceInfo is open, only Esc is handled (via MainDisplay.onEsc).
	if nd.inInfoView {
		return event
	}

	switch event.Key() {
	case tcell.KeyCtrlL:
		nd.toggleList()
		return nil
	case tcell.KeyCtrlG:
		nd.ToggleFullscreen()
		return nil
	case tcell.KeyCtrlE:
		if nd.OnEditNode != nil {
			nd.OnEditNode()
		}
		return nil
	case tcell.KeyCtrlP:
		// Python ctrl p: reinit_lxmf_peers() then show_peers()
		// (Network.py:1608-1609). OnShowPeers lets the wiring layer refresh
		// the peer set from the LXMF message router (Phase 5); showPeers
		// swaps the left-pane slot to the peers list regardless.
		if nd.OnShowPeers != nil {
			nd.OnShowPeers()
		}
		nd.showPeers()
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

// addNodeEntry adds a single node to the list. The row text mirrors Python's
// NodeEntry (Network.py:984-1015): "{node_glyph} {display_str}" with no
// secondary text. Per-item trust coloring (list_trusted/list_untrusted/...) is
// applied via the palette/theme, not tview color tags, matching the
// AnnounceStreamEntry wiring.
func (nd *NetworkDisplay) addNodeEntry(node NodeEntry) {
	text := FormatNodeEntryRow(node, nd.glyphs())
	nd.nodes.AddItem(text, "", 0, nil)
}

// showAnnounceDetail is retained for backward compatibility (tests); it looks
// up the entry in the full announce data by index. New callers should use
// showAnnounceDetailFor with the entry resolved through the AnnounceStream.
func (nd *NetworkDisplay) showAnnounceDetail(i int) {
	if i < 0 || i >= len(nd.announceData) {
		return
	}
	nd.showAnnounceDetailFor(nd.announceData[i])
}

// showAnnounceDetailFor replaces the left panel with an AnnounceInfo view
// matching Python's AnnounceInfo widget (Network.py:59-256): a TOP-filled Pile
// of Time/Addr/Type/Name/[Oprtr]/Trust rows, divider lines, the announce data
// block, and a weighted button row (node: Back/Connect/Msg Op/Save; pn:
// Back/Use as default; peer: Back/Converse). Directory-backed fields (trust,
// display string, operator) are resolved via OnResolveAnnounceInfo when wired.
func (nd *NetworkDisplay) showAnnounceDetailFor(ann AnnounceEntry) {
	data := AnnounceInfoData{
		DisplayStr: ann.DisplayName,
		TrustStr:   trustStringFromLevel(ann.TrustLevel),
		TrustStyle: trustStyleFromLevel(ann.TrustLevel),
		OpStr:      "Unknown",
	}
	if nd.OnResolveAnnounceInfo != nil {
		if resolved, ok := nd.OnResolveAnnounceInfo(ann); ok {
			data = resolved
		}
	}

	ai := newAnnounceInfoDisplay(nd, ann, data)
	nd.setLeftList(ai.Widget(), "Announce Info")
	nd.setInfoView(true)
	// Focus the AnnounceInfo button row (Python pile.focus_position = last =
	// buttons; button_columns.focus_position = 0 = Back), so Enter/click reach
	// the buttons.
	nd.focusLeftList()

	// The right pane stays the "Remote Node" browser (Python keeps the browser
	// in the right pane; AnnounceInfo is an overlay/left-swap, not a right-pane
	// detail). The announce fields are all in the left-pane AnnounceInfo view.
}

// trustStringFromLevel maps a Go trust-level string ("trusted"/"untrusted"/
// "unknown"/"warning", as produced by the wiring layer) to the display string
// Python's AnnounceInfo uses (Network.py:103-122). An empty level (announces
// carry no trust until resolved) defaults to Unknown.
func trustStringFromLevel(level string) string {
	switch level {
	case "trusted":
		return "Trusted"
	case "untrusted":
		return "Untrusted"
	case "warning":
		return "Warning"
	case "unknown":
		return "Unknown"
	default:
		return "Unknown"
	}
}

// trustStyleFromLevel maps a Go trust-level string to the palette style key
// Python's AnnounceInfo uses for the trust string (Network.py:103-122).
func trustStyleFromLevel(level string) string {
	switch level {
	case "trusted":
		return "list_trusted"
	case "untrusted":
		return "list_untrusted"
	case "warning":
		return "list_untrusted" // Python's else-branch falls back to list_untrusted
	case "unknown":
		return "list_unknown"
	default:
		return "list_unknown"
	}
}

// ShowKnownNodeInfo opens the editable KnownNodeInfo form for the given saved
// node hash (Python selected_node_info → KnownNodeInfo, Network.py:1697-1710,
// triggered by Ctrl-E on the Saved Nodes list). The directory-backed fields
// resolve at view time via OnResolveKnownNodeInfo; RNS-dependent fields (operator
// string, hop distance, PN address) are stubs until Phase 5. Esc returns to the
// saved-nodes list.
func (nd *NetworkDisplay) ShowKnownNodeInfo(nodeHash string) {
	data := KnownNodeInfoData{
		DisplayStr:  "<" + nodeHash + ">",
		SortStr:     "None",
		TrustLevel:  "unknown",
		OpStr:       "Unknown",
		HopsStr:     "Unknown",
		LXMFAddrStr: "No associated Propagation Node known",
	}
	if nd.OnResolveKnownNodeInfo != nil {
		if resolved, ok := nd.OnResolveKnownNodeInfo(nodeHash); ok {
			data = resolved
		}
	}
	ki := newKnownNodeInfoDisplay(nd, nodeHash, data)
	nd.setLeftList(ki.Widget(), "Node Info")
	nd.setInfoView(true)
	nd.focusLeftList()
}

// showKnownNodes restores the left panel to the saved-nodes list from a
// KnownNodeInfo/AnnounceInfo view (Python show_known_nodes, Network.py:690-693).
func (nd *NetworkDisplay) showKnownNodes() {
	nd.setLeftList(nd.nodesView(), "Saved Nodes")
	nd.setInfoView(false)
	nd.showingPeers = false
	nd.showingNodes = true
	nd.focusLeftList()
}

// connectKnownNode initiates a browser connection to the node then returns to
// the saved-nodes list (Python connect, Network.py:695-699).
func (nd *NetworkDisplay) connectKnownNode(nodeHash string) {
	nd.showKnownNodes()
	if nd.onNavigate != nil {
		nd.onNavigate(nodeHash)
	}
}

// msgOpKnownNode starts a conversation with the node's operator then returns to
// the saved-nodes list (Python msg_op, Network.py:707-731).
func (nd *NetworkDisplay) msgOpKnownNode(_ string) {
	nd.showKnownNodes()
	if nd.OnMsgOp != nil {
		nd.OnMsgOp()
	}
}

// saveKnownNode writes the edited KnownNodeInfo form to the directory then
// returns to the saved-nodes list (Python save_node, Network.py:755-785).
func (nd *NetworkDisplay) saveKnownNode(k *knownNodeInfoDisplay) {
	data := k.FormData()
	nd.showKnownNodes()
	if nd.OnKnownNodeSave != nil {
		nd.OnKnownNodeSave(k.nodeHash, data)
	}
}

// showAnnounceStream restores the left panel to the announce/node list.
func (nd *NetworkDisplay) showAnnounceStream() {
	if !nd.inInfoView {
		return
	}

	// Remove info view from left panel
	if nd.showingNodes {
		nd.setLeftList(nd.nodesView(), "Saved Nodes")
	} else {
		nd.setLeftList(nd.announceStream.Widget(), "Announce Stream")
	}
	nd.setInfoView(false)
	nd.showingPeers = false
	nd.focusLeftList()
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
func (nd *NetworkDisplay) msgOpNode(_ AnnounceEntry) {
	nd.showAnnounceStream()
	if nd.OnMsgOp != nil {
		nd.OnMsgOp()
	}
}

// saveNode saves the node to the directory.
// Matches Python's save_node(sender).
func (nd *NetworkDisplay) saveNode(_ AnnounceEntry) {
	nd.showAnnounceStream()
	if nd.OnSaveNode != nil {
		nd.OnSaveNode()
	}
}

// useAsPN sets the announce's source as the default propagation node.
// Matches Python's use_pn(sender).
func (nd *NetworkDisplay) useAsPN(_ AnnounceEntry) {
	nd.showAnnounceStream()
	if nd.OnUseAsPN != nil {
		nd.OnUseAsPN()
	}
}

// converseWith starts a conversation with a peer.
// Matches Python's converse(sender).
func (nd *NetworkDisplay) converseWith(_ AnnounceEntry) {
	nd.showAnnounceStream()
	if nd.OnConverse != nil {
		nd.OnConverse()
	}
}

// BrowserPane returns the right-pane BrowserPane.
func (nd *NetworkDisplay) BrowserPane() *BrowserPane {
	return nd.browser
}

// BrowserDisplay returns the underlying BrowserDisplay inside the right pane.
func (nd *NetworkDisplay) BrowserDisplay() *BrowserDisplay {
	if nd.browser != nil {
		return nd.browser.BrowserDisplay()
	}
	return nil
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

	nd.announceStream.update()

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
	if nd.showingNodes {
		return AnnounceEntry{}, false
	}
	return nd.announceStream.selectedEntry()
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
	nd.setLeftList(nd.nodesView(), "Saved Nodes")
}

// ShowingNodes reports whether the saved-nodes list is currently displayed
// (vs. the announce stream). Used by wiring to pick the right delete action.
func (nd *NetworkDisplay) ShowingNodes() bool { return nd.showingNodes }

// setLeftList swaps the content and title of the bordered list slot
// (nd.listBox) — the first, weight-1 item of the left pane — while the Local
// Peer Info panel stays packed below it. Mirrors Python's left_pile, where
// contents[0] swaps between KnownNodes/AnnounceStream/AnnounceInfo/KnownNodeInfo
// but the LocalPeer (contents[1]) stays put (Network.py:1641, 1705, 335).
//
// It does NOT establish keyboard focus on the swapped item — that is the
// caller's responsibility via focusLeftList (see below). tview dispatches keys
// to the root primitive, which cascades through the Flex chain to the focused
// descendant; a freshly swapped item has no focused descendant (HasFocus is
// false), so without an explicit SetFocus the cascade has nowhere to deliver
// Enter/arrows. focusLeftList fixes that for UI-thread-driven swaps.
func (nd *NetworkDisplay) setLeftList(item tview.Primitive, title string) {
	nd.listBox.Clear()
	nd.listBox.AddItem(item, 0, 1, true)
	SetTitledBorder(nd.listBox, title)
}

// setInfoView records whether a detail view (AnnounceInfo / KnownNodeInfo) is
// open in the left pane and, with it, toggles the left column's self-managing
// flag on the outer Network Columns. While a detail view is open the left
// column holds a button row that owns Left/Right (Back→Connect→Msg Op→Save),
// so the outer Columns must forward Left/Right to it instead of grabbing Right
// to jump to the browser pane (Python's nested button_columns consumes Left/
// Right; tview can't report consumption, so the self-managing flag marks the
// columns that would). When the detail view closes (back to a plain list),
// the left column is no longer self-managing and Right moves panes again.
func (nd *NetworkDisplay) setInfoView(active bool) {
	nd.inInfoView = active
	if nd.mainCols != nil {
		nd.mainCols.SetSelfManaging(0, active)
	}
}

// focusLeftList establishes keyboard focus on the primitive currently swapped
// into the bordered list slot, so subsequent keys (Enter/arrows/button
// activation) cascade to it via the root InputHandler. Mirrors Python's
// left_pile.focus_position = 0 (the list slot is the focused region; the
// AnnounceStream/KnownNodes list — or the AnnounceInfo button row — receives
// focus). MUST be called on the UI thread (it calls app.SetFocus, which locks
// the application); background-driven swaps (refreshNodesView from the RNS
// init goroutine) must NOT call it — use QueueUpdateDraw instead, or leave
// focus untouched.
func (nd *NetworkDisplay) focusLeftList() {
	if nd.app == nil {
		return
	}
	nd.app.SetFocus(nd.listBox)
}

// FocusLists moves keyboard focus to the left node/announce list, mirroring
// Python's focus_lists (the left_pile gets focus when the browser body
// releases focus via micron_released_focus, MicronParser.py:972-974). It is the
// public entry point wired to BrowserDisplay.OnReleaseFocus from the
// cmd/gonomadnet wiring layer (the right browser pane's release hands focus
// back to the left list).
func (nd *NetworkDisplay) FocusLists() {
	nd.focusLeftList()
}

// UpdateLocalPeer refreshes the read-only Local Peer Info fields with the
// app's real identity data: the LXMF address and identity hash
// (prettyhexrep-formatted "<hex>") and the last-announce time (zero → "Never").
// It does NOT set the name — that is seeded once via SetLocalPeerName when the
// Network display is first wired (mirroring Python's LocalPeer, which never
// re-sets e_name.edit_text after construction, Network.py:1271). Called by the
// wiring layer once the app's RNS identity is ready and on every
// UIChangeCallback refresh.
func (nd *NetworkDisplay) UpdateLocalPeer(lxmfAddr, identityHash string, lastAnnounce time.Time) {
	if nd.localPeer == nil {
		return
	}
	nd.localPeer.SetData(lxmfAddr, identityHash, lastAnnounce)
}

// SetLocalPeerName seeds the name edit field once with the current display
// name. It mirrors Python's LocalPeer.__init__ setting e_name.edit_text at
// construction (Network.py:1271) and never re-setting it; the Go wiring layer
// calls this once when the Network display is first wired, after PeerSettings
// has been loaded synchronously during Init. See LocalPeerDisplay.SetName.
func (nd *NetworkDisplay) SetLocalPeerName(name string) {
	if nd.localPeer == nil {
		return
	}
	nd.localPeer.SetName(name)
}

// SetLocalPeerHandlers wires the Save / Announce Now / Node Info button
// callbacks on the Local Peer Info panel. The wiring layer connects these to
// the app's set_display_name / announce_now / NodeInfo-panel actions.
func (nd *NetworkDisplay) SetLocalPeerHandlers(onSave func(name string), onAnnounce, onNodeInfo func()) {
	if nd.localPeer == nil {
		return
	}
	nd.localPeer.OnSave = onSave
	nd.localPeer.OnAnnounce = onAnnounce
	nd.localPeer.OnNodeInfo = onNodeInfo
}

// ShowNodeInfo swaps the bottom of the left pane from the Local Peer Info panel
// to the Local Node Info panel (Python node_info_query, Network.py:1399-1401),
// building it fresh from the given data. The list slot above is unaffected.
// The NodeInfo panel's Back button swaps back via ShowLocalPeer.
//
// The panel is rebuilt on every call rather than cached, mirroring Python's
// node_info_query which constructs a fresh NodeInfo(self.app, self.parent) each
// time (Network.py:1399-1401). The dynamic stat lines (Last Announce, Storage,
// Active/Total/Pages/Files) are live providers that re-read the app each tick
// regardless, but the static fields (Name, Addr, LXMF propagation address) are
// captured at construction — rebuilding them each open keeps them current if the
// user changes their display name or propagation setting mid-session (the
// previous `if nd.nodeInfo == nil` cache showed the first-open values forever).
// Any previously built panel is stopped (halting its ticker) and removed first
// so no background refresh goroutine leaks.
func (nd *NetworkDisplay) ShowNodeInfo(data NodeInfoData) {
	if nd.nodeInfo != nil {
		nd.nodeInfo.Stop()
		nd.leftPanel.RemoveItem(nd.nodeInfo.Widget())
	}
	nd.nodeInfo = NewNodeInfoDisplay(nd.app, data)
	nd.nodeInfo.OnBack = nd.ShowLocalPeer
	nd.leftPanel.RemoveItem(nd.localPeer.Widget())
	nd.leftPanel.AddItem(nd.nodeInfo.Widget(), nd.nodeInfo.Height(), 0, false)
	// Start the periodic stat refresh while the panel is visible (Python
	// NodeInfo.start, Network.py:1528-1535, runs the UpdatingText timers).
	nd.nodeInfo.Start()
}

// ShowLocalPeer swaps the bottom of the left pane back to the Local Peer Info
// panel (Python show_peer_info, Network.py:1396-1398), removing the NodeInfo
// panel if it was shown.
func (nd *NetworkDisplay) ShowLocalPeer() {
	if nd.nodeInfo != nil {
		nd.leftPanel.RemoveItem(nd.nodeInfo.Widget())
		// Halt the stat refresh while the panel is hidden (Python leaves the
		// timers running, but stopping here is visually identical and avoids a
		// background ticker churning while the NodeInfo panel is offscreen).
		nd.nodeInfo.Stop()
	}
	nd.leftPanel.AddItem(nd.localPeer.Widget(), nd.localPeer.Height(), 0, false)
}

// nodesView returns the left-pane widget for the saved-nodes mode: the
// IndicativeListBox when nodes exist, or the centered empty-state message when
// none are saved (Python KnownNodes empty-state, Network.py:833-882).
func (nd *NetworkDisplay) nodesView() tview.Primitive {
	if nd.nodes.GetItemCount() == 0 {
		return nd.nodeEmptyState
	}
	return nd.nodesList
}

// ToggleFullscreen collapses the left list pane to width 0 so the in-page
// browser fills the page, or restores the saved list width, matching Python's
// NetworkDisplay.toggle_fullscreen (Network.py:1678-1686): when going fullscreen
// the current list width is saved and the pane collapses to width 0; toggling
// again restores it. OnToggleFullscreen fires after the flip so the app layer
// can react (Python keeps the toggle self-contained on the display, so the
// callback is optional). The resize is applied to mainCols column 0 directly.
func (nd *NetworkDisplay) ToggleFullscreen() {
	nd.fullscreen = !nd.fullscreen
	if nd.mainCols != nil {
		if nd.fullscreen {
			nd.mainCols.SetFixedWidth(0, 0)
		} else {
			nd.mainCols.SetFixedWidth(0, nd.listWidth)
		}
	}
	if nd.OnToggleFullscreen != nil {
		nd.OnToggleFullscreen()
	}
}

// Fullscreen reports whether the left list pane is currently collapsed (the
// in-page browser fills the page).
func (nd *NetworkDisplay) Fullscreen() bool { return nd.fullscreen }

// toggleList switches between announces and nodes views.
func (nd *NetworkDisplay) toggleList() {
	if nd.inInfoView {
		nd.showAnnounceStream()
	}

	if nd.showingNodes {
		nd.setLeftList(nd.announceStream.Widget(), "Announce Stream")
		nd.showingNodes = false
	} else {
		nd.setLeftList(nd.nodesView(), "Saved Nodes")
		nd.showingNodes = true
	}
	nd.showingPeers = false
	nd.focusLeftList()
}

// showPeers swaps the left-pane list slot to the LXMF Propagation Peers list
// (Python show_peers, Network.py:1688). Python's show_peers also flips
// list_display so a subsequent ctrl-l (toggle_list) returns to whichever of
// Saved Nodes/Announce Stream was showing before; the flip of showingNodes
// here reproduces that (toggleList shows the opposite of showingNodes).
func (nd *NetworkDisplay) showPeers() {
	if nd.inInfoView {
		nd.setInfoView(false)
	}
	nd.showingNodes = !nd.showingNodes
	nd.showingPeers = true
	nd.setLeftList(nd.lxmfPeers.Widget(), nd.lxmfPeers.Title())
	nd.focusLeftList()
}

// UpdateLXMFPeers repopulates the LXMF Propagation Peers list (Python
// reinit_lxmf_peers, Network.py:1717). Called by the wiring layer in Phase 5
// once the LXMF message router's peer set is known; until then the no-content
// branch renders.
func (nd *NetworkDisplay) UpdateLXMFPeers(peers []LXMFPeerEntry) {
	if nd.lxmfPeers == nil {
		return
	}
	nd.lxmfPeers.SetPeers(peers)
	if nd.showingPeers {
		nd.setLeftList(nd.lxmfPeers.Widget(), nd.lxmfPeers.Title())
	}
}

// ShowingPeers reports whether the LXMF peers list is currently displayed.
func (nd *NetworkDisplay) ShowingPeers() bool { return nd.showingPeers }

// ToggleDisplayMode toggles between showing display names and
// destination hashes in the announce stream.
func (nd *NetworkDisplay) ToggleDisplayMode() {
	nd.displayMode = ToggleDisplayMode(nd.displayMode)
	nd.announceStream.toggle.SetLabel(nd.DisplayModeLabel())
	nd.announceStream.update()
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
	fmt.Fprintf(&sb, "[::b]%v[-]\n", ann.DisplayName)
	fmt.Fprintf(&sb, "Type: %v\n", ann.Type)
	fmt.Fprintf(&sb, "Trust: %v\n", ann.TrustLevel)
	fmt.Fprintf(&sb, "Hash: %v\n", ann.SourceHash)
	fmt.Fprintf(&sb, "Time: %v\n", ann.Timestamp.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&sb, "Data: %v\n", ann.AppData)
	return sb.String()
}

// ShowLocalPeerDialog shows the local peer information panel.
// Matches Python's LocalPeer at Network.py:1259-1350.
func (nd *NetworkDisplay) ShowLocalPeerDialog(lxmfAddr, identityHash, name string, lastAnnounce string) {
	var sb strings.Builder
	fmt.Fprintf(&sb, " LXMF Addr : %v\n", lxmfAddr)
	fmt.Fprintf(&sb, " Identity  : %v\n", identityHash)
	fmt.Fprintf(&sb, " Name      : %v\n", name)
	if lastAnnounce != "" {
		fmt.Fprintf(&sb, " Last Announce: %v\n", lastAnnounce)
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
		text := fmt.Sprintf("%v %v %v", peer.Name, status, truncateStr(peer.Hash, 8))
		secondary := fmt.Sprintf("Pending: %v", peer.Pending)
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
