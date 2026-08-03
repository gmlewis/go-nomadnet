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

	"github.com/vmihailenco/msgpack/v5"
)

// TestCopyAttachmentToDest mirrors Python's _copy_attachment_to_dest +
// _resolve_attachment_save_path (Conversations.py:2871-2894): the saved file
// lands under saveDir with a sanitized name, content is copied byte-faithful,
// and a name collision appends an "_N" counter before the extension.
func TestCopyAttachmentToDest(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "report.txt")
	body := []byte("attachment body bytes")
	if err := os.WriteFile(srcPath, body, 0o644); err != nil {
		t.Fatal(err)
	}

	saveDir := t.TempDir()
	// Remove saveDir so the helper must create it (mirrors os.makedirs).
	_ = os.RemoveAll(saveDir)

	got, err := CopyAttachmentToDest("report.txt", srcPath, saveDir)
	if err != nil {
		t.Fatalf("CopyAttachmentToDest: %v", err)
	}
	if want := filepath.Join(saveDir, "report.txt"); got != want {
		t.Errorf("path = %q, want %q", got, want)
	}
	data, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(body) {
		t.Errorf("copied content = %q, want %q", data, body)
	}
}

// TestCopyAttachmentToDestCollision verifies the unique-name counter: a second
// copy of the same filename into an occupied saveDir yields "report_1.txt".
func TestCopyAttachmentToDestCollision(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "report.txt")
	if err := os.WriteFile(srcPath, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	saveDir := t.TempDir()

	first, err := CopyAttachmentToDest("report.txt", srcPath, saveDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(srcPath, []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := CopyAttachmentToDest("report.txt", srcPath, saveDir)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("collision not resolved: both = %q", first)
	}
	if want := filepath.Join(saveDir, "report_1.txt"); second != want {
		t.Errorf("second path = %q, want %q", second, want)
	}
}

// TestCopyAttachmentToDestSanitizesName verifies a path-traversal filename is
// sanitized so the destination stays under saveDir (Python safe_attachment_name
// strips separators; _resolve_attachment_save_path guards with realpath).
func TestCopyAttachmentToDestSanitizesName(t *testing.T) {
	t.Parallel()

	srcDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "x.bin")
	if err := os.WriteFile(srcPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	saveDir := t.TempDir()

	got, err := CopyAttachmentToDest("../../etc/passwd", srcPath, saveDir)
	if err != nil {
		t.Fatalf("CopyAttachmentToDest: %v", err)
	}
	// The destination directory must be saveDir itself (no escape).
	if dir, _ := filepath.Split(got); filepath.Clean(dir) != filepath.Clean(saveDir+string(filepath.Separator)) {
		t.Errorf("destination %q escapes saveDir %q", got, saveDir)
	}
	// SafeAttachmentName strips separators from "../../etc/passwd" → "passwd".
	if filepath.Base(got) != "passwd" {
		t.Errorf("sanitized base = %q, want passwd", filepath.Base(got))
	}
}

// TestSaveAttachmentsToDir verifies Conversation.SaveAttachmentsToDir locates
// each selection's extracted attachment file (via the message's attachment
// directory + field index) and copies it to destDir, mirroring Python's
// do_save (Conversations.py:2368-2391). Unknown message hashes or missing
// files count as failures rather than aborting the whole batch.
func TestSaveAttachmentsToDir(t *testing.T) {
	t.Parallel()

	attBase := t.TempDir()
	msgHash := []byte{0xaa, 0xbb, 0xcc, 0xdd}
	hashHex := "aabbccdd"
	attDir := filepath.Join(attBase, hashHex)
	if err := os.MkdirAll(attDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte("extracted file body")
	if err := os.WriteFile(filepath.Join(attDir, "file_0"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	// Minimal manifest so GetAttachmentFilePath resolves field index 0.
	manifest := map[string]any{"files": []any{
		map[string]any{"name": "report.txt", "stored_name": "file_0", "size": len(body)},
	}}
	manifestData, err := msgpack.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(attDir, "manifest"), manifestData, 0o644); err != nil {
		t.Fatal(err)
	}

	conv := &Conversation{
		attachmentPath: attBase,
	}
	m := &Message{
		FilePath:       filepath.Join(attDir, "unused.msg"),
		AttachmentPath: attBase,
		CachedHash:     msgHash,
	}
	conv.Messages = []*Message{m}

	destDir := t.TempDir()
	saved, failed := conv.SaveAttachmentsToDir([]SaveAttachmentSelection{
		{MessageHash: msgHash, FieldType: "file", FieldIndex: 0, Name: "report.txt"},
		{MessageHash: []byte{0xff}, FieldType: "file", FieldIndex: 0, Name: "missing.txt"},
	}, destDir)

	if failed != 1 {
		t.Errorf("failed = %v, want 1", failed)
	}
	if len(saved) != 1 {
		t.Fatalf("saved = %v, want 1 path", saved)
	}
	got, err := os.ReadFile(saved[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(body) {
		t.Errorf("copied content = %q, want %q", got, body)
	}
}
