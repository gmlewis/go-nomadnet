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
	_ "embed"
	"os"
	"path/filepath"
	"testing"
)

//go:embed testdata/py-index-byteparity.msgpack
var pyIndexByteParityGolden []byte

// TestWriteIndexPythonByteParity pins the hard parity requirement for the
// conversation .index msgpack migration: WriteIndex MUST emit byte-for-byte
// identical msgpack to Python NomadNet's ConversationMessage.write_index for
// the same messages.
//
// The golden (testdata/py-index-byteparity.msgpack) was produced by
// msgpack.packb of an insertion-ordered dict {filename: to_index_entry()} for
// two messages (both with _cached_state set, since write_index skips
// state=None messages). entry1 is fully populated; entry2 uses Python
// zero-values (false/""/0) that map exactly to the Go Message zero-value
// representation, so the bytes match Go's output. This exercises:
//   - outer fixmap(0x82) with two 64-char filename keys -> str8(0xd9 0x40) in
//     the messages-slice insertion order (the rns/msgpack migration uses
//     OrderedMap.Set so existing keys keep their place and new keys append,
//     matching Python dict semantics; a Go map would randomize the order).
//   - inner fixmap(0x8e) with the 14 to_index_entry keys in Python's
//     declaration order (via OrderedMap).
//   - state/method as unsigned fixint (0x03/0x01/0x00), matching Python's
//     unsigned encoding; a Go int would diverge for values > 0x7f.
//   - source_hash as bin8(0xc4), timestamps as float64(0xcb), booleans as
//     0xc3/0xc2, nil as 0xc0, empty string as fixstr(0xa0).
//   - attachment_names as fixarray(0x91) of fixarray3(0x93) tuples
//     ["file","doc.pdf",1024], with size 1024 -> uint16(0xcd 0x04 0x00); a Go
//     int would pack 1024 as signed int16(0xd1 0x04 0x00), diverging, so size
//     is widened to uint.
//
// Note: Python distinguishes None from zero for title/content/method/
// transport_encrypted/transport_encryption. The Go Message struct uses
// non-nilable zero values there, so a Python entry storing None for those
// fields is NOT byte-reproducible by Go. That is a pre-existing data-model
// gap (present under vmihailenco too), not a codec regression, and is out of
// scope for the rns/msgpack migration; this test deliberately uses Python
// zero-values for entry2 so the codec layer is byte-identical.
func TestWriteIndexPythonByteParity(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	convPath := filepath.Join(dir, "conv")
	if err := os.MkdirAll(convPath, 0o755); err != nil {
		t.Fatal(err)
	}

	const nameA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const nameB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if err := os.WriteFile(filepath.Join(convPath, nameA), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(convPath, nameB), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// entry1: fully populated.
	ts := 1234567890.0
	stateSent := StateSent
	sigValid := true
	msg1 := NewMessage(filepath.Join(convPath, nameA))
	msg1.Timestamp = &ts
	msg1.SortTimestamp = 1234567890.0
	msg1.CachedState = &stateSent
	msg1.CachedTitle = "Test Title"
	msg1.CachedContent = "Hello World"
	msg1.CachedSourceHash = []byte{0x01, 0x02}
	msg1.CachedTransportEncrypted = true
	msg1.CachedTransportEncryption = "AES-256"
	msg1.CachedSignatureValidated = &sigValid
	msg1.CachedUnverifiedReason = nil
	msg1.CachedMethod = 1
	msg1.Renderer = nil
	msg1.CachedHasAttachments = true
	msg1.CachedAttachmentNames = []AttachmentInfo{{Type: "file", Name: "doc.pdf", Size: 1024}}

	// entry2: Go zero-value representation (matches Python zero-values).
	stateDraft := StateDraft
	msg2 := NewMessage(filepath.Join(convPath, nameB))
	msg2.SortTimestamp = 9999999999.0
	msg2.CachedState = &stateDraft

	if err := WriteIndex(convPath, []*Message{msg1, msg2}); err != nil {
		t.Fatalf("WriteIndex: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(convPath, ".index"))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if !bytes.Equal(got, pyIndexByteParityGolden) {
		t.Fatalf(".index bytes diverge from Python golden:\n got %x\n want %x", got, pyIndexByteParityGolden)
	}
}
