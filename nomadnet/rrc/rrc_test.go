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
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/gmlewis/go-reticulum/rns"
)

func TestProtocolConstants(t *testing.T) {
	t.Parallel()

	if RRCVersion != 1 {
		t.Errorf("RRCVersion = %v, want 1", RRCVersion)
	}
	if TypeHello != 1 {
		t.Errorf("TypeHello = %v, want 1", TypeHello)
	}
	if TypeWelcome != 2 {
		t.Errorf("TypeWelcome = %v, want 2", TypeWelcome)
	}
	if TypeMsg != 20 {
		t.Errorf("TypeMsg = %v, want 20", TypeMsg)
	}
	if TypeResourceEnvelope != 50 {
		t.Errorf("TypeResourceEnvelope = %v, want 50", TypeResourceEnvelope)
	}
}

func TestRRCMessageHistoryEntry(t *testing.T) {
	t.Parallel()

	msg := &RRCMessage{
		Kind: "msg",
		Room: "#test",
		Src:  []byte{0x01, 0x02},
		Nick: "Alice",
		Text: "Hello",
		Ts:   1234567890,
	}

	entry := msg.HistoryEntry()

	if entry[HKind] != "msg" {
		t.Errorf("kind = %v, want %q", entry[HKind], "msg")
	}
	if entry[HTS] != int64(1234567890) {
		t.Errorf("ts = %v, want %v", entry[HTS], 1234567890)
	}
	if entry[HText] != "Hello" {
		t.Errorf("text = %v, want %q", entry[HText], "Hello")
	}
	if entry[HNick] != "Alice" {
		t.Errorf("nick = %v, want %q", entry[HNick], "Alice")
	}
	if src, ok := entry[HSrc].([]byte); !ok || len(src) != 2 {
		t.Errorf("src = %v, want [1 2]", entry[HSrc])
	}
}

func TestDecodeHistoryEntry(t *testing.T) {
	t.Parallel()

	entry := map[string]any{
		HKind:    "action",
		HTS:      int64(9876543210),
		HText:    "waves",
		HNick:    "Bob",
		HSrc:     []byte{0x03, 0x04},
		HMention: true,
	}

	msg := DecodeHistoryEntry(entry)

	if msg.Kind != "action" {
		t.Errorf("Kind = %q, want %q", msg.Kind, "action")
	}
	if msg.Ts != 9876543210 {
		t.Errorf("Ts = %v, want %v", msg.Ts, 9876543210)
	}
	if msg.Text != "waves" {
		t.Errorf("Text = %q, want %q", msg.Text, "waves")
	}
	if msg.Nick != "Bob" {
		t.Errorf("Nick = %q, want %q", msg.Nick, "Bob")
	}
	if !msg.Mention {
		t.Error("Mention = false, want true")
	}
}

func TestMakeEnvelope(t *testing.T) {
	t.Parallel()

	src := []byte{0x01, 0x02}
	room := []byte("#test")
	nick := []byte("Alice")
	body := "Hello"
	mid := []byte{0x0A, 0x0B}
	ts := int64(1234567890)

	env := MakeEnvelope(TypeMsg, src, room, nick, body, mid, ts)

	if env[KeyVersion] != RRCVersion {
		t.Errorf("version = %v, want %v", env[KeyVersion], RRCVersion)
	}
	if env[KeyType] != TypeMsg {
		t.Errorf("type = %v, want %v", env[KeyType], TypeMsg)
	}
	if env[KeyTimestamp] != ts {
		t.Errorf("timestamp = %v, want %v", env[KeyTimestamp], ts)
	}
	if env[KeyRoom] == nil {
		t.Error("room is nil")
	}
	if env[KeyNick] == nil {
		t.Error("nick is nil")
	}
	if env[KeyBody] != body {
		t.Errorf("body = %v, want %q", env[KeyBody], body)
	}
}

func TestEncodeDecodeEnvelope(t *testing.T) {
	t.Parallel()

	env := MakeEnvelope(TypePing, nil, []byte("#test"), nil, []byte{0x01, 0x02, 0x03}, nil, 1000)

	data, err := EncodeEnvelope(env)
	if err != nil {
		t.Fatalf("EncodeEnvelope: %v", err)
	}

	decoded, err := DecodeEnvelope(data)
	if err != nil {
		t.Fatalf("DecodeEnvelope: %v", err)
	}

	// Verify the envelope has the expected keys
	if len(decoded) < 3 {
		t.Errorf("decoded envelope has %v keys, want >= 3", len(decoded))
	}

	// Verify room is present
	foundRoom := false
	for _, v := range decoded {
		if s, ok := v.([]byte); ok && string(s) == "#test" {
			foundRoom = true
		}
	}
	if !foundRoom {
		t.Error("room '#test' not found in decoded envelope")
	}
}

func TestNowMs(t *testing.T) {
	t.Parallel()

	before := NowMs()
	ts := NowMs()
	after := NowMs()

	if ts < before || ts > after {
		t.Errorf("NowMs() = %v not in range [%v, %v]", ts, before, after)
	}
}

func TestMsgID(t *testing.T) {
	t.Parallel()

	id1 := MsgID()
	id2 := MsgID()

	if len(id1) != 8 {
		t.Errorf("MsgID len = %v, want 8", len(id1))
	}
	if len(id2) != 8 {
		t.Errorf("MsgID len = %v, want 8", len(id2))
	}
	// Two random IDs should (almost certainly) differ
	same := true
	for i := range id1 {
		if id1[i] != id2[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("Two MsgID() calls returned identical values")
	}
}

func TestNewHub(t *testing.T) {
	t.Parallel()

	hash := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	hub := NewHub(nil, hash, "rrc.hub", "Test Hub")

	if hub.DestName != "rrc.hub" {
		t.Errorf("DestName = %q, want %q", hub.DestName, "rrc.hub")
	}
	if hub.Name != "Test Hub" {
		t.Errorf("Name = %q, want %q", hub.Name, "Test Hub")
	}
	if hub.Status != StatusDisconnected {
		t.Errorf("Status = %v, want %v", hub.Status, StatusDisconnected)
	}
	if hub.MaxNickBytes != DefaultMaxNickBytes {
		t.Errorf("MaxNickBytes = %v, want %v", hub.MaxNickBytes, DefaultMaxNickBytes)
	}
}

func TestHubAddRemoveRoom(t *testing.T) {
	t.Parallel()

	hash := []byte{0x01, 0x02, 0x03, 0x04}
	hub := NewHub(nil, hash, "", "")

	hub.AddRoom("general")
	if !hub.Rooms["general"] {
		t.Error("Room 'general' not added")
	}

	hub.RemoveRoom("general")
	if hub.Rooms["general"] {
		t.Error("Room 'general' still exists after RemoveRoom")
	}
}

func TestHubMarkRead(t *testing.T) {
	t.Parallel()

	hash := []byte{0x01, 0x02, 0x03, 0x04}
	hub := NewHub(nil, hash, "", "")

	hub.lock.Lock()
	hub.UnreadRooms["general"] = true
	hub.MentionRooms["general"] = true
	hub.lock.Unlock()

	hub.MarkRead("general")

	hub.lock.Lock()
	defer hub.lock.Unlock()
	if hub.UnreadRooms["general"] {
		t.Error("UnreadRooms still has 'general' after MarkRead")
	}
	if hub.MentionRooms["general"] {
		t.Error("MentionRooms still has 'general' after MarkRead")
	}
}

func TestHubDisplayNameFor(t *testing.T) {
	t.Parallel()

	hash := []byte{0x01, 0x02, 0x03, 0x04}
	hub := NewHub(nil, hash, "", "")

	// Without nick set, returns hex prefix
	name := hub.DisplayNameFor(hash)
	if len(name) > 12 {
		t.Errorf("DisplayNameFor = %q (len %v), want ≤12 chars", name, len(name))
	}

	// With nick set
	hub.lock.Lock()
	hub.Nicks[hexString(hash)] = "Alice"
	hub.lock.Unlock()

	name = hub.DisplayNameFor(hash)
	if name != "Alice" {
		t.Errorf("DisplayNameFor = %q, want %q", name, "Alice")
	}
}

func TestHubSendMessage(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	hash := []byte{0x01, 0x02, 0x03, 0x04}
	hub := NewHub(nil, hash, "", "")
	hub.savedHistoryPath = dir

	mid := hub.SendMessage("general", "Hello!")
	if len(mid) == 0 {
		t.Error("SendMessage returned empty message ID")
	}

	hub.lock.Lock()
	msgs := hub.Messages["general"]
	hub.lock.Unlock()

	if len(msgs) != 1 {
		t.Fatalf("Messages len = %v, want 1", len(msgs))
	}
	if msgs[0].Text != "Hello!" {
		t.Errorf("Message text = %q, want %q", msgs[0].Text, "Hello!")
	}
	if msgs[0].Kind != "msg" {
		t.Errorf("Message kind = %q, want %q", msgs[0].Kind, "msg")
	}
}

func TestHubSendAction(t *testing.T) {
	t.Parallel()

	hash := []byte{0x01, 0x02, 0x03, 0x04}
	hub := NewHub(nil, hash, "", "")

	mid := hub.SendAction("general", "waves")
	if len(mid) == 0 {
		t.Error("SendAction returned empty message ID")
	}

	hub.lock.Lock()
	msgs := hub.Messages["general"]
	hub.lock.Unlock()

	if len(msgs) != 1 {
		t.Fatalf("Messages len = %v, want 1", len(msgs))
	}
	if msgs[0].Kind != "action" {
		t.Errorf("Message kind = %q, want %q", msgs[0].Kind, "action")
	}
}

// TestHubSendCommand verifies SendCommand mirrors Python RRCHub.send_command: a
// command string must start with "/", and the outbound envelope is a T_MSG
// carrying the command text as the body, the (un-normalized) room, the local
// identity hash as the source, and the effective nick. Unlike SendMessage it
// does not record the message locally.
func TestHubSendCommand(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	mgr := NewManager(dir, func() []byte { return []byte("myhash") })
	mgr.SetNickname("Alice")
	hub := mgr.AddHub([]byte{0x01, 0x02, 0x03, 0x04}, "rrc.hub", "Test Hub")

	var captured map[any]any
	hub.onSend = func(env map[any]any) { captured = env }

	if err := hub.SendCommand("/list", "General"); err != nil {
		t.Fatalf("SendCommand: %v", err)
	}
	if captured == nil {
		t.Fatal("SendCommand did not produce an envelope")
	}

	if got := intVal(captured, KeyType); got != TypeMsg {
		t.Errorf("envelope type = %v, want T_MSG (%v)", got, TypeMsg)
	}
	if got, _ := captured[KeyBody].(string); got != "/list" {
		t.Errorf("envelope body = %q, want /list", got)
	}
	if got := byteVal(captured, KeySource); !bytes.Equal(got, []byte("myhash")) {
		t.Errorf("envelope src = %x, want myhash", got)
	}
	// send_command does not normalize the room.
	if got := byteVal(captured, KeyRoom); string(got) != "General" {
		t.Errorf("envelope room = %q, want General (un-normalized)", got)
	}
	if got := byteVal(captured, KeyNick); string(got) != "Alice" {
		t.Errorf("envelope nick = %q, want Alice", got)
	}
	if mid := byteVal(captured, KeyMessageID); len(mid) != 8 {
		t.Errorf("envelope mid len = %v, want 8", len(mid))
	}

	// send_command does not record the message locally.
	hub.lock.Lock()
	defer hub.lock.Unlock()
	if len(hub.Messages) != 0 {
		t.Errorf("Messages = %v, want empty (send_command does not record locally)", hub.Messages)
	}
}

// TestHubSendCommandRejectsNonCommand verifies SendCommand rejects text that
// does not start with "/", matching Python's ValueError.
func TestHubSendCommandRejectsNonCommand(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	mgr := NewManager(dir, nil)
	hub := mgr.AddHub([]byte{0x01}, "rrc.hub", "H")

	sent := false
	hub.onSend = func(env map[any]any) { sent = true }

	if err := hub.SendCommand("hello", "general"); err == nil {
		t.Error("SendCommand(hello) returned nil error, want error for non-/ text")
	}
	if sent {
		t.Error("SendCommand sent an envelope despite rejection")
	}
}

func TestHubSetStatus(t *testing.T) {
	t.Parallel()

	hash := []byte{0x01, 0x02, 0x03, 0x04}
	hub := NewHub(nil, hash, "", "")

	hub.SetStatus(StatusConnected, "Connected!")
	if hub.Status != StatusConnected {
		t.Errorf("Status = %v, want %v", hub.Status, StatusConnected)
	}
	if hub.StatusText != "Connected!" {
		t.Errorf("StatusText = %q, want %q", hub.StatusText, "Connected!")
	}
}

func TestManagerAddRemoveHub(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	mgr := NewManager(dir, nil)

	hash1 := []byte{0x01, 0x02, 0x03, 0x04}
	hub1 := mgr.AddHub(hash1, "rrc.hub", "Hub 1")

	if len(mgr.Hubs) != 1 {
		t.Errorf("Hubs len = %v, want 1", len(mgr.Hubs))
	}

	// Adding same hub returns existing
	hub1b := mgr.AddHub(hash1, "rrc.hub", "Hub 1")
	if hub1 != hub1b {
		t.Error("AddHub returned different hub for same hash")
	}

	hash2 := []byte{0x05, 0x06, 0x07, 0x08}
	mgr.AddHub(hash2, "rrc.hub", "Hub 2")
	if len(mgr.Hubs) != 2 {
		t.Errorf("Hubs len = %v, want 2", len(mgr.Hubs))
	}

	mgr.RemoveHub(hub1)
	if len(mgr.Hubs) != 1 {
		t.Errorf("Hubs len after remove = %v, want 1", len(mgr.Hubs))
	}
}

// TestManagerHubsSnapshot pins HubsSnapshot: a locked copy of the hubs slice
// the TUI reads to render the channels list. Mutating the returned slice must
// not affect the manager's internal slice.
func TestManagerHubsSnapshot(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	mgr := NewManager(dir, nil)
	mgr.AddHub([]byte{0x01, 0x02, 0x03, 0x04}, "rrc.hub", "Hub 1")
	mgr.AddHub([]byte{0x05, 0x06, 0x07, 0x08}, "rrc.hub", "Hub 2")

	snap := mgr.HubsSnapshot()
	if len(snap) != 2 {
		t.Fatalf("HubsSnapshot len = %v, want 2", len(snap))
	}
	if snap[0].GetHubName() != "Hub 1" || snap[1].GetHubName() != "Hub 2" {
		t.Errorf("HubsSnapshot names = %q, %q, want Hub 1, Hub 2", snap[0].GetHubName(), snap[1].GetHubName())
	}

	// Mutating the snapshot must not affect the manager.
	_ = append(snap, nil)
	if len(mgr.Hubs) != 2 {
		t.Errorf("manager Hubs len after snapshot append = %v, want 2", len(mgr.Hubs))
	}
}

func TestManagerFindHub(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	mgr := NewManager(dir, nil)

	hash := []byte{0x01, 0x02, 0x03, 0x04}
	mgr.AddHub(hash, "rrc.hub", "Hub")

	found := mgr.FindHub(hash, "rrc.hub")
	if found == nil {
		t.Fatal("FindHub returned nil for existing hub")
	}

	if mgr.FindHub(hash, "wrong.name") != nil {
		t.Error("FindHub returned non-nil for wrong dest name")
	}

	if mgr.FindHub([]byte{0xFF}, "") != nil {
		t.Error("FindHub returned non-nil for unknown hash")
	}
}

func TestManagerSetActive(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	mgr := NewManager(dir, nil)

	hash := []byte{0x01, 0x02, 0x03, 0x04}
	hub := mgr.AddHub(hash, "", "")

	mgr.SetActive(hub, "general")
	if mgr.ActiveRoomFor(hub) != "general" {
		t.Errorf("ActiveRoomFor = %q, want %q", mgr.ActiveRoomFor(hub), "general")
	}

	// Different hub returns empty
	hash2 := []byte{0x05, 0x06, 0x07, 0x08}
	hub2 := mgr.AddHub(hash2, "", "")
	if mgr.ActiveRoomFor(hub2) != "" {
		t.Errorf("ActiveRoomFor wrong hub = %q, want empty", mgr.ActiveRoomFor(hub2))
	}
}

func TestManagerHasUnread(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	mgr := NewManager(dir, nil)

	if mgr.HasUnread() {
		t.Error("HasUnread = true with no hubs")
	}

	hash := []byte{0x01, 0x02, 0x03, 0x04}
	hub := mgr.AddHub(hash, "", "")

	if mgr.HasUnread() {
		t.Error("HasUnread = true with no unread rooms")
	}

	hub.lock.Lock()
	hub.UnreadRooms["general"] = true
	hub.lock.Unlock()

	if !mgr.HasUnread() {
		t.Error("HasUnread = false with unread room")
	}
}

func TestManagerSaveLoad(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	mgr := NewManager(dir, nil)

	hash := []byte{0x01, 0x02, 0x03, 0x04}
	hub := mgr.AddHub(hash, "rrc.hub", "Test Hub")
	hub.lock.Lock()
	hub.Rooms["general"] = true
	hub.Rooms["random"] = true
	hub.AutoReconnect = true
	hub.NickOverride = "Alice"
	hub.lock.Unlock()

	if err := mgr.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Load into new manager
	mgr2 := NewManager(dir, nil)
	if err := mgr2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(mgr2.Hubs) != 1 {
		t.Fatalf("Loaded Hubs len = %v, want 1", len(mgr2.Hubs))
	}

	loaded := mgr2.Hubs[0]
	if loaded.Name != "Test Hub" {
		t.Errorf("Loaded Name = %q, want %q", loaded.Name, "Test Hub")
	}
	if loaded.DestName != "rrc.hub" {
		t.Errorf("Loaded DestName = %q, want %q", loaded.DestName, "rrc.hub")
	}
	if !loaded.AutoReconnect {
		t.Error("Loaded AutoReconnect = false, want true")
	}
	if loaded.NickOverride != "Alice" {
		t.Errorf("Loaded NickOverride = %q, want %q", loaded.NickOverride, "Alice")
	}

	loaded.lock.Lock()
	if !loaded.Rooms["general"] || !loaded.Rooms["random"] {
		t.Error("Loaded rooms missing")
	}
	loaded.lock.Unlock()
}

func TestManagerSaveLoadEmpty(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	mgr := NewManager(dir, nil)

	if err := mgr.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	mgr2 := NewManager(dir, nil)
	if err := mgr2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(mgr2.Hubs) != 0 {
		t.Errorf("Loaded Hubs len = %v, want 0", len(mgr2.Hubs))
	}
}

func TestManagerShutdown(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	mgr := NewManager(dir, nil)

	hash := []byte{0x01, 0x02, 0x03, 0x04}
	mgr.AddHub(hash, "", "")

	mgr.Shutdown()

	for _, hub := range mgr.Hubs {
		if hub.Status != StatusDisconnected {
			t.Errorf("Hub Status = %v after shutdown, want %v", hub.Status, StatusDisconnected)
		}
	}
}

// TestManagerShutdownDisconnectsHubs verifies Shutdown mirrors Python
// RRCManager.shutdown, which calls h.disconnect() on every hub — tearing down
// the RNS link and clearing it, not merely flipping the status field.
func TestManagerShutdownDisconnectsHubs(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	mgr := NewManager(dir, nil)

	hash := []byte{0x01, 0x02, 0x03, 0x04}
	hub := mgr.AddHub(hash, "rrc.hub", "Test Hub")

	// Attach a link as a connected hub would, then shut the manager down.
	hub.SetLink(&rns.Link{})

	mgr.Shutdown()

	hub.lock.Lock()
	defer hub.lock.Unlock()
	if hub.Status != StatusDisconnected {
		t.Errorf("Status = %v, want %v", hub.Status, StatusDisconnected)
	}
	if hub.link != nil {
		t.Error("link not cleared by shutdown (Python disconnect sets link = None)")
	}
}

func TestManagerNickname(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	mgr := NewManager(dir, nil)

	if mgr.GetNickname() != "" {
		t.Errorf("GetNickname = %q, want empty", mgr.GetNickname())
	}

	mgr.SetNickname("Alice")
	if mgr.GetNickname() != "Alice" {
		t.Errorf("GetNickname = %q, want %q", mgr.GetNickname(), "Alice")
	}
}

func TestHandleDataMsgEnvelopeRecordsMessage(t *testing.T) {
	t.Parallel()

	mgr := NewManager(tempDir(t), func() []byte { return []byte("testhash") })
	mgr.SetNickname("TestNick")
	hub := mgr.AddHub([]byte("hubhash"), "rrc.chat", "TestHub")
	hub.AddRoom("general")

	env := MakeEnvelope(TypeMsg, []byte("sender"), []byte("general"), []byte("OtherNick"), []byte("hello world"), []byte("mid1"), NowMs())
	data, err := EncodeEnvelope(env)
	if err != nil {
		t.Fatal(err)
	}

	msgReceived := make(chan string, 1)
	mgr.SetMessageCallback(func(h *RRCHub, m *RRCMessage) {
		select {
		case msgReceived <- m.Text:
		default:
		}
	})

	hub.HandleData(data)

	select {
	case text := <-msgReceived:
		if text != "hello world" {
			t.Errorf("message text = %q, want %q", text, "hello world")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message callback")
	}

	msgs := hub.GetMessages("general")
	if len(msgs) != 1 {
		t.Fatalf("GetMessages len = %v, want 1", len(msgs))
	}
	if msgs[0].Nick != "OtherNick" {
		t.Errorf("nick = %q, want %q", msgs[0].Nick, "OtherNick")
	}
}

func TestHandleDataJoinEnvelopeAddsMember(t *testing.T) {
	t.Parallel()

	mgr := NewManager(tempDir(t), func() []byte { return []byte("testhash") })
	mgr.SetNickname("TestNick")
	hub := mgr.AddHub([]byte("hubhash"), "rrc.chat", "TestHub")
	hub.AddRoom("general")

	env := MakeEnvelope(TypeJoined, []byte("joinerhash"), []byte("general"), []byte("JoinerNick"), nil, []byte("mid2"), NowMs())
	data, err := EncodeEnvelope(env)
	if err != nil {
		t.Fatal(err)
	}

	hub.HandleData(data)

	members := hub.GetMembers("general")
	found := false
	for _, m := range members {
		if m == "JoinerNick" {
			found = true
		}
	}
	if !found {
		t.Errorf("JoinerNick not in members: %v", members)
	}
}

// TestResourceAdvertisedCap verifies resourceAdvertised mirrors Python
// RRCHub._resource_advertised: transfers at or below 262144 bytes are accepted,
// larger ones rejected.
func TestResourceAdvertisedCap(t *testing.T) {
	t.Parallel()

	hub := NewHub(nil, []byte{0x01}, "", "")

	cases := []struct {
		size int64
		want bool
	}{
		{0, true}, // zero-size (unknown) allowed
		{100, true},
		{262144, true},
		{262145, false},
		{1 << 20, false},
	}
	for _, tc := range cases {
		adv := &rns.ResourceAdvertisement{D: tc.size}
		if got := hub.resourceAdvertised(adv); got != tc.want {
			t.Errorf("resourceAdvertised(D=%v) = %v, want %v", tc.size, got, tc.want)
		}
	}

	// Falls back to transfer size T when data size D is unset.
	if got := hub.resourceAdvertised(&rns.ResourceAdvertisement{T: 262144}); !got {
		t.Error("resourceAdvertised(T=262144) = false, want true (T fallback)")
	}
	if got := hub.resourceAdvertised(&rns.ResourceAdvertisement{T: 262145}); got {
		t.Error("resourceAdvertised(T=262145) = true, want false (T fallback)")
	}
	if got := hub.resourceAdvertised(nil); got {
		t.Error("resourceAdvertised(nil) = true, want false")
	}
}

// makeResourceEnvelope builds an encoded T_RESOURCE_ENVELOPE for tests.
func makeResourceEnvelope(t *testing.T, rid, kind string, size int, sha []byte, encoding string, room string) []byte {
	t.Helper()
	body := map[any]any{
		ResKeyID:       []byte(rid),
		ResKeyKind:     kind,
		ResKeySize:     size,
		ResKeySHA256:   sha,
		ResKeyEncoding: encoding,
	}
	env := MakeEnvelope(TypeResourceEnvelope, []byte("peer"), []byte(room), nil, body, []byte("mid"), NowMs())
	data, err := EncodeEnvelope(env)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// TestRecordResourceExpectation verifies a T_RESOURCE_ENVELOPE records a pending
// expectation keyed by the resource id, capturing kind/size/sha/encoding/room,
// mirroring Python's T_RESOURCE_ENVELOPE branch.
func TestRecordResourceExpectation(t *testing.T) {
	t.Parallel()

	mgr := NewManager(tempDir(t), nil)
	hub := mgr.AddHub([]byte{0x01}, "rrc.hub", "H")

	sha := sha256.Sum256([]byte("payload"))
	data := makeResourceEnvelope(t, "rid1", "notice", 7, sha[:], "utf-8", "General")
	hub.HandleData(data)

	hub.lock.Lock()
	defer hub.lock.Unlock()
	exp, ok := hub.resourceExpectations["rid1"]
	if !ok {
		t.Fatal("resourceExpectations missing rid1")
	}
	if exp.kind != "notice" || exp.size != 7 || exp.room != "general" || exp.encoding != "utf-8" {
		t.Errorf("expectation = %+v, want kind=notice size=7 room=general encoding=utf-8", exp)
	}
	if !bytes.Equal(exp.sha256, sha[:]) {
		t.Errorf("expectation sha256 = %x, want %x", exp.sha256, sha)
	}
}

// TestRecordResourceExpectationRejectsInvalid verifies malformed envelopes
// (missing/invalid id, kind, or non-positive size) record nothing.
func TestRecordResourceExpectationRejectsInvalid(t *testing.T) {
	t.Parallel()

	mgr := NewManager(tempDir(t), nil)
	hub := mgr.AddHub([]byte{0x01}, "rrc.hub", "H")

	for _, tc := range []struct {
		name string
		rid  string
		kind string
		size int
	}{
		{"empty-rid", "", "notice", 5},
		{"empty-kind", "r", "", 5},
		{"zero-size", "r", "notice", 0},
		{"neg-size", "r", "notice", -1},
	} {
		hub.HandleData(makeResourceEnvelope(t, tc.rid, tc.kind, tc.size, nil, "utf-8", "general"))
		hub.lock.Lock()
		n := len(hub.resourceExpectations)
		hub.lock.Unlock()
		if n != 0 {
			t.Errorf("%v: resourceExpectations len = %v, want 0", tc.name, n)
		}
	}
}

// TestHandleConcludedResourceNotice verifies a completed resource matching an
// expectation (by size) records a notice in the expected room.
func TestHandleConcludedResourceNotice(t *testing.T) {
	t.Parallel()

	mgr := NewManager(tempDir(t), func() []byte { return []byte("me") })
	hub := mgr.AddHub([]byte{0x01}, "rrc.hub", "H")
	hub.lock.Lock()
	hub.resourceExpectations["rid"] = &resourceExpectation{
		kind: "notice", size: 5, room: "general", encoding: "utf-8",
		expires: time.Now().Add(30 * time.Second),
	}
	hub.lock.Unlock()

	received := make(chan *RRCMessage, 1)
	mgr.SetMessageCallback(func(h *RRCHub, m *RRCMessage) {
		select {
		case received <- m:
		default:
		}
	})

	hub.handleConcludedResource([]byte("hello"))

	select {
	case m := <-received:
		if m.Text != "hello" || m.Room != "general" || m.Kind != "notice" {
			t.Errorf("recorded notice = %+v, want text=hello room=general kind=notice", m)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for notice callback")
	}

	hub.lock.Lock()
	defer hub.lock.Unlock()
	if len(hub.resourceExpectations) != 0 {
		t.Errorf("expectation not consumed: %v", hub.resourceExpectations)
	}
}

// TestHandleConcludedResourceMOTD verifies a motd resource sets the hub motd,
// fires a change notification, and records a notice.
func TestHandleConcludedResourceMOTD(t *testing.T) {
	t.Parallel()

	mgr := NewManager(tempDir(t), func() []byte { return []byte("me") })
	hub := mgr.AddHub([]byte{0x01}, "rrc.hub", "H")
	hub.lock.Lock()
	hub.resourceExpectations["rid"] = &resourceExpectation{
		kind: "motd", size: 5, room: "", encoding: "utf-8",
		expires: time.Now().Add(30 * time.Second),
	}
	hub.lock.Unlock()

	changed := make(chan struct{}, 1)
	mgr.SetChangeCallback(func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	})

	hub.handleConcludedResource([]byte("MOTD!"))

	select {
	case <-changed:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for change callback (motd)")
	}

	hub.lock.Lock()
	if hub.MOTD != "MOTD!" {
		t.Errorf("MOTD = %q, want MOTD!", hub.MOTD)
	}
	hub.lock.Unlock()
}

// TestHandleConcludedResourceSHA256Mismatch verifies that a sha256 mismatch
// drops the data without recording a notice.
func TestHandleConcludedResourceSHA256Mismatch(t *testing.T) {
	t.Parallel()

	mgr := NewManager(tempDir(t), nil)
	hub := mgr.AddHub([]byte{0x01}, "rrc.hub", "H")
	wrong := sha256.Sum256([]byte("other"))
	hub.lock.Lock()
	hub.resourceExpectations["rid"] = &resourceExpectation{
		kind: "notice", size: 5, room: "general", sha256: wrong[:], encoding: "utf-8",
		expires: time.Now().Add(30 * time.Second),
	}
	hub.lock.Unlock()

	hub.handleConcludedResource([]byte("hello"))

	hub.lock.Lock()
	defer hub.lock.Unlock()
	if len(hub.Messages["general"]) != 0 {
		t.Errorf("Messages[general] = %v, want empty (sha mismatch)", hub.Messages["general"])
	}
}

// TestHandleConcludedResourceExpiresExpectation verifies an expired
// expectation is purged and not matched, so a blob-sized payload (default kind)
// records nothing.
func TestHandleConcludedResourceExpiresExpectation(t *testing.T) {
	t.Parallel()

	mgr := NewManager(tempDir(t), nil)
	hub := mgr.AddHub([]byte{0x01}, "rrc.hub", "H")
	hub.lock.Lock()
	hub.resourceExpectations["rid"] = &resourceExpectation{
		kind: "notice", size: 5, room: "general", encoding: "utf-8",
		expires: time.Now().Add(-time.Minute), // already expired
	}
	hub.lock.Unlock()

	hub.handleConcludedResource([]byte("hello"))

	hub.lock.Lock()
	defer hub.lock.Unlock()
	if len(hub.resourceExpectations) != 0 {
		t.Errorf("expired expectation not purged: %v", hub.resourceExpectations)
	}
	if len(hub.Messages["general"]) != 0 {
		t.Errorf("Messages[general] = %v, want empty (no match)", hub.Messages["general"])
	}
}

// TestDecodeText verifies decodeText mirrors Python's
// data.decode(encoding, errors="replace"): utf-8 passes through, invalid utf-8
// gets U+FFFD replacement, and a non-utf-8 charset (latin-1) decodes correctly.
func TestDecodeText(t *testing.T) {
	t.Parallel()

	if got := decodeText([]byte("hello"), "utf-8"); got != "hello" {
		t.Errorf("decodeText(utf-8) = %q, want hello", got)
	}
	if got := decodeText([]byte("hello"), ""); got != "hello" {
		t.Errorf("decodeText(empty) = %q, want hello", got)
	}
	// Invalid utf-8 byte 0xFF → U+FFFD replacement.
	got := decodeText([]byte("a\xffb"), "utf-8")
	if !strings.Contains(got, "�") || !strings.Contains(got, "a") {
		t.Errorf("decodeText(invalid utf-8) = %q, want replacement char", got)
	}
	// latin-1: 0xE9 → é
	if got := decodeText([]byte{0xE9}, "latin-1"); got != "é" {
		t.Errorf("decodeText(latin-1) = %q, want é", got)
	}
	// Unknown charset falls back to utf-8-with-replacement.
	if got := decodeText([]byte("ok"), "no-such-encoding"); got != "ok" {
		t.Errorf("decodeText(unknown) = %q, want ok (fallback)", got)
	}
}

func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "nomadnet-rrc-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// silentLogger returns an RNS logger that emits nothing, keeping transport and
// link operations quiet during unit tests.
func silentLogger() *rns.Logger {
	l := rns.NewLogger()
	l.SetLogLevel(rns.LogNone)
	return l
}

// TestPacketWouldFitMTUBoundary verifies RRCHub.packetWouldFit matches Python
// RRC._packet_would_fit at the MTU boundary. Python builds RNS.Packet(link,
// payload) and attempts to pack it; pack fails when the packed size (header
// plus the AES-CBC/HMAC-enclosed ciphertext) exceeds the MTU. For the default
// 500-byte MTU the boundary is exactly 431 bytes fit / 432 reject: 431 packs
// to 499 bytes, 432 pads to a 448-byte ciphertext and packs to 515.
//
// The link is set up with a usable encryption token but no live network:
// NewLink generates ephemeral keys, LoadPeer installs the peer's public keys,
// and Handshake derives the shared token locally. The link MTU defaults to
// rns.MTU (500), matching the Python reference used to derive the boundary.
func TestPacketWouldFitMTUBoundary(t *testing.T) {
	t.Parallel()

	ts := rns.NewTransportSystem(silentLogger())
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	dest, err := rns.NewDestination(ts, id, rns.DestinationIn, rns.DestinationSingle, "rrc", "fit")
	if err != nil {
		t.Fatal(err)
	}

	link, err := rns.NewLink(ts, dest)
	if err != nil {
		t.Fatal(err)
	}
	peer, err := rns.NewLink(ts, dest)
	if err != nil {
		t.Fatal(err)
	}
	if err := link.LoadPeer(peer.GetPublicBytes(), peer.GetSigPublicBytes()); err != nil {
		t.Fatal(err)
	}
	if err := link.Handshake(); err != nil {
		t.Fatal(err)
	}
	// Establish populates the link's 16-byte hash (linkID) from the link
	// request packet before any send; with no interface registered the send
	// is a no-op, but the hash — which sizes the packed packet header — is
	// already set. Ignore the send result and stop the establishment watchdog.
	_ = link.Establish()
	t.Cleanup(func() { link.Teardown() })

	dir, err := os.MkdirTemp("/tmp", "nomadnet-rrc-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	mgr := NewManager(dir, func() []byte { return id.Hash })
	hub := mgr.AddHub(dest.Hash, "rrc.fit", "TestHub")

	cases := []struct {
		size int
		want bool
	}{
		{0, true},
		{100, true},
		{431, true},
		{432, false},
		{1000, false},
	}
	for _, tc := range cases {
		payload := make([]byte, tc.size)
		got := hub.packetWouldFit(link, payload)
		if got != tc.want {
			t.Errorf("packetWouldFit(size=%v) = %v, want %v", tc.size, got, tc.want)
		}
	}
}

// TestManagerIdentity verifies RRCManager.Identity returns the identity set
// via SetIdentity, mirroring Python RRCManager.identity (a @property returning
// self.app.identity). It also confirms that with no identity-hash function
// configured, identityHash falls back to the identity's hash — matching
// Python's self.manager.identity.hash used as the envelope source.
func TestManagerIdentity(t *testing.T) {
	t.Parallel()

	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := tempDir(t)
	mgr := NewManager(dir, nil)

	if mgr.Identity() != nil {
		t.Error("Identity() should be nil before SetIdentity")
	}
	mgr.SetIdentity(id)
	if mgr.Identity() != id {
		t.Error("Identity() does not return the identity set by SetIdentity")
	}
	if got := mgr.identityHash(); !bytes.Equal(got, id.Hash) {
		t.Errorf("identityHash() = %x, want identity hash %x", got, id.Hash)
	}
}

// TestManagerIdentityHashPrefersFunction verifies that an explicit
// identity-hash function takes precedence over the identity's own hash, so
// existing callers that supply a function keep their behavior.
func TestManagerIdentityHashPrefersFunction(t *testing.T) {
	t.Parallel()

	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := tempDir(t)
	custom := []byte{0xAA, 0xBB, 0xCC}
	mgr := NewManager(dir, func() []byte { return custom })
	mgr.SetIdentity(id)

	if got := mgr.identityHash(); !bytes.Equal(got, custom) {
		t.Errorf("identityHash() = %x, want function result %x", got, custom)
	}
}

// TestHubHistoryConfigAccessors verifies the hub history-config accessors
// mirror Python RRCHub._per_room_cap, _filter_history and
// _ephemeral_notices_history: they read from the manager (which in turn reads
// the app config), and fall back to the Python getattr defaults — cap 0 (none),
// filter true, SYS_NOTICE_TIMEOUT (600) — when nothing is configured.
func TestHubHistoryConfigAccessors(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)

	// Unconfigured manager → Python getattr defaults.
	mgr := NewManager(dir, nil)
	hub := mgr.AddHub([]byte{0x01}, "rrc.hub", "H")
	if hub.perRoomCap() != 0 {
		t.Errorf("perRoomCap = %v, want 0 (no cap) by default", hub.perRoomCap())
	}
	if !hub.filterHistory() {
		t.Error("filterHistory = false, want true by default")
	}
	if got := hub.ephemeralNoticesHistory(); got != NoticeTimeout {
		t.Errorf("ephemeralNoticesHistory = %v, want %v", got, NoticeTimeout)
	}

	// Configured manager → reflects the configured values.
	mgr.SetHistoryConfig(500, false, 120)
	if got := hub.perRoomCap(); got != 500 {
		t.Errorf("perRoomCap = %v, want 500", got)
	}
	if hub.filterHistory() {
		t.Error("filterHistory = true, want false after config")
	}
	if got := hub.ephemeralNoticesHistory(); got != 120 {
		t.Errorf("ephemeralNoticesHistory = %v, want 120", got)
	}

	// A zero ephemeral value is ignored, keeping the previous setting.
	mgr.SetHistoryConfig(0, true, 0)
	if got := hub.ephemeralNoticesHistory(); got != 120 {
		t.Errorf("ephemeralNoticesHistory = %v, want 120 (zero ignored)", got)
	}

	// A hub with no manager falls back to the Python defaults.
	bare := NewHub(nil, []byte{0x02}, "", "")
	if bare.perRoomCap() != 0 || !bare.filterHistory() || bare.ephemeralNoticesHistory() != NoticeTimeout {
		t.Error("bare hub accessors did not return Python defaults")
	}
}

// TestHubCleanHistoryRemovesOldNotices verifies cleanHistory mirrors Python
// RRCHub._clean_history: it runs at most once per CLEAN_HISTORY_INTERVAL and
// removes system/notice messages older than the ephemeral-notices timeout,
// while leaving regular messages and recent notices untouched.
func TestHubCleanHistoryRemovesOldNotices(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	mgr := NewManager(dir, nil)
	mgr.SetHistoryConfig(0, true, 600)
	hub := mgr.AddHub([]byte{0x01}, "rrc.hub", "H")

	now := time.Now().Unix()
	// Force the interval check to run on the first cleanHistory call.
	hub.lastHistoryClean = 0

	hub.lock.Lock()
	hub.Messages["general"] = []*RRCMessage{
		{Kind: "msg", Room: "general", Text: "keep-msg", Ts: now * 1000},
		{Kind: "notice", Room: "general", Text: "old-notice", Ts: (now - 700) * 1000},
		{Kind: "system", Room: "general", Text: "old-system", Ts: (now - 700) * 1000},
		{Kind: "notice", Room: "general", Text: "fresh-notice", Ts: now * 1000},
	}
	hub.lock.Unlock()

	hub.cleanHistory()

	hub.lock.Lock()
	defer hub.lock.Unlock()
	texts := messageTexts(hub.Messages["general"])
	want := map[string]bool{"keep-msg": true, "fresh-notice": true}
	for _, txt := range texts {
		if txt == "old-notice" || txt == "old-system" {
			t.Errorf("cleanHistory left stale %q in buffer: %v", txt, texts)
		}
	}
	for w := range want {
		if !slices.Contains(texts, w) {
			t.Errorf("cleanHistory removed %q, want it kept: %v", w, texts)
		}
	}
	if hub.cleanLastRemoved == 0 {
		t.Error("cleanLastRemoved not set after a cleanup that removed messages")
	}
}

// TestHubCleanHistoryRespectsInterval verifies cleanHistory is a no-op when
// called again within CLEAN_HISTORY_INTERVAL seconds, matching Python's
// `now > self._last_history_clean + CLEAN_HISTORY_INTERVAL` gate.
func TestHubCleanHistoryRespectsInterval(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	mgr := NewManager(dir, nil)
	hub := mgr.AddHub([]byte{0x01}, "rrc.hub", "H")

	now := time.Now().Unix()
	// Pretend a cleanup just ran, so the interval gate should skip the body.
	hub.lastHistoryClean = now

	hub.lock.Lock()
	hub.Messages["general"] = []*RRCMessage{
		{Kind: "notice", Room: "general", Text: "old", Ts: 0},
	}
	hub.lock.Unlock()

	hub.cleanHistory()

	hub.lock.Lock()
	defer hub.lock.Unlock()
	if len(hub.Messages["general"]) != 1 {
		t.Errorf("messages = %v, want untouched within interval", hub.Messages["general"])
	}
	if hub.cleanLastRemoved != 0 {
		t.Error("cleanLastRemoved set despite interval gate skipping cleanup")
	}
}

// TestHubRecordNotice verifies recordNotice mirrors Python RRCHub._record_notice:
// the notice is appended to the global notices list (capped at 200), added to
// its room's message buffer, marked unread when the room is not active,
// persisted to history, and announced via the message callback.
func TestHubRecordNotice(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	mgr := NewManager(dir, func() []byte { return []byte("me") })
	hub := mgr.AddHub([]byte{0x01}, "rrc.hub", "H")
	hub.savedHistoryPath = dir
	hub.AddRoom("general")

	received := make(chan *RRCMessage, 4)
	mgr.SetMessageCallback(func(h *RRCHub, m *RRCMessage) {
		select {
		case received <- m:
		default:
		}
	})

	// Notice with an explicit room: recorded in notices + the room buffer, and
	// marked unread because "general" is not the active room.
	hub.recordNotice(&RRCMessage{Kind: "notice", Room: "general", Text: "hello", Ts: NowMs()})

	select {
	case m := <-received:
		if m.Text != "hello" {
			t.Errorf("callback got %q, want hello", m.Text)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for notice callback")
	}

	hub.lock.Lock()
	if len(hub.Notices) != 1 || hub.Notices[0].Text != "hello" {
		t.Errorf("Notices = %v, want [hello]", hub.Notices)
	}
	if len(hub.Messages["general"]) != 1 {
		t.Errorf("Messages[general] = %v, want 1 notice", hub.Messages["general"])
	}
	if !hub.UnreadRooms["general"] {
		t.Error("general not marked unread for inactive room")
	}
	hub.lock.Unlock()

	// Notice with no room is routed to the active room and its room is set.
	mgr.SetActive(hub, "lounge")
	hub.recordNotice(&RRCMessage{Kind: "notice", Room: "", Text: "routed", Ts: NowMs()})
	hub.lock.Lock()
	routed := hub.Messages["lounge"]
	if len(routed) != 1 || routed[0].Room != "lounge" || routed[0].Text != "routed" {
		t.Errorf("routed notice = %v, want room=lounge text=routed", routed)
	}
	// Active room is not marked unread.
	if hub.UnreadRooms["lounge"] {
		t.Error("active room lounge marked unread, should not be")
	}
	hub.lock.Unlock()
}

// TestHubRecordNoticeCapsAt200 verifies the global notices list is capped at
// 200, matching Python's `del self.notices[:len(self.notices)-200]`.
func TestHubRecordNoticeCapsAt200(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	mgr := NewManager(dir, func() []byte { return []byte("me") })
	hub := mgr.AddHub([]byte{0x01}, "rrc.hub", "H")

	for i := 0; i < 205; i++ {
		hub.recordNotice(&RRCMessage{Kind: "notice", Room: "general", Text: "n", Ts: NowMs()})
	}

	hub.lock.Lock()
	defer hub.lock.Unlock()
	if len(hub.Notices) > 200 {
		t.Errorf("Notices len = %v, want <= 200", len(hub.Notices))
	}
}

func messageTexts(msgs []*RRCMessage) []string {
	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, m.Text)
	}
	return out
}

// appendHistoryEntry writes a single CBOR history entry for room to the hub's
// history file, mirroring what appendHistory produces on disk.
func appendHistoryEntry(t *testing.T, hub *RRCHub, room string, msg *RRCMessage) {
	t.Helper()
	room = strings.ToLower(room)
	msg.Room = room
	entry := msg.HistoryEntry()
	data, err := cbor.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}
	path := hub.historyPath(room)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
}

// TestHubLoadHistory verifies loadHistory mirrors Python RRCHub._load_history:
// it reads the per-room CBOR history file, keeps only the last perRoomCap
// entries, and replaces the in-memory buffer with the result, setting each
// message's room.
func TestHubLoadHistory(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	mgr := NewManager(dir, nil)
	mgr.SetHistoryConfig(3, true, 600)
	hub := mgr.AddHub([]byte{0x01, 0x02, 0x03, 0x04}, "rrc.hub", "H")
	hub.AddRoom("general") // ensures a buffer entry exists for the room

	for i := 0; i < 5; i++ {
		appendHistoryEntry(t, hub, "general", &RRCMessage{Kind: "msg", Text: "m" + string(rune('1'+i)), Ts: int64(i)})
	}

	hub.loadHistory()

	hub.lock.Lock()
	defer hub.lock.Unlock()
	msgs := hub.Messages["general"]
	if len(msgs) != 3 {
		t.Fatalf("Messages[general] len = %v, want 3 (cap)", len(msgs))
	}
	want := []string{"m3", "m4", "m5"}
	got := messageTexts(msgs)
	if !slices.Equal(got, want) {
		t.Errorf("Messages[general] = %v, want %v", got, want)
	}
	for _, m := range msgs {
		if m.Room != "general" {
			t.Errorf("loaded msg room = %q, want general", m.Room)
		}
	}
}

// TestHubLoadHistoryFiltersSystemNotice verifies that with the loaded-history
// filter enabled, system/notice entries are dropped on load (matching Python's
// filter_msgs branch), while regular messages survive.
func TestHubLoadHistoryFiltersSystemNotice(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	mgr := NewManager(dir, nil)
	mgr.SetHistoryConfig(0, true, 600) // no cap, filter on
	hub := mgr.AddHub([]byte{0x01}, "rrc.hub", "H")
	hub.AddRoom("general")

	appendHistoryEntry(t, hub, "general", &RRCMessage{Kind: "msg", Text: "keep1", Ts: 1})
	appendHistoryEntry(t, hub, "general", &RRCMessage{Kind: "system", Text: "sys", Ts: 2})
	appendHistoryEntry(t, hub, "general", &RRCMessage{Kind: "notice", Text: "note", Ts: 3})
	appendHistoryEntry(t, hub, "general", &RRCMessage{Kind: "msg", Text: "keep2", Ts: 4})

	hub.loadHistory()

	hub.lock.Lock()
	defer hub.lock.Unlock()
	got := messageTexts(hub.Messages["general"])
	want := []string{"keep1", "keep2"}
	if !slices.Equal(got, want) {
		t.Errorf("Messages[general] = %v, want %v (system/notice filtered)", got, want)
	}
}

// TestHubLoadHistoryRespectsFilterDisabled verifies that when the
// loaded-history filter is disabled, system/notice entries are kept.
func TestHubLoadHistoryRespectsFilterDisabled(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	mgr := NewManager(dir, nil)
	mgr.SetHistoryConfig(0, false, 600) // filter off
	hub := mgr.AddHub([]byte{0x01}, "rrc.hub", "H")
	hub.AddRoom("general")

	appendHistoryEntry(t, hub, "general", &RRCMessage{Kind: "system", Text: "sys", Ts: 1})

	hub.loadHistory()

	hub.lock.Lock()
	defer hub.lock.Unlock()
	got := messageTexts(hub.Messages["general"])
	if !slices.Equal(got, []string{"sys"}) {
		t.Errorf("Messages[general] = %v, want [sys] (filter disabled)", got)
	}
}

// TestHistoryPathParity verifies the per-room history file layout matches
// Python RRCManager._history_path: the filename is the sanitized (lowercased,
// non-[a-z0-9._-] → _) room name truncated to 64 chars, an underscore, and
// the first 8 hex chars of sha256(room), plus ".log".
func TestHistoryPathParity(t *testing.T) {
	t.Parallel()

	hub := NewHub(nil, []byte{0x01, 0x02, 0x03, 0x04}, "", "")
	hub.savedHistoryPath = "/tmp/rrc-hist"

	// Plain room.
	got := hub.historyPath("general")
	wantRoomHash := sha256Prefix("general")
	want := "/tmp/rrc-hist/general_" + wantRoomHash + ".log"
	if got != want {
		t.Errorf("historyPath(general) = %q, want %q", got, want)
	}

	// Room with characters requiring sanitizing.
	got = hub.historyPath("My Room!")
	wantRoomHash = sha256Prefix("my room!") // room is lowercased before hashing
	// "my room!" sanitizes to "my_room_" (trailing ! → _), then the separator
	// adds another underscore — matching Python's _history_path.
	want = "/tmp/rrc-hist/my_room__" + wantRoomHash + ".log"
	if got != want {
		t.Errorf("historyPath(My Room!) = %q, want %q", got, want)
	}

	// Long room name is truncated to 64 sanitized chars.
	long := strings.Repeat("a", 100)
	got = hub.historyPath(long)
	if len(filepath.Base(got)) != 64+1+8+4 /* name_ + hash + .log */ {
		t.Errorf("historyPath(long) base = %q (len %v), want 64-char sanitized prefix", filepath.Base(got), len(filepath.Base(got)))
	}
}

// TestHistoryDirParity verifies the per-hub history directory matches Python
// RRCManager._history_dir: keyed by hub-hash hex, with a "__<dest_name hash>"
// suffix when the destination name is non-default.
func TestHistoryDirParity(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	mgr := NewManager(dir, nil)

	// Default dest name: directory is just the hash hex.
	hub := mgr.AddHub([]byte{0x01, 0x02, 0x03, 0x04}, "rrc.hub", "H")
	want := filepath.Join(dir, "rrc_history", hexString([]byte{0x01, 0x02, 0x03, 0x04}))
	if got := mgr.historyDir(hub); got != want {
		t.Errorf("historyDir(default) = %q, want %q", got, want)
	}

	// Non-default dest name: appends __<sha256(dest)[:4] hex>.
	hub2 := mgr.AddHub([]byte{0x09, 0x08, 0x07, 0x06}, "rrc.chat", "H2")
	suffix := sha256Prefix("rrc.chat")
	want2 := filepath.Join(dir, "rrc_history", hexString([]byte{0x09, 0x08, 0x07, 0x06})+"__"+suffix)
	if got := mgr.historyDir(hub2); got != want2 {
		t.Errorf("historyDir(rrc.chat) = %q, want %q", got, want2)
	}
}

func sha256Prefix(s string) string {
	sum := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", sum[:4])
}

// TestHubLoadHistoryTruncatesOnDecodeError verifies that a corrupt tail is
// truncated: loadHistory keeps the valid entries decoded before the error,
// matching Python's decode_error truncation.
func TestHubLoadHistoryTruncatesOnDecodeError(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	mgr := NewManager(dir, nil)
	mgr.SetHistoryConfig(0, true, 600)
	hub := mgr.AddHub([]byte{0x01}, "rrc.hub", "H")
	hub.AddRoom("general")

	for i := 0; i < 3; i++ {
		appendHistoryEntry(t, hub, "general", &RRCMessage{Kind: "msg", Text: "v" + string(rune('1'+i)), Ts: int64(i)})
	}
	// Append corrupt bytes that are not a valid CBOR object.
	path := hub.historyPath("general")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte{0xFF, 0xFF, 0xFF}); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	_ = f.Close()

	hub.loadHistory()

	hub.lock.Lock()
	defer hub.lock.Unlock()
	got := messageTexts(hub.Messages["general"])
	want := []string{"v1", "v2", "v3"}
	if !slices.Equal(got, want) {
		t.Errorf("Messages[general] = %v, want %v (truncated at corrupt tail)", got, want)
	}
}

// TestManagerSaveLoadPartedRoomsParity verifies the on-disk hub config matches
// Python RRCManager.save/load field-by-field. In Python, parted_rooms is not a
// stored set but is derived at save time as sorted(set(h.messages.keys()) -
// joined): every room that has a message buffer but is not currently joined.
// On load, parted rooms get an empty message buffer (messages.setdefault) and
// are NOT added to the joined rooms set.
//
// This test builds a hub in the Python state — joined "general" with messages,
// plus a parted "old" room that has a message buffer but is not joined — saves
// it, decodes the raw CBOR to inspect the stored parted_rooms field, then loads
// it into a fresh manager and asserts the joined/parts split matches Python.
func TestManagerSaveLoadPartedRoomsParity(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	mgr := NewManager(dir, nil)

	hub := mgr.AddHub([]byte{0x01, 0x02, 0x03, 0x04}, "rrc.hub", "Test Hub")
	hub.lock.Lock()
	// Joined room with a message buffer.
	hub.Rooms["general"] = true
	hub.Messages["general"] = []*RRCMessage{{Kind: "msg", Room: "general", Text: "hi", Ts: 1}}
	// Parted room: a message buffer exists but the room is not joined.
	hub.Messages["old"] = []*RRCMessage{{Kind: "msg", Room: "old", Text: "old", Ts: 0}}
	hub.AutoReconnect = true
	hub.lock.Unlock()

	if err := mgr.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Decode the raw CBOR to inspect the stored parted_rooms field directly.
	data, err := os.ReadFile(mgr.storePath())
	if err != nil {
		t.Fatalf("read store: %v", err)
	}
	var raw map[string]any
	if err := cbor.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	entries, _ := raw["hubs"].([]any)
	if len(entries) != 1 {
		t.Fatalf("hubs entries = %v, want 1", len(entries))
	}
	entry, _ := entries[0].(map[any]any)

	if got, _ := entry["rooms"].([]any); len(got) != 1 || got[0] != "general" {
		t.Errorf("stored rooms = %v, want [general]", got)
	}
	// Python derives parted_rooms from messages keys minus joined rooms.
	if got, _ := entry["parted_rooms"].([]any); len(got) != 1 || got[0] != "old" {
		t.Errorf("stored parted_rooms = %v, want [old]", got)
	}
	if got, _ := entry["auto_reconnect"].(bool); got != true {
		t.Errorf("stored auto_reconnect = %v, want true", got)
	}

	// Load into a fresh manager and confirm the joined/parted split.
	mgr2 := NewManager(dir, nil)
	if err := mgr2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(mgr2.Hubs) != 1 {
		t.Fatalf("loaded hubs = %v, want 1", len(mgr2.Hubs))
	}
	loaded := mgr2.Hubs[0]
	loaded.lock.Lock()
	defer loaded.lock.Unlock()

	if !loaded.Rooms["general"] {
		t.Error("loaded Rooms missing joined 'general'")
	}
	if loaded.Rooms["old"] {
		t.Error("loaded Rooms contains parted 'old', should not be joined")
	}
	// Python's load gives parted rooms an empty message buffer.
	if _, ok := loaded.Messages["old"]; !ok {
		t.Error("loaded Messages missing parted 'old' buffer")
	}
}

// ---------------------------------------------------------------------------
// E1 — hub connection lifecycle (setAuto*, reconnect backoff, scheduleReconnect,
// onClosed, connectWorker). Mirrors Python RRC.py connect/_connect_worker/
// _on_closed/_schedule_reconnect/set_auto_*.
// ---------------------------------------------------------------------------

// TestSetAutoReconnectPersists verifies setAutoReconnect flips the field,
// persists to disk when save=true, and fires the change callback.
func TestSetAutoReconnectPersists(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	mgr := NewManager(dir, func() []byte { return []byte("me") })
	hub := mgr.AddHub([]byte{0x01, 0x02, 0x03}, "rrc.hub", "H")

	changed := make(chan struct{}, 1)
	mgr.SetChangeCallback(func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	})

	hub.SetAutoReconnect(true, true)

	hub.lock.Lock()
	if !hub.AutoReconnect {
		t.Error("AutoReconnect not set to true")
	}
	hub.lock.Unlock()

	select {
	case <-changed:
	case <-time.After(time.Second):
		t.Fatal("change callback not fired")
	}

	// Persistence: a fresh manager loading the same storage sees the flag.
	mgr2 := NewManager(dir, func() []byte { return []byte("me") })
	if err := mgr2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	loaded := mgr2.FindHub([]byte{0x01, 0x02, 0x03}, "rrc.hub")
	if loaded == nil {
		t.Fatal("loaded hub is nil")
	}
	if !loaded.AutoReconnect {
		t.Error("loaded AutoReconnect = false, want true")
	}
}

// TestSetAutoReconnectCancelsTimer verifies disabling auto-reconnect cancels
// any pending reconnect timer (Python cancels self._reconnect_timer).
func TestSetAutoReconnectCancelsTimer(t *testing.T) {
	t.Parallel()

	mgr := NewManager(tempDir(t), func() []byte { return []byte("me") })
	hub := mgr.AddHub([]byte{0x01}, "rrc.hub", "H")

	fired := make(chan struct{}, 1)
	hub.afterFunc = func(d time.Duration, f func()) *time.Timer {
		go f()
		return time.NewTimer(d)
	}
	hub.connectFn = func() {
		select {
		case fired <- struct{}{}:
		default:
		}
	}

	hub.lock.Lock()
	hub.AutoReconnect = true
	hub.lock.Unlock()
	hub.scheduleReconnect()

	hub.lock.Lock()
	timer := hub.reconnectTimer
	hub.lock.Unlock()
	if timer == nil {
		t.Fatal("reconnect timer not scheduled")
	}

	hub.SetAutoReconnect(false, false)

	hub.lock.Lock()
	after := hub.reconnectTimer
	hub.lock.Unlock()
	if after != nil {
		t.Error("reconnect timer not cleared after disabling auto-reconnect")
	}
}

// TestSetAutoListAndSetAutoWho verify the list/who setters persist and notify.
func TestSetAutoListAndSetAutoWho(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	mgr := NewManager(dir, func() []byte { return []byte("me") })
	hub := mgr.AddHub([]byte{0x07}, "rrc.hub", "H")

	changes := make(chan struct{}, 4)
	mgr.SetChangeCallback(func() {
		select {
		case changes <- struct{}{}:
		default:
		}
	})

	hub.SetAutoList(true, true)
	hub.SetAutoWho(true, true)

	hub.lock.Lock()
	if !hub.AutoList || !hub.AutoWho {
		t.Error("AutoList/AutoWho not set")
	}
	hub.lock.Unlock()

	// Two notifications fired.
	for i := 0; i < 2; i++ {
		select {
		case <-changes:
		case <-time.After(time.Second):
			t.Fatal("change callback not fired for setAuto*")
		}
	}

	mgr2 := NewManager(dir, func() []byte { return []byte("me") })
	if err := mgr2.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	loaded := mgr2.FindHub([]byte{0x07}, "rrc.hub")
	if loaded == nil {
		t.Fatal("loaded hub is nil")
	}
	if !loaded.AutoList || !loaded.AutoWho {
		t.Error("loaded AutoList/AutoWho = false, want true")
	}
}

// TestReconnectBackoff verifies the exponential backoff against Python's
// _schedule_reconnect: backoff = min(60.0, max(1.0, 2.0 ** min(attempts, 6))).
func TestReconnectBackoff(t *testing.T) {
	t.Parallel()

	cases := []struct {
		attempts int
		want     time.Duration
	}{
		{0, 1 * time.Second},  // 2^0 = 1, max(1,1)=1
		{1, 2 * time.Second},  // 2^1 = 2
		{2, 4 * time.Second},  // 2^2 = 4
		{3, 8 * time.Second},  // 2^3 = 8
		{4, 16 * time.Second}, // 2^4 = 16
		{5, 32 * time.Second}, // 2^5 = 32
		{6, 60 * time.Second}, // 2^6 = 64 -> capped to 60
		{7, 60 * time.Second}, // min(60, 2^6) = 60
		{100, 60 * time.Second},
	}
	for _, tc := range cases {
		got := reconnectBackoff(tc.attempts)
		if got != tc.want {
			t.Errorf("reconnectBackoff(%v) = %v, want %v", tc.attempts, got, tc.want)
		}
	}
}

// TestScheduleReconnect verifies reconnectAttempts increments, the status text
// reflects the backoff, and the timer's fire calls connectFn (guarded by
// manual/auto state), mirroring Python _schedule_reconnect.
func TestScheduleReconnect(t *testing.T) {
	t.Parallel()

	mgr := NewManager(tempDir(t), func() []byte { return []byte("me") })
	hub := mgr.AddHub([]byte{0x02}, "rrc.hub", "H")

	var scheduledDur time.Duration
	var fire func()
	hub.afterFunc = func(d time.Duration, f func()) *time.Timer {
		scheduledDur = d
		fire = f
		return time.NewTimer(d)
	}
	fired := make(chan struct{}, 1)
	hub.connectFn = func() {
		select {
		case fired <- struct{}{}:
		default:
		}
	}

	hub.lock.Lock()
	hub.AutoReconnect = true
	hub.lock.Unlock()
	hub.scheduleReconnect()

	hub.lock.Lock()
	attempts := hub.reconnectAttempts
	status := hub.Status
	text := hub.StatusText
	hub.lock.Unlock()

	if attempts != 1 {
		t.Errorf("reconnectAttempts = %v, want 1", attempts)
	}
	if status != StatusDisconnected {
		t.Errorf("Status = %v, want StatusDisconnected", status)
	}
	if text != "Reconnect in 2s" {
		t.Errorf("StatusText = %q, want %q", text, "Reconnect in 2s")
	}
	if scheduledDur != 2*time.Second {
		t.Errorf("scheduled duration = %v, want 2s", scheduledDur)
	}

	// Fire with auto enabled + no manual disconnect -> connectFn invoked.
	fire()
	select {
	case <-fired:
	case <-time.After(time.Second):
		t.Error("connectFn not invoked on fire")
	}

	// Fire with manual disconnect set -> connectFn NOT invoked again.
	hub.lock.Lock()
	hub.manualDisconnect = true
	hub.lock.Unlock()
	select {
	case <-fired:
		t.Error("connectFn invoked despite manual disconnect")
	default:
	}
}

// TestOnClosedClearsState verifies onClosed clears link-derived state and sets
// DISCONNECTED, mirroring Python _on_closed.
func TestOnClosedClearsState(t *testing.T) {
	t.Parallel()

	mgr := NewManager(tempDir(t), func() []byte { return []byte("me") })
	hub := mgr.AddHub([]byte{0x03}, "rrc.hub", "H")

	// Populate link-derived state.
	hub.lock.Lock()
	hub.link = &rns.Link{}
	hub.Welcomed = true
	hub.MOTD = "hello motd"
	hub.Members["general"] = map[string]bool{"alice": true}
	hub.resourceExpectations["rid"] = &resourceExpectation{kind: "notice", size: 3}
	hub.pendingJoins["general"] = true
	hub.pendingParts["old"] = true
	hub.silentJoins["quiet"] = true
	hub.silentWhoRooms["who"] = true
	hub.lock.Unlock()

	hub.onClosed()

	hub.lock.Lock()
	defer hub.lock.Unlock()
	if hub.link != nil {
		t.Error("link not cleared")
	}
	if hub.Welcomed {
		t.Error("Welcomed not cleared")
	}
	if hub.MOTD != "" {
		t.Errorf("MOTD = %q, want empty", hub.MOTD)
	}
	if len(hub.Members) != 0 {
		t.Errorf("Members not cleared: %v", hub.Members)
	}
	if len(hub.resourceExpectations) != 0 {
		t.Errorf("resourceExpectations not cleared: %v", hub.resourceExpectations)
	}
	if len(hub.pendingJoins) != 0 || len(hub.pendingParts) != 0 || len(hub.silentJoins) != 0 || len(hub.silentWhoRooms) != 0 {
		t.Error("pending/silent maps not cleared")
	}
	if hub.Status != StatusDisconnected {
		t.Errorf("Status = %v, want StatusDisconnected", hub.Status)
	}
}

// TestOnClosedSchedulesReconnect verifies that with autoReconnect enabled and no
// manual disconnect, onClosed triggers a reconnect schedule.
func TestOnClosedSchedulesReconnect(t *testing.T) {
	t.Parallel()

	mgr := NewManager(tempDir(t), func() []byte { return []byte("me") })
	hub := mgr.AddHub([]byte{0x04}, "rrc.hub", "H")

	hub.afterFunc = func(d time.Duration, f func()) *time.Timer {
		return time.NewTimer(d)
	}

	hub.lock.Lock()
	hub.AutoReconnect = true
	hub.manualDisconnect = false
	hub.lock.Unlock()

	hub.onClosed()

	hub.lock.Lock()
	attempts := hub.reconnectAttempts
	hub.lock.Unlock()
	if attempts != 1 {
		t.Errorf("reconnectAttempts = %v, want 1 (reconnect scheduled)", attempts)
	}
}

// TestOnClosedNoReconnectWhenManualDisconnect verifies onClosed does not
// schedule a reconnect after a manual disconnect even with autoReconnect on.
func TestOnClosedNoReconnectWhenManualDisconnect(t *testing.T) {
	t.Parallel()

	mgr := NewManager(tempDir(t), func() []byte { return []byte("me") })
	hub := mgr.AddHub([]byte{0x05}, "rrc.hub", "H")

	hub.lock.Lock()
	hub.AutoReconnect = true
	hub.manualDisconnect = true
	hub.lock.Unlock()

	hub.onClosed()

	hub.lock.Lock()
	attempts := hub.reconnectAttempts
	hub.lock.Unlock()
	if attempts != 0 {
		t.Errorf("reconnectAttempts = %v, want 0 (no reconnect after manual disconnect)", attempts)
	}
}

// TestConnectWorkerIdentityUnknown verifies the FAILED branch when the hub
// identity cannot be recalled, mirroring Python _connect_worker.
func TestConnectWorkerIdentityUnknown(t *testing.T) {
	t.Parallel()

	mgr := NewManager(tempDir(t), func() []byte { return []byte("me") })
	hub := mgr.AddHub([]byte{0x06}, "rrc.hub", "H")

	hub.hasPathFn = func([]byte) bool { return true }
	hub.recallIdentityFn = func([]byte) *rns.Identity { return nil }
	hub.connectTimeout = 50 * time.Millisecond

	hub.connectWorker()

	hub.lock.Lock()
	defer hub.lock.Unlock()
	if hub.Status != StatusFailed {
		t.Errorf("Status = %v, want StatusFailed", hub.Status)
	}
	if hub.StatusText != "Hub identity unknown" {
		t.Errorf("StatusText = %q, want %q", hub.StatusText, "Hub identity unknown")
	}
}

// TestConnectWorkerHashMismatch verifies the FAILED branch when the resolved
// destination hash does not match the stored hub hash.
func TestConnectWorkerHashMismatch(t *testing.T) {
	t.Parallel()

	mgr := NewManager(tempDir(t), func() []byte { return []byte("me") })
	hub := mgr.AddHub([]byte{0x06, 0x06, 0x06}, "rrc.chat", "H")

	id := &rns.Identity{}
	hub.hasPathFn = func([]byte) bool { return true }
	hub.recallIdentityFn = func([]byte) *rns.Identity { return id }
	hub.buildDestFn = func(*rns.Identity) (*rns.Destination, error) {
		return &rns.Destination{Hash: []byte{0xff, 0xff, 0xff}}, nil
	}

	hub.connectWorker()

	hub.lock.Lock()
	defer hub.lock.Unlock()
	if hub.Status != StatusFailed {
		t.Errorf("Status = %v, want StatusFailed", hub.Status)
	}
	if hub.StatusText != "Hash/destination name mismatch" {
		t.Errorf("StatusText = %q, want %q", hub.StatusText, "Hash/destination name mismatch")
	}
}

// TestConnectAsyncGuard verifies ConnectAsync is a no-op when already
// connecting or connected, and otherwise spawns the worker with "Connecting".
func TestConnectAsyncGuard(t *testing.T) {
	t.Parallel()

	mgr := NewManager(tempDir(t), func() []byte { return []byte("me") })
	hub := mgr.AddHub([]byte{0x08}, "rrc.hub", "H")

	workerStarted := make(chan struct{}, 1)
	hub.connectWorkerFn = func() {
		select {
		case workerStarted <- struct{}{}:
		default:
		}
	}

	// Already connecting -> no-op.
	hub.lock.Lock()
	hub.Status = StatusConnecting
	hub.lock.Unlock()
	hub.ConnectAsync()
	select {
	case <-workerStarted:
		t.Error("worker started while already connecting")
	case <-time.After(50 * time.Millisecond):
	}

	// Disconnected -> spawns worker, status becomes Connecting.
	hub.lock.Lock()
	hub.Status = StatusDisconnected
	hub.lock.Unlock()
	hub.ConnectAsync()

	select {
	case <-workerStarted:
	case <-time.After(time.Second):
		t.Fatal("worker not started from ConnectAsync")
	}

	hub.lock.Lock()
	status := hub.Status
	text := hub.StatusText
	hub.lock.Unlock()
	if status != StatusConnecting {
		t.Errorf("Status = %v, want StatusConnecting", status)
	}
	if text != "Connecting" {
		t.Errorf("StatusText = %q, want %q", text, "Connecting")
	}
}

// TestHandleWelcomeParsesFieldsAfterCBORRoundTrip asserts the WELCOME envelope
// is fully parsed after a real CBOR encode→decode cycle. This is the
// prerequisite for the cross-process HELLO/WELCOME test (task 2.1): a WELCOME
// arriving from a Python hub (or even a Go hub over a real link) is CBOR bytes,
// and fxamacker decodes integer map keys/values as uint64, so handleWelcome
// must use the int/uint64-tolerant *Val helpers — not raw bodyMap[intKey]
// indexing, which silently misses every field except Welcomed.
//
// Golden WELCOME body mirrors Python's RRC server contract (RRC.py:73-82) and
// the Go handleHello sender (hub.go:1196-1207): hub name, ver "0.1", empty
// caps, limits {0:32, 1:64, 2:350, 3:32, 4:240}.
func TestHandleWelcomeParsesFieldsAfterCBORRoundTrip(t *testing.T) {
	t.Parallel()

	mgr := NewManager(tempDir(t), func() []byte { return []byte("testhash") })
	mgr.SetNickname("TestNick")
	hub := mgr.AddHub([]byte("hubhash"), "rrc.chat", "TestHub")

	welcomeBody := map[any]any{
		BWelcomeHub:  []byte("PyHub"),
		BWelcomeVer:  []byte("0.1"),
		BWelcomeCaps: map[any]any{},
		BWelcomeLimits: map[any]any{
			LMaxNickBytes:           32,
			LMaxRoomNameBytes:       64,
			LMaxMsgBodyBytes:        350,
			LMaxRoomsPerSession:     32,
			LRateLimitMsgsPerMinute: 240,
		},
	}
	env := MakeEnvelope(TypeWelcome, nil, nil, nil, welcomeBody, []byte("mid-w"), NowMs())
	data, err := EncodeEnvelope(env)
	if err != nil {
		t.Fatalf("EncodeEnvelope: %v", err)
	}

	hub.HandleData(data)

	hub.lock.Lock()
	welcomed := hub.Welcomed
	hubName := hub.HubName
	hubVer := hub.HubVersion
	maxNick := hub.MaxNickBytes
	maxRoom := hub.MaxRoomNameBytes
	maxMsg := hub.MaxMsgBodyBytes
	maxRooms := hub.MaxRoomsPerSession
	rate := hub.RateLimitMsgsPerMin
	hub.lock.Unlock()

	if !welcomed {
		t.Error("Welcomed = false, want true")
	}
	if hubName != "PyHub" {
		t.Errorf("HubName = %q, want %q", hubName, "PyHub")
	}
	if hubVer != "0.1" {
		t.Errorf("HubVersion = %q, want %q", hubVer, "0.1")
	}
	if maxNick != 32 {
		t.Errorf("MaxNickBytes = %v, want 32", maxNick)
	}
	if maxRoom != 64 {
		t.Errorf("MaxRoomNameBytes = %v, want 64", maxRoom)
	}
	if maxMsg != 350 {
		t.Errorf("MaxMsgBodyBytes = %v, want 350", maxMsg)
	}
	if maxRooms != 32 {
		t.Errorf("MaxRoomsPerSession = %v, want 32", maxRooms)
	}
	if rate != 240 {
		t.Errorf("RateLimitMsgsPerMin = %v, want 240", rate)
	}
}

// TestHubListViewAccessors pins the locked read accessors used by the TUI
// HubView adapter to render the channels hub/room list (mirroring Python
// Channels._compose_list_widgets, Channels.py:1599-1662, reading hub.name,
// hub.status, hub.rooms, hub.messages, hub.unread_rooms, hub.mention_rooms).
// The accessors return sorted lists so ComposeHubList's sorted union is stable.
func TestHubListViewAccessors(t *testing.T) {
	t.Parallel()

	hub := NewHub(nil, []byte{0x01, 0x02, 0x03, 0x04}, "rrc.hub", "My Hub")
	hub.SetStatus(StatusConnected, "ok")

	hub.AddRoom("random")
	hub.AddRoom("general")

	// A message-bearing but not-joined room appears via Messages keys.
	hub.lock.Lock()
	hub.Messages["zzz"] = []*RRCMessage{{Text: "hi"}}
	hub.UnreadRooms["onlymsg"] = true
	hub.MentionRooms["joined"] = true
	hub.lock.Unlock()

	if got := hub.GetHubName(); got != "My Hub" {
		t.Errorf("GetHubName = %q, want %q", got, "My Hub")
	}
	if got := hub.GetHubStatus(); got != StatusConnected {
		t.Errorf("GetHubStatus = %v, want %v", got, StatusConnected)
	}
	if got, want := hub.JoinedRoomList(), []string{"general", "random"}; !reflect.DeepEqual(got, want) {
		t.Errorf("JoinedRoomList = %v, want %v", got, want)
	}
	// MessageRoomList includes both joined rooms (which get an empty Messages
	// slice on AddRoom) and the explicit zzz room.
	if got, want := hub.MessageRoomList(), []string{"general", "random", "zzz"}; !reflect.DeepEqual(got, want) {
		t.Errorf("MessageRoomList = %v, want %v", got, want)
	}
	if got, want := hub.UnreadRoomList(), []string{"onlymsg"}; !reflect.DeepEqual(got, want) {
		t.Errorf("UnreadRoomList = %v, want %v", got, want)
	}
	if got, want := hub.MentionRoomList(), []string{"joined"}; !reflect.DeepEqual(got, want) {
		t.Errorf("MentionRoomList = %v, want %v", got, want)
	}
}
