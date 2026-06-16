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

import "fmt"

// ToggleInterfaceEnabled toggles the enabled state of an interface
// item and updates the config dictionary accordingly. If
// "interface_enabled" exists in config it updates that key; otherwise
// it updates the "enabled" key.
// Matches Python's on_toggle_enabled() at Interfaces.py:2516.
func ToggleInterfaceEnabled(item *SelectableInterfaceItem, config map[string]any) {
	item.IsEnabled = !item.IsEnabled

	if _, hasInterfaceEnabled := config["interface_enabled"]; hasInterfaceEnabled {
		config["interface_enabled"] = item.IsEnabled
	} else {
		config["enabled"] = item.IsEnabled
	}
}

// ToggleConfirmationMessage returns the confirmation dialog message
// for toggling an interface's enabled state. Matches Python's
// on_toggle_enabled() at Interfaces.py:2516.
func ToggleConfirmationMessage(ifaceName string, isEnabled bool) string {
	action := "disable"
	if !isEnabled {
		action = "enable"
	}
	return fmt.Sprintf("Are you sure you want to %v the %v interface?", action, ifaceName)
}

// ToggleButtonLabel returns the label for the toggle button based on
// the current enabled state. Matches Python's toggle_button label
// logic at Interfaces.py:2228.
func ToggleButtonLabel(isEnabled bool) string {
	if isEnabled {
		return "Disable"
	}
	return "Enable"
}
