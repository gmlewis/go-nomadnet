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
	rnsmsgpack "github.com/gmlewis/go-reticulum/rns/msgpack"
)

// TestScanStorageRestoreFromIndexUnconditional pins Python scan_storage
// parity (Conversation.py:2240-2246): a newly-discovered message file
// restores ALL cached fields from its .index entry UNCONDITIONALLY — Python
// performs no sort_timestamp/mtime comparison. A previous Go-only
// mtime-equality guard skipped the restore when the index snapshot's
// sort_timestamp differed from the file mtime, so gonomadnet re-parsed the
// envelope and re-verified signatures where nomadnet renders the indexed
// cached state — a visible per-row divergence (fresh "✓ ←" vs indexed
// "← Unknown Origin") on the glenn-macm2pro ⇄ glenn-mac-mini-m2 conversation.
func TestScanStorageRestoreFromIndexUnconditional(t *testing.T) {
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

	dir := tempDir(t)
	convDir := filepath.Join(dir, "peer")
	if err := os.MkdirAll(convDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// A real envelope on disk whose container state is SENDING.
	m, err := lxmf.NewMessage(dest, dest, "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Pack(); err != nil {
		t.Fatal(err)
	}
	m.SetState(lxmf.StateSending)
	if _, err := m.WriteToDirectory(convDir); err != nil {
		t.Fatal(err)
	}

	convHex := hex.EncodeToString(dest.Hash)
	conv := NewConversation(convHex, convDir)
	conv.SetTransport(ts)
	if err := conv.ScanStorage(); err != nil {
		t.Fatal(err)
	}
	if len(conv.Messages) != 1 {
		t.Fatalf("message count = %v, want 1", len(conv.Messages))
	}
	conv.Messages[0].Load()
	// Snapshot the entry with a title that can ONLY come from the index. The
	// WriteIndex-written sort_timestamp equals the file mtime, so rewrite the
	// index with a deliberately mismatching sort_timestamp — the exact case
	// the mtime guard silently skipped.
	saved := *conv.Messages[0]
	saved.CachedTitle = "restored-from-index-title"
	if err := WriteIndex(convDir, []*Message{&saved}); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Base(saved.FilePath)
	index := ReadIndex(convDir)
	entry, ok := index.Get(filename)
	if !ok {
		t.Fatalf("index entry %q missing", filename)
	}
	om, ok := entry.(rnsmsgpack.OrderedMap)
	if !ok {
		t.Fatalf("index entry type %T, want OrderedMap", entry)
	}
	index = index.Set(filename, om)
	data, err := rnsmsgpack.Pack(index)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(convDir, ".index"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Rescan in a fresh conversation: the index title must win (restored),
	// exactly like nomadnet restores from the index.
	restored := NewConversation(convHex, convDir)
	restored.SetTransport(ts)
	if err := restored.ScanStorage(); err != nil {
		t.Fatal(err)
	}
	titles := restored.DisplayMessages()
	if len(titles) != 1 {
		t.Fatalf("restored message count = %v, want 1", len(titles))
	}
	if titles[0].Title != "restored-from-index-title" {
		t.Errorf("restored title = %q, want %q — the index restore was skipped (mtime guard is not python-parity)",
			titles[0].Title, "restored-from-index-title")
	}
}
