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

package rrc

// Protocol version.
const RRCVersion = 1

// Envelope keys for CBOR-encoded messages.
const (
	KeyVersion   = 0
	KeyType      = 1
	KeyMessageID = 2
	KeyTimestamp = 3
	KeySource    = 4
	KeyRoom      = 5
	KeyBody      = 6
	KeyNick      = 7
)

// Message types.
const (
	TypeHello   = 1
	TypeWelcome = 2

	TypeJoin   = 10
	TypeJoined = 11
	TypePart   = 12
	TypeParted = 13

	TypeMsg    = 20
	TypeNotice = 21
	TypeAction = 22

	TypePing = 30
	TypePong = 31

	TypeError = 40

	// TypeResourceEnvelope is the resource-transfer extension message.
	// Doc 3-RRC reserves message types 0-63 for core protocol use and
	// assigns extensions 64+, but the Python nomadnet SOT (RRC.py
	// T_RESOURCE_ENVELOPE) and the rrcd hub it interoperates with both use
	// 50 — the value is kept for wire parity with the SOT.
	TypeResourceEnvelope = 50
)

// Body sub-keys for HELLO messages.
const (
	BHelloName = 0 // client name string ("nomadnet")
	BHelloVer  = 1 // client version string ("0.1")
	BHelloCaps = 2 // capabilities dict
)

// Body sub-keys for WELCOME messages.
const (
	BWelcomeHub    = 0 // hub name string
	BWelcomeVer    = 1 // hub version string
	BWelcomeCaps   = 2 // capabilities dict
	BWelcomeLimits = 3 // limits dict
)

// Limit keys in WELCOME body.
const (
	LMaxNickBytes           = 0
	LMaxRoomNameBytes       = 1
	LMaxMsgBodyBytes        = 2
	LMaxRoomsPerSession     = 3
	LRateLimitMsgsPerMinute = 4
)

// Capability flags.
const (
	CapResourceEnvelope = 0
	CapAction           = 1
)

// Resource envelope body keys.
const (
	ResKeyID       = 0
	ResKeyKind     = 1
	ResKeySize     = 2
	ResKeySHA256   = 3
	ResKeyEncoding = 4
)

// Resource kinds.
const (
	ResKindNotice = "notice"
	ResKindMOTD   = "motd"
	ResKindBlob   = "blob"
)

// Default values.
const (
	DefaultDestName      = "rrc.hub"
	DefaultMaxNickBytes  = 32
	DefaultMaxRoomBytes  = 64
	DefaultMaxMsgBytes   = 350
	DefaultMaxRooms      = 32
	DefaultRatePerMinute = 240
)

// History entry keys for persistence.
const (
	HKind    = "k"
	HSrc     = "s"
	HNick    = "n"
	HText    = "t"
	HTS      = "ts"
	HMention = "m"
)

// Hub connection status.
const (
	StatusDisconnected = 0
	StatusConnecting   = 1
	StatusConnected    = 2
	StatusFailed       = 3
)

// Timing constants.
const (
	CleanHistoryInterval = 5   // seconds between history cleanups
	NoticeTimeout        = 600 // seconds before ephemeral notices expire
)
