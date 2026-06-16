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
	"os"
	"testing"
	"time"
)

func TestProtocolConstants(t *testing.T) {
	t.Parallel()

	if RRCVersion != 1 {
		t.Errorf("RRCVersion = %d, want 1", RRCVersion)
	}
	if TypeHello != 1 {
		t.Errorf("TypeHello = %d, want 1", TypeHello)
	}
	if TypeWelcome != 2 {
		t.Errorf("TypeWelcome = %d, want 2", TypeWelcome)
	}
	if TypeMsg != 20 {
		t.Errorf("TypeMsg = %d, want 20", TypeMsg)
	}
	if TypeResourceEnvelope != 50 {
		t.Errorf("TypeResourceEnvelope = %d, want 50", TypeResourceEnvelope)
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
		t.Errorf("ts = %v, want %d", entry[HTS], 1234567890)
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
		t.Errorf("Ts = %d, want %d", msg.Ts, 9876543210)
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
		t.Errorf("version = %v, want %d", env[KeyVersion], RRCVersion)
	}
	if env[KeyType] != TypeMsg {
		t.Errorf("type = %v, want %d", env[KeyType], TypeMsg)
	}
	if env[KeyTimestamp] != ts {
		t.Errorf("timestamp = %v, want %d", env[KeyTimestamp], ts)
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
		t.Errorf("decoded envelope has %d keys, want >= 3", len(decoded))
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
		t.Errorf("NowMs() = %d not in range [%d, %d]", ts, before, after)
	}
}

func TestMsgID(t *testing.T) {
	t.Parallel()

	id1 := MsgID()
	id2 := MsgID()

	if len(id1) != 8 {
		t.Errorf("MsgID len = %d, want 8", len(id1))
	}
	if len(id2) != 8 {
		t.Errorf("MsgID len = %d, want 8", len(id2))
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
		t.Errorf("Status = %d, want %d", hub.Status, StatusDisconnected)
	}
	if hub.MaxNickBytes != DefaultMaxNickBytes {
		t.Errorf("MaxNickBytes = %d, want %d", hub.MaxNickBytes, DefaultMaxNickBytes)
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
		t.Errorf("DisplayNameFor = %q (len %d), want ≤12 chars", name, len(name))
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
	hub.historyPath = dir

	mid := hub.SendMessage("general", "Hello!")
	if len(mid) == 0 {
		t.Error("SendMessage returned empty message ID")
	}

	hub.lock.Lock()
	msgs := hub.Messages["general"]
	hub.lock.Unlock()

	if len(msgs) != 1 {
		t.Fatalf("Messages len = %d, want 1", len(msgs))
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
		t.Fatalf("Messages len = %d, want 1", len(msgs))
	}
	if msgs[0].Kind != "action" {
		t.Errorf("Message kind = %q, want %q", msgs[0].Kind, "action")
	}
}

func TestHubSetStatus(t *testing.T) {
	t.Parallel()

	hash := []byte{0x01, 0x02, 0x03, 0x04}
	hub := NewHub(nil, hash, "", "")

	hub.SetStatus(StatusConnected, "Connected!")
	if hub.Status != StatusConnected {
		t.Errorf("Status = %d, want %d", hub.Status, StatusConnected)
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
		t.Errorf("Hubs len = %d, want 1", len(mgr.Hubs))
	}

	// Adding same hub returns existing
	hub1b := mgr.AddHub(hash1, "rrc.hub", "Hub 1")
	if hub1 != hub1b {
		t.Error("AddHub returned different hub for same hash")
	}

	hash2 := []byte{0x05, 0x06, 0x07, 0x08}
	mgr.AddHub(hash2, "rrc.hub", "Hub 2")
	if len(mgr.Hubs) != 2 {
		t.Errorf("Hubs len = %d, want 2", len(mgr.Hubs))
	}

	mgr.RemoveHub(hub1)
	if len(mgr.Hubs) != 1 {
		t.Errorf("Hubs len after remove = %d, want 1", len(mgr.Hubs))
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
		t.Fatalf("Loaded Hubs len = %d, want 1", len(mgr2.Hubs))
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
		t.Errorf("Loaded Hubs len = %d, want 0", len(mgr2.Hubs))
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
			t.Errorf("Hub Status = %d after shutdown, want %d", hub.Status, StatusDisconnected)
		}
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
		t.Fatalf("GetMessages len = %d, want 1", len(msgs))
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

func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "nomadnet-rrc-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}
