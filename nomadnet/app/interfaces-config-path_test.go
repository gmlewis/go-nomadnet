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
	"path/filepath"
	"testing"
)

// TestRNSConfigPath pins the RNS config-file path resolution that the
// Interfaces "Open Text Editor" (C-w) action edits — matching Python's
// open_config_editor (Interfaces.py:3160), which edits self.app.rns.configpath.
//
//   - Once RNS is initialized, the resolved path comes from the Reticulum
//     itself (Reticulum.ConfigPath); see TestAppInit for that end-to-end check.
//   - Before RNS is initialized, an explicit -rnsconfig dir is used: the
//     config lives at <dir>/config.
//   - When neither is set (RNS not yet initialized, no -rnsconfig) it returns
//     "" so the wiring can surface a "not ready" message instead of editing
//     nothing.
func TestRNSConfigPath(t *testing.T) {
	t.Run("explicit rns config dir before init", func(t *testing.T) {
		a := &App{RNSConfigDir: "/tmp/rnscfg-example"}
		want := filepath.Join("/tmp/rnscfg-example", "config")
		if got := a.RNSConfigPath(); got != want {
			t.Errorf("RNSConfigPath() = %q, want %q", got, want)
		}
	})

	t.Run("not ready returns empty", func(t *testing.T) {
		a := &App{}
		if got := a.RNSConfigPath(); got != "" {
			t.Errorf("RNSConfigPath() = %q, want empty before RNS init", got)
		}
	})
}
