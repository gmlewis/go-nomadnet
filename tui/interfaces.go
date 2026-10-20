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
	"os/exec"
	"strings"

	"github.com/gmlewis/go-nomadnet/nomadnet/app"
	"github.com/gmlewis/go-nomadnet/nomadnet/asciichart"
	"github.com/gmlewis/tcell/v2"
	"github.com/gmlewis/tview"
)

// InterfaceInfoFromStat converts one live interface statistic into the shape the
// Interfaces page renders. The interface's own configuration parameters come
// along: the detail view's Connection/Radio/Network/IFAC/Additional blocks are
// built from them, so dropping them leaves every interface reading "No
// additional parameters" (Python's page reads app.rns.config['interfaces']
// directly and so always has them).
func InterfaceInfoFromStat(s app.InterfaceStat) InterfaceInfo {
	return InterfaceInfo{
		Name:      s.Name,
		Type:      s.Type,
		Enabled:   s.Enabled,
		Connected: s.Connected,
		TX:        s.TX,
		RX:        s.RX,
		Bitrate:   s.Bitrate,
		Params:    s.Params,
	}
}

// InterfaceInfo holds status information for a network interface.
type InterfaceInfo struct {
	Name   string
	Type   string
	Status string // "connected" or "disconnected"; read by interface-store and the
	// text formatters. The SelectableInterfaceItem list driven by the live
	// transport uses Connected/Enabled/TX/RX instead.
	Connected bool // live link online (RNS Interface.Status)
	Enabled   bool // interface running / not detached
	Target    string
	Bitrate   int
	TX        int64 // cumulative bytes sent
	RX        int64 // cumulative bytes received
	Bandwidth float64
	Traffic   []float64 // recent traffic samples for chart
	// Params holds the interface's raw config parameters for the ShowInterface
	// detail view's Connection/Radio/Network/IFAC/Additional blocks
	// (Python Interfaces.py:2436-2492). nil when unavailable.
	Params map[string]string
}

// InterfacesDisplay shows RNS interface status as a list of selectable,
// rounded bordered interface boxes (Python SelectableInterfaceItem), plus
// add/edit/show/remove and config-editor actions.
type InterfacesDisplay struct {
	app     *App
	widget  tview.Primitive
	layout  *tview.Flex
	listBox *interfaceListBox
	// title is the centered header showing "Interfaces (N)" and the empty-state
	// message (Python Interfaces.py:3146-3149). It is refreshed with the item
	// list.
	title    *centeredText
	items    []InterfaceInfo
	glyphset string
	// pages wraps id.layout so a dialog can overlay the WHOLE interfaces display
	// (Python's frame.body = urwid.Overlay(dialog, frame.body, width=50,
	// height=7), Interfaces.py:2578-2590/2608-2620/2633-2645). "main" = id.layout;
	// "dialog" = the SlotOverlay. dialogOverlay tracks the active overlay.
	pages         *tview.Pages
	dialogOverlay *SlotOverlay

	// showWidget is the full-page ShowInterface detail view mounted as
	// the "show" page over the interface list (Python self.widget =
	// show_interface, Interfaces.py:3004-3007).
	showWidget *interfaceShow

	// pickerOverlay is the Add-Interface type picker mounted as the "picker"
	// page (Python's add_interface overlay, Interfaces.py:3060-3073) and
	// formOverlay is the Add/Edit form mounted as the "form" page (Python's
	// AddInterfaceView overlay, Interfaces.py:1464-1472).
	pickerOverlay *SlotOverlay
	formOverlay   *SlotOverlay
	formView      *InterfaceFormView

	// OnToggleInterface fires the Disable/Enable action from the ShowInterface
	// footer (Python on_toggle_enabled, Interfaces.py:2516). The wiring layer
	// owns the config write + confirm flow.
	OnToggleInterface func(info InterfaceInfo)

	// editor is the embedded terminal running an external editor (Ctrl-W /
	// "Open Text Editor"), mounted as the "editor" page over the interface
	// list while the menu bar + footer stay visible (Python's open_config_editor
	// swaps self.widget for a LineBox(urwid.Terminal); Interfaces.py:3160).
	editor *EmbeddedTerminal

	// Keyboard shortcut callbacks (Python: InterfaceFiller.keypress)
	OnAddInterface    func()
	OnEditInterface   func()
	OnRemoveInterface func()
	OnConfigEditor    func()
	OnShowInterface   func(idx int)

	// OnReleaseFocus is called when the user navigates Up past the top interface
	// item, so focus returns to the menu bar (mirroring the browser pane's
	// OnReleaseFocus → FocusMenu). Without it, UpArrow at the top item is a
	// no-op that traps focus in the list.
	OnReleaseFocus func()
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
	title := newCenteredText(tcell.ColorDefault, interfaceHeader(len(interfaces)), "")
	id.title = title

	id.listBox = newInterfaceListBox(id.app, id.glyphset)
	// Row activation (Python SelectableInterfaceItem.keypress "enter" →
	// parent.switch_to_show_interface(name), Interfaces.py:1246-1249): the
	// layout input capture owns Enter in the tview dispatch, and the listbox's
	// own InputHandler forwards the same activation to OnShowInterface when it
	// owns the event.
	id.listBox.onActivate = func(idx int) {
		if id.OnShowInterface != nil && idx >= 0 && idx < len(id.items) {
			id.OnShowInterface(idx)
		}
	}
	id.SetInterfaces(interfaces)

	// No outer border (Python: pile = urwid.Pile([box_adapter]) wrapped in
	// InterfaceFiller = urwid.Filler(TOP); the interface boxes are laid out
	// directly in the body, each its own rounded LineBox, Interfaces.py:2932).
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(title, 2, 0, false).
		AddItem(id.listBox, 0, 1, true)
	layout.SetInputCapture(id.handleInput)

	id.layout = layout
	id.pages = tview.NewPages().AddPage("main", layout, true, true)
	id.widget = id.pages
	return id
}

// showDialogOverlay overlays a DialogLineBox on the whole interfaces display
// (Python's frame.body = urwid.Overlay(dialog, frame.body, align=CENTER,
// width=N, valign=MIDDLE, height=N), Interfaces.py:2578/2608/2633/2801). The
// display shows through around the fixed-width, fixed-height dialog. Esc/OK
// dismisses via closeDialog, restoring the display.
func (id *InterfacesDisplay) showDialogOverlay(dialog *DialogLineBox, fixedWidth, dialogHeight int) {
	if id.dialogOverlay != nil {
		id.closeDialog()
	}
	ov := NewSlotOverlayFixed(id.layout, dialog, fixedWidth, dialogHeight)
	dialog.onDismiss = id.closeDialog
	id.dialogOverlay = ov
	id.pages.AddPage("dialog", ov, true, true)
	id.pages.SwitchToPage("dialog")
	if id.app != nil {
		id.app.SetFocus(ov)
	}
}

// closeDialog restores the interfaces display after a showDialogOverlay.
func (id *InterfacesDisplay) closeDialog() {
	if id.dialogOverlay == nil {
		return
	}
	id.pages.RemovePage("dialog")
	id.dialogOverlay = nil
	id.pages.SwitchToPage("main")
	if id.app != nil {
		id.app.SetFocus(id.layout)
	}
}

// ShowEditor embeds an external editor (editorCmd on filePath) inside the
// interfaces display as a full-body page titled "Editing RNS Config", matching
// Python's open_config_editor (Interfaces.py:3160): the editor replaces the
// interface list while the menu bar + footer stay visible. On editor exit the
// page is removed and focus returns to the interface list. TERM is "xterm" so
// line editors emit ANSI the embedded emulator supports.
func (id *InterfacesDisplay) ShowEditor(editorCmd, filePath string) {
	if id.editor != nil {
		id.closeEditor()
	}
	_, _, w, h := id.layout.GetRect()
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 24
	}
	// A host that cannot start a program is asked first: its answer is the reason the
	// editor did not open, which is more use than an exec failure would be.
	if err := checkEditor(editorCmd); err != nil {
		id.ShowInterfaceError(err.Error())
		return
	}
	cmd := exec.Command(editorCmd, filePath)
	et, err := NewEmbeddedTerminal(id.app, cmd, w, h, "linux", id.closeEditor)
	if err != nil {
		id.ShowInterfaceError(err.Error())
		return
	}
	et.SetBorder(true)
	// urwid.LineBox wraps the title in a leading + trailing space
	// (format_title: " "+title+" "); match it so the title reads identically.
	et.SetTitle(" Editing RNS Config ")
	id.editor = et
	id.pages.AddPage("editor", et, true, true)
	id.pages.SwitchToPage("editor")
	if id.app != nil {
		id.app.SetFocus(et)
	}
}

// closeEditor removes the embedded editor page and restores the interface list
// + focus. It is the onClose callback fired when the editor child exits
// (matching Python's urwid.Terminal 'closed' signal → restore self.widget).
func (id *InterfacesDisplay) closeEditor() {
	if id.editor == nil {
		return
	}
	id.editor.Close()
	id.editor = nil
	id.pages.RemovePage("editor")
	id.pages.SwitchToPage("main")
	if id.app != nil {
		id.app.SetFocus(id.layout)
	}
}

// ShowInterfaceDetail mounts the full-page ShowInterface detail view
// (Python switch_to_show_interface, Interfaces.py:3004-3007) for the given
// interface, replacing the list. Back (button/Esc) returns to the list
// (Python switch_to_list, Interfaces.py:3011-3016).
func (id *InterfacesDisplay) ShowInterfaceDetail(info InterfaceInfo) {
	if id.showWidget != nil {
		id.closeShowInterface()
	}
	show := NewInterfaceShow(id.app, info)
	show.OnBack = func() { id.closeShowInterface() }
	show.OnEdit = func() {
		id.closeShowInterface()
		if id.OnEditInterface != nil {
			id.OnEditInterface()
		}
	}
	show.OnToggle = func() {
		if id.OnToggleInterface != nil {
			id.OnToggleInterface(info)
		}
	}
	id.showWidget = show
	id.pages.AddPage("show", show.Widget(), true, true)
	id.pages.SwitchToPage("show")
	if id.app != nil {
		id.app.SetFocus(show.body)
	}
	id.refreshShortcuts()
}

// closeShowInterface restores the interface list after the detail view
// (Python switch_to_list, Interfaces.py:3011-3016).
func (id *InterfacesDisplay) closeShowInterface() {
	if id.showWidget == nil {
		return
	}
	id.showWidget = nil
	id.pages.RemovePage("show")
	id.pages.SwitchToPage("main")
	if id.app != nil {
		id.app.SetFocus(id.layout)
	}
	id.refreshShortcuts()
}

// ShowTypePicker overlays Python's "Select Interface Type" dialog
// (Interfaces.py:3017-3073) over the interface list and calls onSelect with the
// chosen interface type. The dialog is 50% of the slot's width and height,
// floored at 20x15 with a 2-column inset (Python's
// urwid.Overlay(width=('relative',50), height=('relative',50), min_width=20,
// min_height=15, left=2, right=2) over self.interfaces_display, so the list
// shows through around it).
func (id *InterfacesDisplay) ShowTypePicker(onSelect func(ifType string)) {
	id.CloseTypePicker()

	picker := NewInterfaceTypePicker(id.app, id.glyphset)
	picker.OnSelect = func(ifType string) {
		if onSelect != nil {
			onSelect(ifType)
		}
	}

	dialog := NewDialogLineBox("Select Interface Type", picker, id.CloseTypePicker)
	ov := NewSlotOverlay(id.layout, dialog, 50, 0)
	ov.SetHeightPct(50).SetMinWidth(20).SetMinHeight(15)
	id.pickerOverlay = ov
	id.pages.AddPage("picker", ov, true, true)
	id.pages.SwitchToPage("picker")
	if id.app != nil {
		id.app.SetFocus(ov)
	}
}

// CloseTypePicker dismisses the type picker and restores the interface list.
func (id *InterfacesDisplay) CloseTypePicker() {
	if id.pickerOverlay == nil {
		return
	}
	id.pages.RemovePage("picker")
	id.pickerOverlay = nil
	id.pages.SwitchToPage("main")
	if id.app != nil {
		id.app.SetFocus(id.layout)
	}
}

// ShowForm mounts an interface Add/Edit form as the "form" page: an 85% x 85%
// overlay over a blank background, matching Python's AddInterfaceView
// (urwid.Overlay(form_box, background=SolidFill(" "), align='center',
// width=('relative',85), valign='middle', height=('relative',85)),
// Interfaces.py:1464-1472). The blank background is the overlay having no
// bottom primitive, so the list is not drawn through it.
func (id *InterfacesDisplay) ShowForm(v *InterfaceFormView) {
	id.CloseForm()

	// The form closes itself through OnCancel, so a Cancel button press, an Esc
	// and the wiring's own close all run the same restore.
	prevCancel := v.OnCancel
	v.OnCancel = func() {
		id.CloseForm()
		if prevCancel != nil {
			prevCancel()
		}
	}

	id.formView = v
	ov := NewSlotOverlay(nil, v.Dialog(), 85, 0)
	ov.SetHeightPct(85).SetInsets(0, 0)
	id.formOverlay = ov
	id.pages.AddPage("form", ov, true, true)
	id.pages.SwitchToPage("form")
	if id.app != nil {
		id.app.SetFocus(ov)
	}
	id.refreshShortcuts()
}

// CloseForm dismisses an open Add/Edit form and restores the interface list.
func (id *InterfacesDisplay) CloseForm() {
	if id.formOverlay == nil {
		return
	}
	id.pages.RemovePage("form")
	id.formOverlay = nil
	id.formView = nil
	id.pages.SwitchToPage("main")
	if id.app != nil {
		id.app.SetFocus(id.layout)
	}
	id.refreshShortcuts()
}

// FormView returns the Add/Edit form currently mounted, or nil.
func (id *InterfacesDisplay) FormView() *InterfaceFormView { return id.formView }

// SetInterfaces replaces the interface list, rebuilding the selectable items.
func (id *InterfacesDisplay) SetInterfaces(interfaces []InterfaceInfo) {
	id.items = interfaces
	if id.title != nil {
		id.title.SetText(interfaceHeader(len(interfaces)))
	}
	items := make([]*SelectableInterfaceItem, 0, len(interfaces))
	for _, iface := range interfaces {
		icon := GetInterfaceIcon(id.glyphset, iface.Type)
		items = append(items, NewSelectableInterfaceItem(iface.Name, iface.Type, iface.Connected, iface.Enabled, iface.TX, iface.RX, icon))
	}
	id.listBox.SetItems(items)

	// An open details view is live too. Python's ShowInterface polls the same
	// interface on the same 1 s cadence as the list (Interfaces.py:2921-2925), so
	// a radio that comes up while the view is open is reported there rather than
	// leaving the screen frozen at the moment it was opened.
	if id.showWidget != nil {
		for _, iface := range interfaces {
			if iface.Name == id.showWidget.Name() {
				id.showWidget.SetInfo(iface)
				break
			}
		}
	}
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
		// Navigating Up past the top interface item releases focus back to the
		// menu bar (matching the browser pane's OnReleaseFocus → FocusMenu), so
		// UpArrow from the first interface returns to the main menu instead of
		// clamping at the top and trapping focus in the list.
		if event.Key() == tcell.KeyUp && id.listBox.focusIdx <= 0 && id.OnReleaseFocus != nil {
			id.OnReleaseFocus()
			return nil
		}
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
	app        *App
	glyphset   string
	onActivate func(idx int)
}

func newInterfaceListBox(app *App, glyphset string) *interfaceListBox {
	b := &interfaceListBox{Box: tview.NewBox(), app: app, glyphset: glyphset, focusIdx: -1}
	return b
}

// SetItems replaces the list contents. Focus is preserved when it points at a
// still-valid item; otherwise it stays at -1 (no item focused). This matches
// Python's interface list, whose ListBox focus defaults to the non-selectable
// header (Interfaces.py:2905-2910 — [interface_header, Divider] lead the
// walker), so NO interface item shows the ● selection glyph until the user
// presses Down to move focus onto the first item. The wired app (if any) is
// propagated to each item so its Draw resolves the palette status colors.
func (b *interfaceListBox) SetItems(items []*SelectableInterfaceItem) {
	b.items = items
	for _, it := range items {
		it.SetApp(b.app)
	}
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
	visible := max(b.visibleCount(), 1)
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
	// Leave 1 blank buffer row at the bottom, matching Python's
	// iface_row_offset behavior (Interfaces.py:2837: list_rows =
	// terminal_rows - iface_row_offset; the ListBox renders 1 blank
	// buffer row after the last item).
	if h > 1 {
		h--
	}
	if w < 4 || h < InterfaceItemHeight || len(b.items) == 0 {
		return
	}
	b.scrollIntoView()
	rowY := y
	for i := b.offset; i < len(b.items); i++ {
		if rowY >= y+h {
			break
		}
		itemH := min(InterfaceItemHeight,
			// Bottom-clipped last item: urwid's ListBox renders the visible top
			// portion of a partially-fit item rather than skipping it entirely.
			y+h-rowY)
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

// MouseHandler implements click-to-focus, click-to-open and wheel scrolling.
// Click-to-focus mirrors Python's urwid ListBox mouse handling for the
// interface list (SelectableInterfaceItem has no mouse_event of its own, so the
// ListBox owns click-to-focus — Interfaces.py:1125-1252): a left click maps the
// click row to an item index via InterfaceItemHeight and the scroll offset,
// focuses that item (○→●), and moves tview focus onto the list so subsequent
// arrow keys navigate it. Click-to-open is a Go-only addition: a left click on
// the row that ALREADY holds the focus opens its details view — the same
// activation Enter fires — while a click that changes the focus only focuses,
// so a stray click never navigates away. Python has no click-to-open path. The
// wheel moves the focused item one step per notch — urwid's ListBox maps wheel
// to "up"/"down" keypresses and the viewport follows the focus — which is also
// what makes the wheel visibly scroll this list, since Draw re-pins the
// viewport to the focused item via scrollIntoView.
func (b *interfaceListBox) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (consumed bool, capture tview.Primitive) {
	return b.WrapMouseHandler(func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(tview.Primitive)) (consumed bool, capture tview.Primitive) {
		x, y := event.Position()
		_, by, _, _ := b.GetRect()
		if !b.InRect(x, y) {
			return false, nil
		}
		if len(b.items) == 0 {
			return false, nil
		}
		switch action {
		case tview.MouseLeftClick:
			idx := b.offset + (y-by)/InterfaceItemHeight
			idx = max(idx, 0)
			if idx >= len(b.items) {
				idx = len(b.items) - 1
			}
			// A click on the row that ALREADY holds the focus opens its details
			// view — the same activation Enter fires (onActivate). A click that
			// changes the focus only focuses, so a stray click never navigates
			// away (Go-only: Python's SelectableInterfaceItem has no mouse_event).
			alreadyFocused := b.focusIdx >= 0 && b.focusIdx == idx
			b.focusIdx = idx
			b.applyFocus()
			b.scrollIntoView()
			setFocus(b)
			if alreadyFocused && b.onActivate != nil {
				b.onActivate(idx)
			}
			return true, b
		case tview.MouseScrollUp, tview.MouseScrollDown:
			// Wheel = move the focused item one step (urwid ListBox maps wheel
			// to "up"/"down" keypresses, Interfaces.py:2911 SimpleFocusListWalker:
			// the viewport follows the focus). An offset-only wheel would be
			// undone every frame by Draw's scrollIntoView, which re-pins the
			// viewport to the focused item.
			if action == tview.MouseScrollDown {
				b.focusIdx++
				if b.focusIdx >= len(b.items) {
					b.focusIdx = len(b.items) - 1
					return false, nil
				}
			} else {
				if b.focusIdx <= 0 {
					return false, nil
				}
				b.focusIdx--
			}
			b.applyFocus()
			b.scrollIntoView()
			return true, nil
		}
		return false, nil
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

		fmt.Fprintf(&sb, "[::b]%v[-] %v(%v)[-]\n", escapeTviewTags(iface.Name), statusColor, iface.Status)
		fmt.Fprintf(&sb, "  Type: %v  Target: %v\n", escapeTviewTags(iface.Type), escapeTviewTags(iface.Target))
		fmt.Fprintf(&sb, "  Bandwidth: %v\n", formatBandwidth(iface.Bandwidth))

		if len(iface.Traffic) > 0 {
			sb.WriteString("\n")
			chartView, err := chart.PlotSingle(iface.Traffic)
			if err != nil {
				chartView = ""
			}
			for line := range strings.SplitSeq(chartView, "\n") {
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
	return fmt.Sprintf("%v %v %v(%v)[-]", icon, escapeTviewTags(iface.Name), statusColor, iface.Status)
}

// FormatInterfaceDetail produces a multi-line detail view for the
// interface info panel. Shows name, type, status, target, bandwidth,
// and recent traffic data when available.
// Matches Python's InterfaceDetail view layout.
func FormatInterfaceDetail(iface InterfaceInfo) string {
	var sb strings.Builder

	sb.WriteString("[::b]Interface Details[-]\n\n")
	fmt.Fprintf(&sb, "  Name: %v\n", escapeTviewTags(iface.Name))
	fmt.Fprintf(&sb, "  Type: %v\n", escapeTviewTags(iface.Type))

	statusIcon := "○"
	if iface.Status == "connected" {
		statusIcon = "●"
	}
	fmt.Fprintf(&sb, "  Status: %v %v\n", statusIcon, iface.Status)

	if iface.Target != "" {
		fmt.Fprintf(&sb, "  Target: %v\n", escapeTviewTags(iface.Target))
	}

	fmt.Fprintf(&sb, "  Bandwidth: %v\n", formatBandwidth(iface.Bandwidth))

	if len(iface.Traffic) > 0 {
		sb.WriteString("\n  [gray]Recent Traffic[-]\n")
		fmt.Fprintf(&sb, "  Samples: %v\n", len(iface.Traffic))
	}

	return sb.String()
}

// ShowEnableDisableConfirm shows a confirm dialog overlaid on the interfaces
// display (Python Interfaces.py:2570-2590: frame.body = urwid.Overlay(...,
// width=50, height=7), title "Confirm").
func (id *InterfacesDisplay) ShowEnableDisableConfirm(name string, enabled bool, onConfirm func()) {
	action := "enable"
	if enabled {
		action = "disable"
	}
	// Python on_toggle_enabled (Interfaces.py:2564-2568): the confirm text names
	// the action and the interface.
	msg := fmt.Sprintf("Are you sure you want to %v the %v interface?", action, name)
	close := id.closeDialog
	yes := NewUrwidButton("Yes").SetSelectedFunc(func() {
		close()
		if onConfirm != nil {
			onConfirm()
		}
	})
	no := NewUrwidButton("No").SetSelectedFunc(close)
	row := CreateUrwidButtonRow(yes, no)
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(NewUrwidCenterText(msg), 3, 0, false).
		AddItem(row, 1, 0, true)
	dialog := NewDialogLineBox("Confirm", layout, close)
	id.showDialogOverlay(dialog, 50, 7)
	wireDialogNav(id.app, close, []tview.Primitive{yes, no})
}

// interfaceHeader renders the interfaces header: the count while any interface
// exists, Python's add hint when none do (Interfaces.py:3146-3149).
func interfaceHeader(n int) string {
	if n == 0 {
		return "No interfaces found. Press Ctrl + A to add a new interface "
	}
	return fmt.Sprintf("Interfaces (%v)", n)
}

// ShowRestartRequired shows a notice that a restart is required, overlaid on
// the interfaces display (Python Interfaces.py:2589-2610: title "Notice",
// width=50, height=7, message + OK).
func (id *InterfacesDisplay) ShowRestartRequired() {
	msg := "RNS must be restarted for interface changes to take effect.\nRestart Nomad Network to apply changes."
	close := id.closeDialog
	ok := NewUrwidButton("OK").SetSelectedFunc(close)
	row := CreateUrwidButtonRow(ok)
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(NewUrwidCenterText(msg), 0, 1, false).
		AddItem(row, 1, 0, true)
	dialog := NewDialogLineBox("Notice", layout, close)
	id.showDialogOverlay(dialog, 50, 7)
	wireDialogNav(id.app, close, []tview.Primitive{ok})
}

// ShowInterfaceSaved shows Python's named save notice: an add reports
// "Interface {name} added. Restart NomadNet to start using this interface"
// (Interfaces.py:1960) and an edit "Interface {name} updated. Restart NomadNet
// for these changes to take effect" (:2201). The generic restart notice named
// no interface.
func (id *InterfacesDisplay) ShowInterfaceSaved(name string, added bool) {
	var msg string
	if added {
		msg = fmt.Sprintf("Interface %v added. Restart NomadNet to start using this interface", name)
	} else {
		msg = fmt.Sprintf("Interface %v updated. Restart NomadNet for these changes to take effect", name)
	}
	close := id.closeDialog
	ok := NewUrwidButton("OK").SetSelectedFunc(close)
	row := CreateUrwidButtonRow(ok)
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(NewUrwidCenterText(msg), 0, 1, false).
		AddItem(row, 1, 0, true)
	dialog := NewDialogLineBox("Notice", layout, close)
	id.showDialogOverlay(dialog, 50, 7)
	wireDialogNav(id.app, close, []tview.Primitive{ok})
}

// ShowEnableDisableNotice reports a completed toggle with Python's per-interface
// restart notice (Interfaces.py:2589-2617): title "Notice", width 50, height 8.
// enabled is the NEW state.
func (id *InterfacesDisplay) ShowEnableDisableNotice(name string, enabled bool) {
	state := "disabled"
	if enabled {
		state = "enabled"
	}
	msg := fmt.Sprintf("Interface %v has been %v.\nRestart required for changes to take effect.", name, state)
	close := id.closeDialog
	ok := NewUrwidButton("OK").SetSelectedFunc(close)
	row := CreateUrwidButtonRow(ok)
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(NewUrwidCenterText(msg), 0, 1, false).
		AddItem(row, 1, 0, true)
	dialog := NewDialogLineBox("Notice", layout, close)
	id.showDialogOverlay(dialog, 50, 8)
	wireDialogNav(id.app, close, []tview.Primitive{ok})
}

// ShowRemoveConfirm asks before removing an interface, mirroring Python
// remove_selected_interface (Interfaces.py:3080-3136): "Remove interface
// {name}?", title "?", a 35x5 overlay, and the "No" button focused so Enter
// never deletes. The removal itself shows no further notice in Python.
func (id *InterfacesDisplay) ShowRemoveConfirm(name string, onConfirm func()) {
	close := id.closeDialog
	yes := NewUrwidButton("Yes").SetSelectedFunc(func() {
		close()
		if onConfirm != nil {
			onConfirm()
		}
	})
	no := NewUrwidButton("No").SetSelectedFunc(close)
	row := CreateUrwidButtonRow(yes, no)
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(NewUrwidCenterText("Remove interface "+name+"?"), 3, 0, false).
		AddItem(row, 1, 0, true)
	dialog := NewDialogLineBox("?", layout, close)
	id.showDialogOverlay(dialog, 35, 5)
	wireDialogNav(id.app, close, []tview.Primitive{yes, no})
	if id.app != nil {
		id.app.SetFocus(no)
	}
}

// ShowInterfaceError shows an error message, overlaid on the interfaces display
// (Python Interfaces.py:2619-2650: title "Error", width=50, height=7, message +
// OK).
func (id *InterfacesDisplay) ShowInterfaceError(errMsg string) {
	msg := fmt.Sprintf("Error: %v", errMsg)
	close := id.closeDialog
	ok := NewUrwidButton("OK").SetSelectedFunc(close)
	row := CreateUrwidButtonRow(ok)
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(NewUrwidCenterText(msg), 0, 1, false).
		AddItem(row, 1, 0, true)
	dialog := NewDialogLineBox("Error", layout, close)
	id.showDialogOverlay(dialog, 50, 7)
	wireDialogNav(id.app, close, []tview.Primitive{ok})
}

// ShowRNSDisconnected shows an overlay when the RNS transport is lost, overlaid
// on the interfaces display (Python Interfaces.py:2794-2830: width=35, height=4,
// a passive notice with no button).
func (id *InterfacesDisplay) ShowRNSDisconnected() {
	msg := "(!) RNS Instance Disconnected\nWaiting to Reconnect..."
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(NewUrwidCenterText(msg), 0, 1, false)
	dialog := NewDialogLineBox("Disconnected", layout, id.closeDialog)
	id.showDialogOverlay(dialog, 35, 4)
}

// ShortcutText returns the shortcut bar for the interfaces view currently on
// screen, mirroring Python's InterfaceDisplayShortcuts: the list default, the
// ShowInterface bar advertising the h/v chart keys (set on
// switch_to_show_interface, Interfaces.py:2219,3205-3208), or the Add/Edit form
// bar (Interfaces.py:1428,3209-3215).
func (id *InterfacesDisplay) ShortcutText() string {
	switch {
	case id.formView != nil:
		return "[Up/Down] Navigate Fields [Enter] Select Option"
	case id.showWidget != nil:
		return "[Up/Down] Navigate [Tab] Switch Focus [h] Horizontal Charts [v] Vertical Charts "
	}
	return "[C-a] Add Interface [C-e] Edit Interface [C-x] Remove Interface [Enter] Show Interface [C-w] Open Text Editor"
}

// refreshShortcuts asks the main display to re-read ShortcutText after a view
// transition. The active page does not change, so the footer would otherwise
// keep whichever bar it cached when the page was entered.
func (id *InterfacesDisplay) refreshShortcuts() {
	if id.app != nil && id.app.Main != nil {
		id.app.Main.refreshShortcuts()
	}
}
