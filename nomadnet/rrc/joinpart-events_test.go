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
)

// The join/part event parity with the Python capture (47-mac-mini): join and
// leave events render as "<nick> joined" / "<nick> left" (with the
// 12-hex hash-prefix fallback for unknown nicks), a self-join records
// "You joined #<room>" unless the join was silent (Python T_JOINED
// RRC.py:956-975), and a self-part records nothing (Python T_PARTED
// RRC.py:1012-1015). The events are recorded through the room-message
// pipeline as Kind "system" so the TUI renders them and F8 join/leave

// joinpartFixture builds a hub with a known member nick table and no
// pending joins/parts.
func joinpartFixture(t *testing.T) (*RRCManager, *RRCHub) {
	t.Helper()
	mgr, hub := fanoutFixture(t)
	return mgr, hub
}

// joinedEnvelope builds one inbound T_JOINED fanout like rrcd sends: the body
// is a list of the affected member hashes, with the advisory K_NICK.
func joinedEnvelope(t *testing.T, room string, src []byte, nick string, bodyHashes [][]byte) []byte {
	t.Helper()
	body := make([]any, 0, len(bodyHashes))
	for _, hb := range bodyHashes {
		body = append(body, hb)
	}
	env := MakeEnvelope(TypeJoined, src, []byte(room), []byte(nick), body, MsgID(), NowMs())
	data, err := EncodeEnvelope(env)
	if err != nil {
		t.Fatalf("EncodeEnvelope: %v", err)
	}
	return data
}

// partedEnvelope builds one inbound T_PARTED fanout like rrcd sends.
func partedEnvelope(t *testing.T, room string, src []byte, nick string, bodyHashes [][]byte) []byte {
	t.Helper()
	body := make([]any, 0, len(bodyHashes))
	for _, hb := range bodyHashes {
		body = append(body, hb)
	}
	env := MakeEnvelope(TypeParted, src, []byte(room), []byte(nick), body, MsgID(), NowMs())
	data, err := EncodeEnvelope(env)
	if err != nil {
		t.Fatalf("EncodeEnvelope: %v", err)
	}
	return data
}

// roomTexts returns the recorded texts of a room's buffer, for assertions.
func roomTexts(hub *RRCHub, room string) []string {
	msgs := hub.GetMessages(room)
	texts := make([]string, 0, len(msgs))
	for _, m := range msgs {
		texts = append(texts, m.Text)
	}
	return texts
}

// TestOtherJoinWithKnownNickRecordsEvent pins the other-join event with the
// advisory K_NICK: the recorded text is "<nick> joined" and the existing
// member/nick updates are unchanged.
func TestOtherJoinWithKnownNickRecordsEvent(t *testing.T) {
	t.Parallel()

	_, hub := joinpartFixture(t)

	hub.HandleData(joinedEnvelope(t, "test", []byte("hubsrc"), "CarL", [][]byte{[]byte("carlhash")}))

	texts := roomTexts(hub, "test")
	if len(texts) != 1 {
		t.Fatalf("recorded events = %v, want exactly one join event", texts)
	}
	if want := "CarL joined"; texts[0] != want {
		t.Errorf("join event text = %q, want %q", texts[0], want)
	}
	msgs := hub.GetMessages("test")
	if msgs[0].Kind != "system" {
		t.Errorf("join event kind = %q, want system", msgs[0].Kind)
	}
	if msgs[0].Room != "test" {
		t.Errorf("join event room = %q, want test", msgs[0].Room)
	}
	// The existing bookkeeping is unchanged: the member set and nick table.
	if members := hub.GetMembers("test"); len(members) != 2 {
		t.Errorf("members = %v, want the joiner plus the own hash", members)
	}
	if got := hub.DisplayNameFor([]byte("carlhash")); got != "CarL" {
		t.Errorf("learned nick = %q, want CarL", got)
	}
}

// TestOtherJoinWithUnknownNickHashFallback pins display_name_for parity: with
// no advisory nick the recorded text uses the 12-hex prefix of the joiner's
// identity hash.
func TestOtherJoinWithUnknownNickHashFallback(t *testing.T) {
	t.Parallel()

	_, hub := joinpartFixture(t)

	// The member hash bytes decode from the 12-hex prefix "0a8b370a62de".
	hub.HandleData(joinedEnvelope(t, "test", []byte("hubsrc"), "", [][]byte{mustHex(t, "0a8b370a62de")}))

	texts := roomTexts(hub, "test")
	if len(texts) != 1 {
		t.Fatalf("recorded events = %v, want exactly one", texts)
	}
	if want := "0a8b370a62de joined"; texts[0] != want {
		t.Errorf("join event text = %q, want %q (12-hex hash fallback)", texts[0], want)
	}
}

// TestSelfJoinRecordsYouJoined pins Python T_JOINED's self-join record
// (RRC.py:956-958): "You joined #<room>", recorded through the room pipeline.
func TestSelfJoinRecordsYouJoined(t *testing.T) {
	t.Parallel()

	mgr, hub := joinpartFixture(t)
	mgr.SetActive(hub, "test")

	// A pending self-join (JoinRoom arms it; the link is nil so the send is
	// a no-op after the onSend hook).
	hub.onSend = func(map[any]any) {}
	hub.JoinRoom("general", false)

	hub.HandleData(joinedEnvelope(t, "general", []byte("hubsrc"), "OwnNick", [][]byte{[]byte("ownhash")}))

	texts := roomTexts(hub, "general")
	if len(texts) != 1 {
		t.Fatalf("recorded events = %v, want exactly the self-join event", texts)
	}
	if want := "You joined #general"; texts[0] != want {
		t.Errorf("self-join text = %q, want %q (Python RRC.py:957)", texts[0], want)
	}
	// The pending-join marker is consumed and the room stays joined.
	if hub.HasRoom("general") != true {
		t.Error("self-join did not keep the room joined")
	}
}

// TestSilentSelfJoinRecordsNothing pins the silent-join suppression
// (Python RRC.py:958 `if not silent`): the WELCOME re-join loop records no
// "You joined" event per room.
func TestSilentSelfJoinRecordsNothing(t *testing.T) {
	t.Parallel()

	_, hub := joinpartFixture(t)

	hub.onSend = func(map[any]any) {}
	hub.JoinRoom("general", true)

	hub.HandleData(joinedEnvelope(t, "general", []byte("hubsrc"), "OwnNick", [][]byte{[]byte("ownhash")}))

	if texts := roomTexts(hub, "general"); len(texts) != 0 {
		t.Errorf("silent self-join recorded %v, want nothing", texts)
	}
}

// TestOtherPartRecordsEvent pins the other-part event with the advisory nick:
// the recorded text is "<nick> left", and the parter is dropped from the
// member set (existing behavior unchanged).
func TestOtherPartRecordsEvent(t *testing.T) {
	t.Parallel()

	_, hub := joinpartFixture(t)

	// A member joins first (nick learned via the advisory K_NICK), then parts.
	hub.HandleData(joinedEnvelope(t, "test", []byte("hubsrc"), "CarL", [][]byte{[]byte("carlhash")}))
	hub.HandleData(partedEnvelope(t, "test", []byte("hubsrc"), "CarL", [][]byte{[]byte("carlhash")}))

	texts := roomTexts(hub, "test")
	if len(texts) != 2 {
		t.Fatalf("recorded events = %v, want the join and part events", texts)
	}
	if want := "CarL left"; texts[1] != want {
		t.Errorf("part event text = %q, want %q", texts[1], want)
	}
	if members := hub.GetMembers("test"); len(members) != 1 {
		t.Errorf("members after part = %v, want only the own hash", members)
	}
}

// TestSelfPartRecordsNothing pins Python T_PARTED's self-part parity
// (RRC.py:1012-1015): a self-part records no event.
func TestSelfPartRecordsNothing(t *testing.T) {
	t.Parallel()

	_, hub := joinpartFixture(t)
	hub.AddRoom("general")

	hub.onSend = func(map[any]any) {}
	hub.PartRoom("general")

	hub.HandleData(partedEnvelope(t, "general", []byte("hubsrc"), "OwnNick", [][]byte{[]byte("ownhash")}))

	if texts := roomTexts(hub, "general"); len(texts) != 0 {
		t.Errorf("self-part recorded %v, want nothing (Python parity)", texts)
	}
	if hub.HasRoom("general") {
		t.Error("self-part did not drop the room")
	}
}

// TestMultiHashJoinedRecordsNothing pins Python's joiner=None case: a JOINED
// fanout about more than one member (or about our own hash only) records no
// event.
func TestMultiHashJoinedRecordsNothing(t *testing.T) {
	t.Parallel()

	_, hub := joinpartFixture(t)

	hub.HandleData(joinedEnvelope(t, "test", []byte("hubsrc"), "Nick",
		[][]byte{[]byte("peerA"), []byte("peerB")}))

	if texts := roomTexts(hub, "test"); len(texts) != 0 {
		t.Errorf("multi-member JOINED recorded %v, want nothing", texts)
	}
}
