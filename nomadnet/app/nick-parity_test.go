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
	"strings"
	"testing"

	"github.com/gmlewis/go-nomadnet/nomadnet/config"
	"github.com/gmlewis/go-nomadnet/nomadnet/directory"
	"github.com/gmlewis/go-nomadnet/nomadnet/peersettings"
	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/rrc"
)

// peerNameApp builds an App wired the way Init bootstraps it (config applied,
// peer settings loaded, RRC nickname seeded) but without any RNS transport or
// background jobs, so a test can exercise the display-name/nick flow offline.
func peerNameApp(t *testing.T, dir string) *App {
	t.Helper()

	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	a := NewAppWithTransport(dir, WithTransport(ts), WithIdentity(id))
	a.setupPaths()
	if err := os.MkdirAll(a.StoragePath, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(a.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	a.Config = cfg
	a.applyConfig(cfg)
	a.Dir = directory.New()
	a.RRC = rrc.NewManager(a.StoragePath, nil)
	a.RRC.SetHistoryConfig(a.RRCHistoryPerRoomCap, a.RRCFilterLoadedHistory, a.RRCEphemeralNotices)
	if err := a.RRC.Load(); err != nil {
		t.Fatal(err)
	}
	a.loadPeerSettings()
	a.seedRRCNickname()
	return a
}

// writeNodeNameConfig seeds the app's config file with the given node_name so
// applyConfig picks it up (the ~/.nomadnetwork/config edit the Bug#1 report
// describes).
func writeNodeNameConfig(t *testing.T, dir, nodeName string) {
	t.Helper()

	cfgPath := filepath.Join(dir, "config")
	body := "[node]\nnode_name = " + nodeName + "\n\n[client]\nannounce_interval = 360\n"
	if err := os.WriteFile(cfgPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestNickFollowsPeerSettingsNotNodeName pins Bug#1's parity resolution
// (owner decision: keep Python parity): the chat Nick is the peer settings
// display_name (Python RRCManager.get_nickname, RRC.py:1286-1294 reading
// peer_settings["display_name"]), and the config's [node] node_name only
// names the hosted node (Python Node.py:28-36). Changing node_name in
// ~/.nomadnetwork/config and restarting must NOT change the Nick; changing
// the peer display name (the TUI's Local Peer Info Name field) must.
func TestNickFollowsPeerSettingsNotNodeName(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	writeNodeNameConfig(t, dir, "go-nomadnet on MacM2Max")

	a := peerNameApp(t, dir)

	// Seed the peer settings the way the TUI's Name field + Save does, then
	// simulate a restart by loading the persisted settings fresh.
	a.SetDisplayName("Go port of NomadNet on Mac M2 Max")
	ps, err := peersettings.Load(a.PeerSettingsPath, a.AnnounceInterval)
	if err != nil {
		t.Fatal(err)
	}
	if ps.DisplayName != "Go port of NomadNet on Mac M2 Max" {
		t.Fatalf("peersettings display_name = %q, want the saved name", ps.DisplayName)
	}

	// After the restart the Nick must equal the peer settings display_name.
	if got := a.GetDisplayName(); got != "Go port of NomadNet on Mac M2 Max" {
		t.Fatalf("GetDisplayName after restart = %q, want the persisted peer settings name", got)
	}
	if got := a.RRC.GetNickname(); got != "Go port of NomadNet on Mac M2 Max" {
		t.Errorf("RRC nick after restart = %q, want the peer settings display_name", got)
	}

	// The node_name edit from the report must not leak into the Nick.
	if got := a.NodeName; got != "go-nomadnet on MacM2Max" {
		t.Errorf("NodeName = %q, want the config's node_name", got)
	}
	if got := a.RRC.GetNickname(); got == "go-nomadnet on MacM2Max" {
		t.Error("the chat Nick followed [node] node_name; Python keeps it on the peer settings display_name")
	}

	// node_name still names the hosted node (Python Node.py:28-36).
	if got := a.ResolveNodeName(); got != "go-nomadnet on MacM2Max" {
		t.Errorf("ResolveNodeName = %q, want the config's node_name", got)
	}

	// Renaming the peer (the TUI's Name field path) updates the Nick live,
	// mirroring Python where get_effective_nick reads the display name per
	// hello/send (RRC.py:453-455, 535-537).
	a.SetDisplayName("Renamed Peer")
	if got := a.RRC.GetNickname(); got != "Renamed Peer" {
		t.Errorf("RRC nick after rename = %q, want %q", got, "Renamed Peer")
	}
	if got := a.ResolveNodeName(); got != "go-nomadnet on MacM2Max" {
		t.Errorf("ResolveNodeName after peer rename = %q, want the unchanged node_name", got)
	}
}

// TestNickEmptyPeerSettingsFallsBack pins the load path's behavior when the
// peer settings file is missing entirely (fresh install): the display name
// defaults to Python's "Anonymous Peer" (NomadNetworkApp.py:288) and the Nick
// follows it — not the node name, and not an empty string that would drop
// K_NICK from every chat envelope.
func TestNickEmptyPeerSettingsFallsBack(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	writeNodeNameConfig(t, dir, "Configured Node")
	a := peerNameApp(t, dir)

	if got := a.GetDisplayName(); got != "Anonymous Peer" {
		t.Errorf("GetDisplayName on a fresh install = %q, want %q", got, "Anonymous Peer")
	}
	if got := a.RRC.GetNickname(); got != "Anonymous Peer" {
		t.Errorf("RRC nick on a fresh install = %q, want %q", got, "Anonymous Peer")
	}
	if !strings.Contains(a.ResolveNodeName(), "Configured Node") {
		t.Errorf("ResolveNodeName = %q, want the configured node_name", a.ResolveNodeName())
	}
}
