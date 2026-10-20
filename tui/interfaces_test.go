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
	"strings"
	"testing"

	"github.com/gmlewis/tcell/v2"
	"github.com/gmlewis/tview"
)

func TestFormatInterfaceEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		iface InterfaceInfo
		want  string
	}{
		{
			name: "connected TCP client",
			iface: InterfaceInfo{
				Name:      "TCP-Client",
				Type:      "TCPClientInterface",
				Status:    "connected",
				Target:    "192.168.1.1:4242",
				Bandwidth: 1024,
				Traffic:   []float64{100, 200, 300},
			},
			want: "↗ TCP-Client",
		},
		{
			name: "disconnected RNode",
			iface: InterfaceInfo{
				Name:   "LoRa-0",
				Type:   "RNodeInterface",
				Status: "disconnected",
			},
			want: "R LoRa-0",
		},
		{
			name: "connected serial",
			iface: InterfaceInfo{
				Name:   "USB-Serial",
				Type:   "SerialInterface",
				Status: "connected",
			},
			want: "↔ USB-Serial",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := FormatInterfaceEntry(tt.iface)
			if !strings.Contains(got, tt.want) {
				t.Errorf("FormatInterfaceEntry() = %q, want to contain %q", got, tt.want)
			}
		})
	}
}

func TestFormatInterfaceEntryStatus(t *testing.T) {
	t.Parallel()

	connected := InterfaceInfo{Name: "eth0", Status: "connected"}
	disconnected := InterfaceInfo{Name: "eth0", Status: "disconnected"}

	gotC := FormatInterfaceEntry(connected)
	gotD := FormatInterfaceEntry(disconnected)

	if strings.Contains(gotC, "[red]") {
		t.Error("connected interface should not have red status")
	}
	if !strings.Contains(gotD, "[red]") {
		t.Error("disconnected interface should have red status")
	}
}

func TestFormatInterfaceDetail(t *testing.T) {
	t.Parallel()

	iface := InterfaceInfo{
		Name:      "TCP-Client",
		Type:      "TCPClientInterface",
		Status:    "connected",
		Target:    "192.168.1.1:4242",
		Bandwidth: 1024000,
		Traffic:   []float64{100, 200, 300},
	}

	detail := FormatInterfaceDetail(iface)
	if !strings.Contains(detail, "TCP-Client") {
		t.Error("detail missing interface name")
	}
	if !strings.Contains(detail, "TCPClientInterface") {
		t.Error("detail missing interface type")
	}
	if !strings.Contains(detail, "connected") {
		t.Error("detail missing status")
	}
	if !strings.Contains(detail, "192.168.1.1:4242") {
		t.Error("detail missing target")
	}
	if !strings.Contains(detail, "Bandwidth:") {
		t.Error("detail missing bandwidth")
	}
}

func TestFormatInterfaceDetailNoTraffic(t *testing.T) {
	t.Parallel()

	iface := InterfaceInfo{
		Name:   "eth0",
		Type:   "TCPClientInterface",
		Status: "disconnected",
	}

	detail := FormatInterfaceDetail(iface)
	if strings.Contains(detail, "Traffic:") {
		t.Error("detail should not show traffic when empty")
	}
}

func TestFormatMessageSelf(t *testing.T) {
	t.Parallel()

	msg := ChannelMessage{
		Nick:   "Alice",
		Text:   "Hello",
		IsSelf: true,
	}

	got := FormatMessage(msg, ThemeDark)
	if !strings.Contains(got, "Alice") {
		t.Error("message missing nick")
	}
	if !strings.Contains(got, "Hello") {
		t.Error("message missing text")
	}
	if !strings.Contains(got, "green") {
		t.Error("message missing self indicator")
	}
}

func TestFormatMessageSystem(t *testing.T) {
	t.Parallel()

	msg := ChannelMessage{
		Text:     "User joined",
		IsSystem: true,
	}

	got := FormatMessage(msg, ThemeDark)
	if !strings.Contains(got, "User joined") {
		t.Error("message missing text")
	}
	if !strings.Contains(got, "system") {
		t.Error("message missing system indicator")
	}
}

func TestFormatMessageNotice(t *testing.T) {
	t.Parallel()

	msg := ChannelMessage{
		Text:     "Topic changed",
		IsNotice: true,
	}

	got := FormatMessage(msg, ThemeDark)
	if !strings.Contains(got, "Topic changed") {
		t.Error("message missing text")
	}
	if !strings.Contains(got, "notice") {
		t.Error("message missing notice indicator")
	}
}

func TestFormatMessageError(t *testing.T) {
	t.Parallel()

	msg := ChannelMessage{
		Text:    "Connection lost",
		IsError: true,
	}

	got := FormatMessage(msg, ThemeDark)
	if !strings.Contains(got, "Connection lost") {
		t.Error("message missing text")
	}
	if !strings.Contains(got, "error") {
		t.Error("message missing error indicator")
	}
}

func TestFormatMessageMention(t *testing.T) {
	t.Parallel()

	msg := ChannelMessage{
		Nick:    "Bob",
		Text:    "Hey Alice!",
		Mention: true,
	}

	got := FormatMessage(msg, ThemeDark)
	if !strings.Contains(got, "Bob") {
		t.Error("message missing nick")
	}
	if !strings.Contains(got, "mention") {
		t.Error("message missing mention indicator")
	}
}

// TestInterfacesDisplayTitleCeilLeftCentering verifies the "Interfaces" header
// is CEIL-left centered (urwid Text(align=CENTER) puts the extra column on the
// LEFT when the slack is odd; tview.AlignCenter floors it). At width 81 the
// 10-char title has slack 71 -> ceil-left leftPad 36, floor 35, so the 'I' must
// land at column 36, not 35.
func TestInterfacesDisplayTitleCeilLeftCentering(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	// Python heads the list with "Interfaces (N)" (Interfaces.py:3146), so the
	// centering check uses a page with one interface.
	id := NewInterfacesDisplay(app, []InterfaceInfo{{Name: "eth0", Type: "AutoInterface"}})

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(81, 24)
	id.layout.SetRect(0, 0, 81, 24)
	id.layout.Draw(screen)

	title := "Interfaces (1)"
	start := (81 - len(title) + 1) / 2 // urwid's ceil-left centering
	if main, _, _, _ := cellContent(screen, start, 0); main != 'I' {
		t.Errorf("title 'I' at col %v = %q, want 'I' (ceil-left)", start, main)
	}
	if main, _, _, _ := cellContent(screen, start-1, 0); main == 'I' {
		t.Errorf("title 'I' found at col %v (floor-left); want ceil-left at %v", start-1, start)
	}
}

func TestInterfacesDisplayKeyboardShortcuts(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	id := NewInterfacesDisplay(app, nil)

	var fired []string
	id.OnAddInterface = func() { fired = append(fired, "add") }
	id.OnEditInterface = func() { fired = append(fired, "edit") }
	id.OnRemoveInterface = func() { fired = append(fired, "remove") }
	id.OnConfigEditor = func() { fired = append(fired, "config") }

	tests := []struct {
		name  string
		event *tcell.EventKey
		want  string
	}{
		{"ctrl-a", tcell.NewEventKey(tcell.KeyCtrlA, 0, tcell.ModNone), "add"},
		{"ctrl-e", tcell.NewEventKey(tcell.KeyCtrlE, 0, tcell.ModNone), "edit"},
		{"ctrl-x", tcell.NewEventKey(tcell.KeyCtrlX, 0, tcell.ModNone), "remove"},
		{"ctrl-w", tcell.NewEventKey(tcell.KeyCtrlW, 0, tcell.ModNone), "config"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fired = fired[:0]
			result := id.handleInput(tt.event)
			if result != nil {
				t.Errorf("key %v was not consumed", tt.name)
			}
			if len(fired) != 1 || fired[0] != tt.want {
				t.Errorf("key %v fired %v, want [%v]", tt.name, fired, tt.want)
			}
		})
	}
}

// overlayDialogText walks an open interfaces dialog's content (including the
// button row) and returns every text primitive it contains.
func overlayDialogText(t *testing.T, ov *SlotOverlay) string {
	t.Helper()
	flex, ok := ov.Dialog().Content().(*tview.Flex)
	if !ok {
		t.Fatalf("dialog content is %T, want *tview.Flex", ov.Dialog().Content())
	}
	var sb strings.Builder
	var walk func(p tview.Primitive)
	walk = func(p tview.Primitive) {
		switch v := p.(type) {
		case *tview.TextView:
			sb.WriteString(v.GetText(true))
			sb.WriteString("\n")
		case *centeredText:
			sb.WriteString(v.GetText())
			sb.WriteString("\n")
		case *urwidCenterText:
			sb.WriteString(v.text)
			sb.WriteString("\n")
		case *urwidLeftText:
			sb.WriteString(v.text)
			sb.WriteString("\n")
		case *tview.Flex:
			for i := 0; i < v.GetItemCount(); i++ {
				walk(v.GetItem(i))
			}
		case *urwidColumns:
			for _, child := range v.children {
				walk(child)
			}
		}
	}
	for i := 0; i < flex.GetItemCount(); i++ {
		walk(flex.GetItem(i))
	}
	return sb.String()
}

// overlayDialogButton finds a button in an open interfaces dialog.
func overlayDialogButton(t *testing.T, ov *SlotOverlay, label string) *UrwidButton {
	t.Helper()
	flex, ok := ov.Dialog().Content().(*tview.Flex)
	if !ok {
		t.Fatalf("dialog content is %T, want *tview.Flex", ov.Dialog().Content())
	}
	for i := 0; i < flex.GetItemCount(); i++ {
		row, ok := flex.GetItem(i).(*urwidColumns)
		if !ok {
			continue
		}
		for _, child := range row.children {
			if btn, ok := child.(*UrwidButton); ok && btn.Label() == label {
				return btn
			}
		}
	}
	t.Fatalf("dialog has no %q button", label)
	return nil
}

// TestInterfacesDisplayShowEnableDisableConfirm pins the confirm dialog: the
// question names the action and interface, Yes runs the callback after
// closing, and No closes without it (Interfaces.py:2516-2617).
func TestInterfacesDisplayShowEnableDisableConfirm(t *testing.T) {
	t.Parallel()

	app := newTestApp()
	id := NewInterfacesDisplay(app, nil)

	var confirmed int
	id.ShowEnableDisableConfirm("RNode", true, func() { confirmed++ })
	if id.dialogOverlay == nil {
		t.Fatal("ShowEnableDisableConfirm did not open the overlay")
	}
	if title := id.dialogOverlay.Dialog().GetTitle(); title != "Confirm" {
		t.Errorf("dialog title = %q, want %q", title, "Confirm")
	}
	if text := overlayDialogText(t, id.dialogOverlay); !strings.Contains(text, "Are you sure you want to disable the RNode interface?") {
		t.Errorf("dialog text %q missing the confirm question", text)
	}

	// No closes without confirming.
	overlayDialogButton(t, id.dialogOverlay, "No").InputHandler()(
		tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(tview.Primitive) {})
	if id.dialogOverlay != nil {
		t.Error("No left the dialog open")
	}
	if confirmed != 0 {
		t.Errorf("No ran the confirm callback")
	}

	// Yes closes and confirms.
	id.ShowEnableDisableConfirm("RNode", false, func() { confirmed++ })
	if text := overlayDialogText(t, id.dialogOverlay); !strings.Contains(text, "enable the RNode interface?") {
		t.Errorf("enable dialog text %q missing the question", text)
	}
	overlayDialogButton(t, id.dialogOverlay, "Yes").InputHandler()(
		tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(tview.Primitive) {})
	if id.dialogOverlay != nil {
		t.Error("Yes left the dialog open")
	}
	if confirmed != 1 {
		t.Errorf("Yes did not run the confirm callback (%v calls)", confirmed)
	}
}

func TestInterfacesDisplayShowRestartRequired(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	id := NewInterfacesDisplay(app, nil)

	id.ShowRestartRequired()
	if id.dialogOverlay == nil {
		t.Fatal("ShowRestartRequired did not open the overlay")
	}
	if title := id.dialogOverlay.Dialog().GetTitle(); title != "Notice" {
		t.Errorf("dialog title = %q, want Notice", title)
	}
	text := overlayDialogText(t, id.dialogOverlay)
	for _, want := range []string{
		"RNS must be restarted for interface changes to take effect.",
		"Restart Nomad Network to apply changes.",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("notice %q missing %q", text, want)
		}
	}
}

func TestInterfacesDisplayShowInterfaceError(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	id := NewInterfacesDisplay(app, nil)

	id.ShowInterfaceError("Test error message")
	if id.dialogOverlay == nil {
		t.Fatal("ShowInterfaceError did not open the overlay")
	}
	if title := id.dialogOverlay.Dialog().GetTitle(); title != "Error" {
		t.Errorf("dialog title = %q, want Error", title)
	}
	if text := overlayDialogText(t, id.dialogOverlay); !strings.Contains(text, "Error: Test error message") {
		t.Errorf("error dialog %q missing the message", text)
	}
}

// TestInterfacesDisplayShowRNSDisconnected pins the passive disconnected
// notice (Interfaces.py:2794-2830): title "Disconnected" and the two-line
// warning, with no buttons to dismiss it.
func TestInterfacesDisplayShowRNSDisconnected(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	id := NewInterfacesDisplay(app, nil)

	id.ShowRNSDisconnected()
	if id.dialogOverlay == nil {
		t.Fatal("ShowRNSDisconnected did not open the overlay")
	}
	if title := id.dialogOverlay.Dialog().GetTitle(); title != "Disconnected" {
		t.Errorf("dialog title = %q, want Disconnected", title)
	}
	if text := overlayDialogText(t, id.dialogOverlay); !strings.Contains(text, "(!) RNS Instance Disconnected") ||
		!strings.Contains(text, "Waiting to Reconnect...") {
		t.Errorf("disconnected notice %q missing its lines", text)
	}
}

// TestInterfacesDisplayListFocusDraw verifies the selectable interface list
// renders rounded boxes with NO item focused at boot (○ on all, matching
// Python's ListBox whose focus defaults to the non-selectable header), Down
// focuses the first item (●), a second Down moves ● to the second, and Enter
// fires OnShowInterface with the focused index.
func TestInterfacesDisplayListFocusDraw(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	ifaces := []InterfaceInfo{
		{Name: "RNode1", Type: "RNodeInterface", Status: "connected", Connected: true, Enabled: true},
		{Name: "TCP1", Type: "TCPClientInterface", Status: "disconnected", Connected: false, Enabled: true},
		{Name: "Auto1", Type: "AutoInterface", Status: "connected", Connected: true, Enabled: true},
	}
	id := NewInterfacesDisplay(app, ifaces)

	var shown int
	id.OnShowInterface = func(idx int) { shown = idx }

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(80, 24)
	id.layout.SetRect(0, 0, 80, 24)
	id.layout.Draw(screen)

	// No outer border (Python has none): title(2) => first box top at y=2, title
	// row y=3. Box content starts at x = border(1) + pad(2) = 3. No item is
	// focused at boot (Python's list focus is on the header) => ○ on all.
	if c, _, _, _ := cellContent(screen, 3, 3); c != '○' {
		t.Errorf("first item selection glyph at boot = %q, want ○", c)
	}
	if c, _, _, _ := cellContent(screen, 3, 10); c != '○' {
		t.Errorf("second item selection glyph at boot = %q, want ○", c)
	}
	if id.SelectedIndex() != -1 {
		t.Errorf("SelectedIndex at boot = %v, want -1", id.SelectedIndex())
	}

	// First Down focuses the first item (Python: Down from header -> item 0).
	id.handleInput(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	id.layout.Draw(screen)
	if c, _, _, _ := cellContent(screen, 3, 3); c != '●' {
		t.Errorf("after first Down, first item glyph = %q, want ●", c)
	}
	if c, _, _, _ := cellContent(screen, 3, 10); c != '○' {
		t.Errorf("after first Down, second item glyph = %q, want ○", c)
	}
	if id.SelectedIndex() != 0 {
		t.Errorf("SelectedIndex after first Down = %v, want 0", id.SelectedIndex())
	}

	// A second Down moves focus to the second item.
	id.handleInput(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	id.layout.Draw(screen)
	if c, _, _, _ := cellContent(screen, 3, 3); c != '○' {
		t.Errorf("after second Down, first item glyph = %q, want ○", c)
	}
	if c, _, _, _ := cellContent(screen, 3, 10); c != '●' {
		t.Errorf("after second Down, second item glyph = %q, want ●", c)
	}
	if id.SelectedIndex() != 1 {
		t.Errorf("SelectedIndex after second Down = %v, want 1", id.SelectedIndex())
	}

	// Enter fires OnShowInterface with the focused index.
	id.handleInput(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone))
	if shown != 1 {
		t.Errorf("OnShowInterface fired with %v, want 1", shown)
	}
}

// TestInterfacesListBoxEnterActivation pins the listbox-level Enter path:
// activating the focused item through the listbox's own InputHandler routes to
// OnShowInterface (Python SelectableInterfaceItem.keypress "enter" →
// parent.switch_to_show_interface(name), Interfaces.py:1246-1249 — the Go
// display hoists that to the layout capture, and the listbox forwards the
// same activation when it owns the event).
func TestInterfacesListBoxEnterActivation(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	ifaces := []InterfaceInfo{
		{Name: "RNode1", Type: "RNodeInterface", Connected: true, Enabled: true},
		{Name: "TCP1", Type: "TCPClientInterface", Connected: false, Enabled: true},
	}
	id := NewInterfacesDisplay(app, ifaces)

	var shown int
	id.OnShowInterface = func(idx int) { shown = idx }

	id.listBox.HandleKey(tcell.KeyDown) // focus item 0
	id.listBox.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(p tview.Primitive) { app.SetFocus(p) })
	if shown != 0 {
		t.Errorf("listbox Enter activation fired OnShowInterface with idx %v, want 0", shown)
	}

	id.listBox.HandleKey(tcell.KeyDown) // focus item 1
	id.listBox.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, tcell.ModNone), func(p tview.Primitive) { app.SetFocus(p) })
	if shown != 1 {
		t.Errorf("listbox Enter activation fired OnShowInterface with idx %v, want 1", shown)
	}
}
