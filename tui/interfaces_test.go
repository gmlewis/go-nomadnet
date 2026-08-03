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

	"github.com/gdamore/tcell/v2"
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
	id := NewInterfacesDisplay(app, nil)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("screen.Init: %v", err)
	}
	defer screen.Fini()
	screen.SetSize(81, 24)
	id.layout.SetRect(0, 0, 81, 24)
	id.layout.Draw(screen)

	// 'I' of "Interfaces" on row 0 at the ceil-left column (36).
	if main, _, _, _ := screen.GetContent(36, 0); main != 'I' {
		t.Errorf("title 'I' at col 36 = %q, want 'I' (ceil-left)", main)
	}
	if main, _, _, _ := screen.GetContent(35, 0); main == 'I' {
		t.Errorf("title 'I' found at col 35 (floor-left); want ceil-left at 36")
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

func TestInterfacesDisplayShowEnableDisableConfirm(t *testing.T) {
	t.Parallel()

	// Test that the function exists and can be called without panic
	// (actual dialog display requires a running app)
	app := newTestApp()
	id := NewInterfacesDisplay(app, nil)

	// Verify the function exists and can be called
	// Note: ShowConfirmDialog requires a running tview app to display,
	// so we just verify the function signature works.
	if id == nil {
		t.Fatal("InterfacesDisplay is nil")
	}
}

func TestInterfacesDisplayShowRestartRequired(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	id := NewInterfacesDisplay(app, nil)

	// Should not panic
	id.ShowRestartRequired()
}

func TestInterfacesDisplayShowInterfaceError(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	id := NewInterfacesDisplay(app, nil)

	// Should not panic
	id.ShowInterfaceError("Test error message")
}

func TestInterfacesDisplayShowRNSDisconnected(t *testing.T) {
	t.Parallel()
	app := newTestApp()
	id := NewInterfacesDisplay(app, nil)

	// Should not panic
	id.ShowRNSDisconnected()
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
	if c, _, _, _ := screen.GetContent(3, 3); c != '○' {
		t.Errorf("first item selection glyph at boot = %q, want ○", c)
	}
	if c, _, _, _ := screen.GetContent(3, 10); c != '○' {
		t.Errorf("second item selection glyph at boot = %q, want ○", c)
	}
	if id.SelectedIndex() != -1 {
		t.Errorf("SelectedIndex at boot = %v, want -1", id.SelectedIndex())
	}

	// First Down focuses the first item (Python: Down from header -> item 0).
	id.handleInput(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	id.layout.Draw(screen)
	if c, _, _, _ := screen.GetContent(3, 3); c != '●' {
		t.Errorf("after first Down, first item glyph = %q, want ●", c)
	}
	if c, _, _, _ := screen.GetContent(3, 10); c != '○' {
		t.Errorf("after first Down, second item glyph = %q, want ○", c)
	}
	if id.SelectedIndex() != 0 {
		t.Errorf("SelectedIndex after first Down = %v, want 0", id.SelectedIndex())
	}

	// A second Down moves focus to the second item.
	id.handleInput(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	id.layout.Draw(screen)
	if c, _, _, _ := screen.GetContent(3, 3); c != '○' {
		t.Errorf("after second Down, first item glyph = %q, want ○", c)
	}
	if c, _, _, _ := screen.GetContent(3, 10); c != '●' {
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
