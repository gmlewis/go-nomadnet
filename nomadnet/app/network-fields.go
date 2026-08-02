// Copyright 2026 Glenn Lewis. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

// Package app implements the NomadNet application core.
//
// network-fields.go resolves the RNS-derived fields the Network page shows for
// a known node or node announce: the node-operator display string (the LXMF
// delivery peer's directory name), the node's LXMF propagation-node destination
// hash, and the hop distance. Each mirrors a Python NomadNet lookup in
// Network.py (AnnounceInfo 138-144, KnownNodeInfo 629-634/681-688/791-800).

package app

import (
	"github.com/gmlewis/go-reticulum/rns"
)

// NodeOperatorDisplay returns the display name of the LXMF delivery peer
// associated with the node identified by nodeHash, mirroring Python's
// op_str computation (AnnounceInfo Network.py:138-144; KnownNodeInfo
// Network.py:681-688): RNS.Identity.recall(node_hash) → derive the
// "lxmf.delivery" destination hash → directory.simplest_display_str. When the
// identity cannot be recalled it returns "Unknown" (KnownNodeInfo's branch;
// AnnounceInfo raises, but the TUI surfaces "Unknown" gracefully).
func (a *App) NodeOperatorDisplay(nodeHash []byte) string {
	if a.Transport == nil || nodeHash == nil {
		return "Unknown"
	}
	id := rns.RecallIdentity(a.Transport, nodeHash)
	if id == nil {
		return "Unknown"
	}
	opHash := rns.CalculateHash(id, "lxmf", "delivery")
	return a.Dir.SimplestDisplayStr(opHash)
}

// NodePropagationHash returns the LXMF "lxmf.propagation" destination hash for
// the node identified by nodeHash, mirroring Python's pn_hash derivation
// (KnownNodeInfo Network.py:629): RNS.Identity.recall(node_hash) → derive the
// "lxmf.propagation" destination hash. It returns nil when the identity cannot
// be recalled (rendered as "No associated Propagation Node known").
func (a *App) NodePropagationHash(nodeHash []byte) []byte {
	if a.Transport == nil || nodeHash == nil {
		return nil
	}
	id := rns.RecallIdentity(a.Transport, nodeHash)
	if id == nil {
		return nil
	}
	return rns.CalculateHash(id, "lxmf", "propagation")
}
