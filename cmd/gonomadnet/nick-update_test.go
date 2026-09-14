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

package main

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/gmlewis/go-reticulum/rrc"
)

// TestRRCoomMembersFollowTheLocalNickOverride asserts the Users pane row for
// the local user picks up a /nick change at once. The pane renders every member
// through the hub's learned nick table, and the hub learns our nick from our
// next message — so without the local override the user's own row kept the hash
// prefix, or the previous name, until they spoke again.
func TestRRCoomMembersFollowTheLocalNickOverride(t *testing.T) {
	t.Parallel()

	hub, own := newPrivateRenderHub(t)
	// One self-join is enough: the client adds its own hash to the room's
	// member set (the T_JOINED body carries the member list).
	feedEnvelope(t, hub, rrc.MakeClientEnvelope(rrc.TypeJoined, bytes.Repeat([]byte{0x0a}, 16),
		[]byte("general"), nil, []any{own}, make([]byte, 8), rrc.NowMs()))

	ownHex := fmt.Sprintf("%x", own)
	ownRow := func() (string, bool) {
		for _, m := range rrcRoomMembers(hub, "general") {
			if m.Hash == ownHex {
				return m.Nick, m.IsSelf
			}
		}
		return "", false
	}

	before, isSelf := ownRow()
	if before == "" {
		t.Fatal("the local user is missing from the Users pane after joining a room")
	}
	if !isSelf {
		t.Error("the local user's own row is not flagged as self")
	}

	hub.SetNickOverride("glenn")
	after, isSelf := ownRow()
	if after != "glenn" {
		t.Errorf("Users pane own row = %q after /nick, want %q (was %q)", after, "glenn", before)
	}
	if !isSelf {
		t.Error("the local user's own row lost its self flag after /nick")
	}
}
