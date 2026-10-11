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

package app

import (
	"os"
	"path/filepath"
	"testing"
)

// writeConfig writes a config file into a fresh temp dir and returns the App
// pointing at it.
func writeConfig(t *testing.T, content string) *App {
	t.Helper()
	dir := tempDir(t)
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return &App{RNSConfigDir: dir}
}

// TestTransportEnabled pins the read that gates the Add/Edit form's
// transport-mode options (Python app.rns.transport_enabled(), Interfaces.py:1580).
// The option belongs to the [reticulum] section, and only that section's key
// counts — a look-alike key under another section must not turn it on.
func TestTransportEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  string
		want bool
	}{
		{
			name: "yes",
			cfg:  "[reticulum]\n  enable_transport = yes\n",
			want: true,
		},
		{
			name: "no",
			cfg:  "[reticulum]\n  enable_transport = no\n",
			want: false,
		},
		{
			name: "absent",
			cfg:  "[reticulum]\n  share_instance = yes\n",
			want: false,
		},
		{
			name: "another sections key does not count",
			cfg:  "[logging]\n  enable_transport = yes\n",
			want: false,
		},
		{
			name: "a subsection header does not end the section",
			cfg:  "[reticulum]\n  [[interfaces]]\n  enable_transport = yes\n",
			want: true,
		},
		{
			name: "no config at all",
			cfg:  "",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := writeConfig(t, tt.cfg).TransportEnabled(); got != tt.want {
				t.Errorf("TransportEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestInterfaceExists pins the duplicate-name check the Add/Edit form uses
// (Python on_save against app.rns.config['interfaces'], Interfaces.py:1887).
func TestInterfaceExists(t *testing.T) {
	t.Parallel()

	app := writeConfig(t, `[interfaces]
  [[hub]]
    type = TCPClientInterface
  [[Other]]
    type = AutoInterface
`)

	if !app.InterfaceExists("hub") {
		t.Error("InterfaceExists(hub) = false, want true")
	}
	if !app.InterfaceExists("Other") {
		t.Error("InterfaceExists(Other) = false, want true")
	}
	if app.InterfaceExists("missing") {
		t.Error("InterfaceExists(missing) = true, want false")
	}
	// A section of the same name outside [interfaces] is not an interface.
	other := writeConfig(t, "[rrc]\n  [[hub]]\n    type = AutoInterface\n")
	if other.InterfaceExists("hub") {
		t.Error("InterfaceExists(hub) = true for an [[hub]] outside [interfaces]")
	}
}
