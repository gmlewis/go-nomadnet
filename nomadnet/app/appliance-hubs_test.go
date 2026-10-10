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

	"github.com/gmlewis/go-reticulum/rrc"
)

// The Android appliance seeds a client's channel store before the client has ever run, and it
// writes it in the client's own format — CBOR, the one rrc.RRCManager.Save produces. The
// format therefore has two implementations, and this is the contract between them: the files in
// android/app/src/test/resources are the exact byte sequences the appliance's writer produces,
// pinned there by DefaultHubsTest, and the client's own reader is run against those same bytes
// here.
//
// A store the client cannot read is the worst kind of failure: RRCManager.Load reports it as
// an error at startup and carries on with no channels at all, so the appliance would show an
// empty Channels page and nothing anywhere would say why.
const applianceHubStoreFixture = "../../android/app/src/test/resources/default_hubs.cbor"

// applianceHubNoLocalFixture is the store the appliance writes before its own hub has a
// destination of its own to list: the two public channels and nothing else. It is the seed the
// appliance then completes into applianceHubStoreFixture once the local hub appears, so both
// byte sequences have to be readable by the client.
const applianceHubNoLocalFixture = "../../android/app/src/test/resources/default_hubs_no_local.cbor"

// fixtureHubs is what the fixture says, in the order it says it.
var fixtureHubs = []struct {
	name  string
	hash  string
	rooms []string
}{
	{name: "RNS Community", hash: "28c7c1a68c735693aa8e6b8193ed44b2", rooms: []string{"general"}},
	{name: "gonomadnet Public Hub", hash: "a012129c10205c0b9441fcd2b755b2a7", rooms: []string{"general"}},
	{name: "appliance-hub", hash: "00112233445566778899aabbccddeeff", rooms: []string{"general"}},
}

// TestApplianceHubStoreLoads covers the store the appliance writes reaching the client as
// channels, which is the whole point of writing it. These are the upgraded bytes — the store
// the appliance completes its untouched two-hub seed into, which DefaultHubsTest asserts is
// byte-identical to this fixture.
func TestApplianceHubStoreLoads(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(applianceHubStoreFixture)
	if err != nil {
		t.Fatalf("reading %v: %v", applianceHubStoreFixture, err)
	}
	manager := loadHubStore(t, data)

	hubs := manager.HubsSnapshot()
	if len(hubs) != len(fixtureHubs) {
		t.Fatalf("the store loaded %v channels, want %v", len(hubs), len(fixtureHubs))
	}
	for i, want := range fixtureHubs {
		hub := hubs[i]
		if hub.Name != want.name {
			t.Errorf("channel %v is named %q, want %q", i, hub.Name, want.name)
		}
		if got := hexString(hub.HubHash); got != want.hash {
			t.Errorf("%v's destination is %v, want %v", want.name, got, want.hash)
		}
		// The destination name is how the client finds the hub's announce, so a store that
		// left it out would list a channel that could never connect.
		if hub.DestName != "rrc.hub" {
			t.Errorf("%v announces under %q, want %q", want.name, hub.DestName, "rrc.hub")
		}
		for _, room := range want.rooms {
			_, _, _, rooms := hub.Snapshot()
			if !rooms[room] {
				t.Errorf("%v did not join %v, so the appliance's channels open empty", want.name, room)
			}
		}
		// An appliance is meant to look after its own channels: the seed turns the three auto
		// flags on, and a store that lost them would list channels that never reconnect.
		if !hub.AutoReconnect || !hub.AutoList || !hub.AutoWho {
			t.Errorf("%v loaded with auto_reconnect=%v auto_list=%v auto_who=%v, want all true",
				want.name, hub.AutoReconnect, hub.AutoList, hub.AutoWho)
		}
	}
}

// TestApplianceTwoHubSeedStoreLoads covers the seed itself: the store the appliance writes on a
// first console open, before its own hub has published a destination. The appliance completes
// this file in place on a later open, and the client reads it in between, so the two-public-hub
// store has to load as two channels — not as an empty one, and not as a third channel invented
// for a destination nobody has.
func TestApplianceTwoHubSeedStoreLoads(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(applianceHubNoLocalFixture)
	if err != nil {
		t.Fatalf("reading %v: %v", applianceHubNoLocalFixture, err)
	}
	manager := loadHubStore(t, data)

	hubs := manager.HubsSnapshot()
	if len(hubs) != 2 {
		t.Fatalf("the two-hub seed loaded %v channels, want 2", len(hubs))
	}
	for i, want := range fixtureHubs[:2] {
		if hubs[i].Name != want.name {
			t.Errorf("channel %v is named %q, want %q", i, hubs[i].Name, want.name)
		}
		if got := hexString(hubs[i].HubHash); got != want.hash {
			t.Errorf("%v's destination is %v, want %v", want.name, got, want.hash)
		}
	}
	for _, hub := range hubs {
		if hub.Name == "appliance-hub" {
			t.Error("the seed lists the appliance's own hub before it has a destination")
		}
	}
}

// TestApplianceHubStoreIsNotAnEmptyStore covers the failure this contract exists to prevent in
// the other direction: bytes the client cannot parse must not be mistaken for a store with no
// channels, because those two are indistinguishable from the operator's side of the screen.
func TestApplianceHubStoreIsNotAnEmptyStore(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(applianceHubStoreFixture)
	if err != nil {
		t.Fatalf("reading %v: %v", applianceHubStoreFixture, err)
	}
	manager := loadHubStore(t, data)
	if got := len(manager.HubsSnapshot()); got == 0 {
		t.Fatal("the fixture loaded as an empty store, which is what an unreadable one looks like")
	}
}

// TestApplianceHubStoreMatchesClientWriter pins the assumption the appliance's byte rule rests
// on: the two-hub seed it compares a store against is exactly what the client's own writer would
// write for those two channels. That is what lets the appliance recognise its own untouched file
// — and it is also the limit of the rule, since a store the client saved after the operator
// removed the third channel is byte-identical to the seed and cannot be told apart from it.
func TestApplianceHubStoreMatchesClientWriter(t *testing.T) {
	t.Parallel()

	fixture, err := os.ReadFile(applianceHubStoreFixture)
	if err != nil {
		t.Fatalf("reading %v: %v", applianceHubStoreFixture, err)
	}

	dir := tempDir(t)
	manager := rrc.NewManager(dir, nil)
	for _, want := range fixtureHubs {
		raw, err := hex.DecodeString(want.hash)
		if err != nil {
			t.Fatalf("decoding %v: %v", want.hash, err)
		}
		hub := manager.AddHub(raw, "rrc.hub", want.name)
		for _, room := range want.rooms {
			hub.Rooms[room] = true
		}
		hub.AutoReconnect, hub.AutoList, hub.AutoWho = true, true, true
	}
	if err := manager.Save(); err != nil {
		t.Fatalf("Save() = %v", err)
	}
	saved, err := os.ReadFile(filepath.Join(dir, "rrc_hubs"))
	if err != nil {
		t.Fatalf("reading the saved store: %v", err)
	}
	if !bytes.Equal(saved, fixture) {
		t.Errorf("the client's writer does not reproduce the appliance's store:\n got %x\nwant %x",
			saved, fixture)
	}
}

// loadHubStore writes data as the client's store in a fresh directory and reads it back with the
// client's own reader, which is what the appliance's bytes have to satisfy.
func loadHubStore(t *testing.T, data []byte) *rrc.RRCManager {
	t.Helper()

	dir := tempDir(t)
	if err := os.WriteFile(filepath.Join(dir, "rrc_hubs"), data, 0o644); err != nil {
		t.Fatalf("writing the store: %v", err)
	}
	manager := rrc.NewManager(dir, nil)
	if err := manager.Load(); err != nil {
		t.Fatalf("the client could not read the store the appliance writes: %v", err)
	}
	return manager
}

// hexString renders a destination the way the appliance writes it.
func hexString(b []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, c := range b {
		out = append(out, digits[c>>4], digits[c&0xf])
	}
	return string(out)
}
