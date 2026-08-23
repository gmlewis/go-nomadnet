// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package tui

import (
	"sort"
	"testing"
)

// TestExtractSubInterfaces verifies that extractSubInterfaces extracts
// sub-interface configs from an RNodeMultiInterface config map, matching
// Python's Interfaces.py:2846-2862. Sub-interfaces are dict-valued entries
// that are not standard RNodeMultiInterface keys. Each gets its name set
// from the config key.
func TestExtractSubInterfaces(t *testing.T) {
	t.Parallel()

	t.Run("RNodeMultiInterface with sub-interfaces", func(t *testing.T) {
		t.Parallel()
		config := map[string]any{
			"type":                    "RNodeMultiInterface",
			"port":                    "/dev/ttyUSB0",
			"interface_enabled":       true,
			"selected_interface_mode": "data",
			"configured_bitrate":      115200,
			"SubInterfaceA":           map[string]any{"type": "AX25KISSInterface", "callsign": "TEST"},
			"SubInterfaceB":           map[string]any{"type": "RNodeInterface", "frequency": 433000000},
		}

		subs := extractSubInterfaces(config)
		if len(subs) != 2 {
			t.Fatalf("got %d sub-interfaces, want 2", len(subs))
		}
		names := []string{subs[0].Name, subs[1].Name}
		sort.Strings(names)
		if names[0] != "SubInterfaceA" || names[1] != "SubInterfaceB" {
			t.Errorf("sub-interface names = %v, want [SubInterfaceA SubInterfaceB]", names)
		}
	})

	t.Run("non-RNodeMultiInterface returns nil", func(t *testing.T) {
		t.Parallel()
		config := map[string]any{
			"type": "TCPClientInterface",
			"SubA": map[string]any{"type": "RNodeInterface"},
		}
		subs := extractSubInterfaces(config)
		if subs != nil {
			t.Errorf("non-RNodeMultiInterface returned %d sub-interfaces, want nil", len(subs))
		}
	})

	t.Run("no sub-interfaces", func(t *testing.T) {
		t.Parallel()
		config := map[string]any{
			"type":                    "RNodeMultiInterface",
			"port":                    "/dev/ttyUSB0",
			"interface_enabled":       true,
			"selected_interface_mode": "data",
			"configured_bitrate":      115200,
		}
		subs := extractSubInterfaces(config)
		if len(subs) != 0 {
			t.Errorf("got %d sub-interfaces, want 0", len(subs))
		}
	})

	t.Run("sub-interface name is set from config key", func(t *testing.T) {
		t.Parallel()
		config := map[string]any{
			"type":  "RNodeMultiInterface",
			"port":  "/dev/ttyUSB0",
			"MySub": map[string]any{"type": "AX25KISSInterface"},
		}
		subs := extractSubInterfaces(config)
		if len(subs) != 1 {
			t.Fatalf("got %d sub-interfaces, want 1", len(subs))
		}
		if subs[0].Name != "MySub" {
			t.Errorf("sub-interface name = %q, want %q", subs[0].Name, "MySub")
		}
		if subs[0].Config["name"] != "MySub" {
			t.Errorf("sub-interface config name = %v, want %q", subs[0].Config["name"], "MySub")
		}
	})
}
