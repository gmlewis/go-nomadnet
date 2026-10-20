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
	"os"
	"path/filepath"
	"testing"
)

// TestIndexFilenameIsTheFilesTheIndexerWrites pins the conversation index file
// name against the file WriteIndex creates and ReadIndex reads: the name both
// implementations must agree on (Python writes ".index" in each conversation
// directory). A constant that drifted from the writer would leave the test
// asserting a string nothing used.
func TestIndexFilenameIsTheFilesTheIndexerWrites(t *testing.T) {
	t.Parallel()

	if got := IndexFilename(); got != ".index" {
		t.Fatalf("IndexFilename() = %q, want %q — the name Python writes per conversation", got, ".index")
	}

	dir := tempDir(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	// WriteIndex only records a message whose state is known (its cached state is what
	// to_index_entry serialises), so give the entry a state.
	msg := NewMessage(filepath.Join(dir, "0123456789abcdef"))
	state := StateSent
	msg.CachedState = &state
	msgs := []*Message{msg}
	if err := WriteIndex(dir, msgs); err != nil {
		t.Fatalf("WriteIndex: %v", err)
	}

	path := filepath.Join(dir, IndexFilename())
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("WriteIndex did not create %v: %v", path, err)
	}
	if got := ReadIndex(dir); len(got) == 0 {
		t.Errorf("ReadIndex(%v) read nothing from the file WriteIndex created at %v", dir, path)
	}
}

// TestSetAttachmentPathReachesScannedMessages pins the cache-level half of attachment
// resolution: ConversationCache.SetAttachmentPath stamps the base directory onto every
// conversation it stores, and ScanStorage propagates it to each message so
// Message.attachmentDir can find the extracted files. Setting the path on a Message
// directly is covered by the attachment tests; without this wiring the extracted
// attachments are on disk but unreachable.
func TestSetAttachmentPathReachesScannedMessages(t *testing.T) {
	t.Parallel()

	base := tempDir(t)
	attachDir := filepath.Join(base, "attachments")
	convDir := filepath.Join(base, "conversation")
	if err := os.MkdirAll(convDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// One real message file in the conversation directory, so ScanStorage has something
	// to scan.
	attachmentFixture(t, convDir, "notes.txt", []byte("payload"))

	cache := NewConversationCache()
	cache.SetAttachmentPath(attachDir)

	conv := NewConversation("0123456789abcdef0123456789abcdef", convDir)
	cache.Store(conv)
	if err := conv.ScanStorage(); err != nil {
		t.Fatalf("ScanStorage: %v", err)
	}

	if len(conv.Messages) != 1 {
		t.Fatalf("conversation holds %v messages, want 1", len(conv.Messages))
	}
	msg := conv.Messages[0]
	if msg.AttachmentPath != attachDir {
		t.Errorf("scanned message AttachmentPath = %q, want the cache's %q", msg.AttachmentPath, attachDir)
	}
	if got := msg.attachmentDir(); filepath.Dir(got) != attachDir {
		t.Errorf("message attachment directory = %q, want a directory under %q", got, attachDir)
	}
}
