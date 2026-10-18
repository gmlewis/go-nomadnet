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
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/gmlewis/go-reticulum/rns"
)

// routerSettingsConfig is a config whose LXMF settings are all deliberately
// different from the defaults, so a value that reaches the router can only have
// come from the file.
const routerSettingsConfig = `[client]
  max_accepted_size = 750
  accept_invalid_stamps = no
  required_stamp_cost = 16

[node]
  enable_node = no
  max_transfer_size = 512
  max_sync_size = 4096
  max_peers = 7
`

// TestRouterSettingsReachRouterFromConfig drives the settings through the real
// Init path and asserts each one lands on the router. Python passes them to its
// LXMRouter constructor and to the calls right after it
// (NomadNetworkApp.py:334-377); the port parsed every one into an App field and
// then never handed it over, so a configured transfer cap, peer cap or stamp
// requirement had no effect at all.
func TestRouterSettingsReachRouterFromConfig(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(routerSettingsConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	a := NewAppWithTransport(dir, WithTransport(ts), WithIdentity(id))
	if err := a.InitWithTransport(ts, id); err != nil {
		t.Fatalf("InitWithTransport: %v", err)
	}
	t.Cleanup(a.Shutdown)

	if a.Router == nil {
		t.Fatal("precondition: no LXMF router")
	}

	// The three per-transfer limits, in kilobytes: propagation_limit,
	// sync_limit and delivery_limit.
	if got, want := a.Router.PropagationPerTransferLimit(), 512.0; got != want {
		t.Errorf("propagation transfer limit = %v, want %v (max_transfer_size)", got, want)
	}
	if got, want := a.Router.PropagationPerSyncLimit(), 4096.0; got != want {
		t.Errorf("propagation sync limit = %v, want %v (max_sync_size)", got, want)
	}
	if got, want := a.Router.DeliveryPerTransferLimit(), 750.0; got != want {
		t.Errorf("delivery transfer limit = %v, want %v (max_accepted_size)", got, want)
	}

	if got, want := a.Router.MaxPeers(), 7; got != want {
		t.Errorf("max peers = %v, want %v", got, want)
	}

	// accept_invalid_stamps = no ⇒ enforce_stamps.
	if !a.Router.StampsEnforced() {
		t.Error("StampsEnforced = false, want true for accept_invalid_stamps = no")
	}

	// The stamp cost is passed at registration (NomadNetworkApp.py:355), so the
	// inbound requirement is in place before the first announce.
	if got, ok := a.Router.InboundStampCost(a.LXMFDest.Hash); !ok || got != 16 {
		t.Errorf("inbound stamp cost = %v (present %v), want 16", got, ok)
	}
}

// TestRouterSettingsAcceptInvalidStampsLeavesEnforcementOff pins the other side
// of the same switch, so the assertion above is about the configuration rather
// than about a constant.
func TestRouterSettingsAcceptInvalidStampsLeavesEnforcementOff(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	contents := "[client]\n  accept_invalid_stamps = yes\n\n[node]\n  enable_node = no\n"
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	a := NewAppWithTransport(dir, WithTransport(ts), WithIdentity(id))
	if err := a.InitWithTransport(ts, id); err != nil {
		t.Fatalf("InitWithTransport: %v", err)
	}
	t.Cleanup(a.Shutdown)

	if a.Router.StampsEnforced() {
		t.Error("StampsEnforced = true, want false for accept_invalid_stamps = yes")
	}
}

// TestApplyRouterSettingsNodeBranches covers the half of applyRouterSettings that
// only runs when the node is enabled: the message storage limit and the
// prioritised destinations, both of which were parsed and discarded before.
// Python gates the pair on `if self.enable_node:` (NomadNetworkApp.py:369-377).
func TestApplyRouterSettingsNodeBranches(t *testing.T) {
	t.Parallel()

	a := newTestAppWithRouter(t)

	valid := "00112233445566778899aabbccddeeff"
	a.EnableNode = true
	a.MessageStorageLimit = 321
	a.PrioritisedLXMF = []string{valid, "not-a-hash", "00112233"}

	a.applyRouterSettings()

	if got, want := a.Router.MessageStorageLimit(), 321.0; got != want {
		t.Errorf("message storage limit = %v, want %v", got, want)
	}
	validHash, err := hex.DecodeString(valid)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Router.IsPrioritised(validHash) {
		t.Error("configured prioritised destination was not prioritised on the router")
	}
	// Malformed entries are logged and skipped, not applied: Python rejects
	// anything that is not a 16-byte destination hex (NomadNetworkApp.py:372-377).
	for _, bad := range []string{"not-a-hash", "00112233"} {
		badHash, err := hex.DecodeString(bad)
		if err != nil {
			continue
		}
		if a.Router.IsPrioritised(badHash) {
			t.Errorf("invalid destination %q was prioritised", bad)
		}
	}
}

// TestApplyRouterSettingsSkipsNodeBranchesWhenDisabled pins the gate: with the
// node disabled Python never calls set_message_storage_limit or prioritise, so a
// Go run that applied them anyway would report node-only settings as active on a
// client that hosts nothing.
func TestApplyRouterSettingsSkipsNodeBranchesWhenDisabled(t *testing.T) {
	t.Parallel()

	a := newTestAppWithRouter(t)
	before := a.Router.MessageStorageLimit()

	a.EnableNode = false
	a.MessageStorageLimit = 321
	a.PrioritisedLXMF = []string{"00112233445566778899aabbccddeeff"}

	a.applyRouterSettings()

	if got := a.Router.MessageStorageLimit(); got != before {
		t.Errorf("message storage limit = %v with the node disabled, want the unchanged %v", got, before)
	}
	validHash, err := hex.DecodeString("00112233445566778899aabbccddeeff")
	if err != nil {
		t.Fatal(err)
	}
	if a.Router.IsPrioritised(validHash) {
		t.Error("destination prioritised although the node is disabled")
	}
}

// TestStaticPeerHashesValidation pins the decode rule for [node] static_peers:
// each entry is hex and must name exactly a TRUNCATED_HASHLENGTH//8 (16) byte
// destination; anything else is dropped with a log line, mirroring Python's
// per-entry try/except (NomadNetworkApp.py:334-341).
func TestStaticPeerHashesValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		peer  string
		valid bool
	}{
		{"sixteen bytes", "00112233445566778899aabbccddeeff", true},
		{"uppercase hex", "00112233445566778899AABBCCDDEEFF", true},
		{"surrounding space", "  00112233445566778899aabbccddeeff  ", true},
		{"too short", "00112233", false},
		{"too long", "00112233445566778899aabbccddeeff00", false},
		{"not hex", "zzzz", false},
		{"empty", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			a := NewApp(tempDir(t), "", false, false)
			a.Logger = rns.NewLogger()
			a.StaticPeers = []string{tc.peer}

			got := a.staticPeerHashes()
			if tc.valid {
				if len(got) != 1 {
					t.Fatalf("staticPeerHashes(%q) = %v entries, want 1", tc.peer, len(got))
				}
				want, err := hex.DecodeString("00112233445566778899aabbccddeeff")
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got[0], want) {
					t.Errorf("staticPeerHashes(%q) = %x, want %x", tc.peer, got[0], want)
				}
				return
			}
			if len(got) != 0 {
				t.Errorf("staticPeerHashes(%q) = %v entries, want none", tc.peer, len(got))
			}
		})
	}
}

// TestRRCMaxAcceptedResourceSizeReachesTheManager pins the wiring of
// [rrc] max_accepted_resource_size through to the RRC manager, which is where the
// hub reads the cap for every incoming resource advertisement. The port hardcoded
// 262144 at that call site, so a configured cap had no effect on the client.
func TestRRCMaxAcceptedResourceSizeReachesTheManager(t *testing.T) {
	t.Parallel()

	const contents = "[rrc]\n  max_accepted_resource_size = 512\n\n[node]\n  enable_node = no\n"
	dir := tempDir(t)
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	a := NewAppWithTransport(dir, WithTransport(ts), WithIdentity(id))
	if err := a.InitWithTransport(ts, id); err != nil {
		t.Fatalf("InitWithTransport: %v", err)
	}
	t.Cleanup(a.Shutdown)

	// The config value is kilobytes; the manager works in bytes.
	if got, want := a.RRCMaxAcceptedResourceBytes, 512*1024; got != want {
		t.Errorf("App cap = %v bytes, want %v", got, want)
	}
	if got, want := a.RRC.MaxAcceptedResourceBytes(), 512*1024; got != want {
		t.Errorf("manager cap = %v bytes, want %v (the configured 512 KB)", got, want)
	}
}
