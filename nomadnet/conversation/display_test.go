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
	"time"

	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns"
)

// writeLXMFixtureAt writes a packed LXMF message into dir with the given
// state/method and returns its file path and hash. It stamps the file's
// mtime to `when` so DisplayMessages ordering (by sort_timestamp) is
// deterministic across the two messages.
func writeLXMFixtureAt(t *testing.T, dir string, content, title string, state int, method int, when time.Time) (path string, hash []byte) {
	t.Helper()
	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	dest, err := rns.NewDestination(ts, id, rns.DestinationIn, rns.DestinationSingle, "lxmf", "delivery")
	if err != nil {
		t.Fatal(err)
	}
	msg, err := lxmf.NewMessage(dest, dest, content, title, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := msg.Pack(); err != nil {
		t.Fatal(err)
	}
	msg.State = state
	msg.Method = method
	msg.TransportEncrypted = true
	msg.TransportEncryption = "AES-128"

	written, err := msg.WriteToDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(written, when, when); err != nil {
		t.Fatal(err)
	}
	return written, msg.Hash
}

// TestDisplayMessagesSortingAndFields verifies Conversation.DisplayMessages
// mirrors Python ConversationWidget.update_message_widgets: messages are
// sorted ascending by sort_timestamp (oldest first, reverse=False) and each
// entry carries the raw LXMF fields the LXMessageWidget header needs (raw
// state int, method, source hash, transport encryption, title, content).
func TestDisplayMessagesSortingAndFields(t *testing.T) {
	t.Parallel()

	ts := rns.NewTransportSystem(nil)
	dir := tempDir(t)
	convDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(convDir, 0o755); err != nil {
		t.Fatal(err)
	}

	older := time.Now().Add(-2 * time.Hour)
	newer := time.Now().Add(-1 * time.Hour)
	_, _ = writeLXMFixtureAt(t, convDir, "first body", "Title One", lxmf.StateSent, lxmf.MethodDirect, older)
	_, _ = writeLXMFixtureAt(t, convDir, "second body", "Title Two", lxmf.StateDelivered, lxmf.MethodPropagated, newer)

	conv := NewConversation("src", convDir)
	conv.SetTransport(ts)
	if err := conv.ScanStorage(); err != nil {
		t.Fatalf("ScanStorage: %v", err)
	}

	got := conv.DisplayMessages()
	if len(got) != 2 {
		t.Fatalf("DisplayMessages returned %v entries, want 2", len(got))
	}

	// Ascending by sort_timestamp: oldest first.
	if got[0].Title != "Title One" {
		t.Errorf("first entry title = %q, want %q (oldest first)", got[0].Title, "Title One")
	}
	if got[1].Title != "Title Two" {
		t.Errorf("second entry title = %q, want %q", got[1].Title, "Title Two")
	}
	if got[0].Content != "first body" {
		t.Errorf("first entry content = %q, want %q", got[0].Content, "first body")
	}
	if got[1].Content != "second body" {
		t.Errorf("second entry content = %q, want %q", got[1].Content, "second body")
	}

	// Raw LXMF state ints (0x04 SENT, 0x08 DELIVERED), not the mapped
	// MessageState enum — the LXMessageWidget header compares against the
	// raw LXMF constants.
	if got[0].State != int(lxmf.StateSent) {
		t.Errorf("first entry raw state = %#x, want %#x", got[0].State, lxmf.StateSent)
	}
	if got[1].State != int(lxmf.StateDelivered) {
		t.Errorf("second entry raw state = %#x, want %#x", got[1].State, lxmf.StateDelivered)
	}
	if got[1].Method != int(lxmf.MethodPropagated) {
		t.Errorf("second entry method = %#x, want %#x", got[1].Method, lxmf.MethodPropagated)
	}

	// Transport encryption metadata is parsed from the envelope.
	if !got[0].TransportEncrypted {
		t.Error("first entry should report transport encrypted")
	}

	// Source hash is the sender's LXMF hash parsed from the envelope.
	if len(got[0].SourceHash) == 0 {
		t.Error("first entry source hash is empty; envelope was not parsed")
	}
}

// TestDisplayMessagesEmpty verifies an empty conversation yields no display
// entries rather than a nil-vs-empty ambiguity.
func TestDisplayMessagesEmpty(t *testing.T) {
	t.Parallel()
	ts := rns.NewTransportSystem(nil)
	dir := tempDir(t)
	convDir := filepath.Join(dir, "empty")
	if err := os.MkdirAll(convDir, 0o755); err != nil {
		t.Fatal(err)
	}
	conv := NewConversation("empty", convDir)
	conv.SetTransport(ts)
	if err := conv.ScanStorage(); err != nil {
		t.Fatalf("ScanStorage: %v", err)
	}
	got := conv.DisplayMessages()
	if len(got) != 0 {
		t.Errorf("DisplayMessages returned %v entries, want 0", len(got))
	}
}
