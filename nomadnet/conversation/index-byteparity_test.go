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
// along with this program. If not, see <https://www.gnu.org/licenses>.

package conversation

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/gmlewis/go-nomadnet/testutils"
)

// TestWriteIndexPythonByteParity pins the hard parity requirement for the
// conversation .index msgpack migration: WriteIndex MUST emit byte-for-byte
// identical msgpack to Python NomadNet's ConversationMessage.write_index
// (Conversation.py:990) for the same messages.
//
// This is a LIVE cross-implementation test: it execs the real Python nomadnet
// reference, rebuilds the SAME index dict (same filename keys in the same
// messages-slice insertion order, same per-entry to_index_entry dict in
// Python's declaration order, same values) the Go test constructs,
// msgpack.packb's it FRESH via RNS.vendor.umsgpack, and diffs the bytes against
// Go's WriteIndex output. It is skipped (not failed) when the Python nomadnet
// reference is not importable.
//
// The scenario exercises:
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

	messages := []*Message{msg1, msg2}

	// Build the Python input mirroring the exact message state above so the
	// script can rebuild to_index_entry dicts and packb them.
	pyMsgs := make([]map[string]any, 0, len(messages))
	for _, m := range messages {
		pyMsgs = append(pyMsgs, messageToIndexPy(m))
	}

	// indexParityScript rebuilds write_index's index dict in Python's
	// to_index_entry declaration order and packb's it with RNS.vendor.umsgpack,
	// the exact packer ConversationMessage.write_index uses. sort_timestamp is
	// forced to float so a JSON-int round-trip still packs as float64.
	const indexParityScript = `
import sys, json, base64
import RNS.vendor.umsgpack as msgpack
req = json.loads(sys.stdin.read() or "{}")
def hb(x):
    return bytes.fromhex(x) if x is not None else None
index = {}
for m in req["messages"]:
    atts = []
    for a in m.get("attachment_names", []):
        atts.append((a[0], a[1], a[2]))
    entry = {
        "timestamp": float(m["timestamp"]) if m["timestamp"] is not None else None,
        "sort_timestamp": float(m["sort_timestamp"]),
        "state": m["state"],
        "title": m["title"],
        "content": m["content"],
        "source_hash": hb(m["source_hash"]),
        "transport_encrypted": m["transport_encrypted"],
        "transport_encryption": m["transport_encryption"],
        "signature_validated": m["signature_validated"],
        "unverified_reason": m["unverified_reason"],
        "method": m["method"],
        "renderer": m["renderer"],
        "has_attachments": m["has_attachments"],
        "attachment_names": atts,
    }
    index[m["filename"]] = entry
print(json.dumps(base64.b64encode(msgpack.packb(index)).decode()))
`
	var pyB64 string
	testutils.RunPythonNomadnet(t, map[string]any{"messages": pyMsgs}, indexParityScript, &pyB64)

	want, err := base64.StdEncoding.DecodeString(pyB64)
	if err != nil {
		t.Fatalf("decode python bytes: %v", err)
	}

	if err := WriteIndex(convPath, messages); err != nil {
		t.Fatalf("WriteIndex: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(convPath, ".index"))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf(".index bytes diverge from fresh Python:\n got %x\n want %x", got, want)
	}
}

// messageToIndexPy maps a Go Message to the JSON-friendly dict the Python index
// parity script consumes, mirroring the fields ToIndexEntry serialises. Bytes
// fields are hex-encoded; pointer fields forward their value or nil.
func messageToIndexPy(m *Message) map[string]any {
	var timestamp any
	if m.Timestamp != nil {
		timestamp = *m.Timestamp
	}
	var state any
	if m.CachedState != nil {
		state = uint(*m.CachedState)
	}
	var sigValid any
	if m.CachedSignatureValidated != nil {
		sigValid = *m.CachedSignatureValidated
	}
	var sourceHash any
	if m.CachedSourceHash != nil {
		sourceHash = hex.EncodeToString(m.CachedSourceHash)
	}
	atts := make([][]any, 0, len(m.CachedAttachmentNames))
	for _, a := range m.CachedAttachmentNames {
		atts = append(atts, []any{a.Type, a.Name, uint(a.Size)})
	}
	return map[string]any{
		"filename":             filepath.Base(m.FilePath),
		"timestamp":            timestamp,
		"sort_timestamp":       m.SortTimestamp,
		"state":                state,
		"title":                m.CachedTitle,
		"content":              m.CachedContent,
		"source_hash":          sourceHash,
		"transport_encrypted":  m.CachedTransportEncrypted,
		"transport_encryption": m.CachedTransportEncryption,
		"signature_validated":  sigValid,
		"unverified_reason":    m.CachedUnverifiedReason,
		"method":               uint(m.CachedMethod),
		"renderer":             nil,
		"has_attachments":      m.CachedHasAttachments,
		"attachment_names":     atts,
	}
}
