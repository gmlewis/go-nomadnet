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

// Package tui implements the NomadNet terminal user interface.
//
// network-rns-fields.go holds the pure string formatters for the RNS-derived
// fields shown in the Network page's AnnounceInfo / KnownNodeInfo views: the
// hop-distance string and the centered LXMF Propagation Node address line. The
// RNS lookups themselves (identity recall, hops_to, hash derivation) live in
// the app package; these formatters turn the resolved values into the exact
// strings Python renders (Network.py:629-634, 791-800).

package tui

import (
	"strconv"

	"github.com/gmlewis/go-reticulum/rns"
)

// FormatHopsStr renders the KnownNodeInfo "Distance" value (Network.py:791-800):
// "1 hop" for a single hop, "<N> hops" for any other reachable hop count, and
// "Unknown" when no path is known. The app surfaces "no path" (hops ==
// RNS.Transport.PATHFINDER_M) as a nil pointer, mirroring Python's
// `hops == PATHFINDER_M` branch.
func FormatHopsStr(hops *int) string {
	if hops == nil {
		return "Unknown"
	}
	if *hops == 1 {
		return "1 hop"
	}
	return strconv.Itoa(*hops) + " hops"
}

// FormatLXMFAddrStr renders the KnownNodeInfo centered LXMF Propagation Node
// address line (Network.py:629-634). When the node identity is known (pnHash
// non-nil), it is `g["sent"] + " LXMF Propagation Node Address is " +
// prettyhexrep(pn_hash)`; when the identity could not be recalled (pnHash nil)
// it is the literal "No associated Propagation Node known".
func FormatLXMFAddrStr(pnHash []byte, sentGlyph string) string {
	if pnHash == nil {
		return "No associated Propagation Node known"
	}
	return sentGlyph + " LXMF Propagation Node Address is " + rns.PrettyHexRep(pnHash)
}
