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
	"testing"
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
