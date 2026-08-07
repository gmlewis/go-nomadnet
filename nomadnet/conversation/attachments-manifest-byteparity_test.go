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

	"github.com/gmlewis/go-reticulum/lxmf"
)

//go:embed testdata/py-attachment-manifest.msgpack
var pyAttachmentManifestGolden []byte

// TestExtractAttachmentsManifestByteParity pins the hard parity requirement for
// the attachment manifest migration: ExtractAttachmentsFromLXM MUST emit a
// byte-for-byte identical manifest to Python NomadNet's
// extract_attachments_from_lxm, which writes msgpack.packb({"files": [...]}).
//
// The golden (testdata/py-attachment-manifest.msgpack) was produced by
// msgpack.packb of {"files": [{"name":"a.txt","stored_name":"file_0","size":200},
// {"name":"b.txt","stored_name":"file_1","size":300}]} with default options.
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
	msg := &lxmf.Message{
		Hash: []byte{0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00, 0x11},
		Fields: map[any]any{
			lxmf.FieldFileAttachments: []any{
				[]any{"a.txt", bytes.Repeat([]byte{0x41}, 200)},
				[]any{"b.txt", bytes.Repeat([]byte{0x42}, 300)},
			},
		},
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
	if !bytes.Equal(got, pyAttachmentManifestGolden) {
		t.Fatalf("manifest bytes diverge from Python golden:\n got %x\n want %x", got, pyAttachmentManifestGolden)
	}
}
