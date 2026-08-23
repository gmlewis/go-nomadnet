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

// SubInterfaceInfo holds data for a sub-interface of an RNodeMultiInterface,
// matching Python's Interfaces.py:2846-2862 sub-interface extraction.
type SubInterfaceInfo struct {
	Name   string
	Config map[string]any
}

// extractSubInterfaces extracts sub-interface configs from an
// RNodeMultiInterface config map. Sub-interfaces are dict-valued entries
// whose keys are NOT standard RNodeMultiInterface keys (type, port,
// interface_enabled, selected_interface_mode, configured_bitrate). Each
// sub-interface's Name is set from its config key, and the config dict gets
// a "name" key added. Returns nil for non-RNodeMultiInterface types.
//
// Python source: Interfaces.py:2846-2862.
func extractSubInterfaces(config map[string]any) []SubInterfaceInfo {
	if config["type"] != IfaceRNodeMulti {
		return nil
	}
	standardKeys := map[string]bool{
		"type":                    true,
		"port":                    true,
		"interface_enabled":       true,
		"selected_interface_mode": true,
		"configured_bitrate":      true,
	}
	var subs []SubInterfaceInfo
	for name, val := range config {
		if standardKeys[name] {
			continue
		}
		if sub, ok := val.(map[string]any); ok {
			sub["name"] = name
			subs = append(subs, SubInterfaceInfo{Name: name, Config: sub})
		}
	}
	return subs
}
