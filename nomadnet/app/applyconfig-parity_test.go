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
	"embed"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/config"
)

//go:embed testdata/applyconfig_parity.json
var appCfgFS embed.FS

type appCfgFixture struct {
	Variants map[string]string         `json:"variants"`
	Golden   map[string]map[string]any `json:"golden"`
}

// TestApplyConfigAppFieldsParity verifies that app.applyConfig maps the loaded
// config.Config onto App fields with the same values Python's
// NomadNetworkApp.applyConfig produces. It focuses on the app-level fields
// whose conversion differs from the raw config struct:
//   - RRCEphemeralNotices: Python seconds (value*60); Go config holds minutes,
//     so applyConfig must multiply by 60.
//   - DisablePropagation: must honor the [node] disable_propagation key rather
//     than being hardcoded true.
//   - NodeAnnounceInterval: Python minutes; Go config holds seconds, so
//     applyConfig must divide by 60.
//
// Golden values come from the same Python applyConfig capture as the
// config-package parity test (testdata/applyconfig_parity.json).
func TestApplyConfigAppFieldsParity(t *testing.T) {
	t.Parallel()

	data, err := appCfgFS.ReadFile("testdata/applyconfig_parity.json")
	if err != nil {
		t.Fatalf("read embed: %v", err)
	}
	var fx appCfgFixture
	if err := json.Unmarshal(data, &fx); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for name, text := range fx.Variants {
		golden := fx.Golden[name]
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			path := filepath.Join(dir, "nomadnet.conf")
			if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
				t.Fatalf("write conf: %v", err)
			}
			cfg, err := config.Load(path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			a := &App{}
			a.applyConfig(cfg)

			hasSection := func(sec string) bool {
				return strings.Contains(text, "["+sec+"]")
			}
			hasKey := func(sec, key string) bool {
				_, ok := cfg.Raw[sec][key]
				return ok
			}

			if hasSection("rrc") && hasKey("rrc", "ephemeral_notices") {
				// Python stores ephemeral_notices in seconds; the App field is
				// documented in seconds.
				want := gIntApp(golden["rrc_ephemeral_notices"])
				if a.RRCEphemeralNotices != want {
					t.Errorf("RRCEphemeralNotices = %v (sec), want %v (sec)", a.RRCEphemeralNotices, want)
				}
			}

			if hasSection("node") {
				if hasKey("node", "disable_propagation") {
					want := gBoolApp(golden["disable_propagation"])
					if a.DisablePropagation != want {
						t.Errorf("DisablePropagation = %v, want %v", a.DisablePropagation, want)
					}
				}
				if hasKey("node", "announce_interval") {
					// Python stores node_announce_interval in minutes; the App
					// field is documented in minutes.
					want := gIntApp(golden["node_announce_interval"])
					if a.NodeAnnounceInterval != want {
						t.Errorf("NodeAnnounceInterval = %v (min), want %v (min)", a.NodeAnnounceInterval, want)
					}
				}
			}
		})
	}
}

func gBoolApp(v any) bool {
	if v == nil {
		return false
	}
	b, _ := v.(bool)
	return b
}

func gIntApp(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}
