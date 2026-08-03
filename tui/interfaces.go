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
	"github.com/gmlewis/go-nomadnet/nomadnet/asciichart"
	"github.com/rivo/tview"
)

// InterfaceInfo holds status information for a network interface.
type InterfaceInfo struct {
	Name   string
	Type   string
	Status string // "connected" or "disconnected" — legacy field used by
	// interface-store and the text formatters; the SelectableInterfaceItem list
	// driven by the live transport uses Connected/Enabled/TX/RX instead.
	Connected bool // live link online (RNS Interface.Status)
	Enabled   bool // interface running / not detached
	Target    string
	Bitrate   int
	TX        int64 // cumulative bytes sent
	RX        int64 // cumulative bytes received
	Bandwidth float64
	Traffic   []float64 // recent traffic samples for chart
}

// InterfacesDisplay shows RNS interface status as a list of selectable,
// rounded bordered interface boxes (Python SelectableInterfaceItem), plus
// add/edit/show/remove and config-editor actions.
type InterfacesDisplay struct {
	app      *App
	widget   tview.Primitive
	layout   *tview.Flex
	listBox  *interfaceListBox
	items    []InterfaceInfo
	glyphset string

	// Keyboard shortcut callbacks (Python: InterfaceFiller.keypress)
	OnAddInterface    func()
	OnEditInterface   func()
	OnRemoveInterface func()
	OnConfigEditor    func()
	OnShowInterface   func(idx int)
}

// NewInterfacesDisplay creates a new interfaces display.
func NewInterfacesDisplay(app *App, interfaces []InterfaceInfo) *InterfacesDisplay {
	id := &InterfacesDisplay{app: app}
	if app != nil {
		id.glyphset = app.GlyphSet
	}

	// Centered "Interfaces" header (Python interface_header = urwid.Text(
	// ("interface_title", "Interfaces"), align=CENTER), Interfaces.py:2917).
	// interface_title is the default style (no bold, no fg color) in the dark
	// palette, so ColorDefault (terminal default) and no bold. urwid
	// Text(align=CENTER) is CEIL-left (extra col to the left on odd slack);
	// tview.AlignCenter floors left, so use centeredText for parity. A 2-row
	// item renders the title on row 0 and a blank on row 1, matching Python's
	// header + urwid.Divider().
	title := newCenteredText(tcell.ColorDefault, "Interfaces", "")

	id.listBox = newInterfaceListBox(id.glyphset)
	id.SetInterfaces(interfaces)

	// No outer border (Python: pile = urwid.Pile([box_adapter]) wrapped in
	// InterfaceFiller = urwid.Filler(TOP); the interface boxes are laid out
	// directly in the body, each its own rounded LineBox, Interfaces.py:2932).
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(title, 2, 0, false).
		AddItem(id.listBox, 0, 1, true)
	layout.SetInputCapture(id.handleInput)

	id.layout = layout
	id.widget = layout
	return id
}

// SetInterfaces replaces the interface list, rebuilding the selectable items.
func (id *InterfacesDisplay) SetInterfaces(interfaces []InterfaceInfo) {
	id.items = interfaces
	items := make([]*SelectableInterfaceItem, 0, len(interfaces))
	for _, iface := range interfaces {
		icon := GetInterfaceIcon(id.glyphset, iface.Type)
		items = append(items, NewSelectableInterfaceItem(iface.Name, iface.Type, iface.Connected, iface.Enabled, iface.TX, iface.RX, icon))
	}
	id.listBox.SetItems(items)
}

// SelectedIndex returns the focused interface index, or -1 if none.
func (id *InterfacesDisplay) SelectedIndex() int {
	return id.listBox.focusIdx
}

// Items returns the current interface info slice backing the list (the live
// transport snapshot most recently passed to SetInterfaces). Callers that need
// to reflect a freshly-refreshed list (e.g. the Show-Interface handler) read
// this instead of a captured local so they see the current data.
func (id *InterfacesDisplay) Items() []InterfaceInfo {
	return id.items
}

// handleInput processes keyboard shortcuts for the interfaces display.
// Matches Python's InterfaceFiller.keypress() at Interfaces.py:1391. Up/Down
// and Enter are forwarded to the selectable list (Python SelectableInterfaceItem
// keypress: up/down move focus, enter shows the interface).
func (id *InterfacesDisplay) handleInput(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyCtrlA:
		if id.OnAddInterface != nil {
			id.OnAddInterface()
		}
		return nil
	case tcell.KeyCtrlE:
		if id.OnEditInterface != nil {
			id.OnEditInterface()
		}
		return nil
	case tcell.KeyCtrlX:
		if id.OnRemoveInterface != nil {
			id.OnRemoveInterface()
		}
		return nil
	case tcell.KeyCtrlW:
		if id.OnConfigEditor != nil {
			id.OnConfigEditor()
		}
		return nil
	case tcell.KeyEnter:
		if id.OnShowInterface != nil && id.listBox.focusIdx >= 0 {
			id.OnShowInterface(id.listBox.focusIdx)
		}
		return nil
	case tcell.KeyUp, tcell.KeyDown, tcell.KeyPgUp, tcell.KeyPgDn, tcell.KeyHome, tcell.KeyEnd:
		id.listBox.HandleKey(event.Key())
		return nil
	}

	return event
}

// Widget returns the tview primitive for this display.
func (id *InterfacesDisplay) Widget() tview.Primitive {
	return id.widget
}

// interfaceListBox is a vertical, scrollable list of SelectableInterfaceItem
// boxes with focus cycling, mirroring Python's IndicativeListBox of
// SelectableInterfaceItem entries (Interfaces.py:1945/2886).
type interfaceListBox struct {
	*tview.Box
	items      []*SelectableInterfaceItem
	focusIdx   int
	offset     int // index of the first visible item
	glyphset   string
	onActivate func(idx int)
}

func newInterfaceListBox(glyphset string) *interfaceListBox {
	b := &interfaceListBox{Box: tview.NewBox(), glyphset: glyphset, focusIdx: -1}
	return b
}

// SetItems replaces the list contents. Focus is preserved when it points at a
// still-valid item; otherwise it stays at -1 (no item focused). This matches
// Python's interface list, whose ListBox focus defaults to the non-selectable
// header (Interfaces.py:2905-2910 — [interface_header, Divider] lead the
// walker), so NO interface item shows the ● selection glyph until the user
// presses Down to move focus onto the first item.
func (b *interfaceListBox) SetItems(items []*SelectableInterfaceItem) {
	b.items = items
	switch {
	case len(items) == 0:
		b.focusIdx = -1
	case b.focusIdx < 0 || b.focusIdx >= len(items):
		// No valid focus yet (initial set or the focused item vanished in a
		// refresh) — stay unfocused rather than silently focusing item 0.
		b.focusIdx = -1
	}
	b.offset = 0
	b.applyFocus()
}

func (b *interfaceListBox) applyFocus() {
	for i, it := range b.items {
		it.SetFocused(i == b.focusIdx)
	}
}

// HandleKey moves focus for navigation keys.
func (b *interfaceListBox) HandleKey(key tcell.Key) {
	if len(b.items) == 0 {
		return
	}
	switch key {
	case tcell.KeyUp:
		if b.focusIdx > 0 {
			b.focusIdx--
		}
	case tcell.KeyDown:
		if b.focusIdx < 0 {
			b.focusIdx = 0
		} else if b.focusIdx < len(b.items)-1 {
			b.focusIdx++
		}
	case tcell.KeyPgUp:
		visible := b.visibleCount()
		if b.focusIdx < 0 {
			b.focusIdx = 0
		}
		b.focusIdx -= visible
		if b.focusIdx < 0 {
			b.focusIdx = 0
		}
	case tcell.KeyPgDn:
		visible := b.visibleCount()
		if b.focusIdx < 0 {
			b.focusIdx = 0
		}
		b.focusIdx += visible
		if b.focusIdx > len(b.items)-1 {
			b.focusIdx = len(b.items) - 1
		}
	case tcell.KeyHome:
		b.focusIdx = 0
	case tcell.KeyEnd:
		b.focusIdx = len(b.items) - 1
	}
	b.applyFocus()
	b.scrollIntoView()
}

func (b *interfaceListBox) visibleCount() int {
	_, _, _, h := b.GetRect()
	if h <= 0 {
		return 1
	}
	return h / InterfaceItemHeight
}

func (b *interfaceListBox) scrollIntoView() {
	visible := b.visibleCount()
	if visible < 1 {
		visible = 1
	}
	if b.focusIdx < b.offset {
		b.offset = b.focusIdx
	}
	if b.focusIdx >= b.offset+visible {
		b.offset = b.focusIdx - visible + 1
	}
	if b.offset > len(b.items)-visible && len(b.items) >= visible {
		b.offset = len(b.items) - visible
	}
	if b.offset < 0 {
		b.offset = 0
	}
}

// Draw renders the visible interface boxes.
func (b *interfaceListBox) Draw(screen tcell.Screen) {
	b.Box.DrawForSubclass(screen, b)
	x, y, w, h := b.GetRect()
	if w < 4 || h < InterfaceItemHeight || len(b.items) == 0 {
		if len(b.items) == 0 {
			tview.Print(screen, "No interfaces configured", x, y, w, tview.AlignCenter, tcell.ColorYellow)
		}
		return
	}
	b.scrollIntoView()
	rowY := y
	for i := b.offset; i < len(b.items); i++ {
		if rowY >= y+h {
			break
		}
		itemH := InterfaceItemHeight
		if itemH > y+h-rowY {
			// Bottom-clipped last item: urwid's ListBox renders the visible top
			// portion of a partially-fit item rather than skipping it entirely.
			itemH = y + h - rowY
		}
		it := b.items[i]
		it.SetRect(x, rowY, w, itemH)
		it.Draw(screen)
		rowY += itemH
		if itemH < InterfaceItemHeight {
			break // the clipped last item consumed the remaining height
		}
	}
}

// InputHandler delegates navigation keys to HandleKey.
func (b *interfaceListBox) InputHandler() func(event *tcell.EventKey, setFocus func(tview.Primitive)) {
	return b.WrapInputHandler(func(event *tcell.EventKey, setFocus func(tview.Primitive)) {
		switch event.Key() {
		case tcell.KeyUp, tcell.KeyDown, tcell.KeyPgUp, tcell.KeyPgDn, tcell.KeyHome, tcell.KeyEnd:
			b.HandleKey(event.Key())
		case tcell.KeyEnter:
			if b.onActivate != nil && b.focusIdx >= 0 {
				b.onActivate(b.focusIdx)
			}
		}
	})
}

// formatInterfaces formats the interface list as text.
func formatInterfaces(interfaces []InterfaceInfo) string {
	if len(interfaces) == 0 {
		return "\n[yellow]No interfaces configured[-]\n\nAdd interfaces to your Reticulum config."
	}

	var sb strings.Builder
	chart := asciichart.New("unicode")

	for _, iface := range interfaces {
		statusColor := "[green]"
		if iface.Status != "connected" {
			statusColor = "[red]"
		}

		sb.WriteString(fmt.Sprintf("[::b]%v[-] %v(%v)[-]\n", iface.Name, statusColor, iface.Status))
		sb.WriteString(fmt.Sprintf("  Type: %v  Target: %v\n", iface.Type, iface.Target))
		sb.WriteString(fmt.Sprintf("  Bandwidth: %v\n", formatBandwidth(iface.Bandwidth)))

		if len(iface.Traffic) > 0 {
			sb.WriteString("\n")
			chartView := chart.PlotSingle(iface.Traffic)
			for _, line := range strings.Split(chartView, "\n") {
				sb.WriteString("  ")
				sb.WriteString(line)
				sb.WriteString("\n")
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// formatBandwidth formats a bandwidth value in bytes/sec.
func formatBandwidth(bps float64) string {
	units := []string{"B/s", "KB/s", "MB/s", "GB/s"}
	size := bps
	unitIdx := 0

	for size >= 1024.0 && unitIdx < len(units)-1 {
		size /= 1024.0
		unitIdx++
	}

	if unitIdx == 0 {
		return fmt.Sprintf("%.0f %v", size, units[unitIdx])
	}
	return fmt.Sprintf("%.1f %v", size, units[unitIdx])
}

// FormatInterfaceEntry produces a single-line summary for the interface
// list widget. Includes icon, name, and status indicator.
// Matches Python's InterfaceListEntry widget.
func FormatInterfaceEntry(iface InterfaceInfo) string {
	icon := InterfaceIcon(iface.Type)
	statusColor := "[green]"
	if iface.Status != "connected" {
		statusColor = "[red]"
	}
	return fmt.Sprintf("%v %v %v(%v)[-]", icon, iface.Name, statusColor, iface.Status)
}

// FormatInterfaceDetail produces a multi-line detail view for the
// interface info panel. Shows name, type, status, target, bandwidth,
// and recent traffic data when available.
// Matches Python's InterfaceDetail view layout.
func FormatInterfaceDetail(iface InterfaceInfo) string {
	var sb strings.Builder

	sb.WriteString("[::b]Interface Details[-]\n\n")
	sb.WriteString(fmt.Sprintf("  Name: %v\n", iface.Name))
	sb.WriteString(fmt.Sprintf("  Type: %v\n", iface.Type))

	statusIcon := "○"
	if iface.Status == "connected" {
		statusIcon = "●"
	}
	sb.WriteString(fmt.Sprintf("  Status: %v %v\n", statusIcon, iface.Status))

	if iface.Target != "" {
		sb.WriteString(fmt.Sprintf("  Target: %v\n", iface.Target))
	}

	sb.WriteString(fmt.Sprintf("  Bandwidth: %v\n", formatBandwidth(iface.Bandwidth)))

	if len(iface.Traffic) > 0 {
		sb.WriteString("\n  [gray]Recent Traffic[-]\n")
		sb.WriteString(fmt.Sprintf("  Samples: %v\n", len(iface.Traffic)))
	}

	return sb.String()
}

// ShowEnableDisableConfirm shows a confirmation dialog for enabling or
// disabling an interface. Matches Python's Interfaces.py:2570-2590.
func (id *InterfacesDisplay) ShowEnableDisableConfirm(name string, enabled bool, onConfirm func()) {
	action := "Enable"
	if enabled {
		action = "Disable"
	}
	msg := fmt.Sprintf("%v interface %v?", action, name)

	id.app.Dialogs.ShowConfirmDialog(msg, func() {
		if onConfirm != nil {
			onConfirm()
		}
	}, nil)
}

// ShowRestartRequired shows a notice that a restart is required after
// interface changes. Matches Python's Interfaces.py:2589-2610.
func (id *InterfacesDisplay) ShowRestartRequired() {
	msg := "RNS must be restarted for interface changes to take effect.\nRestart Nomad Network to apply changes."

	id.app.Dialogs.ShowDialog("Restart Required",
		tview.NewTextView().
			SetDynamicColors(true).
			SetTextColor(tcell.NewHexColor(0xdddddd)).
			SetTextAlign(tview.AlignCenter).
			SetText(msg),
		50, 5, nil)
}

// ShowInterfaceError shows an error message for interface operations.
// Matches Python's Interfaces.py:2619-2650.
func (id *InterfacesDisplay) ShowInterfaceError(errMsg string) {
	msg := fmt.Sprintf("[red]Error:[-] %v", errMsg)

	id.app.Dialogs.ShowDialog("Interface Error",
		tview.NewTextView().
			SetDynamicColors(true).
			SetTextColor(tcell.NewHexColor(0xdddddd)).
			SetTextAlign(tview.AlignCenter).
			SetText(msg),
		50, 5, nil)
}

// ShowRNSDisconnected shows an overlay when the RNS transport is lost.
// Matches Python's Interfaces.py:2794-2830.
func (id *InterfacesDisplay) ShowRNSDisconnected() {
	msg := "[red]RNS Instance Disconnected[-]\n\nThe RNS transport connection has been lost.\nCheck your network configuration and restart if necessary."

	id.app.Dialogs.ShowDialog("Disconnected",
		tview.NewTextView().
			SetDynamicColors(true).
			SetTextColor(tcell.NewHexColor(0xdddddd)).
			SetTextAlign(tview.AlignCenter).
			SetText(msg),
		50, 8, nil)
}
