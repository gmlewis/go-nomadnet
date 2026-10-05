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

// TestApplyLocationFromRaw pins how the optional [location] section becomes a
// reader position: any notation the geodesy engine understands works, and
// anything it cannot place leaves the client without a position rather than
// guessing one.
func TestApplyLocationFromRaw(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		raw       map[string]map[string]string
		wantKnown bool
		wantLat   float64
		wantLng   float64
		wantErr   bool
	}{
		{
			name:      "no section at all",
			raw:       map[string]map[string]string{},
			wantKnown: false,
		},
		{
			name:      "section with no fix",
			raw:       map[string]map[string]string{"location": {}},
			wantKnown: false,
		},
		{
			name:      "blank fix",
			raw:       map[string]map[string]string{"location": {"fix": "   "}},
			wantKnown: false,
		},
		{
			name:      "decimal degrees",
			raw:       map[string]map[string]string{"location": {"fix": "37.4220, -122.0841"}},
			wantKnown: true,
			wantLat:   37.4220,
			wantLng:   -122.0841,
		},
		{
			name:      "plus code",
			raw:       map[string]map[string]string{"location": {"fix": "849VCWC8+R9"}},
			wantKnown: true,
			wantLat:   37.4220625,
			wantLng:   -122.0840625,
		},
		{
			// CM87wk is the 5x2.5 minute subsquare whose centre is 37.4375 N,
			// 122.1250 W, the corner being 37.41667 N, 122.16667 W.
			name:      "maidenhead grid",
			raw:       map[string]map[string]string{"location": {"fix": "CM87wk"}},
			wantKnown: true,
			wantLat:   37.4375,
			wantLng:   -122.125,
		},
		{
			name:      "unplaceable text is refused",
			raw:       map[string]map[string]string{"location": {"fix": "somewhere"}},
			wantKnown: false,
			wantErr:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := DefaultConfig()
			c.Raw = tc.raw
			c.Apply()

			if c.Location.Known != tc.wantKnown {
				t.Fatalf("Location.Known = %v, want %v", c.Location.Known, tc.wantKnown)
			}
			if got := c.Location.Err != nil; got != tc.wantErr {
				t.Errorf("Location.Err set = %v, want %v (%v)", got, tc.wantErr, c.Location.Err)
			}
			if !tc.wantKnown {
				return
			}
			if diff := c.Location.Pos.Lat - tc.wantLat; diff > 1e-6 || diff < -1e-6 {
				t.Errorf("Location.Pos.Lat = %v, want %v", c.Location.Pos.Lat, tc.wantLat)
			}
			if diff := c.Location.Pos.Lng - tc.wantLng; diff > 1e-6 || diff < -1e-6 {
				t.Errorf("Location.Pos.Lng = %v, want %v", c.Location.Pos.Lng, tc.wantLng)
			}
		})
	}
}

// TestLoadLocationSection pins that the INI reader captures a hand-added
// [location] section, which is how the feature is configured: the section is
// deliberately absent from the default file so that file stays byte-identical
// to the one Python ships.
func TestLoadLocationSection(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	path := filepath.Join(dir, "config")
	contents := "[logging]\nloglevel = 4\n\n[location]\nfix = 51.5000, -0.1200\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if !c.Location.Known {
		t.Fatalf("Location.Known = false, want true (raw: %v)", c.Raw["location"])
	}
	if want := "51.5000, -0.1200"; c.Location.Fix != want {
		t.Errorf("Location.Fix = %q, want %q", c.Location.Fix, want)
	}
	if want := 51.5; c.Location.Pos.Lat != want {
		t.Errorf("Location.Pos.Lat = %v, want %v", c.Location.Pos.Lat, want)
	}
}

// TestDefaultConfigOmitsLocation pins that the shipped default configuration is
// untouched: Python has no [location] section, and the Go port writes the same
// file on first run.
func TestDefaultConfigOmitsLocation(t *testing.T) {
	t.Parallel()

	if got := DefaultConfig().Location; got.Known || got.Fix != "" || got.Err != nil {
		t.Errorf("DefaultConfig().Location = %+v, want the zero value", got)
	}
	if DefaultConfigText() == "" {
		t.Fatal("DefaultConfigText() is empty")
	}
	if strings.Contains(DefaultConfigText(), "[location]") {
		t.Error("the default config text contains a [location] section, which would break parity with Python")
	}
}
