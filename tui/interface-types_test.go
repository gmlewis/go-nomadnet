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

func TestInterfaceTypeCategory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ifType string
		want   string
	}{
		{"BackboneInterface", "network"},
		{"AutoInterface", "network"},
		{"TCPClientInterface", "network"},
		{"TCPServerInterface", "network"},
		{"UDPInterface", "network"},
		{"I2PInterface", "network"},
		{"RNodeInterface", "rnode"},
		{"RNodeMultiInterface", "rnode"},
		{"SerialInterface", "serial"},
		{"KISSInterface", "serial"},
		{"AX25KISSInterface", "serial"},
		{"PipeInterface", "other"},
		{"CustomInterface", "other"},
		{"UnknownType", "other"},
	}

	for _, tt := range tests {
		t.Run(tt.ifType, func(t *testing.T) {
			t.Parallel()
			got := InterfaceCategory(tt.ifType)
			if got != tt.want {
				t.Errorf("InterfaceCategory(%q) = %q, want %q", tt.ifType, got, tt.want)
			}
		})
	}
}

func TestInterfaceTypeGlyph(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ifType string
		want   string
	}{
		{"BackboneInterface", "network"},
		{"AutoInterface", "network"},
		{"RNodeInterface", "rnode"},
		{"SerialInterface", "serial"},
		{"PipeInterface", "other"},
		{"CustomInterface", "other"},
		{"Unknown", "other"},
	}

	for _, tt := range tests {
		t.Run(tt.ifType, func(t *testing.T) {
			t.Parallel()
			got := InterfaceGlyph(tt.ifType)
			if got != tt.want {
				t.Errorf("InterfaceGlyph(%q) = %q, want %q", tt.ifType, got, tt.want)
			}
		})
	}
}

func TestInterfaceTypeIcon(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ifType string
		want   string
	}{
		{"TCPClientInterface", "↗"},
		{"TCPServerInterface", "↙"},
		{"RNodeInterface", "R"},
		{"SerialInterface", "↔"},
		{"PipeInterface", "#"},
	}

	for _, tt := range tests {
		t.Run(tt.ifType, func(t *testing.T) {
			t.Parallel()
			got := InterfaceIcon(tt.ifType)
			if got != tt.want {
				t.Errorf("InterfaceIcon(%q) = %q, want %q", tt.ifType, got, tt.want)
			}
		})
	}
}

func TestAllInterfaceTypes(t *testing.T) {
	t.Parallel()

	allTypes := AllInterfaceTypes()
	if len(allTypes) < 10 {
		t.Errorf("AllInterfaceTypes() returned %d types, want >= 10", len(allTypes))
	}
	// Verify all types have a category
	for _, it := range allTypes {
		cat := InterfaceCategory(it)
		if cat == "" {
			t.Errorf("InterfaceCategory(%q) returned empty", it)
		}
	}
}

func TestRequiredFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		ifType string
		want   []string
	}{
		{"TCPClientInterface", []string{"target_host", "target_port"}},
		{"TCPServerInterface", []string{"listen_ip"}},
		{"UDPInterface", []string{"listen_ip", "forward_ip", "forward_port"}},
		{"RNodeInterface", []string{"frequency"}},
		{"SerialInterface", []string{"speed"}},
		{"PipeInterface", []string{"command"}},
		{"KISSInterface", []string{"speed", "preamble", "txtail", "slottime", "persistence"}},
	}

	for _, tt := range tests {
		t.Run(tt.ifType, func(t *testing.T) {
			t.Parallel()
			got := RequiredFields(tt.ifType)
			if len(got) != len(tt.want) {
				t.Errorf("RequiredFields(%q) = %v (len %d), want %v (len %d)",
					tt.ifType, got, len(got), tt.want, len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("RequiredFields(%q)[%d] = %q, want %q", tt.ifType, i, got[i], tt.want[i])
				}
			}
		})
	}
}
