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
	"github.com/gmlewis/go-nomadnet/nomadnet/peersettings"
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
