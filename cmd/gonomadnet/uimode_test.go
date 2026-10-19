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

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResolveUIMode pins Python's UI selection (NomadNetworkApp.py:432-443,
// ui/__init__.py:24-45): --daemon forces the daemon; otherwise
// user_interface picks text/none; the unimplemented UIs and an absent or
// unknown value are fatal; --textui only cancels --daemon.
func TestResolveUIMode(t *testing.T) {
	t.Parallel()

	writeConfig := func(t *testing.T, body string) string {
		t.Helper()
		dir := tempDir(t)
		if err := os.WriteFile(filepath.Join(dir, "config"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return dir
	}

	tests := []struct {
		name    string
		config  string
		daemon  bool
		textUI  bool
		want    bool
		wantErr string
	}{
		{name: "missing config runs the text UI", want: false},
		{name: "text", config: "[client]\nuser_interface = text\n", want: false},
		{name: "none runs the daemon", config: "[client]\nuser_interface = none\n", want: true},
		{name: "absent key is invalid", config: "[client]\n", wantErr: "invalid UI mode"},
		{name: "unknown value is invalid", config: "[client]\nuser_interface = bogus\n", wantErr: "invalid UI mode: bogus"},
		{name: "menu is not implemented", config: "[client]\nuser_interface = menu\n", wantErr: "Menu UI not implemented"},
		{name: "graphical is not implemented", config: "[client]\nuser_interface = graphical\n", wantErr: "Graphical UI not implemented"},
		{name: "web is not implemented", config: "[client]\nuser_interface = web\n", wantErr: "Web UI not implemented"},
		{name: "daemon overrides a text config", config: "[client]\nuser_interface = text\n", daemon: true, want: true},
		{name: "daemon overrides an invalid config", config: "[client]\n", daemon: true, want: true},
		{name: "textui cancels daemon and uses the text config", config: "[client]\nuser_interface = text\n", daemon: true, textUI: true, want: false},
		{name: "textui does not override user_interface none", config: "[client]\nuser_interface = none\n", textUI: true, want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := tempDir(t)
			if tc.config != "" {
				dir = writeConfig(t, tc.config)
			}
			got, err := resolveUIMode(dir, tc.daemon, tc.textUI)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("resolveUIMode error = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveUIMode: %v", err)
			}
			if got != tc.want {
				t.Errorf("resolveUIMode daemon = %v, want %v", got, tc.want)
			}
		})
	}
}
