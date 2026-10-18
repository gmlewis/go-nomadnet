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

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestIntroTextFromConfig pins [textui] intro_text, the title the startup splash
// renders. Python reads config["textui"]["intro_text"] and defaults it to
// "Nomad Network" when the key is absent (NomadNetworkApp.py:919-920); the value
// is consumed by IntroDisplay (ui/textui/Extras.py:8). The port had no field at
// all and rendered a hardcoded title, so the key did nothing.
//
// The key is deliberately NOT written back by Save: Python's createDefaultConfig
// writes the shipped default text verbatim and never rewrites the file
// (NomadNetworkApp.py:790-798), and that text has no intro_text entry, so adding
// one to the written config would be a new divergence rather than a fix.
func TestIntroTextFromConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		textUI string
		want   string
	}{
		{"explicit value", "intro_text = My Own Net\n", "My Own Net"},
		{"surrounding space is trimmed", "intro_text =   Spaced Net   \n", "Spaced Net"},
		{"absent uses the reference default", "", "Nomad Network"},
		{"blank falls back to the default", "intro_text =    \n", "Nomad Network"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := tempDir(t)
			contents := "[textui]\n" + tc.textUI
			path := filepath.Join(dir, "config")
			if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
				t.Fatal(err)
			}

			c, err := Load(path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if c.TextUI.IntroText != tc.want {
				t.Errorf("IntroText = %q, want %q", c.TextUI.IntroText, tc.want)
			}
		})
	}
}

// TestIntroTextNotWrittenBySave pins the other half of the rule above: the key
// must not appear in a saved config, because the reference never writes it.
func TestIntroTextNotWrittenBySave(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	path := filepath.Join(dir, "config")
	if err := os.WriteFile(path, []byte("[textui]\nintro_text = My Own Net\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := Save(c, path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); strings.Contains(got, "intro_text") {
		t.Errorf("Save wrote intro_text, which the reference config never carries:\n%v", got)
	}
}
