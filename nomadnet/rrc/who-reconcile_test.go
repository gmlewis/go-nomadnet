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

import (
	"testing"
	"time"
)

// The fleet's 2026-09-03 20:5x captures showed every client's member count
// frozen at its own join-time snapshot (6/6/6/5/3/4 across six nodes on ONE
// hub — even the Python SOT was wrong). rrcd's include_joined_member_list
// defaults to FALSE, so JOINED/PARTED fanouts carry body=None and no member
// data, and the /who reply was only used to decorate members already in the
// set — so membership never healed and parted members were never removed.
// The periodic silent /who reconciliation plus the reply-as-authority
// semantics heal every client to the hub's live list.

func reconcileFixture(t *testing.T) (*RRCManager, *RRCHub, *[]string) {
	t.Helper()
	mgr, hub := fanoutFixture(t)
	hub.AddRoom("test3")
	hub.AddRoom("test4")
	sent := &[]string{}
	hub.onSend = func(env map[any]any) {
		switch body := env[KeyBody].(type) {
		case []byte:
			*sent = append(*sent, string(body))
		case string:
			*sent = append(*sent, body)
		}
	}
	// A welcomed hub — the reconciliation gates on the welcome.
	hub.Welcomed = true
	return mgr, hub, sent
}

// TestWhoRefreshSendsSilentWhoPerJoinedRoom pins the reconciliation pass:
// one silent /who per joined room, marked silent so the replies never hit
// the message log.
func TestWhoRefreshSendsSilentWhoPerJoinedRoom(t *testing.T) {
	t.Parallel()

	_, hub, sent := reconcileFixture(t)
	hub.refreshRoomMembership()

	want := []string{"/who test", "/who test3", "/who test4"}
	if len(*sent) != len(want) {
		t.Fatalf("sent %v commands, want %v", *sent, want)
	}
	for i, w := range want {
		if (*sent)[i] != w {
			t.Errorf("command %v = %q, want %q", i, (*sent)[i], w)
		}
	}
	for _, room := range []string{"test", "test3", "test4"} {
		if !hub.silentWhoRooms[room] {
			t.Errorf("room %q not marked for silent who consumption", room)
		}
	}
}

// TestWhoRefreshArmsOnWelcomeAndReschedules pins the timer wiring: the
// WELCOME arms the refresh at whoRefreshInterval; firing the scheduled
// callback sends the commands and reschedules.
func TestWhoRefreshArmsOnWelcomeAndReschedules(t *testing.T) {
	t.Parallel()

	_, hub, sent := reconcileFixture(t)

	hub.afterFunc = func(d time.Duration, fn func()) *time.Timer {
		if d != whoRefreshInterval {
			t.Errorf("refresh interval = %v, want %v", d, whoRefreshInterval)
		}
		return time.AfterFunc(d, func() {}) // never fires in the test
	}
	hub.handleWelcome(map[any]any{BWelcomeHub: []byte("RaspPi Local Hub")})
	if hub.whoRefreshPending == nil {
		t.Fatal("welcome did not arm the membership refresh")
	}

	// Fire the scheduled callback by hand (time delays are simulated). The
	// auto-who JOIN burst also sent its JOIN envelopes; only the /who
	// commands count for the reconciliation pass.
	hub.whoRefreshPending()
	whos := 0
	for _, s := range *sent {
		if len(s) >= 5 && s[:5] == "/who " {
			whos++
		}
	}
	if whos != 3 {
		t.Fatalf("refresh sent %v /who commands, want 3 (sent: %v)", whos, *sent)
	}
	if hub.whoRefreshPending == nil {
		t.Error("refresh did not reschedule after firing")
	}
}

// TestWhoRefreshStopsWhenNotWelcomed pins the stop condition: if the welcome
// is lost (link closed) before the armed callback fires, the reconciliation
// neither sends nor reschedules.
func TestWhoRefreshStopsWhenNotWelcomed(t *testing.T) {
	t.Parallel()

	_, hub, sent := reconcileFixture(t)
	hub.afterFunc = func(d time.Duration, fn func()) *time.Timer {
		return time.AfterFunc(d, func() {}) // never fires in the test
	}
	hub.handleWelcome(map[any]any{})
	if hub.whoRefreshPending == nil {
		t.Fatal("welcome did not arm the refresh")
	}
	// The link closes: Welcomed drops to false before the fire.
	hub.Welcomed = false
	hub.whoRefreshPending()
	if len(*sent) != 0 {
		t.Errorf("refresh sent %v commands while not welcomed, want 0", len(*sent))
	}
	if hub.whoRefreshPending != nil {
		t.Error("refresh rescheduled while not welcomed")
	}
}

// TestWhoReplyReplacesMembership pins the healing direction: the /who reply
// REPLACES the room's member set with the hub's live list, so stale
// join-time snapshots and departed members converge away (the reply is the
// only authoritative member source while rrcd's member-list fanout flag is
// off). Known full-hash members keep their full keys; unknown nicked
// members enter under their 12-hex reply prefix so the count is correct.
func TestWhoReplyReplacesMembership(t *testing.T) {
	t.Parallel()

	_, hub, _ := reconcileFixture(t)
	// A stale join-time set: one live member plus one that has since left.
	// The own member's key is hexString([]byte("ownhash")) = "6f776e68617368".
	hub.Members["test"] = map[string]bool{
		"6f776e68617368":   true, // own full-hash member, still live
		"deaddeaddeadbeef": true, // departed member
	}
	hub.handleWelcome(map[any]any{})

	// rrcd's reply lists every live member as "nick (hash12)".
	hub.HandleData(noticeEnvelope(t, "",
		"members in test: OwnNick (6f776e686173), Go port of NomadNet on RaspPi (aabbccddeeff)", "who1"))

	members := hub.GetMembers("test")
	if len(members) != 2 {
		t.Fatalf("members after who reply = %v, want 2 (the reply replaces the stale set and the parted member is healed away)", members)
	}
	for _, m := range members {
		switch m {
		case "6f776e68617368":
			// The prefix-resolvable member kept its full hash.
		case "aabbccddeeff":
			// The unknown member entered under its reply prefix.
		default:
			t.Errorf("unexpected member key %q", m)
		}
	}
	// The nick is learned for the prefix-resolvable member.
	if got := hub.Nicks["6f776e68617368"]; got != "OwnNick" {
		t.Errorf("learned nick = %q, want OwnNick", got)
	}
}

// TestWhoReplyEmptyListEmptiesRoom pins the room-emptied case: a "(none)"
// reply replaces the member set with nothing.
func TestWhoReplyEmptyListEmptiesRoom(t *testing.T) {
	t.Parallel()

	_, hub, _ := reconcileFixture(t)
	hub.Members["test"] = map[string]bool{"aaaa": true}
	hub.handleWelcome(map[any]any{})

	hub.HandleData(noticeEnvelope(t, "", "members in test: (none)", "who2"))
	if got := hub.GetMembers("test"); len(got) != 0 {
		t.Fatalf("members after (none) reply = %v, want empty", got)
	}
}
