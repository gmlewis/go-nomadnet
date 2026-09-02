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
		env := MakeEnvelope(TypeJoined, []byte(hash), []byte("general"), []byte(nick), nil, []byte(nick), NowMs())
		data, err := EncodeEnvelope(env)
		if err != nil {
			t.Fatal(err)
		}
		hub.HandleData(data)
	}
	// Out-of-order nick sort: zeta sorts before Alpha case-insensitively.
	join("1111", "zeta")
	join("2222", "Alpha")

	members := hub.GetRoomMembers("general")
	if len(members) != 2 {
		t.Fatalf("members = %v, want 2 entries", members)
	}
	if members[0].Nick != "Alpha" || members[0].HashHex != "32323232" {
		t.Errorf("members[0] = %+v, want nick Alpha with hash hex 32323232", members[0])
	}
	if members[1].Nick != "zeta" || members[1].HashHex != "31313131" {
		t.Errorf("members[1] = %+v, want nick zeta with hash hex 31313131", members[1])
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
