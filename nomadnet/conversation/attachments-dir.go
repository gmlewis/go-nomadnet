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
	"regexp"

	"github.com/gmlewis/go-reticulum/lxmf"
	rnsmsgpack "github.com/gmlewis/go-reticulum/rns/msgpack"
)

var storedNameRE = regexp.MustCompile(`^file_\d+$`)

// attachmentDir returns the per-message attachment directory path, derived
// from the message's AttachmentPath and its hash. It returns an empty string
// when no attachment path is set or no hash is available. The attachment path
// is stamped onto each message by ConversationCache.Store (via
// Conversation.ScanStorage), so the conversation package never needs a
// package-level provider to locate attachments. This mirrors the Python
// NomadNet _attachment_dir.
func (m *Message) attachmentDir() string {
	base := m.AttachmentPath
	if base == "" {
		return ""
	}
	hash := m.GetHash()
	if len(hash) == 0 {
		return ""
	}
	return filepath.Join(base, hex.EncodeToString(hash))
}

// readAttachmentManifest reads and sanitizes the manifest file within an
// attachment directory, mirroring the Python NomadNet _read_attachment_manifest.
// Names are sanitized and stored names are validated against the file_N form.
// It returns nil when the manifest is absent or invalid.
func (m *Message) readAttachmentManifest(attDir string) map[any]any {
	manifestPath := filepath.Join(attDir, "manifest")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil
	}
	raw, err := rnsmsgpack.Unpack(data)
	if err != nil {
		return nil
	}
	manifest, ok := raw.(map[any]any)
	if !ok {
		return nil
	}
	rawFiles, _ := manifest["files"].([]any)
	safeFiles := make([]any, 0, len(rawFiles))
	for idx, entry := range rawFiles {
		e, ok := entry.(map[any]any)
		if !ok {
			continue
		}
		name := SafeAttachmentName(e["name"], fallbackName(idx))
		stored, _ := e["stored_name"].(string)
		if !storedNameRE.MatchString(stored) {
			continue
		}
		e["name"] = name
		safeFiles = append(safeFiles, e)
	}
	manifest["files"] = safeFiles
	return manifest
}

func fallbackName(idx int) string {
	return "attachment_" + itoaSimple(idx)
}

func itoaSimple(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// GetImage returns the image attachment bytes for the message, preferring an
// extracted attachment directory file and falling back to the parsed LXMF
// image field. This mirrors the Python NomadNet get_image.
func (m *Message) GetImage() []byte {
	attDir := m.attachmentDir()
	if attDir != "" {
		if isFile(filepath.Join(attDir, "image")) {
			data, err := os.ReadFile(filepath.Join(attDir, "image"))
			if err == nil {
				return data
			}
		}
	}
	fields := m.GetFields()
	if v, ok := fieldLookup(fields, lxmf.FieldImage); ok {
		_, data := UnpackMediaField(v)
		return data
	}
	return nil
}

// GetAudio returns the audio attachment bytes for the message, preferring an
// extracted attachment directory file and falling back to the parsed LXMF
// audio field. This mirrors the Python NomadNet get_audio.
func (m *Message) GetAudio() []byte {
	attDir := m.attachmentDir()
	if attDir != "" {
		if isFile(filepath.Join(attDir, "audio")) {
			data, err := os.ReadFile(filepath.Join(attDir, "audio"))
			if err == nil {
				return data
			}
		}
	}
	fields := m.GetFields()
	if v, ok := fieldLookup(fields, lxmf.FieldAudio); ok {
		_, data := UnpackMediaField(v)
		return data
	}
	return nil
}

// GetAttachmentFilePath returns the on-disk path for the attachment of the given
// field type and index, or an empty string when none is found. It mirrors the
// Python NomadNet get_attachment_file_path, consulting the extracted attachment
// manifest first and falling back to the legacy "image"/"audio" file layout.
func (m *Message) GetAttachmentFilePath(fieldType string, fieldIndex int) string {
	attDir := m.attachmentDir()
	if attDir != "" && isDir(attDir) {
		manifest := m.readAttachmentManifest(attDir)
		if manifest != nil {
			if files, ok := manifest["files"].([]any); ok && fieldIndex < len(files) {
				if entry, ok := files[fieldIndex].(map[any]any); ok {
					if stored, ok := entry["stored_name"].(string); ok {
						return filepath.Join(attDir, stored)
					}
				}
			}
		}
		// Fallback for old extraction format.
		switch fieldType {
		case "image":
			p := filepath.Join(attDir, "image")
			if isFile(p) {
				return p
			}
		case "audio":
			p := filepath.Join(attDir, "audio")
			if isFile(p) {
				return p
			}
		}
	}
	return ""
}

// ExtractAttachmentsFromLXM extracts file/image/audio fields from an LXMF
// message into a per-message attachment directory under attachmentPath, writing
// a msgpack manifest. When the directory already exists it is a no-op. This
// mirrors the Python NomadNet extract_attachments_from_lxm.
func ExtractAttachmentsFromLXM(lxm *lxmf.Message, attachmentPath string) error {
	if lxm == nil || lxm.Fields == nil {
		return nil
	}
	fields := lxm.Fields
	_, hasFile := fieldLookup(fields, lxmf.FieldFileAttachments)
	_, hasImage := fieldLookup(fields, lxmf.FieldImage)
	_, hasAudio := fieldLookup(fields, lxmf.FieldAudio)
	if !hasFile && !hasImage && !hasAudio {
		return nil
	}

	if len(lxm.Hash) == 0 {
		return nil
	}
	attDir := filepath.Join(attachmentPath, hex.EncodeToString(lxm.Hash))
	if isDir(attDir) {
		return nil
	}
	if err := os.MkdirAll(attDir, 0o755); err != nil {
		return err
	}

	var entries []any

	if fv, ok := fieldLookup(fields, lxmf.FieldFileAttachments); ok {
		fileAtts, _ := fv.([]any)
		for idx, att := range fileAtts {
			entry, ok := att.([]any)
			if !ok || len(entry) < 2 {
				continue
			}
			data, _ := entry[1].([]byte)
			name := SafeAttachmentName(entry[0], fallbackName(idx))
			stored := "file_" + itoaSimple(idx)
			if err := os.WriteFile(filepath.Join(attDir, stored), data, 0o644); err != nil {
				return err
			}
			entries = append(entries, attachmentManifestEntry(name, stored, len(data)))
		}
	}

	if imageField, ok := fieldLookup(fields, lxmf.FieldImage); ok {
		fmtVal, data := UnpackMediaField(imageField)
		if data != nil {
			ext := ExtFromMediaFormat(fmtVal, data, false)
			name := SafeAttachmentName("image"+ext, "image")
			stored := "file_" + itoaSimple(len(entries))
			if err := os.WriteFile(filepath.Join(attDir, stored), data, 0o644); err != nil {
				return err
			}
			entries = append(entries, attachmentManifestEntry(name, stored, len(data)))
		}
	}

	if audioField, ok := fieldLookup(fields, lxmf.FieldAudio); ok {
		fmtVal, data := UnpackMediaField(audioField)
		if data != nil {
			ext := ExtFromMediaFormat(fmtVal, data, true)
			name := SafeAttachmentName("audio"+ext, "audio")
			stored := "file_" + itoaSimple(len(entries))
			if err := os.WriteFile(filepath.Join(attDir, stored), data, 0o644); err != nil {
				return err
			}
			entries = append(entries, attachmentManifestEntry(name, stored, len(data)))
		}
	}

	// The manifest is an insertion-ordered map {"files": [...]} (mirroring
	// Python's manifest = {"files": []} ... msgpack.packb(manifest)); each file
	// entry is itself an OrderedMap keyed name, stored_name, size in that
	// order. size is widened to uint so its msgpack encoding matches Python's
	// unsigned encoding for non-negative values (a Go int would pack sizes
	// above 0x7f as signed int16/int32 and diverge).
	manifest := rnsmsgpack.OrderedMap{{Key: "files", Value: entries}}
	manifestData, err := rnsmsgpack.Pack(manifest)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(attDir, "manifest"), manifestData, 0o644)
}

// attachmentManifestEntry builds one manifest file entry as an OrderedMap keyed
// in Python's insertion order (name, stored_name, size), with size as uint.
func attachmentManifestEntry(name, storedName string, size int) rnsmsgpack.OrderedMap {
	return rnsmsgpack.OrderedMap{
		{Key: "name", Value: name},
		{Key: "stored_name", Value: storedName},
		{Key: "size", Value: uint(size)},
	}
}

// fieldLookup looks up a field key in a parsed LXMF fields map, tolerating the
// integer key type variations (int/int64/uint8/uint64) produced by msgpack
// round-tripping.
func fieldLookup(fields map[any]any, key int) (any, bool) {
	if fields == nil {
		return nil, false
	}
	if v, ok := fields[key]; ok {
		return v, true
	}
	for k, v := range fields {
		switch c := k.(type) {
		case int:
			if c == key {
				return v, true
			}
		case int64:
			if c == int64(key) {
				return v, true
			}
		case uint8:
			if int(c) == key {
				return v, true
			}
		case uint64:
			if int(c) == key {
				return v, true
			}
		}
	}
	return nil, false
}

func isFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}
