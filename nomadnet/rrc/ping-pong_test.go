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
	"bytes"
	"strings"
	"testing"
)

// The PONG regression: answering a hub PING with the PING's source as the
// PONG's sender identity (Field 4) made every real hub attribute the pong to
// ITSELF, treat the client as dead, tear the link down, and spam every room
// member with the client's re-join fanouts (observed live on the RNS
// Community hub). Per the Python SOT (RRC.py:857-863) the pong carries the
// RESPONDER's own identity hash, the body echoed unchanged, no room field,
// and a fresh message id.

func pingFixture(t *testing.T) (*RRCManager, *RRCHub, *[]map[any]any) {
	t.Helper()
	mgr := NewManager(tempDir(t), func() []byte { return []byte("ownhash") })
	mgr.SetNickname("OwnNick")
	hub := mgr.AddHub([]byte("hubhash"), "rrc.chat", "TestHub")
	hub.AddRoom("general")
	sent := &[]map[any]any{}
	hub.onSend = func(env map[any]any) {
		*sent = append(*sent, env)
	}
	return mgr, hub, sent
}

// pingEnvelope builds one inbound hub T_PING envelope with a fixed body.
func pingEnvelope(t *testing.T, body []byte, mid string) []byte {
	t.Helper()
	env := MakeEnvelope(TypePing, []byte("hubhash"), nil, nil, body, []byte(mid), NowMs())
	data, err := EncodeEnvelope(env)
	if err != nil {
		t.Fatalf("EncodeEnvelope: %v", err)
	}
	return data
}

// pongEnvelope builds one inbound T_PONG echo for a pending ping body.
func pongEnvelope(t *testing.T, body []byte) []byte {
	t.Helper()
	env := MakeEnvelope(TypePong, []byte("hubhash"), nil, nil, body, []byte("pongmid"), NowMs())
	data, err := EncodeEnvelope(env)
	if err != nil {
		t.Fatalf("EncodeEnvelope: %v", err)
	}
	return data
}

// TestPongCarriesOwnIdentityAndEchoesBody pins the Python T_PING branch
// (RRC.py:857-863): the pong's Field 4 sender identity is the LOCAL client's
// identity hash, the room field is omitted, the body is echoed back
// byte-for-byte, and the message id is fresh.
func TestPongCarriesOwnIdentityAndEchoesBody(t *testing.T) {
	t.Parallel()

	_, hub, sent := pingFixture(t)

	pingBody := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x01, 0x02, 0x03, 0x04}
	hub.HandleData(pingEnvelope(t, pingBody, "pingmid"))

	pongs := filterByType(*sent, TypePong)
	if len(pongs) != 1 {
		t.Fatalf("sent %v pong envelopes, want 1 (sent: %v)", len(pongs), typesOf(*sent))
	}
	pong := pongs[0]

	if got := byteVal(pong, KeySource); string(got) != "ownhash" {
		t.Errorf("pong sender identity = %q, want %q (the local client's hash, not the hub's)", string(got), "ownhash")
	}
	if _, ok := pong[KeyRoom]; ok {
		t.Error("pong carries a room field, want omitted (Python's pong has no room)")
	}
	if got := byteVal(pong, KeyBody); !bytes.Equal(got, pingBody) {
		t.Errorf("pong body = %v, want the ping body echoed unchanged %v", got, pingBody)
	}
	mid := byteVal(pong, KeyMessageID)
	if len(mid) != 8 {
		t.Errorf("pong mid len = %v, want 8", len(mid))
	}
	if bytes.Equal(mid, []byte("pingmid")) {
		t.Error("pong echoed the ping's mid, want a fresh one (Python _make_envelope generates it)")
	}
}

// TestPongWithoutBodyOmitsBodyField pins the body-absent echo: a PING with no
// body gets a PONG with no body field (Python env.get(K_BODY) → None → the
// envelope omits the field).
func TestPongWithoutBodyOmitsBodyField(t *testing.T) {
	t.Parallel()

	_, hub, sent := pingFixture(t)

	hub.HandleData(pingEnvelope(t, nil, "pingmid"))

	pongs := filterByType(*sent, TypePong)
	if len(pongs) != 1 {
		t.Fatalf("sent %v pong envelopes, want 1", len(pongs))
	}
	if _, ok := pongs[0][KeyBody]; ok {
		t.Error("pong carries a body field for a body-less ping, want omitted")
	}
}

// TestSendPingEnvelopeAndPendingTable pins Python send_ping (RRC.py:592-600):
// the ping envelope carries the local identity as the source and no room
// field, and the 8-byte random body keys the pending-pings table with the
// target room recorded.
func TestSendPingEnvelopeAndPendingTable(t *testing.T) {
	t.Parallel()

	_, hub, sent := pingFixture(t)

	hub.SendPing("general")

	pings := filterByType(*sent, TypePing)
	if len(pings) != 1 {
		t.Fatalf("sent %v ping envelopes, want 1", len(pings))
	}
	ping := pings[0]
	if got := byteVal(ping, KeySource); string(got) != "ownhash" {
		t.Errorf("ping sender = %q, want %q (Python sends the local identity)", string(got), "ownhash")
	}
	if _, ok := ping[KeyRoom]; ok {
		t.Error("ping envelope carries a room field, want omitted (Python RRC.py:594)")
	}
	body := byteVal(ping, KeyBody)
	if len(body) != 8 {
		t.Fatalf("ping body len = %v, want 8 random bytes", len(body))
	}
	hub.lock.Lock()
	pending, ok := hub.pendingPings[string(body)]
	hub.lock.Unlock()
	if !ok {
		t.Fatal("the sent ping body was not recorded in the pending table")
	}
	if pending.room != "general" {
		t.Errorf("pending ping room = %q, want general", pending.room)
	}
}

// TestPongClearsPendingPing pins the pending-table bookkeeping: the PONG's
// echoed body (keyed by the raw body, not a hex form) clears the entry.
func TestPongClearsPendingPing(t *testing.T) {
	t.Parallel()

	_, hub, _ := pingFixture(t)

	hub.SendPing("general")
	hub.lock.Lock()
	if got := len(hub.pendingPings); got != 1 {
		hub.lock.Unlock()
		t.Fatalf("pending pings after send = %v, want 1", got)
	}
	var body string
	for key := range hub.pendingPings {
		body = key
	}
	hub.lock.Unlock()

	hub.HandleData(pongEnvelope(t, []byte(body)))

	hub.lock.Lock()
	defer hub.lock.Unlock()
	if got := len(hub.pendingPings); got != 0 {
		t.Errorf("pending pings after pong = %v, want 0 (the entry is keyed by the raw body)", got)
	}
}

// TestPendingPingsExpireAfter15s pins Python's 15 s expiry: a stale pending
// ping is dropped at the next send.
func TestPendingPingsExpireAfter15s(t *testing.T) {
	t.Parallel()

	_, hub, _ := pingFixture(t)

	hub.lock.Lock()
	hub.pendingPings["staleping"] = pendingPing{sentMs: NowMs() - pingExpiryMs - 1, room: "general"}
	hub.lock.Unlock()

	hub.SendPing("general")

	hub.lock.Lock()
	defer hub.lock.Unlock()
	if _, ok := hub.pendingPings["staleping"]; ok {
		t.Error("stale pending ping survived the expiry sweep")
	}
	if got := len(hub.pendingPings); got != 1 {
		t.Errorf("pending pings after expiry + send = %v, want 1 (the fresh ping)", got)
	}
}

// TestPingPongFlowNeverRenders pins the Python rendering split (RRC.py:865-
// 880): the hub-initiated ping/pong exchange never touches a conversation
// buffer (inbound PING, outbound PONG, an unmatching PONG), while the client's
// OWN answered ping records exactly one "Pong from hub: <rtt> ms" system row
// in the ping's room (RRC.py:878).
func TestPingPongFlowNeverRenders(t *testing.T) {
	t.Parallel()

	mgr, hub, _ := pingFixture(t)
	mgr.SetActive(hub, "general")

	// Hub-initiated ping + our PONG + an unrelated PONG: all silent.
	hub.HandleData(pingEnvelope(t, []byte{1, 2, 3, 4, 5, 6, 7, 8}, "pingmid2"))
	hub.HandleData(pongEnvelope(t, []byte{1, 2, 3, 4, 5, 6, 7, 8}))

	// The client's own ping, answered by a matching PONG: one system row.
	hub.SendPing("general")
	hub.lock.Lock()
	var body string
	for key := range hub.pendingPings {
		body = key
	}
	hub.lock.Unlock()
	hub.HandleData(pongEnvelope(t, []byte(body)))

	msgs := hub.GetMessages("general")
	if len(msgs) != 1 {
		t.Fatalf("room buffer after ping/pong = %v entries, want 1 (the answered pong's RTT row)", len(msgs))
	}
	if msgs[0].Kind != "system" {
		t.Errorf("pong row kind = %q, want system (Python _record_system)", msgs[0].Kind)
	}
	if want := "Pong from hub: "; !strings.HasPrefix(msgs[0].Text, want) {
		t.Errorf("pong row text = %q, want prefix %q", msgs[0].Text, want)
	}
	if !strings.HasSuffix(msgs[0].Text, " ms") {
		t.Errorf("pong row text = %q, want a \" ms\" suffix", msgs[0].Text)
	}

	// A PONG echoing an already-answered (or hub-initiated) body is silent.
	hub.HandleData(pongEnvelope(t, []byte(body)))
	if msgs := hub.GetMessages("general"); len(msgs) != 1 {
		t.Errorf("room buffer after duplicate pong = %v entries, want 1", len(msgs))
	}
	hub.lock.Lock()
	defer hub.lock.Unlock()
	if got := len(hub.Notices); got != 0 {
		t.Errorf("global notices after ping/pong = %v, want 0", got)
	}
}

// filterByType returns the sent envelopes of one message type.
func filterByType(envs []map[any]any, msgType int) []map[any]any {
	out := make([]map[any]any, 0, len(envs))
	for _, env := range envs {
		if intVal(env, KeyType) == msgType {
			out = append(out, env)
		}
	}
	return out
}

// typesOf lists the message types of the sent envelopes, for failure output.
func typesOf(envs []map[any]any) []int {
	out := make([]int, 0, len(envs))
	for _, env := range envs {
		out = append(out, intVal(env, KeyType))
	}
	return out
}
