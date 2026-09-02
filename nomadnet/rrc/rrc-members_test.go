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

import "testing"

// TestGetRoomMembers pins the TUI member-list feed (Python
// RoomWidget._refresh_users_pane, Channels.py:663-724): members carry their
// identity-hash hex (from the JOIN/JOINED envelopes' source) alongside the
// display nick, and the list sorts case-insensitively by nick.
func TestGetRoomMembers(t *testing.T) {
	t.Parallel()

	mgr := NewManager(tempDir(t), func() []byte { return []byte("testhash") })
	mgr.SetNickname("TestNick")
	hub := mgr.AddHub([]byte("hubhash"), "rrc.chat", "TestHub")
	hub.AddRoom("general")

	join := func(hash, nick string) {
		// rrcd fans out JOINED with a single-element list of the joiner's
		// identity hash as the body (RRC.py:975-978).
		env := MakeEnvelope(TypeJoined, []byte(hash), []byte("general"), []byte(nick), []any{[]byte(hash)}, []byte(nick), NowMs())
		data, err := EncodeEnvelope(env)
		if err != nil {
			t.Fatal(err)
		}
		hub.HandleData(data)
	}
	// Out-of-order nick sort: zeta sorts before Alpha case-insensitively.
	join("1111", "zeta")
	join("2222", "Alpha")

	// Python adds the client's own identity hash to the member set on every
	// JOINED (RRC.py:985-986); the manager's identity hash here is "testhash",
	// displayed via the hash-prefix fallback since it was never learned as a
	// nick.
	ownHex := hexString([]byte("testhash"))

	members := hub.GetRoomMembers("general")
	if len(members) != 3 {
		t.Fatalf("members = %v, want 3 entries (two joiners plus own hash)", members)
	}
	if members[0].Nick != ownHex[:12] || members[0].HashHex != ownHex {
		t.Errorf("members[0] = %+v, want own hash %v with prefix nick", members[0], ownHex)
	}
	if members[1].Nick != "Alpha" || members[1].HashHex != "32323232" {
		t.Errorf("members[1] = %+v, want nick Alpha with hash hex 32323232", members[1])
	}
	if members[2].Nick != "zeta" || members[2].HashHex != "31313131" {
		t.Errorf("members[2] = %+v, want nick zeta with hash hex 31313131", members[2])
	}
}

// TestGetRoomMembersEmptyAndUnknown pins the empty/absent-room branches.
func TestGetRoomMembersEmptyAndUnknown(t *testing.T) {
	t.Parallel()

	mgr := NewManager(tempDir(t), func() []byte { return []byte("testhash") })
	hub := mgr.AddHub([]byte("hubhash"), "rrc.chat", "TestHub")

	if members := hub.GetRoomMembers("nosuchroom"); len(members) != 0 {
		t.Errorf("unknown room members = %v, want empty", members)
	}
	hub.AddRoom("quiet")
	if members := hub.GetRoomMembers("quiet"); len(members) != 0 {
		t.Errorf("empty room members = %v, want empty", members)
	}
}
