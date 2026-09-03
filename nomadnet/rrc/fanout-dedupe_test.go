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

	"github.com/gmlewis/go-reticulum/rns"
)

// fanoutFixture builds a hub whose manager identity is fixed, mirroring the
// test style of rrc_test.go.
func fanoutFixture(t *testing.T) (*RRCManager, *RRCHub) {
	t.Helper()
	mgr := NewManager(tempDir(t), func() []byte { return []byte("ownhash") })
	mgr.SetNickname("OwnNick")
	hub := mgr.AddHub([]byte("hubhash"), "rrc.chat", "TestHub")
	hub.AddRoom("test")
	return mgr, hub
}

// fanoutCopy builds one rrcd fanout copy: rrcd 0.3.2 fans each message out
// once per room member with a unique mid, a per-copy rewritten source hash,
// and a registry-derived (often wrong or missing) nick, keeping the body
// and timestamp identical across copies.
func fanoutCopy(src, nick, body string, mid string, ts int64) []byte {
	env := MakeEnvelope(TypeMsg, []byte(src), []byte("test"), []byte(nick), body, []byte(mid), ts)
	data, err := EncodeEnvelope(env)
	if err != nil {
		panic(err)
	}
	return data
}

// TestRecordMessageChronological pins TODO item 1: Python RRCHub
// _record_message APPENDS to the room buffer (RRC.py:790 buf.append(msg)), so
// the buffer is oldest→newest and the UI renders Message 1 before Message 6.
func TestRecordMessageChronological(t *testing.T) {
	t.Parallel()
	mgr, hub := fanoutFixture(t)
	_ = mgr

	base := NowMs()
	for i, body := range []string{"Message 1", "Message 2", "Message 3"} {
		hub.HandleData(fanoutCopy("peerA", "Nick A", body, string(rune('a'+i))+"mid1", base+int64(i)*1000))
	}

	msgs := hub.GetMessages("test")
	if len(msgs) != 3 {
		t.Fatalf("GetMessages len = %v, want 3", len(msgs))
	}
	for i, want := range []string{"Message 1", "Message 2", "Message 3"} {
		if msgs[i].Text != want {
			t.Errorf("msgs[%v].Text = %q, want %q (buffer must be chronological)", i, msgs[i].Text, want)
		}
	}
}

// TestFanoutCopiesCollapseToSingleMessage pins TODO item 4 (user-ordered
// deviation): rrcd's per-member fanout copies — unique mids, rewritten srcs,
// garbage nicks, identical body — render once, not once per copy. Python
// renders every copy; the user wants gonomadnet better here.
func TestFanoutCopiesCollapseToSingleMessage(t *testing.T) {
	t.Parallel()
	mgr, hub := fanoutFixture(t)
	_ = mgr

	base := NowMs()
	// Six fanout copies, exactly like the rrcd Forwarded log: unique mids,
	// varying (wrong) srcs, one copy missing the nick entirely.
	for i, c := range []struct{ src, nick, mid string }{
		{"src1", "Wrong Nick A", "mid1"},
		{"src2", "", "mid2"},
		{"src3", "Wrong Nick B", "mid3"},
		{"src1", "Wrong Nick A", "mid4"},
		{"src2", "", "mid5"},
		{"src3", "", "mid6"},
	} {
		hub.HandleData(fanoutCopy(c.src, c.nick, "Message 5 from penguin", c.mid, base+int64(i)*10))
	}

	msgs := hub.GetMessages("test")
	if len(msgs) != 1 {
		t.Fatalf("GetMessages len = %v, want 1 (fanout copies must collapse)", len(msgs))
	}
	if msgs[0].Text != "Message 5 from penguin" {
		t.Errorf("kept copy text = %q", msgs[0].Text)
	}
}

// TestSelfEchoCollapsesWithLocalRecord pins the second half of TODO item 4:
// after SendMessage the hub's fanout copies of our own message (unique mids,
// possibly rewritten srcs) must not duplicate the local record.
func TestSelfEchoCollapsesWithLocalRecord(t *testing.T) {
	t.Parallel()
	mgr, hub := fanoutFixture(t)
	_ = mgr

	hub.SendMessage("test", "Message 6 from raspberrypi")

	// The hub's fanout echoes back copies with unique mids; rrcd rewrites the
	// src per copy, so even srcs that are not ours can be echoes of our send.
	hub.HandleData(fanoutCopy("ownhash", "OwnNick", "Message 6 from raspberrypi", "hubmid1", time.Now().UnixMilli()))
	hub.HandleData(fanoutCopy("otherpeer", "OtherNick", "Message 6 from raspberrypi", "hubmid2", time.Now().UnixMilli()))

	msgs := hub.GetMessages("test")
	if len(msgs) != 1 {
		t.Fatalf("GetMessages len = %v, want 1 (self echo must collapse with the local record)", len(msgs))
	}
	if msgs[0].Nick != "OwnNick" {
		t.Errorf("nick = %q, want %q (own message renders with our own nick)", msgs[0].Nick, "OwnNick")
	}
}

// TestFanoutWindowExpiry pins the dedupe window: identical bodies arriving
// well after the fanout window are distinct messages and must both render.
func TestFanoutWindowExpiry(t *testing.T) {
	t.Parallel()
	mgr, hub := fanoutFixture(t)
	_ = mgr

	old := NowMs() - 10*60*1000 // ten minutes apart: far outside any window
	hub.HandleData(fanoutCopy("peerA", "Nick A", "ok", "mid1", old))
	hub.HandleData(fanoutCopy("peerB", "Nick B", "ok", "mid2", NowMs()))

	msgs := hub.GetMessages("test")
	if len(msgs) != 2 {
		t.Fatalf("GetMessages len = %v, want 2 (distinct-time copies must not collapse)", len(msgs))
	}
}

// TestDifferentSendersSameBodyWithinWindow: copies from genuinely different
// senders inside the window with different bodies stay separate (the window
// only groups identical bodies).
func TestDifferentSendersSameBodySeparateMessages(t *testing.T) {
	t.Parallel()
	mgr, hub := fanoutFixture(t)
	_ = mgr

	base := NowMs()
	hub.HandleData(fanoutCopy("peerA", "Nick A", "hello", "mid1", base))
	hub.HandleData(fanoutCopy("peerB", "Nick B", "hello there", "mid2", base+100))

	if got := len(hub.GetMessages("test")); got != 2 {
		t.Fatalf("GetMessages len = %v, want 2", got)
	}
}

// TestNoticeFanoutCollapses pins TODO item 5: rrcd's repeated "room …:
// unregistered; mode=(none); topic=(none)" notice copies per join fan out and
// must render once per join, not once per fanout copy.
func TestNoticeFanoutCollapses(t *testing.T) {
	t.Parallel()
	mgr, hub := fanoutFixture(t)
	_ = mgr

	base := NowMs()
	notice := "room test: unregistered; mode=(none); topic=(none)"
	for i := range 3 {
		env := MakeEnvelope(TypeNotice, nil, []byte("test"), nil, notice, []byte{byte(i)}, base+int64(i)*5)
		data, err := EncodeEnvelope(env)
		if err != nil {
			t.Fatal(err)
		}
		hub.HandleData(data)
	}

	msgs := hub.GetMessages("test")
	if len(msgs) != 1 {
		t.Fatalf("GetMessages len = %v, want 1 (notice fanout copies must collapse)", len(msgs))
	}
}

// TestSameBodySameSenderOutsideWindowIsKept pins that two distinct sends of
// the same body by the SAME remote sender, spaced beyond the fanout window,
// both render (the fanout window must not eat legitimate repeats).
func TestSameBodySameSenderOutsideWindowIsKept(t *testing.T) {
	t.Parallel()
	mgr, hub := fanoutFixture(t)
	_ = mgr

	hub.HandleData(fanoutCopy("peerA", "Nick A", "ok", "mid1", NowMs()-5000))
	hub.HandleData(fanoutCopy("peerA", "Nick A", "ok", "mid2", NowMs()))

	if got := len(hub.GetMessages("test")); got != 2 {
		t.Fatalf("GetMessages len = %v, want 2 (repeats outside the window are real messages)", got)
	}
}

// TestLoadHistoryCollapsesFanoutCopies pins the load-time application of the
// fanout collapse: history files written before the fix carry rrcd's
// per-member copies (rewritten srcs, garbage nicks, identical body and ts);
// they must collapse to one message on load.
func TestLoadHistoryCollapsesFanoutCopies(t *testing.T) {
	t.Parallel()
	mgr, hub := fanoutFixture(t)
	_ = mgr
	hub.AddRoom("general")

	ts := NowMs()
	// Two disk copies of one fanout burst: identical body and ts, rewritten
	// srcs and nicks — exactly what the decoded history files show.
	appendHistoryEntry(t, hub, "general", &RRCMessage{
		Kind: "msg", Room: "general", Src: []byte("ownhash"), Nick: "OwnNick",
		Text: "Message 1 from glenn-macm2pro", Ts: ts,
	})
	appendHistoryEntry(t, hub, "general", &RRCMessage{
		Kind: "msg", Room: "general", Src: []byte("peerhash"), Nick: "",
		Text: "Message 1 from glenn-macm2pro", Ts: ts,
	})
	// A distinct-time repeat stays.
	appendHistoryEntry(t, hub, "general", &RRCMessage{
		Kind: "msg", Room: "general", Src: []byte("peerA"), Nick: "Nick A",
		Text: "ok", Ts: ts - 10*60*1000,
	})

	hub.loadHistory()

	msgs := hub.GetMessages("general")
	if len(msgs) != 2 {
		t.Fatalf("loaded messages = %v, want 2 (fanout copies must collapse on load)", len(msgs))
	}
	// File order is preserved on load (the render path sorts by timestamp).
	if msgs[0].Text != "Message 1 from glenn-macm2pro" || msgs[1].Text != "ok" {
		t.Errorf("loaded order/text = %q, %q", msgs[0].Text, msgs[1].Text)
	}
	if msgs[0].Nick != "OwnNick" {
		t.Errorf("kept copy nick = %q, want %q (first-loaded copy wins)", msgs[0].Nick, "OwnNick")
	}
}

// TestLoadedHistoryCollapsesAgainstLiveReplay pins that a join-time history
// replay of an already-loaded message does not duplicate it: rrcd preserves
// the original envelope ts on replay copies, so the replay copy collapses
// against the loaded entry's fanout window.
func TestLoadedHistoryCollapsesAgainstLiveReplay(t *testing.T) {
	t.Parallel()
	mgr, hub := fanoutFixture(t)
	_ = mgr
	hub.AddRoom("general")

	ts := NowMs()
	appendHistoryEntry(t, hub, "general", &RRCMessage{
		Kind: "msg", Room: "general", Src: []byte("peerA"), Nick: "Nick A",
		Text: "History 1", Ts: ts,
	})
	hub.loadHistory()

	hub.HandleData(fanoutCopy("peerB", "", "History 1", "replaymid", ts))

	if got := hub.GetMessages("general"); len(got) != 1 {
		t.Fatalf("GetMessages len = %v, want 1 (replay of a loaded message must collapse)", len(got))
	}
}

// TestMsgCopiesLearnNicksAndMembers pins Python's T_MSG bookkeeping
// (RRC.py:1031-1035): every copy with a source hash and a non-empty nick
// learns nicks[src] and adds src to the room's member set — BEFORE the
// fanout collapse skips rendering, so the member set converges on the
// per-member fanout copies exactly like Python's does.
func TestMsgCopiesLearnNicksAndMembers(t *testing.T) {
	t.Parallel()
	mgr, hub := fanoutFixture(t)
	_ = mgr
	hub.AddRoom("test")

	ts := NowMs()
	// Two fanout copies of one message: each copy carries a (rewritten)
	// member hash and rrcd's registry nick for it.
	hub.HandleData(fanoutCopy("peerA", "Nick A", "hello", "mid1", ts))
	hub.HandleData(fanoutCopy("peerB", "Nick B", "hello", "mid2", ts+10))

	// The copies collapse to ONE rendered message…
	if got := hub.GetMessages("test"); len(got) != 1 {
		t.Fatalf("GetMessages len = %v, want 1", len(got))
	}
	// …but both sources are learned as members and nicks.
	srcA := hexString([]byte("peerA"))
	srcB := hexString([]byte("peerB"))
	members := hub.GetMembers("test")
	found := map[string]bool{}
	for _, m := range members {
		found[m] = true
	}
	if !found[srcA] || !found[srcB] {
		t.Errorf("members = %v, want both %v and %v (fanout srcs are members)", members, srcA, srcB)
	}
	if got := hub.DisplayNameFor([]byte("peerA")); got != "Nick A" {
		t.Errorf("DisplayNameFor(peerA) = %q, want %q", got, "Nick A")
	}
	if got := hub.DisplayNameFor([]byte("peerB")); got != "Nick B" {
		t.Errorf("DisplayNameFor(peerB) = %q, want %q", got, "Nick B")
	}
}

// TestMsgWithoutNickDoesNotLearnMember pins Python's guard: the copy's src
// joins the member set only when the copy carries a non-empty nick
// (RRC.py:1031).
func TestMsgWithoutNickDoesNotLearn(t *testing.T) {
	t.Parallel()
	mgr, hub := fanoutFixture(t)
	_ = mgr
	hub.AddRoom("test")

	hub.HandleData(fanoutCopy("peerA", "", "hello", "mid1", NowMs()))
	if members := hub.GetMembers("test"); len(members) != 0 {
		t.Errorf("members = %v, want none (nickless copies do not learn)", members)
	}
}

// TestOnEstablishedStaysConnectingUntilWelcome pins TODO item 20 (Python
// _on_established + T_WELCOME, RRC.py:415/906): the hub status is CONNECTING
// "Identified, sending HELLO" at link establishment and flips to CONNECTED
// only when the WELCOME envelope arrives.
func TestOnEstablishedStaysConnectingUntilWelcome(t *testing.T) {
	t.Parallel()
	mgr, hub := fanoutFixture(t)
	_ = mgr

	hub.onEstablished(&rns.Link{})

	if got := hub.GetHubStatus(); got != StatusConnecting {
		t.Errorf("status after link establishment = %v, want StatusConnecting", got)
	}
	if got := hub.GetStatusText(); got != "Identified, sending HELLO" {
		t.Errorf("status text = %q, want %q", got, "Identified, sending HELLO")
	}

	welcomeBody := map[any]any{
		BWelcomeHub:    []byte("PyHub"),
		BWelcomeVer:    []byte("0.3.2"),
		BWelcomeCaps:   map[any]any{},
		BWelcomeLimits: map[any]any{LMaxNickBytes: 32},
	}
	env := MakeEnvelope(TypeWelcome, nil, nil, nil, welcomeBody, []byte("mid-w"), NowMs())
	data, err := EncodeEnvelope(env)
	if err != nil {
		t.Fatal(err)
	}
	hub.HandleData(data)

	if got := hub.GetHubStatus(); got != StatusConnected {
		t.Errorf("status after WELCOME = %v, want StatusConnected", got)
	}
}

// TestHandleWelcomeResetsReconnectAttempts pins Python RRC.py:907-908: the
// WELCOME resets the reconnect attempt counter.
func TestHandleWelcomeResetsReconnectAttempts(t *testing.T) {
	t.Parallel()
	mgr, hub := fanoutFixture(t)
	_ = mgr

	hub.lock.Lock()
	hub.reconnectAttempts = 5
	hub.lock.Unlock()

	welcomeBody := map[any]any{BWelcomeHub: []byte("PyHub"), BWelcomeCaps: map[any]any{}}
	env := MakeEnvelope(TypeWelcome, nil, nil, nil, welcomeBody, []byte("mid-w"), NowMs())
	data, err := EncodeEnvelope(env)
	if err != nil {
		t.Fatal(err)
	}
	hub.HandleData(data)

	hub.lock.Lock()
	defer hub.lock.Unlock()
	if hub.reconnectAttempts != 0 {
		t.Errorf("reconnectAttempts = %v, want 0 after WELCOME", hub.reconnectAttempts)
	}
}

// TestFanoutCollapseNickBackfill pins the user-ordered fanout behavior from
// the 2026-09-03 12:32 full-fleet capture (the A2 symptom): the collapse
// keeps the FIRST-arrived copy, and rrcd's per-copy rewritten source hashes
// mean later copies can carry the sender's registry nick even when the kept
// copy's nick is empty — the kept copy renders ONCE, WITH the sender's nick
// (Python learns the (src, nick) pair from every fanout copy before its own
// dedupe, RRC.py:1031-1035). Six fanout copies: first nick="", a later copy
// nick="Nick N" → the kept message's nick backfills to "Nick N".
func TestFanoutCollapseNickBackfill(t *testing.T) {
	t.Parallel()

	mgr, hub := fanoutFixture(t)
	mgr.SetActive(hub, "test")

	base := NowMs()
	hub.HandleData(fanoutCopy("aaaa", "", "Message A2 body", "mid1", base))
	for i := 1; i < 6; i++ {
		nick := ""
		if i >= 2 {
			nick = "Nick N"
		}
		hub.HandleData(fanoutCopy(fmt.Sprintf("src%v", i), nick,
			"Message A2 body", fmt.Sprintf("mid%v", i+1), base+int64(i)*10))
	}

	msgs := hub.GetMessages("test")
	if len(msgs) != 1 {
		t.Fatalf("GetMessages len = %v, want 1 (fanout copies must collapse)", len(msgs))
	}
	if msgs[0].Nick != "Nick N" {
		t.Errorf("kept copy nick = %q, want %q (backfilled from a later copy)", msgs[0].Nick, "Nick N")
	}
}
