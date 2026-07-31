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

package conversation

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns"
	"github.com/vmihailenco/msgpack/v5"
)

func TestMessageStates(t *testing.T) {
	t.Parallel()

	if StateDraft != 0 {
		t.Errorf("StateDraft = %d, want 0", StateDraft)
	}
	if StateFailed != 5 {
		t.Errorf("StateFailed = %d, want 5", StateFailed)
	}
}

func TestNewMessage(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	path := filepath.Join(dir, "abc123")
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	msg := NewMessage(path)
	if msg.FilePath != path {
		t.Errorf("FilePath = %q, want %q", msg.FilePath, path)
	}
	if msg.Loaded {
		t.Error("Loaded = true, want false")
	}
	if msg.SortTimestamp == 0 {
		t.Error("SortTimestamp should be non-zero for existing file")
	}
}

func TestMessageGetTimestamp(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	path := filepath.Join(dir, "abc123")
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	msg := NewMessage(path)
	ts := msg.GetTimestamp()
	if ts == 0 {
		t.Error("GetTimestamp returned 0")
	}
}

func TestMessageGetTitleEmpty(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	path := filepath.Join(dir, "abc123")
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	msg := NewMessage(path)
	if title := msg.GetTitle(); title != "" {
		t.Errorf("GetTitle = %q, want empty", title)
	}
}

func TestMessageGetContentEmpty(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	path := filepath.Join(dir, "abc123")
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	msg := NewMessage(path)
	if content := msg.GetContent(); content != "" {
		t.Errorf("GetContent = %q, want empty", content)
	}
}

func TestMessageGetStateDefault(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	path := filepath.Join(dir, "abc123")
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	msg := NewMessage(path)
	if state := msg.GetState(); state != StateDraft {
		t.Errorf("GetState = %d, want %d", state, StateDraft)
	}
}

func TestMessageSignatureDescription(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	path := filepath.Join(dir, "abc123")
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	msg := NewMessage(path)
	desc := msg.GetSignatureDescription()
	if desc != "Unknown signature validation failure" {
		t.Errorf("GetSignatureDescription = %q, want %q", desc, "Unknown signature validation failure")
	}
}

func TestMessagePurge(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	path := filepath.Join(dir, "abc123")
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	msg := NewMessage(path)
	if err := msg.Purge(); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("File still exists after Purge")
	}
}

func TestMessageUnload(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	path := filepath.Join(dir, "abc123")
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	msg := NewMessage(path)
	msg.Load()
	if !msg.Loaded {
		t.Error("Loaded = false after Load")
	}

	msg.Unload()
	if msg.Loaded {
		t.Error("Loaded = true after Unload")
	}
}

func TestToIndexEntryAndRestore(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	path := filepath.Join(dir, "abc123")
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	ts := 1234567890.0
	state := StateSent
	sigValid := true
	msg := NewMessage(path)
	msg.Timestamp = &ts
	msg.CachedState = &state
	msg.CachedTitle = "Test Title"
	msg.CachedContent = "Hello World"
	msg.CachedSourceHash = []byte{0x01, 0x02}
	msg.CachedTransportEncrypted = true
	msg.CachedSignatureValidated = &sigValid
	msg.CachedMethod = 1
	msg.CachedHasAttachments = true
	msg.CachedAttachmentNames = []AttachmentInfo{
		{Type: "file", Name: "doc.pdf", Size: 1024},
	}

	entry := msg.ToIndexEntry()

	// Verify entry can be serialized/deserialized
	data, err := msgpack.Marshal(entry)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var decoded map[string]any
	if err := msgpack.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	// Restore to a new message
	msg2 := NewMessage(path)
	msg2.RestoreFromIndex(decoded)

	if msg2.Timestamp == nil || *msg2.Timestamp != ts {
		t.Errorf("Timestamp = %v, want %v", msg2.Timestamp, ts)
	}
	if msg2.CachedTitle != "Test Title" {
		t.Errorf("CachedTitle = %q, want %q", msg2.CachedTitle, "Test Title")
	}
	if msg2.CachedContent != "Hello World" {
		t.Errorf("CachedContent = %q, want %q", msg2.CachedContent, "Hello World")
	}
	if !msg2.CachedTransportEncrypted {
		t.Error("CachedTransportEncrypted = false, want true")
	}
	if msg2.CachedSignatureValidated == nil || !*msg2.CachedSignatureValidated {
		t.Error("CachedSignatureValidated = nil/false, want true")
	}
	if msg2.CachedMethod != 1 {
		t.Errorf("CachedMethod = %d, want 1", msg2.CachedMethod)
	}
	if !msg2.CachedHasAttachments {
		t.Error("CachedHasAttachments = false, want true")
	}
}

func TestReadWriteIndex(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	convPath := filepath.Join(dir, "abc123")
	if err := os.MkdirAll(convPath, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a message file
	msgPath := filepath.Join(convPath, "0102030405060708010203040506070801020304050607080102030405060708")
	if err := os.WriteFile(msgPath, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	ts := 1234567890.0
	state := StateSent
	msg := NewMessage(msgPath)
	msg.Timestamp = &ts
	msg.CachedState = &state
	msg.CachedTitle = "Test"

	// Write index
	if err := WriteIndex(convPath, []*Message{msg}); err != nil {
		t.Fatalf("WriteIndex: %v", err)
	}

	// Read it back
	index := ReadIndex(convPath)
	if len(index) != 1 {
		t.Fatalf("ReadIndex len = %d, want 1", len(index))
	}

	entry, ok := index["0102030405060708010203040506070801020304050607080102030405060708"]
	if !ok {
		t.Fatal("Index entry not found for message")
	}

	ie, ok := entry.(map[string]any)
	if !ok {
		t.Fatal("Index entry is not a map")
	}

	if title, ok := ie["title"].(string); !ok || title != "Test" {
		t.Errorf("title = %v, want %q", ie["title"], "Test")
	}
}

func TestReadIndexMissing(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	index := ReadIndex(filepath.Join(dir, "nonexistent"))
	if len(index) != 0 {
		t.Errorf("ReadIndex for missing file returned %d entries, want 0", len(index))
	}
}

func TestScanStorage(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	convPath := filepath.Join(dir, "abc123")
	if err := os.MkdirAll(convPath, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create some message files
	for i := 0; i < 3; i++ {
		hash := make([]byte, 32)
		hash[0] = byte(i)
		name := hexHash(hash)
		msgPath := filepath.Join(convPath, name)
		if err := os.WriteFile(msgPath, []byte("test"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	conv := NewConversation("abc123", convPath)
	if err := conv.ScanStorage(); err != nil {
		t.Fatalf("ScanStorage: %v", err)
	}

	if len(conv.Messages) != 3 {
		t.Errorf("Messages len = %d, want 3", len(conv.Messages))
	}
}

func TestScanStorageEmpty(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	convPath := filepath.Join(dir, "abc123")
	if err := os.MkdirAll(convPath, 0o755); err != nil {
		t.Fatal(err)
	}

	conv := NewConversation("abc123", convPath)
	if err := conv.ScanStorage(); err != nil {
		t.Fatalf("ScanStorage: %v", err)
	}

	if len(conv.Messages) != 0 {
		t.Errorf("Messages len = %d, want 0", len(conv.Messages))
	}
}

func TestScanStorageSkipsNonHex(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	convPath := filepath.Join(dir, "abc123")
	if err := os.MkdirAll(convPath, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create valid and invalid message files
	validHash := "0102030405060708010203040506070801020304050607080102030405060708"
	if err := os.WriteFile(filepath.Join(convPath, validHash), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(convPath, "unread"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(convPath, "not_a_hash"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(convPath, ".index"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	conv := NewConversation("abc123", convPath)
	if err := conv.ScanStorage(); err != nil {
		t.Fatalf("ScanStorage: %v", err)
	}

	if len(conv.Messages) != 1 {
		t.Errorf("Messages len = %d, want 1", len(conv.Messages))
	}
}

func TestPurgeFailed(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	convPath := filepath.Join(dir, "abc123")
	if err := os.MkdirAll(convPath, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create message files
	for i := 0; i < 3; i++ {
		hash := make([]byte, 32)
		hash[0] = byte(i + 10)
		name := hexHash(hash)
		msgPath := filepath.Join(convPath, name)
		if err := os.WriteFile(msgPath, []byte("test"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	conv := NewConversation("abc123", convPath)
	if err := conv.ScanStorage(); err != nil {
		t.Fatal(err)
	}

	// Mark one message as failed
	failedState := StateFailed
	if len(conv.Messages) > 0 {
		conv.Messages[0].CachedState = &failedState
		conv.Messages[0].Loaded = true
	}

	conv.PurgeFailed()

	remaining := 0
	for _, msg := range conv.Messages {
		if msg.GetState() != StateFailed {
			remaining++
		}
	}
	if remaining != 2 {
		t.Errorf("After PurgeFailed, remaining = %d, want 2", remaining)
	}
}

func TestClearHistory(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	convPath := filepath.Join(dir, "abc123")
	if err := os.MkdirAll(convPath, 0o755); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		hash := make([]byte, 32)
		hash[0] = byte(i + 20)
		name := hexHash(hash)
		if err := os.WriteFile(filepath.Join(convPath, name), []byte("test"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	conv := NewConversation("abc123", convPath)
	if err := conv.ScanStorage(); err != nil {
		t.Fatal(err)
	}

	conv.ClearHistory()

	if len(conv.Messages) != 0 {
		t.Errorf("After ClearHistory, Messages len = %d, want 0", len(conv.Messages))
	}

	// Verify files are deleted
	entries, _ := os.ReadDir(convPath)
	for _, e := range entries {
		if !e.IsDir() && e.Name() != ".index" {
			t.Errorf("File %s still exists after ClearHistory", e.Name())
		}
	}
}

func TestConversationList(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	convPath := filepath.Join(dir, "conversations")
	if err := os.MkdirAll(convPath, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create two conversations
	hash1 := "0102030405060708010203040506070801020304050607080102030405060708"
	hash2 := "090a0b0c0d0e0f10090a0b0c0d0e0f10090a0b0c0d0e0f10090a0b0c0d0e0f10"
	if err := os.MkdirAll(filepath.Join(convPath, hash1), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(convPath, hash2), 0o755); err != nil {
		t.Fatal(err)
	}

	// Create unread flag in one
	if err := os.WriteFile(filepath.Join(convPath, hash1, "unread"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	displayNames := map[string]string{
		hash1: "Alice",
		hash2: "Bob",
	}
	trustLevels := map[string]byte{
		hash1: 0xFF, // Trusted
		hash2: 0x02, // Unknown
	}

	list := ConversationList(convPath, displayNames, trustLevels)
	if len(list) != 2 {
		t.Fatalf("ConversationList len = %d, want 2", len(list))
	}

	// Find Alice
	found := false
	for _, info := range list {
		if info.SourceHash == hash1 {
			if info.DisplayName != "Alice" {
				t.Errorf("Alice DisplayName = %q, want %q", info.DisplayName, "Alice")
			}
			if !info.Unread {
				t.Error("Alice Unread = false, want true")
			}
			if info.TrustLevel != 0xFF {
				t.Errorf("Alice TrustLevel = 0x%02X, want 0xFF", info.TrustLevel)
			}
			found = true
		}
	}
	if !found {
		t.Error("Alice not found in conversation list")
	}
}

func TestDeleteConversation(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	convPath := filepath.Join(dir, "conversations")
	hash := "0102030405060708010203040506070801020304050607080102030405060708"
	if err := os.MkdirAll(filepath.Join(convPath, hash), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(convPath, hash, "unread"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	// Cache it first
	cache := NewConversationCache()
	c := NewConversation(hash, filepath.Join(convPath, hash))
	cache.Store(c)

	if err := cache.Delete(hash, convPath); err != nil {
		t.Fatalf("DeleteConversation: %v", err)
	}

	if _, err := os.Stat(filepath.Join(convPath, hash)); !os.IsNotExist(err) {
		t.Error("Conversation directory still exists after delete")
	}

	if cache.Get(hash) != nil {
		t.Error("Conversation still cached after delete")
	}
}

func TestCacheConversation(t *testing.T) {
	t.Parallel()

	cache := NewConversationCache()

	hash := "abc123"
	c := NewConversation(hash, "/tmp/test")
	cache.Store(c)

	got := cache.Get(hash)
	if got != c {
		t.Error("Cache.Get did not return the cached conversation")
	}

	if cache.Get("nonexistent") != nil {
		t.Error("Cache.Get returned non-nil for nonexistent hash")
	}
}

func TestIngestCreatesConversationDirAndWritesMessage(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}

	dest, err := rns.NewDestination(ts, id, rns.DestinationIn, rns.DestinationSingle, "lxmf", "delivery")
	if err != nil {
		t.Fatal(err)
	}

	msg, err := lxmf.NewMessage(dest, dest, "test content", "test title", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := msg.Pack(); err != nil {
		t.Fatal(err)
	}

	cache := NewConversationCache()
	ingestedPath, err := cache.Ingest(msg, dir, false)
	if err != nil {
		t.Fatalf("Ingest error: %v", err)
	}

	sourceHex := hex.EncodeToString(msg.SourceHash)
	convDir := filepath.Join(dir, sourceHex)

	if _, statErr := os.Stat(convDir); os.IsNotExist(statErr) {
		t.Errorf("conversation dir %q should exist", convDir)
	}
	if ingestedPath == "" {
		t.Error("Ingest should return the ingested file path")
	}

	list := ConversationList(dir, nil, nil)
	found := false
	for _, ci := range list {
		if ci.SourceHash == sourceHex {
			found = true
		}
	}
	if !found {
		t.Errorf("source %q not found in ConversationList", sourceHex)
	}
}

func TestIngestTwoMessagesSameSourceSingleConversation(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}

	dest, err := rns.NewDestination(ts, id, rns.DestinationIn, rns.DestinationSingle, "lxmf", "delivery")
	if err != nil {
		t.Fatal(err)
	}

	msg1, err := lxmf.NewMessage(dest, dest, "first", "title1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := msg1.Pack(); err != nil {
		t.Fatal(err)
	}

	msg2, err := lxmf.NewMessage(dest, dest, "second", "title2", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := msg2.Pack(); err != nil {
		t.Fatal(err)
	}

	cache := NewConversationCache()
	if _, err := cache.Ingest(msg1, dir, false); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Ingest(msg2, dir, false); err != nil {
		t.Fatal(err)
	}

	list := ConversationList(dir, nil, nil)
	sourceHex := hex.EncodeToString(msg1.SourceHash)
	if len(list) != 1 {
		t.Errorf("ConversationList len = %d, want 1", len(list))
	}
	if len(list) > 0 && list[0].SourceHash != sourceHex {
		t.Errorf("SourceHash = %q, want %q", list[0].SourceHash, sourceHex)
	}

	conv := NewConversation(sourceHex, filepath.Join(dir, sourceHex))
	if err := conv.ScanStorage(); err != nil {
		t.Fatal(err)
	}
	if conv.MessageCount() != 2 {
		t.Errorf("MessageCount = %d, want 2", conv.MessageCount())
	}
}

func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "nomadnet-conversation-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func hexHash(hash []byte) string {
	const hexDigits = "0123456789abcdef"
	buf := make([]byte, len(hash)*2)
	for i, b := range hash {
		buf[i*2] = hexDigits[b>>4]
		buf[i*2+1] = hexDigits[b&0x0f]
	}
	return string(buf)
}
