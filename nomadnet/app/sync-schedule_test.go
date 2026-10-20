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
	"testing"
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/peersettings"
)

// The periodic LXMF sync must honour the persisted last-sync time across
// launches. Python's job loop compares against the persisted value directly
// (NomadNetworkApp.py:464, "now > self.peer_settings['last_lxmf_sync'] +
// self.lxmf_sync_interval", with the field defaulted to 0 at :263-264 and
// loaded from disk at startup). The Go app keeps a separate in-memory clock, so
// it must be seeded from the persisted value at load or the client re-syncs on
// every start.
func TestLoadPeerSettingsSeedsLastLXMFSync(t *testing.T) {
	t.Parallel()

	a, _ := testAppWithTransport(t)

	// Persisted state says: synced one minute ago, with a 6 h interval.
	persisted := peersettings.DefaultSettings(a.AnnounceInterval)
	persisted.LastLXMFSync = int(time.Now().Add(-time.Minute).Unix())
	if err := peersettings.Save(persisted, a.PeerSettingsPath); err != nil {
		t.Fatalf("Save: %v", err)
	}

	a.loadPeerSettings()

	if a.LastLXMFSync.IsZero() {
		t.Fatal("LastLXMFSync is the zero time after loading peer settings; the persisted last_lxmf_sync was ignored")
	}
	if got, want := a.LastLXMFSync.Unix(), int64(persisted.LastLXMFSync); got != want {
		t.Errorf("LastLXMFSync = %v, want %v", got, want)
	}
	if a.lxmfSyncDue(time.Now()) {
		t.Error("lxmfSyncDue = true one minute after a sync with a 6 h interval; want false")
	}
}

func TestLXMFSyncDue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		interval int
		lastSync time.Duration // how long ago the last sync happened
		periodic bool
		want     bool
	}{
		{"recent sync is not due", 6 * 3600, time.Minute, true, false},
		{"stale sync is due", 6 * 3600, 7 * time.Hour, true, true},
		{"one minute short of the interval is not due", 6 * 3600, 6*time.Hour - time.Minute, true, false},
		{"one second past the interval is due", 6 * 3600, 6*time.Hour + time.Second, true, true},
		{"never synced is due", 6 * 3600, 0, true, true},
		{"disabled is never due", 6 * 3600, 7 * time.Hour, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a, _ := testAppWithTransport(t)
			a.LXMFSyncInterval = tc.interval
			a.PeriodicLXMFSync = tc.periodic
			if tc.lastSync == 0 {
				a.LastLXMFSync = time.Time{}
			} else {
				a.LastLXMFSync = time.Now().Add(-tc.lastSync)
			}
			if got := a.lxmfSyncDue(time.Now()); got != tc.want {
				t.Errorf("lxmfSyncDue(interval=%v, lastSync=%v, periodic=%v) = %v, want %v",
					tc.interval, tc.lastSync, tc.periodic, got, tc.want)
			}
		})
	}
}
