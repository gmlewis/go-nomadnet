//go:build integration

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

// Integration tests for the RRC Channels fixes from the full-fleet A/B
// re-deploy diff (the 2026-09-03 12:32 TODO section), driven through a REAL
// Python RRC hub (testdata/mini_hub.py) over a loopback TCP RNS transport -
// no tmux sessions, no live fleet. The mini hub's fanout/MOTD test hooks
// replay the rrcd 0.3.2 per-member fanout burst and the global MOTD notice
// the Python source-of-truth rode on (RRC.py:1031-1035, 1128-1136).

package rrc

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gmlewis/go-reticulum/rns"
	"github.com/gmlewis/go-reticulum/testutils"
)

// startIntegrationHub spawns the Python mini hub, connects a Go client
// RRCHub to it over a loopback TCP RNS transport, waits for the link to be
// Connected+Welcomed, joins the "general" room as the active room, and
// returns the client hub, the hub's event log path, and a cleanup func.
func startIntegrationHub(t *testing.T) (*RRCHub, string, func()) {
	t.Helper()
	port := freePortRRC(t)
	hubHash, hubLog, hubCleanup := startPythonMiniHub(t, port)
	defer func() {
		if t.Failed() {
			t.Logf("mini hub events: %v", readMiniHubEvents(t, hubLog))
		}
	}()

	dir, err := os.MkdirTemp("/tmp", "nomadnet-rrc-int")
	if err != nil {
		t.Fatal(err)
	}
	cfgDir := dir + "/config"
	writeRNSConfigRRC(t, cfgDir)
	appendTCPClientInterface(t, cfgDir+"/config", port)

	ts := rns.NewTransportSystem(nil)
	if _, err := rns.NewReticulum(ts, cfgDir); err != nil {
		t.Fatalf("NewReticulum: %v", err)
	}

	mgr := NewManager(dir+"/storage", func() []byte { return ts.Identity().Hash })
	mgr.SetNickname("GoClient")
	mgr.SetTransport(ts)
	hubHashBytes, err := hex.DecodeString(hubHash)
	if err != nil {
		t.Fatalf("hub hash: %v", err)
	}
	hub := mgr.AddHub(hubHashBytes, "rrc.hub", "MiniHub")
	hub.ConnectAsync()

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		hub.lock.Lock()
		status, welcomed := hub.Status, hub.Welcomed
		hub.lock.Unlock()
		if welcomed && status == StatusConnected {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	hub.lock.Lock()
	status := hub.Status
	hub.lock.Unlock()
	if status != StatusConnected {
		t.Fatalf("hub never reached Connected")
	}

	// Join the room and make it the client's ACTIVE room so roomless
	// notices attribute to it (Python _record_notice, RRC.py:817-824).
	hub.JoinRoom("general", false)
	mgr.SetActive(hub, "general")
	// The 500ms settle this poll replaces existed so the hub had processed
	// the JOIN before tests send messages; poll the hub's event log for the
	// received T_JOIN packet instead.
	if !testutils.PollUntil(10*time.Second, func() bool {
		for _, ev := range readMiniHubEvents(t, hubLog) {
			if ev["event"] == "packet" && ev["type"] == float64(TypeJoin) {
				return true
			}
		}
		return false
	}) {
		t.Fatalf("hub never received the T_JOIN for 'general'; events: %v", readMiniHubEvents(t, hubLog))
	}

	return hub, hubLog, func() {
		hubCleanup()
		_ = os.RemoveAll(dir)
	}
}

// waitMsgs polls the room's buffer until want returns true or times out.
func waitMsgs(t *testing.T, hub *RRCHub, room string, want func([]*RRCMessage) bool, timeout time.Duration) []*RRCMessage {
	t.Helper()
	snapshot := func() []*RRCMessage {
		hub.lock.Lock()
		defer hub.lock.Unlock()
		msgs := make([]*RRCMessage, len(hub.Messages[room]))
		copy(msgs, hub.Messages[room])
		return msgs
	}
	var msgs []*RRCMessage
	if !testutils.PollUntil(timeout, func() bool {
		msgs = snapshot()
		return want(msgs)
	}) {
		t.Fatalf("timeout waiting on %v messages; have %v", room, len(snapshot()))
	}
	return msgs
}

// TestIntegrationFanoutBurst replays the rrcd 0.3.2 per-member fanout burst
// through the wire (the 2026-09-03 12:32 A2 capture): the client sends the
// mini hub's FANOUT trigger, the hub fans out six per-member copies with
// per-copy rewritten source hashes - copy 0 carrying NO nick, copies 1-5
// carrying registry nicks, all sharing the sender's stale envelope
// timestamp (RRC.py:1031-1035: Python learns (src, nick) from EVERY copy
// before its own dedupe). Asserts: the burst collapses to ONE recorded
// message, the kept copy's nick backfills from a nicked copy, the recorded
// timestamp is the ARRIVAL time (not the stale envelope ts), and the
// nicked copies are all learned in the room's member set.
func TestIntegrationFanoutBurst(t *testing.T) {
	t.Parallel()
	hub, _, cleanup := startIntegrationHub(t)
	defer cleanup()

	stale := NowMs() - 60_000
	before := NowMs()
	hub.SendMessage("general", fmt.Sprintf("FANOUT:6:%d:burst body", stale))

	msgs := waitMsgs(t, hub, "general", func(m []*RRCMessage) bool {
		n := 0
		for _, m := range m {
			if m.Text == "burst body" {
				n++
			}
		}
		return n == 1
	}, 15*time.Second)

	var kept *RRCMessage
	for _, m := range msgs {
		if m.Text == "burst body" {
			kept = m
		}
	}
	if kept == nil {
		t.Fatal("kept fanout copy not found")
	}
	// The collapse keeps the FIRST-arrived copy (copy 0, no nick) and the
	// later nicked copies backfill its empty nick (the A2 capture symptom:
	// the kept copy rendered the bare <hash>).
	if kept.Nick != "HubNick1" {
		t.Errorf("kept copy nick = %q, want %q (backfilled from a nicked copy)", kept.Nick, "HubNick1")
	}
	// Python stamps every inbound message with its own arrival time
	// (RRC.py:1043) - the stale envelope ts must not be used.
	if kept.Ts < before-2_000 || kept.Ts > before+15_000+2_000 {
		t.Errorf("kept copy Ts = %v, want the arrival time (~%v), not the stale envelope ts %v", kept.Ts, before, stale)
	}
}

// TestIntegrationFanoutNickLearning pins the member/nick learning coverage
// on the fanout burst: every NICKED copy learns nicks[src] = nick and adds
// src to the room's member set (Python RRC.py:1031-1035 - the learning runs
// before the collapse); the empty-nick copies carry no nick to learn.
// TestIntegrationFanoutNickLearning pins the member/nick learning coverage
// on the fanout burst: every NICKED copy learns nicks[src] = nick and adds
// src to the room's member set (Python RRC.py:1031-1035 - the learning runs
// before the collapse); the empty-nick copies carry no nick to learn.
func TestIntegrationFanoutNickLearning(t *testing.T) {
	t.Parallel()
	hub, hubLog, cleanup := startIntegrationHub(t)
	defer cleanup()

	// The mini hub's per-copy rewritten source hashes are derived from its
	// identity hash (identity.hash[:28] + byte(i) + identity.hash[29:32]),
	// which the hub announces in its event log's announce records.
	idHash := ""
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && idHash == "" {
		for _, ev := range readMiniHubEvents(t, hubLog) {
			if h, ok := ev["id-hash"].(string); ok && h != "" {
				idHash = h
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	if idHash == "" {
		t.Fatal("hub id-hash never announced")
	}
	// The mini hub's identity hash is 16 bytes (32 hex chars); its
	// per-copy rewritten source is the identity hash plus the copy index
	// byte (identity.hash[:28] + byte(i) + identity.hash[29:32] on the
	// Python side - 17 bytes, 34 hex chars).
	srcHex := func(i int) string {
		return idHash + fmt.Sprintf("%02x", i)
	}

	stale := NowMs() - 60_000
	hub.SendMessage("general", fmt.Sprintf("FANOUT:4:%d:learn burst", stale))

	wantNicks := map[string]string{
		srcHex(1): "HubNick1",
		srcHex(2): "HubNick2",
		srcHex(3): "HubNick3",
	}
	emptySrcs := []string{srcHex(0)}

	deadline = time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		hub.lock.Lock()
		missing := ""
		for s, nick := range wantNicks {
			if hub.Nicks[s] != nick {
				missing = s
				break
			}
			if !hub.Members["general"][s] {
				missing = s
				break
			}
		}
		hub.lock.Unlock()
		if missing == "" {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	hub.lock.Lock()
	for s, nick := range wantNicks {
		if got := hub.Nicks[s]; got != nick {
			t.Errorf("nicks[%v] = %q, want %q (Python learns from every nicked copy, RRC.py:1031-1035)", s, got, nick)
		}
		if !hub.Members["general"][s] {
			t.Errorf("members[general] misses %v (Python adds every nicked copy's src)", s)
		}
	}
	// The empty-nick copies learn nothing (the learning block is gated on
	// the nick, RRC.py:1031-1035).
	for _, s := range emptySrcs {
		if got, ok := hub.Nicks[s]; ok {
			t.Errorf("nicks[%v] = %q, want absent (an empty-nick copy carries no nick)", s, got)
		}
		if hub.Members["general"][s] {
			t.Errorf("members[general] has %v; an empty-nick copy must not learn", s)
		}
	}
	// The burst collapses to ONE recorded message whose nick backfills
	// from a nicked copy and whose timestamp is the ARRIVAL time, not the
	// stale envelope ts (RRC.py:1043).
	msgs := make([]*RRCMessage, len(hub.Messages["general"]))
	copy(msgs, hub.Messages["general"])
	hub.lock.Unlock()

	n := 0
	var kept *RRCMessage
	for _, m := range msgs {
		if m.Text == "learn burst" {
			n++
			kept = m
		}
	}
	if n != 1 {
		t.Fatalf("learn-burst copies recorded = %v, want 1 (the collapse)", n)
	}
	if kept.Nick != "HubNick1" {
		t.Errorf("kept copy nick = %q, want HubNick1 (backfilled from a nicked copy)", kept.Nick)
	}
	if kept.Ts < time.Now().Add(-15_000*time.Millisecond).UnixMilli() {
		t.Errorf("kept copy Ts = %v, want the arrival time (not the stale envelope ts %v)", kept.Ts, stale)
	}
}

// TestIntegrationRoomlessMOTDNotice pins Python's roomless MOTD notice
// attribution (RRC.py:817-839, 1128-1136): a roomless T_NOTICE sets the
// hub's MOTD AND joins the ACTIVE room's buffer as a notice - the hub's
// global welcome must render IN the open room.
func TestIntegrationRoomlessMOTDNotice(t *testing.T) {
	t.Parallel()
	hub, _, cleanup := startIntegrationHub(t)
	defer cleanup()

	const welcome = "Welcome to the MiniHub!"
	hub.SendMessage("general", "MOTD:"+welcome)

	msgs := waitMsgs(t, hub, "general", func(m []*RRCMessage) bool {
		for _, msg := range m {
			if msg.Kind == "notice" && strings.Contains(msg.Text, welcome) {
				return true
			}
		}
		return false
	}, 15*time.Second)

	for _, m := range msgs {
		if m.Kind == "notice" && strings.Contains(m.Text, welcome) {
			if m.Room != "general" {
				t.Errorf("roomless notice room = %q, want general (attributed to the active room)", m.Room)
			}
			return
		}
	}
	t.Error("the roomless MOTD notice never landed in the active room's buffer")

	hub.lock.Lock()
	motd := hub.MOTD
	hub.lock.Unlock()
	if motd != welcome {
		t.Errorf("MOTD = %q, want %q", motd, welcome)
	}
}

// TestIntegrationJoinedHeal pins Python's JOINED bookkeeping (RRC.py:944-948)
// over the wire: a T_JOINED whose body carries the FULL member hash list
// adds EVERY body hash to the room's member set, healing the whole set
// client-side.
func TestIntegrationJoinedHeal(t *testing.T) {
	t.Parallel()
	hub, _, cleanup := startIntegrationHub(t)
	defer cleanup()

	const (
		a = "464360ee59ed0000000000000000000000000000000000000000000000000000"
		b = "1ab1ed2d6293000000000000000000000000000000000000000000000000000000"
		c = "d252c38c2e7b000000000000000000000000000000000000000000000000000000"
	)
	hub.SendMessage("general", "JOINED-FANOUT:"+a+","+b+","+c)

	// The JOINED fanout heals the member set client-side; it does not
	// append a chat message (Python RRC.py:944-948 only touches members).
	deadline := time.Now().Add(15 * time.Second)
	healed := false
	for time.Now().Before(deadline) {
		hub.lock.Lock()
		healed = true
		for _, hash := range []string{a, b, c} {
			if !hub.Members["general"][hash] {
				healed = false
			}
		}
		hub.lock.Unlock()
		if healed {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	hub.lock.Lock()
	defer hub.lock.Unlock()
	for _, hash := range []string{a, b, c} {
		if !hub.Members["general"][hash] {
			t.Errorf("members[general] misses %v after the full-list JOINED", hash)
		}
	}
}
