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
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/gmlewis/go-reticulum/bot"
	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/rrc"
)

// The appliance's own halves, and the paths they read.
//
// Android reaches these the way the platform allows: gorrcd and gorrcbot are separate
// programs, started with a command line, a working directory and a HOME each. iOS allows
// none of that, so they are values here — the same two programs, as goroutines, pointed at
// the same directories.
//
// What is deliberately *not* here is their configuration. The hub's identity and its
// published address are the hub's own business, and the bot's TOML is rendered by the Swift
// half, which is where the client's own configuration comes from too. The Go half reads what
// the Swift half wrote and nothing else, so there is one place that decides what a file says.
type ApplianceDirs struct {
	// ClientConfig is the NomadNet client's own directory: its configuration, its storage,
	// and the local hub's identity. The hub's identity lives here rather than with the
	// hub's other files because the address it publishes lands beside it
	// (`<identity>.rrc.hub`), and that is the file the client's channel list and the bot's
	// configuration both already look for.
	ClientConfig string

	// ClientRNS is the Reticulum directory the hub and the bot attach to the shared
	// instance through, the one whose configuration says `require_shared_instance = yes`.
	// It is not the transport's: a second process pointed at the transport's directory
	// would try to own an instance that is already owned, and come up attached to nothing.
	ClientRNS string

	// TransportRNS is the Reticulum directory that owns the shared instance.
	TransportRNS string
}

// The names and numbers the appliance's own halves are started with.
const (
	// hubIdentityName is the hub's identity, beside the client's own configuration.
	hubIdentityName = "hub_identity"

	// hubDestinationSuffix is what the hub's published address is called. It is the suffix
	// gorrcd publishes under, so a file written here is one a supervisor on any platform
	// would recognize.
	hubDestinationSuffix = ".rrc.hub"

	// hubAdvertisedName is what the hub calls itself when it announces. It is the name the
	// client's channel list shows for it, so the two say the same thing about the same hub.
	hubAdvertisedName = "appliance-hub"

	// hubAnnouncePeriodS is how often the hub re-announces itself. A hub that announces once
	// and never again is one a peer that started an hour later never learns about; a minute
	// is what the Android appliance uses.
	hubAnnouncePeriodS = 60.0

	// botHomeName is the bot's home directory, under the client's own configuration
	// directory. It holds the bot's identity and its storage, and it is the directory the
	// bot creates for itself on its first run.
	botHomeName = "gorrcbot"

	// botConfigName is the bot's own TOML, rendered by the Swift half from the hubs the
	// appliance's settings name.
	botConfigName = "gorrcbot.toml"

	// botIdentityName is the bot's identity inside its home directory, under the name the
	// bot itself gives it.
	botIdentityName = "bot_identity"

	// roomRegistryName is the hub's own record of the rooms it has registered, under the
	// name gorrcd keeps it in.
	roomRegistryName = "rooms.toml"
)

// Subsystems are everything the appliance runs, in the order it has to start.
//
// The order is the Android appliance's, and it is not arbitrary. The transport owns the
// shared instance the other two attach to, so it is first. The hub publishes its address as
// it comes up, and both the bot and the client find it there rather than being told it. The
// bot dials hubs and may dial the local one, so it is last — and on the first run of a fresh
// install, when the hub has not published yet, it dials only the public ones and picks the
// local hub up on the next start.
func (d ApplianceDirs) Subsystems() []Subsystem {
	return []Subsystem{
		&Transport{ConfigDir: d.TransportRNS},
		d.Hub(),
		d.Bot(),
	}
}

// Hub is the appliance's own RRC hub, configured to live in the client's container.
func (d ApplianceDirs) Hub() *Hub {
	identity := d.HubIdentity()
	rooms := d.RoomRegistry()
	configdir := d.ClientRNS

	cfg := rrc.DefaultHubConfig()
	cfg.Configdir = &configdir
	cfg.IdentityPath = &identity
	cfg.RoomRegistryPath = &rooms
	cfg.HubName = hubAdvertisedName
	cfg.AnnouncePeriodS = hubAnnouncePeriodS
	return &Hub{Config: cfg}
}

// RoomRegistry is the file the hub keeps its registered rooms in.
//
// It is stated rather than left to the hub's own default, which is a path under the
// operator's home directory — on iOS the app's container root, which the app may not write
// to. Stating it also keeps the registry with the rest of the client's files, where an
// operator can find it and where it survives a reinstall of the app itself. A hub with no
// registry path keeps its rooms in memory and loses them when the stack stops.
func (d ApplianceDirs) RoomRegistry() string {
	return filepath.Join(d.ClientConfig, roomRegistryName)
}

// Bot is the appliance's chat bot, configured to live in the client's container.
//
// Both paths are stated, and the identity is why. A configuration file that leaves a path
// unsaid gets the bot's own default for it, which is under the *operator's* home directory —
// and on iOS the home directory is the app's container root, which is not writable, so the
// default is not merely wrong but impossible. The home directory alone does not settle it:
// the identity is the one path a configuration file may still overrule the command line
// with, so an unstated one takes the default back. Naming both is what leaves the bot with
// nothing to default.
func (d ApplianceDirs) Bot() *Bot {
	home := filepath.Join(d.ClientConfig, botHomeName)
	return &Bot{Options: bot.Options{
		ConfigDir: d.ClientRNS,
		BotConfig: filepath.Join(d.ClientConfig, botConfigName),
		Home:      home,
		Identity:  filepath.Join(home, botIdentityName),
	}}
}

// HubIdentity is the file the hub's identity is kept in, which is also where its published
// address lands beside.
func (d ApplianceDirs) HubIdentity() string {
	return filepath.Join(d.ClientConfig, hubIdentityName)
}

// ensureIdentity creates the identity at path if nothing is there.
//
// A hub loads its identity and has nothing to fall back on: gorrcd's bootstrap run makes one
// the first time, and the appliance has no separate bootstrap to make it in. This is where
// that happens instead. It is a no-op when the file is already there, which matters more
// than it looks: the identity is what makes the hub's address stable, and replacing it would
// leave every client that knows the old one talking to nobody.
func ensureIdentity(path string) error {
	err := ensureFile(path, func() error {
		identity, err := rns.NewIdentity(true, rns.NewLogger())
		if err != nil {
			return fmt.Errorf("bridge: making an identity for %v: %w", path, err)
		}
		if err := identity.ToFile(path); err != nil {
			return fmt.Errorf("bridge: writing the identity to %v: %w", path, err)
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return fmt.Errorf("bridge: restricting the identity at %v: %w", path, err)
		}
		return nil
	})
	return err
}

// ensureFile makes the file at path with write unless it is already there.
func ensureFile(path string, write func() error) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("bridge: looking for %v: %w", path, err)
	}
	return write()
}

// publishHubDestination writes the hub's own address beside its identity, which is where the
// bot's configuration and the client's channel list both look for it.
//
// The format is the one gorrcd publishes: the destination hash in lowercase hexadecimal and
// a newline. A file that named the address in another way would be a file the bot could not
// read and a client that could not reach the hub it was just told about.
func publishHubDestination(identityPath string, hash []byte) error {
	if len(hash) == 0 {
		return errors.New("bridge: the hub has no address to publish")
	}
	path := identityPath + hubDestinationSuffix
	if err := os.WriteFile(path, []byte(hex.EncodeToString(hash)+"\n"), 0o644); err != nil {
		return fmt.Errorf("bridge: publishing the hub's address to %v: %w", path, err)
	}
	return nil
}
