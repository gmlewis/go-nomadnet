// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; even without the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package tui

import (
	"sync"
	"time"

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

	// Stat-line providers for the hosting branch (Network.py:1059-1256,
	// 1473-1517). Each returns the text shown after its label; nil providers
	// yield the "None"/"Never" fallbacks the Python UpdatingText widgets show
	// before the node reports.
	LastAnnounce  func() string
	StorageStats  func() string
	ActiveLinks   func() string
	TotalConnects func() string
	TotalPages    func() string
	TotalFiles    func() string
	OnBrowse      func()
	OnResetStats  func()
	OnAnnounce    func()
}

// nodeStatLine describes one stat row of the hosting branch: the colon-aligned
// label, the value provider, and the fallback shown when the provider is nil
// (matching Python's "Never"/"None" defaults).
type nodeStatLine struct {
	label    string
	provider func() string
	fallback string
}

// NodeInfoDisplay is the "Local Node Info" panel that replaces the Local Peer
// Info panel in the Network left pane when the user activates the "Node Info"
// button (Python Network.py:1357-1554). It mirrors Python's NodeInfo: a titled
// LineBox ("Local Node Info") wrapping a Pile. When the instance is not hosting
// a node the Pile is a centered info glyph, the "This instance is not hosting a
// node" message, and a centered "Back" button that swaps the panel back to the
// Local Peer Info (show_peer_info, Network.py:1396-1398). When hosting, the Pile
// holds the node addr/name, the LXMF propagation address (when propagation is
// enabled), six live stat lines refreshed every animation interval, and a
// Back/Browse/Rst Stats/Announce button row (Network.py:1473-1517).
type NodeInfoDisplay struct {
	app     *App
	widget  *tview.Flex // bordered "Local Node Info"
	backBtn *UrwidButton
	OnBack  func()
	data    NodeInfoData

	// Hosting-branch state.
	statLines   []nodeStatLine
	statViews   []*tview.TextView
	browseBtn   *UrwidButton
	resetBtn    *UrwidButton
	announceBtn *UrwidButton

	mu      sync.Mutex
	started bool
	stopCh  chan struct{}
	wg      sync.WaitGroup
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
		ni.buildNotHosting()
		return ni
	}
	ni.buildHosting()
	// Initial population from the providers (Python's UpdatingText.__init__
	// calls update() once).
	ni.refreshStats()
	return ni
}

// buildNotHosting builds the not-hosting branch (Network.py:1543-1551): a
// centered info glyph, the "This instance is not hosting a node" message, and a
// centered Back button.
func (ni *NodeInfoDisplay) buildNotHosting() {
	glyph := "-"
	if ni.app != nil && ni.app.Glyphs != nil {
		glyph = ni.app.Glyphs["info"]
	}

	infoText := newCenteredText(tcell.ColorDefault, "", glyph)
	msgText := newCenteredText(tcell.ColorDefault,
		"", "This instance is not hosting a node", "", "")
	btnRow := newCenteredButtonRow(ni.backBtn, urwidButtonNaturalWidth("Back"))

	ni.widget = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(infoText, 2, 0, false).
		AddItem(msgText, 4, 0, false).
		AddItem(btnRow, 1, 0, true)
	ni.widget.SetBorder(true)
	SetTitledBorder(ni.widget, "Local Node Info")
}

// buildHosting builds the hosting branch (Network.py:1473-1517): Addr, Name,
// divider, the centered LXMF propagation address (when propagation enabled),
// divider, the six stat lines, divider, and the Back/Browse/Rst Stats/Announce
// button row.
func (ni *NodeInfoDisplay) buildHosting() {
	g := dividerGlyph(ni.app)

	addrText := tview.NewTextView().SetDynamicColors(false).SetText("Addr : " + ni.data.Addr)
	nameText := tview.NewTextView().SetDynamicColors(false).SetText("Name : " + ni.data.Name)

	// Stat lines (Network.py:1479-1484,1501-1506) in pile order: Last Announce,
	// LXMF Storage, Connected Now, Total Connects, Served Pages, Served Files.
	ni.statLines = []nodeStatLine{
		{"Last Announce  : ", ni.data.LastAnnounce, "Never"},
		{"LXMF Storage   : ", ni.data.StorageStats, "None"},
		{"Connected Now  : ", ni.data.ActiveLinks, "None"},
		{"Total Connects : ", ni.data.TotalConnects, "None"},
		{"Served Pages   : ", ni.data.TotalPages, "None"},
		{"Served Files   : ", ni.data.TotalFiles, "None"},
	}
	ni.statViews = make([]*tview.TextView, len(ni.statLines))

	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(addrText, 1, 0, false).
		AddItem(nameText, 1, 0, false).
		AddItem(newDividerRow(g), 1, 0, false)

	if !ni.data.DisablePropagation {
		// Centered LXMF propagation address (Network.py:1465-1466,1477).
		lxmf := tview.NewTextView().
			SetDynamicColors(false).
			SetTextAlign(tview.AlignCenter).
			SetText("LXMF Propagation Node Address is " + ni.data.LXMFPropAddr)
		layout.AddItem(lxmf, 1, 0, false).
			AddItem(newDividerRow(g), 1, 0, false)
	}

	// The stat lines refresh in place; each is its own 1-row TextView so
	// refreshStats can update a single line without rebuilding the layout.
	for i := range ni.statLines {
		tv := tview.NewTextView().SetDynamicColors(false)
		ni.statViews[i] = tv
		layout.AddItem(tv, 1, 0, false)
	}

	layout.AddItem(newDividerRow(g), 1, 0, false).
		AddItem(ni.hostingButtonRow(), 1, 0, true)

	ni.widget = layout
	ni.widget.SetBorder(true)
	SetTitledBorder(ni.widget, "Local Node Info")
}

// hostingButtonRow builds the weighted Back/Browse/Rst Stats/Announce columns
// (Network.py:1486-1494): weights 5/0.5/6/0.5/8/0.5/7 scaled by 2 to
// [10,1,12,1,16,1,14].
func (ni *NodeInfoDisplay) hostingButtonRow() *urwidColumns {
	ni.browseBtn = NewUrwidButton("Browse").SetSelectedFunc(func() {
		if ni.data.OnBrowse != nil {
			ni.data.OnBrowse()
		}
	})
	ni.resetBtn = NewUrwidButton("Rst Stats").SetSelectedFunc(func() {
		if ni.data.OnResetStats != nil {
			ni.data.OnResetStats()
		}
	})
	ni.announceBtn = NewUrwidButton("Announce").SetSelectedFunc(func() {
		if ni.data.OnAnnounce != nil {
			ni.data.OnAnnounce()
		}
	})
	row := newURWIDColumns(0,
		ni.backBtn,
		tview.NewBox(),
		ni.browseBtn,
		tview.NewBox(),
		ni.resetBtn,
		tview.NewBox(),
		ni.announceBtn,
	).
		SetWeight(0, 10).SetWeight(1, 1).SetWeight(2, 12).
		SetWeight(3, 1).SetWeight(4, 16).SetWeight(5, 1).SetWeight(6, 14)
	return row
}

// actionButton returns the hosting-branch button with the given label, or nil
// (used by tests to fire button callbacks).
func (ni *NodeInfoDisplay) actionButton(label string) *UrwidButton {
	switch label {
	case "Back":
		return ni.backBtn
	case "Browse":
		return ni.browseBtn
	case "Rst Stats":
		return ni.resetBtn
	case "Announce":
		return ni.announceBtn
	}
	return nil
}

// refreshStats re-reads every stat-line provider and updates the displayed
// text, mirroring one pass of Python's UpdatingText.update_callback
// (Network.py:1552-1554). Nil providers fall back to "Never"/"None".
func (ni *NodeInfoDisplay) refreshStats() {
	for i, line := range ni.statLines {
		value := line.fallback
		if line.provider != nil {
			value = line.provider()
		}
		ni.statViews[i].SetText(line.label + value)
	}
}

// Start begins the periodic stat refresh (Python NodeInfo.start,
// Network.py:1529-1535, animation_interval per widget), marshaling each refresh
// onto the tview event loop via QueueUpdateDraw (production). It is idempotent
// and a no-op when no node is hosted.
func (ni *NodeInfoDisplay) Start() { ni.start(true) }

// start launches the periodic stat refresh. When marshal is true each refresh
// is queued onto the application event loop via QueueUpdateDraw (production);
// when false it runs synchronously (tests, where no event loop is running and
// QueueUpdateDraw would block forever on an undrained channel — same pattern as
// NetworkStats.start). Idempotent.
func (ni *NodeInfoDisplay) start(marshal bool) {
	ni.mu.Lock()
	if !ni.data.HasNode || ni.started {
		ni.mu.Unlock()
		return
	}
	ni.stopCh = make(chan struct{})
	stop := ni.stopCh
	ni.started = true
	ni.mu.Unlock()

	ni.wg.Add(1)
	go func() {
		defer ni.wg.Done()
		ticker := time.NewTicker(animationInterval())
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				if marshal && ni.app != nil {
					ni.app.QueueUpdateDraw(ni.refreshStats)
				} else {
					ni.refreshStats()
				}
			}
		}
	}()
}

// Stop halts the periodic stat refresh. It is idempotent and safe to call when
// no ticker is running.
func (ni *NodeInfoDisplay) Stop() {
	ni.mu.Lock()
	ni.started = false
	ch := ni.stopCh
	ni.stopCh = nil
	ni.mu.Unlock()
	if ch != nil {
		close(ch)
		ni.wg.Wait()
	}
}

// animationInterval returns the stat-refresh interval (Python
// config["textui"]["animation_interval"]). Until the Go config exposes it,
// default to 1 second (the Phase 4 NodeInfo task spec: "1 s refresh").
func animationInterval() time.Duration {
	// TODO: read from app config when exposed; 1s matches the task spec and
	// Python's default animation_interval.
	return time.Second
}

// Widget returns the bordered "Local Node Info" primitive for layout.
func (ni *NodeInfoDisplay) Widget() *tview.Flex { return ni.widget }

// Height returns the rendered height of the panel: the not-hosting branch is
// 7 content rows + 2 border rows = 9. The hosting branch is 11 content rows +
// 2 border = 13 with propagation disabled, or 13 content + 2 border = 15 with
// the LXMF propagation address line (Network.py:1473-1517).
func (ni *NodeInfoDisplay) Height() int {
	if !ni.data.HasNode {
		return 9
	}
	// 2 header (Addr, Name) + 1 divider + [1 LXMF + 1 divider] + 6 stats +
	// 1 divider + 1 buttons = 11 base + 2 when propagation enabled.
	h := 2 + 1 + 6 + 1 + 1
	if !ni.data.DisablePropagation {
		h += 2
	}
	return h + 2 // +2 border rows
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
