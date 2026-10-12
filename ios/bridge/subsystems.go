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
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gmlewis/go-reticulum/bot"
	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/rrc"
)

// The appliance's three long-running subsystems.
//
// On Android each of these is a program the Kotlin supervisor starts and
// watches. Here they are values the engine starts and stops, which is the whole
// difference the platform forces: no forks, no exec, no supervisor — one process
// holding three goroutine trees.
//
// They are thin on purpose. Each one is the smallest thing that turns a daemon's
// entry point into a Subsystem: the behavior belongs to go-reticulum, where it
// is already tested, and duplicating any of it here would give the appliance a
// second implementation to keep in step.

// Transport is the Reticulum transport the whole appliance talks through.
//
// It owns the shared instance the hub, the bot and the client all attach to, so it
// is the first subsystem up and the last one down.
type Transport struct {
	// ConfigDir is the Reticulum configuration directory. The Swift half renders
	// its `config` file before the appliance starts; this only points at it.
	ConfigDir string

	mu sync.Mutex
	r  *rns.Reticulum
}

// Name identifies the subsystem in an error.
func (t *Transport) Name() string { return "transport" }

// Start brings the transport up and starts serving.
func (t *Transport) Start(ctx context.Context) error {
	logger := rns.NewLogger()
	ts := rns.NewTransportSystem(logger)
	r, err := rns.NewReticulumWithLogger(ts, t.ConfigDir, logger)
	if err != nil {
		return fmt.Errorf("starting Reticulum on %v: %w", t.ConfigDir, err)
	}

	t.mu.Lock()
	t.r = r
	t.mu.Unlock()
	return nil
}

// Close releases the transport and the shared instance with it.
func (t *Transport) Close() error {
	t.mu.Lock()
	r := t.r
	t.r = nil
	t.mu.Unlock()
	if r == nil {
		return nil
	}
	if err := r.Close(); err != nil {
		return fmt.Errorf("closing Reticulum: %w", err)
	}
	return nil
}

// Hub is the in-app Reticulum Relay Chat hub.
//
// It is what makes the appliance a hub rather than only a client: peers connect
// to it and talk to each other through the device, with no server anywhere else.
type Hub struct {
	// Config is the hub's own configuration, including the directories the Swift
	// half rendered for it.
	Config rrc.HubConfig

	mu  sync.Mutex
	svc *rrc.HubService
}

// Name identifies the subsystem in an error.
func (h *Hub) Name() string { return "hub" }

// Start brings the hub up, begins announcing it, and publishes where it can be found.
//
// The hub has no bootstrap of its own here: gorrcd makes its identity on a first run from
// the command line, and there is no command line on iOS. Its identity is therefore made
// here if it is missing, and its address is published beside it — which is the whole reason
// the bot and the client can reach the appliance's own hub without either being told what it
// is.
func (h *Hub) Start(ctx context.Context) error {
	if h.Config.IdentityPath == nil || *h.Config.IdentityPath == "" {
		return errors.New("bridge: the hub has no identity to announce itself with")
	}
	identityPath := *h.Config.IdentityPath
	if err := ensureIdentity(identityPath); err != nil {
		return err
	}

	svc := rrc.NewHubService(h.Config)
	svc.SetLogger(rns.NewLogger())
	if err := svc.Start(); err != nil {
		return fmt.Errorf("starting the RRC hub: %w", err)
	}

	// A hub that is serving but unpublished is one nobody can dial: the address is how the
	// client's channel list and the bot's configuration learn about it at all. Failing the
	// start is the honest answer — the engine then says the hub did not come up, rather than
	// leaving a hub running that nothing can find.
	if err := publishHubDestination(identityPath, svc.DestinationHash()); err != nil {
		svc.Stop()
		return err
	}

	h.mu.Lock()
	h.svc = svc
	h.mu.Unlock()
	return nil
}

// Close stops the hub.
func (h *Hub) Close() error {
	h.mu.Lock()
	svc := h.svc
	h.svc = nil
	h.mu.Unlock()
	if svc == nil {
		return nil
	}
	svc.Stop()
	return nil
}

// Bot is the RRC chat bot: the appliance's offline field assistant.
//
// It reads the live sensor stream the appliance publishes, so it answers where
// the device is from the device's own receiver rather than from anything it was
// told.
type Bot struct {
	// Options is the bot's own configuration, including its home directory and
	// the TOML file the Swift half rendered.
	Options bot.Options

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan error
}

// Name identifies the subsystem in an error.
func (b *Bot) Name() string { return "bot" }

// Start brings the bot up: it loads its configuration and its identity, dials
// every hub it was given, and serves until the engine stops it.
//
// A bot in its first run creates the files it needs and stops instead of
// connecting, which is not a failure — the engine runs it again, and the second
// run is the one that connects. Reporting the first run as an error would make a
// fresh install look broken. The second run's failure *is* reported, and it is
// waited for: the appliance found on a device that a bot which cannot start its
// second run — a path it cannot write, say — left the appliance reporting itself
// running for as long as nobody looked.
func (b *Bot) Start(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)

	go func() { done <- bot.Run(runCtx, b.Options) }()

	// The first run returns almost immediately; waiting for it here is what makes
	// Start's contract — that the subsystem is ready when it returns — true.
	firstRun := false
	select {
	case err := <-done:
		if err == nil || errors.Is(err, bot.ErrFirstRun) {
			firstRun = true
			go func() { done <- bot.Run(runCtx, b.Options) }()
		} else {
			cancel()
			return fmt.Errorf("starting the bot: %w", err)
		}
	case <-ctx.Done():
		cancel()
		return ctx.Err()
	case <-time.After(botStartGrace):
		// Still running after the grace period means it is serving.
	}

	if firstRun {
		// A second run that comes back is a bot that did not start, and saying so is
		// the difference between an appliance that runs a bot and one that says it does.
		select {
		case err := <-done:
			cancel()
			return fmt.Errorf("starting the bot after its first run: %w", err)
		case <-ctx.Done():
			cancel()
			return ctx.Err()
		case <-time.After(botStartGrace):
			// Serving, which is what the second run is for.
		}
	}

	b.mu.Lock()
	b.cancel = cancel
	b.done = done
	b.mu.Unlock()
	return nil
}

// Close stops the bot and waits for it to release everything it started.
func (b *Bot) Close() error {
	b.mu.Lock()
	cancel := b.cancel
	done := b.done
	b.cancel = nil
	b.done = nil
	b.mu.Unlock()

	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case err := <-done:
		// A bot that returned a reason on its way out is not a failure of the stop: the
		// engine asked it to go, and it has gone.
		_ = err
	case <-time.After(botStopGrace):
		// A bot that will not come back is still stopped: the context is
		// cancelled, and the engine must not be held by it.
	}
	return nil
}

// botStartGrace is how long Start waits to find out whether the bot is going to
// return immediately or serve. A first run returns in milliseconds; a serving
// bot never returns, so this only ever bounds a real failure.
const botStartGrace = 500 * time.Millisecond

// botStopGrace bounds the wait for a stopped bot to return.
const botStopGrace = 3 * time.Second
