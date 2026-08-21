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
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/gmlewis/go-nomadnet/testutils"
	"github.com/gmlewis/go-reticulum/lxmf"
)

// TestExtractAttachmentsManifestByteParity pins the hard parity requirement for
// the attachment manifest migration: ExtractAttachmentsFromLXM MUST emit a
// byte-for-byte identical manifest to Python NomadNet's
// extract_attachments_from_lxm (Conversation.py:753), which writes
// msgpack.packb({"files": [...]}).
//
// This is a LIVE cross-implementation test: it execs the real Python nomadnet
// reference, rebuilds the SAME manifest the Go test's attachments would
// produce — calling the real ConversationMessage.safe_attachment_name on each
// attachment name (the exact function extract_attachments_from_lxm uses), then
// msgpack.packb'ing {"files": [...]} FRESH via RNS.vendor.umsgpack — and diffs
// the bytes against Go's manifest output. It is skipped (not failed) when the
// Python nomadnet reference is not importable.
//
// The scenario exercises:
//   - a top-level fixmap(0x81) with a single "files" key -> fixarray(0x92) of two
//     entries.
//   - each entry a fixmap(0x83) with keys "name","stored_name","size" in Python's
//     insertion order (the rns/msgpack migration uses OrderedMap to reproduce it;
//     a Go map would randomize the key order).
//   - size as an unsigned encoding: 200 -> uint8 (0xcc 0xc8), 300 -> uint16
//     (0xcd 0x01 0x2c). A Go int would pack 200/300 as signed int16 (0xd1 ...),
//     diverging from Python's unsigned encoding, so size is widened to uint.
func TestExtractAttachmentsManifestByteParity(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Two file attachments: "a.txt" (200 bytes) and "b.txt" (300 bytes).
	attA := []any{"a.txt", bytes.Repeat([]byte{0x41}, 200)}
	attB := []any{"b.txt", bytes.Repeat([]byte{0x42}, 300)}
	msg := &lxmf.Message{
		Hash: []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00, 0x11},
		Fields: map[any]any{
			lxmf.FieldFileAttachments: []any{attA, attB},
		},
	}

	// Forward the file attachments (name + size) to the Python parity script,
	// mirroring the inputs Go's ExtractAttachmentsFromLXM consumes.
	pyAttachments := fileAttachmentsToPy(msg.Fields)

	// manifestParityScript rebuilds extract_attachments_from_lxm's manifest dict
	// using the real ConversationMessage.safe_attachment_name (the exact function
	// the Python reference uses to sanitise attachment names) and packb's it with
	// RNS.vendor.umsgpack, the exact packer extract_attachments_from_lxm uses.
	const manifestParityScript = `
import sys, json, base64
import RNS.vendor.umsgpack as msgpack
from nomadnet.Conversation import ConversationMessage
req = json.loads(sys.stdin.read() or "{}")
manifest = {"files": []}
for idx, a in enumerate(req["attachments"]):
    name = ConversationMessage.safe_attachment_name(a["name"], fallback="attachment_"+str(idx))
    stored = "file_" + str(idx)
    manifest["files"].append({"name": name, "stored_name": stored, "size": a["size"]})
print(json.dumps(base64.b64encode(msgpack.packb(manifest)).decode()))
`
	var pyB64 string
	testutils.RunPythonNomadnet(t, map[string]any{"attachments": pyAttachments}, manifestParityScript, &pyB64)

	want, err := base64.StdEncoding.DecodeString(pyB64)
	if err != nil {
		t.Fatalf("decode python bytes: %v", err)
	}

	if err := ExtractAttachmentsFromLXM(msg, dir); err != nil {
		t.Fatalf("ExtractAttachmentsFromLXM: %v", err)
	}

	attDir := filepath.Join(dir, "aabbccddeeff0011")
	manifestPath := filepath.Join(attDir, "manifest")
	got, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("manifest bytes diverge from fresh Python:\n got %x\n want %x", got, want)
	}
}

// fileAttachmentsToPy extracts the FIELD_FILE_ATTACHMENTS list from a parsed
// LXMF fields map and returns the name + size pair the Python manifest parity
// script consumes, mirroring the inputs extract_attachments_from_lxm reads.
func fileAttachmentsToPy(fields map[any]any) []map[string]any {
	var out []map[string]any
	fileAtts, _ := fieldLookup(fields, lxmf.FieldFileAttachments)
	atts, _ := fileAtts.([]any)
	for _, att := range atts {
		entry, ok := att.([]any)
		if !ok || len(entry) < 2 {
			continue
		}
		name, _ := entry[0].(string)
		data, _ := entry[1].([]byte)
		out = append(out, map[string]any{"name": name, "size": len(data)})
	}
	return out
}
