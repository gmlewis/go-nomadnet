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
)

func TestCategorizeInterfaceParams(t *testing.T) {
	t.Parallel()

	config := map[string]any{
		"type":              "TCPServerInterface",
		"interface_enabled": true,
		"port":              "4242",
		"listen_ip":         "0.0.0.0",
		"frequency":         868500000,
		"bandwidth":         62500,
		"network_name":      "mynet",
		"passphrase":        "secret123",
		"custom_key":        "custom_val",
	}

	cats := CategorizeInterfaceParams(config)

	if _, ok := cats.Connection["port"]; !ok {
		t.Error("port should be in Connection category")
	}
	if _, ok := cats.Connection["listen_ip"]; !ok {
		t.Error("listen_ip should be in Connection category")
	}
	if _, ok := cats.Radio["frequency"]; !ok {
		t.Error("frequency should be in Radio category")
	}
	if _, ok := cats.Radio["bandwidth"]; !ok {
		t.Error("bandwidth should be in Radio category")
	}
	if _, ok := cats.Network["network_name"]; !ok {
		t.Error("network_name should be in Network category")
	}
	if _, ok := cats.IFAC["passphrase"]; !ok {
		t.Error("passphrase should be in IFAC category")
	}
	if _, ok := cats.Other["custom_key"]; !ok {
		t.Error("custom_key should be in Other category")
	}
	if _, ok := cats.Connection["type"]; ok {
		t.Error("type should be excluded")
	}
	if _, ok := cats.Connection["interface_enabled"]; ok {
		t.Error("interface_enabled should be excluded")
	}
}

func TestCategorizeInterfaceParamsSkipsEmpty(t *testing.T) {
	t.Parallel()

	config := map[string]any{
		"port":   "",
		"device": nil,
	}

	cats := CategorizeInterfaceParams(config)

	if len(cats.Connection)+len(cats.Radio)+len(cats.Network)+len(cats.IFAC)+len(cats.Other) != 0 {
		t.Error("empty/nil values should be excluded")
	}
}

func TestFormatParamValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		key   string
		value any
		want  string
	}{
		{"bool true", "outgoing", true, "Yes"},
		{"bool false", "outgoing", false, "No"},
		{"frequency", "frequency", 868500000, "868.500 MHz"},
		{"bandwidth", "bandwidth", 62500, "62.5 kHz"},
		{"passphrase", "passphrase", "secret123", "*********"},
		{"string value", "listen_ip", "0.0.0.0", "0.0.0.0"},
		{"int value", "speed", 115200, "115200"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := FormatParamValue(tt.key, tt.value)
			if got != tt.want {
				t.Errorf("FormatParamValue(%q, %v) = %q, want %q", tt.key, tt.value, got, tt.want)
			}
		})
	}
}

func TestFormatParamKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"listen_port", "Listen Port"},
		{"target_host", "Target Host"},
		{"i2p_tunneled", "I2P Tunneled"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			got := FormatParamKey(tt.input)
			if got != tt.want {
				t.Errorf("FormatParamKey(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestInterfaceShowChartOrientationToggle pins the h/v chart-layout keys
// (Interfaces.py:2717-2752): below 145 columns the RX/TX boxes start stacked,
// `h` switches to the side-by-side Columns layout, `v` switches back, and
// neither key does anything while the interface is disconnected.
func TestInterfaceShowChartOrientationToggle(t *testing.T) {
	t.Parallel()

	const (
		rxTitle = "RX Traffic (60s)"
		txTitle = "TX Traffic (60s)"
	)
	sideBySide := func(rows []string) bool {
		for _, row := range rows {
			if strings.Contains(row, rxTitle) && strings.Contains(row, txTitle) {
				return true
			}
		}
		return false
	}

	info := InterfaceInfo{Name: "test", Type: "TCPServerInterface", Connected: true, Enabled: true}
	s := NewInterfaceShow(nil, info)
	if sideBySide(s.chartsRows(100)) {
		t.Fatal("width 100 default = side by side, want stacked")
	}
	if !sideBySide(s.chartsRows(145)) {
		t.Fatal("width 145 default = stacked, want side by side")
	}

	// A narrow view: `h` overrides the width default.
	if sideBySide(s.chartsRows(100)) {
		t.Fatal("precondition: width 100 must start stacked")
	}
	if key := s.handleKey(tcell.NewEventKey(tcell.KeyRune, 'h', tcell.ModNone)); key != nil {
		t.Errorf("h key = %v, want consumed while connected", key)
	}
	if !sideBySide(s.chartsRows(100)) {
		t.Error("after h the boxes must be side by side at width 100 (Interfaces.py:2717-2720)")
	}

	// `v` switches back, and the override wins over the width default.
	if key := s.handleKey(tcell.NewEventKey(tcell.KeyRune, 'v', tcell.ModNone)); key != nil {
		t.Errorf("v key = %v, want consumed while connected", key)
	}
	if sideBySide(s.chartsRows(200)) {
		t.Error("after v the boxes must be stacked even at width 200 (Interfaces.py:2721-2723)")
	}

	// Disconnected: neither key changes the layout (Python gates both on
	// self.is_connected).
	off := NewInterfaceShow(nil, InterfaceInfo{Name: "test", Connected: false})
	if key := off.handleKey(tcell.NewEventKey(tcell.KeyRune, 'h', tcell.ModNone)); key == nil {
		t.Error("h key on a disconnected interface must not be consumed")
	}
	if off.layoutSet {
		t.Error("h key on a disconnected interface must not set the layout")
	}
}

// TestInterfaceShortcutText pins Python's three interfaces shortcut bars and
// when each is shown: the list default, the ShowInterface bar with the h/v
// hints (set on switch_to_show_interface, Interfaces.py:2219,3205-3208), and
// the Add/Edit form bar (Interfaces.py:1428,3209-3215).
func TestInterfaceShortcutText(t *testing.T) {
	t.Parallel()

	const (
		defaultBar = "[C-a] Add Interface [C-e] Edit Interface [C-x] Remove Interface [Enter] Show Interface [C-w] Open Text Editor"
		showBar    = "[Up/Down] Navigate [Tab] Switch Focus [h] Horizontal Charts [v] Vertical Charts "
		formBar    = "[Up/Down] Navigate Fields [Enter] Select Option"
	)

	id := &InterfacesDisplay{}
	if got := id.ShortcutText(); got != defaultBar {
		t.Errorf("list bar = %q, want %q", got, defaultBar)
	}
	id.showWidget = &interfaceShow{}
	if got := id.ShortcutText(); got != showBar {
		t.Errorf("show bar = %q, want %q", got, showBar)
	}
	id.formView = &InterfaceFormView{}
	if got := id.ShortcutText(); got != formBar {
		t.Errorf("form bar = %q, want %q", got, formBar)
	}
}
