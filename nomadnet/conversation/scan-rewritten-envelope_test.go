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
	"time"

	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns"
)

// TestScanStorageRereadsRewrittenEnvelope pins Python scan_storage parity for a
// message whose file changed while it was cached (Conversation.py:2240-2258):
// a path already in memory is ALWAYS reused, and when the file is newer than
// the cached sort_timestamp the cached state is dropped so it is re-read from
// the envelope. The Go port instead required the file's mtime to equal
// SortTimestamp exactly and otherwise threw the in-memory message away and
// rebuilt it from the .index — which reinstates the stale snapshot.
//
// The live symptom is a delivery proof: the proof rewrites the envelope with
// LXMF DELIVERED, and the conversation kept rendering the indexed SENT state
// (the 󰳐 glyph) forever, because scan_storage restored the index entry instead
// of the rewritten file. Real storage shows exactly that split —
// conversations/<peer>/.index says state 4 while the message file says 8.
func TestScanStorageRereadsRewrittenEnvelope(t *testing.T) {
	t.Parallel()

	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	dest, err := rns.NewDestination(ts, id, rns.DestinationIn, rns.DestinationSingle, "lxmf", "delivery")
	if err != nil {
		t.Fatal(err)
	}

	convDir := filepath.Join(tempDir(t), "peer")
	if err := os.MkdirAll(convDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// A transmitted outbound message: the envelope on disk says SENT.
	m, err := lxmf.NewMessage(dest, dest, "body", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Pack(); err != nil {
		t.Fatal(err)
	}
	m.SetState(lxmf.StateSent)
	path, err := m.WriteToDirectory(convDir)
	if err != nil {
		t.Fatal(err)
	}

	conv := NewConversation(hex.EncodeToString(dest.Hash), convDir)
	conv.SetTransport(ts)
	if err := conv.ScanStorage(); err != nil {
		t.Fatal(err)
	}
	if len(conv.Messages) != 1 {
		t.Fatalf("message count = %v, want 1", len(conv.Messages))
	}

	// The UI displays it (which caches the state), then rescans, which writes
	// the .index entry — the state a later scan would otherwise restore.
	if got := conv.DisplayMessages()[0].State; got != lxmf.StateSent {
		t.Fatalf("displayed state = %#x, want %#x (SENT)", got, lxmf.StateSent)
	}
	if err := conv.ScanStorage(); err != nil {
		t.Fatal(err)
	}

	// The delivery proof rewrites the envelope with DELIVERED.
	m.SetState(lxmf.StateDelivered)
	if _, err := m.WriteToDirectory(convDir); err != nil {
		t.Fatal(err)
	}
	// Make the rewrite unambiguously newer than the cached sort_timestamp, so
	// the test does not depend on filesystem timestamp resolution.
	later := time.Now().Add(5 * time.Second)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}

	if err := conv.ScanStorage(); err != nil {
		t.Fatal(err)
	}
	got := conv.DisplayMessages()[0].State
	if got != lxmf.StateDelivered {
		t.Errorf("state = %#x, want %#x (DELIVERED): scan_storage restored the stale .index entry instead of re-reading the rewritten envelope, so the header keeps the SENT glyph",
			got, lxmf.StateDelivered)
	}

	// A fresh scan of the same directory (what a restart does) must also show
	// the envelope's state: the snapshot the .index holds was taken before the
	// rewrite, so the file supersedes it.
	fresh := NewConversation(hex.EncodeToString(dest.Hash), convDir)
	fresh.SetTransport(ts)
	if err := fresh.ScanStorage(); err != nil {
		t.Fatal(err)
	}
	if got := fresh.DisplayMessages()[0].State; got != lxmf.StateDelivered {
		t.Errorf("fresh scan state = %#x, want %#x (DELIVERED): the .index snapshot taken before the rewrite still won",
			got, lxmf.StateDelivered)
	}
}

// TestScanStorageEnvelopeNewerThanIndexSnapshot covers the shape the operator's
// own storage is in: the .index holds the state as it was when the message was
// displayed, and the envelope was rewritten afterwards (a delivery proof). A
// scan that has no in-memory message must still render the file, not the
// snapshot — conversations/<peer>/.index said state 4 (SENT) while the message
// file said 8 (DELIVERED), 0.13 s newer.
func TestScanStorageEnvelopeNewerThanIndexSnapshot(t *testing.T) {
	t.Parallel()

	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	dest, err := rns.NewDestination(ts, id, rns.DestinationIn, rns.DestinationSingle, "lxmf", "delivery")
	if err != nil {
		t.Fatal(err)
	}

	convDir := filepath.Join(tempDir(t), "peer")
	if err := os.MkdirAll(convDir, 0o755); err != nil {
		t.Fatal(err)
	}

	m, err := lxmf.NewMessage(dest, dest, "body", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Pack(); err != nil {
		t.Fatal(err)
	}
	m.SetState(lxmf.StateSent)
	path, err := m.WriteToDirectory(convDir)
	if err != nil {
		t.Fatal(err)
	}

	// Snapshot the SENT state into the .index, exactly as a display followed by
	// a scan does.
	snapshot := NewConversation(hex.EncodeToString(dest.Hash), convDir)
	snapshot.SetTransport(ts)
	if err := snapshot.ScanStorage(); err != nil {
		t.Fatal(err)
	}
	_ = snapshot.DisplayMessages()
	if err := snapshot.ScanStorage(); err != nil {
		t.Fatal(err)
	}
	if _, ok := ReadIndex(convDir).Get(filepath.Base(path)); !ok {
		t.Fatal("the .index holds no entry for the message, so there is no snapshot to supersede")
	}

	// The proof rewrites the envelope with DELIVERED, just after the snapshot.
	m.SetState(lxmf.StateDelivered)
	if _, err := m.WriteToDirectory(convDir); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(5 * time.Second)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}

	// A scan with no in-memory history — a restarted client — must render the
	// envelope.
	restarted := NewConversation(hex.EncodeToString(dest.Hash), convDir)
	restarted.SetTransport(ts)
	if err := restarted.ScanStorage(); err != nil {
		t.Fatal(err)
	}
	if got := restarted.DisplayMessages()[0].State; got != lxmf.StateDelivered {
		t.Errorf("state = %#x, want %#x (DELIVERED): the pre-rewrite .index snapshot was restored for a message whose file is newer",
			got, lxmf.StateDelivered)
	}
}
