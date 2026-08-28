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

	"github.com/gmlewis/go-reticulum/rns"
)

// TestInitWithTransportHonorsLoglevelConfig verifies that the [logging]
// loglevel key in the NomadNet config drives the RNS logger level, matching
// Python NomadNetworkApp.applyConfig (NomadNetworkApp.py:804-809), which sets
// RNS.loglevel = int(value) clamped to 0..7. Python's default config ships
// loglevel = 4 (info); the Go config package applies the same default and
// clamping, so the App must forward it to the logger instead of hardcoding
// LogNotice. This is what lets an operator enable debug/extreme logging
// without a code change.
func TestInitWithTransportHonorsLoglevelConfig(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		loglevel string
		want     int
	}{
		{name: "debug", loglevel: "6", want: 6},
		{name: "info (default value)", loglevel: "4", want: 4},
		{name: "critical", loglevel: "0", want: 0},
		{name: "clamped high", loglevel: "9", want: 7},
		{name: "no loglevel key keeps default 4", loglevel: "", want: 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := tempDir(t)
			// Private config with enable_node = no so InitWithTransport (which
			// mirrors the production initRNS path and calls startNode) does not
			// auto-start a node (see writeTestNomadNetConfig).
			contents := "[node]\nenable_node = no\n"
			if tc.loglevel != "" {
				contents += "[logging]\nloglevel = " + tc.loglevel + "\n"
			}
			if err := os.WriteFile(filepath.Join(dir, "config"), []byte(contents), 0o644); err != nil {
				t.Fatalf("writing config: %v", err)
			}
			ts := rns.NewTransportSystem(nil)
			id, err := rns.NewIdentity(true, nil)
			if err != nil {
				t.Fatal(err)
			}

			a := NewAppWithTransport(dir, WithTransport(ts), WithIdentity(id))
			if err := a.InitWithTransport(ts, id); err != nil {
				t.Fatalf("InitWithTransport error: %v", err)
			}
			defer a.Shutdown()

			if got := a.Logger.GetLogLevel(); got != tc.want {
				t.Errorf("Logger loglevel = %v, want %v", got, tc.want)
			}
		})
	}
}
