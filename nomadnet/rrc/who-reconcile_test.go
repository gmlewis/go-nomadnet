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
	"fmt"
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

// TestAutoWhoMarkerConsumesEveryReplyCopy pins the rrcd 0.3.2 fanout fix: the
// hub fans the who-notice out once per room member, so the auto-request
// marker must consume EVERY reply copy (a one-shot delete on the first copy
// leaked the marker and the duplicates flooded the conversation window every
// 60 s sweep — observed live). The marker stays set after the burst.
func TestAutoWhoMarkerConsumesEveryReplyCopy(t *testing.T) {
	t.Parallel()

	_, hub, _ := reconcileFixture(t)
	hub.markAutoWhoRequest("test")

	for i := range 5 {
		hub.HandleData(noticeEnvelope(t, "", "members in test: qbit (b253938bf730)", fmt.Sprintf("whomid%v", i)))
	}

	if got := len(hub.GetMessages("test")); got != 0 {
		t.Errorf("who replies recorded = %v entries, want 0 (every copy consumed silently)", got)
	}
	hub.lock.Lock()
	sticky := hub.silentWhoRooms["test"]
	hub.lock.Unlock()
	if !sticky {
		t.Error("auto-request marker was deleted by a reply copy, want sticky until cleared")
	}
}

// TestUserWhoReplyRendersAndClearsMarkers pins the user-initiated /who path:
// the reply renders through the normal notice pipeline and answering it
// clears both the user marker and the room's auto marker.
func TestUserWhoReplyRendersAndClearsMarkers(t *testing.T) {
	t.Parallel()

	_, hub, sent := reconcileFixture(t)
	hub.AddRoom("general")

	if _, err := hub.SendUserCommand("/who", "general"); err != nil {
		t.Fatalf("SendUserCommand: %v", err)
	}
	// The command went out as a T_MSG with the command text.
	if len(*sent) != 1 || (*sent)[0] != "/who" {
		t.Fatalf("sent command bodies = %v, want [\"/who\"]", *sent)
	}
	hub.lock.Lock()
	pending := hub.userWhoRooms["general"]
	hub.lock.Unlock()
	if !pending {
		t.Fatal("user /who did not mark the room as user-requested")
	}

	hub.HandleData(noticeEnvelope(t, "", "members in general: qbit (b253938bf730)", "whomid-u"))

	msgs := hub.GetMessages("general")
	if len(msgs) != 1 {
		t.Fatalf("user who reply recorded = %v entries, want 1 (rendered)", len(msgs))
	}
	if msgs[0].Kind != "notice" || msgs[0].Text != "members in general: qbit (b253938bf730)" {
		t.Errorf("recorded who reply = (%v, %q)", msgs[0].Kind, msgs[0].Text)
	}
	hub.lock.Lock()
	defer hub.lock.Unlock()
	if hub.userWhoRooms["general"] {
		t.Error("user marker not cleared when the reply was answered")
	}
	if hub.silentWhoRooms["general"] {
		t.Error("auto marker not cleared when the user's who was answered")
	}
}

// TestUserWhoBeatsStaleAutoMarker pins the distinct bookkeeping: a user
// -initiated /who for a room whose auto marker was left stale still renders
// its reply (the auto marker can never swallow a user request).
func TestUserWhoBeatsStaleAutoMarker(t *testing.T) {
	t.Parallel()

	_, hub, _ := reconcileFixture(t)
	hub.AddRoom("general")
	// A stale auto marker left behind by an earlier sweep.
	hub.markAutoWhoRequest("general")

	if _, err := hub.SendUserCommand("/who general", ""); err != nil {
		t.Fatalf("SendUserCommand: %v", err)
	}

	hub.HandleData(noticeEnvelope(t, "", "members in general: qbit (b253938bf730)", "whomid-s"))

	if got := len(hub.GetMessages("general")); got != 1 {
		t.Fatalf("user who reply recorded = %v entries, want 1 (a stale auto marker must not swallow it)", got)
	}
}

// TestBareUserWhoWithoutActiveRoomClearsAutoMarkers pins the no-target case:
// a bare "/who" with no active room cannot know its target room ahead of the
// reply, so every stale auto marker is dropped at request time.
func TestBareUserWhoWithoutActiveRoomClearsAutoMarkers(t *testing.T) {
	t.Parallel()

	_, hub, _ := reconcileFixture(t)
	hub.markAutoWhoRequest("test")
	hub.markAutoWhoRequest("test3")

	if _, err := hub.SendUserCommand("/who", ""); err != nil {
		t.Fatalf("SendUserCommand: %v", err)
	}

	hub.lock.Lock()
	got := len(hub.silentWhoRooms)
	hub.lock.Unlock()
	if got != 0 {
		t.Errorf("auto markers after a targetless user /who = %v, want 0", got)
	}
}

// TestSendMessageRoutesSlashCommand pins the composer path: slash-prefixed
// text typed in the room composer routes through the user-command path (the
// reply renders, the command text itself is not recorded as chat).
func TestSendMessageRoutesSlashCommand(t *testing.T) {
	t.Parallel()

	_, hub, _ := reconcileFixture(t)
	hub.AddRoom("general")

	mid := hub.SendMessage("general", "/who")

	if mid == "" {
		t.Fatal("SendMessage returned an empty mid for a slash command")
	}
	if texts := roomTexts(hub, "general"); len(texts) != 0 {
		t.Errorf("slash command recorded as chat: %v, want none", texts)
	}
	hub.lock.Lock()
	pending := hub.userWhoRooms["general"]
	hub.lock.Unlock()
	if !pending {
		t.Error("SendMessage(\"/who\") did not mark the user-request marker")
	}

	// The reply renders (the user's request must not be swallowed).
	hub.HandleData(noticeEnvelope(t, "", "members in general: qbit (b253938bf730)", "whomid-c"))
	if got := len(hub.GetMessages("general")); got != 1 {
		t.Fatalf("user who reply via SendMessage path = %v entries, want 1 (rendered)", got)
	}
}

// TestLinkCloseClearsWhoMarkers pins the marker cleanup on link close: both
// the auto markers and the user-request markers drop with the link.
func TestLinkCloseClearsWhoMarkers(t *testing.T) {
	t.Parallel()

	_, hub, _ := reconcileFixture(t)
	hub.markAutoWhoRequest("test")
	hub.lock.Lock()
	hub.userWhoRooms["test"] = true
	hub.lock.Unlock()

	hub.onClosed()

	hub.lock.Lock()
	defer hub.lock.Unlock()
	if len(hub.silentWhoRooms) != 0 || len(hub.userWhoRooms) != 0 {
		t.Errorf("who markers after link close: silent=%v user=%v, want both empty",
			hub.silentWhoRooms, hub.userWhoRooms)
	}
}

// TestWhoReplyViaResourceConsumedSilently verifies that large /who replies
// delivered via Reticulum Resource transfer (common for rooms with many
// members like #general on public hubs) are processed through ParseWhoNotice
// and consumed silently when an auto-who / silent-who request is outstanding.
func TestWhoReplyViaResourceConsumedSilently(t *testing.T) {
	t.Parallel()

	_, hub, _ := reconcileFixture(t)
	hub.AddRoom("general")
	hub.markAutoWhoRequest("general")

	payload := []byte("members in general: Alice (112233445566), Bob (aabbccddeeff)")
	hub.lock.Lock()
	hub.resourceExpectations["rid-who-silent"] = &resourceExpectation{
		kind:     ResKindNotice,
		size:     len(payload),
		room:     "general",
		encoding: "utf-8",
		expires:  time.Now().Add(30 * time.Second),
	}
	hub.lock.Unlock()

	hub.handleConcludedResource(payload)

	// The reply must NOT hit the message log.
	if msgs := hub.GetMessages("general"); len(msgs) != 0 {
		t.Fatalf("who reply via resource was recorded as %v messages, want 0 (silently consumed)", len(msgs))
	}

	// The membership must have been updated.
	members := hub.GetMembers("general")
	if len(members) != 2 {
		t.Fatalf("members after resource who reply = %v, want 2", members)
	}
}

// TestWhoReplyViaResourceUserInitiatedRenders verifies that user-initiated
// /who replies delivered via Resource transfer are rendered in the room.
func TestWhoReplyViaResourceUserInitiatedRenders(t *testing.T) {
	t.Parallel()

	_, hub, _ := reconcileFixture(t)
	hub.AddRoom("general")
	hub.lock.Lock()
	hub.userWhoRooms["general"] = true
	hub.lock.Unlock()

	payload := []byte("members in general: Alice (112233445566), Bob (aabbccddeeff)")
	hub.lock.Lock()
	hub.resourceExpectations["rid-who-user"] = &resourceExpectation{
		kind:     ResKindNotice,
		size:     len(payload),
		room:     "general",
		encoding: "utf-8",
		expires:  time.Now().Add(30 * time.Second),
	}
	hub.lock.Unlock()

	hub.handleConcludedResource(payload)

	// The user-initiated reply must render in the message log.
	msgs := hub.GetMessages("general")
	if len(msgs) != 1 {
		t.Fatalf("user who reply via resource recorded %v messages, want 1", len(msgs))
	}
	if msgs[0].Text != string(payload) {
		t.Errorf("rendered message text = %q, want %q", msgs[0].Text, string(payload))
	}
}

// TestRoomListReplyViaResourceConsumedSilently verifies that room list
// replies delivered via Resource transfer are parsed and consumed silently
// when an auto-list request is pending.
func TestRoomListReplyViaResourceConsumedSilently(t *testing.T) {
	t.Parallel()

	_, hub, _ := reconcileFixture(t)
	hub.lock.Lock()
	hub.silentListPending = 1
	hub.lock.Unlock()

	payload := []byte("Registered public rooms:\n  #general - General discussion\n  #test")
	hub.lock.Lock()
	hub.resourceExpectations["rid-list-silent"] = &resourceExpectation{
		kind:     ResKindNotice,
		size:     len(payload),
		room:     "",
		encoding: "utf-8",
		expires:  time.Now().Add(30 * time.Second),
	}
	hub.lock.Unlock()

	hub.handleConcludedResource(payload)

	if msgs := hub.GetMessages(""); len(msgs) != 0 {
		t.Fatalf("list reply via resource was recorded as %v messages, want 0", len(msgs))
	}

	hub.lock.Lock()
	rooms := len(hub.AvailableRooms)
	hub.lock.Unlock()
	if rooms != 2 {
		t.Fatalf("available rooms after resource list reply = %v, want 2", rooms)
	}
}
