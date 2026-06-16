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

func TestToggleInterfaceEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		ifaceName     string
		isEnabled     bool
		config        map[string]any
		wantEnabled   bool
		wantConfigKey string
		wantConfigVal bool
	}{
		{
			name:      "disable with interface_enabled key",
			ifaceName: "myiface",
			isEnabled: true,
			config: map[string]any{
				"interface_enabled": true,
			},
			wantEnabled:   false,
			wantConfigKey: "interface_enabled",
			wantConfigVal: false,
		},
		{
			name:      "enable with interface_enabled key",
			ifaceName: "myiface",
			isEnabled: false,
			config: map[string]any{
				"interface_enabled": false,
			},
			wantEnabled:   true,
			wantConfigKey: "interface_enabled",
			wantConfigVal: true,
		},
		{
			name:      "disable with enabled key only",
			ifaceName: "myiface",
			isEnabled: true,
			config: map[string]any{
				"enabled": true,
			},
			wantEnabled:   false,
			wantConfigKey: "enabled",
			wantConfigVal: false,
		},
		{
			name:          "no existing key defaults to enabled",
			ifaceName:     "myiface",
			isEnabled:     true,
			config:        map[string]any{},
			wantEnabled:   false,
			wantConfigKey: "enabled",
			wantConfigVal: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			item := &SelectableInterfaceItem{
				Name:      tt.ifaceName,
				IsEnabled: tt.isEnabled,
			}

			ToggleInterfaceEnabled(item, tt.config)

			if item.IsEnabled != tt.wantEnabled {
				t.Errorf("IsEnabled = %v, want %v", item.IsEnabled, tt.wantEnabled)
			}
			if tt.config[tt.wantConfigKey] != tt.wantConfigVal {
				t.Errorf("config[%q] = %v, want %v", tt.wantConfigKey, tt.config[tt.wantConfigKey], tt.wantConfigVal)
			}
		})
	}
}

func TestToggleConfirmationMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		ifaceName string
		isEnabled bool
		want      string
	}{
		{"disable msg", "myiface", true, "Are you sure you want to disable the myiface interface?"},
		{"enable msg", "myiface", false, "Are you sure you want to enable the myiface interface?"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ToggleConfirmationMessage(tt.ifaceName, tt.isEnabled)
			if got != tt.want {
				t.Errorf("ToggleConfirmationMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestToggleButtonLabel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		isEnabled bool
		want      string
	}{
		{"enabled shows Disable", true, "Disable"},
		{"disabled shows Enable", false, "Enable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ToggleButtonLabel(tt.isEnabled)
			if got != tt.want {
				t.Errorf("ToggleButtonLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}
