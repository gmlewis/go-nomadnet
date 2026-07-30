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

// Package conversation manages LXMF conversations and their messages,
// including attachment extraction and on-disk persistence.
package conversation

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	controlCharRE  = regexp.MustCompile(`[\x00-\x1f\x7f]`)
	pathSeparateRE = regexp.MustCompile(`[/\\]`)
	nonAlnumRE     = regexp.MustCompile(`[^A-Za-z0-9]`)
)

// SafeAttachmentName sanitizes an arbitrary attachment filename so it is safe
// to store on disk. It mirrors the Python NomadNet safe_attachment_name: bytes
// inputs are decoded as UTF-8 (with replacement), path separators and colons
// are stripped, leading dots are removed, and over-long names are truncated
// while preserving a short extension. When the result is empty, a reserved
// name, or otherwise invalid, fallback is returned instead.
func SafeAttachmentName(name any, fallback string) string {
	var s string
	switch v := name.(type) {
	case nil:
		s = ""
	case []byte:
		s = decodeBytes(v)
	case string:
		s = v
	default:
		s = fmt.Sprintf("%v", v)
	}
	s = controlCharRE.ReplaceAllString(s, "")
	parts := pathSeparateRE.Split(s, -1)
	if len(parts) > 0 {
		s = parts[len(parts)-1]
	} else {
		s = ""
	}
	if idx := strings.LastIndex(s, ":"); idx >= 0 {
		s = s[idx+1:]
	}
	s = strings.TrimLeft(s, ".")
	if s == "" || s == "." || s == ".." {
		return fallback
	}
	if len(s) > 200 {
		ext := filepath.Ext(s)
		if len(ext) > 16 {
			ext = ext[:16]
		}
		base := strings.TrimSuffix(s, filepath.Ext(s))
		s = base[:200-len(ext)] + ext
	}
	return s
}

func decodeBytes(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	// Replace invalid UTF-8 sequences with U+FFFD, mirroring Python's
	// errors="replace".
	var sb strings.Builder
	for len(b) > 0 {
		r, size := utf8.DecodeRune(b)
		if r == utf8.RuneError && size == 1 {
			sb.WriteRune('\uFFFD')
			b = b[1:]
			continue
		}
		sb.WriteRune(r)
		b = b[size:]
	}
	return sb.String()
}

// DetectImageExt inspects magic bytes and returns the matching image file
// extension, defaulting to ".bin" when the data is too short or unrecognized.
func DetectImageExt(data []byte) string {
	if len(data) < 12 {
		return ".bin"
	}
	if bytesEqual(data[:8], []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}) {
		return ".png"
	}
	if bytesEqual(data[:3], []byte{0xff, 0xd8, 0xff}) {
		return ".jpg"
	}
	if bytesEqual(data[:4], []byte("GIF8")) {
		return ".gif"
	}
	if bytesEqual(data[:4], []byte("RIFF")) && bytesEqual(data[8:12], []byte("WEBP")) {
		return ".webp"
	}
	if bytesEqual(data[:4], []byte{0x00, 0x00, 0x00, 0x1c}) || bytesEqual(data[:4], []byte{0x00, 0x00, 0x00, 0x18}) {
		return ".heic"
	}
	return ".bin"
}

// DetectAudioExt inspects magic bytes and returns the matching audio file
// extension, defaulting to ".bin" when the data is too short or unrecognized.
func DetectAudioExt(data []byte) string {
	if len(data) < 12 {
		return ".bin"
	}
	if bytesEqual(data[:4], []byte("OggS")) {
		return ".ogg"
	}
	if bytesEqual(data[:2], []byte{0xff, 0xfb}) || bytesEqual(data[:3], []byte("ID3")) {
		return ".mp3"
	}
	if bytesEqual(data[:4], []byte("RIFF")) && bytesEqual(data[8:12], []byte("WAVE")) {
		return ".wav"
	}
	if bytesEqual(data[:4], []byte("fLaC")) {
		return ".flac"
	}
	return ".bin"
}

// ExtFromMediaFormat maps a media format descriptor to a file extension. A
// non-empty string format is sanitized to alphanumerics (lower-cased, up to 8
// runes). An integer format selects a named audio mode when isAudio is true.
// Otherwise the extension is detected from the data bytes, choosing image or
// audio detection based on isAudio.
func ExtFromMediaFormat(fmtVal any, data []byte, isAudio bool) string {
	switch f := fmtVal.(type) {
	case string:
		if len(f) > 0 {
			safe := strings.ToLower(nonAlnumRE.ReplaceAllString(f, ""))
			if len(safe) > 8 {
				safe = safe[:8]
			}
			if safe != "" {
				return "." + safe
			}
		}
	case int:
		if isAudio {
			if f >= 16 && f <= 25 {
				return ".ogg"
			}
			if f >= 1 && f <= 9 {
				return ".c2"
			}
		}
	case int64:
		if isAudio {
			iv := int(f)
			if iv >= 16 && iv <= 25 {
				return ".ogg"
			}
			if iv >= 1 && iv <= 9 {
				return ".c2"
			}
		}
	}
	if isAudio {
		return DetectAudioExt(data)
	}
	return DetectImageExt(data)
}

// UnpackMediaField normalizes a FIELD_IMAGE or FIELD_AUDIO value, which may be
// raw bytes or a [format, bytes] pair. It returns the format descriptor and the
// data bytes, or (nil, nil) when the value is not a recognized media field.
func UnpackMediaField(fieldData any) (any, []byte) {
	switch v := fieldData.(type) {
	case []byte:
		return nil, v
	case []any:
		if len(v) >= 2 {
			data, ok := v[1].([]byte)
			if ok {
				return v[0], data
			}
		}
	}
	return nil, nil
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
