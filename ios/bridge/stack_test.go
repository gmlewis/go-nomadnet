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

package bridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dirs is the three directories the appliance reads, as the app's own container names them.
func dirs(container string) ApplianceDirs {
	return ApplianceDirs{
		ClientConfig: filepath.Join(container, "nomadnetwork"),
		ClientRNS:    filepath.Join(container, "client-reticulum"),
		TransportRNS: filepath.Join(container, "reticulum"),
	}
}

func TestTheSubsystemsStartInTheOrderTheyDependOnEachOther(t *testing.T) {
	t.Parallel()
	subs := dirs("/container").Subsystems()

	names := make([]string, 0, len(subs))
	for _, sub := range subs {
		names = append(names, sub.Name())
	}
	want := []string{"transport", "hub", "bot"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("the subsystems are %v, want %v", names, want)
	}
}

func TestTheHubLivesInTheClientsContainer(t *testing.T) {
	t.Parallel()
	d := dirs("/container")
	hub := d.Hub()

	if hub.Config.Configdir == nil || *hub.Config.Configdir != d.ClientRNS {
		t.Errorf("the hub's Reticulum directory = %v, want the client's own, %v", hub.Config.Configdir, d.ClientRNS)
	}
	if hub.Config.IdentityPath == nil || *hub.Config.IdentityPath != d.HubIdentity() {
		t.Errorf("the hub's identity = %v, want %v", hub.Config.IdentityPath, d.HubIdentity())
	}
	if hub.Config.HubName != hubAdvertisedName {
		t.Errorf("the hub advertises itself as %v, want %v", hub.Config.HubName, hubAdvertisedName)
	}
	if hub.Config.AnnouncePeriodS != hubAnnouncePeriodS {
		t.Errorf("the hub announces every %v seconds, want %v", hub.Config.AnnouncePeriodS, hubAnnouncePeriodS)
	}

	// The hub announces under a destination name, and a hub with none cannot be reached
	// even by a client that knows its identity.
	if hub.Config.DestName == "" {
		t.Error("the hub has no destination name")
	}

	// A hub with nowhere to keep its room registry keeps it in memory, and loses it the
	// next time the stack stops.
	if hub.Config.RoomRegistryPath == nil || *hub.Config.RoomRegistryPath != d.RoomRegistry() {
		t.Errorf("the hub's room registry = %v, want %v", hub.Config.RoomRegistryPath, d.RoomRegistry())
	}
}

// TestTheHubPublishesWhereTheClientAndTheBotLookForIt is the contract between the two
// halves at its narrowest: the hub's address is written beside its identity, and the
// Swift half reads it from `<client config>/hub_identity.rrc.hub` — a path neither half can
// see the other's copy of.
func TestTheHubPublishesWhereTheClientAndTheBotLookForIt(t *testing.T) {
	t.Parallel()
	d := dirs("/container")

	if got := d.HubIdentity() + hubDestinationSuffix; got != "/container/nomadnetwork/hub_identity.rrc.hub" {
		t.Errorf("the hub publishes to %v, want the file the appliance reads", got)
	}
}

func TestTheHubGetsAnIdentityAndKeepsIt(t *testing.T) {
	t.Parallel()
	path := filepath.Join(tempDir(t), "hub_identity")

	if err := ensureIdentity(path); err != nil {
		t.Fatalf("ensureIdentity: %v", err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the identity: %v", err)
	}
	if len(first) == 0 {
		t.Fatal("the identity written is empty")
	}

	// An identity is what makes the hub's address stable: replacing it would leave every
	// client that knows the old address talking to nobody.
	if err := ensureIdentity(path); err != nil {
		t.Fatalf("the second ensureIdentity: %v", err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the identity again: %v", err)
	}
	if string(first) != string(second) {
		t.Error("the second call replaced an identity that was already there")
	}
}

func TestAPublishedHubAddressIsLowerCaseHex(t *testing.T) {
	t.Parallel()
	identity := filepath.Join(tempDir(t), "hub_identity")
	hash := []byte{0xAB, 0x01, 0xCD, 0xEF}

	if err := publishHubDestination(identity, hash); err != nil {
		t.Fatalf("publishHubDestination: %v", err)
	}
	got, err := os.ReadFile(identity + hubDestinationSuffix)
	if err != nil {
		t.Fatalf("reading the published address: %v", err)
	}
	// gorrcd publishes exactly this, and a bot or client reading the file parses exactly
	// this: lowercase hexadecimal and one newline.
	if string(got) != "ab01cdef\n" {
		t.Errorf("the published address is %q, want %q", got, "ab01cdef\n")
	}
}

func TestAHubWithNoAddressIsNotPublished(t *testing.T) {
	t.Parallel()
	identity := filepath.Join(tempDir(t), "hub_identity")

	if err := publishHubDestination(identity, nil); err == nil {
		t.Error("a hub with no address published one anyway")
	}
}

func TestTheBotLivesInTheClientsContainer(t *testing.T) {
	t.Parallel()
	d := dirs("/container")
	b := d.Bot()

	if b.Options.ConfigDir != d.ClientRNS {
		t.Errorf("the bot's Reticulum directory = %v, want the client's own, %v", b.Options.ConfigDir, d.ClientRNS)
	}
	if b.Options.BotConfig != "/container/nomadnetwork/gorrcbot.toml" {
		t.Errorf("the bot's configuration = %v, want the file the appliance renders", b.Options.BotConfig)
	}
	if b.Options.Home != "/container/nomadnetwork/gorrcbot" {
		t.Errorf("the bot's home = %v, want a directory of its own under the client's", b.Options.Home)
	}

	// The identity is stated as well as the home, and the device is why: a path the
	// configuration file leaves unsaid takes the bot's own default, which is under the
	// operator's home directory — on iOS the container root, which cannot be written to at
	// all. The bot could not start, and the appliance reported itself running anyway.
	if b.Options.Identity != "/container/nomadnetwork/gorrcbot/bot_identity" {
		t.Errorf("the bot's identity = %v, want it named inside the bot's own home", b.Options.Identity)
	}
}
