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

	"github.com/gmlewis/go-reticulum/rrc"
)

// The Android appliance seeds a client's channel store before the client has ever run, and it
// writes it in the client's own format — CBOR, the one rrc.RRCManager.Save produces. The
// format therefore has two implementations, and this is the contract between them: the file in
// android/app/src/test/resources is the exact byte sequence the appliance's writer produces,
// pinned there by DefaultHubsTest, and the client's own reader is run against those same bytes
// here.
//
// A store the client cannot read is the worst kind of failure: RRCManager.Load reports it as
// an error at startup and carries on with no channels at all, so the appliance would show an
// empty Channels page and nothing anywhere would say why.
const applianceHubStoreFixture = "../../android/app/src/test/resources/default_hubs.cbor"

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
// channels, which is the whole point of writing it.
func TestApplianceHubStoreLoads(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile(applianceHubStoreFixture)
	if err != nil {
		t.Fatalf("reading %v: %v", applianceHubStoreFixture, err)
	}

	dir := tempDir(t)
	if err := os.WriteFile(filepath.Join(dir, "rrc_hubs"), data, 0o644); err != nil {
		t.Fatalf("writing the store: %v", err)
	}

	manager := rrc.NewManager(dir, nil)
	if err := manager.Load(); err != nil {
		t.Fatalf("the client could not read the store the appliance writes: %v", err)
	}

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

// TestApplianceHubStoreIsNotAnEmptyStore covers the failure this contract exists to prevent in
// the other direction: bytes the client cannot parse must not be mistaken for a store with no
// channels, because those two are indistinguishable from the operator's side of the screen.
func TestApplianceHubStoreIsNotAnEmptyStore(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	data, err := os.ReadFile(applianceHubStoreFixture)
	if err != nil {
		t.Fatalf("reading %v: %v", applianceHubStoreFixture, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rrc_hubs"), data, 0o644); err != nil {
		t.Fatalf("writing the store: %v", err)
	}

	manager := rrc.NewManager(dir, nil)
	if err := manager.Load(); err != nil {
		t.Fatalf("Load() = %v, want the fixture read", err)
	}
	if got := len(manager.HubsSnapshot()); got == 0 {
		t.Fatal("the fixture loaded as an empty store, which is what an unreadable one looks like")
	}
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
