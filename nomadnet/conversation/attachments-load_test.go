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
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/gmlewis/go-reticulum/lxmf"
	"github.com/gmlewis/go-reticulum/rns"
	rnsmsgpack "github.com/gmlewis/go-reticulum/rns/msgpack"
)

// attachmentFixture builds a real, packed LXMF message carrying one file
// attachment and writes it to convDir. It returns the on-disk path, the
// attachment payload, and the original file bytes.
func attachmentFixture(t *testing.T, convDir, name string, payload []byte) (path string, original []byte) {
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
	fields := map[any]any{
		lxmf.FieldFileAttachments: []any{[]any{name, payload}},
	}
	msg, err := lxmf.NewMessage(dest, dest, "World body", "Hello", fields)
	if err != nil {
		t.Fatal(err)
	}
	if err := msg.Pack(); err != nil {
		t.Fatal(err)
	}
	msg.SetState(lxmf.StateSent)
	msg.SetMethod(lxmf.MethodDirect)
	path, err = msg.WriteToDirectory(convDir)
	if err != nil {
		t.Fatal(err)
	}
	original, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return path, original
}

// TestMessageLoadPersistsAttachmentsAcrossReload pins the attachment lifecycle
// Python's ConversationMessage load path implements (Conversation.py:495-552):
//
//  1. attachment names are cached from the envelope's file/image/audio fields;
//  2. extract_attachments_from_lxm writes the payloads plus a manifest into the
//     per-message attachment directory (Conversation.py:549-550);
//  3. strip_attachments_from_file rewrites the on-disk envelope WITHOUT the
//     attachment fields (Conversation.py:551), so the payload is stored once,
//     in the attachment directory, instead of twice;
//  4. a later load — by which time the fields are gone — rebuilds the names
//     from the manifest (Conversation.py:527-548), so the attachment rows the
//     user sees do not disappear on reload.
//
// This test drives the real load path twice and asserts the payload and names
// survive both times.
func TestMessageLoadPersistsAttachmentsAcrossReload(t *testing.T) {
	t.Parallel()

	payload := []byte("attachment-payload-0123456789")
	base := tempDir(t)
	convDir := filepath.Join(base, "src")
	if err := os.MkdirAll(convDir, 0o755); err != nil {
		t.Fatal(err)
	}
	attachmentPath := filepath.Join(base, "attachments")

	path, original := attachmentFixture(t, convDir, "notes.txt", payload)
	ts := rns.NewTransportSystem(nil)

	// First load: extract the attachment, then strip it from the envelope.
	first := NewMessageWithTransport(path, ts)
	first.AttachmentPath = attachmentPath
	first.Load()

	if !first.Loaded {
		t.Fatal("first load failed to parse the envelope")
	}
	wantNames := []AttachmentInfo{{Type: "file", Name: "notes.txt", Size: len(payload)}}
	assertAttachmentNames(t, "first load", first.CachedAttachmentNames, wantNames)

	// The payload must now live in the attachment directory, byte-faithful.
	attDir := first.attachmentDir()
	if attDir == "" {
		t.Fatal("attachmentDir is empty: the message hash was not derived")
	}
	stored, err := os.ReadFile(filepath.Join(attDir, "file_0"))
	if err != nil {
		t.Fatalf("reading extracted attachment: %v", err)
	}
	if !bytes.Equal(stored, payload) {
		t.Errorf("extracted attachment = %q, want %q", stored, payload)
	}

	// The envelope must have been rewritten without the attachment fields.
	stripped, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(stripped, original) {
		t.Error("load did not strip the attachment payload from the on-disk envelope")
	}

	// Second load: the fields are gone from the envelope, so the names can only
	// come from the manifest the first load wrote.
	second := NewMessageWithTransport(path, ts)
	second.AttachmentPath = attachmentPath
	second.Load()

	if !second.Loaded {
		t.Fatal("second load failed to parse the stripped envelope")
	}
	if _, ok := fieldLookup(second.GetFields(), lxmf.FieldFileAttachments); ok {
		t.Error("attachment field still present in the stripped envelope")
	}
	assertAttachmentNames(t, "reload from manifest", second.CachedAttachmentNames, wantNames)
	if !second.HasAttachments() {
		t.Error("HasAttachments = false after reload, want true")
	}

	// The attachment must still be reachable and byte-faithful after the reload
	// — this is what the per-attachment save row needs.
	gotPath := second.GetAttachmentFilePath("file", 0)
	if gotPath == "" {
		t.Fatal("GetAttachmentFilePath returned no path after reload")
	}
	saved, err := os.ReadFile(gotPath)
	if err != nil {
		t.Fatalf("reading attachment after reload: %v", err)
	}
	if !bytes.Equal(saved, payload) {
		t.Errorf("attachment after reload = %q, want %q", saved, payload)
	}
}

// TestMessageLoadKeepsFilenameHashAfterStrip pins the hash rule Python's load
// uses (Conversation.py:466-467): the hash seeded from the 64-hex file name is
// authoritative (`if self._cached_hash is None:`) and is NOT replaced by the
// hash recomputed from the envelope contents. The two differ for a stripped
// message — stripping removes the attachment fields, so the content hash
// changes while the file keeps its name — and the per-message attachment
// directory is keyed by the ORIGINAL hash. Overwriting it would send every
// post-strip attachment lookup to a directory that does not exist.
func TestMessageLoadKeepsFilenameHashAfterStrip(t *testing.T) {
	t.Parallel()

	payload := []byte("payload")
	base := tempDir(t)
	convDir := filepath.Join(base, "src")
	if err := os.MkdirAll(convDir, 0o755); err != nil {
		t.Fatal(err)
	}

	path, _ := attachmentFixture(t, convDir, "notes.txt", payload)
	ts := rns.NewTransportSystem(nil)

	first := NewMessageWithTransport(path, ts)
	first.AttachmentPath = filepath.Join(base, "attachments")
	first.Load()

	// Strip so the envelope content no longer hashes to the file name.
	fileHash := first.CachedHash
	afterStrip := NewMessageWithTransport(path, ts)
	afterStrip.AttachmentPath = first.AttachmentPath
	afterStrip.Load()

	if !bytes.Equal(afterStrip.GetHash(), fileHash) {
		t.Errorf("hash after reload = %x, want the file-name hash %x", afterStrip.GetHash(), fileHash)
	}
}

func assertAttachmentNames(t *testing.T, ctx string, got, want []AttachmentInfo) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%v: attachment names = %v, want %v", ctx, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%v: attachment[%v] = %v, want %v", ctx, i, got[i], want[i])
		}
	}
}

// TestMessageIndexAttachmentNamesRoundTrip pins the index half of the
// attachment cache: Python's to_index_entry writes both "has_attachments" and
// "attachment_names" and restore_from_index reads both back (Conversation.py:
// 483,485,472,473). The name list is what lets the conversation list render its
// attachment rows from the .index alone, without opening every envelope.
func TestMessageIndexAttachmentNamesRoundTrip(t *testing.T) {
	t.Parallel()

	want := []AttachmentInfo{{Type: "file", Name: "notes.txt", Size: 16}}
	src := &Message{
		CachedHasAttachments:  true,
		CachedAttachmentNames: want,
	}
	entry := src.ToIndexEntry()

	got := &Message{}
	got.RestoreFromIndex(entry)

	if !got.HasAttachments() {
		t.Error("HasAttachments = false after restore, want true")
	}
	assertAttachmentNames(t, "index round trip", got.CachedAttachmentNames, want)
}

// TestMessageIndexAttachmentFlagAuthoritativeOverEnvelope pins Python's
// `if self._cached_has_attachments is None:` guard around the load-time
// attachment scan (Conversation.py:495): once the .index has supplied the flag,
// reading the envelope must not recompute it. A message restored from an index
// that says it has attachments keeps saying so even when the envelope it is
// read from carries no attachment fields of its own.
func TestMessageIndexAttachmentFlagAuthoritativeOverEnvelope(t *testing.T) {
	t.Parallel()

	base := tempDir(t)
	convDir := filepath.Join(base, "src")
	if err := os.MkdirAll(convDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A message with no attachments at all: nothing on disk can justify the
	// restored TRUE, so only the guard keeps it.
	path, _ := attachmentFixture(t, convDir, "notes.txt", []byte("x"))
	ts := rns.NewTransportSystem(nil)
	stripFileAttachments(t, path)

	want := []AttachmentInfo{{Type: "file", Name: "from-the-index", Size: 7}}
	m := NewMessageWithTransport(path, ts)
	m.AttachmentPath = filepath.Join(base, "attachments")
	m.RestoreFromIndex(rnsmsgpack.OrderedMap{
		{Key: "has_attachments", Value: true},
		{Key: "attachment_names", Value: []any{[]any{"file", "from-the-index", uint(7)}}},
	})
	m.Load()

	if !m.HasAttachments() {
		t.Error("HasAttachments = false after load, want the index value true")
	}
	assertAttachmentNames(t, "index over envelope", m.CachedAttachmentNames, want)
}

// stripFileAttachments removes the attachment fields from a packed message file
// by driving strip_attachments_from_file's precondition: an extraction is
// performed first so the attachment directory exists.
func stripFileAttachments(t *testing.T, path string) {
	t.Helper()
	base := tempDir(t)
	attachmentPath := filepath.Join(base, "attachments")
	ts := rns.NewTransportSystem(nil)
	m := NewMessageWithTransport(path, ts)
	m.AttachmentPath = attachmentPath
	m.Load()
	if err := StripAttachmentsFromFile(path, attachmentPath); err != nil {
		t.Fatalf("StripAttachmentsFromFile: %v", err)
	}
}
