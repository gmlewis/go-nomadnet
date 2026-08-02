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
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// SaveAttachmentSelection identifies one attachment to copy out of a
// conversation: the owning message's LXMF hash (to locate the extracted
// attachment directory), the field type + field index within that message (to
// select the extracted file_N), and the display name to use for the saved
// copy. It mirrors the per-checkbox data Python's do_save reads from each
// attachment ref (Conversations.py:2358-2379).
type SaveAttachmentSelection struct {
	MessageHash []byte
	FieldType   string
	FieldIndex  int
	Name        string
}

// SaveAttachmentsToDir copies the selected attachments for this conversation
// into destDir, mirroring Python's do_save (Conversations.py:2368-2391). For
// each selection it locates the owning message by hash, resolves the
// extracted attachment file via GetAttachmentFilePath, and copies it to destDir
// with a sanitized, collision-avoiding name. It returns the saved destination
// paths and a count of failures (unknown message hash or missing/unreadable
// source file); one failure does not abort the remaining copies (Python
// collects errors and continues).
func (c *Conversation) SaveAttachmentsToDir(selections []SaveAttachmentSelection, destDir string) (saved []string, failed int) {
	for _, sel := range selections {
		msg := c.findMessageByHash(sel.MessageHash)
		if msg == nil {
			failed++
			continue
		}
		src := msg.GetAttachmentFilePath(sel.FieldType, sel.FieldIndex)
		if src == "" || !isFile(src) {
			failed++
			continue
		}
		path, err := CopyAttachmentToDest(sel.Name, src, destDir)
		if err != nil {
			failed++
			continue
		}
		saved = append(saved, path)
	}
	return saved, failed
}

// findMessageByHash returns the first message in this conversation whose hash
// matches, or nil. Compares against the parsed LXMF hash (GetHash).
func (c *Conversation) findMessageByHash(hash []byte) *Message {
	if len(hash) == 0 {
		return nil
	}
	for _, m := range c.Messages {
		if bytes.Equal(m.GetHash(), hash) {
			return m
		}
	}
	return nil
}

// CopyAttachmentToDest copies the file at srcPath into saveDir under a
// sanitized, collision-avoiding name derived from filename, mirroring Python's
// _copy_attachment_to_dest + _resolve_attachment_save_path
// (Conversations.py:2871-2894). saveDir is created when missing. When a file
// already exists at the resolved safe name, an "_N" counter is inserted before
// the extension (report.txt → report_1.txt → report_2.txt). The destination is
// guarded to stay within saveDir (a sanitized name that would escape is
// rejected with the underlying errno-style error). Returns the absolute
// destination path.
func CopyAttachmentToDest(filename, srcPath, saveDir string) (string, error) {
	if err := os.MkdirAll(saveDir, 0o755); err != nil {
		return "", err
	}
	savePath, err := resolveAttachmentSavePath(filename, saveDir)
	if err != nil {
		return "", err
	}
	if err := copyFile(srcPath, savePath); err != nil {
		return "", err
	}
	return savePath, nil
}

// resolveAttachmentSavePath resolves a safe, unique destination path for
// filename under saveDir, mirroring Python's _resolve_attachment_save_path
// (Conversations.py:2871-2888): sanitize the name, guard against path
// traversal via realpath, and append an "_N" counter on collision.
func resolveAttachmentSavePath(filename, saveDir string) (string, error) {
	safeName := SafeAttachmentName(filename, "attachment")
	baseDir := filepath.Clean(saveDir) + string(filepath.Separator)
	candidate := filepath.Clean(filepath.Join(saveDir, safeName))
	if !startsWithDir(candidate, baseDir) {
		return "", os.ErrPermission
	}
	counter := 0
	ext := filepath.Ext(safeName)
	base := safeName[:len(safeName)-len(ext)]
	for isFile(candidate) {
		counter++
		candidate = filepath.Clean(filepath.Join(saveDir, fmt.Sprintf("%s_%d%s", base, counter, ext)))
		if !startsWithDir(candidate, baseDir) {
			return "", os.ErrPermission
		}
	}
	return candidate, nil
}

// startsWithDir reports whether path is equal to or nested under baseDir
// (baseDir must end in a separator), guarding against traversal escapes.
func startsWithDir(path, baseDir string) bool {
	if path == baseDir {
		return true
	}
	return len(path) > len(baseDir) && path[:len(baseDir)] == baseDir
}

// copyFile copies the contents and mode of src to dst, mirroring shutil.copy2
// (content + permission bits). It does not preserve mtime/atime.
func copyFile(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { err = in.Close() }()
	info, err := in.Stat()
	if err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
