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
	"time"

	"github.com/gmlewis/go-nomadnet/nomadnet/peersettings"
	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/rns/msgpack"
)

// GetUserSelectedPropagationNode returns the manually selected propagation
// node hash, or nil when none is set. This mirrors the Python NomadNet
// get_user_selected_propagation_node.
func (a *App) GetUserSelectedPropagationNode() []byte {
	a.psMu.Lock()
	defer a.psMu.Unlock()
	if a.PeerSettings == nil {
		return nil
	}
	if h, ok := a.PeerSettings.PropagationNode.([]byte); ok {
		return h
	}
	return nil
}

// SetUserSelectedPropagationNode stores the manually selected propagation node
// hash, persists peer settings, and re-runs automatic selection so the router
// uses it. This mirrors the Python NomadNet set_user_selected_propagation_node.
func (a *App) SetUserSelectedPropagationNode(nodeHash []byte) {
	a.psMu.Lock()
	if a.PeerSettings == nil {
		a.PeerSettings = peersettings.DefaultSettings(a.AnnounceInterval)
	}
	a.PeerSettings.PropagationNode = nodeHash
	a.savePeerSettingsLocked()
	a.psMu.Unlock()
	a.AutoSelectPropagationNode()
}

// GetDefaultPropagationNode returns the propagation node currently in use by
// the LXMF router, or nil when the router is unavailable. This mirrors the
// Python NomadNet get_default_propagation_node.
func (a *App) GetDefaultPropagationNode() []byte {
	if a.Router == nil {
		return nil
	}
	return a.Router.GetOutboundPropagationNode()
}

// applyPropagationConfig applies the [node] disable_propagation setting to the
// LXMF router at startup, mirroring Python NomadNetworkApp.py:378-396
// (v1.2.8):
//
//	if self.disable_propagation:
//	    if os.path.isfile(self.pnannouncedpath):
//	        self.message_router.disable_propagation()   # announces the withdrawal
//	        os.unlink(self.pnannouncedpath)
//	else:
//	    self.message_router.enable_propagation()
//	    # write self.pnannouncedpath (a timestamp)
//
// The marker file (<configdir>/pnannounced, NomadNetworkApp.py:110) records that
// this instance has announced propagation state at some point, which is what
// makes the disable branch announce: LXMRouter.disable_propagation publishes the
// node_state=false declaration (LXMRouter.py:692-694), and an instance that never
// served a propagation node has nothing to withdraw. An enabled instance starts
// serving here, so a Go NomadNet with disable_propagation = No hosts its own
// propagation node exactly as Python's does.
func (a *App) applyPropagationConfig() {
	if a.Router == nil || a.Storage == nil {
		return
	}
	marker := a.Storage.PNAnnounced

	if a.DisablePropagation {
		if _, err := os.Stat(marker); err != nil {
			return
		}
		a.Logger.Debug("Sending indication to peered LXMF Propagation Node that this node is no longer participating")
		a.Router.DisablePropagation()
		if err := os.Remove(marker); err != nil {
			a.Logger.Error("Could not remove the propagation announce marker %v: %v", marker, err)
		}
		return
	}

	a.Router.EnablePropagation()
	a.Logger.Info("LXMF Propagation Node started on: %v", rns.PrettyHex(a.Router.PropagationDestinationHash()))
	packed, err := msgpack.Pack(float64(time.Now().Unix()))
	if err != nil {
		a.Logger.Error("Could not pack the propagation announce marker: %v", err)
		return
	}
	if err := os.WriteFile(marker, packed, 0o644); err != nil {
		a.Logger.Error("Could not write the propagation announce marker %v: %v", marker, err)
	}
}

// declarePropagationNodeAddress declares this instance's lxmf.propagation
// address. It is the callback Node.Announce runs after each node announce,
// mirroring the trailing self.app.message_router.announce_propagation_node() in
// Python's Node.announce (Node.py:216-220): every NomadNet node that hosts a
// node declares the propagation address derived from its own identity, so the
// "LXMF Propagation Node Address is <hex>" line on a peer's node Info page
// (Network.py:629-634) names a destination that exists on the network — with
// node_state false when this instance does not serve propagation.
func (a *App) declarePropagationNodeAddress() {
	if a.Router == nil {
		return
	}
	a.Router.AnnouncePropagationNode()
}
