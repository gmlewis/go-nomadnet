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
)

func buildLXMWithAttachments(t *testing.T, dir, attachmentPath string) (string, []byte, []byte) {
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
	png := append([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, bytes.Repeat([]byte{0}, 20)...)
	fileData := []byte("hello file bytes")
	msg, err := lxmf.NewMessage(dest, dest, "body", "title", nil)
	if err != nil {
		t.Fatal(err)
	}
	msg.Fields = map[any]any{
		lxmf.FieldFileAttachments: []any{[]any{"notes.txt", fileData}},
		lxmf.FieldImage:           png,
	}
	if err := msg.Pack(); err != nil {
		t.Fatal(err)
	}
	msg.State = lxmf.StateDelivered

	path, err := msg.WriteToDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := ExtractAttachmentsFromLXM(msg, attachmentPath); err != nil {
		t.Fatal(err)
	}
	return path, png, fileData
}

func TestExtractAndGetAttachments(t *testing.T) {
	t.Parallel()
	base := tempDir(t)
	attachmentPath := filepath.Join(base, "attachments")
	if err := os.MkdirAll(attachmentPath, 0o755); err != nil {
		t.Fatal(err)
	}
	msgDir := filepath.Join(base, "msg")
	if err := os.MkdirAll(msgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path, png, _ := buildLXMWithAttachments(t, msgDir, attachmentPath)

	ts := rns.NewTransportSystem(nil)
	m := NewMessageWithTransport(path, ts)
	m.AttachmentPath = attachmentPath
	m.Load()

	if !m.HasAttachments() {
		t.Fatal("message should report attachments")
	}

	gotImage := m.GetImage()
	if !bytes.Equal(gotImage, png) {
		t.Errorf("GetImage = %x, want %x", gotImage, png)
	}

	// attachment dir should exist and contain a manifest
	attDir := m.attachmentDir()
	if attDir == "" {
		t.Fatal("attachment dir should not be empty")
	}
	if _, err := os.Stat(filepath.Join(attDir, "manifest")); os.IsNotExist(err) {
		t.Fatal("manifest file should exist")
	}

	// File attachment path lookup: index 0 -> file_0
	fp := m.GetAttachmentFilePath("file", 0)
	if fp == "" {
		t.Fatal("expected a file path for file index 0")
	}
	data, err := os.ReadFile(fp)
	if err != nil {
		t.Fatalf("could not read attachment file %q: %v", fp, err)
	}
	if !bytes.Equal(data, []byte("hello file bytes")) {
		t.Errorf("file attachment data = %q, want %q", data, "hello file bytes")
	}

	// The image is stored at manifest index 1 (after the file attachment).
	imgPath := m.GetAttachmentFilePath("file", 1)
	if imgPath == "" {
		t.Fatal("expected image path at index 1")
	}
	imgData, err := os.ReadFile(imgPath)
	if err != nil {
		t.Fatalf("could not read image file %q: %v", imgPath, err)
	}
	if !bytes.Equal(imgData, png) {
		t.Errorf("image file data mismatch")
	}
}

func TestExtractAttachmentsIdempotent(t *testing.T) {
	t.Parallel()
	base := tempDir(t)
	attachmentPath := filepath.Join(base, "attachments")
	if err := os.MkdirAll(attachmentPath, 0o755); err != nil {
		t.Fatal(err)
	}
	msgDir := filepath.Join(base, "msg")
	if err := os.MkdirAll(msgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	dest, err := rns.NewDestination(ts, id, rns.DestinationIn, rns.DestinationSingle, "lxmf", "delivery")
	if err != nil {
		t.Fatal(err)
	}
	msg, err := lxmf.NewMessage(dest, dest, "b", "t", nil)
	if err != nil {
		t.Fatal(err)
	}
	msg.Fields = map[any]any{lxmf.FieldImage: []byte("rawimg")}
	if err := msg.Pack(); err != nil {
		t.Fatal(err)
	}
	if err := ExtractAttachmentsFromLXM(msg, attachmentPath); err != nil {
		t.Fatal(err)
	}
	// second extraction should be a no-op (dir already exists)
	if err := ExtractAttachmentsFromLXM(msg, attachmentPath); err != nil {
		t.Fatal(err)
	}
}

func TestExtractAttachmentsNoAttachments(t *testing.T) {
	t.Parallel()
	base := tempDir(t)
	attachmentPath := filepath.Join(base, "attachments")
	if err := os.MkdirAll(attachmentPath, 0o755); err != nil {
		t.Fatal(err)
	}
	ts := rns.NewTransportSystem(nil)
	id, err := rns.NewIdentity(true, nil)
	if err != nil {
		t.Fatal(err)
	}
	dest, err := rns.NewDestination(ts, id, rns.DestinationIn, rns.DestinationSingle, "lxmf", "delivery")
	if err != nil {
		t.Fatal(err)
	}
	msg, err := lxmf.NewMessage(dest, dest, "b", "t", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := msg.Pack(); err != nil {
		t.Fatal(err)
	}
	// No attachment fields -> no-op, no error
	if err := ExtractAttachmentsFromLXM(msg, attachmentPath); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(attachmentPath)
	if len(entries) != 0 {
		t.Fatalf("expected no attachment dirs, got %d", len(entries))
	}
}
