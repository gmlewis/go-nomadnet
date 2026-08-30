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
	"strings"
	"testing"

	rnsmsgpack "github.com/gmlewis/go-reticulum/rns/msgpack"
)

// TestRestoredEmptyFieldsDoNotReload verifies the Python lazy-getter
// presence semantics on index-restored messages: Python's getters check
// `_cached_X is not None`, so a message restored from the .index with an
// EMPTY title, EMPTY content, FALSE transport_encrypted or FALSE
// has_attachments is fully populated and NEVER reloads from disk. The Go
// port previously treated an empty string / false bool as "missing", so
// GetTitle (called by DisplayMessages for every message) triggered a
// full envelope reload that clobbered the index-restored raw state
// (e.g. OUTBOUND 0x01 → FAILED 0xFF via the pending-outbound check) and
// signature validation (true → false, "Unknown Origin"), producing
// completely different LXMessageWidget headers from nomadnet.
func TestRestoredEmptyFieldsDoNotReload(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	name := strings.Repeat("ab", 32) // 64-hex-char message filename
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("not-a-real-envelope"), 0o644); err != nil {
		t.Fatal(err)
	}

	entry := rnsmsgpack.OrderedMap{
		{Key: "state", Value: uint(0x01)},  // raw LXMF OUTBOUND (Python header default "→")
		{Key: "method", Value: uint(0x02)}, // DIRECT
		{Key: "source_hash", Value: []byte{0x2a, 0x61, 0x05, 0xf5}},
		{Key: "signature_validated", Value: true},
		{Key: "transport_encrypted", Value: false}, // absent-value trap: must count as PRESENT
		{Key: "transport_encryption", Value: ""},
		{Key: "title", Value: ""}, // EMPTY title restored from index
		{Key: "content", Value: "Hello from the index"},
		{Key: "has_attachments", Value: false},
	}

	msg := NewMessage(path)
	msg.RestoreFromIndex(entry)

	// Every getter DisplayMessages touches must return the index-restored
	// values WITHOUT triggering a disk load.
	if got := msg.GetTitle(); got != "" {
		t.Errorf("GetTitle = %q, want restored empty title", got)
	}
	if got := msg.GetContent(); got != "Hello from the index" {
		t.Errorf("GetContent = %q, want index-restored content", got)
	}
	if msg.GetTransportEncrypted() {
		t.Error("GetTransportEncrypted = true, want restored false (index value is present even when false)")
	}
	if got := msg.GetTransportEncryption(); got != "" {
		t.Errorf("GetTransportEncryption = %q, want restored empty string", got)
	}
	if msg.HasAttachments() {
		t.Error("HasAttachments = true, want restored false (index value is present even when false)")
	}
	if got := msg.GetHash(); got == nil || len(got) != 32 {
		t.Errorf("GetHash = %v, want 32 bytes derived from the hex filename (Python __init__ bytes.fromhex)", got)
	}
	if got := msg.CachedRawState; got != 0x01 {
		t.Errorf("CachedRawState = %#x, want %#x (index-restored OUTBOUND)", got, 0x01)
	}
	if !msg.SignatureValidated() {
		t.Error("SignatureValidated = false, want index-restored true")
	}
	if got := msg.GetSignatureDescription(); got != "Signature Verified" {
		t.Errorf("GetSignatureDescription = %q, want Signature Verified", got)
	}
	if msg.Loaded {
		t.Error("getters loaded the message from disk; restored empty fields must not trigger a reload")
	}
}

// TestNewMessageDerivesHashFromFilename pins Python __init__'s behavior that
// the cached hash comes from the hex filename (bytes.fromhex(filename)), so
// get_hash never needs to load from disk.
func TestNewMessageDerivesHashFromFilename(t *testing.T) {
	t.Parallel()

	dir := tempDir(t)
	name := strings.Repeat("cd", 32)
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	msg := NewMessage(path)
	if msg.CachedHash == nil || len(msg.CachedHash) != 32 {
		t.Fatalf("NewMessage CachedHash = %v, want 32 bytes from the filename", msg.CachedHash)
	}
	if got := msg.GetHash(); len(got) != 32 || msg.CachedHash[0] != msg.CachedHash[31] || msg.CachedHash[0] != 0xcd {
		t.Errorf("GetHash = %x, want filename bytes", msg.CachedHash)
	}
	if msg.Loaded {
		t.Error("GetHash must not load from disk when the filename supplies the hash")
	}
}
