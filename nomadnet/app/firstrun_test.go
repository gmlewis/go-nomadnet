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

	"github.com/gmlewis/go-nomadnet/nomadnet/config"
)

// TestCreateDefaultConfigFirstrunParity verifies that CreateDefaultConfig writes
// the verbatim Python __default_nomadnet_config__ string to disk (byte-identical
// to what Python's createDefaultConfig writes), marks FirstRun, and applies the
// resulting config so its parsed fields match the default file's values.
func TestCreateDefaultConfigFirstrunParity(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	a := &App{
		ConfigDir:  dir,
		ConfigPath: filepath.Join(dir, "config"),
	}

	if err := a.CreateDefaultConfig(); err != nil {
		t.Fatalf("CreateDefaultConfig: %v", err)
	}

	if !a.FirstRun {
		t.Error("FirstRun = false, want true")
	}

	// The written file must be byte-identical to Python's default config.
	written, err := os.ReadFile(a.ConfigPath)
	if err != nil {
		t.Fatalf("read written config: %v", err)
	}
	if want := config.DefaultConfigText(); string(written) != want {
		t.Errorf("written config is not byte-identical to Python default:\n got len %d, want len %d", len(written), len(want))
	}

	// The applied config must reflect the default file's parsed values.
	if a.Config == nil {
		t.Fatal("a.Config is nil after CreateDefaultConfig")
	}
	if !a.Config.Client.EnableClient {
		t.Error("default file has enable_client = yes, but EnableClient is false")
	}
	if !a.Config.Client.CompactAnnounceStream {
		t.Error("default file has compact_announce_stream = yes, but CompactAnnounceStream is false")
	}
	if !a.Config.RRC.ShowGutters {
		t.Error("default file has show_gutters = yes, but ShowGutters is false")
	}
	// Default file: node announce_interval = 360 (minutes); config stores seconds.
	if got, want := a.Config.Node.AnnounceInterval, 360*60; got != want {
		t.Errorf("node announce_interval = %d, want %d (360 min in seconds)", got, want)
	}
	// App field is in minutes.
	if got, want := a.NodeAnnounceInterval, 360; got != want {
		t.Errorf("NodeAnnounceInterval = %d, want %d minutes", got, want)
	}
}

// TestCreateDefaultConfigIdempotent verifies a second CreateDefaultConfig call
// overwrites cleanly and still loads.
func TestCreateDefaultConfigIdempotent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	a := &App{ConfigDir: dir, ConfigPath: filepath.Join(dir, "config")}
	if err := a.CreateDefaultConfig(); err != nil {
		t.Fatalf("first: %v", err)
	}
	a.FirstRun = false
	if err := a.CreateDefaultConfig(); err != nil {
		t.Fatalf("second: %v", err)
	}
	if !a.FirstRun {
		t.Error("FirstRun = false after second call, want true")
	}
}
