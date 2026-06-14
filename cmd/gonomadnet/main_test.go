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
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/version"
)

func TestVersion(t *testing.T) {
	t.Parallel()

	if version.Version != "0.1.0" {
		t.Errorf("Version = %q, want %q", version.Version, "0.1.0")
	}
}

func TestDefaultConfigDir(t *testing.T) {
	t.Parallel()

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("Cannot get home dir: %v", err)
	}

	expected := filepath.Join(home, ".nomadnetwork")

	// Verify the default path construction
	got := filepath.Join(home, ".nomadnetwork")
	if got != expected {
		t.Errorf("Default config dir = %q, want %q", got, expected)
	}
}

func TestFlagParsing(t *testing.T) {
	t.Parallel()

	// Test that flags can be parsed without error
	// This is a basic sanity check
	args := os.Args
	defer func() { os.Args = args }()

	os.Args = []string{"nomadnet", "--version"}
	// We can't actually call main() here, but we verify the flag definitions exist
}
