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

package micron

import (
	"crypto/sha256"
	"encoding/hex"
)

// partialPlaceholder is the hourglass shown for a not-yet-received partial,
// mirroring Python's urwid.Text("⧖") (MicronParser.py:186).
const partialPlaceholder = "⧖"

// PartialHash returns the lowercase hex SHA-256 of the partial's descriptor
// (UTF-8), mirroring Python's
// RNS.hexrep(RNS.Identity.full_hash(partial_descriptor.encode("utf-8")),
// delimit=False) (MicronParser.py:189). RNS.Identity.full_hash is SHA-256.
// The hash identifies the partial for dedup and refresh scheduling.
func PartialHash(node *Node) string {
	sum := sha256.Sum256([]byte(node.PartialDescriptor))
	return hex.EncodeToString(sum[:])
}
