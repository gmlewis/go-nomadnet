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
	"github.com/rivo/tview"
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

func TestInterfacesDisplayKeyboardShortcuts(t *testing.T) {
	t.Parallel()

	app := tview.NewApplication()
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
				t.Errorf("key %s was not consumed", tt.name)
			}
			if len(fired) != 1 || fired[0] != tt.want {
				t.Errorf("key %s fired %v, want [%s]", tt.name, fired, tt.want)
			}
		})
	}
}
