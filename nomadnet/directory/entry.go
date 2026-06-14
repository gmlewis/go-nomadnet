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

package directory

// Trust levels for directory entries, matching the Python NomadNet values.
const (
	TrustWarning   byte = 0x00
	TrustUntrusted byte = 0x01
	TrustUnknown   byte = 0x02
	TrustTrusted   byte = 0xFF
)

// Delivery types for preferred message delivery.
const (
	DeliveryDirect     byte = 0x01
	DeliveryPropagated byte = 0x02
)

// Entry represents a single peer in the NomadNet directory.
type Entry struct {
	SourceHash        []byte
	DisplayName       string
	SortRank          *int
	PreferredDelivery byte
	TrustLevel        byte
	HostsNode         bool
	IdentifyOnConnect bool
	Notes             string
}

// NewEntry creates a DirectoryEntry with the given source hash and defaults.
// Source hash must be the correct length for RNS identity hashes.
func NewEntry(sourceHash []byte) *Entry {
	return &Entry{
		SourceHash:        sourceHash,
		DisplayName:       "",
		PreferredDelivery: DeliveryDirect,
		TrustLevel:        TrustUnknown,
		HostsNode:         false,
		IdentifyOnConnect: false,
		Notes:             "",
	}
}
